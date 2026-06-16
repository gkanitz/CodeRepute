/**
 * verify.js — CodeRepute report verification state machine.
 *
 * Pure ES module with no external dependencies. Can be imported in tests
 * (Node 18+) and inlined into the static HTML page.
 *
 * Exported surface:
 *   classifyReport(report)                       → VerifyClass
 *   fetchAttestation(repo, digest, fetchFn?)      → Promise<attestation | null>
 *   fetchRekorEntries(digest, fetchFn?)           → Promise<RekorEntry[]>
 *   extractWorkflowRefFromCert(certBytes)         → string | null
 *   extractWorkflowRefFromRekorEntry(entry)       → string | null
 *   verifyReport(report, rawBytes, fetchFn?)      → Promise<VerifyResult>
 *   sha256Hex(bytes)                              → Promise<string>
 *
 * Durability: when GitHub's attestation API can't resolve a report's
 * attestation (deleted repo/org, 404, any resolution failure),
 * verifyReport() falls back to querying Sigstore's public Rekor
 * transparency log directly by the artifact's SHA-256 digest before
 * concluding the report is tampered. This is a read-only lookup against
 * an existing public third-party service — no CodeRepute-operated
 * infrastructure is introduced. See docs/verification.md.
 */

// --- constants ---------------------------------------------------------------

/** The only workflow ref prefix that is considered canonical. */
export const CANONICAL_WORKFLOW =
  "grkanitz/CodeRepute/.github/workflows/coderepute-report.yml";

/** GitHub public attestation API base URL. */
export const GITHUB_ATTESTATION_API =
  "https://api.github.com/repos/{owner}/{repo}/attestations/sha256:{digest}";

// Verification result classes.
export const CLASS_VERIFIED = "verified";
export const CLASS_TAMPERED = "tampered";
export const CLASS_NON_CANONICAL = "non-canonical";
export const CLASS_UNVERIFIABLE = "unverifiable";
export const CLASS_ERROR = "error";
/** Verified via the Rekor transparency-log fallback rather than GitHub's API. */
export const CLASS_VERIFIED_VIA_REKOR = "verified-via-rekor";
/** Neither GitHub nor Rekor could be reached/queried — not a verdict either way. */
export const CLASS_VERIFY_UNAVAILABLE = "verify-unavailable";

/** Sigstore's public Rekor transparency log REST API base. */
export const REKOR_INDEX_API = "https://rekor.sigstore.dev/api/v1/index/retrieve";
export const REKOR_ENTRY_API = "https://rekor.sigstore.dev/api/v1/log/entries/{uuid}";

// --- SHA-256 -----------------------------------------------------------------

/**
 * Compute the SHA-256 hex digest of raw bytes using the Web Crypto API
 * (available in browsers and Node 18+).
 *
 * @param {Uint8Array} bytes
 * @returns {Promise<string>} lowercase hex string
 */
export async function sha256Hex(bytes) {
  const buffer = await crypto.subtle.digest("SHA-256", bytes);
  return Array.from(new Uint8Array(buffer))
    .map((b) => b.toString(16).padStart(2, "0"))
    .join("");
}

// --- attestation API ---------------------------------------------------------

/**
 * Fetch the attestation bundle for a given digest from the GitHub public API.
 * Returns the first attestation object, or null when none exists.
 *
 * @param {string} repo  "owner/repo" format
 * @param {string} digest  SHA-256 hex string (no prefix)
 * @param {function} fetchFn  injectable fetch for testing (default: globalThis.fetch)
 * @returns {Promise<object|null>}
 */
export async function fetchAttestation(repo, digest, fetchFn = globalThis.fetch) {
  const [owner, repoName] = repo.split("/");
  if (!owner || !repoName) return null;

  const url = GITHUB_ATTESTATION_API
    .replace("{owner}", encodeURIComponent(owner))
    .replace("{repo}", encodeURIComponent(repoName))
    .replace("{digest}", encodeURIComponent(digest));

  let response;
  try {
    response = await fetchFn(url, {
      headers: { Accept: "application/json" },
    });
  } catch (err) {
    throw new Error(`Network error contacting GitHub attestation API: ${err.message}`);
  }

  if (response.status === 404) return null;
  if (!response.ok) {
    throw new Error(`GitHub attestation API returned HTTP ${response.status}`);
  }

  const data = await response.json();
  const attestations = data?.attestations;
  if (!Array.isArray(attestations) || attestations.length === 0) return null;
  return attestations[0];
}

// --- Rekor transparency-log fallback ------------------------------------------

/**
 * Query Sigstore's public Rekor transparency log for entries matching a
 * SHA-256 digest, and resolve each matching UUID to its full log entry.
 *
 * This is a read-only lookup against an existing public third-party
 * service (no CodeRepute-operated infrastructure involved). Every report
 * attested via `actions/attest-build-provenance` has a corresponding
 * Rekor entry as a side effect of that action.
 *
 * @param {string} digest  SHA-256 hex string (no prefix)
 * @param {function} fetchFn  injectable fetch for testing (default: globalThis.fetch)
 * @returns {Promise<object[]>}  array of decoded entries (possibly empty)
 */
export async function fetchRekorEntries(digest, fetchFn = globalThis.fetch) {
  let uuids;
  try {
    const indexResponse = await fetchFn(REKOR_INDEX_API, {
      method: "POST",
      headers: { "Content-Type": "application/json", Accept: "application/json" },
      body: JSON.stringify({ hash: `sha256:${digest}` }),
    });
    if (!indexResponse.ok) {
      throw new Error(`Rekor index API returned HTTP ${indexResponse.status}`);
    }
    uuids = await indexResponse.json();
  } catch (err) {
    throw new Error(`Network error contacting Rekor index API: ${err.message}`);
  }

  if (!Array.isArray(uuids) || uuids.length === 0) return [];

  const entries = [];
  for (const uuid of uuids) {
    const url = REKOR_ENTRY_API.replace("{uuid}", encodeURIComponent(uuid));
    let entryResponse;
    try {
      entryResponse = await fetchFn(url, { headers: { Accept: "application/json" } });
    } catch (err) {
      throw new Error(`Network error contacting Rekor log entries API: ${err.message}`);
    }
    if (!entryResponse.ok) {
      throw new Error(`Rekor log entries API returned HTTP ${entryResponse.status}`);
    }
    const data = await entryResponse.json();
    const raw = data?.[uuid];
    if (!raw) {
      throw new Error(`Rekor log entries API response did not contain UUID ${uuid}`);
    }

    let body;
    try {
      body = JSON.parse(atob(raw.body));
    } catch (err) {
      throw new Error(`Failed to decode Rekor entry body for UUID ${uuid}: ${err.message}`);
    }

    entries.push({
      uuid,
      logIndex: raw.logIndex,
      integratedTime: raw.integratedTime,
      kind: body?.kind ?? null,
      body,
    });
  }

  return entries;
}

/**
 * DER encoding of Fulcio's "Build Signer URI" X.509v3 extension OID
 * (1.3.6.1.4.1.57264.1.9), as an ASN.1 OBJECT IDENTIFIER TLV:
 * tag 0x06, length 0x0a, then the base-128 arc encoding of the OID.
 *
 * This is GitHub Actions' job_workflow_ref, expressed as a URI such as
 * "https://github.com/{org}/{repo}/.github/workflows/{file}.yml@{ref}".
 * See https://github.com/sigstore/fulcio/blob/main/docs/oid-info.md
 *
 * Verified by generating a real Fulcio-shaped cert with openssl and
 * locating these exact bytes preceding the extnValue.
 */
const FULCIO_BUILD_SIGNER_URI_OID_DER = Uint8Array.from([
  0x06, 0x0a, 0x2b, 0x06, 0x01, 0x04, 0x01, 0x83, 0xbf, 0x30, 0x01, 0x09,
]);

/**
 * Extract the GitHub Actions job workflow ref URI from a raw DER-encoded
 * X.509 certificate, by scanning for Fulcio's custom "Build Signer URI"
 * extension (OID 1.3.6.1.4.1.57264.1.9).
 *
 * This is intentionally NOT a general ASN.1/DER parser. It performs a
 * bounded, scoped scan: locate the OID's exact byte sequence, then walk
 * forward through the well-known TLV shape Fulcio always emits for this
 * extension (an OCTET STRING wrapping a UTF8String/IA5String payload).
 * Any structural surprise causes this function to return null rather
 * than throw or guess — callers must treat null as "could not
 * independently re-derive identity", never as a verified match.
 *
 * @param {Uint8Array} certBytes  raw DER bytes of the signing certificate
 * @returns {string|null}
 */
export function extractWorkflowRefFromCert(certBytes) {
  try {
    if (!certBytes || certBytes.length === 0) return null;
    const bytes = certBytes instanceof Uint8Array ? certBytes : new Uint8Array(certBytes);

    const oidStart = indexOfSequence(bytes, FULCIO_BUILD_SIGNER_URI_OID_DER);
    if (oidStart === -1) return null;

    let pos = oidStart + FULCIO_BUILD_SIGNER_URI_OID_DER.length;

    // Optional: a BOOLEAN (critical flag) TLV may follow the OID before
    // the extnValue OCTET STRING. Skip it if present (tag 0x01).
    if (bytes[pos] === 0x01) {
      const [, boolLen, boolHeaderLen] = readTLV(bytes, pos);
      pos += boolHeaderLen + boolLen;
    }

    // extnValue: OCTET STRING (tag 0x04) wrapping the actual string.
    const [octetTag, octetLen, octetHeaderLen] = readTLV(bytes, pos);
    if (octetTag !== 0x04) return null;
    const octetValueStart = pos + octetHeaderLen;

    // Inside the OCTET STRING: a UTF8String (0x0c) or IA5String (0x16).
    const [innerTag, innerLen, innerHeaderLen] = readTLV(bytes, octetValueStart);
    if (innerTag !== 0x0c && innerTag !== 0x16) return null;
    const strStart = octetValueStart + innerHeaderLen;
    const strBytes = bytes.slice(strStart, strStart + innerLen);

    const text = new TextDecoder("utf-8", { fatal: true }).decode(strBytes);
    return text || null;
  } catch {
    // Any parsing surprise → honest "could not extract", never a guess.
    return null;
  }
}

/** Find the start index of `needle` within `haystack`, or -1. */
function indexOfSequence(haystack, needle) {
  outer: for (let i = 0; i <= haystack.length - needle.length; i++) {
    for (let j = 0; j < needle.length; j++) {
      if (haystack[i + j] !== needle[j]) continue outer;
    }
    return i;
  }
  return -1;
}

/**
 * Read a single DER TLV (tag-length-value) header at `pos`.
 * Supports short-form and long-form definite lengths (sufficient for the
 * small extension values we scan; certs never use indefinite length).
 *
 * @returns {[number, number, number]} [tag, contentLength, headerLength]
 */
function readTLV(bytes, pos) {
  const tag = bytes[pos];
  const firstLenByte = bytes[pos + 1];
  if (firstLenByte == null) throw new Error("truncated DER TLV");
  if ((firstLenByte & 0x80) === 0) {
    // Short form: length is the byte itself.
    return [tag, firstLenByte, 2];
  }
  // Long form: low 7 bits = number of subsequent length bytes.
  const numLenBytes = firstLenByte & 0x7f;
  if (numLenBytes === 0 || numLenBytes > 4) throw new Error("unsupported DER length form");
  let length = 0;
  for (let i = 0; i < numLenBytes; i++) {
    length = (length << 8) | bytes[pos + 2 + i];
  }
  return [tag, length, 2 + numLenBytes];
}

/**
 * Extract the embedded signing certificate from a decoded Rekor entry and
 * delegate to extractWorkflowRefFromCert. Handles both entry kinds Rekor
 * commonly returns for Sigstore-signed artifacts:
 *   - "hashedrekord": cert lives at spec.signature.publicKey.content
 *   - "dsse":          cert lives at spec.signatures[].verifier
 * Neither kind's read-mode schema exposes a friendlier JSON identity
 * field (confirmed against Rekor's published entry-type schemas), so
 * both paths bottom out in parsing the same base64 DER certificate.
 *
 * @param {object} entry  one decoded entry from fetchRekorEntries
 * @returns {string|null}
 */
export function extractWorkflowRefFromRekorEntry(entry) {
  try {
    const spec = entry?.body?.spec;
    if (!spec) return null;

    let certB64 = spec?.signature?.publicKey?.content ?? null;
    if (!certB64) {
      const verifiers = spec?.signatures;
      if (Array.isArray(verifiers) && verifiers.length > 0) {
        certB64 = verifiers[0]?.verifier ?? null;
      }
    }
    if (!certB64) return null;

    const certBytes = Uint8Array.from(atob(certB64), (c) => c.charCodeAt(0));
    const ref = extractWorkflowRefFromCert(certBytes);
    return normalizeFulcioWorkflowRef(ref);
  } catch {
    return null;
  }
}

/**
 * Fulcio's Build Signer URI extension carries a full URI
 * ("https://github.com/{org}/{repo}/.github/workflows/{file}.yml@{ref}"),
 * while GitHub's own attestation API surfaces the bare
 * "{org}/{repo}/.github/workflows/{file}.yml@{ref}" form (matching
 * CANONICAL_WORKFLOW). Normalize so both paths compare and render the
 * same way.
 *
 * @param {string|null} ref
 * @returns {string|null}
 */
function normalizeFulcioWorkflowRef(ref) {
  if (!ref) return ref;
  const prefix = "https://github.com/";
  return ref.startsWith(prefix) ? ref.slice(prefix.length) : ref;
}

// --- classification ----------------------------------------------------------

/**
 * Classify a parsed report without performing any network calls.
 * This lets us short-circuit unverifiable reports before touching the network.
 *
 * @param {object} report  parsed JSON object
 * @returns {string}  one of the CLASS_* constants
 */
export function classifyReport(report) {
  const v = report?.verification;
  if (!v) return CLASS_UNVERIFIABLE;

  if (v.status === "unverified" || !v.provider || v.provider !== "github-actions") {
    return CLASS_UNVERIFIABLE;
  }

  // Has a claimed verified status with github-actions provider.
  // We still need to check the workflow ref for canonicity.
  const ref = v.workflow_ref ?? "";
  if (!ref.startsWith(CANONICAL_WORKFLOW)) {
    return CLASS_NON_CANONICAL;
  }

  // Potentially verifiable — needs network check.
  return null; // caller should proceed with network check
}

// --- full verification -------------------------------------------------------

/**
 * @typedef {object} VerifyResult
 * @property {string}  status       — one of the CLASS_* constants
 * @property {string}  [org]        — owning org (from verification.repository)
 * @property {string}  [subject]    — subject username from report
 * @property {string}  [workflowRef]— verified workflow_ref from report
 * @property {string}  [runURL]     — link to the producing run
 * @property {string}  [digest]     — SHA-256 of the file
 * @property {string}  [error]      — human-readable error when status=error
 */

/**
 * Full verification pipeline:
 *  1. Hash the raw file bytes.
 *  2. Classify the report's verification block (fast path for unverifiable).
 *  3. Call the GitHub attestation API to look up the digest.
 *  4. Compare the attested workflow ref against the canonical workflow.
 *
 * @param {object}    report     parsed JSON object
 * @param {Uint8Array} rawBytes  the raw file bytes (for hashing)
 * @param {function}  [fetchFn] injectable fetch for testing
 * @returns {Promise<VerifyResult>}
 */
export async function verifyReport(report, rawBytes, fetchFn = globalThis.fetch) {
  let digest;
  try {
    digest = await sha256Hex(rawBytes);
  } catch (err) {
    return { status: CLASS_ERROR, error: `Failed to hash file: ${err.message}` };
  }

  const subject = report?.subject?.username ?? null;
  const v = report?.verification ?? {};
  const repo = v.repository ?? null;
  const workflowRef = v.workflow_ref ?? null;
  const runURL = v.run_url ?? null;
  const org = repo ? repo.split("/")[0] : null;

  // Fast classification before touching the network.
  const earlyClass = classifyReport(report);
  if (earlyClass === CLASS_UNVERIFIABLE) {
    return {
      status: CLASS_UNVERIFIABLE,
      subject,
      digest,
    };
  }
  if (earlyClass === CLASS_NON_CANONICAL) {
    return {
      status: CLASS_NON_CANONICAL,
      org,
      subject,
      workflowRef,
      runURL,
      digest,
    };
  }

  // Network check: look up the attestation by digest.
  if (!repo) {
    return {
      status: CLASS_ERROR,
      error: "Report is missing verification.repository; cannot look up attestation.",
      digest,
    };
  }

  let attestation;
  try {
    attestation = await fetchAttestation(repo, digest, fetchFn);
  } catch (err) {
    return { status: CLASS_ERROR, error: err.message, digest };
  }

  if (!attestation) {
    // GitHub couldn't resolve an attestation (404, deleted repo/org, etc).
    // Fall back to Sigstore's public Rekor transparency log before giving
    // up — every report attested via actions/attest-build-provenance has
    // a corresponding Rekor entry as a side effect of that action.
    let rekorEntries;
    try {
      rekorEntries = await fetchRekorEntries(digest, fetchFn);
    } catch (err) {
      // Rekor itself is unreachable/erroring — this is NOT the same as
      // "tampered". We genuinely don't know; never render this as a
      // pass or a fail.
      return {
        status: CLASS_VERIFY_UNAVAILABLE,
        org,
        subject,
        workflowRef,
        runURL,
        digest,
        error: `Could not reach the Rekor transparency log: ${err.message}`,
      };
    }

    if (rekorEntries.length === 0) {
      // Neither GitHub nor Rekor has anything for this digest — tampered
      // or never attested, now corroborated by two independent sources.
      return {
        status: CLASS_TAMPERED,
        org,
        subject,
        workflowRef,
        runURL,
        digest,
      };
    }

    const entry = rekorEntries[0];
    const rekorLogIndex = entry.logIndex ?? null;
    const rekorLoggedAt =
      typeof entry.integratedTime === "number"
        ? new Date(entry.integratedTime * 1000).toISOString()
        : null;
    const rekorWorkflowRef = extractWorkflowRefFromRekorEntry(entry);

    if (rekorWorkflowRef && !rekorWorkflowRef.startsWith(CANONICAL_WORKFLOW)) {
      // Rekor's entry resolves to a non-canonical signer — same verdict
      // as the GitHub path, not a Rekor-flavored pass.
      return {
        status: CLASS_NON_CANONICAL,
        org,
        subject,
        workflowRef: rekorWorkflowRef,
        runURL,
        digest,
      };
    }

    return {
      status: CLASS_VERIFIED_VIA_REKOR,
      source: "rekor",
      org,
      subject,
      workflowRef: rekorWorkflowRef ?? workflowRef,
      runURL,
      digest,
      identityConfirmed: Boolean(rekorWorkflowRef),
      rekorLogIndex,
      rekorLoggedAt,
    };
  }

  // Attestation found. Extract the signer workflow from the bundle.
  const signerWorkflow = extractSignerWorkflow(attestation);

  if (!signerWorkflow || !signerWorkflow.startsWith(CANONICAL_WORKFLOW)) {
    return {
      status: CLASS_NON_CANONICAL,
      org,
      subject,
      workflowRef: signerWorkflow ?? workflowRef,
      runURL,
      digest,
    };
  }

  return {
    status: CLASS_VERIFIED,
    org,
    subject,
    workflowRef: signerWorkflow ?? workflowRef,
    runURL,
    digest,
  };
}

/**
 * Extract the signer's job_workflow_ref from an attestation bundle.
 * GitHub's attestation API returns a bundle with the DSSE envelope;
 * the provenance predicate lives inside the decoded payload.
 *
 * We look in two places:
 *   1. bundle.verificationResult.statement.predicate.buildDefinition.externalParameters.workflow.ref
 *      (newer GitHub attestation API shape)
 *   2. The decoded DSSE payload's predicate (raw bundle)
 *
 * Returns the workflow ref string, or null when it can't be extracted.
 *
 * @param {object} attestation — one attestation object from the GitHub API
 * @returns {string|null}
 */
export function extractSignerWorkflow(attestation) {
  // Try the verificationResult path (gh attestation verify --format json shape).
  const vr = attestation?.bundle?.verificationResult;
  if (vr) {
    const wf =
      vr?.statement?.predicate?.buildDefinition?.externalParameters?.workflow;
    if (wf?.ref) return wf.ref; // "grkanitz/CodeRepute/.github/workflows/..."
    // Some shapes embed it in the signer identity.
    const jobRef = vr?.signerIdentity?.SubjectAlternativeName;
    if (jobRef) return jobRef;
  }

  // Try raw DSSE envelope: decode the payload base64 and parse the predicate.
  const envelope = attestation?.bundle?.dsseEnvelope;
  if (envelope?.payload) {
    try {
      const decoded = JSON.parse(atob(envelope.payload));
      const params =
        decoded?.predicate?.buildDefinition?.externalParameters;
      // SLSA provenance v1 shape.
      if (params?.workflow?.ref) return params.workflow.ref;
      // Older shape: buildConfig.workflowRef
      const jobRef = decoded?.predicate?.buildConfig?.workflow?.ref;
      if (jobRef) return jobRef;
    } catch {
      // ignore parse errors
    }
  }

  // Fallback: extensions field on the attestation itself (GitHub sometimes
  // surfaces job_workflow_ref here for quick checks).
  const ext = attestation?.extensions;
  if (ext?.jobWorkflowRef) return ext.jobWorkflowRef;

  return null;
}

// --- prose copy --------------------------------------------------------------

/**
 * Return the "what this proves / what it doesn't" copy for a given result status.
 *
 * @param {string} status
 * @returns {{ proves: string[], doesNotProve: string[] }}
 */
export function explainResult(status) {
  switch (status) {
    case CLASS_VERIFIED:
      return {
        proves: [
          "The bytes of this report.json are unchanged since the attested run.",
          "The report was produced inside CI of the named org/repo.",
          "The producing workflow is the canonical CodeRepute reusable workflow at a pinned version.",
        ],
        doesNotProve: [
          "Coverage completeness — check the coverage block for which repos, window, and token scope the run could see.",
          "That the underlying GitHub activity data is honest (e.g. activity manufactured before the run).",
          "Anything beyond what the token could see at run time.",
        ],
      };

    case CLASS_TAMPERED:
      return {
        proves: [],
        doesNotProve: [
          "The file cannot be trusted: no attestation was found for its current content.",
          "Either the file was modified after the attested run, or it was never attested.",
          "Do not use this report to make any trust judgement.",
        ],
      };

    case CLASS_NON_CANONICAL:
      return {
        proves: [],
        doesNotProve: [
          "The attestation was NOT produced by the canonical CodeRepute workflow (grkanitz/CodeRepute).",
          "A fork or modified copy could produce this attestation — the metrics may not be computed honestly.",
          "Do not use this report to make any trust judgement without manually inspecting the producing workflow.",
        ],
      };

    case CLASS_UNVERIFIABLE:
      return {
        proves: [],
        doesNotProve: [
          "This report carries no CI attestation (it was produced outside GitHub Actions, or on an unsupported platform).",
          "The content may be accurate but there is no cryptographic proof.",
          "Treat this as an unverified claim only.",
        ],
      };

    case CLASS_VERIFIED_VIA_REKOR:
      // Note: this copy intentionally does not claim the producing
      // workflow identity was confirmed — that depends on
      // result.identityConfirmed, which only the caller (with the full
      // VerifyResult) can know. See index.html's showResult() for the
      // identityConfirmed-specific sub-copy layered on top of this.
      return {
        proves: [
          "GitHub's attestation API could not resolve this report's attestation (the producing repo or org may have been deleted, renamed, or made private), but a matching entry was found in Sigstore's public Rekor transparency log by the file's SHA-256 digest.",
          "The bytes of this report.json are unchanged since that Rekor entry was logged — the same integrity guarantee as the GitHub path, sourced independently.",
          "The original GitHub Actions run that produced this report can no longer be checked directly; this result is sourced from Rekor, not from GitHub's API.",
        ],
        doesNotProve: [
          "Coverage completeness — check the coverage block for which repos, window, and token scope the run could see.",
          "That the underlying GitHub activity data is honest (e.g. activity manufactured before the run).",
          "Rekor is public-good infrastructure with no contractual uptime guarantee; this result reflects what its public log currently shows.",
        ],
      };

    case CLASS_VERIFY_UNAVAILABLE:
      return {
        proves: [],
        doesNotProve: [
          "Neither GitHub's attestation API nor Sigstore's public Rekor log could be queried right now — this is NOT a verification result.",
          "This is not the same as \"tampered\" and it is not the same as \"verified\": it means try again, possibly later, not that anything failed or passed.",
          "Do not treat this as a pass or a fail. Re-run verification once the network issue clears.",
        ],
      };

    default:
      return {
        proves: [],
        doesNotProve: ["An error occurred during verification. Check the error message above."],
      };
  }
}

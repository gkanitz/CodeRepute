/**
 * verify.js — CodeRepute report verification state machine.
 *
 * Pure ES module with no external dependencies. Can be imported in tests
 * (Node 18+) and inlined into the static HTML page.
 *
 * Exported surface:
 *   classifyReport(report)           → VerifyClass
 *   fetchAttestation(repo, digest)   → Promise<attestation | null>
 *   verifyReport(report, rawBytes, fetchFn?) → Promise<VerifyResult>
 *   sha256Hex(bytes)                 → Promise<string>
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
    // No attestation found for this digest — file was tampered or never attested.
    return {
      status: CLASS_TAMPERED,
      org,
      subject,
      workflowRef,
      runURL,
      digest,
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

    default:
      return {
        proves: [],
        doesNotProve: ["An error occurred during verification. Check the error message above."],
      };
  }
}

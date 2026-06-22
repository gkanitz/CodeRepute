/**
 * verify.test.js — tests for the CodeRepute browser-side verification
 * state machine (verify.js).
 *
 * Run with: node --test docs/verify/verify.test.js
 * Requires Node 18+ (node:test, node:crypto, native fetch or custom fetch stub).
 */

import { describe, it, before } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";

// Polyfill Web Crypto + atob for Node (they are globals in Node 18+ but
// making them explicit keeps the module import simpler).
import { webcrypto } from "node:crypto";
if (!globalThis.crypto) {
  globalThis.crypto = webcrypto;
}
if (!globalThis.atob) {
  globalThis.atob = (b64) => Buffer.from(b64, "base64").toString("binary");
}

import {
  CANONICAL_WORKFLOW,
  CLASS_VERIFIED,
  CLASS_TAMPERED,
  CLASS_NON_CANONICAL,
  CLASS_UNVERIFIABLE,
  CLASS_ERROR,
  CLASS_VERIFIED_VIA_REKOR,
  CLASS_VERIFY_UNAVAILABLE,
  sha256Hex,
  classifyReport,
  verifyReport,
  extractSignerWorkflow,
  fetchRekorEntries,
  extractWorkflowRefFromCert,
  explainResult,
  verifyHTML,
  verifyPDF,
  verifyFile,
  extractRepoFromPDFXMP,
  prefillFromURL,
} from "./verify.js";

const __dirname = dirname(fileURLToPath(import.meta.url));
const fixture = (name) =>
  readFileSync(join(__dirname, "testdata", name));
const fixtureText = (name) =>
  readFileSync(join(__dirname, "testdata", name), "utf8").trim();

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Build a minimal fake fetch that returns the given attestation payload. */
function fakeAttestationFetch(attestation) {
  return async (_url, _opts) => ({
    ok: true,
    status: 200,
    json: async () => ({
      attestations: attestation ? [attestation] : [],
    }),
  });
}

/**
 * A fake fetch that returns 404 for GitHub's attestation API, and routes
 * Rekor calls to a (by default empty) Rekor responder — i.e. neither
 * source has anything, the honest "tampered" case.
 */
function noAttestationFetch(rekorFetch = emptyRekorFetch()) {
  return async (url, opts) => {
    if (typeof url === "string" && url.includes("rekor.sigstore.dev")) {
      return rekorFetch(url, opts);
    }
    return { ok: false, status: 404, json: async () => ({ attestations: [] }) };
  };
}

/** A fake Rekor responder that finds no log entries for any digest. */
function emptyRekorFetch() {
  return async (url) => {
    if (url.includes("/api/v1/index/retrieve")) {
      return { ok: true, status: 200, json: async () => [] };
    }
    throw new Error("unexpected Rekor URL " + url);
  };
}

/** A fake Rekor responder that throws (network failure / Rekor unreachable). */
function erroringRekorFetch() {
  return async (url) => {
    if (url.includes("/api/v1/index/retrieve")) {
      throw new Error("rekor.sigstore.dev unreachable");
    }
    throw new Error("unexpected Rekor URL " + url);
  };
}

/**
 * A fake Rekor responder that finds exactly one entry whose embedded
 * cert carries the given workflow ref fixture.
 */
function singleEntryRekorFetch(certFixtureName, { logIndex = 555, integratedTime = 1750000000 } = {}) {
  const uuid = "9".repeat(64);
  const certB64 = fixtureText(certFixtureName);
  const entryBody = {
    apiVersion: "0.0.1",
    kind: "hashedrekord",
    spec: { signature: { publicKey: { content: certB64 } } },
  };
  const encodedBody = Buffer.from(JSON.stringify(entryBody)).toString("base64");

  return async (url) => {
    if (url.includes("/api/v1/index/retrieve")) {
      return { ok: true, status: 200, json: async () => [uuid] };
    }
    if (url.includes("/api/v1/log/entries/")) {
      return {
        ok: true,
        status: 200,
        json: async () => ({ [uuid]: { logIndex, integratedTime, body: encodedBody } }),
      };
    }
    throw new Error("unexpected Rekor URL " + url);
  };
}

/** Build a minimal fake attestation bundle for the canonical workflow. */
function canonicalAttestation(workflowRef = CANONICAL_WORKFLOW + "@refs/tags/v1.0.0") {
  const payload = {
    predicate: {
      buildDefinition: {
        externalParameters: {
          workflow: { ref: workflowRef },
        },
      },
    },
  };
  return {
    bundle: {
      dsseEnvelope: {
        payload: Buffer.from(JSON.stringify(payload)).toString("base64"),
      },
    },
  };
}

// ---------------------------------------------------------------------------
// sha256Hex
// ---------------------------------------------------------------------------

describe("sha256Hex", () => {
  it("returns a 64-char lowercase hex string", async () => {
    const bytes = new TextEncoder().encode("hello");
    const digest = await sha256Hex(bytes);
    assert.equal(digest.length, 64);
    assert.match(digest, /^[0-9a-f]{64}$/);
  });

  it("returns consistent output for the same input", async () => {
    const bytes = new TextEncoder().encode("deterministic");
    assert.equal(await sha256Hex(bytes), await sha256Hex(bytes));
  });

  it("differs for different inputs", async () => {
    const a = await sha256Hex(new TextEncoder().encode("aaa"));
    const b = await sha256Hex(new TextEncoder().encode("bbb"));
    assert.notEqual(a, b);
  });
});

// ---------------------------------------------------------------------------
// classifyReport
// ---------------------------------------------------------------------------

describe("classifyReport", () => {
  it("returns unverifiable for null/missing input", () => {
    assert.equal(classifyReport(null), CLASS_UNVERIFIABLE);
    assert.equal(classifyReport({}), CLASS_UNVERIFIABLE);
    assert.equal(classifyReport({ verification: null }), CLASS_UNVERIFIABLE);
  });

  it("returns unverifiable when status=unverified", () => {
    const report = JSON.parse(fixture("unverified.json"));
    assert.equal(classifyReport(report), CLASS_UNVERIFIABLE);
  });

  it("returns unverifiable when provider is not github-actions", () => {
    const report = {
      verification: {
        status: "verified",
        provider: "gitlab-ci",
        workflow_ref: CANONICAL_WORKFLOW + "@refs/tags/v1.0.0",
      },
    };
    assert.equal(classifyReport(report), CLASS_UNVERIFIABLE);
  });

  it("returns non-canonical when workflow_ref does not match canonical prefix", () => {
    const report = JSON.parse(fixture("non-canonical-workflow.json"));
    assert.equal(classifyReport(report), CLASS_NON_CANONICAL);
  });

  it("returns null (needs network check) for a canonical-looking verified report", () => {
    const report = JSON.parse(fixture("attested.json"));
    assert.equal(classifyReport(report), null);
  });
});

// ---------------------------------------------------------------------------
// extractSignerWorkflow
// ---------------------------------------------------------------------------

describe("extractSignerWorkflow", () => {
  it("returns null for empty/null input", () => {
    assert.equal(extractSignerWorkflow(null), null);
    assert.equal(extractSignerWorkflow({}), null);
  });

  it("extracts from DSSE envelope payload", () => {
    const att = canonicalAttestation(CANONICAL_WORKFLOW + "@refs/tags/v2.0.0");
    assert.equal(
      extractSignerWorkflow(att),
      CANONICAL_WORKFLOW + "@refs/tags/v2.0.0"
    );
  });

  it("extracts from verificationResult.statement.predicate path", () => {
    const att = {
      bundle: {
        verificationResult: {
          statement: {
            predicate: {
              buildDefinition: {
                externalParameters: {
                  workflow: { ref: CANONICAL_WORKFLOW + "@refs/tags/v3.0.0" },
                },
              },
            },
          },
        },
      },
    };
    assert.equal(
      extractSignerWorkflow(att),
      CANONICAL_WORKFLOW + "@refs/tags/v3.0.0"
    );
  });

  it("extracts from extensions.jobWorkflowRef fallback", () => {
    const att = { extensions: { jobWorkflowRef: "someorg/other/.github/workflows/foo.yml@refs/heads/main" } };
    assert.equal(extractSignerWorkflow(att), "someorg/other/.github/workflows/foo.yml@refs/heads/main");
  });
});

// ---------------------------------------------------------------------------
// verifyReport — unverifiable path
// ---------------------------------------------------------------------------

describe("verifyReport — unverifiable reports", () => {
  it("returns unverifiable without calling fetch for an unverified report", async () => {
    const raw = fixture("unverified.json");
    const report = JSON.parse(raw);
    let fetchCalled = false;
    const fakeFetch = async () => { fetchCalled = true; return { ok: true, status: 200, json: async () => ({ attestations: [] }) }; };

    const result = await verifyReport(report, raw, fakeFetch);
    assert.equal(result.status, CLASS_UNVERIFIABLE);
    assert.equal(fetchCalled, false, "fetch should not be called for unverified reports");
    assert.equal(result.subject, "localuser");
    assert.ok(result.digest);
  });

  it("includes the digest in the result", async () => {
    const raw = fixture("unverified.json");
    const report = JSON.parse(raw);
    const result = await verifyReport(report, raw, noAttestationFetch());
    assert.equal(result.status, CLASS_UNVERIFIABLE);
    assert.ok(result.digest, "digest should be present");
    assert.match(result.digest, /^[0-9a-f]{64}$/);
  });
});

// ---------------------------------------------------------------------------
// verifyReport — non-canonical workflow
// ---------------------------------------------------------------------------

describe("verifyReport — non-canonical workflow (pre-network fast fail)", () => {
  it("returns non-canonical without calling fetch", async () => {
    const raw = fixture("non-canonical-workflow.json");
    const report = JSON.parse(raw);
    let fetchCalled = false;
    const fakeFetch = async () => { fetchCalled = true; return { ok: false, status: 404, json: async () => ({}) }; };

    const result = await verifyReport(report, raw, fakeFetch);
    assert.equal(result.status, CLASS_NON_CANONICAL);
    assert.equal(fetchCalled, false);
    assert.equal(result.subject, "someuser");
    assert.ok(result.workflowRef.includes("ForkRepute"));
  });
});

// ---------------------------------------------------------------------------
// verifyReport — tampered (no attestation found)
// ---------------------------------------------------------------------------

describe("verifyReport — tampered file", () => {
  it("returns tampered when the digest has no attestation on GitHub", async () => {
    const raw = fixture("attested.json");
    const report = JSON.parse(raw);

    const result = await verifyReport(report, raw, noAttestationFetch());
    assert.equal(result.status, CLASS_TAMPERED);
    assert.equal(result.subject, "someuser");
    assert.equal(result.org, "example-org");
    assert.ok(result.digest);
  });

  it("returns tampered even if report claims verified", async () => {
    const raw = fixture("attested.json");
    const report = JSON.parse(raw);
    // Simulate a one-byte change: change the username in memory but keep raw bytes as original.
    // The point: report claims verified but fetch returns no attestation.
    const result = await verifyReport(report, raw, noAttestationFetch());
    assert.equal(result.status, CLASS_TAMPERED);
  });

  it("returns tampered when GitHub 404s and Rekor also finds nothing", async () => {
    const raw = fixture("attested.json");
    const report = JSON.parse(raw);
    const result = await verifyReport(report, raw, noAttestationFetch(emptyRekorFetch()));
    assert.equal(result.status, CLASS_TAMPERED);
    assert.equal(result.org, "example-org");
    assert.equal(result.subject, "someuser");
  });
});

// ---------------------------------------------------------------------------
// verifyReport — Rekor fallback
// ---------------------------------------------------------------------------

describe("verifyReport — Rekor fallback when GitHub can't resolve the attestation", () => {
  it("returns verified-via-rekor when Rekor finds a canonical-workflow entry", async () => {
    const raw = fixture("attested.json");
    const report = JSON.parse(raw);
    const fetchFn = noAttestationFetch(singleEntryRekorFetch("rekor-cert-canonical.b64.txt"));

    const result = await verifyReport(report, raw, fetchFn);
    assert.equal(result.status, CLASS_VERIFIED_VIA_REKOR);
    assert.equal(result.source, "rekor");
    assert.equal(result.subject, "someuser");
    assert.equal(result.org, "example-org");
    assert.ok(result.workflowRef.startsWith(CANONICAL_WORKFLOW));
    assert.equal(result.identityConfirmed, true);
    assert.equal(result.rekorLogIndex, 555);
    assert.ok(result.rekorLoggedAt);
    assert.ok(result.digest);
  });

  it("returns non-canonical when Rekor's entry resolves to a fork workflow", async () => {
    const raw = fixture("attested.json");
    const report = JSON.parse(raw);
    const fetchFn = noAttestationFetch(singleEntryRekorFetch("rekor-cert-noncanonical.b64.txt"));

    const result = await verifyReport(report, raw, fetchFn);
    assert.equal(result.status, CLASS_NON_CANONICAL);
    assert.ok(result.workflowRef.includes("ForkRepute"));
  });

  it("returns verified-via-rekor with identityConfirmed:false when the cert can't be parsed for identity", async () => {
    const raw = fixture("attested.json");
    const report = JSON.parse(raw);
    const fetchFn = noAttestationFetch(singleEntryRekorFetch("rekor-cert-no-extension.b64.txt"));

    const result = await verifyReport(report, raw, fetchFn);
    assert.equal(result.status, CLASS_VERIFIED_VIA_REKOR);
    assert.equal(result.source, "rekor");
    assert.equal(result.identityConfirmed, false);
    assert.equal(result.rekorLogIndex, 555);
    assert.ok(result.rekorLoggedAt);
  });

  it("returns verify-unavailable when Rekor itself errors", async () => {
    const raw = fixture("attested.json");
    const report = JSON.parse(raw);
    const fetchFn = noAttestationFetch(erroringRekorFetch());

    const result = await verifyReport(report, raw, fetchFn);
    assert.equal(result.status, CLASS_VERIFY_UNAVAILABLE);
    assert.notEqual(result.status, CLASS_TAMPERED);
    assert.ok(result.error);
  });
});

// ---------------------------------------------------------------------------
// verifyReport — verified (happy path)
// ---------------------------------------------------------------------------

describe("verifyReport — verified happy path", () => {
  it("returns verified when attestation exists and workflow matches canonical", async () => {
    const raw = fixture("attested.json");
    const report = JSON.parse(raw);
    const att = canonicalAttestation(CANONICAL_WORKFLOW + "@refs/tags/v1.0.0");
    const fakeFetch = fakeAttestationFetch(att);

    const result = await verifyReport(report, raw, fakeFetch);
    assert.equal(result.status, CLASS_VERIFIED);
    assert.equal(result.subject, "someuser");
    assert.equal(result.org, "example-org");
    assert.ok(result.workflowRef.startsWith(CANONICAL_WORKFLOW));
    assert.ok(result.digest);
  });

  it("result contains a valid digest", async () => {
    const raw = fixture("attested.json");
    const report = JSON.parse(raw);
    const att = canonicalAttestation();
    const fakeFetch = fakeAttestationFetch(att);

    const result = await verifyReport(report, raw, fakeFetch);
    assert.equal(result.status, CLASS_VERIFIED);
    assert.match(result.digest, /^[0-9a-f]{64}$/);
  });
});

// ---------------------------------------------------------------------------
// verifyReport — attestation exists but non-canonical signer
// ---------------------------------------------------------------------------

describe("verifyReport — attestation from non-canonical signer", () => {
  it("returns non-canonical when the attested workflow is a fork", async () => {
    const raw = fixture("attested.json");
    const report = JSON.parse(raw);
    const forkWorkflow = "someorg/ForkRepute/.github/workflows/coderepute-report.yml@refs/tags/v1.0.0";
    const att = canonicalAttestation(forkWorkflow);
    const fakeFetch = fakeAttestationFetch(att);

    const result = await verifyReport(report, raw, fakeFetch);
    assert.equal(result.status, CLASS_NON_CANONICAL);
    assert.ok(result.workflowRef.includes("ForkRepute"));
  });
});

// ---------------------------------------------------------------------------
// verifyReport — network / API errors
// ---------------------------------------------------------------------------

describe("verifyReport — network errors", () => {
  it("returns error when fetch throws", async () => {
    const raw = fixture("attested.json");
    const report = JSON.parse(raw);
    const errorFetch = async () => { throw new Error("network down"); };

    const result = await verifyReport(report, raw, errorFetch);
    assert.equal(result.status, CLASS_ERROR);
    assert.ok(result.error.includes("network down"));
  });

  it("returns error when API returns unexpected HTTP status", async () => {
    const raw = fixture("attested.json");
    const report = JSON.parse(raw);
    const badFetch = async () => ({ ok: false, status: 500, json: async () => ({}) });

    const result = await verifyReport(report, raw, badFetch);
    assert.equal(result.status, CLASS_ERROR);
    assert.ok(result.error.includes("500"));
  });
});

// ---------------------------------------------------------------------------
// fetchRekorEntries
// ---------------------------------------------------------------------------

describe("fetchRekorEntries", () => {
  it("returns an empty array when the index has no matching UUIDs", async () => {
    const fakeFetch = async (url, opts) => {
      assert.ok(url.includes("/api/v1/index/retrieve"));
      assert.equal(opts.method, "POST");
      const body = JSON.parse(opts.body);
      assert.equal(body.hash, "sha256:" + "a".repeat(64));
      return { ok: true, status: 200, json: async () => [] };
    };

    const entries = await fetchRekorEntries("a".repeat(64), fakeFetch);
    assert.deepEqual(entries, []);
  });

  it("resolves a single matching UUID to its decoded entry", async () => {
    const uuid = "f".repeat(64);
    const certB64 = fixtureText("rekor-cert-canonical.b64.txt");
    const entryBody = {
      apiVersion: "0.0.1",
      kind: "hashedrekord",
      spec: {
        signature: { publicKey: { content: certB64 } },
        data: { hash: { algorithm: "sha256", value: "a".repeat(64) } },
      },
    };
    const encodedBody = Buffer.from(JSON.stringify(entryBody)).toString("base64");

    const fakeFetch = async (url) => {
      if (url.includes("/api/v1/index/retrieve")) {
        return { ok: true, status: 200, json: async () => [uuid] };
      }
      if (url.includes("/api/v1/log/entries/")) {
        assert.ok(url.endsWith(uuid));
        return {
          ok: true,
          status: 200,
          json: async () => ({
            [uuid]: {
              logIndex: 12345,
              logID: "b".repeat(64),
              integratedTime: 1750000000,
              body: encodedBody,
            },
          }),
        };
      }
      throw new Error("unexpected URL " + url);
    };

    const entries = await fetchRekorEntries("a".repeat(64), fakeFetch);
    assert.equal(entries.length, 1);
    assert.equal(entries[0].uuid, uuid);
    assert.equal(entries[0].logIndex, 12345);
    assert.equal(entries[0].integratedTime, 1750000000);
    assert.equal(entries[0].kind, "hashedrekord");
    assert.equal(entries[0].body.spec.signature.publicKey.content, certB64);
  });

  it("resolves multiple UUIDs to multiple entries", async () => {
    const uuidA = "1".repeat(64);
    const uuidB = "2".repeat(64);
    const makeBody = (kind) =>
      Buffer.from(JSON.stringify({ kind, spec: {} })).toString("base64");

    const fakeFetch = async (url) => {
      if (url.includes("/api/v1/index/retrieve")) {
        return { ok: true, status: 200, json: async () => [uuidA, uuidB] };
      }
      if (url.endsWith(uuidA)) {
        return {
          ok: true,
          status: 200,
          json: async () => ({ [uuidA]: { logIndex: 1, integratedTime: 100, body: makeBody("hashedrekord") } }),
        };
      }
      if (url.endsWith(uuidB)) {
        return {
          ok: true,
          status: 200,
          json: async () => ({ [uuidB]: { logIndex: 2, integratedTime: 200, body: makeBody("dsse") } }),
        };
      }
      throw new Error("unexpected URL " + url);
    };

    const entries = await fetchRekorEntries("a".repeat(64), fakeFetch);
    assert.equal(entries.length, 2);
    assert.deepEqual(entries.map((e) => e.uuid), [uuidA, uuidB]);
    assert.deepEqual(entries.map((e) => e.kind), ["hashedrekord", "dsse"]);
  });

  it("throws when the index API request fails", async () => {
    const fakeFetch = async (url) => {
      if (url.includes("/api/v1/index/retrieve")) {
        throw new Error("connection refused");
      }
      throw new Error("unexpected URL " + url);
    };

    await assert.rejects(
      () => fetchRekorEntries("a".repeat(64), fakeFetch),
      /connection refused/
    );
  });

  it("throws when the index API returns a non-OK HTTP status", async () => {
    const fakeFetch = async () => ({ ok: false, status: 503, json: async () => ({}) });

    await assert.rejects(
      () => fetchRekorEntries("a".repeat(64), fakeFetch),
      /503/
    );
  });

  it("throws when the entry lookup returns malformed JSON for a UUID", async () => {
    const uuid = "3".repeat(64);
    const fakeFetch = async (url) => {
      if (url.includes("/api/v1/index/retrieve")) {
        return { ok: true, status: 200, json: async () => [uuid] };
      }
      // Entry response missing the body entirely under the UUID key.
      return { ok: true, status: 200, json: async () => ({ wrongKey: {} }) };
    };

    await assert.rejects(() => fetchRekorEntries("a".repeat(64), fakeFetch));
  });

  it("throws when an individual entry GET fails", async () => {
    const uuid = "4".repeat(64);
    const fakeFetch = async (url) => {
      if (url.includes("/api/v1/index/retrieve")) {
        return { ok: true, status: 200, json: async () => [uuid] };
      }
      return { ok: false, status: 500, json: async () => ({}) };
    };

    await assert.rejects(() => fetchRekorEntries("a".repeat(64), fakeFetch), /500/);
  });
});

// ---------------------------------------------------------------------------
// extractWorkflowRefFromCert
// ---------------------------------------------------------------------------

describe("extractWorkflowRefFromCert", () => {
  it("extracts the Fulcio Build Signer URI from a real DER cert", () => {
    const certB64 = fixtureText("rekor-cert-canonical.b64.txt");
    const certBytes = Buffer.from(certB64, "base64");
    const ref = extractWorkflowRefFromCert(certBytes);
    assert.equal(
      ref,
      "https://github.com/grkanitz/CodeRepute/.github/workflows/coderepute-report.yml@refs/tags/v1.0.0"
    );
  });

  it("extracts a non-canonical (fork) workflow ref from a real DER cert", () => {
    const certB64 = fixtureText("rekor-cert-noncanonical.b64.txt");
    const certBytes = Buffer.from(certB64, "base64");
    const ref = extractWorkflowRefFromCert(certBytes);
    assert.equal(
      ref,
      "https://github.com/someorg/ForkRepute/.github/workflows/coderepute-report.yml@refs/tags/v1.0.0"
    );
  });

  it("returns null (no throw) when the cert has no Fulcio extension", () => {
    const certB64 = fixtureText("rekor-cert-no-extension.b64.txt");
    const certBytes = Buffer.from(certB64, "base64");
    assert.equal(extractWorkflowRefFromCert(certBytes), null);
  });

  it("returns null (no throw) for garbage bytes", () => {
    const garbage = new Uint8Array([1, 2, 3, 4, 5]);
    assert.equal(extractWorkflowRefFromCert(garbage), null);
  });

  it("returns null (no throw) for empty bytes", () => {
    assert.equal(extractWorkflowRefFromCert(new Uint8Array()), null);
  });
});

// ---------------------------------------------------------------------------
// explainResult
// ---------------------------------------------------------------------------

describe("explainResult", () => {
  it("verified result has proves items", () => {
    const { proves, doesNotProve } = explainResult(CLASS_VERIFIED);
    assert.ok(proves.length > 0);
    assert.ok(doesNotProve.length > 0);
  });

  it("tampered result has no proves items and has doesNotProve", () => {
    const { proves, doesNotProve } = explainResult(CLASS_TAMPERED);
    assert.equal(proves.length, 0);
    assert.ok(doesNotProve.length > 0);
  });

  it("non-canonical result has no proves items", () => {
    const { proves } = explainResult(CLASS_NON_CANONICAL);
    assert.equal(proves.length, 0);
  });

  it("unverifiable result has no proves items", () => {
    const { proves } = explainResult(CLASS_UNVERIFIABLE);
    assert.equal(proves.length, 0);
  });

  it("unknown status returns graceful fallback", () => {
    const { proves, doesNotProve } = explainResult("something-new");
    assert.equal(proves.length, 0);
    assert.ok(doesNotProve.length > 0);
  });

  it("verified-via-rekor has proves items and mentions Rekor, not GitHub's API", () => {
    const { proves, doesNotProve } = explainResult(CLASS_VERIFIED_VIA_REKOR);
    assert.ok(proves.length > 0);
    assert.ok(proves.some((s) => /rekor/i.test(s)));
    assert.ok(doesNotProve.length > 0);
  });

  it("verify-unavailable has no proves items and is explicit it is not a verdict", () => {
    const { proves, doesNotProve } = explainResult(CLASS_VERIFY_UNAVAILABLE);
    assert.equal(proves.length, 0);
    assert.ok(doesNotProve.length > 0);
    assert.ok(doesNotProve.some((s) => /not.*(tampered|fail)|try again/i.test(s)));
  });
});

// ---------------------------------------------------------------------------
// prefillFromURL
// ---------------------------------------------------------------------------

describe("prefillFromURL", () => {
  it("does nothing when document is not available (Node environment)", () => {
    // In Node, `document` is undefined — prefillFromURL should be a no-op.
    assert.doesNotThrow(() => {
      prefillFromURL(new URLSearchParams("repo=owner/repo&subject=alice"));
    });
  });
});

// ---------------------------------------------------------------------------
// extractRepoFromPDFXMP
// ---------------------------------------------------------------------------

describe("extractRepoFromPDFXMP", () => {
  it("returns null for bytes with no XMP packet", () => {
    const bytes = new TextEncoder().encode("some random bytes, no xpacket here");
    assert.equal(extractRepoFromPDFXMP(bytes), null);
  });

  it("extracts coderepute:repo from an XMP packet with double-quoted meta", () => {
    const xmp = `<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?>
<meta name="coderepute:repo" content="xmp-org/xmp-repo">
<?xpacket end="r"?>`;
    const bytes = new TextEncoder().encode(xmp);
    assert.equal(extractRepoFromPDFXMP(bytes), "xmp-org/xmp-repo");
  });

  it("extracts coderepute:repo from a realistic XMP packet embedded in PDF prefix", () => {
    const pdfBytes = fixture("report.pdf");
    const repo = extractRepoFromPDFXMP(pdfBytes);
    assert.equal(repo, "pdf-org/pdf-repo");
  });

  it("returns null when XMP packet has no coderepute:repo tag", () => {
    const xmp = `<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?>
<meta name="other:thing" content="irrelevant">
<?xpacket end="r"?>`;
    const bytes = new TextEncoder().encode(xmp);
    assert.equal(extractRepoFromPDFXMP(bytes), null);
  });
});

// ---------------------------------------------------------------------------
// verifyHTML
// ---------------------------------------------------------------------------

describe("verifyHTML", () => {
  it("extracts repo from embedded JSON and returns verified when attestation found", async () => {
    const rawBytes = fixture("report.html");
    const att = canonicalAttestation(CANONICAL_WORKFLOW + "@refs/tags/v1.0.0");
    const fakeFetch = fakeAttestationFetch(att);

    const result = await verifyHTML(rawBytes, fakeFetch);
    assert.equal(result.status, CLASS_VERIFIED);
    assert.equal(result.subject, "testuser");
    assert.equal(result.org, "example-org");
    assert.ok(result.digest);
  });

  it("returns tampered when no attestation found for the HTML file's digest", async () => {
    const rawBytes = fixture("report.html");
    const result = await verifyHTML(rawBytes, noAttestationFetch());
    assert.equal(result.status, CLASS_TAMPERED);
  });

  it("returns error for HTML without the embedded JSON script tag", async () => {
    const rawBytes = new TextEncoder().encode("<html><body>no script tag</body></html>");
    const result = await verifyHTML(rawBytes);
    assert.equal(result.status, CLASS_ERROR);
    assert.ok(result.error.includes("coderepute-report"));
    assert.ok(result.digest);
  });

  it("returns error for HTML with malformed JSON in the script tag", async () => {
    const html = `<html><head><script type="application/json" id="coderepute-report">{bad json}</script></head></html>`;
    const rawBytes = new TextEncoder().encode(html);
    const result = await verifyHTML(rawBytes);
    assert.equal(result.status, CLASS_ERROR);
    assert.ok(result.digest);
  });
});

// ---------------------------------------------------------------------------
// verifyPDF
// ---------------------------------------------------------------------------

describe("verifyPDF", () => {
  it("uses repoHint to skip XMP extraction and returns verified when attestation found", async () => {
    const rawBytes = fixture("report.pdf");
    const att = canonicalAttestation(CANONICAL_WORKFLOW + "@refs/tags/v1.0.0");
    const fakeFetch = fakeAttestationFetch(att);

    const result = await verifyPDF(rawBytes, "hint-org/hint-repo", fakeFetch);
    assert.equal(result.status, CLASS_VERIFIED);
    assert.equal(result.org, "hint-org");
  });

  it("falls back to XMP when no repoHint is provided", async () => {
    const rawBytes = fixture("report.pdf");
    const att = canonicalAttestation(CANONICAL_WORKFLOW + "@refs/tags/v1.0.0");
    const fakeFetch = fakeAttestationFetch(att);

    const result = await verifyPDF(rawBytes, null, fakeFetch);
    // The fixture has coderepute:repo = "pdf-org/pdf-repo" in its XMP.
    assert.equal(result.status, CLASS_VERIFIED);
    assert.equal(result.org, "pdf-org");
  });

  it("returns needs-repo when no hint and no XMP", async () => {
    const rawBytes = new TextEncoder().encode("%PDF-1.4\nno xmp content here\n%%EOF");
    const result = await verifyPDF(rawBytes, null);
    assert.equal(result.status, "needs-repo");
    assert.ok(result.digest);
  });

  it("returns tampered when attestation not found for PDF digest", async () => {
    const rawBytes = fixture("report.pdf");
    const result = await verifyPDF(rawBytes, "some-org/some-repo", noAttestationFetch());
    assert.equal(result.status, CLASS_TAMPERED);
  });
});

// ---------------------------------------------------------------------------
// verifyFile
// ---------------------------------------------------------------------------

describe("verifyFile", () => {
  /**
   * Wrap a Uint8Array as a File-like object so verifyFile can dispatch on
   * the name/type and read the bytes via arrayBuffer().
   */
  function makeFileLike(name, type, bytes) {
    return {
      name,
      type,
      arrayBuffer: async () => bytes.buffer,
    };
  }

  it("dispatches .html file to verifyHTML and returns verified", async () => {
    const rawBytes = fixture("report.html");
    const att = canonicalAttestation(CANONICAL_WORKFLOW + "@refs/tags/v1.0.0");
    const fakeFetch = fakeAttestationFetch(att);

    const file = makeFileLike("report.html", "text/html", rawBytes);
    const result = await verifyFile(file, null, fakeFetch);
    assert.equal(result.status, CLASS_VERIFIED);
  });

  it("dispatches .pdf file to verifyPDF using repoHint", async () => {
    const rawBytes = fixture("report.pdf");
    const att = canonicalAttestation(CANONICAL_WORKFLOW + "@refs/tags/v1.0.0");
    const fakeFetch = fakeAttestationFetch(att);

    const file = makeFileLike("report.pdf", "application/pdf", rawBytes);
    const result = await verifyFile(file, "hint-org/hint-repo", fakeFetch);
    assert.equal(result.status, CLASS_VERIFIED);
  });

  it("rejects .json files with a clear error message", async () => {
    const file = makeFileLike("report.json", "application/json", new Uint8Array());
    await assert.rejects(
      () => verifyFile(file),
      /report\.html or report\.pdf/
    );
  });

  it("rejects unsupported file types with a clear error message", async () => {
    const file = makeFileLike("report.xml", "application/xml", new Uint8Array());
    await assert.rejects(
      () => verifyFile(file),
      /report\.html or report\.pdf/
    );
  });

  it("dispatches a Uint8Array with .html name", async () => {
    const rawBytes = fixture("report.html");
    // verifyFile accepts Uint8Array directly as a special case.
    const fakeFile = Object.assign(rawBytes, { name: "report.html", type: "text/html" });
    const att = canonicalAttestation(CANONICAL_WORKFLOW + "@refs/tags/v1.0.0");
    const fakeFetch = fakeAttestationFetch(att);
    const result = await verifyFile(fakeFile, null, fakeFetch);
    assert.equal(result.status, CLASS_VERIFIED);
  });
});

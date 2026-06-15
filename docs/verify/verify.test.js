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
  sha256Hex,
  classifyReport,
  verifyReport,
  extractSignerWorkflow,
  explainResult,
} from "./verify.js";

const __dirname = dirname(fileURLToPath(import.meta.url));
const fixture = (name) =>
  readFileSync(join(__dirname, "testdata", name));

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

/** A fake fetch that returns 404 (no attestation). */
function noAttestationFetch() {
  return async (_url, _opts) => ({
    ok: false,
    status: 404,
    json: async () => ({ attestations: [] }),
  });
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
});

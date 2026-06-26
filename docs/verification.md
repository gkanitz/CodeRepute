# Verifying a CodeRepute report

A CodeRepute report is only as trustworthy as the process that produced
it. The CLI computes statistics from GitHub API metadata, so anyone could
edit the JSON afterwards — unless the run happened in CI and left a
cryptographic trail. This document explains that trail: what gets
attested, what a passing verification proves, and what it does not.

## The trust chain

1. A workflow in the subject's organization runs the CodeRepute action
   **pinned to a tagged version** (e.g. `gkanitz/CodeRepute@v0.1.0`).
2. The action builds the CLI from that pinned source, writes `report.html`
   (a self-contained HTML file with inline SVG charts and the full report JSON
   embedded in a `<script type="application/json" id="coderepute-report">` tag)
   and `report.pdf` (a CI-generated PDF produced by headless Chromium from
   `report.html`). Both files are attested independently with
   [`actions/attest-build-provenance`](https://github.com/actions/attest-build-provenance)
   — a Sigstore signature over each file's SHA-256 digest, bound to the
   workflow's OIDC identity and stored on the producing repository. The verify
   page extracts the embedded JSON from the HTML automatically; there is no
   separate `report.json` artifact.
3. The report's own `verification` block (embedded in the HTML) records the
   producing identity (`provider`, `repository`, `workflow_ref`, `run_id`,
   `run_url`) and a pointer to the attestation, including the exact verify
   command.

Reports produced outside CI carry an explicit
`"verification": {"status": "unverified"}` block. The CLI never claims
more than its environment proves.

The `verification` block itself is part of the report — untrusted input
until checked. It tells a verifier *where to look* (repository, workflow
ref, the verify command); only running `gh attestation verify` proves
anything. A report whose block says `verified` but whose attestation
does not exist or does not match fails verification.

## Verifying

Verification is **two checks**. Both must pass.

### 1. Verify the attestation

```sh
gh attestation verify report.html --repo <org/repo>
gh attestation verify report.pdf --repo <org/repo>
```

where `<org/repo>` is the repository the report claims in
`verification.repository`. This proves:

- **Integrity** — `report.html` (and `report.pdf`) are bit-for-bit the files
  that were attested; any post-run edit fails verification.
- **Origin** — the attestation was created by a GitHub Actions workflow
  running in that repository (and therefore that org), signed via
  GitHub's OIDC issuer. Nobody outside that repo's CI can mint it.
- **Run identity** — the verified provenance names the exact workflow
  ref, commit SHA, and run that produced the file. Inspect it with
  `--format json`.

### 2. Check the producing workflow identity against the canonical action

Step 1 proves *which workflow* produced the report — not that the
workflow ran *unmodified CodeRepute*. A fork of this repository with
doctored metrics could attest its own output and pass step 1 in its own
org. The workflow identity must therefore be matched against the
canonical action. Two ways, strongest first:

**a. Canonical reusable workflow (machine-checkable).** Consumers who run
reports via the reusable workflow

```yaml
jobs:
  report:
    permissions:
      contents: read
      pull-requests: read
      id-token: write
      attestations: write
    uses: gkanitz/CodeRepute/.github/workflows/coderepute-report.yml@v0.1.0
    with:
      repos: your-org/your-repo
      subject: some-username
```

get a Sigstore certificate whose `job_workflow_ref` names
`gkanitz/CodeRepute/.github/workflows/coderepute-report.yml` at the
pinned tag. Verify with:

```sh
gh attestation verify report.html --repo <org/repo> \
  --signer-workflow gkanitz/CodeRepute/.github/workflows/coderepute-report.yml
```

A modified fork (`someorg/CodeRepute`) produces a different
`job_workflow_ref` and **fails this command**. The reusable workflow
checks out the action source at `github.job_workflow_sha` — exactly the
commit of the pinned workflow file — so the binary cannot diverge from
the tag being verified.

**b. Direct composite action (manual identity check).** When a consumer
workflow uses the action directly (`uses: gkanitz/CodeRepute@v0.1.0`),
the attested identity is the *consumer's* workflow. Then:

```sh
gh attestation verify report.html --repo <org/repo> --format json \
  --jq '.[].verificationResult.statement.predicate.buildDefinition.externalParameters.workflow'
```

returns the producing workflow's repository, ref, and commit. Inspect
that workflow file **at that commit** and confirm it references
`gkanitz/CodeRepute` at a published tag — not a fork, not a mutable
branch. A workflow that used a fork or a modified copy fails this
inspection.

## Durability fallback: verifying when GitHub can't resolve the attestation

GitHub's attestation API answers by `owner/repo`. If the producing
repository or its owning org is later deleted, renamed, or made private,
`gh attestation verify` (and the [browser verify page](verify/index.html))
can no longer resolve the attestation through GitHub at all — even though
the report itself was never tampered with.

`actions/attest-build-provenance` doesn't just register the attestation
with GitHub: as a side effect, it also writes an entry to
[Sigstore's Rekor transparency log](https://docs.sigstore.dev/logging/overview/),
a public, append-only log of signing events keyed by artifact digest.
That entry is independent of GitHub's repository-scoped API and keyed
only by the artifact's SHA-256 digest — it survives the producing
repository disappearing.

When GitHub's API can't resolve a report's attestation, the verify page
automatically falls back to querying Rekor's public REST API directly by
digest (`POST /api/v1/index/retrieve`, then `GET /api/v1/log/entries/{uuid}`
against `rekor.sigstore.dev`) before concluding the report is tampered:

- **Rekor finds a matching entry, canonical workflow confirmed** — the
  page reports **verified via Rekor**. This proves the same thing as a
  GitHub-sourced verification (the file is unchanged since it was signed
  by the canonical CodeRepute workflow, at a specific time), but the
  original GitHub Actions run can no longer be inspected directly since
  GitHub's side of the trail is gone. The page makes this distinction
  explicit in its copy and detail rows (Rekor log index, logged-at
  timestamp).
- **Rekor finds a matching entry, but the embedded signing certificate's
  workflow identity could not be independently re-derived** — the page
  still reports verified via Rekor (the digest match is the load-bearing
  integrity proof), but with an honest caveat that the producing workflow
  identity could not be confirmed from the raw log entry. Identity
  extraction parses the Fulcio-issued signing certificate's custom X.509
  extension (OID `1.3.6.1.4.1.57264.1.9`, "Build Signer URI") with a
  small, scoped, dependency-free DER scanner — not a general ASN.1
  parser — so this degraded case is expected to be rare but possible.
- **Rekor finds nothing either** — the report fails verification exactly
  as it would have without the fallback (tampered, or never attested),
  now corroborated by two independent sources instead of one.
- **Rekor itself can't be reached or errors** — the page shows a distinct
  "could not verify right now" state. This is deliberately never
  conflated with "tampered" or rendered as a pass: an availability
  problem in a third-party service is not evidence about the report one
  way or the other.

**Honest caveat about Rekor's availability.** Rekor is public-good
infrastructure governed by the Linux Foundation / OpenSSF and operated
multi-vendor, not a CodeRepute-operated service — no CodeRepute
infrastructure or stored data is introduced by this fallback. It has no
contractual uptime guarantee. In practice it is relied upon heavily:
GitHub's own artifact attestation feature and npm's provenance feature
both depend on the same public Rekor instance. Treat it as best-effort,
widely-relied-upon infrastructure, not a guaranteed-available service.

## What passing proves — and what it does not

Passing both checks (via GitHub, or via the Rekor fallback above) proves:

- the bytes of `report.html` (and `report.pdf`) are unchanged since the
  attested run;
- the report was produced inside CI of the named org/repo (or, on the
  Rekor fallback path, that a canonical-workflow signature exists for
  these exact bytes even if the producing org/repo can no longer be
  queried directly);
- the producing workflow is (a) the canonical CodeRepute reusable
  workflow at a pinned version, or (b) a consumer workflow you have
  inspected and found to pin the canonical action.

It does **not** prove:

- that the coverage is complete — read the report's `coverage` block for
  the repos, window, and token scope the run could see;
- that the underlying GitHub data is honest (e.g. activity manufactured
  before the run);
- anything about reports whose `verification.status` is `unverified` —
  those are honest local runs with no chain at all.

## Pinned-version convention

- Consumers reference the action or reusable workflow at a **tagged
  release** (`@v0.1.0`), never `@main`.
- Tags are immutable once published; a new behavior means a new tag.
- Verifiers match the workflow identity against the canonical repository
  `gkanitz/CodeRepute` and a tagged ref, per the checks above.

## Platform requirements

- Sigstore artifact attestations via the public Sigstore instance
  require a **public** repository (or GitHub Enterprise Cloud for
  private repositories). On a private repo without Enterprise, the
  attest step fails and no attestation is produced.
- The calling workflow must grant `id-token: write` and
  `attestations: write` (plus `contents: read`, and
  `pull-requests: read` when the report runs with the default
  `GITHUB_TOKEN`).
- Verification needs the GitHub CLI ≥ 2.49 (`gh attestation`).

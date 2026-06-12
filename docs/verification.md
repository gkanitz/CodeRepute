# Verifying a CodeRepute report

A CodeRepute report is only as trustworthy as the process that produced
it. The CLI computes statistics from GitHub API metadata, so anyone could
edit the JSON afterwards — unless the run happened in CI and left a
cryptographic trail. This document explains that trail: what gets
attested, what a passing verification proves, and what it does not.

## The trust chain

1. A workflow in the subject's organization runs the CodeRepute action
   **pinned to a tagged version** (e.g. `grkanitz/CodeRepute@v0.1.0`).
2. The action builds the CLI from that pinned source, writes
   `report.json` and `report.html`, and attests `report.json` with
   [`actions/attest-build-provenance`](https://github.com/actions/attest-build-provenance)
   — a Sigstore signature over the file's SHA-256 digest, bound to the
   workflow's OIDC identity and stored on the producing repository.
3. The report's own `verification` block records the producing identity
   (`provider`, `repository`, `workflow_ref`, `run_id`, `run_url`) and a
   pointer to the attestation, including the exact verify command.

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
gh attestation verify report.json --repo <org/repo>
```

where `<org/repo>` is the repository the report claims in
`verification.repository`. This proves:

- **Integrity** — `report.json` is bit-for-bit the file that was attested;
  any post-run edit fails verification.
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
    uses: grkanitz/CodeRepute/.github/workflows/coderepute-report.yml@v0.1.0
    with:
      repos: your-org/your-repo
      subject: some-username
```

get a Sigstore certificate whose `job_workflow_ref` names
`grkanitz/CodeRepute/.github/workflows/coderepute-report.yml` at the
pinned tag. Verify with:

```sh
gh attestation verify report.json --repo <org/repo> \
  --signer-workflow grkanitz/CodeRepute/.github/workflows/coderepute-report.yml
```

A modified fork (`someorg/CodeRepute`) produces a different
`job_workflow_ref` and **fails this command**. The reusable workflow
checks out the action source at `github.job_workflow_sha` — exactly the
commit of the pinned workflow file — so the binary cannot diverge from
the tag being verified.

**b. Direct composite action (manual identity check).** When a consumer
workflow uses the action directly (`uses: grkanitz/CodeRepute@v0.1.0`),
the attested identity is the *consumer's* workflow. Then:

```sh
gh attestation verify report.json --repo <org/repo> --format json \
  --jq '.[].verificationResult.statement.predicate.buildDefinition.externalParameters.workflow'
```

returns the producing workflow's repository, ref, and commit. Inspect
that workflow file **at that commit** and confirm it references
`grkanitz/CodeRepute` at a published tag — not a fork, not a mutable
branch. A workflow that used a fork or a modified copy fails this
inspection.

## What passing proves — and what it does not

Passing both checks proves:

- the bytes of `report.json` are unchanged since the attested run;
- the report was produced inside CI of the named org/repo;
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
  `grkanitz/CodeRepute` and a tagged ref, per the checks above.

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

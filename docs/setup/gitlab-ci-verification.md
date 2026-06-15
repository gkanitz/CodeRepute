# GitLab CI verification checklist

This document describes how to manually verify a CodeRepute report produced by
the GitLab CI/CD component. Read `docs/verification.md` first for the full
trust model; this page covers only what is different (and what is absent) on
GitLab.

## What the component does

1. Downloads the CodeRepute CLI from GitHub Releases at a pinned version tag.
2. Runs the CLI against your GitLab group with a group access token.
3. Writes `report/report.json` and `report/report.html`.
4. Patches the `verification` block with GitLab CI job identity using `jq`.
5. Uploads both files as job artifacts.

## What the verification block proves on GitLab

| Field          | Source (GitLab predefined variable) | What it tells you |
|----------------|-------------------------------------|-------------------|
| `provider`     | hardcoded `"gitlab-ci"`             | The report was produced in a GitLab CI job |
| `repository`   | `CI_PROJECT_PATH`                   | Which GitLab project ran the job |
| `workflow_ref` | `CI_PROJECT_PATH` + `CI_COMMIT_REF_NAME` | Which branch/tag the pipeline ran on |
| `run_url`      | `CI_JOB_URL`                        | Direct link to the specific job log |
| `note`         | hardcoded explanation               | Summary of what cannot be verified |

## What GitLab CI cannot prove

GitLab CI does **not** provide Sigstore OIDC attestation:

- There is **no cryptographic signature** over `report.json`'s content.
- There is **no machine-verifiable proof** that the file was unmodified
  after the job wrote it.
- There is **no way to verify** (without GitLab API access) that the
  `run_url` corresponds to the actual report content.

This is the key difference from the GitHub Actions component, which calls
`actions/attest-build-provenance` and produces a Sigstore attestation
verifiable with `gh attestation verify`.

## Manual verification checklist

Use this checklist when a consumer hands you a `report.json` produced by
the GitLab CI component and claims it is trustworthy.

### 1. Confirm the verification block is populated

```sh
jq '.verification' report.json
```

Expected shape:

```json
{
  "status": "verified",
  "provider": "gitlab-ci",
  "repository": "my-org/my-project",
  "workflow_ref": "my-org/my-project/.gitlab-ci.yml@main",
  "run_url": "https://gitlab.com/my-org/my-project/-/jobs/12345",
  "note": "GitLab CI does not provide Sigstore OIDC attestation..."
}
```

A block with `"status": "unverified"` means the report was produced outside
CI or the component was not used.

### 2. Open the job log at `run_url`

Navigate to `verification.run_url` in your browser. Confirm:

- The job belongs to the project named in `verification.repository`.
- The job ran on the branch/tag named in `verification.workflow_ref`.
- The job log shows the CodeRepute CLI running and writing `report.json`.
- The artifact download link produces the same file you are inspecting.

### 3. Check the component version used

In the job log, look for the download URL line:

```
Downloading CodeRepute vX.Y.Z from https://github.com/grkanitz/CodeRepute/releases/download/...
```

Confirm the version is a published release tag at
`https://github.com/grkanitz/CodeRepute/releases`. A `latest` download is
not pinned and cannot be verified after the fact.

### 4. Confirm the artifact SHA matches

Download the artifact from the GitLab job artifacts page. Compute its SHA-256:

```sh
sha256sum report.json
```

Compare against `sha256sum` of the file you are verifying. If they differ,
the file was modified after the job ran.

This is a **manual integrity check** — not a cryptographic attestation. It
requires that the GitLab project's artifacts storage is not compromised and
that the artifact has not been replaced (GitLab does not prevent artifact
replacement by project maintainers).

### 5. Inspect the `.gitlab-ci.yml` at the reported ref

Visit the repository at `verification.repository`, check out the ref from
`verification.workflow_ref`, and read `.gitlab-ci.yml`. Confirm:

- It includes the CodeRepute component at a pinned version tag.
- The `gitlab_token` input maps to a CI/CD variable (not a hardcoded value).
- No post-processing step modifies `report/report.json` after the component
  runs.

## Pinned-version convention

Always pin the component to a tagged version:

```yaml
include:
  - component: gitlab.com/grkanitz/coderepute/coderepute@v0.1.0
    inputs:
      version: v0.1.0   # also pin the CLI download
      subject: some-username
```

Using `@main` or `version: latest` means the component and binary can change
between runs. A verifier cannot confirm what code produced a report from a
mutable ref.

## Comparison with GitHub Actions

| Property | GitHub Actions | GitLab CI |
|----------|---------------|-----------|
| Cryptographic attestation | Yes (Sigstore, `gh attestation verify`) | No |
| Integrity proof | SHA-256 in Sigstore certificate | Manual SHA comparison only |
| Workflow identity proof | Machine-verifiable via `--signer-workflow` | Manual log inspection only |
| Requires internet access to verify | Yes (Sigstore TLog) | No (local comparison) |
| Verification command | `gh attestation verify report.json --repo org/repo` | Manual checklist above |

If your trust model requires cryptographic attestation, use the GitHub Actions
component (`grkanitz/CodeRepute@vX.Y.Z`) and follow `docs/verification.md`.

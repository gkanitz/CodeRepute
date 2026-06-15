# CodeRepute GitLab CI/CD Component

Generates a CodeRepute collaboration report inside a GitLab CI pipeline, populating the `verification` block with GitLab job identity.

## Usage

In your `.gitlab-ci.yml`:

```yaml
include:
  - component: gitlab.com/grkanitz/coderepute/coderepute@v0.1.0
    inputs:
      subject: some-username
      gitlab_token: $MY_GROUP_TOKEN
      group: my-group
```

Pin to a tagged version (`@v0.1.0`). Never use `@main` — the tag is immutable once published.

## Inputs

| Input          | Required | Default         | Description |
|----------------|----------|-----------------|-------------|
| `subject`      | yes      | —               | GitLab username the report is about |
| `gitlab_token` | yes      | `$GITLAB_TOKEN` | Group access token with `read_api` scope |
| `group`        | no       | —               | GitLab group to cover (all visible repos) |
| `since`        | no       | —               | Window start date `YYYY-MM-DD`; omit for 365-day default |
| `until`        | no       | —               | Window end date `YYYY-MM-DD`; omit for today |
| `version`      | no       | `latest`        | CLI version tag to download (e.g. `v0.1.0`) |

## Artifacts

The component uploads two files as job artifacts:

- `report/report.json` — machine-readable report (retained 90 days)
- `report/report.html` — human-readable report (retained 90 days)

## Verification block

The report's `verification` block is populated with GitLab CI job identity:

```json
{
  "verification": {
    "status": "verified",
    "provider": "gitlab-ci",
    "repository": "my-org/my-repo",
    "workflow_ref": "my-org/my-repo/.gitlab-ci.yml@main",
    "run_url": "https://gitlab.com/my-org/my-repo/-/jobs/12345",
    "note": "GitLab CI does not provide Sigstore OIDC attestation..."
  }
}
```

**GitLab limitation vs. GitHub Actions:** GitLab CI does not provide Sigstore OIDC attestation. The `verification` block records job identity (project, ref, job URL) so a reader knows which pipeline produced the report, but there is no cryptographic proof that the file was unmodified after the job ran. See [docs/setup/gitlab-ci-verification.md](../../docs/setup/gitlab-ci-verification.md) for the manual verification checklist.

For cryptographic attestation, use the GitHub Actions component (`grkanitz/CodeRepute@vX.Y.Z`), which produces a Sigstore artifact attestation verifiable with `gh attestation verify`.

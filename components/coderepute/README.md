# coderepute-report

Produce a verifiable CodeRepute collaboration report from GitLab API metadata.

The component downloads the CodeRepute CLI from a GitHub Release, runs it
against the configured group with a group access token, and uploads
`report.json` and `report.html` as CI job artifacts.

## Usage

```yaml
include:
  - component: gitlab.com/grkanitz/coderepute/coderepute-report@v0.1.0
    inputs:
      subject: some-gitlab-username
      group: your-group
```

**Pin to a tagged version** — use `@vX.Y.Z`, never `@main` or `@latest`.
Pinning ensures reproducibility and is required for GitLab CI/CD Catalog
consumers who want to verify the producing workflow identity.

## Inputs

| Input | Required | Default | Description |
|---|---|---|---|
| `subject` | yes | — | GitLab username the report is about |
| `gitlab_token` | no | `$GITLAB_TOKEN` | Group access token with `read_api` scope |
| `group` | no | `""` | GitLab group to cover (all visible projects in the group) |
| `since` | no | `""` | Window start date (YYYY-MM-DD); omit for default 365-day window |
| `until` | no | `""` | Window end date (YYYY-MM-DD); omit for today |
| `version` | no | `latest` | CodeRepute CLI version to download (e.g. `v0.1.0`) |

## Artifacts

| File | Description |
|---|---|
| `report/report.json` | Machine-readable report (schema-versioned JSON) |
| `report/report.html` | Self-contained HTML report for sharing |

Artifacts expire in 90 days.

## Token setup

Create a group access token at:

```
https://gitlab.com/groups/{YOUR_GROUP}/-/settings/access_tokens
```

- **Role**: Reporter
- **Scope**: `read_api` only

Store it as a masked, protected CI/CD variable named `GITLAB_TOKEN` (or pass
it explicitly via the `gitlab_token` input).

See the [full GitLab setup guide](https://github.com/grkanitz/CodeRepute/blob/main/docs/setup/gitlab.md).

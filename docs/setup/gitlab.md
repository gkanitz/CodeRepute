# GitLab Setup Guide

This guide takes a GitLab group admin from zero to a working CodeRepute report
for one of their group's members.

## Prerequisites

- You are a group owner (or have the Maintainer role with token-management
  permissions) for the GitLab group whose projects you want to cover.
- The developer whose report you want to generate has a GitLab account that
  is a member of that group.
- GitLab 16.0 or later (CI components are a 16.0+ feature).

---

## Step 1 — Create a group access token

A group access token is the narrowest credential that covers multiple projects
in a group. It is org-issued (not tied to a personal account), and you can
revoke it without affecting any user.

1. Go to:
   ```
   https://gitlab.com/groups/{YOUR_GROUP}/-/settings/access_tokens
   ```
   replacing `{YOUR_GROUP}` with your group's GitLab path (e.g. `acme-corp`).

2. Fill in the token details:
   - **Token name** — something like `coderepute`.
   - **Expiration date** — set an appropriate rotation schedule (GitLab
     enforces a maximum of 365 days for group access tokens).
   - **Role** — select **Reporter**. This is the minimum role needed to list
     merge requests and notes (GitLab's equivalent of pull requests and review
     comments).
   - **Scopes** — check **`read_api`** only. This grants read-only access to
     the group's projects via the REST API. Do not check `api`, `write_*`, or
     any repository-content scope.

3. Click **Create group access token** and copy the token value. Store it in
   a secrets manager immediately — GitLab shows it only once.

---

## Step 2 — Add the token as a CI/CD variable

In the project where you will run the report pipeline:

1. Go to **Settings → CI/CD → Variables → Add variable**.
2. Set:
   - **Key** — `GITLAB_TOKEN`
   - **Value** — the token from Step 1
   - **Type** — Variable
   - **Protect variable** — enabled (so it is only available on protected
     branches and tags)
   - **Mask variable** — enabled (so it never appears in job logs)
3. Click **Add variable**.

---

## Step 3 — Include the GitLab CI component

Add the following to your `.gitlab-ci.yml`:

```yaml
include:
  - component: gitlab.com/gkanitz/coderepute/coderepute-report@v0.1.0
    inputs:
      subject: some-gitlab-username
      group: your-group
```

**Pin to a tagged version** — always use `@vX.Y.Z`, never `@main` or
`@latest`. GitLab CI components use catalog versioning; a pinned version
ensures reproducibility.

The component reads `$GITLAB_TOKEN` from CI/CD variables automatically. Pass
`gitlab_token: $MY_CUSTOM_VAR` to use a different variable name.

---

## Step 4 — What the component does

Each pipeline run:

1. Downloads the CodeRepute CLI binary from the pinned GitHub Release.
2. Calls the CodeRepute CLI with the GitLab REST API backend, using
   `read_api`-scoped metadata endpoints only (merge requests, notes,
   member lookups). No repository contents are fetched.
2. Write `report.html` (a self-contained HTML file with embedded report JSON)
   and `report.pdf` (a CI-generated PDF produced by headless Chromium) to a
   configurable output path (default: `coderepute-report/`).
3. Attach both files as a GitLab job artifact, available for download from
   the pipeline UI (expires in 90 days).

### GitLab CI attestation limitations

The report's `verification` block is populated with GitLab CI job identity
(project path, ref, job URL). This records **where and when** the report was
produced so a reader can trace it back to the pipeline run.

What this cannot provide: GitLab CI does not issue Sigstore OIDC tokens for
artifact attestation. There is no cryptographic proof that the `report.json`
file was unmodified after the job ran, and there is no machine-verifiable
workflow identity without GitLab API access to the specific pipeline run.

For cryptographic attestation (Sigstore, verifiable with `gh attestation
verify`), use the GitHub Actions component (`gkanitz/CodeRepute@vX.Y.Z`)
instead.

See [docs/setup/gitlab-ci-verification.md](gitlab-ci-verification.md) for the
manual verification checklist applicable to GitLab CI reports.

---

## API endpoints used (GitLab)

For reference, the GitLab provider uses these REST API endpoints
(all read-only, all gated on `read_api`):

| Endpoint | Purpose |
|---|---|
| `GET /users?username={username}` | Resolve subject to immutable numeric user ID |
| `GET /groups/{group}/projects` | List projects in the group |
| `GET /projects/{id}/merge_requests` | List merge requests in the window |
| `GET /projects/{id}/merge_requests/{iid}/approvals` | List approvals |
| `GET /projects/{id}/merge_requests/{iid}/notes` | List review notes/comments |

No `/repository/`, `/repository/files/`, `/repository/archive`, or other
file-content endpoints are called.

---

## Permissions summary

| What | Permission | Why |
|---|---|---|
| List merge requests and notes | `read_api` scope on group access token | Core data source |
| Resolve username to user ID | `read_api` (included) | Identity binding |
| Repository contents | Not requested | Never needed |

If you have questions about the GitLab support, open an issue at
[github.com/gkanitz/CodeRepute](https://github.com/gkanitz/CodeRepute).

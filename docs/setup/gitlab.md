# GitLab Setup Guide

> **Status:** The GitLab CI component (issue #8) is not yet merged. This guide
> describes the planned interface. The CLI already supports GitLab group access
> tokens via the `-token` flag when called directly, but the CI component that
> wraps it — including Sigstore attestation — is coming in a future release.
> Steps 3 and 4 describe the planned interface; they will be updated when the
> component ships.

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
   - **Key** — `CR_GITLAB_TOKEN`
   - **Value** — the token from Step 1
   - **Type** — Variable
   - **Protect variable** — enabled (so it is only available on protected
     branches and tags)
   - **Mask variable** — enabled (so it never appears in job logs)
3. Click **Add variable**.

---

## Step 3 — Include the GitLab CI component (coming soon)

> **Note:** The GitLab CI component is planned for issue #8 and is not yet
> available. The include path below describes the intended interface.

Once the component ships, add the following to your `.gitlab-ci.yml`:

```yaml
include:
  - component: gitlab.com/grkanitz/coderepute/coderepute-report@v0.1.0
    inputs:
      subject: some-gitlab-username
      group: your-group
      window_days: 365
```

**Pin to a tagged version** — always use `@vX.Y.Z`, never `@main` or
`@latest`. GitLab CI components use catalog versioning; a pinned version
ensures reproducibility.

The component will read `$CR_GITLAB_TOKEN` from CI/CD variables automatically.

---

## Step 4 — What the component produces (planned)

When the component ships, each pipeline run will:

1. Call the CodeRepute CLI with the GitLab REST API backend, using
   `read_api`-scoped metadata endpoints only (merge requests, notes,
   member lookups). No repository contents are fetched.
2. Write `report.html` (a self-contained HTML file with embedded report JSON)
   and `report.pdf` (a CI-generated PDF produced by headless Chromium) to a
   configurable output path (default: `coderepute-report/`).
3. Attach both files as a GitLab job artifact, available for download from
   the pipeline UI.

Attestation for GitLab CI reports is under design. GitLab's OIDC integration
and Sigstore tooling differ from GitHub Actions; the exact signing mechanism
will be documented when the component ships.

---

## API endpoints used (GitLab)

For reference, the planned GitLab provider will use these REST API endpoints
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

If you have questions about the GitLab support timeline, open an issue at
[github.com/grkanitz/CodeRepute](https://github.com/grkanitz/CodeRepute).

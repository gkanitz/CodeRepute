# GitHub Setup Guide

This guide takes an org admin from zero to a working, attested CodeRepute
report. Estimated time: 10–15 minutes.

## Prerequisites

- You are an owner of the GitHub org whose repositories you want to cover.
- The developer whose report you want to generate has a GitHub account in
  that org.
- You have the [GitHub CLI](https://cli.github.com/) (`gh`) installed
  (version 2.49 or later) if you plan to verify attestations.

---

## Step 1 — Create a GitHub App in your org

A GitHub App installation token is the narrowest credential CodeRepute
supports: it is org-issued, scoped to specific repositories, and expires
automatically after one hour. The app credentials never leave your org.

1. Go to:
   ```
   https://github.com/organizations/{YOUR_ORG}/settings/apps/new
   ```
   replacing `{YOUR_ORG}` with your organisation's GitHub slug.

2. Fill in the app details:
   - **GitHub App name** — something like `coderepute-{your-org}` (must be
     globally unique on GitHub).
   - **Homepage URL** — your org URL or `https://github.com/grkanitz/CodeRepute`.
   - **Webhook** — uncheck "Active"; CodeRepute does not use webhooks.

3. Under **Repository permissions**, grant:
   - **Metadata** — Read-only (required by GitHub for all apps; gives access
     to basic repo info only).
   - **Pull requests** — Read-only (lists PRs, reviews, and review comments).
   - Leave everything else at "No access".

4. Under **Where can this GitHub App be installed?**, select
   "Only on this account" to restrict the app to your org.

5. Click **Create GitHub App**.

6. On the app's settings page, note the **App ID** (a number like `123456`).

7. Scroll to **Private keys** and click **Generate a private key**. Download
   the `.pem` file and store it in a secure location (e.g. a secrets manager).
   You will add it to GitHub secrets in Step 4.

---

## Step 2 — Install the app on your repositories

1. From the app's settings page, click **Install App** in the left sidebar.
2. Click **Install** next to your org.
3. Select **Only select repositories** and choose the repositories you want
   CodeRepute to be able to read. Click **Install**.
4. After installation, note the **Installation ID** from the URL of the
   resulting page:
   ```
   https://github.com/organizations/{YOUR_ORG}/settings/installations/{INSTALLATION_ID}
   ```

---

## Step 3 — Add the secrets to GitHub

In the repository where you will run the workflow:

1. Go to **Settings → Secrets and variables → Actions → New repository secret**.
2. Add two secrets:
   - `CR_APP_ID` — the numeric App ID from Step 1.
   - `CR_APP_KEY` — the full contents of the `.pem` private key file.

---

## Step 4 — Add the reusable workflow to your CI

In the repository where you want to run the report, create a workflow file
(e.g. `.github/workflows/coderepute.yml`):

```yaml
name: CodeRepute report

on:
  workflow_dispatch:
    inputs:
      subject:
        description: GitHub username to report on
        required: true

jobs:
  report:
    permissions:
      contents: read
      pull-requests: read
      id-token: write
      attestations: write
    uses: grkanitz/CodeRepute/.github/workflows/coderepute-report.yml@v0.1.0
    with:
      repos: your-org/your-repo,your-org/another-repo
      subject: ${{ inputs.subject }}
      window-days: "365"
```

**Pin to a tagged version** — always use `@vX.Y.Z`, never `@main`. Pinning
ensures that the Sigstore certificate's `job_workflow_ref` matches the canonical
workflow at the exact version, which is what `gh attestation verify
--signer-workflow` checks.

### Using the GitHub App instead of the default token

The `coderepute-report.yml` reusable workflow accepts a `token` input. To pass
an app installation token, generate one in a preceding step:

```yaml
jobs:
  mint-token:
    runs-on: ubuntu-latest
    outputs:
      token: ${{ steps.app-token.outputs.token }}
    steps:
      - uses: actions/create-github-app-token@v1
        id: app-token
        with:
          app-id: ${{ secrets.CR_APP_ID }}
          private-key: ${{ secrets.CR_APP_KEY }}

  report:
    needs: mint-token
    permissions:
      id-token: write
      attestations: write
    uses: grkanitz/CodeRepute/.github/workflows/coderepute-report.yml@v0.1.0
    with:
      repos: your-org/your-repo
      subject: some-username
    secrets:
      token: ${{ needs.mint-token.outputs.token }}
```

> The reusable workflow does not currently expose a `token` secret input —
> this pattern requires using the composite action directly
> (`uses: grkanitz/CodeRepute@v0.1.0`). A dedicated `token` secret input
> on the reusable workflow is planned.

---

## Step 5 — Run the workflow and download the report

1. In your repository, go to **Actions → CodeRepute report → Run workflow**.
2. Enter the GitHub username of the developer.
3. After the run completes, download the `coderepute-report` artifact — it
   contains `report.json` and `report.html`.

---

## Step 6 — Verify the report (optional but recommended)

```sh
gh attestation verify report.json --repo your-org/your-repo \
  --signer-workflow grkanitz/CodeRepute/.github/workflows/coderepute-report.yml
```

A passing result confirms:

- `report.json` is unchanged since the attested run.
- The report was produced by the canonical CodeRepute workflow at the pinned
  version, not a fork or a modified copy.

See [docs/verification.md](../verification.md) for the full two-step
verification procedure.

---

## Permissions summary

| What | Permission | Why |
|---|---|---|
| List PRs and reviews | `pull-requests: read` | Core data source |
| Resolve username to account ID | `metadata: read` (implicit) | Identity binding |
| Sigstore OIDC signing | `id-token: write` | Mints the OIDC token for attestation |
| Store attestation | `attestations: write` | Saves the attestation to the repo |
| Check out action source | `contents: read` | Builds the CLI from the pinned tag |

No write permission to pull requests, issues, code, or any other resource is
needed or requested.

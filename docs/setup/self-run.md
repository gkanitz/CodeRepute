# Self-run setup guide

Run CodeRepute on your own repositories without an organisation (org) account or
org admin. Two paths, same codebase:

1. **Local trial** (5 minutes) -- produces an unverified report; no CI needed.
2. **Attested self-run** (15 minutes) -- full Sigstore attestation from your own
   personal GitHub repository.

---

## Prerequisites

- A GitHub account.
- The [GitHub CLI](https://cli.github.com/) (`gh`) installed (version 2.49 or
  later) if you plan to verify attestations.
- The repositories you want to cover are on GitHub (this guide covers only
  GitHub; GitLab follows the same CLI workflow for local trials).

---

## Local trial (5 minutes)

Try CodeRepute right now with your own token. No CI setup, no YAML, no
attestation -- just a report.

### Step 1 -- Create a fine-grained PAT

1. Go to [Fine-grained tokens](https://github.com/settings/tokens?type=beta).
2. Click **Generate new token**.
3. Set a name (e.g. `coderepute-trial`).
4. Under **Repository access**, select **All repositories** (read-only; the
   token cannot write anything).
5. Under **Permissions** -> **Repository permissions**, set:
   - **Pull requests** -> **Read-only**
   - All other permissions stay at **None**
6. **Metadata: Read-only** is auto-granted by GitHub for all fine-grained PATs
   and cannot be removed -- it is the minimum needed to resolve account IDs.
7. Click **Generate token** and copy the value (it starts with `github_pat_`).

That is the only scope CodeRepute needs -- read access to pull requests
(including reviews and review comments) and metadata.

This PAT is used directly by the `coderepute` CLI command (Step 3 below).
Because you pass it as a `-token` argument, the CLI can read any repository the
token has access to -- your own public and private repos, plus public OSS repos
you contribute to. See [Coverage guidance](#coverage-guidance) for the full
table.

### Step 2 -- Download the CLI

```sh
# macOS / Linux (replace OS and ARCH as needed)
curl -fsSL https://github.com/gkanitz/CodeRepute/releases/latest/download/coderepute_linux_amd64.tar.gz \
  | tar -xz -C /usr/local/bin coderepute
```

Or build from source:

```sh
go install github.com/gkanitz/coderepute/cmd/coderepute@latest
```

### Step 3 -- Run the report

```sh
coderepute -repo owner/repo -subject your-username -token github_pat_YOUR_TOKEN -out ./report
```

Replace `owner/repo` with a repository you have access to and `your-username`
with your GitHub username. The output directory `./report` will contain:

| File | Purpose |
|---|---|
| `report.html` | Full interactive report with inline SVG charts |
| `report.json` | Machine-readable report data (embedded in the HTML too) |

**What to expect.** The report is fully populated with your collaboration
metrics -- PRs authored, reviews given, review depth, time to merge, cadence,
and monthly trends. The verification block reads:

```json
"verification": {
  "status": "unverified",
  "reason": "report produced locally; no CI attestation"
}
```

There is also no pdf file -- PDF generation requires headless Chromium, which
`action.yml` runs automatically in CI but the CLI does not install for you.

The coverage stamp records your token's scope class:

```json
"coverage": {
  "token_scope_class": "fine-grained-pat"
}
```

The report is useful immediately for your own review. To make it shareable and
verifiable, move to the attested run.

---

## Attested self-run (15 minutes)

Run CodeRepute in your own personal GitHub repository with Sigstore
attestation. The result is a fully attested, verifiable report -- the same trust
model an org run produces, without any org involvement.

### How it works

You create a repository (public or private), add a workflow file that pins to
the canonical CodeRepute reusable workflow, and trigger the workflow manually.
The Sigstore certificate records the workflow identity as
`gkanitz/CodeRepute/.github/workflows/coderepute-report.yml` at the pinned
version -- the same machine-checkable origin proof that an org run would carry.

**Coverage note for the workflow path.** The reusable workflow uses the ambient
`GITHUB_TOKEN` of the repository it runs in, which is scoped to that
repository. It can read pull request data from that repository and from any
public repository on GitHub. It does **not** have access to other private
repositories you own. For those, use the local trial path above (it produces an
honest `unverified` report rather than an attested one). See the
[Coverage boundary](#coverage-boundary) section after Step 2 for the full
details.

### Step 1 -- Create a personal repository

Create a new repository on GitHub under your personal account. It can be public
or private -- neither affects the attestation (though public is easier for
verifiers to inspect). Name it something like `coderepute-self-run`.

This repository will contain only the workflow file and its secrets -- no source
code. You do **not** need a PAT for this workflow; the ambient
`GITHUB_TOKEN` handles authentication for the repository's own data and
public data.

### Step 2 -- Add the workflow file

In your repository, create `.github/workflows/self-run.yml`:

```yaml
name: CodeRepute self-run

on:
  workflow_dispatch:
    inputs:
      subject:
        description: GitHub username to report on
        required: true
      repos:
        description: Repositories to cover (owner/name, comma-separated)
        required: true

jobs:
  report:
    permissions:
      contents: read
      pull-requests: read
      id-token: write
      attestations: write
    uses: gkanitz/CodeRepute/.github/workflows/coderepute-report.yml@v0.1.0
    with:
      repos: ${{ inputs.repos }}
      subject: ${{ inputs.subject }}
```

**All references must be pinned to a tag** -- `@v0.1.0` in the example above.
Never use `@main`. Pinning ensures the Sigstore certificate's
`job_workflow_ref` matches exactly the version that produced the report, which
is what `gh attestation verify --signer-workflow` checks.

> **Why the reusable workflow and not the composite action directly?**
> Calling the canonical reusable workflow makes the Sigstore certificate record
> the producing workflow identity as
> `gkanitz/CodeRepute/.github/workflows/coderepute-report.yml` at the pinned
> version. This is what `gh attestation verify --signer-workflow` checks -- a
> reader can confirm the report was produced by the unmodified CodeRepute
> pipeline, not a fork or a modified copy. The composite action
> (`uses: gkanitz/CodeRepute@v0.1.0`) produces a valid attestation too, but its
> signer-workflow identity is your own workflow file instead, which makes the
> `--signer-workflow` check fail.

### Coverage boundary

The workflow above uses `gkanitz/CodeRepute/.github/workflows/coderepute-report.yml@v0.1.0`,
which internally calls the composite action (`action.yml`) with the ambient
`GITHUB_TOKEN`. This token is scoped to the repository hosting the workflow and
has no access to other private repositories.

| What you list in `repos:` | Will it be covered? | Why |
|---|---|---|
| The workflow-hosting repo itself (`your-name/coderepute-self-run`) | Yes | `GITHUB_TOKEN` has pull-requests:read on its own repo |
| Any public repo (`your-name/public-project`, `lodash/lodash`) | Yes | GitHub's API returns public data to any authenticated token |
| Another private repo you own (`your-name/private-project`) | **No** | `GITHUB_TOKEN` is scoped to the workflow's own repo only |

If your goal is to cover another private repository, use the
[local trial](#local-trial-5-minutes) path instead. The CLI accepts `-token`
with a PAT that has access to that repo, and although the report carries
`"status": "unverified"` rather than a Sigstore attestation, it is otherwise
identical -- same metrics, same coverage stamp, same HTML report.

> The reusable workflow does not currently accept a custom `token` input. A
> future version may add one. When it does, the attested workflow path will be
> able to cover your other private repos as well.

### Step 3 -- Run the workflow

1. Go to your repository -> **Actions** -> **CodeRepute self-run** ->
   **Run workflow**.
2. Enter the GitHub username and the repositories to cover (e.g.
   `your-name/your-repo,your-name/another-repo`).
3. Keep the coverage boundary above in mind: `your-name/another-repo` will only
   produce data if it is a public repo or the same repo the workflow runs in.
4. Wait for the run to complete (typically 30-60 seconds per repo).
5. Download the `coderepute-report` artifact -- it contains `report.pdf`,
   `report.html`, and the share card files.

### Step 4 -- Verify the report

```sh
gh attestation verify report.pdf --repo your-name/coderepute-self-run
gh attestation verify report.html --repo your-name/coderepute-self-run
gh attestation verify report.pdf --repo your-name/coderepute-self-run \
  --signer-workflow gkanitz/CodeRepute/.github/workflows/coderepute-report.yml
```

A passing result confirms:

- `report.pdf` (and `report.html`) is unchanged since the attested run.
- The report was produced by the canonical CodeRepute action at the pinned
  version, not a fork or a modified copy.

---

## Coverage guidance

What CodeRepute can see depends on which path you use.

### Local trial (CLI with PAT)

When you pass a PAT via the `-token` flag, the report covers every repository
that token can read. A fine-grained PAT with **Pull requests: Read-only** and
**All repositories** access can read:

| What it covers | Example | Visible |
|---|---|---|
| Your own private repos | `your-name/private-project` | Yes |
| Your own public repos | `your-name/open-source-tool` | Yes |
| Public OSS repos you contribute to | `lodash/lodash` | Yes (public data; token for rate limits only) |
| Private repos you do not own | `another-org/internal-tool` | No |

### Attested workflow (CI with GITHUB_TOKEN)

The workflow path uses the ambient `GITHUB_TOKEN`, which is scoped to the
repository hosting the workflow. It can read PR data from that repository and
from any public repository on GitHub. It cannot read data from other private
repositories you own:

| What it covers | Example | Visible |
|---|---|---|
| The workflow-hosting repo | `your-name/coderepute-self-run` | Yes |
| Your own public repos | `your-name/open-source-tool` | Yes |
| Public OSS repos you contribute to | `lodash/lodash` | Yes (public data) |
| Your other private repos | `your-name/private-project` | No -- use the local trial instead |

To cover an org's private repositories, you need the org's own installation
token, which is what the [org setup guide](github.md) covers.

---

## Honesty section: what a self-run proves vs an org run

Reading a self-run-attested report, a verifier sees the same Sigstore
attestation signature that an org run produces. The trust model has important
differences:

### What is the same

- **Integrity proof.** The Sigstore signature proves `report.pdf` and
  `report.html` are bit-for-bit the files the CI run attested. Any edit after
  the fact fails `gh attestation verify`.
- **Workflow identity.** The certificate records
  `gkanitz/CodeRepute/.github/workflows/coderepute-report.yml` at the pinned
  version. Verifiers can run `--signer-workflow` to confirm the producing code
  is the canonical, unmodified CodeRepute action.
- **Coverage stamp.** The report's `coverage` block records every repository
  queried, the time window, and `token_scope_class` (which will read
  `"fine-grained-pat"` for an org-run report that uses an app token, or the
  token class of whatever credential the workflow used). A verifier can see
  exactly what scope the token carried -- nothing is hidden.

### What is different

- **No org endorsement.** The report was produced in a personal repository with
  a personal token. It proves the developer ran CodeRepute against their own
  accessible repositories; it does **not** prove that an employer or
  organisation reviewed, approved, or endorsed the report.
- **Self-selected coverage, bounded by token scope.** For the local trial, you
  choose which repositories to include (the PAT can read whatever you have
  access to). For the workflow path, the ambient `GITHUB_TOKEN` further limits
  coverage to the hosting repository plus public repos -- your other private
  repos are not reachable. An org run, by contrast, an admin configures, so it
  covers a known set of org repositories -- omit a repo and it shows in the
  coverage gap. In a self-run, the reader must trust that the included repos are
  representative and that no private repos were silently omitted.
- **Runner environment.** The self-run workflow runs in your personal
  repository's CI, which uses GitHub-hosted runners by default. A verifier may
  additionally require that `runner_environment == github-hosted` to rule out
  tampering on a self-hosted runner where the environment is not
  GitHub-controlled. If you use a self-hosted runner, the report is still
  attested, but a sceptical verifier could argue the runner itself might have
  been tampered with.

### How to communicate the difference

When sharing a self-run report, be upfront:

> This report was produced by a personal CodeRepute run. The Sigstore
> attestation proves the data comes from the unmodified CodeRepute action at
> version v0.1.0 and has not been edited. The coverage is limited to
> repositories my personal token could read -- my own public repos and the
> public OSS repos I contribute to. It does not carry my employer's endorsement.

A thoughtful reader may additionally ask:

- Were the repositories included chosen selectively? (Share the coverage list
  from the report's `coverage.repos` block.)
- Was a GitHub-hosted runner used, or could the runner environment have been
  tampered with? (The workflow above uses the default `ubuntu-latest`, which is
  GitHub-hosted.)
- Does the report cover private repositories the developer worked in? (If you
  used the local trial instead of the workflow, explain that the local trial
  covers any repo the PAT could read but carries an `unverified` status instead
  of a Sigstore attestation. The data is the same; only the attestation differs.)

An org-run report does not have to answer these questions -- the org's CI
policies and admin oversight provide that context by default.

---

## Next steps

- [Org setup guide](github.md) -- run CodeRepute in your organisation's CI.
- [Verification documentation](../verification.md) -- full trust model and
  verification procedures.
- [GitLab CI setup](gitlab-ci-verification.md) -- run on GitLab (no Sigstore
  attestation; see the GitLab guide for the differences).

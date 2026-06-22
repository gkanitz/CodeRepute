# CodeRepute

**Verifiable developer collaboration reports for private-repo engineers.**

[![CI](https://github.com/grkanitz/CodeRepute/actions/workflows/ci.yml/badge.svg)](https://github.com/grkanitz/CodeRepute/actions/workflows/ci.yml)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8.svg)](go.mod)
[![GitHub Marketplace](https://img.shields.io/badge/GitHub%20Marketplace-CodeRepute-blue?logo=github)](https://github.com/marketplace/actions/coderepute-report)

Generate a cryptographically attested report of a developer's GitHub or GitLab
collaboration activity — pull requests authored, code reviews given, review
comment depth, time to merge, and activity cadence — directly from API
metadata, with no source code access required.

The report runs inside your organization's CI pipeline, attests the output
with a Sigstore signature, and produces a self-contained HTML file and a
machine-readable JSON record that hiring managers and engineering teams can
independently verify have not been edited after collection.

---

## Why CodeRepute

Most developer analytics tools either require public repositories, read source
code, or produce unverifiable self-reported numbers. CodeRepute is different:

- **Works on private repositories** — runs inside your org's CI with a
  narrowly-scoped read-only token; no public exposure required.
- **No source code access** — reads only API event metadata (pull requests,
  reviews, comments). Repository contents are never fetched.
- **Cryptographically attested** — the GitHub Actions integration signs
  `report.html` and `report.pdf` with Sigstore artifact attestations. Anyone
  can verify the files have not been modified since the CI run that produced them.
- **Self-contained output** — a single HTML file with inline charts, no
  external dependencies. Share by email or attach to a job application.
- **GitHub and GitLab** — both platforms supported with the same schema.
- **Apache-2.0, self-hosted** — no data leaves your org; no third-party
  SaaS; no account required.

---

## What the report measures

| Metric | What it shows |
|---|---|
| Pull requests authored / merged | Shipping cadence |
| Reviews given (approve / changes requested) | Peer review engagement |
| Deep review % (≥ 3 inline comments) | Review depth, not just approval clicks |
| Review comments written / received | Collaboration texture |
| Median time to merge | PR scoping and team review responsiveness |
| Time to first review | How quickly teammates pick up your PRs |
| Rework rate | Share of PRs that required a revision cycle |
| Active days / contribution cadence | Consistency of engagement over the window |
| Monthly trend charts | How contribution patterns evolved over time |

Every metric ships with honest interpretation copy and explicit statements of
what it cannot show. No composite score is computed.

---

## Who uses it

**Developers job-hunting from private-repo roles** — most of your best work
lives in private repositories. CodeRepute gives you a shareable, verifiable
record of collaboration activity without exposing any code or repo names.

**Engineering managers evaluating candidates** — request a report as part of a
technical screen. The attestation proves the numbers come directly from the
platform API and were not edited by the candidate.

**Staff engineers and tech leads** — demonstrate code review investment and
team impact that doesn't show up in personal commit counts.

---

## Quick start

### Install

Download a pre-built binary from [GitHub Releases](https://github.com/grkanitz/CodeRepute/releases):

```sh
# macOS / Linux (replace OS and ARCH as needed)
curl -fsSL https://github.com/grkanitz/CodeRepute/releases/latest/download/coderepute_linux_amd64.tar.gz \
  | tar -xz -C /usr/local/bin coderepute
```

Or build from source (requires Go 1.21+):

```sh
go install github.com/grkanitz/coderepute/cmd/coderepute@latest
```

### Run locally

```sh
coderepute -repo owner/repo -subject username -out ./report
```

A GitHub token is read from `-token` or `GITHUB_TOKEN`. Local runs produce
a full report but carry `"status": "unverified"` — cryptographic attestation
is only available in CI.

**Cover multiple repositories in one pass:**

```sh
coderepute -repo owner/repo1,owner/repo2 -subject username -out ./report
```

**Cover an entire GitHub organisation** — every repository visible to the token:

```sh
coderepute -org your-org -subject username -out ./report
```

**Cover an entire GitLab group:**

```sh
coderepute -platform gitlab -group your-group -subject username -out ./report
```

---

## Run in CI with attestation

### GitHub Actions

The recommended path is the **canonical reusable workflow**. When you pin it
to a tagged version, the Sigstore certificate records the producing workflow
identity as `grkanitz/CodeRepute/.github/workflows/coderepute-report.yml` at
that exact tag — making it machine-checkable that an unmodified copy of
CodeRepute produced the report, not a fork:

```yaml
jobs:
  coderepute:
    permissions:
      contents: read
      pull-requests: read
      id-token: write       # Sigstore OIDC signing
      attestations: write   # store attestation on the repo
    uses: grkanitz/CodeRepute/.github/workflows/coderepute-report.yml@v0.1.0
    with:
      repos: your-org/your-repo   # or: org: your-org
      subject: some-username
```

Alternatively, use the **composite action** directly when you need to add
steps after the report (email, Slack, Pages — see the next section). The
report is still fully attested; only the signer-workflow identity differs:

```yaml
jobs:
  coderepute:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      pull-requests: read
      id-token: write
      attestations: write
    steps:
      - uses: grkanitz/CodeRepute@v0.1.0
        with:
          repos: your-org/your-repo   # or: org: your-org
          subject: some-username
```

See [docs/setup/github.md](docs/setup/github.md) for the full setup guide
including GitHub App token configuration.

### GitLab CI

```yaml
include:
  - component: gitlab.com/grkanitz/coderepute/coderepute-report@v0.1.0
    inputs:
      subject: some-gitlab-username
      group: your-group
```

See [docs/setup/gitlab.md](docs/setup/gitlab.md) for the full setup guide.

---

## Automated org-wide reports

Run CodeRepute on a schedule for every engineer in your organisation and
distribute the results automatically. The workflow below generates one
attested report per person every Monday morning.

### Step 1 — Define your team

Create `.github/coderepute-subjects.json` in the repository that runs the
workflow:

```json
[
  { "username": "alice",   "email": "alice@your-org.com" },
  { "username": "bob",     "email": "bob@your-org.com" },
  { "username": "charlie", "email": "charlie@your-org.com" }
]
```

### Step 2 — The workflow

```yaml
name: weekly-coderepute-reports

on:
  schedule:
    - cron: '0 7 * * 1'   # Every Monday at 07:00 UTC
  workflow_dispatch:

jobs:
  setup:
    runs-on: ubuntu-latest
    outputs:
      matrix: ${{ steps.load.outputs.matrix }}
    steps:
      - uses: actions/checkout@v4
      - id: load
        run: echo "matrix=$(jq -c . .github/coderepute-subjects.json)" >> "$GITHUB_OUTPUT"

  report:
    needs: setup
    runs-on: ubuntu-latest
    strategy:
      matrix:
        include: ${{ fromJson(needs.setup.outputs.matrix) }}
      fail-fast: false   # one failure does not cancel other reports
    permissions:
      contents: read
      pull-requests: read
      id-token: write
      attestations: write
    steps:
      - uses: grkanitz/CodeRepute@v0.1.0
        with:
          org: your-org        # covers every repo visible to the token
          subject: ${{ matrix.username }}
          out: report

      # --- distribute: pick one or combine several ---

      # Option A — email the HTML report as an attachment
      - uses: dawidd6/action-send-mail@v3
        with:
          server_address: smtp.gmail.com
          server_port: 465
          username: ${{ secrets.MAIL_USERNAME }}
          password: ${{ secrets.MAIL_PASSWORD }}
          to: ${{ matrix.email }}
          from: Engineering Reports <reports@your-org.com>
          subject: Your collaboration report — ${{ matrix.username }}
          body: Your weekly CodeRepute report is attached. Open in any browser.
          attachments: report/report.html

      # Option B — post a Slack notification with the artifact link
      # - uses: slackapi/slack-github-action@v2
      #   with:
      #     webhook: ${{ secrets.SLACK_WEBHOOK_URL }}
      #     webhook-type: incoming-webhook
      #     payload: |
      #       {"text": "Report ready for ${{ matrix.username }}: ${{ env.ACTIONS_RUN_URL }}"}

      # Option C — push each report to a private GitHub Pages branch
      # - uses: peaceiris/actions-gh-pages@v4
      #   with:
      #     github_token: ${{ secrets.GITHUB_TOKEN }}
      #     publish_dir: report
      #     destination_dir: reports/${{ matrix.username }}
      #     keep_files: true
```

> **Why the composite action here, not the reusable workflow?**
> Reusable workflow jobs cannot have additional steps, so email and Slack
> distribution must run in the same job as the report. The composite action
> still produces a full Sigstore attestation — the only difference is that the
> `--signer-workflow` check points to your org's own workflow rather than the
> canonical CodeRepute workflow. For internal distribution to your own team
> this is exactly the right trust model.

### Distribution options at a glance

| Method | Best for | What to add |
|---|---|---|
| Workflow artifact (default) | Manual download, auditing | Nothing — included automatically |
| Email attachment | Pushing reports to individuals | `dawidd6/action-send-mail` |
| Slack notification | Team visibility with a download link | `slackapi/slack-github-action` |
| GitHub Pages | Browseable history per person | `peaceiris/actions-gh-pages` |
| S3 / Cloud storage | Long-term retention, custom access control | `aws-actions/configure-aws-credentials` + `aws s3 cp` |

---

## Verifying a report

Verification is two steps:

```sh
# 1. Verify the HTML report
gh attestation verify report.html --repo your-org/your-repo

# 2. Verify the PDF report
gh attestation verify report.pdf --repo your-org/your-repo

# 3. Confirm the producing workflow is the canonical CodeRepute action
gh attestation verify report.html --repo your-org/your-repo \
  --signer-workflow grkanitz/CodeRepute/.github/workflows/coderepute-report.yml
```

A modified fork or a locally-edited report fails step 3. If the producing
repository has been deleted or renamed, verification automatically falls back
to the public Sigstore Rekor transparency log.

See [docs/verification.md](docs/verification.md) for the complete trust model,
what passing verification proves, and what it does not.

---

## Platform support

| Platform | Data source | CI integration | Sigstore attestation |
|---|---|---|---|
| GitHub | GitHub REST API | GitHub Actions composite action + reusable workflow | ✅ `actions/attest-build-provenance` |
| GitLab | GitLab REST API | GitLab CI/CD Catalog component | ⚠️ job identity only (no Sigstore) |

---

## Report output

| File | Description |
|---|---|
| `report.html` | Self-contained HTML with inline SVG charts and embedded report JSON. The HTML file itself is the attested artifact — the embedded JSON is not a separate file. |
| `report.pdf` | CI-generated PDF produced by headless Chromium from `report.html`. Independently attested with its own Sigstore signature. |

---

## License

[Apache-2.0](LICENSE)

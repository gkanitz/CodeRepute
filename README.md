# CodeRepute

**Verifiable developer collaboration reports for private-repo engineers.**

[![CI](https://github.com/gkanitz/CodeRepute/actions/workflows/ci.yml/badge.svg)](https://github.com/gkanitz/CodeRepute/actions/workflows/ci.yml)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8.svg)](go.mod)
[![GitHub Marketplace](https://img.shields.io/badge/GitHub%20Marketplace-CodeRepute-blue?logo=github)](https://github.com/marketplace/actions/coderepute-report)

Generate a cryptographically attested report of a developer's GitHub or GitLab
collaboration activity — pull requests authored, code reviews given, review
comment depth, time to merge, and activity cadence — directly from API
metadata, with no source code access required. In an era when AI writes more
code, CodeRepute measures the human contribution that still matters: judgment,
peer review, and collaboration that cannot be faked.

The report runs inside your organization's CI pipeline, attests the output
with a Sigstore signature, and produces a PDF file (the deliverable you
share), a self-contained HTML file (the interactive copy you host), and a
machine-readable JSON record (for tooling) that hiring managers and
engineering teams can independently verify have not been edited after collection.

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
- **PDF for sharing** — the PDF is the artifact you attach to applications
  and send to recruiters. Attested in CI alongside the HTML. The HTML is
  the interactive copy you host or open locally. The JSON is for tooling.
- **GitHub and GitLab** — both platforms supported with the same schema.
- **Apache-2.0, self-hosted** — no data leaves your org; no third-party
  SaaS; no account required.

### Measurement you can trust, not a scoreboard

Developer collaboration is not a competition. Unlike many developer analytics
tools that rank or score individuals — often drawing from the DORA framework or
individual productivity dashboards — CodeRepute presents numbers in context, not
verdicts. It shows collaboration data without reducing it to a single score
or team comparison.

This skepticism is widespread. The
[JetBrains State of Developer Ecosystem 2025](https://byteiota.com/developer-productivity-metrics-crisis-66-dont-trust-dora)
survey (24,534 developers) found that **66% of developers do not trust
DORA metrics** when applied as personal performance indicators — a finding that
aligns with CodeRepute's decision to keep the report score-free.

### Built for the AI era: measuring judgment, not output

As AI writes more code, raw output metrics — PRs authored, commits, active days
— decay as signals of a developer's value. The human contribution shifts toward
**judgment**: reviewing AI-generated code, directing AI agents, and making the
decisions that shape shipped work.

CodeRepute measures this with an uninflatable signal: **reviews given on
AI/bot-authored PRs** and the **deep-review share** on them. It does not infer
whether code "looks AI-generated" (gameable and off-ethos) and does not read
commit messages — an attested invariant recorded in every report's transparency
manifest. It therefore cannot and will not measure how much AI you personally
used, and says so in every report. Classification uses a curated, versioned
recognition ruleset disclosed in the transparency manifest; unrecognized agents
pass as human.

Cryptographic attestation of what really happened becomes more valuable
precisely as AI makes output cheap and fakeable. A Sigstore-signed record of
review engagement and collaboration is the durable, verifiable signal.

---

## What the report measures

| Metric | What it shows |
|---|---|
| Pull requests authored / merged | Shipping cadence |
| Reviews given (approve / changes requested) | Peer review engagement |
| Deep review % (≥ 3 inline comments) | Review depth, not just approval clicks |
| AI/bot PRs reviewed + deep-review share | Judgment applied to AI-authored code |
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

Download a pre-built binary from [GitHub Releases](https://github.com/gkanitz/CodeRepute/releases):

```sh
# macOS / Linux (replace OS and ARCH as needed)
curl -fsSL https://github.com/gkanitz/CodeRepute/releases/latest/download/coderepute_linux_amd64.tar.gz \
  | tar -xz -C /usr/local/bin coderepute
```

Or build from source (requires Go 1.21+):

```sh
go install github.com/gkanitz/coderepute/cmd/coderepute@latest
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
identity as `gkanitz/CodeRepute/.github/workflows/coderepute-report.yml` at
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
    uses: gkanitz/CodeRepute/.github/workflows/coderepute-report.yml@v0.1.0
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
      - uses: gkanitz/CodeRepute@v0.1.0
        with:
          repos: your-org/your-repo   # or: org: your-org
          subject: some-username
```

See [docs/setup/github.md](docs/setup/github.md) for the full setup guide
including GitHub App token configuration.

**Self-run (no org required).** Run the same attested workflow from your own
personal repository. No org admin needed.
See the [self-run setup guide](docs/setup/self-run.md) for the complete
walkthrough.

### GitLab CI

```yaml
include:
  - component: gitlab.com/gkanitz/coderepute/coderepute-report@v0.1.0
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
      - uses: gkanitz/CodeRepute@v0.1.0
        with:
          org: your-org        # covers every repo visible to the token
          subject: ${{ matrix.username }}
          out: report

      # --- distribute: pick one or combine several ---

      # Option A — email the PDF report as an attachment
      - uses: dawidd6/action-send-mail@v3
        with:
          server_address: smtp.gmail.com
          server_port: 465
          username: ${{ secrets.MAIL_USERNAME }}
          password: ${{ secrets.MAIL_PASSWORD }}
          to: ${{ matrix.email }}
          from: Engineering Reports <reports@your-org.com>
          subject: Your collaboration report — ${{ matrix.username }}
          body: Your weekly CodeRepute report is attached. Share the PDF with recruiters or attach to applications.
          attachments: report/report.pdf

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
| Email attachment | Pushing reports to individuals (attach the PDF) | `dawidd6/action-send-mail` |
| Slack notification | Team visibility with a download link (link to the PDF or hosted HTML) | `slackapi/slack-github-action` |
| GitHub Pages | Browseable HTML history per person | `peaceiris/actions-gh-pages` |
| S3 / Cloud storage | Long-term retention, custom access control | `aws-actions/configure-aws-credentials` + `aws s3 cp` |

---

## Verifying a report

Verification is two steps:

```sh
# 1. Verify the PDF report
gh attestation verify report.pdf --repo your-org/your-repo

# 2. Verify the HTML report
gh attestation verify report.html --repo your-org/your-repo

# 3. Verify the share card
gh attestation verify card.png --repo your-org/your-repo

# 4. Confirm the producing workflow is the canonical CodeRepute action
gh attestation verify report.pdf --repo your-org/your-repo \
  --signer-workflow gkanitz/CodeRepute/.github/workflows/coderepute-report.yml
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

| File | When to use |
|---|---|
| `report.pdf` | **The file you share.** Attach to job applications, send to recruiters. Attested in CI with its own Sigstore signature. Print-friendly. |
| `report.html` | **The interactive copy you host.** Open locally for the full interactive view with inline SVG charts, or host on your personal site / LinkedIn. Embed the report JSON. Also attested in CI. |
| `card.svg` | Static 1200x627 share card with four headline numbers, QR verify link, and Sigstore attestation mark. Self-contained, no external references. |
| `card.png` | CI-generated PNG from `card.svg`, rendered by headless Chromium. Independently attested with its own Sigstore signature. |

---

## Which file do I share?

Three files come out of a CodeRepute CI run, each for a different purpose:

- **`report.pdf` — send and share.** Attach it to job applications, email it
  to recruiters, upload it to application portals. Corporate email gateways
  commonly flag or quarantine HTML attachments, so the PDF is the safe choice
  for email. It is attested independently in CI and carries a verify QR code
  on its cover page.
- **`report.html` — host and view.** The HTML file is the full interactive
  report with inline SVG charts. Host it on your personal site or LinkedIn
  featured section, or open it locally to browse the metrics. It embeds a
  verified report JSON for tooling and is attested in CI just like the PDF.
- **`card.svg` / `card.png` — the share card.** A static 1200x627 image with
  four headline numbers and a QR verify link. Use it anywhere you need a
  visual summary.

The PDF is what you share; the HTML is what you host; the JSON is for
tooling and the career merge (when it lands). All three artifact types are
actively verified in CI — none is deprecated or demoted.

---

## Feedback

CodeRepute has **no telemetry, analytics, or tracking** of any kind. Feedback is
explicit or it does not exist.

- **Bug report** — found an issue?
  [Open a bug report](https://github.com/gkanitz/CodeRepute/issues/new?template=bug_report.yml)
- **Feedback** — tell us what worked, what didn't, and what's missing.
  [Share your experience](https://github.com/gkanitz/CodeRepute/issues/new?template=feedback.yml)
- **Discussion** — questions, ideas, and general conversation.
  [Launch feedback discussion](https://github.com/gkanitz/CodeRepute/discussions)

## License

[Apache-2.0](LICENSE)

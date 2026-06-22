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
  `report.json` with a Sigstore artifact attestation. Anyone can verify the
  file has not been modified since the CI run that produced it.
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

**Tip:** run against multiple repositories in one pass:

```sh
coderepute -repo owner/repo1,owner/repo2 -subject username -out ./report
```

---

## Run in CI with attestation

### GitHub Actions

Use the composite action, pinned to a tagged version:

```yaml
jobs:
  coderepute:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      pull-requests: read
      id-token: write       # Sigstore OIDC signing
      attestations: write   # store attestation on the repo
    steps:
      - uses: grkanitz/CodeRepute@v0.1.0
        with:
          repos: your-org/your-repo
          subject: some-username
```

This produces `report.json` and `report.html` as workflow artifacts and a
Sigstore attestation over `report.json`. For the strongest trust chain — one
where the producing workflow identity is machine-checkable — use the canonical
reusable workflow instead:

```yaml
jobs:
  coderepute:
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

## Verifying a report

Verification is two steps:

```sh
# 1. Verify the attestation (proves the file is unmodified since the CI run)
gh attestation verify report.json --repo your-org/your-repo

# 2. Confirm the producing workflow is the canonical CodeRepute action
gh attestation verify report.json --repo your-org/your-repo \
  --signer-workflow grkanitz/CodeRepute/.github/workflows/coderepute-report.yml
```

A modified fork or a locally-edited report fails step 2. If the producing
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

Each run produces two files:

- **`report.json`** — machine-readable, schema-versioned. Contains the full
  metric set, a coverage stamp (window, token scope class, org names), and a
  verification block with the attestation URL and the exact `gh attestation
  verify` command.
- **`report.html`** — self-contained HTML with inline SVG charts (stacked
  contribution timeline, per-year activity heatmap, review reciprocity chart).
  No JavaScript, no external resources. Open in any browser or email directly.

---

## License

[Apache-2.0](LICENSE)

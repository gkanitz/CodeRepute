# CodeRepute

Verifiable collaboration reports for private-repo developers.

CodeRepute runs inside your org's CI, reads **only API metadata** — it never
clones repositories or reads source code — and produces an attested report of
one developer's collaboration statistics (PRs authored/merged, reviews given,
responsiveness) that can be shared with recruiters and verified against the
platform's public attestation APIs.

Status: walking skeleton. A minimal end-to-end pipeline exists:
GitHub repo → metrics → versioned schema-v0 report → self-contained HTML.

## Usage

```sh
coderepute -repo owner/name -subject username -out ./out
```

A GitHub token is read from `-token` or the `GITHUB_TOKEN` environment
variable. Local runs always emit a verification block with status
`unverified`; cryptographic attestation only exists in CI.

## Running in CI with attestation

Use the composite action, pinned to a tagged version:

```yaml
jobs:
  report:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      pull-requests: read
      id-token: write
      attestations: write
    steps:
      - uses: grkanitz/CodeRepute@v0.1.0
        with:
          repos: your-org/your-repo
          subject: some-username
```

This produces `report.json` + `report.html` as workflow artifacts and a
Sigstore attestation over `report.json`. The strongest trust chain is the
canonical reusable workflow, whose identity a verifier can check
mechanically. See [docs/verification.md](docs/verification.md) for both
patterns, the exact `gh attestation verify` commands, and what passing
proves.

## License

Apache-2.0

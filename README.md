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

## License

Apache-2.0

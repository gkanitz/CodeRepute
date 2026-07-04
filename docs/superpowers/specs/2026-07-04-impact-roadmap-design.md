# CodeRepute impact roadmap — design (2026-07-04)

Status: approved by owner 2026-07-04 (brainstorming session).
Scope: what makes the report output significantly more impactful for its three
audiences — recruiters reading it, developers sharing it, orgs approving it —
without surrendering the privacy criteria.

## Decisions taken in the session

- **Balanced roadmap** across three axes: recruiter legibility, developer
  shareability, org comfort. No axis waits two releases.
- **New data classes approved** (always reduced to subject-only aggregates,
  never raw): diff shape stats, language/tech-stack mix, commit metadata,
  issue & discussion activity.
- **External context is banded, never percentile.** Qualitative ranges sourced
  from published research/industry data, with citation and caveat. No numeric
  percentile claims, no team comparisons, and — restated as a permanent
  anti-goal — **no composite score**.
- **Employer-facing value = making individual reports org-friendly.** No
  team-aggregate report product.
- **GraphQL fetch path approved** for diff-shape data on both providers,
  because the REST file-list endpoints return patch text (code) in the
  response body; the GraphQL file-stats queries do not. This keeps "never
  requests code" literally true at the wire level.
- Standing constraints remain binding: static/client-side only (no hosted
  service, no credentials held), no colleague usernames/titles/branch names,
  safe defaults over configurability, honest verification blocks.

## Research grounding (sources for bands and framing)

- Recruiter behavior: tech-stack fit is the first filter; consistency and
  collaboration signals follow; gameable signals (contribution graphs) are
  distrusted. (Reczee recruiter guides; Riem.ai "9 signals"; GitRoll blog.)
- Time-to-first-review baselines: LinearB 2025 benchmarks median 7–12 h;
  Code Climate Velocity org median ≈ 15 h; top decile ≈ sub-4 h.
- PR size: research recommends ≈ 200–400 changed lines; reviews degrade
  sharply past ≈ 1,000 lines ("Do Small Code Changes Merge Faster?",
  arXiv 2203.05045; Graphite/industry guidance).
- Revert rates: ≈ 1 % of commits in studied OSS projects; 3–5 % industrial
  (Springer EMSE 10.1007/s10664-019-09688-8; Sony Mobile comparative study).
- Selective disclosure: IETF SD-JWT pattern (salted per-claim digests inside
  the signed document, disclosures outside) — draft-ietf-oauth-selective-
  disclosure-jwt.
- Credentials interop: Open Badges 3.0 (W3C VC aligned) finalized 2024,
  adoption still ramping; OB 2.0 still dominant in 2026.

Exact band values live in `metrics/bands/bands.json` (embedded, versioned),
each entry carrying its citation; the values above are the starting sources,
re-verified at implementation time.

## Release plan

### v0.2 "Meaning" — every number becomes legible; org trust moves early

| Slice | Feature | Size |
|---|---|---|
| A | Banded context layer (KPI context lines with citations) | S |
| B | GraphQL diff-shape fetch capability, both providers | M |
| C | PR-size stats + normalized review depth (implements #26) | M |
| D | Verified tech-stack fingerprint | M |
| E | Narrative front page + interviewer kit (render-only) | S |
| F | Verified share card artifact | S |
| G | Transparency manifest ("what this tool read") | M |

Dependencies: A ∥ B ∥ F start immediately; C, D after B; E after A (soft:
narrates whatever sections exist); G last (must reflect the final access
surface).

### v0.3 "Proof that travels" (sketch — each gets its own design pass)

- **Selective disclosure / redactable reports**: salted per-section digests
  (`section_digests`) in the attested JSON, SD-JWT style; `coderepute redact`
  strips sections; verify page validates remaining sections against digests.
- **Revert-rate lower bound**: detected from PR metadata only (platform revert
  PRs carry title patterns + a cross-reference to the original PR). Reported
  as an explicit lower-bound heuristic with band context (~1 % OSS, 3–5 %
  industrial). Matched text is never quoted.
- **Glue-work section**: counts of issues opened/triaged/closed and
  discussions answered (GitHub Discussions is GraphQL-only — fits the new
  fetch path).

### v0.4 "Career asset" (sketch)

- **Career timeline merge**: `coderepute merge` combines multiple attested
  reports into one career-view HTML; each segment independently verifiable.
- **Org policy file**: `.github/coderepute-policy.yml`; report stamps
  compliance so admins approve once.
- **Open Badges 3.0 export**: headline claims as a W3C-VC-compatible JSON.

## Cross-cutting design (v0.2)

### Schema evolution

`schema_version` stays `v0`; every addition is an optional `omitempty` block.
`report.Validate()` must keep accepting documents without any new block.
Bump only on a breaking change.

New top-level / nested blocks introduced in v0.2:

```
report.bands                     {version, entries[{key, range, label, source, caveat}]}
collaboration.pr_size            {count, median_lines, files_median, small_share, small_threshold_lines}
collaboration.reviews_given.*    normalized-depth fields (formula per issue #26)
collaboration.language_mix       {basis, pr_count, languages[{name, share_pct}], other_share_pct}
report.access_manifest           {endpoints[{class, count}], never_requested[], notes}
```

### Bands subsystem

- `metrics/bands/bands.json` embedded via `go:embed`, with a top-level
  `version` and one entry per metric key.
- The report JSON embeds `bands.version` plus the entries actually used, so
  rendered context is part of the attested artifact.
- Render rules: neutral typography only (no red/green judgment), every band
  line carries a citation and the caveat that ranges vary by team size and
  workflow. A missing metric renders no orphan context line.

### Privacy safeguards catalog

Every new metric/section must state which safeguards apply; tests enforce
them. The catalog:

1. **Subject-only aggregates** — nothing keyed by any other person.
2. **Paths→extensions at the adapter boundary** — file paths from diff-stats
   queries are reduced to extensions inside the provider adapter; no type
   outside the adapter carries a path. Paths never appear in JSON, HTML,
   logs, or fixtures beyond recorded raw API responses.
3. **Small-slice folding** — language shares < 3 % fold into "Other" so niche
   internal tech never leaks.
4. **Minimum-sample suppression** — sections are omitted entirely (with a
   machine-readable reason) below sample thresholds: language mix and
   pr_size require ≥ 5 merged PRs.
5. **Patterns, never quotes** — any pattern-matched signal (v0.3 reverts)
   reports counts only; matched text is never reproduced.
6. **Prohibited-strings render test** — the seeded-strings golden test
   (see `render/testdata/sample-report.json`) extends to every new section:
   colleague names, PR titles, branch names, file paths seeded in fixtures
   must never appear in rendered output.
7. **No-judgment copy** — narrative and bands use descriptive language;
   grading adjectives are test-blocked via a wordlist.

### GraphQL fetch capability

- Additive `provider` interface method returning per-PR diff shape:
  `[]FileStat{Ext, Additions, Deletions}` — Ext already reduced by the
  adapter to one canonical form: lowercase, no leading dot, empty string
  for extensionless files.
- Adapters: GitHub GraphQL `pullRequest.files` (paths + counts, no patch);
  GitLab GraphQL `mergeRequest.diffStats` (paths + counts, no patch).
- Capability probe: when GraphQL is unavailable/forbidden, adapters return a
  typed unsupported error; downstream metrics honestly omit their sections.
- The recorded GraphQL query strings must contain no field that returns diff
  or file content; a test asserts this against the fixture requests.

### Transparency manifest

- Counting middleware wraps each adapter's HTTP client; every request must
  map to a registered route class (REST route template or GraphQL query
  name). An unregistered call fails tests — this is the honesty invariant
  that keeps the manifest complete forever.
- `access_manifest.never_requested` is a static per-adapter declaration
  (file contents, diffs/patches, branches, colleague profiles, commit
  contents), rendered in a plain-language "What this tool read" section.

### Narrative derivation rules

- Implemented as pure, unit-testable rule functions over the report struct;
  the technical annex lists every rule that fired ("how this narrative was
  derived"). No rule may reference data outside the attested JSON.
- Interviewer kit: exactly three probes, selected by a documented priority
  order, phrased as questions; graceful omission when sections are missing.

### Share card

- `card.svg` (1200×627) rendered from the same report struct: username,
  platform, org, coverage window, four fixed headline numbers (PRs merged,
  reviews given, median time to merge, active days), verify-page QR,
  "Sigstore attested" mark. CI converts to PNG with the existing
  headless-Chromium step and attests the card as a third artifact.

## Slice acceptance criteria

Authoritative success/failure criteria live in the GitHub issues (one per
slice, milestone "v0.2 Meaning"), written for TDD: success criteria are the
test list; failure criteria are the QA-agent red flags. This spec is the
shared design context those issues reference.

## Anti-goals (permanent)

- No composite score, ranking, or grade.
- No within-team or named-colleague comparison.
- No source code, diff content, or file paths in any output.
- No hosted service; no credentials held by the project.

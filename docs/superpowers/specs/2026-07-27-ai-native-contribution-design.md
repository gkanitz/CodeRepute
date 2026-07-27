# CodeRepute AI-native contribution — design (2026-07-27)

## Purpose

Adapt CodeRepute to AI-native development workflows. As AI writes more code, raw
output metrics (PRs authored, commits, active days) decay as signals of a
developer's value, while the human's real contribution shifts toward judgment.
This feature adds an **AI collaboration** dimension that measures the human
contribution AI makes *more* valuable — the human's review judgment applied to
AI-authored code — using only attestable facts within CodeRepute's existing
privacy invariants. It is wave one of a named multi-wave roadmap theme (see
"Roadmap theme").

## How grilling reshaped this design (record)

This spec was grilled against the actual Go codebase. Two findings invalidated
parts of the original brainstorm and are recorded so the reasoning survives:

1. **No commit messages.** `provider/manifest.go` carries a static,
   code-review-enforced `neverRequested` list that includes "commit contents or
   messages", attested verbatim in the transparency manifest. The original
   Signal 1 mechanism (reading `Co-authored-by:` trailers from commit messages)
   would break this attested invariant. Rejected.
2. **Account-ID attribution.** The adapters attribute PRs by immutable account ID
   (`if p.User.ID == subjectID`). A PR authored by an agent identity is therefore
   "someone else's PR", never the subject's, so "the subject's own PRs authored
   via an agent" has no honest data source. The original Signal 1 (declared AI
   co-authorship of the subject's own work) is therefore **dropped entirely**.

The feature narrows to the one signal that survives cleanly — human review of
AI-authored PRs — plus an honest disclosure of why self-AI-usage is not measured.
That disclosure is a trust statement, not merely a limitation.

## Decisions taken

- **Job:** measure the human's AI-native contribution (judgment), with a
  secondary positioning angle that CodeRepute's signals survive when raw output
  is AI-gameable.
- **No inference.** Only attestable facts; never estimate/classify AI in
  undeclared code. A "looks AI-generated" classifier is rejected as gameable and
  off-ethos.
- **One signal for wave 1** (human review of AI-authored PRs). Declared
  self-AI-usage is not measurable within the invariants and is dropped, not
  deferred. Issue/spec authorship (the "directing" signal) is a later wave.
- **Recognition is embedded and version-pinned** (no runtime override in wave 1),
  because runtime override would break the report's attestation claim.
- No composite score, no ranking, no within-team/named-colleague comparison.

## The signal — Human review of AI-authored PRs

Of the reviews the subject *gave*, how many were on AI/bot-authored PRs, and the
deep-review rate on them:

> "You reviewed N AI/bot-authored PRs, M of them deeply (>= 3 inline comments)."

This is the uninflatable, AI-era-defining human work: applying judgment to
machine-generated code. It reuses CodeRepute's existing size-normalized
deep-review computation (`metrics/reviews.go`, `deepReviewThreshold(PRLines)`
with the legacy `>= 3` fallback), segmented by whether the reviewed PR was
AI/bot-authored. Ships with the standard "what this cannot show" copy. It is a
contribution measure, not a quality grade.

### Privacy-preserving classification (in-adapter)

The reviewed PR's author (`p.User`) is available inside the adapter at the
"someone else's PR" branch. The adapter classifies that author against the
recognition ruleset and retains **only a small class string** on
`provider.Review` — the recognized agent id (`"copilot"`, `"devin"`, ...) when
the ruleset matches, `"bot"` when only structural bot-type matches, `""` for a
human author. The human colleague's identity never leaves the adapter, fully
preserving the "subject-only data leaves the adapter / no colleague profiles"
architecture. The class string is the *machine's* identity, never a human's.

## Recognition mechanism (layered, embedded, versioned)

- **Layer 1 — curated agent ruleset.** A versioned `airuleset.json` embedded via
  `go:embed`, following the existing `metrics/bands/bands.json` precedent
  (top-level version + entries). Entries map recognized agent identities (bot
  logins / account handles) to a canonical agent id. Matched against the
  reviewed PR's author identity in the adapter.
- **Layer 2 — structural bot-type.** GitHub API author `type: "Bot"` (and the
  `*[bot]` login pattern); on GitLab the bot-type signal is weaker, so detection
  degrades gracefully to the ruleset-login match and discloses the weaker
  coverage honestly. Structural-only matches are labeled `"bot"`, not a named AI.
- **Versioned + attested.** The ruleset version is known at build time and
  embedded in the report (like the verification block), so an older report stays
  honest after the rules change.
- **Published in the transparency manifest**, verbatim: the ruleset version and
  the agent ids recognized in this window.
- **Extensible via community PR + release** (add a rule to the embedded JSON, a
  data change, version bumps). **No runtime override in wave 1** — that would
  make the attested ruleset version unverifiable.

## How the signal surfaces in the report

- A new **"AI collaboration" section** in the report body (HTML/PDF): the
  aggregate — "you reviewed N AI/bot-authored PRs, M deeply" — with interpretation
  and "what this cannot show" copy. No per-agent table in the body (keeps it
  non-scoreboard). No score, no rank.
- **Structured JSON fields** (additive, informational), mirroring existing
  metrics in the report JSON.
- **Transparency manifest entries:** ruleset version; the agent ids recognized in
  this window (per-agent detail lives here, not the report body); and the
  **Signal-1-absence disclosure**: "CodeRepute does not measure how much AI you
  personally used — that would require reading your commit messages, which we
  attest we never do."
- **Zero-state is shown, not hidden:** "No AI/bot-authored PRs reviewed in this
  window" so the reader knows it was checked.
- **Share card unchanged** (YAGNI).

## Data sources and constraints

- Reuses data the adapters already fetch: reviews given, and the reviewed PR's
  author (already in hand at the classification point). **No new endpoints**, so
  the `neverRequested` invariant and route tables are unchanged — the manifest's
  "no commit data" claim stays true. (This is a benefit of dropping Signal 1: the
  original per-PR commit fetch is no longer needed.)
- GitHub + GitLab: classification runs in both adapters; GitLab's weaker bot-type
  signal is disclosed, not papered over.
- No source, diffs, file paths, or colleague identities in any output (unchanged).

## Testing

Deterministic, no LLM:

- Unit: ruleset matching (author identity -> class), deep-review segmentation by
  class (reuse `metrics/reviews.go` logic), zero-state, JSON shape, manifest
  carrying ruleset version + recognized agents + disclosure.
- Golden-report fixtures (`render/testdata`): one populated case (reviews on
  AI/bot-authored PRs, some deep) and one zero-state case, following the pattern
  established by the transparency-section work (`60-transparency.tmpl`).

## Roadmap theme: AI-native contribution (multi-wave)

CodeRepute's durable moat is cryptographic attestation of what really happened,
which becomes more valuable as AI makes output cheap and fakeable. Metrics must
evolve from "how much did you do" toward "the nature and quality of your
judgment."

- **Wave 1 (this spec):** human review of AI-authored PRs.
- **Wave 2:** issue/spec authorship (the "directing" signal).
- **Wave 3 (candidate):** rework-caught (did the human catch AI's mistakes; builds
  on the existing rework-rate metric); agent-orchestration footprint.
- **Accepted blind spots (permanent):** self-AI-usage (would require reading
  commit messages — attested never); prompt authoring and in-editor AI direction
  (happen in tools CodeRepute structurally cannot see). The tool will not pretend
  to measure them, and says so.

## Anti-goals (permanent)

- No inference/estimation of undeclared AI involvement.
- No reading of commit messages/contents (unchanged attested invariant).
- No composite AI score, no ranking, no "AI-nativeness" grade.
- No named-colleague identity in any output.
- No new API endpoints beyond what the adapters already request.

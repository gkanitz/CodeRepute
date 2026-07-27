# CodeRepute AI-native contribution — design (2026-07-27)

## Purpose

Adapt CodeRepute to AI-native development workflows. As AI writes more code,
raw output metrics (PRs authored, commits, active days) decay as signals of a
developer's value, while the human's real contribution shifts toward judgment
and direction. This feature adds an **AI collaboration** dimension that measures
the human contribution AI makes *more* valuable, using only declared and
attestable facts. It is wave one of a named multi-wave roadmap theme (see
"Roadmap theme" below).

## Decisions taken in the session

- **Job:** measure the human's AI-native contribution (judgment/direction), with
  a secondary positioning angle that CodeRepute's signals are the ones that
  survive when raw output is AI-gameable.
- **No inference.** CodeRepute never estimates or classifies AI involvement in
  undeclared code. It surfaces only *declared, attestable* AI signals. A
  probabilistic "this looks AI-generated" classifier is rejected as gameable and
  against the tool's attest-don't-guess ethos.
- **Two signals** (Approach 1). Issue/spec authorship (the "directing" signal,
  Approach 2) is deliberately a later wave.
- **Layered recognition**, built to evolve, because the AI tooling landscape is
  in its infancy and conventions will change over the coming years.
- No composite score, no ranking, no within-team/named-colleague comparison
  (unchanged anti-goals).

## The two signals

### Signal 1 — Declared AI co-authorship (a transparent fact about the subject's own work)

Counts the subject's merged PRs whose commits carry a recognized AI
`Co-authored-by:` trailer, plus PRs the subject opened via a recognized agent
identity. Reported as a plain rate:

> "X of your Y merged PRs (Z%) carried a declared AI co-author."

**Framed explicitly as a lower bound**, stated inline in the report: this counts
only *declared* AI involvement; undeclared AI use is invisible and the report
says so. The disclosure is the honest move — it turns the limitation into a
trust signal rather than a false-precision claim. Never scored, never presented
as negative.

### Signal 2 — Human review of AI output (the star signal)

Of the reviews the subject *gave*, how many were on AI/bot-authored PRs, and the
deep-review rate (>= 3 inline comments) on them:

> "You reviewed N AI/bot-authored PRs, M of them deeply (>= 3 inline comments)."

This captures the uninflatable, AI-era-defining human work: applying judgment to
machine-generated code. It reuses CodeRepute's existing review-depth machinery,
segmented by whether the reviewed PR was AI/bot-authored. No privacy concern: it
is about the subject and the machine, not named colleagues, so it does not touch
the no-named-colleague-comparison rule.

Both signals ship with CodeRepute's standard "what this cannot show" copy.
Signal 1 is a lower bound; Signal 2 is a contribution measure, not a quality
grade.

## Recognition mechanism (layered, versioned, extensible)

### Layer 1 — Curated AI-agent ruleset (precise "which AI")

A **versioned data file** mapping recognized AI identities to the signals they
emit: `Co-authored-by:` trailer names/emails (e.g. `Claude` /
`noreply@anthropic.com`, `Copilot` / `copilot@github.com`, Cursor, Devin) and
known agent bot-logins. Structured as *rules*, not a flat list, so a new
recognition type (a future standardized AI-provenance trailer, a native
platform AI-attribution field) is added as a new rule without rearchitecting.
This is the concession to the infancy of the environment.

### Layer 2 — Structural bot detection (coarse "bot-authored")

The GitHub/GitLab API author `type: Bot` / App identity and the `*[bot]` login
pattern. Catches bot-authored PRs generically, labeled honestly as
"bot-authored" (not "AI"), since a `[bot]` could be a CI bot.

### Three properties that make it honest and durable

- **Versioned + attested.** The report records which recognition ruleset version
  produced it, and that version is inside the attested output, so an older
  report stays honest after the rules change.
- **Published in the transparency manifest**, verbatim: "CodeRepute recognized
  AI involvement using ruleset vN; here are the identities and rules it
  matched." Limitation-as-honesty.
- **Extensible** by config/community PR — recognizing a new agent means adding a
  rule, not shipping a code change.

## How the signals surface in the report

- A new **"AI collaboration" section** in the report body (HTML/PDF) with the two
  signals and their inline lower-bound / interpretation copy. No score, no rank.
- **Structured JSON fields** (additive, informational) so the data is
  machine-readable, mirroring how existing metrics live in the report JSON.
- **Transparency manifest entries:** the recognition ruleset version, the
  identities/rules matched, and the explicit lower-bound statement.
- **Zero-state is shown, not hidden.** When there is no declared AI involvement
  (the common case today), the section renders "No declared AI involvement
  detected in this window" so the reader knows it was checked, not skipped.
- **Share card unchanged** in this first cut (YAGNI). The AI section lives in the
  full report, not the one-glance card.

## Data sources and constraints

- **Signal 1** reads commit **messages** of the subject's merged PRs (PR-commits
  API) for `Co-authored-by:` trailers, plus PR author identity. Commit *messages*
  are metadata; diffs/content are never fetched. The no-source-access invariant
  is preserved.
- **Signal 2** correlates the subject's existing review data with the reviewed
  PR's author identity (AI/bot flag); mostly reuses data already collected.
- **Both platforms:** `Co-authored-by:` is a git convention that works on GitHub
  and GitLab; structural bot-type detection is stronger on GitHub than GitLab, so
  the design degrades gracefully (GitLab may yield the co-authorship signal but a
  weaker bot-type signal, disclosed honestly).
- **API-cost constraint to verify at implementation:** Signal 1 may add per-PR
  commit-message fetches. If CodeRepute does not already pull PR commits, that is
  added API cost, relevant to the GitHub-App-vs-OAuth rate-limit consideration.
  Not a blocker; an implementation check.

## Testing

Deterministic, no LLM (on ethos):

- Unit: ruleset matching (given trailers/authors -> recognized or not),
  lower-bound copy presence, zero-state rendering, JSON shape, manifest carrying
  the ruleset version.
- Golden-report fixtures: one populated case (declared co-authorship +
  AI-authored-PR reviews) and one zero-state case.

## Roadmap theme: AI-native contribution (multi-wave)

Strategic framing recorded so the arc is on the record. CodeRepute's durable
moat is cryptographic attestation of *what really happened* — which becomes more
valuable, not less, as AI makes output cheap and fakeable. The metrics on top
must evolve from "how much did you do" toward "the nature and quality of your
judgment and direction."

- **Wave 1 (this spec):** judgment on AI output + declared AI co-authorship.
- **Wave 2:** issue/spec authorship (the "directing" signal) — Approach 2.
- **Wave 3 (candidate):** rework-caught (did the human catch AI's mistakes;
  builds on the existing rework-rate metric); agent-orchestration footprint.
- **Accepted blind spots:** prompt authoring and in-editor AI direction happen in
  tools CodeRepute structurally cannot see; the tool will not pretend to measure
  them.

## Anti-goals (permanent)

- No inference/estimation of undeclared AI involvement.
- No composite AI score, no ranking, no "AI-nativeness" grade.
- No named-colleague comparison.
- No source code, diffs, or file paths in any output (unchanged).

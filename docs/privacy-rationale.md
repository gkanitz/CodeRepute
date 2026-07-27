# Privacy Rationale

This document explains the design decisions behind what this report
explicitly does *not* do, and why those choices protect the privacy and
fairness of the people whose collaboration activity is being measured.

---

## No Composite Score (anchor: `#no-composite-score`)

Every metric in a CodeRepute report is reported independently as a
distribution or count with its own units, caveats, and interpretation copy.
No single number or grade is derived by weighting, averaging, or otherwise
combining them. This is a deliberate product constraint for three reasons.

First, a composite score compresses multidimensional context into a single
value. A score cannot express, for example, that a developer merged fewer
PRs than median but gave unusually deep reviews and had a very low
rework rate: each dimension tells a different part of the story, and
compressing them hides the signal a human reader would act on.
Second, a single score invites misuse as a ranking proxy. Once a composite
number exists, it is natural to sort people by it, which this product exists
to prevent. Third, a composite score cannot be independently verified.
Each constituent metric can be checked against the underlying API data,
but the weightings and transformations that produce the composite are at
best opaque and at worst arbitrary.

The machine-readable transparency manifest
(`access_manifest.json` and the embedded `access_manifest` block in every
report) records the API endpoints and data classes that were fetched
and explicitly names data that was never requested. The omission entry for
`"composite score"` in that manifest links here as its privacy rationale.

---

## No Team Ranking (anchor: `#no-team-ranking`)

The report places the subject's metrics alongside peer-benchmark bands
(typical ranges sourced from published research) so the reader has a rough
sense of scale. It never assigns a position within those bands and never
compares the subject against another named person, a team, or a cohort.

Ranking against a team or cohort is avoided because team baselines vary
widely with team size, codebase age, project phase, review culture, and
the share of work that happens outside the tracked platform (spec
documents, design sessions, incident response). A developer on a mature
team with a careful review culture may merge fewer PRs but have higher
impact per merge than someone on a fast-moving greenfield team. A rank
captures none of this context and actively misleads.

The transparency manifest records `"team ranking"` as a named omission,
confirming that no ranking computation was performed and no ranking data
was emitted.

---

## No Named-Colleague Comparison (anchor: `#no-named-colleague-comparison`)

The only named individual in any CodeRepute output is the report subject.
Review interactions with colleagues are summarized as aggregate counts
(reviews given, review comments written and received, time to first review)
but the colleagues themselves are never identified by name, username, or
account ID. This applies to every output format: the PDF, the interactive
HTML, the share card, and the embedded JSON report.

This constraint protects the privacy of every developer whose activity
intersects with the subject's. A review comment thread involves two
people (the author and the reviewer) but neither individual consented to
appear in a report about the other. By suppressing all colleague identities,
CodeRepute ensures that reading a subject's report never reveals anything
about a non-subject's behaviour.

The transparency manifest records `"named-colleague comparison"` as a
named omission, and the `never_requested` array explicitly includes
`"colleague profiles"` to confirm that colleague-identifying data was never
fetched from the API in the first place.

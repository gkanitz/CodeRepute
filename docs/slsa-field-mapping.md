# SLSA v1.2 provenance field mapping

This document maps the existing CodeRepute `verification` block fields (and other
report-level metadata) to the corresponding SLSA v1.2 provenance predicate fields
defined in the [SLSA Attestation Model](https://slsa.dev/spec/v1.2/attestation-model).

The mapping is provisional until a human reviewer has approved the field selection
(see [Open questions for human review](#open-questions-for-human-review)).

---

## Field mapping

| Existing CodeRepute field | SLSA v1.2 predicate field | Source | Known gap |
|---|---|---|---|
| `verification.workflow_ref` | `runDetails.builder.id` | `WorkflowRef` field in the `Verification` struct. Populated from `GITHUB_WORKFLOW_REF` (GitHub Actions) or constructed as `<CI_PROJECT_PATH>/.gitlab-ci.yml@<CI_COMMIT_REF_NAME>` (GitLab CI). | The current value is a platform-specific workflow path+ref (e.g. `acme/widgets/.github/workflows/report.yml@refs/heads/main`), not a URI that resolves to a SLSA-compliant builder identity. An OIDC-based builder URI would need to be derived or normalised. |
| `verification.run_id` / `verification.run_url` | `runDetails.metadata.invocationId` | `RunID` (GitHub Actions, from `GITHUB_RUN_ID`) and `RunURL` (both platforms) in the `Verification` struct. On GitHub Actions, `RunURL` is constructed as `<GITHUB_SERVER_URL>/<GITHUB_REPOSITORY>/actions/runs/<GITHUB_RUN_ID>`. On GitLab CI, only `RunURL` (`CI_JOB_URL`) is populated. | SLSA v1.2 defines `invocationId` as a URI identifying the build invocation. The codebase has two separate fields (`run_id` and `run_url`) that together identify a single CI run. The human reviewer must decide whether to map `run_url` (a URI that fits the `invocationId` type) or to synthesise a canonical URI from both. |
| `report.generated_at` | `runDetails.metadata.startedOn` | `GeneratedAt` timestamp on the `Report` struct. Set at build time via `time.Now().UTC()` and serialised as `generated_at`. | `GeneratedAt` records only the finish time of report generation. SLSA requires both `startedOn` (when the build started) and `finishedOn` (when the build completed). The codebase does not currently record a separate **start timestamp** (`startedOn`). See [Open questions](#open-questions-for-human-review). |
| (not yet recorded) | `runDetails.metadata.finishedOn` | Not yet captured. | The codebase records a single generation timestamp (`generated_at`) but does not model a start/finish pair. Adding `finishedOn` would require recording a second timestamp distinct from `generated_at`, or re-purposing `generated_at` as the finish timestamp and adding a new start timestamp. |
| (not yet recorded) | `buildDefinition.resolvedDependencies` | Not yet captured. The pinned action versions are expressed in `action.yml` (e.g. `actions/setup-go@4a3601121dd01d1626a1e23e37211e3254c1c06c # v6.4.0`), but these are not surfaced in the report's JSON. | SLSA v1.2 requires that `resolvedDependencies` list every artifact consumed by the build with its immutable reference. The report does not currently expose any dependency material hash or version. An implementation path would need to extract pinned SHA/tag references from the composite action and surface them in the report output. |
| (not yet recorded) | `buildDefinition.buildType` | Not yet captured. No URI identifying the CodeRepute report build process currently exists. | A URI must be assigned to identify the CodeRepute build process (e.g. `https://coderepute.dev/build-type/v1`). The exact URI value is deliberately left as an open question for the human reviewer. |

---

## Open questions for human review

The following questions require a human decision before the mapping can be finalised
and implemented. Each question is scoped to a single decision; the answer determines
the exact Go struct and JSON output changes needed in a follow-up slice.

### Q1: Which field maps to `invocationId` — `run_url`, `run_id`, or a synthesised URI?

The codebase carries both `verification.run_id` (a numeric string, GitHub only)
and `verification.run_url` (a full URI, both platforms). SLSA v1.2's `invocationId`
is defined as a URI. Two options:

- **A** — Use `verification.run_url` directly (already a URI; present on both
  platforms).
- **B** — Synthesise a canonical invocation URI from `run_id` when `run_url` is
  absent (e.g. `https://github.com/<repo>/actions/runs/<run_id>`), and drop or
  deprecate the separate `run_url` field.
- **C** — Create an additional provenance-only field and keep the existing
  `run_id` / `run_url` fields unchanged.

### Q2: How should start and finish timestamps be modelled?

SLSA v1.2 expects a `startedOn` / `finishedOn` pair. The codebase currently
records only `generated_at`. Three approaches:

- **A** — Record a new `build_started_at` timestamp at the point report generation
  begins, rename the current `generated_at` to `build_finished_at`, and map both
  to SLSA fields; keep `generated_at` as a JSON alias for backward compatibility
  or document the rename as a breaking schema change.
- **B** — Repurpose `generated_at` as `finishedOn` and record a new `startedOn`
  timestamp only (not present in the rendered report unless provenance is active),
  keeping the existing field name unchanged.
- **C** — Leave the existing report schema as-is and populate `startedOn` /
  `finishedOn` only when emitting a SLSA provenance predicate (i.e. as a
  non-breaking addition).

### Q3: What is the exact URI for `buildDefinition.buildType`?

The issue spec explicitly defers the value of the `buildType` URI. The reviewer
should decide on a specific URI string, for example:

- `https://coderepute.dev/build-types/report/v1`
- `https://github.com/gkanitz/CodeRepute/slsa/build-type/v1`
- A GN URI under a registry namespace

### Q4: How should pinned action versions be surfaced as `resolvedDependencies`?

Pinned versions exist today only in `action.yml` comment annotations
(e.g. `actions/setup-go@4a3601121dd01d1626a1e23e37211e3254c1c06c # v6.4.0`).
These are not currently reflected in the report JSON. Two approaches:

- **A** — Extract pinned SHA/tag references from the action manifest at build
  time and include them in the verification block (or a new `provenance` block).
- **B** — Include only the report builder's own version (the `coderepute` binary
  version) as a single dependency entry and omit transitive action dependencies.
  This is simpler but covers fewer SLSA requirements.

### Q5: Should the builder identity (`runDetails.builder.id`) be a normalised URI?

The current `workflow_ref` field is already a URI-like string
(e.g. `acme/widgets/.github/workflows/report.yml@refs/heads/main`). For SLSA
v1.2 compliance, the builder should be identified by a URI (not a platform-specific
path). Two options:

- **A** — Keep the current `workflow_ref` as-is (it is functionally a URI for
  GitHub/GitLab workflows) and map it directly to `builder.id`.
- **B** — Normalise it into a more standard URI form
  (e.g. `https://github.com/acme/widgets/.github/workflows/report.yml@refs/heads/main`)
  or derive an OIDC-based identity.

---

## Review process

This PR is marked `do-not-merge`. Merge is blocked until a human reviewer comments
approval of the field selection above. Implementations based on the approved
mapping will follow in separate slices.

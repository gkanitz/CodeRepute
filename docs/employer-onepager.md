# CodeRepute — Security One-Pager for Org Admins

This document answers the three questions every security review asks before
approving a new tool that touches your source control system.

---

## 1. What does CodeRepute access?

CodeRepute reads only **GitHub REST API metadata**. Specifically:

| Endpoint | Purpose |
|---|---|
| `GET /users/{username}` | Resolve the subject's account ID (immutable; never changes even if the login does) |
| `GET /repos/{owner}/{repo}/pulls?state=all` | List pull requests in the coverage window |
| `GET /repos/{owner}/{repo}/pulls/{number}/reviews` | List review events on each PR |
| `GET /repos/{owner}/{repo}/pulls/comments` | List inline review comments |

That is the complete set. CodeRepute:

- **Never clones repositories.** No `git clone`, no `git archive`, no tarball download.
- **Never reads file contents.** The `/contents`, `/git/trees`, `/git/blobs`,
  and `/zipball`/`/tarball` endpoints are never called.
- **Uses the narrowest credential that works:**
  - A [GitHub App installation token](https://docs.github.com/en/apps/creating-github-apps)
    scoped to `Pull requests: Read` and `Metadata: Read` on selected repositories
    (recommended for org deployments).
  - A fine-grained Personal Access Token with `pull_requests: read` on selected
    repositories.
  - The default `GITHUB_TOKEN` inside a workflow run (read-only by default).

The tool never requests `contents: read`, `code: read`, or any write
permission.

---

## 2. What leaves the repository?

Only **subject-only aggregates** are captured. The adapter drops all colleague
identities, PR titles, branch names, and comment bodies before the data leaves
the API response. What survives into the report:

- **Counts** — how many PRs the subject authored, how many reviews they gave,
  how many inline comments they wrote and received.
- **Rates and breakdowns** — what share of reviews were approvals vs. changes
  requested, what share of the subject's PRs required a rework cycle.
- **Durations** — median hours from PR creation to first review; median hours
  from PR creation to merge.
- **Cadence** — how many days the subject was active and a bucketed trend of
  activity over the window.
- **Coverage stamp** — the org/repo names, the window (start date, end date),
  and the credential scope class (e.g. "app-installation") so any reader
  knows exactly what the run could see.

What is **never** written to the report:

- Colleague names or account IDs.
- PR titles, branch names, commit messages, or commit SHAs.
- Review comment bodies.
- Any file path or code.

The attestation that accompanies a CI run links the report to the producing
workflow identity (org, repository, run URL). It does not contain any code or
API response bodies.

---

## 3. What does the attestation prove?

When a report is produced through the canonical
`gkanitz/CodeRepute/.github/workflows/coderepute-report.yml` reusable
workflow, GitHub's OIDC service signs a Sigstore artifact attestation over
`report.json`. The attestation is stored in the producing repository.

**Passing verification proves:**

- The bytes of `report.json` are unchanged since the attested run — any
  post-run edit to the numbers breaks verification.
- The report was produced inside a GitHub Actions run in the named org and
  repository.
- The producing workflow is the canonical CodeRepute reusable workflow at the
  pinned version — not a modified fork, not a mutable branch.

Verify with:

```sh
gh attestation verify report.json --repo <org/repo> \
  --signer-workflow gkanitz/CodeRepute/.github/workflows/coderepute-report.yml
```

A fork (`someorg/CodeRepute`) produces a different `job_workflow_ref` in the
Sigstore certificate and **fails this command**.

**What the attestation does NOT prove:**

- That the coverage is complete. The report's `coverage` block lists exactly
  which repositories, which time window, and which token scope the run used.
  A token that can see only three of thirty repositories produces a report
  that says so — it is accurate about what it saw, not about what it missed.
- That the underlying GitHub data is honest (e.g. activity manufactured
  before the run window).
- Anything about reports whose `verification.status` is `unverified`. Local
  runs (outside CI) carry an explicit unverified block; they are honest about
  having no chain.

See [docs/verification.md](verification.md) for the full two-step verification
procedure.

---

## What the report contains — in plain language

A CodeRepute report describes one developer's collaboration patterns over a
fixed time window (default: the past year). It is not a performance score; it
is a structured summary of observable workflow behaviour.

**Pull request authorship** — how many PRs the developer opened and how many
were merged. This shows whether the developer is shipping work through the
standard review process.

**Review participation** — how many pull requests the developer reviewed for
colleagues, and whether those reviews approved the work or requested changes.
This reflects how actively the developer engages with their team's work
(rather than only submitting their own).

**Inline feedback** — how many line-level review comments the developer wrote
on colleagues' code, and how many they received on their own. Dense written
commentary is one signal of thorough engagement with code detail.

**Time to first review** — median hours between when the developer opens a PR
and when a colleague first reviews it. This reflects how quickly the team
responds to the developer's contributions.

**Time to merge** — median hours between PR creation and merge. Together with
time to first review, this contextualises the pace of the developer's delivery
cycle.

**Rework rate** — what share of the developer's reviewed PRs had at least one
"changes requested" review before merging. A moderate rework rate is a normal
sign of healthy review; context from the developer is needed to interpret
outliers.

**Activity cadence** — how many days in the window the developer had at least
one contribution, and a bucketed trend chart showing whether activity was
consistent or clustered.

None of these numbers carry a pass/fail threshold. The report is a factual
summary; interpretation is the employer's and candidate's responsibility.

---

## Next steps

- [GitHub setup guide](setup/github.md) — install the app and add the workflow
  in under 15 minutes.
- [GitLab setup guide](setup/gitlab.md) — GitLab CI component (coming in a
  future release).
- [Verification details](verification.md) — full two-step verification
  procedure for security reviewers.

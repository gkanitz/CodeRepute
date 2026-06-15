package report_test

import (
	"testing"
	"time"

	"github.com/grkanitz/coderepute/report"
)

// envFrom returns a getenv func backed by a fixed map, so tests never
// depend on the real process environment.
func envFrom(vars map[string]string) func(string) string {
	return func(key string) string { return vars[key] }
}

// actionsEnv is a complete GitHub Actions identity environment, as the
// runner sets it for any job.
func actionsEnv() map[string]string {
	return map[string]string{
		"GITHUB_ACTIONS":      "true",
		"GITHUB_REPOSITORY":   "acme/widgets",
		"GITHUB_WORKFLOW_REF": "acme/widgets/.github/workflows/report.yml@refs/heads/main",
		"GITHUB_RUN_ID":       "9000000001",
		"GITHUB_SERVER_URL":   "https://github.com",
	}
}

func TestCIVerificationInGitHubActions(t *testing.T) {
	v := report.CIVerification(envFrom(actionsEnv()))
	if v == nil {
		t.Fatal("CIVerification in GitHub Actions = nil, want a populated block")
	}
	if v.Status != report.StatusVerified {
		t.Errorf("Status = %q, want %q", v.Status, report.StatusVerified)
	}
	if v.Provider != "github-actions" {
		t.Errorf("Provider = %q, want github-actions", v.Provider)
	}
	if v.Repository != "acme/widgets" {
		t.Errorf("Repository = %q, want acme/widgets", v.Repository)
	}
	if want := "acme/widgets/.github/workflows/report.yml@refs/heads/main"; v.WorkflowRef != want {
		t.Errorf("WorkflowRef = %q, want %q", v.WorkflowRef, want)
	}
	if v.RunID != "9000000001" {
		t.Errorf("RunID = %q, want 9000000001", v.RunID)
	}
	if want := "https://github.com/acme/widgets/actions/runs/9000000001"; v.RunURL != want {
		t.Errorf("RunURL = %q, want %q", v.RunURL, want)
	}
	if v.Attestation == nil {
		t.Fatal("Attestation = nil, want a pointer to where the attestation can be checked")
	}
	if want := "gh attestation verify report.json --repo acme/widgets"; v.Attestation.VerifyCommand != want {
		t.Errorf("Attestation.VerifyCommand = %q, want %q", v.Attestation.VerifyCommand, want)
	}
	if want := "https://github.com/acme/widgets/attestations"; v.Attestation.URL != want {
		t.Errorf("Attestation.URL = %q, want %q", v.Attestation.URL, want)
	}
	if v.Attestation.Type == "" {
		t.Error("Attestation.Type is empty, want a named attestation type")
	}

	// The populated block must survive a build → marshal → parse round trip.
	r := report.Build(activityFixture(), nil, nil, time.Now())
	r.Verification = v
	if err := r.Validate(); err != nil {
		t.Fatalf("Validate() with CI verification block: %v", err)
	}
}

func TestCIVerificationOutsideCIReturnsNil(t *testing.T) {
	if v := report.CIVerification(envFrom(nil)); v != nil {
		t.Errorf("CIVerification outside CI = %+v, want nil", v)
	}
	// GITHUB_ACTIONS must be exactly "true"; anything else is not CI.
	if v := report.CIVerification(envFrom(map[string]string{"GITHUB_ACTIONS": "false"})); v != nil {
		t.Errorf("CIVerification with GITHUB_ACTIONS=false = %+v, want nil", v)
	}
}

// gitlabEnv is a complete GitLab CI identity environment, as the runner sets
// it for any job.
func gitlabEnv() map[string]string {
	return map[string]string{
		"GITLAB_CI":          "true",
		"CI":                 "true",
		"CI_JOB_URL":         "https://gitlab.com/acme/widgets/-/jobs/1234",
		"CI_PIPELINE_URL":    "https://gitlab.com/acme/widgets/-/pipelines/5678",
		"CI_PROJECT_PATH":    "acme/widgets",
		"CI_COMMIT_REF_NAME": "main",
		"CI_JOB_ID":          "1234",
	}
}

func TestGitLabVerificationInGitLabCI(t *testing.T) {
	v := report.GitLabVerification(envFrom(gitlabEnv()))
	if v == nil {
		t.Fatal("GitLabVerification in GitLab CI = nil, want a populated block")
	}
	if v.Status != report.StatusVerified {
		t.Errorf("Status = %q, want %q", v.Status, report.StatusVerified)
	}
	if v.Provider != "gitlab-ci" {
		t.Errorf("Provider = %q, want gitlab-ci", v.Provider)
	}
	if v.Repository != "acme/widgets" {
		t.Errorf("Repository = %q, want acme/widgets", v.Repository)
	}
	if want := "acme/widgets/.gitlab-ci.yml@main"; v.WorkflowRef != want {
		t.Errorf("WorkflowRef = %q, want %q", v.WorkflowRef, want)
	}
	if want := "https://gitlab.com/acme/widgets/-/jobs/1234"; v.RunURL != want {
		t.Errorf("RunURL = %q, want %q", v.RunURL, want)
	}
	if v.Note == "" {
		t.Error("Note is empty; want an explanation of GitLab attestation limitations")
	}
	// GitLab does not provide Sigstore: no Attestation pointer.
	if v.Attestation != nil {
		t.Errorf("Attestation = %+v, want nil (GitLab has no Sigstore OIDC attestation)", v.Attestation)
	}

	// Must survive build → validate round-trip.
	r := report.Build(activityFixture(), nil, nil, time.Now())
	r.Verification = v
	if err := r.Validate(); err != nil {
		t.Fatalf("Validate() with GitLab verification block: %v", err)
	}
}

func TestGitLabVerificationOutsideCIReturnsNil(t *testing.T) {
	// No env at all → nil.
	if v := report.GitLabVerification(envFrom(nil)); v != nil {
		t.Errorf("GitLabVerification outside CI = %+v, want nil", v)
	}
	// CI=true alone (e.g. some other CI platform) must not trigger GitLab.
	if v := report.GitLabVerification(envFrom(map[string]string{"CI": "true"})); v != nil {
		t.Errorf("GitLabVerification with CI=true but no GITLAB_CI = %+v, want nil", v)
	}
	// GITLAB_CI must be exactly "true".
	if v := report.GitLabVerification(envFrom(map[string]string{"GITLAB_CI": "false"})); v != nil {
		t.Errorf("GitLabVerification with GITLAB_CI=false = %+v, want nil", v)
	}
}

// TestProviderPrecedenceGitHubBeatsGitLab documents the contract that callers
// (main.go) must honour: when both GitHub Actions and GitLab CI env vars are
// present, GitHub Actions is preferred because it provides cryptographic
// attestation. The functions themselves are independent; the precedence lives
// in the caller — CIVerification takes priority over GitLabVerification.
func TestProviderPrecedenceGitHubBeatsGitLab(t *testing.T) {
	// Merge both env sets to simulate the unusual overlap.
	both := make(map[string]string)
	for k, v := range actionsEnv() {
		both[k] = v
	}
	for k, v := range gitlabEnv() {
		both[k] = v
	}

	// CIVerification finds GitHub Actions and returns a github-actions block.
	github := report.CIVerification(envFrom(both))
	if github == nil || github.Provider != "github-actions" {
		t.Errorf("CIVerification with both envs: Provider = %v, want github-actions", github)
	}

	// GitLabVerification also finds GitLab and returns a gitlab-ci block.
	gitlab := report.GitLabVerification(envFrom(both))
	if gitlab == nil || gitlab.Provider != "gitlab-ci" {
		t.Errorf("GitLabVerification with both envs: Provider = %v, want gitlab-ci", gitlab)
	}

	// The correct caller pattern is: prefer GitHub (non-nil CIVerification
	// wins; GitLabVerification is only the else-branch).
	var chosen *report.Verification
	if v := report.CIVerification(envFrom(both)); v != nil {
		chosen = v
	} else if v := report.GitLabVerification(envFrom(both)); v != nil {
		chosen = v
	}
	if chosen == nil || chosen.Provider != "github-actions" {
		t.Errorf("caller precedence: chose Provider = %v, want github-actions", chosen)
	}
}

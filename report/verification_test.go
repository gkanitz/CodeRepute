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

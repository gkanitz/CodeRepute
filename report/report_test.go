package report_test

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/grkanitz/coderepute/provider"
	"github.com/grkanitz/coderepute/report"
)

func activityFixture() provider.ActivitySet {
	return provider.ActivitySet{
		Subject: provider.Subject{
			Platform:  "github",
			Username:  "octocat",
			AccountID: "583231",
		},
		Window: provider.Window{
			Since: time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
			Until: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		},
		Repos:      []string{"acme/widgets"},
		TokenScope: "repo",
	}
}

func TestBuildValidateRoundTrip(t *testing.T) {
	collab := &report.Collaboration{
		PullRequests: &report.PullRequestStats{Authored: 3, Merged: 2},
	}
	now := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)

	r := report.Build(activityFixture(), collab, nil, now)

	if r.SchemaVersion != report.SchemaVersion {
		t.Fatalf("schema version = %q, want %q", r.SchemaVersion, report.SchemaVersion)
	}
	if r.Verification == nil || r.Verification.Status != report.StatusUnverified {
		t.Fatalf("local build must carry an explicit unverified verification block, got %+v", r.Verification)
	}
	if err := r.Validate(); err != nil {
		t.Fatalf("built report failed validation: %v", err)
	}

	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	parsed, err := report.Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !reflect.DeepEqual(r, parsed) {
		t.Errorf("round-trip mismatch:\n built: %+v\nparsed: %+v", r, parsed)
	}
}

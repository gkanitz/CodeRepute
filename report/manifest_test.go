package report_test

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/gkanitz/coderepute/provider"
	"github.com/gkanitz/coderepute/report"
)

// TestManifestRoundTrip verifies that a manifest-bearing report round-trips
// through JSON marshal, Parse, and Validate. (AC-5)
func TestManifestRoundTrip(t *testing.T) {
	collab := &report.Collaboration{
		PullRequests: &report.PullRequestStats{Authored: 3, Merged: 2},
	}
	now := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)

	as := activityFixture()
	as.AccessManifest = provider.Manifest{
		Endpoints: []provider.EndpointCount{
			{Class: "rest:users_show", Count: 1},
			{Class: "rest:list_pulls", Count: 2},
		},
		NeverRequested: []string{
			"file contents (any endpoint)",
			"diffs / patch text (any endpoint)",
			"branch names",
			"colleague profiles",
			"commit contents or messages",
		},
		Notes: "All requests are to the API metadata endpoints only.",
	}

	r := report.Build(as, collab, nil, now)

	if r.AccessManifest == nil {
		t.Fatal("Build() produced nil AccessManifest")
	}
	if len(r.AccessManifest.Endpoints) != 2 {
		t.Fatalf("expected 2 endpoints, got %d", len(r.AccessManifest.Endpoints))
	}
	if r.AccessManifest.Endpoints[0].Class != "rest:list_pulls" && r.AccessManifest.Endpoints[0].Class != "rest:users_show" {
		t.Errorf("unexpected first endpoint class: %q", r.AccessManifest.Endpoints[0].Class)
	}
	if len(r.AccessManifest.NeverRequested) != 5 {
		t.Errorf("expected 5 never-requested items, got %d", len(r.AccessManifest.NeverRequested))
	}
	if r.AccessManifest.Notes == "" {
		t.Error("manifest notes should not be empty")
	}

	// Round-trip through JSON
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	parsed, err := report.Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if parsed.AccessManifest == nil {
		t.Fatal("round-trip lost AccessManifest block")
	}
	if !reflect.DeepEqual(r.AccessManifest, parsed.AccessManifest) {
		t.Errorf("round-trip mismatch:\n  built: %+v\nparsed: %+v", r.AccessManifest, parsed.AccessManifest)
	}
}

// TestBackwardCompatNoManifest verifies that Parse still accepts JSON without
// an access_manifest block (backward compat with pre-v0.2 reports). (AC-5)
func TestBackwardCompatNoManifest(t *testing.T) {
	now := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	r := report.Build(activityFixture(), nil, nil, now)
	r.AccessManifest = nil

	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	parsed, err := report.Parse(raw)
	if err != nil {
		t.Fatalf("parse pre-manifest report: %v", err)
	}
	if parsed.AccessManifest != nil {
		t.Error("parsed report has non-nil AccessManifest despite nil input")
	}
}

// TestBuildWithAccessManifestOption verifies that the WithAccessManifest
// BuildOption works correctly.
func TestBuildWithAccessManifestOption(t *testing.T) {
	now := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	m := provider.Manifest{
		Endpoints: []provider.EndpointCount{
			{Class: "rest:users_show", Count: 1},
		},
		NeverRequested: []string{"file contents"},
		Notes:          "test notes",
	}

	r := report.Build(activityFixture(), nil, nil, now, report.WithAccessManifest(m))

	if r.AccessManifest == nil {
		t.Fatal("WithAccessManifest did not stamp the manifest")
	}
	if len(r.AccessManifest.Endpoints) != 1 {
		t.Errorf("expected 1 endpoint, got %d", len(r.AccessManifest.Endpoints))
	}
	if r.AccessManifest.Endpoints[0].Class != "rest:users_show" {
		t.Errorf("class = %q, want rest:users_show", r.AccessManifest.Endpoints[0].Class)
	}
	if r.AccessManifest.Endpoints[0].Count != 1 {
		t.Errorf("count = %d, want 1", r.AccessManifest.Endpoints[0].Count)
	}
	if len(r.AccessManifest.NeverRequested) != 1 || r.AccessManifest.NeverRequested[0] != "file contents" {
		t.Errorf("never_requested = %v", r.AccessManifest.NeverRequested)
	}
	if r.AccessManifest.Notes != "test notes" {
		t.Errorf("notes = %q, want 'test notes'", r.AccessManifest.Notes)
	}
}

// TestValidateWithManifest ensures a report with an access_manifest block
// passes validation.
func TestValidateWithManifest(t *testing.T) {
	now := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	as := activityFixture()
	as.AccessManifest = provider.Manifest{
		Endpoints: []provider.EndpointCount{
			{Class: "rest:users_show", Count: 1},
		},
		NeverRequested: []string{"file contents"},
		Notes:          "test notes",
	}

	r := report.Build(as, nil, nil, now)
	if err := r.Validate(); err != nil {
		t.Fatalf("Validate with manifest: %v", err)
	}
}

// TestManifestPresentInLocalRun verifies that the manifest is present even
// in unverified (local) runs. (AC-6)
func TestManifestPresentInLocalRun(t *testing.T) {
	now := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	as := activityFixture()
	as.AccessManifest = provider.Manifest{
		Endpoints: []provider.EndpointCount{
			{Class: "rest:users_show", Count: 1},
		},
		NeverRequested: []string{"file contents"},
		Notes:          "local run manifest",
	}

	r := report.Build(as, nil, nil, now)

	if r.Verification.Status != report.StatusUnverified {
		t.Error("local run should have unverified status")
	}
	if r.AccessManifest == nil {
		t.Fatal("local run should carry access manifest")
	}
	if len(r.AccessManifest.Endpoints) == 0 {
		t.Error("local run manifest should have endpoints")
	}

	// Round-trip to verify
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	parsed, err := report.Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if parsed.AccessManifest == nil {
		t.Error("round-trip lost AccessManifest from unverified report")
	}
}

package report_test

import (
	"encoding/json"
	"reflect"
	"strings"
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

// TestOmissionsRoundTrip verifies that a report with omissions round-trips
// through JSON marshal, Parse, and Validate.
func TestOmissionsRoundTrip(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	r := report.Build(activityFixture(), nil, nil, now)
	r.AccessManifest = &report.AccessManifest{
		Endpoints: []report.EndpointCount{
			{Class: "rest:users_show", Count: 1},
		},
		NeverRequested: []string{"file contents"},
		Notes:          "test notes",
		Omissions: []report.OmissionEntry{
			{
				Category:            "composite score",
				Description:         "No single score or grade is derived from the metrics.",
				PrivacyRationaleRef: "docs/privacy-rationale.md#no-composite-score",
			},
		},
	}

	if err := r.Validate(); err != nil {
		t.Fatalf("Validate with valid omissions: %v", err)
	}

	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	parsed, err := report.Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if parsed.AccessManifest == nil {
		t.Fatal("round-trip lost AccessManifest")
	}
	if len(parsed.AccessManifest.Omissions) != 1 {
		t.Fatalf("expected 1 omission, got %d", len(parsed.AccessManifest.Omissions))
	}
	got := parsed.AccessManifest.Omissions[0]
	if got.Category != "composite score" {
		t.Errorf("category = %q, want %q", got.Category, "composite score")
	}
	if got.Description != "No single score or grade is derived from the metrics." {
		t.Errorf("description = %q, want %q", got.Description, "No single score or grade is derived from the metrics.")
	}
	if got.PrivacyRationaleRef != "docs/privacy-rationale.md#no-composite-score" {
		t.Errorf("privacyRationaleRef = %q, want %q", got.PrivacyRationaleRef, "docs/privacy-rationale.md#no-composite-score")
	}
}

// TestOmissionsJSONShape verifies the JSON serialization shape of omissions.
func TestOmissionsJSONShape(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	r := report.Build(activityFixture(), nil, nil, now)
	r.AccessManifest = &report.AccessManifest{
		Endpoints: []report.EndpointCount{
			{Class: "rest:users_show", Count: 1},
		},
		NeverRequested: []string{"file contents"},
		Notes:          "test notes",
		Omissions: []report.OmissionEntry{
			{
				Category:            "composite score",
				Description:         "No single score or grade is derived from the metrics.",
				PrivacyRationaleRef: "docs/privacy-rationale.md#no-composite-score",
			},
		},
	}

	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Check the omission fields appear at the expected JSON paths.
	var doc struct {
		AccessManifest struct {
			Omissions []struct {
				Category            string `json:"category"`
				Description         string `json:"description"`
				PrivacyRationaleRef string `json:"privacyRationaleRef"`
			} `json:"omissions"`
		} `json:"access_manifest"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(doc.AccessManifest.Omissions) != 1 {
		t.Fatalf("expected 1 omission, got %d", len(doc.AccessManifest.Omissions))
	}
	o := doc.AccessManifest.Omissions[0]
	if o.Category != "composite score" {
		t.Errorf("category = %q, want %q", o.Category, "composite score")
	}
	if o.Description != "No single score or grade is derived from the metrics." {
		t.Errorf("description = %q, want %q", o.Description, "No single score or grade is derived from the metrics.")
	}
	if o.PrivacyRationaleRef != "docs/privacy-rationale.md#no-composite-score" {
		t.Errorf("privacyRationaleRef = %q, want %q", o.PrivacyRationaleRef, "docs/privacy-rationale.md#no-composite-score")
	}
}

// TestOmissionsMissingCategory verifies validation rejects an omission with
// an empty category.
func TestOmissionsMissingCategory(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	r := report.Build(activityFixture(), nil, nil, now)
	r.AccessManifest = &report.AccessManifest{
		Endpoints: []report.EndpointCount{
			{Class: "rest:users_show", Count: 1},
		},
		NeverRequested: []string{"file contents"},
		Notes:          "test notes",
		Omissions: []report.OmissionEntry{
			{
				Category:            "",
				Description:         "No single score or grade is derived from the metrics.",
				PrivacyRationaleRef: "docs/privacy-rationale.md#no-composite-score",
			},
		},
	}

	err := r.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want error for missing category")
	}
	if !strings.Contains(err.Error(), "category") {
		t.Errorf("error %q does not mention 'category'", err)
	}
}

// TestOmissionsMissingDescription verifies validation rejects an omission with
// an empty description.
func TestOmissionsMissingDescription(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	r := report.Build(activityFixture(), nil, nil, now)
	r.AccessManifest = &report.AccessManifest{
		Endpoints: []report.EndpointCount{
			{Class: "rest:users_show", Count: 1},
		},
		NeverRequested: []string{"file contents"},
		Notes:          "test notes",
		Omissions: []report.OmissionEntry{
			{
				Category:            "composite score",
				Description:         "",
				PrivacyRationaleRef: "docs/privacy-rationale.md#no-composite-score",
			},
		},
	}

	err := r.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want error for missing description")
	}
	if !strings.Contains(err.Error(), "description") {
		t.Errorf("error %q does not mention 'description'", err)
	}
}

// TestOmissionsMissingPrivacyRationaleRef verifies validation rejects an
// omission with an empty privacyRationaleRef.
func TestOmissionsMissingPrivacyRationaleRef(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	r := report.Build(activityFixture(), nil, nil, now)
	r.AccessManifest = &report.AccessManifest{
		Endpoints: []report.EndpointCount{
			{Class: "rest:users_show", Count: 1},
		},
		NeverRequested: []string{"file contents"},
		Notes:          "test notes",
		Omissions: []report.OmissionEntry{
			{
				Category:            "composite score",
				Description:         "No single score or grade is derived from the metrics.",
				PrivacyRationaleRef: "",
			},
		},
	}

	err := r.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want error for missing privacyRationaleRef")
	}
	if !strings.Contains(err.Error(), "privacyRationaleRef") {
		t.Errorf("error %q does not mention 'privacyRationaleRef'", err)
	}
}

// TestOmissionsEmptyPrivacyRationaleRef verifies validation rejects an
// omission where privacyRationaleRef is an empty string (tested via
// JSON that sets it to an empty string explicitly).
func TestOmissionsEmptyPrivacyRationaleRef(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	r := report.Build(activityFixture(), nil, nil, now)
	r.AccessManifest = &report.AccessManifest{
		Endpoints: []report.EndpointCount{
			{Class: "rest:users_show", Count: 1},
		},
		NeverRequested: []string{"file contents"},
		Notes:          "test notes",
		Omissions: []report.OmissionEntry{
			{
				Category:            "composite score",
				Description:         "No single score or grade is derived from the metrics.",
				PrivacyRationaleRef: "",
			},
		},
	}

	// Also verify that Parse rejects it via JSON marshal/unmarshal.
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if _, err := report.Parse(raw); err == nil {
		t.Error("Parse accepted omission with empty privacyRationaleRef")
	}
}

// TestOmissionsNilManifestAcceptsNilOmissions verifies that a report with
// nil AccessManifest still passes validation (backward compat).
func TestOmissionsNilManifestAcceptsNilOmissions(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	r := report.Build(activityFixture(), nil, nil, now)
	r.AccessManifest = nil

	if err := r.Validate(); err != nil {
		t.Fatalf("Validate with nil AccessManifest: %v", err)
	}
}

// TestOmissionsMultipleEntryOrder verifies that omission entries preserve
// their order through a JSON round-trip.
func TestOmissionsMultipleEntryOrder(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	r := report.Build(activityFixture(), nil, nil, now)
	r.AccessManifest = &report.AccessManifest{
		Endpoints: []report.EndpointCount{
			{Class: "rest:users_show", Count: 1},
		},
		NeverRequested: []string{"file contents"},
		Notes:          "test notes",
		Omissions: []report.OmissionEntry{
			{
				Category:            "composite score",
				Description:         "No single score or grade is derived from the metrics.",
				PrivacyRationaleRef: "docs/privacy-rationale.md#no-composite-score",
			},
			{
				Category:            "comparison",
				Description:         "No comparison against a named colleague or team.",
				PrivacyRationaleRef: "docs/privacy-rationale.md#no-comparison",
			},
		},
	}

	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	parsed, err := report.Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if len(parsed.AccessManifest.Omissions) != 2 {
		t.Fatalf("expected 2 omissions, got %d", len(parsed.AccessManifest.Omissions))
	}
	if parsed.AccessManifest.Omissions[0].Category != "composite score" {
		t.Errorf("first omission category = %q, want %q", parsed.AccessManifest.Omissions[0].Category, "composite score")
	}
	if parsed.AccessManifest.Omissions[1].Category != "comparison" {
		t.Errorf("second omission category = %q, want %q", parsed.AccessManifest.Omissions[1].Category, "comparison")
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

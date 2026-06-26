package report_test

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/gkanitz/coderepute/provider"
	"github.com/gkanitz/coderepute/report"
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

func TestCadenceJSONShape(t *testing.T) {
	cadence := &report.Cadence{
		ActiveDays:    4,
		Contributions: 7,
		Trend: []report.TrendBucket{
			{
				Start:  time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
				End:    time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC),
				Counts: map[string]int{"pull_requests": 2},
			},
		},
	}
	now := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)

	r := report.Build(activityFixture(), nil, cadence, now)
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var doc struct {
		Cadence struct {
			ActiveDays    int `json:"active_days"`
			Contributions int `json:"contributions"`
			Trend         []struct {
				Start  time.Time      `json:"start"`
				End    time.Time      `json:"end"`
				Counts map[string]int `json:"counts"`
			} `json:"trend"`
		} `json:"cadence"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if doc.Cadence.ActiveDays != 4 || doc.Cadence.Contributions != 7 {
		t.Errorf("cadence = %+v, want active_days=4 contributions=7", doc.Cadence)
	}
	if len(doc.Cadence.Trend) != 1 {
		t.Fatalf("trend has %d buckets, want 1", len(doc.Cadence.Trend))
	}
	b := doc.Cadence.Trend[0]
	if !b.Start.Equal(cadence.Trend[0].Start) || !b.End.Equal(cadence.Trend[0].End) || b.Counts["pull_requests"] != 2 {
		t.Errorf("trend bucket = %+v, want %+v", b, cadence.Trend[0])
	}
}

// TestBuildAllTimeWindow verifies that a zero provider.Window.Since is
// preserved as a nil report.Window.Since ("all time / no lower bound").
func TestBuildAllTimeWindow(t *testing.T) {
	as := activityFixture()
	as.Window.Since = time.Time{} // zero = all-time

	r := report.Build(as, nil, nil, time.Now())

	if r.Coverage.Window.Since != nil {
		t.Errorf("all-time window: Coverage.Window.Since = %v, want nil", r.Coverage.Window.Since)
	}
	if err := r.Validate(); err != nil {
		t.Fatalf("all-time window report failed validation: %v", err)
	}

	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// The "since" field must be absent (omitempty) in the JSON output.
	var doc map[string]json.RawMessage
	json.Unmarshal(raw, &doc)
	var coverage map[string]json.RawMessage
	json.Unmarshal(doc["coverage"], &coverage)
	var window map[string]json.RawMessage
	json.Unmarshal(coverage["window"], &window)
	if _, ok := window["since"]; ok {
		t.Errorf("all-time window JSON must omit the 'since' field, got: %s", window["since"])
	}

	// Round-trip via Parse must succeed and preserve nil Since.
	parsed, err := report.Parse(raw)
	if err != nil {
		t.Fatalf("parse all-time window report: %v", err)
	}
	if parsed.Coverage.Window.Since != nil {
		t.Errorf("round-trip lost nil Since: got %v", parsed.Coverage.Window.Since)
	}
}

// TestBuildBoundedWindow verifies that a non-zero provider.Window.Since
// is preserved as a non-nil report.Window.Since (bounded window).
func TestBuildBoundedWindow(t *testing.T) {
	r := report.Build(activityFixture(), nil, nil, time.Now())

	if r.Coverage.Window.Since == nil {
		t.Fatal("bounded window: Coverage.Window.Since must not be nil")
	}
	want := activityFixture().Window.Since
	if !r.Coverage.Window.Since.Equal(want) {
		t.Errorf("Coverage.Window.Since = %v, want %v", r.Coverage.Window.Since, want)
	}
}

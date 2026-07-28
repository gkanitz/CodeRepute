package report_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gkanitz/coderepute/provider"
	"github.com/gkanitz/coderepute/report"
)

// TestAICollaborationJSONShape verifies that the ai_collaboration field
// serializes correctly in the report JSON.
func TestAICollaborationJSONShape(t *testing.T) {
	collab := &report.Collaboration{
		AICollaboration: &report.AICollaborationStats{
			Total:            8,
			DeepReviewCount:  5,
			DeepReviewShare:  0.625,
			RecognizedAgents: []string{"bot", "copilot"},
		},
	}
	now := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	r := report.Build(activityFixture(), collab, nil, now)
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var doc struct {
		Collaboration struct {
			AICollaboration struct {
				Total           int     `json:"total"`
				DeepReviewCount int     `json:"deep_review_count"`
				DeepReviewShare float64 `json:"deep_review_share"`
			} `json:"ai_collaboration"`
		} `json:"collaboration"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if doc.Collaboration.AICollaboration.Total != 8 {
		t.Errorf("total = %d, want 8", doc.Collaboration.AICollaboration.Total)
	}
	if doc.Collaboration.AICollaboration.DeepReviewCount != 5 {
		t.Errorf("deep_review_count = %d, want 5", doc.Collaboration.AICollaboration.DeepReviewCount)
	}
	if doc.Collaboration.AICollaboration.DeepReviewShare != 0.625 {
		t.Errorf("deep_review_share = %f, want 0.625", doc.Collaboration.AICollaboration.DeepReviewShare)
	}
}

// TestAICollaborationJSONRoundTrip verifies that a report with AI
// collaboration data round-trips through JSON marshal, Parse, and Validate.
func TestAICollaborationJSONRoundTrip(t *testing.T) {
	collab := &report.Collaboration{
		AICollaboration: &report.AICollaborationStats{
			Total:            8,
			DeepReviewCount:  5,
			DeepReviewShare:  0.625,
			RecognizedAgents: []string{"bot", "copilot"},
		},
	}
	now := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	as := activityFixture()
	as.AccessManifest = provider.Manifest{
		Endpoints: []provider.EndpointCount{
			{Class: "rest:users_show", Count: 1},
		},
		NeverRequested:       []string{"file contents"},
		Notes:                "test manifest",
		AIRecognitionVersion: 1,
	}

	r := report.Build(as, collab, nil, now)

	if r.AccessManifest == nil {
		t.Fatal("Build() produced nil AccessManifest")
	}

	// Verify propagation to manifest
	if len(r.AccessManifest.AIRecognizedAgents) != 2 {
		t.Errorf("AIRecognizedAgents = %v, want 2 entries", r.AccessManifest.AIRecognizedAgents)
	}
	if r.AccessManifest.Signal1AbsenceDisclosure == "" {
		t.Error("Signal1AbsenceDisclosure is empty")
	}
	if r.AccessManifest.AIRecognitionVersion != 1 {
		t.Errorf("AIRecognitionVersion = %d, want 1", r.AccessManifest.AIRecognitionVersion)
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

	if parsed.Collaboration == nil {
		t.Fatal("round-trip lost Collaboration")
	}
	if parsed.Collaboration.AICollaboration == nil {
		t.Fatal("round-trip lost AICollaboration")
	}
	if parsed.Collaboration.AICollaboration.Total != 8 {
		t.Errorf("round-trip Total = %d, want 8", parsed.Collaboration.AICollaboration.Total)
	}
	if parsed.AccessManifest == nil {
		t.Fatal("round-trip lost AccessManifest")
	}
	if len(parsed.AccessManifest.AIRecognizedAgents) != 2 {
		t.Errorf("round-trip AIRecognizedAgents = %v, want 2", parsed.AccessManifest.AIRecognizedAgents)
	}
	if parsed.AccessManifest.Signal1AbsenceDisclosure == "" {
		t.Error("round-trip lost Signal1AbsenceDisclosure")
	}
	if parsed.AccessManifest.AIRecognitionVersion != 1 {
		t.Errorf("round-trip AIRecognitionVersion = %d, want 1", parsed.AccessManifest.AIRecognitionVersion)
	}
}

// TestAICollaborationZeroStateSerializes verifies that the
// ai_collaboration field with zero values still serializes (non-nil pointer
// means the block appears in JSON with zero values).
func TestAICollaborationZeroStateSerializes(t *testing.T) {
	collab := &report.Collaboration{
		AICollaboration: &report.AICollaborationStats{
			Total:           0,
			DeepReviewCount: 0,
			DeepReviewShare: 0,
		},
	}
	now := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	as := activityFixture()
	as.AccessManifest = provider.Manifest{
		Endpoints: []provider.EndpointCount{
			{Class: "rest:users_show", Count: 1},
		},
		NeverRequested: []string{"file contents"},
		Notes:          "test manifest",
	}

	r := report.Build(as, collab, nil, now)
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if !strings.Contains(string(raw), "ai_collaboration") {
		t.Error("JSON output should contain ai_collaboration field even with zero values")
	}
	if !strings.Contains(string(raw), `"total":0`) && !strings.Contains(string(raw), `"total": 0`) {
		t.Error("JSON output should contain total: 0")
	}
}

// TestManifestRecognizedAgentsPropagated verifies that recognized agents
// from AICollaboration are propagated into the AccessManifest.
func TestManifestRecognizedAgentsPropagated(t *testing.T) {
	now := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	as := activityFixture()
	as.AccessManifest = provider.Manifest{
		Endpoints: []provider.EndpointCount{
			{Class: "rest:users_show", Count: 1},
		},
		NeverRequested: []string{"file contents"},
		Notes:          "test manifest",
	}

	collab := &report.Collaboration{
		AICollaboration: &report.AICollaborationStats{
			Total:            3,
			DeepReviewCount:  2,
			DeepReviewShare:  0.667,
			RecognizedAgents: []string{"copilot", "devin", "bot"},
		},
	}

	r := report.Build(as, collab, nil, now)

	if r.AccessManifest == nil {
		t.Fatal("Build() produced nil AccessManifest")
	}
	if len(r.AccessManifest.AIRecognizedAgents) != 3 {
		t.Errorf("AIRecognizedAgents = %v, want 3 entries", r.AccessManifest.AIRecognizedAgents)
	}
	expected := []string{"copilot", "devin", "bot"} // alphabetically sorted by metric
	for i, a := range r.AccessManifest.AIRecognizedAgents {
		if a != expected[i] {
			t.Errorf("AIRecognizedAgents[%d] = %q, want %q", i, a, expected[i])
		}
	}
}

// TestManifestNoRecognizedAgentsWhenNoAICollab verifies that when there is
// no AI collaboration data, the recognized agents list is not set.
func TestManifestNoRecognizedAgentsWhenNoAICollab(t *testing.T) {
	now := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	as := activityFixture()
	as.AccessManifest = provider.Manifest{
		Endpoints: []provider.EndpointCount{
			{Class: "rest:users_show", Count: 1},
		},
		NeverRequested: []string{"file contents"},
		Notes:          "test manifest",
	}

	r := report.Build(as, nil, nil, now)

	if r.AccessManifest == nil {
		t.Fatal("Build() produced nil AccessManifest")
	}
	if len(r.AccessManifest.AIRecognizedAgents) != 0 {
		t.Errorf("AIRecognizedAgents = %v, want empty", r.AccessManifest.AIRecognizedAgents)
	}
	if r.AccessManifest.Signal1AbsenceDisclosure != "" {
		t.Errorf("Signal1AbsenceDisclosure = %q, want empty when no AI collab", r.AccessManifest.Signal1AbsenceDisclosure)
	}
}

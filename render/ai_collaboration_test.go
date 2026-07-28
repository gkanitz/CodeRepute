package render_test

import (
	"strings"
	"testing"

	"github.com/gkanitz/coderepute/render"
	"github.com/gkanitz/coderepute/report"
)

// TestHTMLEmptyAICollaborationRender verifies that when Collaboration is nil,
// the AI collaboration section is omitted entirely.
func TestHTMLEmptyAICollaborationRender(t *testing.T) {
	r := reportFixture()
	r.Collaboration = nil

	out, err := render.HTML(r)
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	html := string(out)

	if strings.Contains(html, "AI collaboration") {
		t.Error("AI collaboration section rendered despite nil Collaboration")
	}
}

// TestHTMLAICollaborationSectionRenderPopulated verifies that the AI
// collaboration section renders correctly when data is present.
func TestHTMLAICollaborationSectionRenderPopulated(t *testing.T) {
	r := reportFixture()
	r.Collaboration.AICollaboration = &report.AICollaborationStats{
		Total:            8,
		DeepReviewCount:  5,
		DeepReviewShare:  0.625,
		RecognizedAgents: []string{"copilot", "bot"},
	}

	out, err := render.HTML(r)
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	html := string(out)

	// Section renders with title
	if !strings.Contains(html, "AI collaboration") {
		t.Error("rendered HTML missing AI collaboration section title")
	}

	// Shows the aggregate counts
	if !strings.Contains(html, "8") {
		t.Error("rendered HTML missing AI PR count 8")
	}
	if !strings.Contains(html, "5") {
		t.Error("rendered HTML missing deep review count 5")
	}

	// Shows deep review percentage
	if !strings.Contains(html, "63%") { // 0.625 rounds to 63%
		t.Error("rendered HTML missing deep review percentage")
	}

	// Shows the limit copy
	if !strings.Contains(html, "Limit:") {
		t.Error("rendered HTML missing limit section")
	}

	// Shows ruleset version mention
	if !strings.Contains(html, "transparency manifest") {
		t.Error("rendered HTML missing transparency manifest reference")
	}

	// Zero-state text should NOT appear when data is present
	if strings.Contains(html, "No AI/bot-authored PRs reviewed in this window") {
		t.Error("zero-state text appears despite populated data")
	}

	// No per-agent breakdown in the body
	if strings.Contains(html, "copilot") && strings.Contains(html, "Recognized agents") {
		// The agent IDs should only appear in the transparency manifest section
		// Check they don't appear in the AI collaboration section
		collabSection := extractSection(html, "AI collaboration")
		manifestSection := extractSection(html, "What this tool read")
		if collabSection != "" && strings.Contains(collabSection, "copilot") {
			t.Error("copilot agent ID appears in AI collaboration section body - should only be in manifest")
		}
		if manifestSection == "" || !strings.Contains(manifestSection, "copilot") {
			t.Log("Note: agent IDs may appear in transparency manifest, not body")
		}
	}
}

// TestHTMLAICollaborationZeroState verifies that the AI collaboration section
// renders the zero-state text when there are no AI/bot reviews.
func TestHTMLAICollaborationZeroState(t *testing.T) {
	r := reportFixture()
	r.Collaboration.AICollaboration = &report.AICollaborationStats{
		Total:           0,
		DeepReviewCount: 0,
		DeepReviewShare: 0,
	}

	out, err := render.HTML(r)
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	html := string(out)

	// Section renders with title
	if !strings.Contains(html, "AI collaboration") {
		t.Error("rendered HTML missing AI collaboration section title")
	}

	// Shows the zero-state text
	if !strings.Contains(html, "No AI/bot-authored PRs reviewed in this window") {
		t.Error("rendered HTML missing zero-state text")
	}

	// Shows the disclosure text
	if !strings.Contains(html, "CodeRepute does not measure how much AI you personally used") {
		t.Error("rendered HTML missing AI usage disclosure text")
	}

	// Zero-state text in the collaboration section should say the right thing
	if !strings.Contains(html, "No AI/bot-authored PRs reviewed in this window") {
		t.Error("zero-state text missing")
	}
}

// TestHTMLAICollaborationNilDoesNotRender verifies that when AICollaboration
// is nil (the old report format without the field), no section appears.
func TestHTMLAICollaborationNilDoesNotRender(t *testing.T) {
	r := reportFixture()
	r.Collaboration.AICollaboration = nil

	out, err := render.HTML(r)
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	html := string(out)

	if strings.Contains(html, "AI collaboration") {
		t.Error("AI collaboration section rendered despite nil AICollaboration")
	}
}

// TestHTMLAICollaborationNotScoreboard verifies that the AI collaboration
// section does not introduce any composite score, ranking, or named comparison.
func TestHTMLAICollaborationNotScoreboard(t *testing.T) {
	r := reportFixture()
	r.Collaboration.AICollaboration = &report.AICollaborationStats{
		Total:            8,
		DeepReviewCount:  5,
		DeepReviewShare:  0.625,
		RecognizedAgents: []string{"copilot", "bot"},
	}

	out, err := render.HTML(r)
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	html := string(out)

	prohibited := []string{"excellent", "weak", "slow", "fast", "top", "elite", "poor", "expert", "proficient", "skilled", "mastery", "junior", "senior", "staff", "principal"}

	collabSection := extractSection(html, "AI collaboration")
	if collabSection == "" {
		t.Fatal("AI collaboration section not found")
	}

	// Strip HTML tags before checking for prohibited words
	stripped := stripHTMLTags(collabSection)
	lower := strings.ToLower(stripped)
	for _, w := range prohibited {
		if strings.Contains(lower, w) {
			t.Errorf("AI collaboration section contains prohibited term %q", w)
		}
	}
}

// TestTransparencyIncAIDisclosure verifies that the Signal-1-absence
// disclosure text appears in the transparency manifest section.
func TestTransparencyIncAIDisclosure(t *testing.T) {
	r := reportFixture()
	r.AccessManifest = &report.AccessManifest{
		Endpoints: []report.EndpointCount{
			{Class: "rest:users_show", Count: 1},
		},
		NeverRequested:           []string{"file contents"},
		Notes:                    "test",
		Signal1AbsenceDisclosure: "CodeRepute does not measure how much AI you personally used - that would require reading your commit messages, which we attest we never do.",
		AIRecognizedAgents:       []string{"copilot", "bot"},
		AIRecognitionVersion:     1,
	}

	out, err := render.HTML(r)
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	html := string(out)

	// AI recognition section in manifest
	if !strings.Contains(html, "AI recognition") {
		t.Error("rendered HTML missing AI recognition section in manifest")
	}

	// Ruleset version shown
	if !strings.Contains(html, "Ruleset version:") {
		t.Error("rendered HTML missing ruleset version")
	}
	if !strings.Contains(html, "1") {
		t.Error("rendered HTML missing ruleset version number")
	}

	// Recognized agents listed
	if !strings.Contains(html, "Recognized agents in this window:") {
		t.Error("rendered HTML missing recognized agents heading")
	}
	if !strings.Contains(html, "copilot") {
		t.Error("rendered HTML missing copilot in recognized agents")
	}
	if !strings.Contains(html, "bot") {
		t.Error("rendered HTML missing bot in recognized agents")
	}

	// Signal-1-absence disclosure
	if !strings.Contains(html, "What this does not measure") {
		t.Error("rendered HTML missing 'What this does not measure' heading")
	}
	if !strings.Contains(html, "CodeRepute does not measure how much AI you personally used") {
		t.Error("rendered HTML missing Signal-1-absence disclosure text")
	}
}

// TestTransparencyEmptyAICollabDoesNotShowAIIncSection verifies that when
// there is no AI collaboration data, the AI recognition section of the
// manifest is also absent.
func TestTransparencyEmptyAICollabDoesNotShowAIIncSection(t *testing.T) {
	r := reportFixture()
	r.AccessManifest = &report.AccessManifest{
		Endpoints: []report.EndpointCount{
			{Class: "rest:users_show", Count: 1},
		},
		NeverRequested: []string{"file contents"},
		Notes:          "test",
	}

	out, err := render.HTML(r)
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	html := string(out)

	if strings.Contains(html, "AI recognition") {
		t.Error("AI recognition section rendered despite no AI collaboration data")
	}
	if strings.Contains(html, "What this does not measure") {
		t.Error("'What this does not measure' section rendered despite no data")
	}
}

// stripHTMLTags removes all HTML tags from a string, keeping only the text content.
func stripHTMLTags(s string) string {
	var out strings.Builder
	inTag := false
	for _, r := range s {
		if r == '<' {
			inTag = true
		} else if r == '>' {
			inTag = false
		} else if !inTag {
			out.WriteRune(r)
		}
	}
	return out.String()
}

// extractSection extracts the section content between a section heading and
// the next heading at the same level. Simplified version for test assertions.
func extractSection(html, headingText string) string {
	idx := strings.Index(html, headingText)
	if idx < 0 {
		return ""
	}
	// Find the section boundary
	start := strings.LastIndex(html[:idx], "<h")
	if start < 0 {
		start = idx
	}
	// Look for the next <h2 or <h3 at a similar level
	next := strings.Index(html[start+1:], "<h3")
	if next < 0 {
		next = strings.Index(html[start+1:], "<h2")
	}
	if next < 0 {
		return html[start:]
	}
	return html[start : start+next]
}

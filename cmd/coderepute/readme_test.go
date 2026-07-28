package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// readmePath is the path to README.md relative to this test file's working
// directory (cmd/coderepute/).
var readmePath = filepath.Join("..", "..", "README.md")

// extractMarkdownSection returns the text between the first occurrence of
// marker and the start of the next ## top-level heading. If marker is not
// found, the returned content is empty and ok is false. This is a coarse
// capture for content asserts within a subsection.
func extractMarkdownSection(content, marker string) (section string, ok bool) {
	start := strings.Index(content, marker)
	if start < 0 {
		return "", false
	}
	// Look for the next ## heading on its own line (top-level section).
	idx := strings.Index(content[start+len(marker):], "\n## ")
	if idx < 0 {
		// No more sections — capture to end.
		return content[start:], true
	}
	return content[start : start+len(marker)+idx], true
}

func TestReadmeExists(t *testing.T) {
	if _, err := os.Stat(readmePath); err != nil {
		t.Fatalf("README.md not found: %v", err)
	}
}

func TestNoScoreSubsectionHeading(t *testing.T) {
	raw, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	content := string(raw)

	// Must contain a ### subsection heading under ## Why CodeRepute. The
	// exact heading text is placeholder but must be non-empty and relate to
	// the no-score/no-ranking stance.
	if !strings.Contains(content, "### ") {
		t.Error("README.md missing ### subsection heading under ## Why CodeRepute")
	}
}

func TestNoScoreSubsectionOrder(t *testing.T) {
	raw, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	content := string(raw)

	// The ### subsection must appear within the ## Why CodeRepute section
	// (between that heading and the next ## heading).
	sec, ok := extractMarkdownSection(content, "## Why CodeRepute")
	if !ok {
		t.Fatal("## Why CodeRepute section not found")
	}
	if !strings.Contains(sec, "\n### ") {
		t.Error("## Why CodeRepute section does not contain a ### subsection heading")
	}
}

func TestReadmeDoraContrastCopy(t *testing.T) {
	raw, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	content := string(raw)

	// Locate the no-score subsection for scoped checks.
	sec, ok := extractMarkdownSection(content, "### ")
	if !ok {
		t.Fatal("no-score subsection not found — cannot check DORA-contrast copy")
	}

	// Must contrast CodeRepute's no-score, no-ranking, no-within-team-
	// comparison design against the harms of misapplied DORA / individual
	// productivity metrics — without naming a specific competing tool.
	contrastKeywords := []string{
		"score",
		"rank",
		"comparison",
		"DORA",
	}
	for _, kw := range contrastKeywords {
		if !strings.Contains(sec, kw) {
			t.Errorf("no-score subsection missing keyword %q", kw)
		}
	}

	// Must contain the 66% figure attributed to the JetBrains survey.
	if !strings.Contains(sec, "66%") {
		t.Error("no-score subsection missing required 66% figure")
	}
	if !strings.Contains(sec, "JetBrains State of Developer Ecosystem 2025") {
		t.Error("no-score subsection missing attribution to JetBrains State of Developer Ecosystem 2025")
	}
	if !strings.Contains(sec, "24,534") {
		t.Error("no-score subsection missing survey respondent count (24,534)")
	}

	// The attribution must be a markdown hyperlink to the byteiota.com URL.
	wantHref := "https://byteiota.com/developer-productivity-metrics-crisis-66-dont-trust-dora"
	if !strings.Contains(content, wantHref) {
		t.Errorf("README.md missing hyperlink to %s", wantHref)
	}

	// No competing tool names within the subsection.
	for _, name := range []string{"LinearB", "Pluralsight", "GitPrime", "Waydev", "Allstacks", "HaydenAI", "Stepsize", "CodeClimate"} {
		if strings.Contains(sec, name) {
			t.Errorf("no-score subsection contains competing tool name %q — forbidden", name)
		}
	}

	// No composite score, ranking, grade, or named-colleague comparison in
	// the subsection.
	sectionLower := strings.ToLower(sec)
	for _, p := range []string{
		"composite score", "ranking", "grade",
	} {
		if strings.Contains(sectionLower, p) {
			t.Errorf("no-score subsection contains prohibited wording %q", p)
		}
	}
}

func TestReadmeAISubsection(t *testing.T) {
	raw, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	content := string(raw)

	sec, ok := extractMarkdownSection(content, "## Why CodeRepute")
	if !ok {
		t.Fatal("## Why CodeRepute section not found")
	}

	// Must contain a ### subsection for AI-era stance (sibling to the
	// existing "Measurement you can trust, not a scoreboard" subsection).
	if !strings.Contains(sec, "### Built for the AI era") {
		t.Error("## Why CodeRepute section missing ### AI subsection heading")
	}

	subsectionLower := strings.ToLower(sec)

	// Condensed version of the four points:
	// 1. Thesis: AI era, judgment
	if !strings.Contains(subsectionLower, "ai") {
		t.Error("AI subsection missing AI reference")
	}
	if !strings.Contains(subsectionLower, "judgment") {
		t.Error("AI subsection missing judgment reference")
	}

	// 2. What it measures
	if !strings.Contains(subsectionLower, "review") {
		t.Error("AI subsection missing review measurement reference")
	}

	// 3. What it refuses to do — no commit message reading, no AI inference.
	if !strings.Contains(subsectionLower, "commit message") {
		t.Error("AI subsection missing commit message refusal")
	}

	// 4. Attestation value claim
	if !strings.Contains(subsectionLower, "attest") {
		t.Error("AI subsection missing attestation value claim")
	}

	// No composite score, ranking, AI-nativeness grade.
	for _, p := range []string{
		"composite score", "ranking", "grade", "ai-nativeness",
	} {
		if strings.Contains(subsectionLower, p) {
			t.Errorf("AI subsection contains prohibited wording %q", p)
		}
	}

	// No AI vendor/agent names in the copy.
	vendorNames := []string{
		"ChatGPT", "Claude", "Copilot", "Codex", "Devin", "Cursor",
	}
	for _, name := range vendorNames {
		if strings.Contains(content, name) {
			t.Errorf("README.md contains vendor/agent name %q — forbidden", name)
		}
	}
}

func TestReadmeAITableRow(t *testing.T) {
	raw, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	content := string(raw)

	if !strings.Contains(content, "AI/bot PRs reviewed") {
		t.Error("README.md missing AI/bot PRs reviewed table row")
	}
	if !strings.Contains(content, "deep-review share") {
		t.Error("README.md missing deep-review share in table row")
	}
}

// TestReadmeHeadingsPreserved verifies that all pre-existing ## and ###
// headings from README.md are still present with identical text after the
// new subsection is added.
func TestReadmeHeadingsPreserved(t *testing.T) {
	raw, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	content := string(raw)

	// All pre-existing top-level headings (excluding the section we added
	// content to) and subheadings must still be present.
	expectedHeadings := []string{
		"# CodeRepute",
		"## Why CodeRepute",
		"### Measurement you can trust, not a scoreboard",
		"### Built for the AI era: measuring judgment, not output",
		"## What the report measures",
		"## Who uses it",
		"## Quick start",
		"### Install",
		"### Run locally",
		"## Run in CI with attestation",
		"### GitHub Actions",
		"### GitLab CI",
		"## Automated org-wide reports",
		"### Step 1 — Define your team",
		"### Step 2 — The workflow",
		"### Distribution options at a glance",
		"## Verifying a report",
		"## Platform support",
		"## Report output",
		"## Which file do I share?",
		"## Feedback",
		"## License",
	}
	for _, h := range expectedHeadings {
		if !strings.Contains(content, h) {
			t.Errorf("expected heading missing from README.md: %q", h)
		}
	}

	// No new ## heading beyond those listed.
	// Check by counting ## headings.
	if strings.Count(content, "\n## ") != 12 {
		t.Errorf("expected exactly 12 ## headings in README.md, got %d", strings.Count(content, "\n## "))
	}
}

func TestReadmeLinkCheck(t *testing.T) {
	raw, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	content := string(raw)

	wantHref := "https://byteiota.com/developer-productivity-metrics-crisis-66-dont-trust-dora"
	if !strings.Contains(content, wantHref) {
		t.Fatalf("hyperlink %q not found in README.md — cannot verify", wantHref)
	}

	req, err := http.NewRequest(http.MethodGet, wantHref, nil)
	if err != nil {
		t.Fatalf("creating link-check request: %v", err)
	}
	req.Header.Set("User-Agent", "CodeRepute-link-check/1.0")

	client := &http.Client{Timeout: 10_000_000_000} // 10s
	resp, err := client.Do(req)
	if err != nil {
		t.Logf("link-check GET %q skipped (network): %v", wantHref, err)
		t.Skip("link-check skipped: network request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("link-check GET %q returned HTTP %d, want 200", wantHref, resp.StatusCode)
	}
}

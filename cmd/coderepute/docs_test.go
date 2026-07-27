package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// docsIndex is the path to docs/index.html relative to this test file's
// working directory (cmd/coderepute/).
var docsIndex = filepath.Join("..", "..", "docs", "index.html")

// trustSectionHead is the exact heading text of the new trust section.
const trustSectionHead = "Measurement you can trust, not a scoreboard"

// extractSection returns the text between the first occurrence of marker
// and the start of the next <section> tag. If marker is not found, the
// returned content is empty and ok is false.
func extractSection(content, marker string) (section string, ok bool) {
	start := strings.Index(content, marker)
	if start < 0 {
		return "", false
	}
	// Find the next <section> or </section> boundary. We look for the next
	// instance of <section that begins a new block. This gives us a coarse
	// capture of the section's body text.
	next := strings.Index(content[start+1:], "<section")
	if next < 0 {
		// No more sections — capture to end.
		return content[start:], true
	}
	return content[start : start+next], true
}

func TestDocsIndexExists(t *testing.T) {
	if _, err := os.Stat(docsIndex); err != nil {
		t.Fatalf("docs/index.html not found: %v", err)
	}
}

func TestTrustSectionHeading(t *testing.T) {
	raw, err := os.ReadFile(docsIndex)
	if err != nil {
		t.Fatalf("read docs/index.html: %v", err)
	}
	content := string(raw)

	if !strings.Contains(content, trustSectionHead) {
		t.Errorf("docs/index.html missing required heading: %q", trustSectionHead)
	}
}

func TestTrustSectionOrder(t *testing.T) {
	raw, err := os.ReadFile(docsIndex)
	if err != nil {
		t.Fatalf("read docs/index.html: %v", err)
	}
	content := string(raw)

	assertDocOrder(t, content,
		"What the report measures",
		trustSectionHead,
		"Add to CI in one YAML block",
	)
}

func TestDoraContrastCopy(t *testing.T) {
	raw, err := os.ReadFile(docsIndex)
	if err != nil {
		t.Fatalf("read docs/index.html: %v", err)
	}
	content := string(raw)

	// Extract just the new trust section for scoped checks.
	sec, ok := extractSection(content, trustSectionHead)
	if !ok {
		t.Fatal("trust section not found — cannot check DORA-contrast copy")
	}

	// Must contain the 66% figure attributed to the JetBrains survey.
	if !strings.Contains(sec, "66%") {
		t.Error("trust section missing required 66% figure")
	}
	if !strings.Contains(sec, "JetBrains State of Developer Ecosystem 2025") {
		t.Error("trust section missing attribution to JetBrains State of Developer Ecosystem 2025")
	}
	if !strings.Contains(sec, "24,534") {
		t.Error("trust section missing survey respondent count (24,534)")
	}

	// The attribution must be wrapped in an <a href="..."> pointing to the
	// byteiota.com URL.
	wantHref := "https://byteiota.com/developer-productivity-metrics-crisis-66-dont-trust-dora"
	if !strings.Contains(content, wantHref) {
		t.Errorf("docs/index.html missing hyperlink to %s", wantHref)
	}

	// No competing tool names within the section.
	for _, name := range []string{"LinearB", "Pluralsight", "GitPrime", "Waydev", "Allstacks", "HaydenAI", "Stepsize", "CodeClimate"} {
		if strings.Contains(sec, name) {
			t.Errorf("trust section contains competing tool name %q — forbidden", name)
		}
	}

	// No composite score, ranking, grade, or named-colleague comparison in
	// the new section.
	sectionLower := strings.ToLower(sec)
	for _, p := range []string{
		"composite score", "ranking", "grade",
	} {
		if strings.Contains(sectionLower, p) {
			t.Errorf("trust section contains prohibited wording %q", p)
		}
	}
}

func TestPrSizeBandsExplanation(t *testing.T) {
	raw, err := os.ReadFile(docsIndex)
	if err != nil {
		t.Fatalf("read docs/index.html: %v", err)
	}
	content := string(raw)

	sec, ok := extractSection(content, trustSectionHead)
	if !ok {
		t.Fatal("trust section not found — cannot check PR-size-bands explanation")
	}
	secLower := strings.ToLower(sec)

	// Must contain PR-size-band explanation with relevant keywords.
	required := []string{
		"size band",
		"review comment",
		"normalised",
		"fewer comment",
	}
	for _, r := range required {
		if !strings.Contains(secLower, r) {
			t.Errorf("trust section missing PR-size-band explanation phrase: %q", r)
		}
	}

	// Must NOT contain source-code diffs, file paths, or colleague names.
	forbidden := []string{
		"diff --git", "--- a/", "+++ b/",
	}
	for _, f := range forbidden {
		if strings.Contains(secLower, f) {
			t.Errorf("trust section contains forbidden phrase: %q", f)
		}
	}
}

func TestPreExistingSectionsComplete(t *testing.T) {
	raw, err := os.ReadFile(docsIndex)
	if err != nil {
		t.Fatalf("read docs/index.html: %v", err)
	}
	content := string(raw)

	// All pre-existing top-level section headings must still be present and
	// in the correct order relative to the new section.
	expectedHeadings := []string{
		trustSectionHead,
		"What the report measures",
		"Add to CI in one YAML block",
		"Who uses it",
		"Received a report?",
		"Have feedback?",
	}
	// Verify all exist.
	for _, h := range expectedHeadings {
		if !strings.Contains(content, h) {
			t.Errorf("expected heading missing from docs/index.html: %q", h)
		}
	}

	// Verify document order: new section after metrics, before quickstart.
	assertDocOrder(t, content,
		"What the report measures",
		trustSectionHead,
		"Add to CI in one YAML block",
	)
}

func TestLinkCheck(t *testing.T) {
	raw, err := os.ReadFile(docsIndex)
	if err != nil {
		t.Fatalf("read docs/index.html: %v", err)
	}
	content := string(raw)

	wantHref := "https://byteiota.com/developer-productivity-metrics-crisis-66-dont-trust-dora"
	if !strings.Contains(content, wantHref) {
		t.Fatalf("hyperlink %q not found in page — cannot verify", wantHref)
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

// assertDocOrder asserts that the three substrings appear in that relative
// order within content.
func assertDocOrder(t *testing.T, content, first, second, third string) {
	t.Helper()
	i1 := strings.Index(content, first)
	i2 := strings.Index(content, second)
	i3 := strings.Index(content, third)

	if i1 < 0 {
		t.Errorf("expected string %q not found in document", first)
	}
	if i2 < 0 {
		t.Errorf("expected string %q not found in document", second)
	}
	if i3 < 0 {
		t.Errorf("expected string %q not found in document", third)
	}
	if i1 < 0 || i2 < 0 || i3 < 0 {
		return
	}

	if !(i1 < i2 && i2 < i3) {
		t.Errorf("document order wrong: expected %q before %q before %q, got positions %d, %d, %d",
			first, second, third, i1, i2, i3)
	}
}

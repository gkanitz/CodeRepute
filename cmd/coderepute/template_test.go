package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Templates are stored in .github/ISSUE_TEMPLATE/ relative to the repo root.
// Test files run from cmd/coderepute/, so we walk up two directories.
var templateDir = func() string {
	// Resolve from the test's working directory (package dir) to repo root.
	return filepath.Join("..", "..", ".github", "ISSUE_TEMPLATE")
}()

func TestBugReportTemplate(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(templateDir, "bug_report.yml"))
	if err != nil {
		t.Fatalf("bug_report.yml not found: %v", err)
	}
	content := string(raw)

	// Must contain required fields
	checks := []string{
		"name: Bug report",
		"description:",
		"labels: [bug]",
		"body:",
		"id: platform",
		"id: run-mode",
		"id: version",
		"id: what-happened",
		"id: expected",
		"id: reproduction",
		"id: context",
	}
	for _, c := range checks {
		if !strings.Contains(content, c) {
			t.Errorf("bug_report.yml missing required content: %q", c)
		}
	}

	// Privacy reminder must be present — the issue spec requires it.
	if !strings.Contains(content, "privacy") && !strings.Contains(content, "Privacy") {
		t.Error("bug_report.yml must contain a privacy reminder about not pasting private repo names")
	}

	// Must mention not to paste private repo names
	if !strings.Contains(content, "private") || !strings.Contains(content, "repository") {
		t.Error("bug_report.yml privacy reminder must mention private repository names")
	}
}

func TestFeedbackTemplateStructure(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(templateDir, "feedback.yml"))
	if err != nil {
		t.Fatalf("feedback.yml not found: %v", err)
	}
	content := string(raw)

	// Header checks
	checks := []string{
		"name: Feedback",
		"description:",
		"labels: [feedback]",
		"body:",
	}
	for _, c := range checks {
		if !strings.Contains(content, c) {
			t.Errorf("feedback.yml missing required content: %q", c)
		}
	}

	// Must have the audience dropdown with three options
	audienceChecks := []string{
		"id: audience",
		"developer who ran CodeRepute",
		"hiring manager",
		"recruiter",
		"org admin",
	}
	for _, c := range audienceChecks {
		if !strings.Contains(content, c) {
			t.Errorf("feedback.yml missing audience option: %q", c)
		}
	}

	// The issue spec requires three questions per audience:
	//   "what convinced you, what confused you, what's missing"
	// Verify each audience gets the three questions.
	for _, suffix := range []string{"convinced", "confused", "missing"} {
		for _, prefix := range []string{"developer-", "manager-", "admin-"} {
			field := "id: " + prefix + suffix
			if !strings.Contains(content, field) {
				t.Errorf("feedback.yml missing field %q (per-audience %s question)", field, suffix)
			}
		}
	}

	// Must mention no telemetry
	if !strings.Contains(content, "no telemetry") && !strings.Contains(content, "No telemetry") {
		t.Error("feedback.yml must state no telemetry stance")
	}
}

func TestConfigTemplate(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(templateDir, "config.yml"))
	if err != nil {
		t.Fatalf("config.yml not found: %v", err)
	}
	content := string(raw)

	// Blank issues must be disabled
	if !strings.Contains(content, "blank_issues_enabled: false") {
		t.Error("config.yml must set blank_issues_enabled: false")
	}

	// Must have a contact link pointing at discussions
	if !strings.Contains(content, "contact_links:") {
		t.Error("config.yml must define contact_links")
	}
	if !strings.Contains(content, "Launch feedback") {
		t.Error("config.yml contact link must mention launch feedback")
	}
	if !strings.Contains(content, "github.com/gkanitz/CodeRepute/discussions") {
		t.Error("config.yml contact link must point to GitHub Discussions")
	}
}

func TestNoTemplatesWithoutPrivacyReminder(t *testing.T) {
	// Every template that asks for reproduction steps or report contents
	// must carry a privacy reminder. Currently only bug_report.yml does
	// that — verify it's there.
	raw, err := os.ReadFile(filepath.Join(templateDir, "bug_report.yml"))
	if err != nil {
		t.Fatalf("bug_report.yml not found: %v", err)
	}
	content := string(raw)

	// The reminder text must be in a markdown block at the top
	if !strings.Contains(content, "Privacy reminder") {
		t.Error("bug_report.yml must start with a Privacy reminder in its markdown block")
	}
}

func TestNoTelemetryAbsentFromAllTemplates(t *testing.T) {
	// Confirm no telemetry/analytics/tracking is introduced in any template.
	// The feedback template is allowed to SAY "no telemetry" or "no analytics"
	// — that's stating the project stance. We check only for concrete
	// tracking implementation patterns, not the words used to describe
	// the project's privacy guarantees.
	telemetryPatterns := []string{
		"tracking pixel",
		"phone-home",
		"beacon",
		"gtag",
		"gtm.",
		"google-analytics",
		"segment.",
		"amplitude.",
	}

	dir, err := os.Open(templateDir)
	if err != nil {
		t.Fatalf("cannot open template directory: %v", err)
	}
	defer dir.Close()

	entries, err := dir.Readdir(-1)
	if err != nil {
		t.Fatalf("cannot read template directory: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yml") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(templateDir, entry.Name()))
		if err != nil {
			t.Errorf("cannot read %s: %v", entry.Name(), err)
			continue
		}
		content := string(raw)
		for _, p := range telemetryPatterns {
			if strings.Contains(content, p) {
				t.Errorf("%s contains telemetry/analytics pattern %q — this is forbidden", entry.Name(), p)
			}
		}
	}
}

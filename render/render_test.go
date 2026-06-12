package render_test

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/grkanitz/coderepute/render"
	"github.com/grkanitz/coderepute/report"
)

func reportFixture() report.Report {
	return report.Report{
		SchemaVersion: report.SchemaVersion,
		GeneratedAt:   time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC),
		Subject: report.Subject{
			Platform:  "github",
			Username:  "octocat",
			AccountID: "583231",
		},
		Coverage: &report.Coverage{
			Repos: []string{"acme/widgets"},
			Window: report.Window{
				Since: time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
				Until: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
			},
			TokenScope: "repo",
		},
		Verification: &report.Verification{
			Status: report.StatusUnverified,
			Reason: "local run; no CI attestation",
		},
		Collaboration: &report.Collaboration{
			PullRequests: &report.PullRequestStats{Authored: 3, Merged: 2},
		},
	}
}

func TestHTMLContainsReportFacts(t *testing.T) {
	out, err := render.HTML(reportFixture())
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	html := string(out)

	for _, want := range []string{
		"octocat",
		"583231",
		"github",
		"acme/widgets",
		"repo", // token scope
		"unverified",
		"2025-06-01",
		"2026-06-01",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered HTML missing %q", want)
		}
	}

	counts := regexp.MustCompile(`>\s*3\s*<`)
	if !counts.MatchString(html) {
		t.Errorf("rendered HTML missing authored count 3")
	}
	if !regexp.MustCompile(`>\s*2\s*<`).MatchString(html) {
		t.Errorf("rendered HTML missing merged count 2")
	}
}

func TestHTMLIsSelfContained(t *testing.T) {
	out, err := render.HTML(reportFixture())
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	html := string(out)

	if !strings.HasPrefix(strings.TrimSpace(html), "<!DOCTYPE html>") {
		t.Error("output is not a full HTML document")
	}
	for _, external := range []string{"<script src=", `<link rel="stylesheet"`, "src=\"http", "href=\"http"} {
		if strings.Contains(html, external) {
			t.Errorf("output references external resource: found %q", external)
		}
	}
}

func TestHTMLOmitsAbsentSections(t *testing.T) {
	r := reportFixture()
	r.Collaboration = nil

	out, err := render.HTML(r)
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	if strings.Contains(strings.ToLower(string(out)), "pull requests") {
		t.Error("collaboration section rendered despite nil data")
	}
}

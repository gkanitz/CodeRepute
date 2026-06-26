package render_test

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gkanitz/coderepute/render"
	"github.com/gkanitz/coderepute/report"
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
				Since: func() *time.Time { t := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC); return &t }(),
				Until: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
			},
			TokenScope: "repo",
		},
		Verification: &report.Verification{
			Status: report.StatusUnverified,
			Reason: "local run; no CI attestation",
		},
		Collaboration: &report.Collaboration{
			PullRequests:      &report.PullRequestStats{Authored: 3, Merged: 2},
			ReviewsGiven:      &report.ReviewStats{Total: 5, Approvals: 4, ChangesRequested: 1},
			ReviewComments:    &report.ReviewCommentStats{Written: 7, Received: 4},
			TimeToMerge:       &report.DurationStats{Count: 2, MedianHours: 30.5},
			TimeToFirstReview: &report.DurationStats{Count: 2, MedianHours: 6.25},
			Rework:            &report.ReworkStats{ReviewedPRs: 2, ReworkedPRs: 1, Share: 0.5},
		},
	}
}

func TestHTMLRendersCollaborationMetrics(t *testing.T) {
	out, err := render.HTML(reportFixture())
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	html := string(out)

	for _, want := range []string{
		"Reviews given",
		"Review comments",
		"Time to merge",
		"Time to first review",
		"Rework",
		"30.5", // median hours to merge
		"6.3",  // median hours to first review, rounded to one decimal
		"50%",  // rework share
	} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered HTML missing %q", want)
		}
	}
}

func TestHTMLOmitsAbsentCollaborationStats(t *testing.T) {
	r := reportFixture()
	r.Collaboration.TimeToMerge = nil
	r.Collaboration.TimeToFirstReview = nil
	r.Collaboration.Rework = nil

	out, err := render.HTML(r)
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	html := string(out)
	for _, absent := range []string{"Time to merge", "Time to first review", "Rework"} {
		if strings.Contains(html, absent) {
			t.Errorf("rendered HTML shows %q despite omitted stat", absent)
		}
	}
	if !strings.Contains(html, "Reviews given") {
		t.Error("present stats should still render")
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
		"org acme", // coverage stamp names the org, not individual repos
		"repo",     // token scope
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
	// href="http" is intentionally excluded: <a href> navigation links are fine
	// in a self-contained document since they don't load external resources.
	// <link rel="stylesheet" href=...> is already covered by the link check below.
	for _, external := range []string{"<script src=", `<link rel="stylesheet"`, "src=\"http"} {
		if strings.Contains(html, external) {
			t.Errorf("output references external resource: found %q", external)
		}
	}
}

func TestHTMLCadenceIsSubordinateContext(t *testing.T) {
	r := reportFixture()
	r.Cadence = &report.Cadence{
		ActiveDays:    42,
		Contributions: 87,
		Trend: []report.TrendBucket{
			{
				Start:  time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
				End:    time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC),
				Counts: map[string]int{"pull_requests": 2, "reviews_given": 1},
			},
			{
				Start:  time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC),
				End:    time.Date(2025, 8, 1, 0, 0, 0, 0, time.UTC),
				Counts: map[string]int{},
			},
		},
	}

	out, err := render.HTML(r)
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	html := string(out)

	for _, want := range []string{"42", "87", "2025-06-01", "2025-07-01"} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered HTML missing cadence fact %q", want)
		}
	}

	// Cadence is context, never headline: it renders after the
	// collaboration section, inside a context-styled section, and its
	// numbers never get the headline stat treatment.
	collab := strings.Index(html, "Pull requests")
	cadence := strings.Index(html, `<section class="context">`)
	if collab == -1 || cadence == -1 || cadence < collab {
		t.Fatalf("cadence must render as a context section after collaboration (collab at %d, cadence at %d)", collab, cadence)
	}
	section := html[cadence:]
	if end := strings.Index(section, "</section>"); end != -1 {
		section = section[:end]
	}
	if strings.Contains(section, `class="stat"`) || strings.Contains(section, `class="n"`) {
		t.Error("cadence section uses headline stat styling; it must stay subordinate")
	}

	// No composite or aggregate score anywhere in the rendering: the only
	// allowed uses of the word are disclaimers that none exists.
	stripped := strings.ReplaceAll(strings.ToLower(html), "not a quality score", "")
	stripped = strings.ReplaceAll(stripped, "no score", "")
	if strings.Contains(stripped, "score") {
		t.Error("rendered HTML presents a score")
	}
}

func TestHTMLShowsVerifiedIdentity(t *testing.T) {
	r := reportFixture()
	r.Verification = &report.Verification{
		Status:      report.StatusVerified,
		Provider:    "github-actions",
		Repository:  "acme/widgets",
		WorkflowRef: "acme/widgets/.github/workflows/report.yml@refs/heads/main",
		RunID:       "9000000001",
		RunURL:      "https://github.com/acme/widgets/actions/runs/9000000001",
		Attestation: &report.Attestation{
			Type:          report.AttestationTypeSigstore,
			URL:           "https://github.com/acme/widgets/attestations",
			VerifyCommand: "gh attestation verify report.json --repo acme/widgets",
		},
	}

	out, err := render.HTML(r)
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	html := string(out)
	for _, want := range []string{
		"verified",
		"acme/widgets/.github/workflows/report.yml@refs/heads/main",
		"https://github.com/acme/widgets/actions/runs/9000000001",
		"gh attestation verify report.json --repo acme/widgets",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered HTML missing %q", want)
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

func TestHTMLKPIStripRendersAllSixCards(t *testing.T) {
	r := reportFixture()
	// Add cadence so the Active days and all six KPI cards render.
	r.Cadence = &report.Cadence{ActiveDays: 131, Contributions: 200}

	out, err := render.HTML(r)
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	html := string(out)

	for _, want := range []string{
		"kpi-strip",
		"Active days",
		"PRs merged",
		"Reviews given",
		"Deep review %",
		"Review comments",
		"Median TTM",
		"131",  // active days
		"2",    // PRs merged from fixture
		"5",    // reviews given from fixture
		"30.5", // TTM formatted in KPI
	} {
		if !strings.Contains(html, want) {
			t.Errorf("KPI strip missing %q", want)
		}
	}
}

func TestHTMLSVGChartsAreEmbedded(t *testing.T) {
	r := reportFixture()
	r.Cadence = &report.Cadence{
		ActiveDays:    90,
		Contributions: 150,
		Trend: []report.TrendBucket{
			{
				Start:  time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
				End:    time.Date(2025, 9, 1, 0, 0, 0, 0, time.UTC),
				Counts: map[string]int{"pull_requests": 10, "reviews_given": 15},
			},
			{
				Start:  time.Date(2025, 9, 1, 0, 0, 0, 0, time.UTC),
				End:    time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC),
				Counts: map[string]int{"pull_requests": 8, "reviews_given": 12},
			},
		},
	}

	out, err := render.HTML(r)
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	html := string(out)

	// All three SVG charts should be embedded.
	svgCount := strings.Count(html, "<svg ")
	if svgCount < 3 {
		t.Errorf("expected at least 3 inline SVG elements, got %d", svgCount)
	}
	// No external chart library references.
	for _, ext := range []string{"chart.js", "d3.js", "plotly", "echarts", "highcharts"} {
		if strings.Contains(strings.ToLower(html), ext) {
			t.Errorf("report uses external chart library %q", ext)
		}
	}
	// SVG should contain the chart elements.
	if !strings.Contains(html, "<polyline") {
		t.Error("dual-line chart missing polyline element")
	}
	if !strings.Contains(html, `fill="#0EA5E9"`) {
		t.Error("charts missing teal accent colour")
	}
}

func TestHTMLInterpretationCopyPresent(t *testing.T) {
	r := reportFixture()
	r.Cadence = &report.Cadence{ActiveDays: 42, Contributions: 87}

	out, err := render.HTML(r)
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	html := string(out)

	// The collaboration section should have interpretation copy class.
	if !strings.Contains(html, `class="interpretation"`) {
		t.Error("rendered HTML missing interpretation sections")
	}
}

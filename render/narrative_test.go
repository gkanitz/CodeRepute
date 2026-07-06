package render_test

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gkanitz/coderepute/render"
	"github.com/gkanitz/coderepute/report"
)

// Fixture helpers

// narrativeFixture returns a report with collaboration data suitable for
// narrative testing. Defaults: authored=10, reviews=20 (r=2.0, review-weighted).
func narrativeFixture() report.Report {
	r := reportFixture()
	r.Collaboration.PullRequests = &report.PullRequestStats{Authored: 10, Merged: 8}
	r.Collaboration.ReviewsGiven = &report.ReviewStats{Total: 20, Approvals: 15, ChangesRequested: 5, DeepReviewCount: 8}
	r.Collaboration.ReviewComments = &report.ReviewCommentStats{Written: 30, Received: 15}
	r.Collaboration.TimeToMerge = &report.DurationStats{Count: 8, MedianHours: 12.5}
	r.Collaboration.TimeToFirstReview = &report.DurationStats{Count: 7, MedianHours: 4.0}
	r.Collaboration.Rework = &report.ReworkStats{ReviewedPRs: 8, ReworkedPRs: 2, Share: 0.25}
	return r
}

// authoredReport returns a report with auth=20, reviews=10 (r=0.5, authoring-weighted).
func authoredReport() report.Report {
	r := narrativeFixture()
	r.Collaboration.PullRequests = &report.PullRequestStats{Authored: 20, Merged: 16}
	r.Collaboration.ReviewsGiven = &report.ReviewStats{Total: 10, Approvals: 8, ChangesRequested: 2, DeepReviewCount: 3}
	return r
}

// balancedReport returns a report with auth=10, reviews=12 (r=1.2, balanced).
func balancedReport() report.Report {
	r := narrativeFixture()
	r.Collaboration.PullRequests = &report.PullRequestStats{Authored: 10, Merged: 9}
	r.Collaboration.ReviewsGiven = &report.ReviewStats{Total: 12, Approvals: 9, ChangesRequested: 3, DeepReviewCount: 4}
	return r
}

// noAuthoredReport returns a report with authored=0 (profile-mix skipped).
func noAuthoredReport() report.Report {
	r := narrativeFixture()
	r.Collaboration.PullRequests = &report.PullRequestStats{Authored: 0, Merged: 0}
	r.Collaboration.ReviewsGiven = &report.ReviewStats{Total: 5, Approvals: 4, ChangesRequested: 1, DeepReviewCount: 1}
	return r
}

// trajectoryReport returns a fixture with trend data that fires the trajectory
// rule. 4 buckets: first half r~=1.0, second half r~=3.0, shift ~+2.0.
func trajectoryReport() report.Report {
	r := narrativeFixture()
	r.Cadence = &report.Cadence{
		ActiveDays:    100,
		Contributions: 150,
		Trend: []report.TrendBucket{
			{Start: time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC), End: time.Date(2025, 9, 1, 0, 0, 0, 0, time.UTC),
				Counts: map[string]int{"pull_requests": 10, "reviews_given": 10}},
			{Start: time.Date(2025, 9, 1, 0, 0, 0, 0, time.UTC), End: time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC),
				Counts: map[string]int{"pull_requests": 10, "reviews_given": 10}},
			{Start: time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
				Counts: map[string]int{"pull_requests": 5, "reviews_given": 15}},
			{Start: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
				Counts: map[string]int{"pull_requests": 5, "reviews_given": 15}},
		},
	}
	return r
}

// decliningTrajectoryReport returns a fixture where trajectory shifts toward
// authoring (shift < -0.30). First half r~=3.0, second half r~=1.0.
func decliningTrajectoryReport() report.Report {
	r := narrativeFixture()
	r.Cadence = &report.Cadence{
		ActiveDays:    100,
		Contributions: 150,
		Trend: []report.TrendBucket{
			{Start: time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC), End: time.Date(2025, 9, 1, 0, 0, 0, 0, time.UTC),
				Counts: map[string]int{"pull_requests": 5, "reviews_given": 15}},
			{Start: time.Date(2025, 9, 1, 0, 0, 0, 0, time.UTC), End: time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC),
				Counts: map[string]int{"pull_requests": 5, "reviews_given": 15}},
			{Start: time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
				Counts: map[string]int{"pull_requests": 10, "reviews_given": 10}},
			{Start: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
				Counts: map[string]int{"pull_requests": 10, "reviews_given": 10}},
		},
	}
	return r
}

// subThresholdTrajectoryReport has a shift of +0.10 (below 0.30, silent).
func subThresholdTrajectoryReport() report.Report {
	r := narrativeFixture()
	r.Cadence = &report.Cadence{
		ActiveDays:    100,
		Contributions: 150,
		Trend: []report.TrendBucket{
			{Start: time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC), End: time.Date(2025, 9, 1, 0, 0, 0, 0, time.UTC),
				Counts: map[string]int{"pull_requests": 10, "reviews_given": 10}},
			{Start: time.Date(2025, 9, 1, 0, 0, 0, 0, time.UTC), End: time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC),
				Counts: map[string]int{"pull_requests": 10, "reviews_given": 10}},
			{Start: time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
				Counts: map[string]int{"pull_requests": 10, "reviews_given": 11}},
			{Start: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
				Counts: map[string]int{"pull_requests": 10, "reviews_given": 11}},
		},
	}
	return r
}

// sparseTrajectoryReport has < 10 combined events in one half (silent).
func sparseTrajectoryReport() report.Report {
	r := narrativeFixture()
	r.Cadence = &report.Cadence{
		ActiveDays:    10,
		Contributions: 15,
		Trend: []report.TrendBucket{
			{Start: time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC), End: time.Date(2025, 9, 1, 0, 0, 0, 0, time.UTC),
				Counts: map[string]int{"pull_requests": 2, "reviews_given": 1}},
			{Start: time.Date(2025, 9, 1, 0, 0, 0, 0, time.UTC), End: time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC),
				Counts: map[string]int{"pull_requests": 20, "reviews_given": 25}},
		},
	}
	return r
}

// fullNarrativeReport returns a report with all data for narrative testing.
func fullNarrativeReport() report.Report {
	return report.Report{
		SchemaVersion: report.SchemaVersion,
		GeneratedAt:   time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC),
		Subject: report.Subject{
			Platform:  "github",
			Username:  "alice",
			AccountID: "1234567",
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
			Reason: "local run",
		},
		Collaboration: &report.Collaboration{
			PullRequests:      &report.PullRequestStats{Authored: 47, Merged: 42},
			ReviewsGiven:      &report.ReviewStats{Total: 63, Approvals: 51, ChangesRequested: 12, DeepReviewCount: 19},
			ReviewComments:    &report.ReviewCommentStats{Written: 124, Received: 89},
			TimeToMerge:       &report.DurationStats{Count: 42, MedianHours: 18.5},
			TimeToFirstReview: &report.DurationStats{Count: 40, MedianHours: 5.2},
			Rework:            &report.ReworkStats{ReviewedPRs: 42, ReworkedPRs: 11, Share: 0.262},
			PRSize:            &report.PRSizeStats{Count: 42, MedianLines: 256, FilesMedian: 8, SmallShare: 0.762, SmallThresholdLines: 400},
			LanguageMix: &report.LanguageMixStats{
				Basis:      "merged_pr_diff_lines",
				PRCount:    42,
				TotalLines: 28400,
				Languages:  []report.LangShare{{Name: "Go", SharePct: 42}, {Name: "TypeScript", SharePct: 28}},
				OtherShare: 30,
			},
		},
		Cadence: &report.Cadence{
			ActiveDays:    198,
			Contributions: 371,
			Trend: []report.TrendBucket{
				{Start: time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC), End: time.Date(2025, 9, 1, 0, 0, 0, 0, time.UTC),
					Counts: map[string]int{"pull_requests": 12, "reviews_given": 17}},
				{Start: time.Date(2025, 9, 1, 0, 0, 0, 0, time.UTC), End: time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC),
					Counts: map[string]int{"pull_requests": 15, "reviews_given": 21}},
				{Start: time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
					Counts: map[string]int{"pull_requests": 11, "reviews_given": 15}},
				{Start: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
					Counts: map[string]int{"pull_requests": 9, "reviews_given": 10}},
			},
		},
	}
}

// minimalReport returns a report with only mandatory fields.
func minimalReport() report.Report {
	return report.Report{
		SchemaVersion: report.SchemaVersion,
		GeneratedAt:   time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC),
		Subject: report.Subject{
			Platform:  "github",
			Username:  "minimal",
			AccountID: "9999999",
		},
		Coverage: &report.Coverage{
			Repos: []string{"acme/minimal"},
			Window: report.Window{
				Since: func() *time.Time { t := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC); return &t }(),
				Until: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
			},
			TokenScope: "repo",
		},
		Verification: &report.Verification{
			Status: report.StatusUnverified,
			Reason: "local run",
		},
	}
}

// AC 1: Rule-function unit tests

func TestProfileMixRuleReviewWeighted(t *testing.T) {
	r := narrativeFixture() // authored=10, reviews=20 -> r=2.0 > 1.5
	html, err := render.HTML(r)
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	body := string(html)
	if !strings.Contains(body, "review-weighted") {
		t.Error("review-weighted fixture should produce 'review-weighted' phrasing")
	}
	if !strings.Contains(body, "20 reviews") {
		t.Error("review-weighted sentence should cite 20 reviews")
	}
	if !strings.Contains(body, "10 authored") {
		t.Error("review-weighted sentence should cite 10 authored")
	}
}

func TestProfileMixRuleAuthoringWeighted(t *testing.T) {
	r := authoredReport() // authored=20, reviews=10 -> r=0.5 < 0.67
	html, err := render.HTML(r)
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	body := string(html)
	if !strings.Contains(body, "authoring-weighted") {
		t.Error("authoring-weighted fixture should produce 'authoring-weighted' phrasing")
	}
	if !strings.Contains(body, "20 pull requests") {
		t.Error("authoring-weighted sentence should cite 20 authored")
	}
	if !strings.Contains(body, "10 reviews") {
		t.Error("authoring-weighted sentence should cite 10 reviews")
	}
}

func TestProfileMixRuleBalanced(t *testing.T) {
	r := balancedReport() // authored=10, reviews=12 -> r=1.2
	html, err := render.HTML(r)
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	body := string(html)
	if !strings.Contains(body, "balanced") {
		t.Error("balanced fixture should produce 'balanced' phrasing")
	}
	if !strings.Contains(body, "10 pull requests") {
		t.Error("balanced sentence should cite 10 authored")
	}
	if !strings.Contains(body, "12 reviews") {
		t.Error("balanced sentence should cite 12 reviews")
	}
}

func TestProfileMixRuleSkippedWhenAuthoredZero(t *testing.T) {
	r := noAuthoredReport() // authored=0 -> skip
	html, err := render.HTML(r)
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	body := string(html)
	if strings.Contains(body, "review-weighted") || strings.Contains(body, "authoring-weighted") || strings.Contains(body, "balanced") {
		t.Error("authored=0 report should not include any profile-mix sentence")
	}
	if strings.Contains(body, "profile-mix") {
		t.Error("authored=0 report should not list profile-mix in annex")
	}
}

func TestTrajectoryRuleFiresAboveThreshold(t *testing.T) {
	r := trajectoryReport() // shift ~ +2.0
	html, err := render.HTML(r)
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	body := string(html)
	if !strings.Contains(body, "shift toward reviewing") {
		t.Error("trajectory +2.0 should fire 'shift toward reviewing'")
	}
}

func TestTrajectoryRuleDecliningFires(t *testing.T) {
	r := decliningTrajectoryReport() // shift ~ -2.0
	html, err := render.HTML(r)
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	body := string(html)
	if !strings.Contains(body, "shift toward authoring") {
		t.Error("trajectory -2.0 should fire 'shift toward authoring'")
	}
}

func TestTrajectoryRuleSilentBelowThreshold(t *testing.T) {
	r := subThresholdTrajectoryReport() // shift ~ +0.29
	html, err := render.HTML(r)
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	body := string(html)
	if strings.Contains(body, "shift toward") {
		t.Error("shift +0.29 should not fire trajectory sentence")
	}
}

func TestTrajectoryRuleSilentWithSparseHalves(t *testing.T) {
	r := sparseTrajectoryReport() // first half has 3 events
	html, err := render.HTML(r)
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	body := string(html)
	if strings.Contains(body, "shift toward") {
		t.Error("sparse halves should not fire trajectory sentence")
	}
}

// AC 2: Golden narratives for fixture permutations

func TestNarrativeRenderReviewWeighted(t *testing.T) {
	r := narrativeFixture() // review-weighted, no trajectory
	html, err := render.HTML(r)
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	body := string(html)
	if !strings.Contains(body, "review-weighted") {
		t.Error("must contain review-weighted sentence")
	}
	if strings.Contains(body, "shift toward") {
		t.Error("fixture has no trend data; must not have trajectory sentence")
	}
	if !strings.Contains(body, "deep") && !strings.Contains(body, "deep reviews") {
		t.Error("expected deep review probe")
	}
	for _, placeholder := range []string{"N/A", "n/a", "unknown", "{}", "[]"} {
		if strings.Contains(body, placeholder) {
			t.Errorf("found placeholder text %q", placeholder)
		}
	}
}

func TestNarrativeRenderAuthoringWeighted(t *testing.T) {
	r := authoredReport() // authoring-weighted
	html, err := render.HTML(r)
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	body := string(html)
	if !strings.Contains(body, "authoring-weighted") {
		t.Error("must contain authoring-weighted sentence")
	}
}

func TestNarrativeRenderBalanced(t *testing.T) {
	r := balancedReport() // balanced
	html, err := render.HTML(r)
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	body := string(html)
	if !strings.Contains(body, "balanced") {
		t.Error("must contain balanced sentence")
	}
}

func TestNarrativeRenderWithTrajectory(t *testing.T) {
	r := trajectoryReport() // review-weighted + trajectory
	html, err := render.HTML(r)
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	body := string(html)
	if !strings.Contains(body, "review-weighted") {
		t.Error("must contain profile-mix sentence")
	}
	if !strings.Contains(body, "shift toward reviewing") {
		t.Error("must contain trajectory sentence")
	}
	if !strings.Contains(body, "profile-mix") {
		t.Error("annex must list profile-mix rule")
	}
	if !strings.Contains(body, "trajectory") {
		t.Error("annex must list trajectory rule")
	}
}

func TestNarrativeRenderWithoutTrajectory(t *testing.T) {
	r := balancedReport() // balanced, no cadence data
	html, err := render.HTML(r)
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	body := string(html)
	if !strings.Contains(body, "balanced") {
		t.Error("must contain profile-mix sentence")
	}
	if strings.Contains(body, "shift toward") {
		t.Error("must not have trajectory sentence without cadence data")
	}
	if strings.Contains(body, "trajectory") {
		t.Error("annex must not list trajectory rule when it did not fire")
	}
}

func TestNarrativeRenderMinimalSections(t *testing.T) {
	r := minimalReport()
	html, err := render.HTML(r)
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	body := string(html)
	if strings.Contains(body, "Narrative") {
		if strings.Contains(body, "review-weighted") || strings.Contains(body, "authoring-weighted") ||
			strings.Contains(body, "balanced") || strings.Contains(body, "shift toward") {
			t.Error("minimal report narrative must not contain any narrative sentences")
		}
	}
	for _, placeholder := range []string{"N/A", "n/a", "unknown", "{}", "[]"} {
		if strings.Contains(body, placeholder) {
			t.Errorf("found placeholder text %q", placeholder)
		}
	}
}

// AC 3: Interviewer kit tests

func TestInterviewerKitFullReportThreeProbes(t *testing.T) {
	r := fullNarrativeReport()
	html, err := render.HTML(r)
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	body := string(html)
	if !strings.Contains(body, "Interviewer kit") {
		t.Error("must render Interviewer kit heading")
	}
	liCount := strings.Count(body, "<li")
	if liCount < 3 {
		t.Errorf("expected at least 3 probe <li> elements, got %d", liCount)
	}
	if !strings.Contains(body, "deep reviews") {
		t.Error("expected deep review probe")
	}
	if !strings.Contains(body, "rework cycle") {
		t.Error("expected rework probe")
	}
	if !strings.Contains(body, "time to merge") {
		t.Error("expected time-to-merge probe")
	}
}

func TestInterviewerKitMissingReworkAndLanguagesFallsBack(t *testing.T) {
	r := fullNarrativeReport()
	r.Collaboration.Rework = nil
	r.Collaboration.LanguageMix = nil
	r.Cadence.Trend = nil

	html, err := render.HTML(r)
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	body := string(html)

	liCount := strings.Count(body, "<li")
	if liCount < 2 {
		t.Errorf("expected at least 2 probes with rework missing, got %d", liCount)
	}
}

func TestInterviewerKitMinimalReportSparse(t *testing.T) {
	r := minimalReport()
	html, err := render.HTML(r)
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	body := string(html)
	if strings.Contains(body, "Interviewer kit") {
		t.Error("minimal report must not render Interviewer kit")
	}
	if strings.Contains(body, "probe-") {
		t.Error("minimal report must not contain probe rule IDs")
	}
}

// AC 4: Numeral extraction test

func TestNarrativeNumeralsMatchReportJSON(t *testing.T) {
	r := fullNarrativeReport()
	html, err := render.HTML(r)
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	body := string(html)

	narrStart := strings.Index(body, "Narrative")
	if narrStart < 0 {
		t.Skip("no Narrative section rendered; skipping numeral extraction")
	}
	narrEnd := strings.Index(body[narrStart:], "</section>")
	if narrEnd < 0 {
		narrEnd = len(body) - narrStart
	}
	narrText := body[narrStart : narrStart+narrEnd]

	// Strip inline style attributes before extracting numbers — CSS values
	// like "1.25rem" are not narrative data.
	styleRe := regexp.MustCompile(`style="[^"]*"`)
	narrText = styleRe.ReplaceAllString(narrText, "")

	numRe := regexp.MustCompile(`\b(\d+(?:\.\d+)?)`)
	matches := numRe.FindAllString(narrText, -1)

	if len(matches) == 0 {
		t.Fatal("narrative text contains no numerals — the extraction test cannot run")
	}

	rawJSON, _ := json.Marshal(r)
	rawStr := string(rawJSON)
	for _, num := range matches {
		if !strings.Contains(rawStr, num) {
			t.Errorf("narrative numeral %q not found in report JSON", num)
		}
	}
}

// AC 5: Derivation annex tests

func TestDerivationAnnexListsOnlyFiredRules(t *testing.T) {
	r := balancedReport() // profile-mix fires, trajectory does not
	html, err := render.HTML(r)
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	body := string(html)

	if !strings.Contains(body, "profile-mix") {
		t.Error("annex must list profile-mix rule")
	}
	if strings.Contains(body, "trajectory") {
		t.Error("annex must not list trajectory rule (no cadence data)")
	}
	codeRe := regexp.MustCompile(`<code>([^<]+)</code>`)
	codeMatches := codeRe.FindAllStringSubmatch(body, -1)
	for _, m := range codeMatches {
		ruleID := m[1]
		if !strings.HasPrefix(ruleID, "probe-") && ruleID != "profile-mix" && ruleID != "trajectory" {
			t.Errorf("unexpected rule ID in annex: %s", ruleID)
		}
	}
}

func TestDerivationAnnexWithTrajectory(t *testing.T) {
	r := trajectoryReport()
	html, err := render.HTML(r)
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	body := string(html)

	if !strings.Contains(body, "profile-mix") {
		t.Error("annex must list profile-mix")
	}
	if !strings.Contains(body, "trajectory") {
		t.Error("annex must list trajectory")
	}
}

func TestDerivationAnnexEmptyForMinimalReport(t *testing.T) {
	r := minimalReport()
	html, err := render.HTML(r)
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	body := string(html)

	if strings.Contains(body, "How this narrative was derived") {
		t.Error("minimal report must not render derivation annex")
	}
}

// AC 6: Wordlist test (no grading adjectives or seniority labels).
// Only the narrative section text is checked — other report sections
// use their own vocabulary and are tested separately.

func extractNarrativeText(html string) string {
	narrStart := strings.Index(html, "Narrative")
	if narrStart < 0 {
		return ""
	}
	narrEnd := strings.Index(html[narrStart:], "</section>")
	if narrEnd < 0 {
		return ""
	}
	text := html[narrStart : narrStart+narrEnd]
	// Strip HTML tags for text-only checking.
	tagRe := regexp.MustCompile(`<[^>]*>`)
	return tagRe.ReplaceAllString(text, " ")
}

func TestNarrativeWordlistNoGradingAdjectives(t *testing.T) {
	r := fullNarrativeReport()
	html, err := render.HTML(r)
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	body := string(html)
	narrText := extractNarrativeText(body)

	prohibited := []string{
		"excellent", "strong", "weak", "slow", "fast", "top", "elite", "poor",
		"junior", "senior", "staff", "principal",
	}
	lower := strings.ToLower(narrText)
	for _, w := range prohibited {
		if strings.Contains(lower, w) {
			t.Errorf("narrative contains prohibited word %q", w)
		}
	}
}

// AC 7: Prohibited-strings golden test

func TestNarrativeProhibitedStrings(t *testing.T) {
	r := fullNarrativeReport()
	html, err := render.HTML(r)
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	body := string(html)

	prohibited := []string{
		"mallory-reviewer",
		"trent-teammate",
		"rocket telemetry",
		"Megacorp",
		"feature/rocket",
	}
	lower := strings.ToLower(body)
	for _, p := range prohibited {
		if strings.Contains(lower, strings.ToLower(p)) {
			t.Errorf("rendered HTML leaks prohibited data %q", p)
		}
	}
}

// AC 8: Minimal report renders without errors

func TestNarrativeRenderMinimalReportNoErrors(t *testing.T) {
	r := minimalReport()
	_, err := render.HTML(r)
	if err != nil {
		t.Fatalf("HTML must not error on minimal report: %v", err)
	}
}

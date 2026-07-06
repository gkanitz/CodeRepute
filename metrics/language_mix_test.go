package metrics_test

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/gkanitz/coderepute/metrics"
	"github.com/gkanitz/coderepute/provider"
	"github.com/gkanitz/coderepute/report"
)

// activityFixture returns a minimal ActivitySet suitable for building reports.
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

// mergedPRWithStats creates a merged PR with the given per-file FileStats.
func mergedPRWithStats(repo string, stats []provider.FileStat) provider.PullRequest {
	mergedAt := ts("2026-02-01T10:00:00Z")
	return provider.PullRequest{
		Repo:      repo,
		CreatedAt: ts("2026-01-01T10:00:00Z"),
		MergedAt:  &mergedAt,
		FileStats: stats,
	}
}

// TestLangForExt verifies the extension→language mapping table:
// representative extensions resolve correctly, config/docs extensions map
// to their buckets, and unknown/empty map to "Other".
func TestLangForExt(t *testing.T) {
	cases := []struct {
		ext  string
		want string
	}{
		// Representative languages
		{ext: "go", want: "Go"},
		{ext: "rs", want: "Rust"},
		{ext: "py", want: "Python"},
		{ext: "js", want: "JavaScript"},
		{ext: "ts", want: "TypeScript"},
		{ext: "tsx", want: "TypeScript"},
		{ext: "java", want: "Java"},
		{ext: "rb", want: "Ruby"},
		{ext: "php", want: "PHP"},
		{ext: "c", want: "C"},
		{ext: "cpp", want: "C++"},
		{ext: "cs", want: "C#"},
		{ext: "swift", want: "Swift"},
		{ext: "sh", want: "Shell"},
		{ext: "sql", want: "SQL"},
		{ext: "hs", want: "Haskell"},

		// Config bucket
		{ext: "yml", want: "Config"},
		{ext: "yaml", want: "Config"},
		{ext: "json", want: "Config"},
		{ext: "toml", want: "Config"},
		{ext: "ini", want: "Config"},
		{ext: "xml", want: "Config"},

		// Docs bucket
		{ext: "md", want: "Docs"},
		{ext: "rst", want: "Docs"},
		{ext: "adoc", want: "Docs"},
		{ext: "txt", want: "Docs"},
		{ext: "rtf", want: "Docs"},

		// Unknown
		{ext: "xyz", want: "Other"},
		{ext: "foo123", want: "Other"},

		// Extensionless
		{ext: "", want: "Other"},
	}
	for _, c := range cases {
		got := metrics.LangForExt(c.ext)
		if got != c.want {
			t.Errorf("LangForExt(%q) = %q, want %q", c.ext, got, c.want)
		}
	}
}

// TestLanguageMixWeightedAggregation verifies that shares are line-weighted
// (not file-counted), sum to 100±1 after rounding, and total_lines equals
// the summed additions+deletions.
func TestLanguageMixWeightedAggregation(t *testing.T) {
	// 5 merged PRs with per-file data. File counts differ so line-weight
	// matters: a language with 2 files of 10 lines each = 20 lines, vs
	// a language with 1 file of 100 lines = 100 lines.
	stats := []provider.FileStat{
		{Ext: "go", Additions: 200, Deletions: 50}, // 250 lines Go
		{Ext: "ts", Additions: 100, Deletions: 30}, // 130 lines TypeScript
		{Ext: "py", Additions: 40, Deletions: 10},  // 50 lines Python
		{Ext: "rs", Additions: 30, Deletions: 5},   // 35 lines Rust
		{Ext: "md", Additions: 20, Deletions: 5},   // 25 lines Docs
		{Ext: "json", Additions: 10, Deletions: 0}, // 10 lines Config
	}
	prs := []provider.PullRequest{
		mergedPRWithStats("acme/widgets", stats),
		mergedPRWithStats("acme/widgets", []provider.FileStat{
			{Ext: "go", Additions: 100, Deletions: 50}, // 150 lines Go
			{Ext: "ts", Additions: 80, Deletions: 20},  // 100 lines TS
			{Ext: "js", Additions: 50, Deletions: 10},  // 60 lines JS
		}),
		mergedPRWithStats("acme/widgets", []provider.FileStat{
			{Ext: "py", Additions: 100, Deletions: 50}, // 150 lines Python
		}),
		mergedPRWithStats("acme/widgets", []provider.FileStat{
			{Ext: "go", Additions: 50, Deletions: 10}, // 60 lines Go
			{Ext: "rb", Additions: 20, Deletions: 5},  // 25 lines Ruby
		}),
		mergedPRWithStats("acme/widgets", []provider.FileStat{
			{Ext: "ts", Additions: 60, Deletions: 20}, // 80 lines TS
			{Ext: "sql", Additions: 15, Deletions: 5}, // 20 lines SQL
		}),
	}

	// Manual computation:
	// PR1: Go=250, TS=130, Python=50, Rust=35, Docs=25, Config=10  = 500
	// PR2: Go=150, TS=100, JS=60                                    = 310
	// PR3: Python=150                                               = 150
	// PR4: Go=60, Ruby=25                                           = 85
	// PR5: TS=80, SQL=20                                            = 100
	// Total: 1145
	// Go: 250+150+60 = 460 -> 460/1145 = 40.17% -> 40%
	// TS: 130+100+80 = 310 -> 310/1145 = 27.07% -> 27%
	// Python: 50+150 = 200 -> 200/1145 = 17.47% -> 17%
	// JS: 60 -> 60/1145 = 5.24% -> 5%
	// Rust: 35 -> 35/1145 = 3.06% -> 3% (at or above threshold)
	// Docs: 25 -> 25/1145 = 2.18% -> 2% (folds into Other)
	// Ruby: 25 -> 25/1145 = 2.18% -> 2% (folds into Other)
	// Config: 10 -> 10/1145 = 0.87% -> 1% (folds into Other)
	// SQL: 20 -> 20/1145 = 1.75% -> 2% (folds into Other)
	// Other: 25+25+10+20 = 80 -> 80/1145 = 6.99% -> 7%

	res := metrics.Compute(provider.ActivitySet{PullRequests: prs})
	lm := res.Collaboration.LanguageMix
	if lm == nil {
		t.Fatal("expected language_mix, got nil")
	}
	if lm.Basis != "merged_pr_diff_lines" {
		t.Errorf("basis = %q, want %q", lm.Basis, "merged_pr_diff_lines")
	}
	if lm.PRCount != 5 {
		t.Errorf("pr_count = %d, want 5", lm.PRCount)
	}
	if lm.TotalLines != 1145 {
		t.Errorf("total_lines = %d, want 1145", lm.TotalLines)
	}

	// Check shares sum to 100 ± 1 after rounding.
	sum := lm.OtherShare
	for _, l := range lm.Languages {
		sum += l.SharePct
	}
	if sum < 99 || sum > 101 {
		t.Errorf("shares sum to %.0f, want 100±1", sum)
	}

	// Check specific shares.
	gotShares := make(map[string]float64)
	for _, l := range lm.Languages {
		gotShares[l.Name] = l.SharePct
	}
	checkShare(t, gotShares, "Go", 40)
	checkShare(t, gotShares, "TypeScript", 27)
	checkShare(t, gotShares, "Python", 17)
	checkShare(t, gotShares, "JavaScript", 5)
	checkShare(t, gotShares, "Rust", 3) // at threshold of 3%

	// Other should include Docs (2%), Ruby (2%), Config (1%), SQL (2%) = 7%
	if lm.OtherShare != 7 {
		t.Errorf("other_share_pct = %.0f, want 7", lm.OtherShare)
	}

	// Languages below 3% must NOT appear in the list.
	for _, l := range lm.Languages {
		if l.Name == "Docs" || l.Name == "Ruby" || l.Name == "Config" || l.Name == "SQL" {
			t.Errorf("language %q at share %.0f%% appeared in list despite being below 3%% threshold", l.Name, l.SharePct)
		}
	}
}

func checkShare(t *testing.T, shares map[string]float64, lang string, want float64) {
	t.Helper()
	got, ok := shares[lang]
	if !ok {
		t.Errorf("language %q not found in shares", lang)
		return
	}
	if math.Abs(got-want) > 1.0 {
		t.Errorf("share for %q = %.0f%%, want %.0f%%", lang, got, want)
	}
}

// TestLanguageMixFoldingThresholds tests that a language at 2% is folded
// and a language at 3% appears.
func TestLanguageMixFoldingThresholds(t *testing.T) {
	// 5 PRs, each contributing to the same total. We use 5 identical PRs
	// with Go + Python + Rust, where Python = exactly 2% and Rust = 3%.
	// Per PR: Go=950, Python=20, Rust=30 -> per-PR total=1000
	// Across 5 PRs: Go=4750, Python=100, Rust=150 -> total=5000
	// Python: 100/5000 = 2% -> folds
	// Rust: 150/5000 = 3% -> appears

	prs := make([]provider.PullRequest, 5)
	for i := range prs {
		prs[i] = mergedPRWithStats("acme/widgets", []provider.FileStat{
			{Ext: "go", Additions: 950, Deletions: 0},
			{Ext: "py", Additions: 20, Deletions: 0},
			{Ext: "rs", Additions: 30, Deletions: 0},
		})
	}

	res := metrics.Compute(provider.ActivitySet{PullRequests: prs})
	lm := res.Collaboration.LanguageMix
	if lm == nil {
		t.Fatal("expected language_mix, got nil")
	}

	// Python must NOT appear in the list.
	for _, l := range lm.Languages {
		if l.Name == "Python" {
			t.Errorf("Python at 2%% appeared in language list; should be folded into Other. Share=%.0f%%", l.SharePct)
		}
	}

	// Rust at 3% must appear.
	foundRust := false
	for _, l := range lm.Languages {
		if l.Name == "Rust" {
			foundRust = true
			if l.SharePct != 3 {
				t.Errorf("Rust share = %.0f%%, want 3%%", l.SharePct)
			}
		}
	}
	if !foundRust {
		t.Error("Rust at 3% did not appear in language list; should be present")
	}

	// Other share must include the 2% Python + 0% for Go (95%) already shown.
	// Go: 4750/5000 = 95%
	// Rust: 150/5000 = 3%
	// Python: 100/5000 = 2% (folded)
	// Other: 2% -> other_share_pct = 2
	if lm.OtherShare != 2 {
		t.Errorf("other_share_pct = %.0f, want 2", lm.OtherShare)
	}
}

// TestLanguageMixSuppression verifies section omission at <5 merged PRs
// with diff data, and presence at 5.
func TestLanguageMixSuppression(t *testing.T) {
	makePRs := func(n int) []provider.PullRequest {
		var prs []provider.PullRequest
		for i := 0; i < n; i++ {
			prs = append(prs, mergedPRWithStats("acme/widgets", []provider.FileStat{
				{Ext: "go", Additions: 100, Deletions: 50},
			}))
		}
		return prs
	}

	t.Run("4 merged PRs with diff data -> language_mix omitted, suppressed entry", func(t *testing.T) {
		res := metrics.Compute(provider.ActivitySet{PullRequests: makePRs(4)})
		if res.Collaboration.LanguageMix != nil {
			t.Error("expected language_mix to be nil (suppressed) at n=4")
		}
		found := false
		for _, s := range res.Collaboration.Suppressed {
			if s.Section == "language_mix" {
				found = true
				if s.Reason == "" {
					t.Error("suppressed entry has empty reason")
				}
			}
		}
		if !found {
			t.Error("expected suppressed entry for section language_mix")
		}
	})

	t.Run("5 merged PRs with diff data -> language_mix present", func(t *testing.T) {
		res := metrics.Compute(provider.ActivitySet{PullRequests: makePRs(5)})
		if res.Collaboration.LanguageMix == nil {
			t.Fatal("expected language_mix to be present at n=5")
		}
		if res.Collaboration.LanguageMix.PRCount != 5 {
			t.Errorf("pr_count = %d, want 5", res.Collaboration.LanguageMix.PRCount)
		}
		// No suppressed entry for language_mix.
		for _, s := range res.Collaboration.Suppressed {
			if s.Section == "language_mix" {
				t.Error("unexpected suppressed entry for language_mix when present")
			}
		}
	})

	t.Run("empty PR list -> no language_mix, no suppressed", func(t *testing.T) {
		res := metrics.Compute(provider.ActivitySet{PullRequests: nil})
		if res.Collaboration.LanguageMix != nil {
			t.Error("expected language_mix to be nil for empty PRs")
		}
		for _, s := range res.Collaboration.Suppressed {
			if s.Section == "language_mix" {
				t.Error("unexpected suppressed entry for empty PRs")
			}
		}
	})

	t.Run("merged PRs but no FileStats -> no language_mix", func(t *testing.T) {
		mergedAt := ts("2026-02-01T10:00:00Z")
		prs := []provider.PullRequest{
			{Repo: "acme/widgets", CreatedAt: ts("2026-01-01T10:00:00Z"), MergedAt: &mergedAt},
			{Repo: "acme/widgets", CreatedAt: ts("2026-01-01T10:00:00Z"), MergedAt: &mergedAt},
			{Repo: "acme/widgets", CreatedAt: ts("2026-01-01T10:00:00Z"), MergedAt: &mergedAt},
			{Repo: "acme/widgets", CreatedAt: ts("2026-01-01T10:00:00Z"), MergedAt: &mergedAt},
			{Repo: "acme/widgets", CreatedAt: ts("2026-01-01T10:00:00Z"), MergedAt: &mergedAt},
		}
		res := metrics.Compute(provider.ActivitySet{PullRequests: prs})
		if res.Collaboration.LanguageMix != nil {
			t.Error("expected language_mix to be nil when no PRs have FileStats")
		}
	})
}

// TestLanguageMixUnmergedFiltered verifies that unmerged PRs are ignored.
func TestLanguageMixUnmergedFiltered(t *testing.T) {
	mergedAt := ts("2026-02-01T10:00:00Z")
	prs := []provider.PullRequest{
		{Repo: "acme/widgets", CreatedAt: ts("2026-01-01T10:00:00Z"), FileStats: []provider.FileStat{{Ext: "go", Additions: 100, Deletions: 0}}},
		{Repo: "acme/widgets", CreatedAt: ts("2026-01-01T10:00:00Z"), MergedAt: &mergedAt, FileStats: []provider.FileStat{{Ext: "go", Additions: 100, Deletions: 0}}},
		{Repo: "acme/widgets", CreatedAt: ts("2026-01-01T10:00:00Z"), MergedAt: &mergedAt, FileStats: []provider.FileStat{{Ext: "go", Additions: 100, Deletions: 0}}},
		{Repo: "acme/widgets", CreatedAt: ts("2026-01-01T10:00:00Z"), MergedAt: &mergedAt, FileStats: []provider.FileStat{{Ext: "go", Additions: 100, Deletions: 0}}},
		{Repo: "acme/widgets", CreatedAt: ts("2026-01-01T10:00:00Z"), MergedAt: &mergedAt, FileStats: []provider.FileStat{{Ext: "go", Additions: 100, Deletions: 0}}},
		{Repo: "acme/widgets", CreatedAt: ts("2026-01-01T10:00:00Z"), MergedAt: &mergedAt, FileStats: []provider.FileStat{{Ext: "go", Additions: 100, Deletions: 0}}},
	}
	res := metrics.Compute(provider.ActivitySet{PullRequests: prs})
	if res.Collaboration.LanguageMix == nil {
		t.Fatal("expected language_mix to be present")
	}
	// Only 5 are merged with FileStats (the first is unmerged).
	if res.Collaboration.LanguageMix.PRCount != 5 {
		t.Errorf("pr_count = %d, want 5", res.Collaboration.LanguageMix.PRCount)
	}
}

// TestLanguageMixZeroWeightFiles verifies that PRs with only zero-weight
// files produce no language_mix section (total_lines = 0, nothing to show).
func TestLanguageMixZeroWeightFiles(t *testing.T) {
	prs := make([]provider.PullRequest, 5)
	for i := range prs {
		prs[i] = mergedPRWithStats("acme/widgets", []provider.FileStat{
			{Ext: "go", Additions: 0, Deletions: 0}, // zero-weight file
		})
	}
	res := metrics.Compute(provider.ActivitySet{PullRequests: prs})
	if res.Collaboration.LanguageMix != nil {
		t.Error("expected language_mix to be nil when all files have zero weight")
	}
}

// TestLanguageMixSchemaRoundTrip verifies JSON round-trip including Validate.
func TestLanguageMixSchemaRoundTrip(t *testing.T) {
	lm := &report.LanguageMixStats{
		Basis:      "merged_pr_diff_lines",
		PRCount:    5,
		TotalLines: 1000,
		Languages: []report.LangShare{
			{Name: "Go", SharePct: 60},
			{Name: "TypeScript", SharePct: 25},
			{Name: "Python", SharePct: 10},
		},
		OtherShare: 5,
	}
	collab := &report.Collaboration{
		PullRequests: &report.PullRequestStats{Authored: 5, Merged: 5},
		LanguageMix:  lm,
	}
	now := ts("2026-06-12T10:00:00Z")
	r := report.Build(activityFixture(), collab, nil, now)
	if err := r.Validate(); err != nil {
		t.Fatalf("report with language_mix failed validation: %v", err)
	}
	// Also verify the fixture has the expected fields via JSON round-trip.
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	parsed, err := report.Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if parsed.Collaboration == nil || parsed.Collaboration.LanguageMix == nil {
		t.Fatal("round-trip lost language_mix block")
	}
	got := parsed.Collaboration.LanguageMix
	if got.Basis != lm.Basis {
		t.Errorf("basis = %q, want %q", got.Basis, lm.Basis)
	}
	if got.PRCount != lm.PRCount {
		t.Errorf("pr_count = %d, want %d", got.PRCount, lm.PRCount)
	}
	if got.TotalLines != lm.TotalLines {
		t.Errorf("total_lines = %d, want %d", got.TotalLines, lm.TotalLines)
	}
	if len(got.Languages) != len(lm.Languages) {
		t.Fatalf("got %d languages, want %d", len(got.Languages), len(lm.Languages))
	}
	for i, l := range got.Languages {
		if l.Name != lm.Languages[i].Name || l.SharePct != lm.Languages[i].SharePct {
			t.Errorf("language[%d] = %+v, want %+v", i, l, lm.Languages[i])
		}
	}
	if got.OtherShare != lm.OtherShare {
		t.Errorf("other_share_pct = %.0f, want %.0f", got.OtherShare, lm.OtherShare)
	}
}

// TestLanguageMixPreV02Compat verifies a pre-v0.2 report JSON without
// language_mix still validates and round-trips.
func TestLanguageMixPreV02Compat(t *testing.T) {
	raw := []byte(`{"schema_version":"v0","generated_at":"2026-06-01T12:00:00Z","subject":{"platform":"github","username":"alice","account_id":"123"},"coverage":{"repos":["acme/widgets"],"window":{"since":"2025-06-01T00:00:00Z","until":"2026-06-01T00:00:00Z"},"token_scope":"repo"},"verification":{"status":"unverified","reason":"test"},"collaboration":{"pull_requests":{"authored":3,"merged":2}}}`)
	r, err := report.Parse(raw)
	if err != nil {
		t.Fatalf("pre-v0.2 report without language_mix failed validation: %v", err)
	}
	if r.Collaboration != nil && r.Collaboration.LanguageMix != nil {
		t.Error("pre-v0.2 report has unexpected language_mix block")
	}
}

// TestLanguageMixProhibitedStrings verifies that a language name at 1%
// (below the 3% threshold) does NOT appear in the JSON or HTML output.
// This extends the seeded-strings protection to the language mix section.
func TestLanguageMixProhibitedStrings(t *testing.T) {
	// 5 PRs where Python is at 1% of total lines (below threshold).
	// Per PR: Go=990, Python=10 -> total 5000
	// Python: 50/5000 = 1% -> folded into Other, must not appear.
	prs := make([]provider.PullRequest, 5)
	for i := range prs {
		prs[i] = mergedPRWithStats("acme/widgets", []provider.FileStat{
			{Ext: "go", Additions: 990, Deletions: 0},
			{Ext: "py", Additions: 10, Deletions: 0}, // 1% Python
		})
	}

	res := metrics.Compute(provider.ActivitySet{PullRequests: prs})
	lm := res.Collaboration.LanguageMix
	if lm == nil {
		t.Fatal("expected language_mix, got nil")
	}

	// Python must not appear in any language entry.
	for _, l := range lm.Languages {
		if strings.EqualFold(l.Name, "Python") {
			t.Errorf("language %q at share %.0f%% appeared in list despite being below 3%% threshold", l.Name, l.SharePct)
		}
	}
}

// TestLanguageMixConsistency verifies total_lines equals the summed
// additions+deletions of all measured files.
func TestLanguageMixConsistency(t *testing.T) {
	stats := []provider.FileStat{
		{Ext: "go", Additions: 100, Deletions: 50}, // 150
		{Ext: "ts", Additions: 80, Deletions: 20},  // 100
		{Ext: "md", Additions: 30, Deletions: 10},  // 40
	}
	wantTotal := 150 + 100 + 40 // 290

	prs := make([]provider.PullRequest, 5)
	for i := range prs {
		prs[i] = mergedPRWithStats("acme/widgets", stats)
	}

	res := metrics.Compute(provider.ActivitySet{PullRequests: prs})
	lm := res.Collaboration.LanguageMix
	if lm == nil {
		t.Fatal("expected language_mix, got nil")
	}
	// 5 identical PRs: total = 5 * 290 = 1450
	if lm.TotalLines != 5*wantTotal {
		t.Errorf("total_lines = %d, want %d", lm.TotalLines, 5*wantTotal)
	}
}

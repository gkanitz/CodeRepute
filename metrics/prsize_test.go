package metrics_test

import (
	"math"
	"testing"

	"github.com/gkanitz/coderepute/metrics"
	"github.com/gkanitz/coderepute/provider"
	"github.com/gkanitz/coderepute/report"
)

func mergedPR(repo string, additions, deletions, files int) provider.PullRequest {
	mergedAt := ts("2026-02-01T10:00:00Z")
	return provider.PullRequest{
		Repo:      repo,
		CreatedAt: ts("2026-01-01T10:00:00Z"),
		MergedAt:  &mergedAt,
		Additions: additions,
		Deletions: deletions,
		Files:     files,
	}
}

func unmergedPR(repo string, additions, deletions, files int) provider.PullRequest {
	return provider.PullRequest{
		Repo:      repo,
		CreatedAt: ts("2026-01-01T10:00:00Z"),
		Additions: additions,
		Deletions: deletions,
		Files:     files,
	}
}

func TestPRSizeStats(t *testing.T) {
	tests := []struct {
		name    string
		prs     []provider.PullRequest
		want    *report.PRSizeStats
		wantNil bool
	}{
		{
			name:    "empty PR list yields nil",
			prs:     nil,
			wantNil: true,
		},
		{
			name: "no merged PRs yields nil",
			prs: []provider.PullRequest{
				{Repo: "acme/widgets", CreatedAt: ts("2026-01-01T10:00:00Z"), Additions: 100, Deletions: 50, Files: 5},
				{Repo: "acme/widgets", CreatedAt: ts("2026-02-01T10:00:00Z"), Additions: 200, Deletions: 100, Files: 8},
			},
			wantNil: true,
		},
		{
			name: "merged PRs without diff data (zeros) yields nil",
			prs: []provider.PullRequest{
				{Repo: "acme/widgets", CreatedAt: ts("2026-01-01T10:00:00Z"), MergedAt: merged("2026-02-01T10:00:00Z")},
			},
			wantNil: true,
		},
		{
			name: "single merged PR with diff data: suppressed (need ≥ 5)",
			prs: []provider.PullRequest{
				mergedPR("acme/widgets", 200, 150, 10),
			},
			wantNil: true,
		},
		{
			name: "odd count: five PRs with different sizes",
			prs: []provider.PullRequest{
				mergedPR("acme/widgets", 100, 50, 3),   // 150 lines, 3 files
				mergedPR("acme/widgets", 200, 100, 6),  // 300 lines, 6 files
				mergedPR("acme/widgets", 300, 100, 8),  // 400 lines, 8 files
				mergedPR("acme/widgets", 400, 150, 12), // 550 lines, 12 files
				mergedPR("acme/widgets", 500, 200, 15), // 700 lines, 15 files
			},
			want: &report.PRSizeStats{
				Count:               5,
				MedianLines:         400,
				FilesMedian:         8,
				SmallShare:          0.6, // 3 of 5 ≤ 400 (150, 300, 400)
				SmallThresholdLines: 400,
			},
		},
		{
			name: "even count: six PRs - median is avg of middle two",
			prs: []provider.PullRequest{
				mergedPR("acme/widgets", 100, 50, 3),   // 150 lines
				mergedPR("acme/widgets", 150, 100, 5),  // 250 lines, 5 files
				mergedPR("acme/widgets", 200, 100, 6),  // 300 lines, 6 files
				mergedPR("acme/widgets", 300, 150, 10), // 450 lines, 10 files
				mergedPR("acme/widgets", 400, 200, 13), // 600 lines, 13 files
				mergedPR("acme/widgets", 500, 200, 14), // 700 lines, 14 files
			},
			want: &report.PRSizeStats{
				Count:               6,
				MedianLines:         375, // (300+450)/2
				FilesMedian:         8,   // (6+10)/2
				SmallShare:          0.5, // 3 of 6 ≤ 400
				SmallThresholdLines: 400,
			},
		},
		{
			name: "boundary: exactly 400 lines counts as small (≤ 400)",
			prs: []provider.PullRequest{
				mergedPR("acme/widgets", 200, 200, 5), // 400 lines
				mergedPR("acme/widgets", 100, 100, 3), // 200 lines
				mergedPR("acme/widgets", 100, 50, 3),  // 150 lines
				mergedPR("acme/widgets", 300, 200, 7), // 500 lines
				mergedPR("acme/widgets", 200, 100, 6), // 300 lines
			},
			want: &report.PRSizeStats{
				Count:               5,
				MedianLines:         300,
				FilesMedian:         6,   // sorted by lines: [3, 3, 6, 5, 7], median=6
				SmallShare:          0.8, // 4 of 5 ≤ 400
				SmallThresholdLines: 400,
			},
		},
		{
			name: "boundary: 401 lines is not small, with enough PRs",
			prs: []provider.PullRequest{
				mergedPR("acme/widgets", 200, 201, 5), // 401 lines
				mergedPR("acme/widgets", 100, 50, 3),  // 150 lines
				mergedPR("acme/widgets", 50, 50, 2),   // 100 lines
				mergedPR("acme/widgets", 300, 100, 7), // 400 lines
				mergedPR("acme/widgets", 100, 100, 4), // 200 lines
			},
			want: &report.PRSizeStats{
				Count:               5,
				MedianLines:         200,
				FilesMedian:         4,
				SmallShare:          0.8, // 4 of 5 ≤ 400
				SmallThresholdLines: 400,
			},
		},
		{
			name: "unmerged PRs filtered out, need 5 merged with diff data",
			prs: []provider.PullRequest{
				mergedPR("acme/widgets", 100, 50, 3),     // 150 lines
				mergedPR("acme/widgets", 200, 100, 6),    // 300 lines
				mergedPR("acme/widgets", 150, 100, 5),    // 250 lines
				mergedPR("acme/widgets", 300, 150, 10),   // 450 lines
				mergedPR("acme/widgets", 200, 200, 8),    // 400 lines
				unmergedPR("acme/widgets", 500, 200, 10), // unmerged, should be filtered
			},
			want: &report.PRSizeStats{
				Count:               5,
				MedianLines:         300,
				FilesMedian:         6,
				SmallShare:          0.8, // 4 of 5 ≤ 400
				SmallThresholdLines: 400,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := metrics.Compute(provider.ActivitySet{PullRequests: tt.prs})
			got := res.Collaboration.PRSize
			if tt.wantNil {
				if got != nil {
					t.Errorf("got pr_size=%+v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("got nil pr_size, want non-nil")
			}
			if got.Count != tt.want.Count {
				t.Errorf("count = %d, want %d", got.Count, tt.want.Count)
			}
			if math.Abs(got.MedianLines-tt.want.MedianLines) > 0.01 {
				t.Errorf("median_lines = %f, want %f", got.MedianLines, tt.want.MedianLines)
			}
			if math.Abs(got.FilesMedian-tt.want.FilesMedian) > 0.01 {
				t.Errorf("files_median = %f, want %f", got.FilesMedian, tt.want.FilesMedian)
			}
			if math.Abs(got.SmallShare-tt.want.SmallShare) > 0.0001 {
				t.Errorf("small_share = %f, want %f", got.SmallShare, tt.want.SmallShare)
			}
			if got.SmallThresholdLines != tt.want.SmallThresholdLines {
				t.Errorf("small_threshold_lines = %d, want %d", got.SmallThresholdLines, tt.want.SmallThresholdLines)
			}
		})
	}
}

func TestPRSizeSuppression(t *testing.T) {
	// 4 merged PRs with diff data -> suppressed; 5 -> present.
	makePRs := func(n int) []provider.PullRequest {
		var prs []provider.PullRequest
		for i := 0; i < n; i++ {
			prs = append(prs, mergedPR("acme/widgets", 100+i*10, 50, 3))
		}
		return prs
	}

	t.Run("4 merged PRs with diff data -> pr_size omitted, suppressed entry", func(t *testing.T) {
		res := metrics.Compute(provider.ActivitySet{PullRequests: makePRs(4)})
		if res.Collaboration.PRSize != nil {
			t.Error("expected pr_size to be nil (suppressed) at n=4")
		}
		found := false
		for _, s := range res.Collaboration.Suppressed {
			if s.Section == "pr_size" {
				found = true
				if s.Reason == "" {
					t.Error("suppressed entry has empty reason")
				}
			}
		}
		if !found {
			t.Error("expected suppressed entry for section pr_size")
		}
	})

	t.Run("5 merged PRs with diff data -> pr_size present", func(t *testing.T) {
		res := metrics.Compute(provider.ActivitySet{PullRequests: makePRs(5)})
		if res.Collaboration.PRSize == nil {
			t.Error("expected pr_size to be present at n=5")
		}
		if res.Collaboration.PRSize.Count != 5 {
			t.Errorf("count = %d, want 5", res.Collaboration.PRSize.Count)
		}
		// No suppressed entry for pr_size
		for _, s := range res.Collaboration.Suppressed {
			if s.Section == "pr_size" {
				t.Error("unexpected suppressed entry for pr_size when present")
			}
		}
	})
}

package metrics_test

import (
	"testing"
	"time"

	"github.com/gkanitz/coderepute/metrics"
	"github.com/gkanitz/coderepute/provider"
)

func ts(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func merged(at string) *time.Time {
	t := ts(at)
	return &t
}

func TestComputePullRequestStats(t *testing.T) {
	tests := []struct {
		name         string
		prs          []provider.PullRequest
		wantAuthored int
		wantMerged   int
	}{
		{
			name:         "empty activity",
			prs:          nil,
			wantAuthored: 0,
			wantMerged:   0,
		},
		{
			name: "single unmerged PR",
			prs: []provider.PullRequest{
				{Repo: "acme/widgets", CreatedAt: ts("2026-01-10T09:00:00Z")},
			},
			wantAuthored: 1,
			wantMerged:   0,
		},
		{
			name: "mixed merged and unmerged",
			prs: []provider.PullRequest{
				{Repo: "acme/widgets", CreatedAt: ts("2026-01-10T09:00:00Z"), MergedAt: merged("2026-01-11T09:00:00Z")},
				{Repo: "acme/widgets", CreatedAt: ts("2026-02-01T09:00:00Z")},
				{Repo: "acme/gears", CreatedAt: ts("2026-03-01T09:00:00Z"), MergedAt: merged("2026-03-02T12:00:00Z")},
			},
			wantAuthored: 3,
			wantMerged:   2,
		},
		{
			name: "closed without merge is not merged",
			prs: []provider.PullRequest{
				{Repo: "acme/widgets", CreatedAt: ts("2026-01-10T09:00:00Z"), ClosedAt: merged("2026-01-12T09:00:00Z")},
			},
			wantAuthored: 1,
			wantMerged:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := metrics.Compute(provider.ActivitySet{PullRequests: tt.prs})
			got := res.Collaboration.PullRequests
			if got == nil {
				t.Fatal("pull request stats not computed")
			}
			if got.Authored != tt.wantAuthored || got.Merged != tt.wantMerged {
				t.Errorf("got authored=%d merged=%d, want authored=%d merged=%d",
					got.Authored, got.Merged, tt.wantAuthored, tt.wantMerged)
			}
		})
	}
}

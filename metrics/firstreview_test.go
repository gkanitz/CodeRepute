package metrics_test

import (
	"testing"

	"github.com/gkanitz/coderepute/metrics"
	"github.com/gkanitz/coderepute/provider"
)

func TestComputeTimeToFirstReview(t *testing.T) {
	tests := []struct {
		name            string
		prs             []provider.PullRequest
		wantNil         bool
		wantCount       int
		wantMedianHours float64
	}{
		{
			name:    "empty window omits the stat",
			prs:     nil,
			wantNil: true,
		},
		{
			name: "PRs that never got a review omit the stat",
			prs: []provider.PullRequest{
				{Repo: "acme/widgets", CreatedAt: ts("2026-01-10T09:00:00Z")},
				{Repo: "acme/widgets", CreatedAt: ts("2026-02-10T09:00:00Z"), MergedAt: merged("2026-02-12T09:00:00Z")},
			},
			wantNil: true,
		},
		{
			name: "single reviewed PR",
			prs: []provider.PullRequest{
				{Repo: "acme/widgets", CreatedAt: ts("2026-01-10T09:00:00Z"), FirstReviewAt: merged("2026-01-10T15:00:00Z")},
			},
			wantCount:       1,
			wantMedianHours: 6,
		},
		{
			name: "unreviewed PRs do not drag the sample",
			prs: []provider.PullRequest{
				{Repo: "acme/widgets", CreatedAt: ts("2026-01-10T00:00:00Z"), FirstReviewAt: merged("2026-01-10T02:00:00Z")},
				{Repo: "acme/widgets", CreatedAt: ts("2026-02-10T00:00:00Z"), FirstReviewAt: merged("2026-02-10T12:00:00Z")},
				{Repo: "acme/gears", CreatedAt: ts("2026-03-10T00:00:00Z"), FirstReviewAt: merged("2026-03-11T00:00:00Z")},
				{Repo: "acme/gears", CreatedAt: ts("2026-04-10T00:00:00Z")},
			},
			wantCount:       3,
			wantMedianHours: 12,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := metrics.Compute(provider.ActivitySet{PullRequests: tt.prs})
			got := res.Collaboration.TimeToFirstReview
			if tt.wantNil {
				if got != nil {
					t.Fatalf("time to first review = %+v, want omitted", got)
				}
				return
			}
			if got == nil {
				t.Fatal("time to first review not computed")
			}
			if got.Count != tt.wantCount || got.MedianHours != tt.wantMedianHours {
				t.Errorf("got count=%d median=%v, want count=%d median=%v",
					got.Count, got.MedianHours, tt.wantCount, tt.wantMedianHours)
			}
		})
	}
}

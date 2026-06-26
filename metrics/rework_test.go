package metrics_test

import (
	"testing"

	"github.com/gkanitz/coderepute/metrics"
	"github.com/gkanitz/coderepute/provider"
)

func TestComputeReworkShare(t *testing.T) {
	tests := []struct {
		name         string
		prs          []provider.PullRequest
		wantNil      bool
		wantReviewed int
		wantReworked int
		wantShare    float64
	}{
		{
			name:    "empty window omits the stat",
			prs:     nil,
			wantNil: true,
		},
		{
			name: "no reviewed PRs omits the stat",
			prs: []provider.PullRequest{
				{Repo: "acme/widgets", CreatedAt: ts("2026-01-10T09:00:00Z")},
				{Repo: "acme/widgets", CreatedAt: ts("2026-02-10T09:00:00Z"), MergedAt: merged("2026-02-12T09:00:00Z")},
			},
			wantNil: true,
		},
		{
			name: "single reviewed PR without rework",
			prs: []provider.PullRequest{
				{Repo: "acme/widgets", CreatedAt: ts("2026-01-10T09:00:00Z"), FirstReviewAt: merged("2026-01-10T15:00:00Z")},
			},
			wantReviewed: 1,
			wantReworked: 0,
			wantShare:    0,
		},
		{
			name: "single reviewed PR with changes requested",
			prs: []provider.PullRequest{
				{Repo: "acme/widgets", CreatedAt: ts("2026-01-10T09:00:00Z"), FirstReviewAt: merged("2026-01-10T15:00:00Z"), ChangesRequested: 2},
			},
			wantReviewed: 1,
			wantReworked: 1,
			wantShare:    1,
		},
		{
			name: "share counts only reviewed PRs in the denominator",
			prs: []provider.PullRequest{
				{Repo: "acme/widgets", CreatedAt: ts("2026-01-10T00:00:00Z"), FirstReviewAt: merged("2026-01-10T02:00:00Z"), ChangesRequested: 1},
				{Repo: "acme/widgets", CreatedAt: ts("2026-02-10T00:00:00Z"), FirstReviewAt: merged("2026-02-10T12:00:00Z")},
				{Repo: "acme/gears", CreatedAt: ts("2026-03-10T00:00:00Z"), FirstReviewAt: merged("2026-03-11T00:00:00Z")},
				{Repo: "acme/gears", CreatedAt: ts("2026-04-10T00:00:00Z"), FirstReviewAt: merged("2026-04-11T00:00:00Z"), ChangesRequested: 3},
				{Repo: "acme/gears", CreatedAt: ts("2026-05-10T00:00:00Z")}, // never reviewed: excluded
			},
			wantReviewed: 4,
			wantReworked: 2,
			wantShare:    0.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := metrics.Compute(provider.ActivitySet{PullRequests: tt.prs})
			got := res.Collaboration.Rework
			if tt.wantNil {
				if got != nil {
					t.Fatalf("rework = %+v, want omitted", got)
				}
				return
			}
			if got == nil {
				t.Fatal("rework share not computed")
			}
			if got.ReviewedPRs != tt.wantReviewed || got.ReworkedPRs != tt.wantReworked || got.Share != tt.wantShare {
				t.Errorf("got reviewed=%d reworked=%d share=%v, want reviewed=%d reworked=%d share=%v",
					got.ReviewedPRs, got.ReworkedPRs, got.Share,
					tt.wantReviewed, tt.wantReworked, tt.wantShare)
			}
		})
	}
}

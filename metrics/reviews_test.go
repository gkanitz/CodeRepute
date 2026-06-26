package metrics_test

import (
	"testing"

	"github.com/gkanitz/coderepute/metrics"
	"github.com/gkanitz/coderepute/provider"
)

func TestComputeReviewsGiven(t *testing.T) {
	tests := []struct {
		name                 string
		reviews              []provider.Review
		wantTotal            int
		wantApprovals        int
		wantChangesRequested int
	}{
		{
			name:    "empty window yields explicit zeros",
			reviews: nil,
		},
		{
			name: "single review",
			reviews: []provider.Review{
				{Repo: "acme/widgets", SubmittedAt: ts("2026-02-01T10:00:00Z"), State: "APPROVED"},
			},
			wantTotal:     1,
			wantApprovals: 1,
		},
		{
			name: "mixed states are counted by kind",
			reviews: []provider.Review{
				{Repo: "acme/widgets", SubmittedAt: ts("2026-02-01T10:00:00Z"), State: "APPROVED"},
				{Repo: "acme/widgets", SubmittedAt: ts("2026-02-03T10:00:00Z"), State: "CHANGES_REQUESTED"},
				{Repo: "acme/gears", SubmittedAt: ts("2026-02-05T10:00:00Z"), State: "COMMENTED"},
				{Repo: "acme/gears", SubmittedAt: ts("2026-02-07T10:00:00Z"), State: "APPROVED"},
			},
			wantTotal:            4,
			wantApprovals:        2,
			wantChangesRequested: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := metrics.Compute(provider.ActivitySet{ReviewsGiven: tt.reviews})
			got := res.Collaboration.ReviewsGiven
			if got == nil {
				t.Fatal("reviews given stats not computed")
			}
			if got.Total != tt.wantTotal || got.Approvals != tt.wantApprovals || got.ChangesRequested != tt.wantChangesRequested {
				t.Errorf("got total=%d approvals=%d changes_requested=%d, want total=%d approvals=%d changes_requested=%d",
					got.Total, got.Approvals, got.ChangesRequested,
					tt.wantTotal, tt.wantApprovals, tt.wantChangesRequested)
			}
		})
	}
}

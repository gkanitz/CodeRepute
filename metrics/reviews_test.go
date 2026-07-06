package metrics_test

import (
	"testing"

	"github.com/gkanitz/coderepute/metrics"
	"github.com/gkanitz/coderepute/provider"
	"github.com/gkanitz/coderepute/report"
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

// TestNormalizedDeepReview verifies the size-normalized deep-review threshold:
// deep ⇔ comments ≥ clamp(ceil(lines/100), 3, 10) where lines = additions+deletions.
// Reviews without diff data (PRLines=0) use the legacy ≥3 threshold.
func TestNormalizedDeepReview(t *testing.T) {
	tests := []struct {
		name        string
		reviews     []provider.Review
		wantDeep    int
		wantDeepPct float64
		wantBasis   *report.DepthBasis
	}{
		{
			name:      "no reviews -> no depth",
			reviews:   nil,
			wantDeep:  0,
			wantBasis: nil,
		},
		// Normalized threshold tests with PRLines > 0
		// threshold = clamp(ceil(lines/100), 3, 10)
		// ceil(10/100) = ceil(0.1) = 1, clamp(1, 3, 10) = 3
		// ceil(500/100) = 5, clamp(5, 3, 10) = 5
		// ceil(2000/100) = 20, clamp(20, 3, 10) = 10
		{
			name: "10 lines, 3 comments -> deep (threshold=3)",
			reviews: []provider.Review{
				{Repo: "acme/widgets", SubmittedAt: ts("2026-02-01T10:00:00Z"), State: "APPROVED", CommentCount: 3, PRLines: 10},
			},
			wantDeep:  1,
			wantBasis: &report.DepthBasis{Measured: 1, Fallback: 0},
		},
		{
			name: "500 lines, 4 comments -> shallow (threshold=5, 4<5)",
			reviews: []provider.Review{
				{Repo: "acme/widgets", SubmittedAt: ts("2026-02-01T10:00:00Z"), State: "APPROVED", CommentCount: 4, PRLines: 500},
			},
			wantDeep:  0,
			wantBasis: &report.DepthBasis{Measured: 1, Fallback: 0},
		},
		{
			name: "500 lines, 5 comments -> deep (threshold=5, 5>=5)",
			reviews: []provider.Review{
				{Repo: "acme/widgets", SubmittedAt: ts("2026-02-01T10:00:00Z"), State: "APPROVED", CommentCount: 5, PRLines: 500},
			},
			wantDeep:  1,
			wantBasis: &report.DepthBasis{Measured: 1, Fallback: 0},
		},
		{
			name: "2000 lines, 10 comments -> deep (threshold=10, 10>=10)",
			reviews: []provider.Review{
				{Repo: "acme/widgets", SubmittedAt: ts("2026-02-01T10:00:00Z"), State: "APPROVED", CommentCount: 10, PRLines: 2000},
			},
			wantDeep:  1,
			wantBasis: &report.DepthBasis{Measured: 1, Fallback: 0},
		},
		{
			name: "2000 lines, 9 comments -> shallow (threshold=10, 9<10)",
			reviews: []provider.Review{
				{Repo: "acme/widgets", SubmittedAt: ts("2026-02-01T10:00:00Z"), State: "APPROVED", CommentCount: 9, PRLines: 2000},
			},
			wantDeep:  0,
			wantBasis: &report.DepthBasis{Measured: 1, Fallback: 0},
		},
		{
			name: "50 lines, 2 comments -> shallow (threshold=3, 2<3)",
			reviews: []provider.Review{
				{Repo: "acme/widgets", SubmittedAt: ts("2026-02-01T10:00:00Z"), State: "APPROVED", CommentCount: 2, PRLines: 50},
			},
			wantDeep:  0,
			wantBasis: &report.DepthBasis{Measured: 1, Fallback: 0},
		},
		{
			name: "75 lines, 3 comments -> deep (threshold=3, 3>=3)",
			reviews: []provider.Review{
				{Repo: "acme/widgets", SubmittedAt: ts("2026-02-01T10:00:00Z"), State: "APPROVED", CommentCount: 3, PRLines: 75},
			},
			wantDeep:  1,
			wantBasis: &report.DepthBasis{Measured: 1, Fallback: 0},
		},
		{
			name: "150 lines, 3 comments -> deep (threshold=3, 3>=3)",
			reviews: []provider.Review{
				{Repo: "acme/widgets", SubmittedAt: ts("2026-02-01T10:00:00Z"), State: "APPROVED", CommentCount: 3, PRLines: 150},
			},
			wantDeep:  1,
			wantBasis: &report.DepthBasis{Measured: 1, Fallback: 0},
		},
		{
			name: "800 lines, 8 comments -> deep (threshold=8, 8>=8)",
			reviews: []provider.Review{
				{Repo: "acme/widgets", SubmittedAt: ts("2026-02-01T10:00:00Z"), State: "APPROVED", CommentCount: 8, PRLines: 800},
			},
			wantDeep:  1,
			wantBasis: &report.DepthBasis{Measured: 1, Fallback: 0},
		},
		{
			name: "1000 lines, 9 comments -> shallow (threshold=10, 9<10)",
			reviews: []provider.Review{
				{Repo: "acme/widgets", SubmittedAt: ts("2026-02-01T10:00:00Z"), State: "APPROVED", CommentCount: 9, PRLines: 1000},
			},
			wantDeep:  0,
			wantBasis: &report.DepthBasis{Measured: 1, Fallback: 0},
		},
		// Fallback tests (PRLines=0, legacy ≥3 threshold)
		{
			name: "no diff data, 2 comments -> shallow (legacy, 2<3)",
			reviews: []provider.Review{
				{Repo: "acme/widgets", SubmittedAt: ts("2026-02-01T10:00:00Z"), State: "APPROVED", CommentCount: 2, PRLines: 0},
			},
			wantDeep:  0,
			wantBasis: &report.DepthBasis{Measured: 0, Fallback: 1},
		},
		{
			name: "no diff data, 3 comments -> deep (legacy, 3>=3)",
			reviews: []provider.Review{
				{Repo: "acme/widgets", SubmittedAt: ts("2026-02-01T10:00:00Z"), State: "APPROVED", CommentCount: 3, PRLines: 0},
			},
			wantDeep:  1,
			wantBasis: &report.DepthBasis{Measured: 0, Fallback: 1},
		},
		{
			name: "no diff data, 10 comments -> deep (legacy)",
			reviews: []provider.Review{
				{Repo: "acme/widgets", SubmittedAt: ts("2026-02-01T10:00:00Z"), State: "APPROVED", CommentCount: 10, PRLines: 0},
			},
			wantDeep:  1,
			wantBasis: &report.DepthBasis{Measured: 0, Fallback: 1},
		},
		// Mixed measured and fallback
		{
			name: "mixed: measured + fallback reviews",
			reviews: []provider.Review{
				{Repo: "acme/widgets", SubmittedAt: ts("2026-02-01T10:00:00Z"), State: "APPROVED", CommentCount: 5, PRLines: 500}, // measured, threshold=5, deep
				{Repo: "acme/widgets", SubmittedAt: ts("2026-02-02T10:00:00Z"), State: "APPROVED", CommentCount: 2, PRLines: 0},   // fallback, shallow
				{Repo: "acme/widgets", SubmittedAt: ts("2026-02-03T10:00:00Z"), State: "APPROVED", CommentCount: 3, PRLines: 0},   // fallback, deep
				{Repo: "acme/widgets", SubmittedAt: ts("2026-02-04T10:00:00Z"), State: "APPROVED", CommentCount: 3, PRLines: 10},  // measured, threshold=3, deep
			},
			wantDeep:  3, // review 1 (deep), review 3 (deep), review 4 (deep)
			wantBasis: &report.DepthBasis{Measured: 2, Fallback: 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := metrics.Compute(provider.ActivitySet{ReviewsGiven: tt.reviews})
			got := res.Collaboration.ReviewsGiven
			if got == nil {
				t.Fatal("reviews given stats not computed")
			}
			if got.DeepReviewCount != tt.wantDeep {
				t.Errorf("deep_review_count = %d, want %d", got.DeepReviewCount, tt.wantDeep)
			}
			if tt.wantBasis != nil {
				if got.DepthBasis == nil {
					t.Fatal("depth_basis is nil, want non-nil")
				}
				if got.DepthBasis.Measured != tt.wantBasis.Measured {
					t.Errorf("depth_basis.measured = %d, want %d", got.DepthBasis.Measured, tt.wantBasis.Measured)
				}
				if got.DepthBasis.Fallback != tt.wantBasis.Fallback {
					t.Errorf("depth_basis.fallback = %d, want %d", got.DepthBasis.Fallback, tt.wantBasis.Fallback)
				}
			} else if got.DepthBasis != nil {
				t.Errorf("depth_basis = %+v, want nil", got.DepthBasis)
			}
		})
	}
}

// TestErrDiffShapeUnsupportedFallback verifies that when no diff data is
// available (PRLines=0 for all reviews), deep review uses the legacy ≥3
// threshold, and depth_basis shows all fallback.
func TestErrDiffShapeUnsupportedFallback(t *testing.T) {
	reviews := []provider.Review{
		{Repo: "acme/widgets", SubmittedAt: ts("2026-02-01T10:00:00Z"), State: "APPROVED", CommentCount: 2, PRLines: 0},
		{Repo: "acme/widgets", SubmittedAt: ts("2026-02-02T10:00:00Z"), State: "APPROVED", CommentCount: 3, PRLines: 0},
		{Repo: "acme/widgets", SubmittedAt: ts("2026-02-03T10:00:00Z"), State: "APPROVED", CommentCount: 5, PRLines: 0},
	}
	res := metrics.Compute(provider.ActivitySet{ReviewsGiven: reviews})
	got := res.Collaboration.ReviewsGiven
	if got == nil {
		t.Fatal("reviews given stats not computed")
	}
	if got.DeepReviewCount != 2 {
		t.Errorf("deep_review_count = %d, want 2 (legacy ≥3)", got.DeepReviewCount)
	}
	if got.DepthBasis == nil {
		t.Fatal("depth_basis is nil")
	}
	if got.DepthBasis.Measured != 0 {
		t.Errorf("depth_basis.measured = %d, want 0", got.DepthBasis.Measured)
	}
	if got.DepthBasis.Fallback != 3 {
		t.Errorf("depth_basis.fallback = %d, want 3", got.DepthBasis.Fallback)
	}
}

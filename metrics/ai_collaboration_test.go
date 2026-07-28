package metrics_test

import (
	"testing"

	"github.com/gkanitz/coderepute/metrics"
	"github.com/gkanitz/coderepute/provider"
)

// TestComputeAICollaboration verifies that the AI collaboration metric
// correctly counts reviews on AI/bot-authored PRs and the deep review
// rate among them, using the same deepReviewThreshold logic.
func TestComputeAICollaboration(t *testing.T) {
	tests := []struct {
		name          string
		reviews       []provider.Review
		wantTotal     int
		wantDeep      int
		wantShare     float64
		wantAgents    []string
		wantZeroState bool // true when Total=0 but struct is populated
	}{
		{
			name:      "empty window yields explicit zero-state",
			reviews:   nil,
			wantTotal: 0,
			wantDeep:  0,
			wantShare: 0,
		},
		{
			name: "human-only reviews excluded entirely",
			reviews: []provider.Review{
				{Repo: "acme/widgets", SubmittedAt: ts("2026-02-01T10:00:00Z"), State: "APPROVED", CommentCount: 5, PRLines: 500, AuthorClass: ""},
				{Repo: "acme/widgets", SubmittedAt: ts("2026-02-02T10:00:00Z"), State: "APPROVED", CommentCount: 3, PRLines: 50, AuthorClass: ""},
			},
			wantTotal: 0,
			wantDeep:  0,
			wantShare: 0,
		},
		{
			name: "single AI review, deep (size-normalized threshold)",
			reviews: []provider.Review{
				{Repo: "acme/widgets", SubmittedAt: ts("2026-02-01T10:00:00Z"), State: "APPROVED", CommentCount: 5, PRLines: 500, AuthorClass: "copilot"},
			},
			wantTotal:  1,
			wantDeep:   1,
			wantShare:  1.0,
			wantAgents: []string{"copilot"},
		},
		{
			name: "single bot review, shallow",
			reviews: []provider.Review{
				{Repo: "acme/widgets", SubmittedAt: ts("2026-02-01T10:00:00Z"), State: "APPROVED", CommentCount: 1, PRLines: 10, AuthorClass: "bot"},
			},
			wantTotal:  1,
			wantDeep:   0,
			wantShare:  0,
			wantAgents: []string{"bot"},
		},
		{
			name: "mixed AI and human: only AI counted",
			reviews: []provider.Review{
				{Repo: "acme/widgets", SubmittedAt: ts("2026-02-01T10:00:00Z"), State: "APPROVED", CommentCount: 5, PRLines: 500, AuthorClass: "copilot"}, // AI, deep (threshold=5)
				{Repo: "acme/widgets", SubmittedAt: ts("2026-02-02T10:00:00Z"), State: "APPROVED", CommentCount: 2, PRLines: 10, AuthorClass: ""},         // human, excluded
				{Repo: "acme/widgets", SubmittedAt: ts("2026-02-03T10:00:00Z"), State: "APPROVED", CommentCount: 3, PRLines: 10, AuthorClass: "devin"},    // AI, deep (threshold=3)
				{Repo: "acme/widgets", SubmittedAt: ts("2026-02-04T10:00:00Z"), State: "APPROVED", CommentCount: 1, PRLines: 2000, AuthorClass: "bot"},    // bot, shallow (threshold=10)
				{Repo: "acme/widgets", SubmittedAt: ts("2026-02-05T10:00:00Z"), State: "APPROVED", CommentCount: 2, PRLines: 0, AuthorClass: "copilot"},   // AI, shallow (legacy threshold=3, 2<3)
			},
			wantTotal:  4,
			wantDeep:   2, // review 1 (deep), review 3 (deep)
			wantShare:  0.5,
			wantAgents: []string{"bot", "copilot", "devin"},
		},
		{
			name: "legacy fallback threshold for no diff data",
			reviews: []provider.Review{
				{Repo: "acme/widgets", SubmittedAt: ts("2026-02-01T10:00:00Z"), State: "APPROVED", CommentCount: 3, PRLines: 0, AuthorClass: "bot"}, // deep (legacy >=3)
				{Repo: "acme/widgets", SubmittedAt: ts("2026-02-02T10:00:00Z"), State: "APPROVED", CommentCount: 2, PRLines: 0, AuthorClass: "bot"}, // shallow (legacy, 2<3)
			},
			wantTotal:  2,
			wantDeep:   1,
			wantShare:  0.5,
			wantAgents: []string{"bot"},
		},
		{
			name: "all size-normalized thresholds respected",
			reviews: []provider.Review{
				{Repo: "acme/widgets", SubmittedAt: ts("2026-02-01T10:00:00Z"), State: "APPROVED", CommentCount: 4, PRLines: 500, AuthorClass: "copilot"},   // threshold=5, shallow
				{Repo: "acme/widgets", SubmittedAt: ts("2026-02-02T10:00:00Z"), State: "APPROVED", CommentCount: 5, PRLines: 500, AuthorClass: "copilot"},   // threshold=5, deep
				{Repo: "acme/widgets", SubmittedAt: ts("2026-02-03T10:00:00Z"), State: "APPROVED", CommentCount: 10, PRLines: 2000, AuthorClass: "copilot"}, // threshold=10, deep
				{Repo: "acme/widgets", SubmittedAt: ts("2026-02-04T10:00:00Z"), State: "APPROVED", CommentCount: 9, PRLines: 2000, AuthorClass: "copilot"},  // threshold=10, shallow
			},
			wantTotal:  4,
			wantDeep:   2,
			wantShare:  0.5,
			wantAgents: []string{"copilot"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := metrics.Compute(provider.ActivitySet{ReviewsGiven: tt.reviews})
			got := res.Collaboration.AICollaboration
			if got == nil {
				t.Fatal("ai_collaboration stats not computed (nil)")
			}
			if got.Total != tt.wantTotal {
				t.Errorf("Total = %d, want %d", got.Total, tt.wantTotal)
			}
			if got.DeepReviewCount != tt.wantDeep {
				t.Errorf("DeepReviewCount = %d, want %d", got.DeepReviewCount, tt.wantDeep)
			}
			if got.DeepReviewShare != tt.wantShare {
				t.Errorf("DeepReviewShare = %f, want %f", got.DeepReviewShare, tt.wantShare)
			}
			if tt.wantAgents == nil {
				if got.RecognizedAgents != nil {
					t.Errorf("RecognizedAgents = %v, want nil", got.RecognizedAgents)
				}
			} else {
				if len(got.RecognizedAgents) != len(tt.wantAgents) {
					t.Errorf("RecognizedAgents = %v, want %v", got.RecognizedAgents, tt.wantAgents)
				} else {
					for i, a := range got.RecognizedAgents {
						if a != tt.wantAgents[i] {
							t.Errorf("RecognizedAgents[%d] = %q, want %q", i, a, tt.wantAgents[i])
						}
					}
				}
			}
		})
	}
}

// TestComputeAICollaborationNonNilForEmptyReviews verifies that the AI
// collaboration stats are populated (non-nil) even when there are no reviews
// at all, signalling that the section should be shown (zero-state).
func TestComputeAICollaborationNonNilForEmptyReviews(t *testing.T) {
	// No reviews at all
	res := metrics.Compute(provider.ActivitySet{ReviewsGiven: nil})
	got := res.Collaboration.AICollaboration
	if got == nil {
		t.Fatal("ai_collaboration should be non-nil even for empty reviews (zero-state)")
	}
	if got.Total != 0 {
		t.Errorf("Total = %d, want 0", got.Total)
	}
	if got.RecognizedAgents != nil {
		t.Errorf("RecognizedAgents = %v, want nil", got.RecognizedAgents)
	}

	// Reviews that are all human-authored
	res2 := metrics.Compute(provider.ActivitySet{
		ReviewsGiven: []provider.Review{
			{Repo: "acme/widgets", SubmittedAt: ts("2026-02-01T10:00:00Z"), State: "APPROVED", CommentCount: 3, AuthorClass: ""},
		},
	})
	got2 := res2.Collaboration.AICollaboration
	if got2 == nil {
		t.Fatal("ai_collaboration should be non-nil even for human-only reviews (zero-state)")
	}
	if got2.Total != 0 {
		t.Errorf("Total = %d, want 0", got2.Total)
	}
}

// TestComputeAICollaborationConsistencyWithOverallReviews verifies that
// the AI collaboration total never exceeds the overall reviews given total.
func TestComputeAICollaborationConsistencyWithOverallReviews(t *testing.T) {
	reviews := []provider.Review{
		{Repo: "acme/widgets", SubmittedAt: ts("2026-02-01T10:00:00Z"), State: "APPROVED", CommentCount: 5, PRLines: 500, AuthorClass: "copilot"},
		{Repo: "acme/widgets", SubmittedAt: ts("2026-02-02T10:00:00Z"), State: "APPROVED", CommentCount: 3, PRLines: 10, AuthorClass: ""}, // human
		{Repo: "acme/widgets", SubmittedAt: ts("2026-02-03T10:00:00Z"), State: "APPROVED", CommentCount: 2, PRLines: 10, AuthorClass: "bot"},
	}
	res := metrics.Compute(provider.ActivitySet{ReviewsGiven: reviews})

	overallReviews := res.Collaboration.ReviewsGiven
	aiReviews := res.Collaboration.AICollaboration

	if overallReviews == nil {
		t.Fatal("reviews_given not computed")
	}
	if aiReviews == nil {
		t.Fatal("ai_collaboration not computed")
	}

	if aiReviews.Total > overallReviews.Total {
		t.Errorf("AI total (%d) exceeds overall reviews total (%d)", aiReviews.Total, overallReviews.Total)
	}
	if aiReviews.DeepReviewCount > overallReviews.DeepReviewCount {
		t.Errorf("AI deep review count (%d) exceeds overall deep review count (%d)", aiReviews.DeepReviewCount, overallReviews.DeepReviewCount)
	}
}

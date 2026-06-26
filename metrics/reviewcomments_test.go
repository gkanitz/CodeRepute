package metrics_test

import (
	"testing"

	"github.com/gkanitz/coderepute/metrics"
	"github.com/gkanitz/coderepute/provider"
)

func TestComputeReviewComments(t *testing.T) {
	tests := []struct {
		name         string
		written      []provider.ReviewComment
		received     []provider.ReviewComment
		wantWritten  int
		wantReceived int
	}{
		{
			name: "empty window yields explicit zeros",
		},
		{
			name: "written and received counted separately",
			written: []provider.ReviewComment{
				{Repo: "acme/widgets", CreatedAt: ts("2026-02-01T10:00:00Z")},
				{Repo: "acme/gears", CreatedAt: ts("2026-02-02T10:00:00Z")},
				{Repo: "acme/gears", CreatedAt: ts("2026-02-03T10:00:00Z")},
			},
			received: []provider.ReviewComment{
				{Repo: "acme/widgets", CreatedAt: ts("2026-02-04T10:00:00Z")},
			},
			wantWritten:  3,
			wantReceived: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := metrics.Compute(provider.ActivitySet{
				ReviewCommentsWritten:  tt.written,
				ReviewCommentsReceived: tt.received,
			})
			got := res.Collaboration.ReviewComments
			if got == nil {
				t.Fatal("review comment stats not computed")
			}
			if got.Written != tt.wantWritten || got.Received != tt.wantReceived {
				t.Errorf("got written=%d received=%d, want written=%d received=%d",
					got.Written, got.Received, tt.wantWritten, tt.wantReceived)
			}
		})
	}
}

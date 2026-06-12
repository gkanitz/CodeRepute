package metrics_test

import (
	"testing"

	"github.com/grkanitz/coderepute/metrics"
	"github.com/grkanitz/coderepute/provider"
)

// window returns the half-open [since, until) coverage window.
func window(since, until string) provider.Window {
	return provider.Window{Since: ts(since), Until: ts(until)}
}

func TestComputeCadenceActiveDaysAndContributions(t *testing.T) {
	tests := []struct {
		name              string
		as                provider.ActivitySet
		wantActiveDays    int
		wantContributions int
	}{
		{
			name:              "empty activity",
			as:                provider.ActivitySet{Window: window("2026-01-01T00:00:00Z", "2026-04-01T00:00:00Z")},
			wantActiveDays:    0,
			wantContributions: 0,
		},
		{
			name: "events across kinds count as contributions",
			as: provider.ActivitySet{
				Window: window("2026-01-01T00:00:00Z", "2026-04-01T00:00:00Z"),
				PullRequests: []provider.PullRequest{
					{Repo: "acme/widgets", CreatedAt: ts("2026-01-10T09:00:00Z")},
				},
				ReviewsGiven: []provider.Review{
					{Repo: "acme/widgets", SubmittedAt: ts("2026-02-03T10:00:00Z")},
				},
				ReviewCommentsWritten: []provider.ReviewComment{
					{Repo: "acme/widgets", CreatedAt: ts("2026-02-03T11:00:00Z")},
				},
			},
			wantActiveDays:    2, // Jan 10 and Feb 3
			wantContributions: 3,
		},
		{
			name: "several events on the same UTC day are one active day",
			as: provider.ActivitySet{
				Window: window("2026-01-01T00:00:00Z", "2026-02-01T00:00:00Z"),
				PullRequests: []provider.PullRequest{
					{Repo: "acme/widgets", CreatedAt: ts("2026-01-10T09:00:00Z")},
					{Repo: "acme/gears", CreatedAt: ts("2026-01-10T17:30:00Z")},
				},
			},
			wantActiveDays:    1,
			wantContributions: 2,
		},
		{
			name: "comments received are not the subject's contributions",
			as: provider.ActivitySet{
				Window: window("2026-01-01T00:00:00Z", "2026-02-01T00:00:00Z"),
				ReviewCommentsReceived: []provider.ReviewComment{
					{Repo: "acme/widgets", CreatedAt: ts("2026-01-10T09:00:00Z")},
				},
			},
			wantActiveDays:    0,
			wantContributions: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := metrics.Compute(tt.as)
			got := res.Cadence
			if got.ActiveDays != tt.wantActiveDays || got.Contributions != tt.wantContributions {
				t.Errorf("got active_days=%d contributions=%d, want active_days=%d contributions=%d",
					got.ActiveDays, got.Contributions, tt.wantActiveDays, tt.wantContributions)
			}
		})
	}
}

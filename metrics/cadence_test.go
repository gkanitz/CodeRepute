package metrics_test

import (
	"reflect"
	"testing"

	"github.com/grkanitz/coderepute/metrics"
	"github.com/grkanitz/coderepute/provider"
	"github.com/grkanitz/coderepute/report"
)

// window returns the half-open [since, until) coverage window.
func window(since, until string) provider.Window {
	return provider.Window{Since: ts(since), Until: ts(until)}
}

func TestComputeCadenceMonthlyTrend(t *testing.T) {
	as := provider.ActivitySet{
		Window: window("2026-01-01T00:00:00Z", "2026-04-01T00:00:00Z"),
		PullRequests: []provider.PullRequest{
			{Repo: "acme/widgets", CreatedAt: ts("2026-01-10T09:00:00Z")},
			{Repo: "acme/widgets", CreatedAt: ts("2026-01-20T09:00:00Z")},
			{Repo: "acme/gears", CreatedAt: ts("2026-03-05T09:00:00Z")},
		},
		ReviewsGiven: []provider.Review{
			{Repo: "acme/widgets", SubmittedAt: ts("2026-03-06T10:00:00Z")},
		},
	}

	got := metrics.Compute(as).Cadence.Trend
	want := []report.TrendBucket{
		{
			Start:  ts("2026-01-01T00:00:00Z"),
			End:    ts("2026-02-01T00:00:00Z"),
			Counts: map[string]int{"pull_requests": 2},
		},
		{
			Start:  ts("2026-02-01T00:00:00Z"),
			End:    ts("2026-03-01T00:00:00Z"),
			Counts: map[string]int{},
		},
		{
			Start:  ts("2026-03-01T00:00:00Z"),
			End:    ts("2026-04-01T00:00:00Z"),
			Counts: map[string]int{"pull_requests": 1, "reviews_given": 1},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("trend mismatch\n got: %+v\nwant: %+v", got, want)
	}
}

func TestComputeCadenceTrendBucketBoundaries(t *testing.T) {
	tests := []struct {
		name string
		as   provider.ActivitySet
		want []report.TrendBucket
	}{
		{
			name: "mid-month window yields partial first and last buckets",
			as: provider.ActivitySet{
				Window: window("2026-01-15T00:00:00Z", "2026-03-10T00:00:00Z"),
				PullRequests: []provider.PullRequest{
					{Repo: "acme/widgets", CreatedAt: ts("2026-01-20T09:00:00Z")},
					{Repo: "acme/widgets", CreatedAt: ts("2026-03-05T09:00:00Z")},
				},
			},
			want: []report.TrendBucket{
				{Start: ts("2026-01-15T00:00:00Z"), End: ts("2026-02-01T00:00:00Z"), Counts: map[string]int{"pull_requests": 1}},
				{Start: ts("2026-02-01T00:00:00Z"), End: ts("2026-03-01T00:00:00Z"), Counts: map[string]int{}},
				{Start: ts("2026-03-01T00:00:00Z"), End: ts("2026-03-10T00:00:00Z"), Counts: map[string]int{"pull_requests": 1}},
			},
		},
		{
			name: "events bucket by UTC instant regardless of source offset",
			as: provider.ActivitySet{
				Window: window("2026-01-01T00:00:00Z", "2026-03-01T00:00:00Z"),
				PullRequests: []provider.PullRequest{
					// 2026-02-01T02:00+05:30 is 2026-01-31T20:30Z: January.
					{Repo: "acme/widgets", CreatedAt: ts("2026-02-01T02:00:00+05:30")},
					// 2026-01-31T20:00-05:00 is 2026-02-01T01:00Z: February.
					{Repo: "acme/widgets", CreatedAt: ts("2026-01-31T20:00:00-05:00")},
				},
			},
			want: []report.TrendBucket{
				{Start: ts("2026-01-01T00:00:00Z"), End: ts("2026-02-01T00:00:00Z"), Counts: map[string]int{"pull_requests": 1}},
				{Start: ts("2026-02-01T00:00:00Z"), End: ts("2026-03-01T00:00:00Z"), Counts: map[string]int{"pull_requests": 1}},
			},
		},
		{
			name: "boundary instants: bucket starts inclusive, window until exclusive",
			as: provider.ActivitySet{
				Window: window("2026-01-01T00:00:00Z", "2026-03-01T00:00:00Z"),
				PullRequests: []provider.PullRequest{
					{Repo: "acme/widgets", CreatedAt: ts("2026-01-01T00:00:00Z")}, // window since: included
					{Repo: "acme/widgets", CreatedAt: ts("2026-02-01T00:00:00Z")}, // month boundary: second bucket
					{Repo: "acme/widgets", CreatedAt: ts("2026-03-01T00:00:00Z")}, // window until: excluded
				},
			},
			want: []report.TrendBucket{
				{Start: ts("2026-01-01T00:00:00Z"), End: ts("2026-02-01T00:00:00Z"), Counts: map[string]int{"pull_requests": 1}},
				{Start: ts("2026-02-01T00:00:00Z"), End: ts("2026-03-01T00:00:00Z"), Counts: map[string]int{"pull_requests": 1}},
			},
		},
		{
			name: "window shorter than a month is a single partial bucket",
			as: provider.ActivitySet{
				Window: window("2026-01-10T00:00:00Z", "2026-01-20T00:00:00Z"),
				PullRequests: []provider.PullRequest{
					{Repo: "acme/widgets", CreatedAt: ts("2026-01-12T09:00:00Z")},
				},
			},
			want: []report.TrendBucket{
				{Start: ts("2026-01-10T00:00:00Z"), End: ts("2026-01-20T00:00:00Z"), Counts: map[string]int{"pull_requests": 1}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := metrics.Compute(tt.as).Cadence.Trend
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("trend mismatch\n got: %+v\nwant: %+v", got, tt.want)
			}
		})
	}
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

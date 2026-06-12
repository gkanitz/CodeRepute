package metrics

import (
	"time"

	"github.com/grkanitz/coderepute/provider"
	"github.com/grkanitz/coderepute/report"
)

// monthlyTrend slices the half-open coverage window into UTC calendar-month
// buckets and counts events per series in each. The first and last buckets
// are clamped to the window, so they may be partial months. Bucketing is
// generic over events: any timestamped series flattened by subjectEvents
// (or, in later slices, collaboration series) lands in the right bucket
// without changes here.
func monthlyTrend(w provider.Window, evs []event) []report.TrendBucket {
	since, until := w.Since.UTC(), w.Until.UTC()

	var buckets []report.TrendBucket
	for start := since; start.Before(until); {
		end := startOfNextMonth(start)
		if end.After(until) {
			end = until
		}
		buckets = append(buckets, report.TrendBucket{
			Start:  start,
			End:    end,
			Counts: map[string]int{},
		})
		start = end
	}

	for _, e := range evs {
		for i := range buckets {
			if !e.at.Before(buckets[i].Start) && e.at.Before(buckets[i].End) {
				buckets[i].Counts[e.series]++
				break
			}
		}
	}
	return buckets
}

// startOfNextMonth returns the first instant of the UTC calendar month
// after t.
func startOfNextMonth(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, time.UTC)
}

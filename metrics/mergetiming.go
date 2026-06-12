package metrics

import (
	"sort"

	"github.com/grkanitz/coderepute/provider"
	"github.com/grkanitz/coderepute/report"
)

func init() {
	Register("time_to_merge", computeTimeToMerge)
}

// computeTimeToMerge summarizes how long the subject's PRs took from
// creation to merge. Only merged PRs contribute; with no merged PRs in
// the window the stat is omitted rather than reported as zero.
func computeTimeToMerge(as provider.ActivitySet, res *Result) {
	var hours []float64
	for _, pr := range as.PullRequests {
		if pr.MergedAt == nil {
			continue
		}
		hours = append(hours, pr.MergedAt.Sub(pr.CreatedAt).Hours())
	}
	if len(hours) == 0 {
		return
	}
	res.Collaboration.TimeToMerge = &report.DurationStats{
		Count:       len(hours),
		MedianHours: median(hours),
	}
}

// median returns the median of a non-empty sample, averaging the middle
// pair for even-sized samples. It sorts its argument in place.
func median(sample []float64) float64 {
	sort.Float64s(sample)
	mid := len(sample) / 2
	if len(sample)%2 == 1 {
		return sample[mid]
	}
	return (sample[mid-1] + sample[mid]) / 2
}

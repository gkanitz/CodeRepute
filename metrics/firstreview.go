package metrics

import (
	"github.com/grkanitz/coderepute/provider"
	"github.com/grkanitz/coderepute/report"
)

func init() {
	Register("time_to_first_review", computeTimeToFirstReview)
}

// computeTimeToFirstReview summarizes how long the subject's PRs waited
// for their first review by someone else. PRs that never received a
// review do not contribute; with no reviewed PRs the stat is omitted.
func computeTimeToFirstReview(as provider.ActivitySet, res *Result) {
	var hours []float64
	for _, pr := range as.PullRequests {
		if pr.FirstReviewAt == nil {
			continue
		}
		hours = append(hours, pr.FirstReviewAt.Sub(pr.CreatedAt).Hours())
	}
	if len(hours) == 0 {
		return
	}
	res.Collaboration.TimeToFirstReview = &report.DurationStats{
		Count:       len(hours),
		MedianHours: median(hours),
	}
}

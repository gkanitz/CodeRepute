package metrics

import (
	"github.com/gkanitz/coderepute/provider"
	"github.com/gkanitz/coderepute/report"
)

func init() {
	Register("reviews_given", computeReviewsGiven)
}

// computeReviewsGiven counts the reviews the subject submitted on other
// people's PRs, by outcome. Zero counts are meaningful, so the stats are
// always present.
func computeReviewsGiven(as provider.ActivitySet, res *Result) {
	stats := report.ReviewStats{Total: len(as.ReviewsGiven)}
	for _, rv := range as.ReviewsGiven {
		switch rv.State {
		case "APPROVED":
			stats.Approvals++
		case "CHANGES_REQUESTED":
			stats.ChangesRequested++
		}
		if rv.CommentCount >= 3 {
			stats.DeepReviewCount++
		}
	}
	res.Collaboration.ReviewsGiven = &stats
}

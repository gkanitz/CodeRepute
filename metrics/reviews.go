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
//
// Deep-review classification uses a size-normalized threshold when the
// reviewed PR has diff-shape data (PRLines > 0):
//
//	deep ⇔ comments ≥ clamp(ceil(lines/100), 3, 10)
//
// Reviews without diff data fall back to the legacy absolute ≥3 threshold.
func computeReviewsGiven(as provider.ActivitySet, res *Result) {
	stats := report.ReviewStats{Total: len(as.ReviewsGiven)}
	depth := report.DepthBasis{}
	for _, rv := range as.ReviewsGiven {
		switch rv.State {
		case "APPROVED":
			stats.Approvals++
		case "CHANGES_REQUESTED":
			stats.ChangesRequested++
		}

		threshold := deepReviewThreshold(rv.PRLines)
		if rv.CommentCount >= threshold {
			stats.DeepReviewCount++
		}

		if rv.PRLines > 0 {
			depth.Measured++
		} else {
			depth.Fallback++
		}
	}
	if depth.Measured > 0 || depth.Fallback > 0 {
		stats.DepthBasis = &depth
	}
	res.Collaboration.ReviewsGiven = &stats
}

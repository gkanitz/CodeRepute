package metrics

import (
	"github.com/grkanitz/coderepute/provider"
	"github.com/grkanitz/coderepute/report"
)

func init() {
	Register("rework", computeReworkShare)
}

// computeReworkShare reports the share of the subject's reviewed PRs that
// went through a rework cycle (a changes-requested review). PRs that
// never received a review are excluded from the denominator; with no
// reviewed PRs in the window the stat is omitted.
func computeReworkShare(as provider.ActivitySet, res *Result) {
	var reviewed, reworked int
	for _, pr := range as.PullRequests {
		if pr.FirstReviewAt == nil {
			continue
		}
		reviewed++
		if pr.ChangesRequested > 0 {
			reworked++
		}
	}
	if reviewed == 0 {
		return
	}
	res.Collaboration.Rework = &report.ReworkStats{
		ReviewedPRs: reviewed,
		ReworkedPRs: reworked,
		Share:       float64(reworked) / float64(reviewed),
	}
}

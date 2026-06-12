package metrics

import (
	"github.com/grkanitz/coderepute/provider"
	"github.com/grkanitz/coderepute/report"
)

func init() {
	Register("review_comments", computeReviewComments)
}

// computeReviewComments counts review comments the subject wrote and
// received. Zero counts are meaningful, so the stats are always present.
func computeReviewComments(as provider.ActivitySet, res *Result) {
	res.Collaboration.ReviewComments = &report.ReviewCommentStats{
		Written:  len(as.ReviewCommentsWritten),
		Received: len(as.ReviewCommentsReceived),
	}
}

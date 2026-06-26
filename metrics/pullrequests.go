package metrics

import (
	"github.com/gkanitz/coderepute/provider"
	"github.com/gkanitz/coderepute/report"
)

func init() {
	Register("pull_requests", computePullRequests)
}

func computePullRequests(as provider.ActivitySet, res *Result) {
	stats := report.PullRequestStats{Authored: len(as.PullRequests)}
	for _, pr := range as.PullRequests {
		if pr.MergedAt != nil {
			stats.Merged++
		}
	}
	res.Collaboration.PullRequests = &stats
}

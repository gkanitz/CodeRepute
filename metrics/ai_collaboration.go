package metrics

import (
	"sort"

	"github.com/gkanitz/coderepute/provider"
	"github.com/gkanitz/coderepute/report"
)

func init() {
	Register("ai_collaboration", computeAICollaboration)
}

// computeAICollaboration counts reviews the subject gave on AI/bot-authored
// PRs and how many of those were deep reviews, using the same size-normalized
// threshold as the overall deep-review metric (deepReviewThreshold). It also
// collects the unique set of recognized agent IDs found among the reviewed
// PR authors in this window.
//
// A review is counted as AI/bot-authored when the reviewed PR's author has
// a non-empty AuthorClass string (set by the adapter's recognition ruleset).
// Reviews on human-authored PRs (AuthorClass "") are excluded entirely.
func computeAICollaboration(as provider.ActivitySet, res *Result) {
	var total, deep int
	seen := make(map[string]bool)

	for _, rv := range as.ReviewsGiven {
		if rv.AuthorClass == "" {
			continue // human-authored PR, not counted
		}
		total++

		threshold := deepReviewThreshold(rv.PRLines)
		if rv.CommentCount >= threshold {
			deep++
		}

		if rv.AuthorClass != "" {
			seen[rv.AuthorClass] = true
		}
	}

	if total == 0 {
		// Zero-state: still populate to signal the section was checked.
		// RecognizedAgents is nil (no agents found in an empty window).
		res.Collaboration.AICollaboration = &report.AICollaborationStats{
			Total:            0,
			DeepReviewCount:  0,
			DeepReviewShare:  0,
			RecognizedAgents: nil,
		}
		return
	}

	agents := make([]string, 0, len(seen))
	for a := range seen {
		agents = append(agents, a)
	}
	sort.Strings(agents)

	var share float64
	if total > 0 {
		share = float64(deep) / float64(total)
	}

	res.Collaboration.AICollaboration = &report.AICollaborationStats{
		Total:            total,
		DeepReviewCount:  deep,
		DeepReviewShare:  share,
		RecognizedAgents: agents,
	}
}

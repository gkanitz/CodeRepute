package metrics

import (
	"time"

	"github.com/grkanitz/coderepute/provider"
)

func init() {
	Register("cadence", computeCadence)
}

// event is one timestamped action the subject took, tagged with the name
// of the series it belongs to. Cadence is generic over events: new series
// (e.g. collaboration metrics landing in later slices) extend cadence by
// extending subjectEvents, not by changing the computation.
type event struct {
	series string
	at     time.Time
}

// subjectEvents flattens the subject's own timestamped actions out of the
// activity set, keeping only those inside the half-open coverage window.
// Activity directed at the subject (e.g. comments received) is not a
// contribution and is excluded.
func subjectEvents(as provider.ActivitySet) []event {
	inWindow := func(t time.Time) bool {
		return !t.Before(as.Window.Since) && t.Before(as.Window.Until)
	}
	var evs []event
	add := func(series string, t time.Time) {
		if inWindow(t) {
			evs = append(evs, event{series: series, at: t.UTC()})
		}
	}
	for _, pr := range as.PullRequests {
		add("pull_requests", pr.CreatedAt)
	}
	for _, rv := range as.ReviewsGiven {
		add("reviews_given", rv.SubmittedAt)
	}
	for _, c := range as.ReviewCommentsWritten {
		add("review_comments_written", c.CreatedAt)
	}
	return evs
}

func computeCadence(as provider.ActivitySet, res *Result) {
	evs := subjectEvents(as)

	days := map[string]struct{}{}
	for _, e := range evs {
		days[e.at.Format("2006-01-02")] = struct{}{}
	}

	res.Cadence.ActiveDays = len(days)
	res.Cadence.Contributions = len(evs)
	res.Cadence.Trend = monthlyTrend(as.Window, evs)
}

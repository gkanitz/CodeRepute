// Package render narrative rules derive front-page narrative sentences and
// interviewer probes from attested report data. Every rule has a stable rule
// ID and a pure (report -> sentence) function.
package render

import (
	"fmt"
	"math"
	"sort"

	"github.com/gkanitz/coderepute/report"
)

// Rule IDs — stable identifiers used in the derivation annex.
const (
	RuleProfileMix      = "profile-mix"
	RuleTrajectory      = "trajectory"
	RuleProbeDeepReview = "probe-deep-review"
	RuleProbeRework     = "probe-rework"
	RuleProbeTTM        = "probe-ttm"
	RuleProbeCadence    = "probe-cadence"
	RuleProbeLanguage   = "probe-language"
)

// NarrativeSection holds all rendered content for the narrative front page
// section and its derivation annex.
type NarrativeSection struct {
	ProfileSentence    string
	TrajectorySentence string
	Probes             []Probe
	AnnexEntries       []AnnexEntry
}

// Probe is one interviewer question derived from attested data.
type Probe struct {
	Question string `json:"question"`
	RuleID   string `json:"rule_id"`
}

// AnnexEntry records a rule that fired and a one-line explanation.
type AnnexEntry struct {
	RuleID      string `json:"rule_id"`
	Explanation string `json:"explanation"`
}

// narrativeBuild constructs the full NarrativeSection from the report struct.
func narrativeBuild(r report.Report) NarrativeSection {
	var ns NarrativeSection

	// Profile-mix rule
	sentence, ok := profileMixRule(r)
	if ok {
		ns.ProfileSentence = sentence
		ns.AnnexEntries = append(ns.AnnexEntries, AnnexEntry{
			RuleID:      RuleProfileMix,
			Explanation: "Compares total reviews given to authored pull requests to determine whether the profile is review-weighted, authoring-weighted, or balanced.",
		})
	}

	// Trajectory rule
	sentence, ok = trajectoryRule(r)
	if ok {
		ns.TrajectorySentence = sentence
		ns.AnnexEntries = append(ns.AnnexEntries, AnnexEntry{
			RuleID:      RuleTrajectory,
			Explanation: "Compares the review-to-PR ratio in the first half of the coverage window to the second half, flagging shifts above 0.30.",
		})
	}

	// Interviewer probes — exactly three, fewer only when fewer data sources exist
	ns.Probes = buildProbes(r)
	for _, p := range ns.Probes {
		ns.AnnexEntries = append(ns.AnnexEntries, AnnexEntry{
			RuleID:      p.RuleID,
			Explanation: probeExplanation(p.RuleID),
		})
	}

	return ns
}

// profileMixRule computes the profile-mix sentence.
// Returns (sentence, true) when data is sufficient, ("", false) when the
// comparison cannot be made.
//
// Rule profile-mix: r = reviews_given.total / pull_requests.authored.
// r > 1.5 — review-weighted phrasing; r < 0.67 — authoring-weighted;
// otherwise balanced. When authored == 0 the comparison is skipped entirely.
func profileMixRule(r report.Report) (string, bool) {
	if r.Collaboration == nil || r.Collaboration.PullRequests == nil || r.Collaboration.ReviewsGiven == nil {
		return "", false
	}
	authored := r.Collaboration.PullRequests.Authored
	reviews := r.Collaboration.ReviewsGiven.Total

	if authored == 0 {
		return "", false
	}

	ratio := float64(reviews) / float64(authored)

	if ratio > 1.5 {
		return fmt.Sprintf(
			"This developer has given %d reviews across %d authored pull requests — a review-to-PR ratio of %.1f, indicating a review-weighted profile.",
			reviews, authored, ratio,
		), true
	} else if ratio < 0.67 {
		return fmt.Sprintf(
			"This developer has authored %d pull requests and given %d reviews — a review-to-PR ratio of %.1f, indicating an authoring-weighted profile.",
			authored, reviews, ratio,
		), true
	}
	return fmt.Sprintf(
		"This developer has authored %d pull requests and given %d reviews — a balanced authoring-to-reviewing profile.",
		authored, reviews,
	), true
}

// trajectoryRule computes the trajectory sentence.
// Splits trend buckets into first/second half, computes the review-to-PR ratio
// per half, and checks whether the relative shift exceeds the 0.30 threshold.
// Skipped when either half has fewer than 10 combined events.
//
// Rule trajectory: shift > +0.30 — "shifted toward reviewing";
// shift < −0.30 — "shifted toward authoring".
func trajectoryRule(r report.Report) (string, bool) {
	if r.Cadence == nil || len(r.Cadence.Trend) < 2 {
		return "", false
	}

	buckets := r.Cadence.Trend
	mid := len(buckets) / 2
	firstHalf := buckets[:mid]
	secondHalf := buckets[mid:]

	firstPRs, firstReviews := sumTrendEvents(firstHalf)
	secondPRs, secondReviews := sumTrendEvents(secondHalf)

	firstTotal := firstPRs + firstReviews
	secondTotal := secondPRs + secondReviews

	if firstTotal < 10 || secondTotal < 10 {
		return "", false
	}

	// Cannot compute a ratio when either half has no authored PRs.
	if firstPRs == 0 || secondPRs == 0 {
		return "", false
	}

	firstRatio := float64(firstReviews) / float64(firstPRs)
	secondRatio := float64(secondReviews) / float64(secondPRs)

	shift := secondRatio - firstRatio

	if shift > 0.30 {
		return fmt.Sprintf(
			"Over the coverage window, the ratio of reviews to authored pull requests shifted from %.1f to %.1f, reflecting a shift toward reviewing.",
			firstRatio, secondRatio,
		), true
	} else if shift < -0.30 {
		return fmt.Sprintf(
			"Over the coverage window, the ratio of reviews to authored pull requests shifted from %.1f to %.1f, reflecting a shift toward authoring.",
			firstRatio, secondRatio,
		), true
	}

	return "", false
}

// sumTrendEvents sums pull_requests and reviews_given counts across a slice
// of trend buckets.
func sumTrendEvents(buckets []report.TrendBucket) (prs, reviews int) {
	for _, b := range buckets {
		prs += b.Counts["pull_requests"]
		reviews += b.Counts["reviews_given"]
	}
	return
}

// buildProbes selects up to three interviewer probes in priority order.
// Priority: (1) deep reviews, (2) rework, (3) time-to-merge, (4) cadence
// trend, (5) language mix. The first three whose data exists are returned;
// fewer only when fewer exist.
func buildProbes(r report.Report) []Probe {
	type probeFn func(report.Report) (Probe, bool)
	candidates := []probeFn{
		deepReviewProbe,
		reworkProbe,
		ttmProbe,
		cadenceProbe,
		languageProbe,
	}

	var probes []Probe
	for _, fn := range candidates {
		if len(probes) >= 3 {
			break
		}
		if p, ok := fn(r); ok {
			probes = append(probes, p)
		}
	}
	if len(probes) == 0 {
		return nil
	}
	return probes
}

// deepReviewProbe generates a probe about review depth.
func deepReviewProbe(r report.Report) (Probe, bool) {
	if r.Collaboration == nil || r.Collaboration.ReviewsGiven == nil {
		return Probe{}, false
	}
	rv := r.Collaboration.ReviewsGiven
	if rv.Total == 0 || rv.DeepReviewCount == 0 {
		return Probe{}, false
	}
	pct := int(math.Round(float64(rv.DeepReviewCount) / float64(rv.Total) * 100))
	q := fmt.Sprintf(
		"This developer left deep reviews on %d of %d reviews (%d%%). Ask: What does a thorough review look like for you? How do you decide when to request changes versus approve?",
		rv.DeepReviewCount, rv.Total, pct,
	)
	return Probe{Question: q, RuleID: RuleProbeDeepReview}, true
}

// reworkProbe generates a probe about rework handling.
func reworkProbe(r report.Report) (Probe, bool) {
	if r.Collaboration == nil || r.Collaboration.Rework == nil {
		return Probe{}, false
	}
	rw := r.Collaboration.Rework
	if rw.ReviewedPRs == 0 {
		return Probe{}, false
	}
	pct := int(math.Round(rw.Share * 100))
	q := fmt.Sprintf(
		"%d of %d reviewed PRs (%d%%) required a rework cycle. Ask: How do you handle revision requests? Do you prefer to iterate on the same PR or open a fresh one?",
		rw.ReworkedPRs, rw.ReviewedPRs, pct,
	)
	return Probe{Question: q, RuleID: RuleProbeRework}, true
}

// ttmProbe generates a probe about time-to-merge and PR scoping.
func ttmProbe(r report.Report) (Probe, bool) {
	if r.Collaboration == nil || r.Collaboration.TimeToMerge == nil {
		return Probe{}, false
	}
	ttm := r.Collaboration.TimeToMerge
	if ttm.Count == 0 {
		return Probe{}, false
	}
	h := math.Round(ttm.MedianHours*10) / 10
	q := fmt.Sprintf(
		"Median time to merge is %.1f hours across %d PRs. Ask: How do you scope your pull requests? Do you aim for small, focused changes or larger feature-complete PRs?",
		h, ttm.Count,
	)
	return Probe{Question: q, RuleID: RuleProbeTTM}, true
}

// cadenceProbe generates a probe about activity consistency.
func cadenceProbe(r report.Report) (Probe, bool) {
	if r.Cadence == nil || len(r.Cadence.Trend) == 0 {
		return Probe{}, false
	}
	q := fmt.Sprintf(
		"Activity spanned %d active days with %d contributions across the coverage window. Ask: How do you maintain consistency? Do you work in focused sprints or steady daily increments?",
		r.Cadence.ActiveDays, r.Cadence.Contributions,
	)
	return Probe{Question: q, RuleID: RuleProbeCadence}, true
}

// languageProbe generates a probe about depth in the most-used language.
func languageProbe(r report.Report) (Probe, bool) {
	if r.Collaboration == nil || r.Collaboration.LanguageMix == nil {
		return Probe{}, false
	}
	lm := r.Collaboration.LanguageMix
	if len(lm.Languages) == 0 {
		return Probe{}, false
	}
	// Languages are already sorted descending by share; first is the largest.
	lang := lm.Languages[0]
	q := fmt.Sprintf(
		"The most-used language by diff volume is %s at %.0f%%. Ask: How deep is your expertise in %s? Do you handle the full stack or specialize in specific areas?",
		lang.Name, lang.SharePct, lang.Name,
	)
	return Probe{Question: q, RuleID: RuleProbeLanguage}, true
}

// probeExplanation returns a one-line explanation for a probe rule ID.
func probeExplanation(ruleID string) string {
	switch ruleID {
	case RuleProbeDeepReview:
		return "Generates an interviewer probe when the report contains deep review data (reviews with at least three inline comments)."
	case RuleProbeRework:
		return "Generates an interviewer probe when the report contains rework rate data."
	case RuleProbeTTM:
		return "Generates an interviewer probe when the report contains time-to-merge data."
	case RuleProbeCadence:
		return "Generates an interviewer probe when the report contains cadence trend data."
	case RuleProbeLanguage:
		return "Generates an interviewer probe when the report contains language mix data."
	default:
		return ""
	}
}

// narrativeProbes returns the probes for template rendering.
func narrativeProbes(r report.Report) []Probe {
	return buildProbes(r)
}

// narrativeAnnex returns the annex entries for template rendering.
func narrativeAnnex(r report.Report) []AnnexEntry {
	ns := narrativeBuild(r)
	sort.Slice(ns.AnnexEntries, func(i, j int) bool {
		return ns.AnnexEntries[i].RuleID < ns.AnnexEntries[j].RuleID
	})
	return ns.AnnexEntries
}

package metrics

import (
	"math"
	"sort"

	"github.com/gkanitz/coderepute/provider"
	"github.com/gkanitz/coderepute/report"
)

func init() {
	Register("pr_size", computePRSize)
}

// prSize holds the total lines and file count for one merged PR with diff data.
type prSize struct {
	lines int
	files int
}

// computePRSize computes PR-size stats over the subject's merged PRs that
// have diff-shape data (Additions+Deletions > 0). Suppressed (omitted) when
// fewer than 5 merged PRs have diff data.
func computePRSize(as provider.ActivitySet, res *Result) {
	// Collect total lines and file counts for merged PRs with diff data.
	var sizes []prSize
	for _, pr := range as.PullRequests {
		if pr.MergedAt == nil {
			continue
		}
		lines := pr.Additions + pr.Deletions
		if lines == 0 {
			continue
		}
		sizes = append(sizes, prSize{lines: lines, files: pr.Files})
	}

	if len(sizes) < 5 {
		if len(sizes) > 0 {
			res.Collaboration.Suppressed = append(res.Collaboration.Suppressed, report.SuppressedEntry{
				Section: "pr_size",
				Reason:  "sample too small: only " + plural(len(sizes), "merged PR") + " with diff data (need ≥ 5)",
			})
		}
		return
	}

	// Sort by lines for median computation.
	sort.Slice(sizes, func(i, j int) bool { return sizes[i].lines < sizes[j].lines })

	medianLines := medianInts(lines(sizes))
	filesMedian := medianInts(files(sizes))

	smallThreshold := 400
	smallCount := 0
	for _, s := range sizes {
		if s.lines <= smallThreshold {
			smallCount++
		}
	}

	res.Collaboration.PRSize = &report.PRSizeStats{
		Count:               len(sizes),
		MedianLines:         medianLines,
		FilesMedian:         filesMedian,
		SmallShare:          float64(smallCount) / float64(len(sizes)),
		SmallThresholdLines: smallThreshold,
	}
}

// lines extracts the lines field from a slice of prSize.
func lines(s []prSize) []int {
	out := make([]int, len(s))
	for i, v := range s {
		out[i] = v.lines
	}
	return out
}

// files extracts the files field from a slice of prSize.
func files(s []prSize) []int {
	out := make([]int, len(s))
	for i, v := range s {
		out[i] = v.files
	}
	return out
}

// medianInts returns the median of a sorted int slice. For even-length slices
// it returns the mean of the two middle values.
func medianInts(vals []int) float64 {
	n := len(vals)
	if n == 0 {
		return 0
	}
	if n%2 == 1 {
		return float64(vals[n/2])
	}
	return float64(vals[n/2-1]+vals[n/2]) / 2.0
}

// deepReviewThreshold returns the minimum comment count for a review to be
// considered "deep" on a PR with the given total lines. Formula:
// clamp(ceil(lines/100), 3, 10). When lines is 0 (no diff data), returns 3
// (legacy fallback).
func deepReviewThreshold(lines int) int {
	if lines <= 0 {
		return 3
	}
	t := int(math.Ceil(float64(lines) / 100.0))
	if t < 3 {
		t = 3
	}
	if t > 10 {
		t = 10
	}
	return t
}

// plural returns a count string with a singular or plural noun, avoiding
// "1" as the count when count is 1 (e.g. "1 merged PR" not "1 merged PRs").
func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return itoa(n) + " " + noun + "s"
}

// itoa is a simple int-to-string for small numbers, avoiding strconv import.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

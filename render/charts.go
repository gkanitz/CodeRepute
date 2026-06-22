package render

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/grkanitz/coderepute/report"
)

// ChartBucket holds the data for one time bucket across all three stacked
// series (PRs merged, shallow reviews, deep reviews) and two comment series.
type ChartBucket struct {
	Label            string
	PRs              int
	ShallowReviews   int
	DeepReviews      int
	CommentsWritten  int
	CommentsReceived int
}

// buildChartBuckets converts raw TrendBucket data into ChartBuckets using the
// overall deep-review ratio as an approximation for per-bucket review splitting.
func buildChartBuckets(trend []report.TrendBucket, totalReviews, deepReviews int) []ChartBucket {
	deepRatio := 0.0
	if totalReviews > 0 {
		deepRatio = float64(deepReviews) / float64(totalReviews)
	}

	var out []ChartBucket
	for _, b := range trend {
		label := bucketLabel(b.Start, b.End)
		prs := b.Counts["pull_requests"]
		reviews := b.Counts["reviews_given"]
		deep := int(math.Round(float64(reviews) * deepRatio))
		shallow := reviews - deep
		if shallow < 0 {
			shallow = 0
		}
		out = append(out, ChartBucket{
			Label:            label,
			PRs:              prs,
			ShallowReviews:   shallow,
			DeepReviews:      deep,
			CommentsWritten:  b.Counts["review_comments_written"],
			CommentsReceived: b.Counts["review_comments_received"],
		})
	}
	return out
}

// bucketLabel returns a human-readable label for a time bucket.
// Monthly buckets get "Jan 25" style labels; quarterly get "Q1 25".
func bucketLabel(start, end time.Time) string {
	dur := end.Sub(start)
	days := dur.Hours() / 24
	if days <= 35 {
		// monthly-ish
		return start.Format("Jan 06")
	}
	if days <= 100 {
		// quarterly-ish
		q := (start.Month()-1)/3 + 1
		return fmt.Sprintf("Q%d %02d", q, start.Year()%100)
	}
	// semi-annual or longer
	return start.Format("Jan 06")
}

// stackedBarChart renders a stacked bar timeline as an inline SVG string.
// Three stacks per bucket: PRs (teal), shallow reviews (amber), deep reviews (indigo).
func stackedBarChart(buckets []ChartBucket, width, height int) string {
	const (
		padLeft   = 40
		padRight  = 16
		padTop    = 16
		padBottom = 48
		legendH   = 20
	)
	if len(buckets) == 0 {
		return svgEmpty(width, height, "No trend data available")
	}

	chartW := width - padLeft - padRight
	chartH := height - padTop - padBottom - legendH

	// Find max stacked value for Y-axis scaling.
	maxVal := 0
	for _, b := range buckets {
		total := b.PRs + b.ShallowReviews + b.DeepReviews
		if total > maxVal {
			maxVal = total
		}
	}
	if maxVal == 0 {
		maxVal = 1
	}

	barW := float64(chartW) / float64(len(buckets))
	barInner := barW * 0.65
	barGap := (barW - barInner) / 2

	yScale := func(v int) float64 {
		return float64(chartH) - float64(v)*float64(chartH)/float64(maxVal)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" role="img" aria-label="Stacked contribution timeline">`, width, height)

	// Grid lines and Y-axis labels.
	yTicks := yAxisTicks(maxVal, 4)
	for _, tick := range yTicks {
		y := float64(padTop) + yScale(tick)
		fmt.Fprintf(&sb, `<line x1="%d" y1="%.1f" x2="%d" y2="%.1f" stroke="#E2E8F0" stroke-width="1"/>`,
			padLeft, y, padLeft+chartW, y)
		fmt.Fprintf(&sb, `<text x="%d" y="%.1f" text-anchor="end" fill="#64748B" font-size="10" font-family="-apple-system,sans-serif">%d</text>`,
			padLeft-4, y+4, tick)
	}

	// Bars.
	for i, b := range buckets {
		x := float64(padLeft) + float64(i)*barW + barGap

		// Stack from bottom: PRs, then shallow, then deep.
		yBase := float64(padTop) + float64(chartH)
		if b.PRs > 0 {
			h := float64(b.PRs) * float64(chartH) / float64(maxVal)
			yBase -= h
			fmt.Fprintf(&sb, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="#0EA5E9" rx="2"/>`,
				x, yBase, barInner, h)
		}
		if b.ShallowReviews > 0 {
			h := float64(b.ShallowReviews) * float64(chartH) / float64(maxVal)
			yBase -= h
			fmt.Fprintf(&sb, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="#F59E0B" rx="2"/>`,
				x, yBase, barInner, h)
		}
		if b.DeepReviews > 0 {
			h := float64(b.DeepReviews) * float64(chartH) / float64(maxVal)
			yBase -= h
			fmt.Fprintf(&sb, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="#6366F1" rx="2"/>`,
				x, yBase, barInner, h)
		}

		// X-axis label.
		labelX := float64(padLeft) + float64(i)*barW + barW/2
		labelY := float64(padTop+chartH) + 14
		fmt.Fprintf(&sb, `<text x="%.1f" y="%.1f" text-anchor="middle" fill="#64748B" font-size="10" font-family="-apple-system,sans-serif">%s</text>`,
			labelX, labelY, b.Label)
	}

	// X-axis baseline.
	fmt.Fprintf(&sb, `<line x1="%d" y1="%d" x2="%d" y2="%d" stroke="#E2E8F0" stroke-width="1"/>`,
		padLeft, padTop+chartH, padLeft+chartW, padTop+chartH)

	// Legend.
	legendY := height - legendH/2 + 4
	legendItems := []struct{ color, label string }{
		{"#0EA5E9", "PRs merged"},
		{"#F59E0B", "Shallow reviews"},
		{"#6366F1", "Deep reviews"},
	}
	legendX := padLeft
	for _, item := range legendItems {
		fmt.Fprintf(&sb, `<rect x="%d" y="%d" width="10" height="10" fill="%s" rx="2"/>`, legendX, legendY-9, item.color)
		fmt.Fprintf(&sb, `<text x="%d" y="%d" fill="#64748B" font-size="10" font-family="-apple-system,sans-serif">%s</text>`,
			legendX+13, legendY, item.label)
		legendX += 110
	}

	sb.WriteString(`</svg>`)
	return sb.String()
}

// dualLineChart renders a dual-line SVG chart for review comments written vs. received.
func dualLineChart(buckets []ChartBucket, width, height int) string {
	const (
		padLeft   = 40
		padRight  = 16
		padTop    = 16
		padBottom = 48
		legendH   = 20
	)
	if len(buckets) == 0 {
		return svgEmpty(width, height, "No trend data available")
	}

	chartW := width - padLeft - padRight
	chartH := height - padTop - padBottom - legendH

	maxVal := 0
	for _, b := range buckets {
		if b.CommentsWritten > maxVal {
			maxVal = b.CommentsWritten
		}
		if b.CommentsReceived > maxVal {
			maxVal = b.CommentsReceived
		}
	}
	if maxVal == 0 {
		maxVal = 1
	}

	xStep := float64(chartW) / float64(max(len(buckets)-1, 1))
	yScale := func(v int) float64 {
		return float64(padTop) + float64(chartH) - float64(v)*float64(chartH)/float64(maxVal)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" role="img" aria-label="Review comment reciprocity over time">`, width, height)

	// Grid lines.
	yTicks := yAxisTicks(maxVal, 4)
	for _, tick := range yTicks {
		y := yScale(tick)
		fmt.Fprintf(&sb, `<line x1="%d" y1="%.1f" x2="%d" y2="%.1f" stroke="#E2E8F0" stroke-width="1"/>`,
			padLeft, y, padLeft+chartW, y)
		fmt.Fprintf(&sb, `<text x="%d" y="%.1f" text-anchor="end" fill="#64748B" font-size="10" font-family="-apple-system,sans-serif">%d</text>`,
			padLeft-4, y+4, tick)
	}

	// X-axis labels.
	for i, b := range buckets {
		lx := float64(padLeft) + float64(i)*xStep
		ly := float64(padTop+chartH) + 14
		fmt.Fprintf(&sb, `<text x="%.1f" y="%.1f" text-anchor="middle" fill="#64748B" font-size="10" font-family="-apple-system,sans-serif">%s</text>`,
			lx, ly, b.Label)
	}

	// Line for comments written (solid teal).
	writtenPts := make([]string, len(buckets))
	for i, b := range buckets {
		x := float64(padLeft) + float64(i)*xStep
		y := yScale(b.CommentsWritten)
		writtenPts[i] = fmt.Sprintf("%.1f,%.1f", x, y)
	}
	if len(writtenPts) > 1 {
		fmt.Fprintf(&sb, `<polyline points="%s" fill="none" stroke="#0EA5E9" stroke-width="2" stroke-linejoin="round" stroke-linecap="round"/>`,
			strings.Join(writtenPts, " "))
	}
	for _, pt := range writtenPts {
		var x, y float64
		fmt.Sscanf(pt, "%f,%f", &x, &y)
		fmt.Fprintf(&sb, `<circle cx="%.1f" cy="%.1f" r="3" fill="#0EA5E9"/>`, x, y)
	}

	// Line for comments received (dashed slate).
	receivedPts := make([]string, len(buckets))
	for i, b := range buckets {
		x := float64(padLeft) + float64(i)*xStep
		y := yScale(b.CommentsReceived)
		receivedPts[i] = fmt.Sprintf("%.1f,%.1f", x, y)
	}
	if len(receivedPts) > 1 {
		fmt.Fprintf(&sb, `<polyline points="%s" fill="none" stroke="#94A3B8" stroke-width="2" stroke-dasharray="6,3" stroke-linejoin="round" stroke-linecap="round"/>`,
			strings.Join(receivedPts, " "))
	}
	for _, pt := range receivedPts {
		var x, y float64
		fmt.Sscanf(pt, "%f,%f", &x, &y)
		fmt.Fprintf(&sb, `<circle cx="%.1f" cy="%.1f" r="3" fill="#94A3B8"/>`, x, y)
	}

	// X-axis baseline.
	fmt.Fprintf(&sb, `<line x1="%d" y1="%d" x2="%d" y2="%d" stroke="#E2E8F0" stroke-width="1"/>`,
		padLeft, padTop+chartH, padLeft+chartW, padTop+chartH)

	// Legend.
	legendY := height - legendH/2 + 4
	fmt.Fprintf(&sb, `<rect x="%d" y="%d" width="10" height="10" fill="#0EA5E9" rx="2"/>`, padLeft, legendY-9)
	fmt.Fprintf(&sb, `<text x="%d" y="%d" fill="#64748B" font-size="10" font-family="-apple-system,sans-serif">Comments written</text>`, padLeft+13, legendY)
	fmt.Fprintf(&sb, `<line x1="%d" y1="%d" x2="%d" y2="%d" stroke="#94A3B8" stroke-width="2" stroke-dasharray="4,2"/>`, padLeft+130, legendY-5, padLeft+144, legendY-5)
	fmt.Fprintf(&sb, `<text x="%d" y="%d" fill="#64748B" font-size="10" font-family="-apple-system,sans-serif">Comments received</text>`, padLeft+147, legendY)

	sb.WriteString(`</svg>`)
	return sb.String()
}

// heatmapChart renders a GitHub-style contribution heatmap as inline SVG.
// It spreads ActiveDays evenly across the trend buckets as an approximation.
func heatmapChart(trend []report.TrendBucket, activeDays, width, height int) string {
	const (
		cellSize = 11
		cellGap  = 2
		padLeft  = 24 // day labels
		padTop   = 20 // month labels
		padBot   = 8
		padRight = 8
	)

	if len(trend) == 0 {
		return svgEmpty(width, height, "No cadence data available")
	}

	// Determine the full date range from trend buckets.
	rangeStart := trend[0].Start
	rangeEnd := trend[len(trend)-1].End

	// Snap start back to the Monday of the first week.
	startWeekday := int(rangeStart.Weekday())
	if startWeekday == 0 {
		startWeekday = 7 // Sunday = 7
	}
	gridStart := rangeStart.AddDate(0, 0, -(startWeekday - 1)) // back to Monday

	// Compute total days in full grid.
	totalDays := int(rangeEnd.Sub(gridStart).Hours()/24) + 1
	totalWeeks := (totalDays + 6) / 7
	if totalWeeks == 0 {
		totalWeeks = 1
	}

	// Build a set of "active" weeks. We spread activeDays evenly across the
	// trend buckets proportional to bucket size, then mark the first N days
	// of each bucket as active (approximation).
	activeDaySet := buildActiveDaySet(trend, activeDays, gridStart)

	svgW := padLeft + totalWeeks*(cellSize+cellGap) + padRight
	svgH := padTop + 7*(cellSize+cellGap) + padBot

	var sb strings.Builder
	fmt.Fprintf(&sb, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" role="img" aria-label="Contribution heatmap">`, svgW, svgH)

	// Month labels above columns.
	currentMonth := -1
	for w := 0; w < totalWeeks; w++ {
		weekDate := gridStart.AddDate(0, 0, w*7)
		m := int(weekDate.Month())
		if m != currentMonth {
			currentMonth = m
			x := padLeft + w*(cellSize+cellGap)
			fmt.Fprintf(&sb, `<text x="%d" y="%d" fill="#64748B" font-size="9" font-family="-apple-system,sans-serif">%s</text>`,
				x, padTop-6, weekDate.Format("Jan"))
		}
	}

	// Day-of-week labels (Mon, Wed, Fri).
	dayLabels := []struct {
		row   int
		label string
	}{{0, "Mon"}, {2, "Wed"}, {4, "Fri"}}
	for _, dl := range dayLabels {
		y := padTop + dl.row*(cellSize+cellGap) + cellSize
		fmt.Fprintf(&sb, `<text x="%d" y="%d" text-anchor="end" fill="#64748B" font-size="9" font-family="-apple-system,sans-serif">%s</text>`,
			padLeft-3, y, dl.label)
	}

	// Grid cells.
	for w := 0; w < totalWeeks; w++ {
		for d := 0; d < 7; d++ {
			cellDate := gridStart.AddDate(0, 0, w*7+d)
			if cellDate.After(rangeEnd) {
				continue
			}
			x := padLeft + w*(cellSize+cellGap)
			y := padTop + d*(cellSize+cellGap)
			dateKey := cellDate.Format("2006-01-02")
			fill := "#E2E8F0"
			if activeDaySet[dateKey] {
				fill = "#0EA5E9"
			}
			fmt.Fprintf(&sb, `<rect x="%d" y="%d" width="%d" height="%d" fill="%s" rx="2"/>`,
				x, y, cellSize, cellSize, fill)
		}
	}

	sb.WriteString(`</svg>`)
	return sb.String()
}

// buildActiveDaySet spreads activeDays across trend buckets proportionally
// and returns a set of date strings "YYYY-MM-DD" that are "active".
func buildActiveDaySet(trend []report.TrendBucket, activeDays int, gridStart time.Time) map[string]bool {
	out := make(map[string]bool)
	if activeDays == 0 {
		return out
	}

	// Total days in the full coverage period.
	totalCovDays := 0
	for _, b := range trend {
		d := int(b.End.Sub(b.Start).Hours() / 24)
		totalCovDays += d
	}
	if totalCovDays == 0 {
		return out
	}

	_ = gridStart // gridStart used only for week alignment above
	remaining := activeDays
	for _, b := range trend {
		bucketDays := int(b.End.Sub(b.Start).Hours() / 24)
		bucketShare := int(math.Round(float64(activeDays) * float64(bucketDays) / float64(totalCovDays)))
		if bucketShare > remaining {
			bucketShare = remaining
		}
		// Mark the first bucketShare days of the bucket as active.
		for i := 0; i < bucketShare; i++ {
			d := b.Start.AddDate(0, 0, i)
			if d.Before(b.End) {
				out[d.Format("2006-01-02")] = true
			}
		}
		remaining -= bucketShare
	}
	return out
}

// yAxisTicks returns n evenly-spaced integer tick values from 0 to maxVal.
func yAxisTicks(maxVal, n int) []int {
	if n <= 0 || maxVal == 0 {
		return nil
	}
	step := int(math.Ceil(float64(maxVal) / float64(n)))
	if step == 0 {
		step = 1
	}
	var ticks []int
	for v := step; v <= maxVal; v += step {
		ticks = append(ticks, v)
	}
	return ticks
}

// svgEmpty returns a placeholder SVG with a centred message.
func svgEmpty(width, height int, msg string) string {
	return fmt.Sprintf(
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d"><text x="%d" y="%d" text-anchor="middle" fill="#64748B" font-size="12" font-family="-apple-system,sans-serif">%s</text></svg>`,
		width, height, width/2, height/2, msg,
	)
}

// max returns the larger of two ints.
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

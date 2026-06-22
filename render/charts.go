package render

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/grkanitz/coderepute/report"
)

// ChartBucket holds the data for one time bucket across all series.
type ChartBucket struct {
	Label            string
	Start            time.Time
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
		prs := b.Counts["pull_requests"]
		reviews := b.Counts["reviews_given"]
		deep := int(math.Round(float64(reviews) * deepRatio))
		shallow := reviews - deep
		if shallow < 0 {
			shallow = 0
		}
		out = append(out, ChartBucket{
			Label:            bucketLabel(b.Start, b.End),
			Start:            b.Start,
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
func bucketLabel(start, end time.Time) string {
	dur := end.Sub(start)
	days := dur.Hours() / 24
	if days <= 35 {
		return start.Format("Jan 06")
	}
	if days <= 100 {
		q := (start.Month()-1)/3 + 1
		return fmt.Sprintf("Q%d %02d", q, start.Year()%100)
	}
	return start.Format("Jan 06")
}

// xAxisLabels decides which bucket indices get a label. For ≤12 buckets show
// all; for longer ranges show only January of each year to avoid crowding.
func xAxisLabels(buckets []ChartBucket) map[int]string {
	out := make(map[int]string, len(buckets))
	if len(buckets) <= 12 {
		for i, b := range buckets {
			out[i] = b.Label
		}
		return out
	}
	// Multi-year: label only January buckets.
	seenYear := map[int]bool{}
	for i, b := range buckets {
		y := b.Start.Year()
		if b.Start.Month() == 1 && !seenYear[y] {
			seenYear[y] = true
			out[i] = fmt.Sprintf("%d", y)
		}
	}
	// Always label the first and last bucket so the reader knows the range.
	if _, ok := out[0]; !ok {
		out[0] = buckets[0].Start.Format("Jan 06")
	}
	last := len(buckets) - 1
	if _, ok := out[last]; !ok {
		out[last] = buckets[last].Start.Format("Jan 06")
	}
	return out
}

// stackedBarChart renders a stacked bar timeline as an inline SVG string.
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

	labels := xAxisLabels(buckets)

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

		if label, ok := labels[i]; ok {
			labelX := float64(padLeft) + float64(i)*barW + barW/2
			labelY := float64(padTop+chartH) + 14
			fmt.Fprintf(&sb, `<text x="%.1f" y="%.1f" text-anchor="middle" fill="#64748B" font-size="10" font-family="-apple-system,sans-serif">%s</text>`,
				labelX, labelY, label)
		}
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

	labels := xAxisLabels(buckets)

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

	// X-axis labels (thinned).
	for i, b := range buckets {
		if label, ok := labels[i]; ok {
			lx := float64(padLeft) + float64(i)*xStep
			ly := float64(padTop+chartH) + 14
			_ = b
			fmt.Fprintf(&sb, `<text x="%.1f" y="%.1f" text-anchor="middle" fill="#64748B" font-size="10" font-family="-apple-system,sans-serif">%s</text>`,
				lx, ly, label)
		}
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

// heatmapChart renders a per-year contribution heatmap using actual active
// dates (YYYY-MM-DD strings). Each calendar year gets its own 53-column row,
// stacked vertically — keeps the chart a readable width regardless of tenure.
func heatmapChart(activeDates []string, width int) string {
	const (
		cellSize = 10
		cellGap  = 2
		cellStep = cellSize + cellGap
		padLeft  = 44 // year + day labels
		padRight = 8
		rowTop   = 18 // month labels above cells
		rowBot   = 10
		rowH     = rowTop + 7*cellStep + rowBot
	)

	if len(activeDates) == 0 {
		return svgEmpty(width, 80, "No cadence data available")
	}

	// Build active date set, filtering out any zero/sentinel dates.
	activeSet := make(map[string]bool, len(activeDates))
	var validDates []string
	for _, d := range activeDates {
		t, err := time.Parse("2006-01-02", d)
		if err == nil && t.Year() >= 2000 {
			activeSet[d] = true
			validDates = append(validDates, d)
		}
	}
	if len(validDates) == 0 {
		return svgEmpty(width, 80, "No cadence data available")
	}

	// Determine year range from valid dates.
	firstDate, _ := time.Parse("2006-01-02", validDates[0])
	lastDate, _ := time.Parse("2006-01-02", validDates[len(validDates)-1])
	firstYear := firstDate.Year()
	lastYear := lastDate.Year()
	nYears := lastYear - firstYear + 1

	svgH := nYears * rowH
	var sb strings.Builder
	fmt.Fprintf(&sb, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" role="img" aria-label="Contribution heatmap by year">`, width, svgH)

	for yi, year := range yearsRange(firstYear, lastYear) {
		yOffset := yi * rowH

		// Year label on left, vertically centred in the cell grid.
		gridMid := yOffset + rowTop + (7*cellStep)/2
		fmt.Fprintf(&sb, `<text x="%d" y="%d" text-anchor="end" fill="#64748B" font-size="10" font-weight="600" font-family="-apple-system,sans-serif" dominant-baseline="middle">%d</text>`,
			padLeft-6, gridMid, year)

		// Day-of-week labels (Mon, Wed, Fri) on the left.
		for row, dl := range []struct {
			r int
			l string
		}{{0, "Mon"}, {2, "Wed"}, {4, "Fri"}} {
			_ = row
			y := yOffset + rowTop + dl.r*cellStep + cellSize
			fmt.Fprintf(&sb, `<text x="%d" y="%d" text-anchor="end" fill="#94A3B8" font-size="8" font-family="-apple-system,sans-serif">%s</text>`,
				padLeft-18, y, dl.l)
		}

		// Snap to Monday on or before Jan 1 of this year.
		jan1 := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
		wd := int(jan1.Weekday())
		if wd == 0 {
			wd = 7 // Sunday = 7 in ISO week
		}
		gridStart := jan1.AddDate(0, 0, -(wd - 1))

		// 53 columns.
		currentMonth := -1
		for w := 0; w < 53; w++ {
			weekStart := gridStart.AddDate(0, 0, w*7)
			// Month label when month changes.
			m := int(weekStart.Month())
			if weekStart.Year() == year && m != currentMonth {
				currentMonth = m
				x := padLeft + w*cellStep
				fmt.Fprintf(&sb, `<text x="%d" y="%d" fill="#64748B" font-size="8" font-family="-apple-system,sans-serif">%s</text>`,
					x, yOffset+rowTop-5, weekStart.Format("Jan"))
			}

			for d := 0; d < 7; d++ {
				cellDate := gridStart.AddDate(0, 0, w*7+d)
				if cellDate.Year() != year {
					continue // don't draw outside this year's lane
				}
				x := padLeft + w*cellStep
				y := yOffset + rowTop + d*cellStep
				dateKey := cellDate.Format("2006-01-02")
				fill := "#E2E8F0"
				if activeSet[dateKey] {
					fill = "#0EA5E9"
				}
				fmt.Fprintf(&sb, `<rect x="%d" y="%d" width="%d" height="%d" fill="%s" rx="2" title="%s"/>`,
					x, y, cellSize, cellSize, fill, dateKey)
			}
		}
	}

	sb.WriteString(`</svg>`)
	return sb.String()
}

// yearsRange returns a slice of years from first to last inclusive.
func yearsRange(first, last int) []int {
	out := make([]int, 0, last-first+1)
	for y := first; y <= last; y++ {
		out = append(out, y)
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

// Package render turns a report into a self-contained HTML document.
//
// The document is composed from per-section templates under
// templates/sections/, executed in filename order. Follow-up slices add
// sections by adding template files, not editing existing ones.
package render

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"math"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	qrcode "github.com/skip2/go-qrcode"

	"github.com/gkanitz/coderepute/report"
)

// verifyFallbackURL is the base verification page URL used when a report has
// no Verification.VerifyURL set (e.g., unverified local runs).
const verifyFallbackURL = "https://gkanitz.github.io/CodeRepute/verify/"

//go:embed templates
var templates embed.FS

var funcs = template.FuncMap{
	// date formats a time.Time or *time.Time as YYYY-MM-DD.
	// When passed a nil *time.Time it returns "all time".
	"date": func(v any) string {
		switch t := v.(type) {
		case time.Time:
			return t.UTC().Format("2006-01-02")
		case *time.Time:
			if t == nil {
				return "all time"
			}
			return t.UTC().Format("2006-01-02")
		default:
			return ""
		}
	},
	"total": func(counts map[string]int) int {
		n := 0
		for _, c := range counts {
			n += c
		}
		return n
	},
	// hours renders a duration sample to one decimal, trimming a
	// trailing ".0" (30.5 -> "30.5", 24 -> "24").
	"hours": func(h float64) string { return strconv.FormatFloat(math.Round(h*10)/10, 'f', -1, 64) },
	// percent renders a 0..1 share as a whole percentage (0.5 -> "50%").
	"percent": func(share float64) string { return strconv.FormatFloat(math.Round(share*100), 'f', -1, 64) + "%" },
	// orgs reduces "owner/repo" coverage entries to their sorted, deduplicated
	// owner names. Individual repo names are never rendered: a recruiter
	// reading the report has no business reason to see private repo names,
	// which can themselves be confidential (unannounced products, codenames).
	"orgs": func(repos []string) []string {
		seen := make(map[string]bool)
		var out []string
		for _, r := range repos {
			org, _, ok := strings.Cut(r, "/")
			if !ok {
				org = r
			}
			if !seen[org] {
				seen[org] = true
				out = append(out, org)
			}
		}
		sort.Strings(out)
		return out
	},
	// chartBuckets converts trend data into chart-ready buckets.
	// Takes: trend []TrendBucket, totalReviews int, deepReviews int.
	"chartBuckets": func(r report.Report) []ChartBucket {
		if r.Cadence == nil {
			return nil
		}
		totalReviews, deepReviews := 0, 0
		if r.Collaboration != nil && r.Collaboration.ReviewsGiven != nil {
			totalReviews = r.Collaboration.ReviewsGiven.Total
			deepReviews = r.Collaboration.ReviewsGiven.DeepReviewCount
		}
		return buildChartBuckets(r.Cadence.Trend, totalReviews, deepReviews)
	},
	// stackedBarSVG generates an inline SVG stacked bar chart.
	"stackedBarSVG": func(r report.Report) template.HTML {
		if r.Cadence == nil || len(r.Cadence.Trend) == 0 {
			return ""
		}
		totalReviews, deepReviews := 0, 0
		if r.Collaboration != nil && r.Collaboration.ReviewsGiven != nil {
			totalReviews = r.Collaboration.ReviewsGiven.Total
			deepReviews = r.Collaboration.ReviewsGiven.DeepReviewCount
		}
		buckets := buildChartBuckets(r.Cadence.Trend, totalReviews, deepReviews)
		return template.HTML(stackedBarChart(buckets, 640, 220))
	},
	// dualLineSVG generates an inline SVG dual-line chart for review comments.
	"dualLineSVG": func(r report.Report) template.HTML {
		if r.Cadence == nil || len(r.Cadence.Trend) == 0 {
			return ""
		}
		totalReviews, deepReviews := 0, 0
		if r.Collaboration != nil && r.Collaboration.ReviewsGiven != nil {
			totalReviews = r.Collaboration.ReviewsGiven.Total
			deepReviews = r.Collaboration.ReviewsGiven.DeepReviewCount
		}
		buckets := buildChartBuckets(r.Cadence.Trend, totalReviews, deepReviews)
		return template.HTML(dualLineChart(buckets, 640, 200))
	},
	// heatmapSVG generates an inline SVG contribution heatmap.
	"heatmapSVG": func(r report.Report) template.HTML {
		if r.Cadence == nil {
			return ""
		}
		return template.HTML(heatmapChart(r.Cadence.ActiveDates, 640))
	},
	// deepReviewPct computes the deep-review percentage from ReviewsGiven.
	// Returns "n/a" when there are no reviews. Uses DeepReviewCount (reviews
	// with ≥3 inline comments) populated from provider.Review.CommentCount.
	"deepReviewPct": func(r report.Report) string {
		if r.Collaboration == nil || r.Collaboration.ReviewsGiven == nil {
			return "n/a"
		}
		rv := r.Collaboration.ReviewsGiven
		if rv.Total == 0 {
			return "0%"
		}
		pct := int(math.Round(float64(rv.DeepReviewCount) / float64(rv.Total) * 100))
		return strconv.Itoa(pct) + "%"
	},
	// shareBarSVG generates an inline SVG horizontal share bar for the language
	// mix section. Returns an empty string when there is no language mix data.
	"shareBarSVG": func(r report.Report) template.HTML {
		if r.Collaboration == nil || r.Collaboration.LanguageMix == nil {
			return ""
		}
		lm := r.Collaboration.LanguageMix
		var segments []shareSegment
		for _, l := range lm.Languages {
			segments = append(segments, shareSegment{label: l.Name, pct: l.SharePct})
		}
		if lm.OtherShare > 0 {
			segments = append(segments, shareSegment{label: "Other", pct: lm.OtherShare})
		}
		return template.HTML(shareBarChart(segments, 640, 40))
	},

	// reverseTrend returns trend buckets in reverse order (newest first).
	"reverseTrend": func(buckets []report.TrendBucket) []report.TrendBucket {
		out := make([]report.TrendBucket, len(buckets))
		for i, b := range buckets {
			out[len(buckets)-1-i] = b
		}
		return out
	},
	// bandContextTmpl looks up a band entry in the report by key and returns a
	// rendered context line with neutral styling, or empty string if no entry
	// exists. The rendered line is derived from the report's own bands block,
	// never from the embedded file. It carries no judgment or state styling.
	"bandContext": func(r report.Report, key string) string { return bandContext(r, key) },

	// narrative builds the full narrative section from the report.
	"narrative": func(r report.Report) NarrativeSection { return narrativeBuild(r) },

	// narrativeProbes returns the interviewer probes for template rendering.
	"narrativeProbes": func(r report.Report) []Probe { return narrativeProbes(r) },

	// narrativeAnnex returns the derivation annex entries.
	"narrativeAnnex": func(r report.Report) []AnnexEntry { return narrativeAnnex(r) },

	// medianTTM formats the median time-to-merge as "X.X hrs".
	"medianTTM": func(r report.Report) string {
		if r.Collaboration == nil || r.Collaboration.TimeToMerge == nil {
			return "n/a"
		}
		h := r.Collaboration.TimeToMerge.MedianHours
		return strconv.FormatFloat(math.Round(h*10)/10, 'f', -1, 64) + " hrs"
	},
	// reportJSON marshals the Report as indented JSON and returns it as
	// template.JS for embedding in a <script type="application/json"> tag.
	// </script> sequences are escaped to <\/script> to prevent XSS injection.
	"reportJSON": func(r report.Report) template.JS {
		data, err := json.MarshalIndent(r, "", "  ")
		if err != nil {
			return ""
		}
		safe := strings.ReplaceAll(string(data), "</script>", `<\/script>`)
		return template.JS(safe)
	},
	// verifyURL returns the URL to use for the verify link/QR code:
	// r.Verification.VerifyURL if set, otherwise verifyFallbackURL.
	"verifyURL": func(r report.Report) string {
		if r.Verification != nil && r.Verification.VerifyURL != "" {
			return r.Verification.VerifyURL
		}
		return verifyFallbackURL
	},
	// verifyQRSVG generates an inline SVG QR code pointing at the report's
	// verify URL (r.Verification.VerifyURL), falling back to verifyFallbackURL.
	"verifyQRSVG": func(r report.Report) template.HTML {
		u := verifyFallbackURL
		if r.Verification != nil && r.Verification.VerifyURL != "" {
			u = r.Verification.VerifyURL
		}
		qr, err := qrcode.New(u, qrcode.Medium)
		if err != nil {
			return ""
		}
		qr.DisableBorder = false
		bitmap := qr.Bitmap()
		n := len(bitmap)
		const px = 80 // rendered size in CSS pixels
		var sb strings.Builder
		fmt.Fprintf(&sb, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" style="display:block;image-rendering:pixelated">`, px, px, n, n)
		for y, row := range bitmap {
			for x, dark := range row {
				if dark {
					fmt.Fprintf(&sb, `<rect x="%d" y="%d" width="1" height="1" fill="#0F172A"/>`, x, y)
				}
			}
		}
		sb.WriteString(`</svg>`)
		return template.HTML(sb.String())
	},
}

// bandContext looks up a band entry in the report by key and returns a
// rendered context line, or empty string if no entry exists. The context
// line is derived from the report's own bands block, never from the
// embedded file, so what is shown is what is attested. The rendered line
// carries no judgment or state styling.
func bandContext(r report.Report, key string) string {
	if r.Bands == nil {
		return ""
	}
	for _, e := range r.Bands.Entries {
		if e.Key == key {
			// Format the range. For "share" unit, format as percentage.
			rangeStr := formatBandRange(e.RangeLo, e.RangeHi, e.Unit)
			return fmt.Sprintf("Typical range: %s (%s, %s). %s", rangeStr, e.SourceTitle, e.SourceYear, e.Caveat)
		}
	}
	return ""
}

// formatBandRange formats a [lo, hi] range as a human-readable string.
func formatBandRange(lo, hi float64, unit string) string {
	switch unit {
	case "hours":
		return fmt.Sprintf("%.0f–%.0f h", lo, hi)
	case "share":
		// Shares are 0..1, render as percentages.
		return fmt.Sprintf("%.0f–%.0f%%", lo*100, hi*100)
	case "lines":
		return fmt.Sprintf("%.0f–%.0f lines", lo, hi)
	default:
		return fmt.Sprintf("%.0f–%.0f %s", lo, hi, unit)
	}
}

// CardSVG renders a 1200x627 share card SVG from the report struct, with
// exactly four headline metrics (PRs merged, reviews given, median TTM,
// active days), a QR code pointing at the verify URL, and a verification
// status mark. Missing metrics render as an em dash ("—").
func CardSVG(r report.Report) ([]byte, error) {
	w, h := 1200, 627
	pad := 48
	innerW := w - 2*pad

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(
		`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" role="img" aria-label="CodeRepute collaboration report">`,
		w, h, w, h))
	// Background
	sb.WriteString(fmt.Sprintf(`<rect width="%d" height="%d" fill="#FFFFFF" rx="12"/>`, w, h))
	// Subtle border
	sb.WriteString(fmt.Sprintf(`<rect width="%d" height="%d" fill="none" stroke="#E2E8F0" stroke-width="1" rx="12"/>`, w, h))

	// ── Header bar ──
	// "CodeRepute" label (left)
	sb.WriteString(fmt.Sprintf(`<text x="%d" y="%d" fill="#0EA5E9" font-size="14" font-weight="700" font-family="-apple-system,Segoe UI,Roboto,Helvetica,Arial,sans-serif" letter-spacing="0.05em">CodeRepute</text>`,
		pad, pad+20))
	// Verification badge (right)
	verifyBadgeX := w - pad
	verifyBadgeText := "unverified"
	verifyBadgeFill := "#9a6700"
	verifyBadgeBg := "#FFF3CD"
	if r.Verification != nil && r.Verification.Status == report.StatusVerified {
		verifyBadgeText = "Sigstore attested"
		verifyBadgeFill = "#0F6F3F"
		verifyBadgeBg = "#D9F2E3"
	}
	// Compute badge width for right-alignment
	badgeTextWidth := len(verifyBadgeText) * 8
	if badgeTextWidth < 100 {
		badgeTextWidth = 100
	} else if badgeTextWidth > 160 {
		badgeTextWidth = 160
	}
	badgeX := verifyBadgeX - badgeTextWidth - 16
	sb.WriteString(fmt.Sprintf(`<rect x="%d" y="%d" width="%d" height="24" rx="12" fill="%s"/>`,
		badgeX, pad-2, badgeTextWidth+16, verifyBadgeBg))
	sb.WriteString(fmt.Sprintf(`<text x="%d" y="%d" text-anchor="end" fill="%s" font-size="12" font-weight="700" font-family="-apple-system,Segoe UI,Roboto,Helvetica,Arial,sans-serif">%s</text>`,
		verifyBadgeX, pad+14, verifyBadgeFill, verifyBadgeText))

	// ── Subject line ──
	subjectY := pad + 80
	sb.WriteString(fmt.Sprintf(`<text x="%d" y="%d" fill="#0F172A" font-size="36" font-weight="800" font-family="-apple-system,Segoe UI,Roboto,Helvetica,Arial,sans-serif" letter-spacing="-0.02em">%s</text>`,
		pad, subjectY, htmlEscape(r.Subject.Username)))
	platformStr := r.Subject.Platform
	if r.Subject.AccountID != "" {
		platformStr += " ID " + r.Subject.AccountID
	}
	sb.WriteString(fmt.Sprintf(`<text x="%d" y="%d" fill="#64748B" font-size="18" font-family="-apple-system,Segoe UI,Roboto,Helvetica,Arial,sans-serif">%s</text>`,
		pad, subjectY+28, htmlEscape(platformStr)))

	// ── Org context line ──
	ownerStr := orgContextLabel(r.Coverage.Repos)
	windowStr := coverageWindowStr(r.Coverage)
	contextStr := ownerStr + "  ·  " + windowStr
	contextY := subjectY + 64
	sb.WriteString(fmt.Sprintf(`<text x="%d" y="%d" fill="#64748B" font-size="15" font-family="-apple-system,Segoe UI,Roboto,Helvetica,Arial,sans-serif">%s</text>`,
		pad, contextY, htmlEscape(contextStr)))

	// ── Four headline metrics ──
	metricsY := contextY + 100
	metricCardW := (innerW - 3*16) / 4 // 4 cards with 16px gaps
	cardX := func(i int) int { return pad + i*(metricCardW+16) }

	metrics := []struct {
		value string
		label string
	}{
		{prsMergedStr(r), "PRs merged"},
		{reviewsGivenStr(r), "Reviews given"},
		{medianTTMStr(r), "Median TTM"},
		{activeDaysStr(r), "Active days"},
	}

	for i, m := range metrics {
		x := cardX(i)
		// Card background
		sb.WriteString(fmt.Sprintf(`<rect x="%d" y="%d" width="%d" height="%d" rx="8" fill="#F8FAFC" stroke="#E2E8F0" stroke-width="1"/>`,
			x, metricsY-20, metricCardW, 110))
		// Value
		sb.WriteString(fmt.Sprintf(`<text x="%d" y="%d" text-anchor="middle" fill="#0F172A" font-size="36" font-weight="800" font-family="-apple-system,Segoe UI,Roboto,Helvetica,Arial,sans-serif" letter-spacing="-0.03em">%s</text>`,
			x+metricCardW/2, metricsY+30, htmlEscape(m.value)))
		// Label
		sb.WriteString(fmt.Sprintf(`<text x="%d" y="%d" text-anchor="middle" fill="#64748B" font-size="12" font-weight="600" font-family="-apple-system,Segoe UI,Roboto,Helvetica,Arial,sans-serif" letter-spacing="0.04em">%s</text>`,
			x+metricCardW/2, metricsY+54, htmlEscape(m.label)))
	}

	// ── QR code + verify link (bottom) ──
	qrY := h - 150
	qrSize := 80

	u := verifyFallbackURL
	if r.Verification != nil && r.Verification.VerifyURL != "" {
		u = r.Verification.VerifyURL
	}
	qrSVG, err := qrSVGForURL(u, qrSize)
	if err != nil {
		return nil, fmt.Errorf("render card QR: %w", err)
	}

	// QR occupies a box at bottom-left
	sb.WriteString(fmt.Sprintf(`<g transform="translate(%d,%d)">%s</g>`, pad, qrY+10, qrSVG))

	// Verify link text to the right of QR
	verifyTextX := pad + qrSize + 24
	verifyTextY := qrY + 24
	sb.WriteString(fmt.Sprintf(`<text x="%d" y="%d" fill="#0F172A" font-size="14" font-weight="600" font-family="-apple-system,Segoe UI,Roboto,Helvetica,Arial,sans-serif">Verify this report</text>`,
		verifyTextX, verifyTextY))
	sb.WriteString(fmt.Sprintf(`<text x="%d" y="%d" fill="#64748B" font-size="12" font-family="-apple-system,Segoe UI,Roboto,Helvetica,Arial,sans-serif">%s</text>`,
		verifyTextX, verifyTextY+20, htmlEscape(u)))

	// Attested / unverified footer
	footerText := "This report was produced locally and has not been independently verified."
	if r.Verification != nil && r.Verification.Status == report.StatusVerified {
		footerText = "This report has been cryptographically attested with Sigstore."
	}
	sb.WriteString(fmt.Sprintf(`<text x="%d" y="%d" fill="#94A3B8" font-size="11" font-family="-apple-system,Segoe UI,Roboto,Helvetica,Arial,sans-serif">%s</text>`,
		pad, h-pad, htmlEscape(footerText)))

	sb.WriteString(`</svg>`)
	return []byte(sb.String()), nil
}

// qrSVGForURL generates an inline SVG QR code for the given URL at the given
// pixel size, returning the raw SVG string (without outer XML wrapper).
func qrSVGForURL(url string, size int) (string, error) {
	qr, err := qrcode.New(url, qrcode.Medium)
	if err != nil {
		return "", err
	}
	qr.DisableBorder = false
	bitmap := qr.Bitmap()
	n := len(bitmap)
	var sb strings.Builder
	fmt.Fprintf(&sb, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" style="display:block;image-rendering:pixelated">`, size, size, n, n)
	for y, row := range bitmap {
		for x, dark := range row {
			if dark {
				fmt.Fprintf(&sb, `<rect x="%d" y="%d" width="1" height="1" fill="#0F172A"/>`, x, y)
			}
		}
	}
	sb.WriteString(`</svg>`)
	return sb.String(), nil
}

// orgContextLabel returns a human-readable label for the org context: the
// single owner name when all repos share one owner, otherwise "N orgs".
func orgContextLabel(repos []string) string {
	seen := make(map[string]bool)
	for _, r := range repos {
		org, _, ok := strings.Cut(r, "/")
		if !ok {
			org = r
		}
		seen[org] = true
	}
	if len(seen) == 0 {
		return ""
	}
	if len(seen) == 1 {
		for o := range seen {
			return o
		}
	}
	return fmt.Sprintf("%d orgs", len(seen))
}

// coverageWindowStr formats the coverage window as a readable string.
func coverageWindowStr(c *report.Coverage) string {
	if c == nil {
		return ""
	}
	since := "all time"
	if c.Window.Since != nil {
		since = c.Window.Since.UTC().Format("2006-01-02")
	}
	until := c.Window.Until.UTC().Format("2006-01-02")
	return since + " → " + until
}

// prsMergedStr returns the PRs merged count as a string, or "—" when nil.
func prsMergedStr(r report.Report) string {
	if r.Collaboration == nil || r.Collaboration.PullRequests == nil {
		return "—"
	}
	return strconv.Itoa(r.Collaboration.PullRequests.Merged)
}

// reviewsGivenStr returns the reviews given total as a string, or "—" when nil.
func reviewsGivenStr(r report.Report) string {
	if r.Collaboration == nil || r.Collaboration.ReviewsGiven == nil {
		return "—"
	}
	return strconv.Itoa(r.Collaboration.ReviewsGiven.Total)
}

// medianTTMStr returns the median time-to-merge as a formatted string with
// " hrs" suffix, or "—" when nil.
func medianTTMStr(r report.Report) string {
	if r.Collaboration == nil || r.Collaboration.TimeToMerge == nil {
		return "—"
	}
	h := r.Collaboration.TimeToMerge.MedianHours
	return strconv.FormatFloat(math.Round(h*10)/10, 'f', -1, 64) + " hrs"
}

// activeDaysStr returns the active days count as a string, or "—" when nil.
func activeDaysStr(r report.Report) string {
	if r.Cadence == nil {
		return "—"
	}
	return strconv.Itoa(r.Cadence.ActiveDays)
}

// htmlEscape replaces special HTML characters with their entities, sufficient
// for SVG text content.
func htmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

// HTML renders the report as a single self-contained HTML document.
func HTML(r report.Report) ([]byte, error) {
	sections, err := fs.Glob(templates, "templates/sections/*.tmpl")
	if err != nil {
		return nil, err
	}
	sort.Strings(sections)

	var body bytes.Buffer
	for _, p := range sections {
		tmpl, err := template.New("section").Funcs(funcs).ParseFS(templates, p)
		if err != nil {
			return nil, fmt.Errorf("render: parse %s: %w", p, err)
		}
		if err := tmpl.ExecuteTemplate(&body, path.Base(p), r); err != nil {
			return nil, fmt.Errorf("render: section %s: %w", p, err)
		}
	}

	layout, err := template.New("layout.tmpl").Funcs(funcs).ParseFS(templates, "templates/layout.tmpl")
	if err != nil {
		return nil, fmt.Errorf("render: parse layout: %w", err)
	}
	var out bytes.Buffer
	err = layout.Execute(&out, struct {
		Report report.Report
		Body   template.HTML
	}{Report: r, Body: template.HTML(body.String())})
	if err != nil {
		return nil, fmt.Errorf("render: layout: %w", err)
	}
	return out.Bytes(), nil
}

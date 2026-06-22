// Package render turns a report into a self-contained HTML document.
//
// The document is composed from per-section templates under
// templates/sections/, executed in filename order. Follow-up slices add
// sections by adding template files, not editing existing ones.
package render

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"math"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/grkanitz/coderepute/report"
)

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
	// reverseTrend returns trend buckets in reverse order (newest first).
	"reverseTrend": func(buckets []report.TrendBucket) []report.TrendBucket {
		out := make([]report.TrendBucket, len(buckets))
		for i, b := range buckets {
			out[len(buckets)-1-i] = b
		}
		return out
	},
	// medianTTM formats the median time-to-merge as "X.X hrs".
	"medianTTM": func(r report.Report) string {
		if r.Collaboration == nil || r.Collaboration.TimeToMerge == nil {
			return "n/a"
		}
		h := r.Collaboration.TimeToMerge.MedianHours
		return strconv.FormatFloat(math.Round(h*10)/10, 'f', -1, 64) + " hrs"
	},
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

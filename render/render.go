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
	"path"
	"sort"
	"time"

	"github.com/grkanitz/coderepute/report"
)

//go:embed templates
var templates embed.FS

var funcs = template.FuncMap{
	"date": func(t time.Time) string { return t.UTC().Format("2006-01-02") },
	"total": func(counts map[string]int) int {
		n := 0
		for _, c := range counts {
			n += c
		}
		return n
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

// Command coderepute produces a collaboration report for one developer
// from platform API metadata: fetch → compute → build → render.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/grkanitz/coderepute/metrics"
	"github.com/grkanitz/coderepute/provider"
	"github.com/grkanitz/coderepute/provider/github"
	"github.com/grkanitz/coderepute/render"
	"github.com/grkanitz/coderepute/report"
)

func main() {
	os.Exit(run(os.Args[1:], os.Getenv, os.Stderr))
}

func run(args []string, getenv func(string) string, stderr io.Writer) int {
	fs := flag.NewFlagSet("coderepute", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		repos      = fs.String("repo", "", "repository to cover, owner/name (comma-separated for several)")
		subject    = fs.String("subject", "", "GitHub username the report is about")
		token      = fs.String("token", "", "GitHub token (defaults to GITHUB_TOKEN)")
		windowDays = fs.Int("window-days", 365, "report window ending now, in days")
		outDir     = fs.String("out", ".", "output directory for report.json and report.html")
		apiBase    = fs.String("api-base", "https://api.github.com", "GitHub API base URL")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *token == "" {
		*token = getenv("GITHUB_TOKEN")
	}
	switch {
	case *repos == "":
		fmt.Fprintln(stderr, "coderepute: -repo is required")
		return 2
	case *subject == "":
		fmt.Fprintln(stderr, "coderepute: -subject is required")
		return 2
	case *token == "":
		fmt.Fprintln(stderr, "coderepute: a token is required (-token or GITHUB_TOKEN)")
		return 2
	case *windowDays <= 0:
		fmt.Fprintln(stderr, "coderepute: -window-days must be positive")
		return 2
	}

	until := time.Now().UTC()
	window := provider.Window{Since: until.AddDate(0, 0, -*windowDays), Until: until}

	adapter := github.New(*token, github.WithBaseURL(*apiBase))
	activity, err := adapter.FetchActivity(context.Background(), provider.FetchOptions{
		Repos:   strings.Split(*repos, ","),
		Subject: *subject,
		Window:  window,
	})
	if err != nil {
		fmt.Fprintf(stderr, "coderepute: fetch: %v\n", err)
		return 1
	}

	result := metrics.Compute(activity)
	r := report.Build(activity, &result.Collaboration, &result.Cadence, time.Now())
	if err := r.Validate(); err != nil {
		fmt.Fprintf(stderr, "coderepute: built an invalid report: %v\n", err)
		return 1
	}

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fmt.Fprintf(stderr, "coderepute: %v\n", err)
		return 1
	}
	rawJSON, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "coderepute: encode report: %v\n", err)
		return 1
	}
	if err := os.WriteFile(filepath.Join(*outDir, "report.json"), append(rawJSON, '\n'), 0o644); err != nil {
		fmt.Fprintf(stderr, "coderepute: %v\n", err)
		return 1
	}
	rawHTML, err := render.HTML(r)
	if err != nil {
		fmt.Fprintf(stderr, "coderepute: render: %v\n", err)
		return 1
	}
	if err := os.WriteFile(filepath.Join(*outDir, "report.html"), rawHTML, 0o644); err != nil {
		fmt.Fprintf(stderr, "coderepute: %v\n", err)
		return 1
	}

	fmt.Fprintf(stderr, "wrote %s and %s\n",
		filepath.Join(*outDir, "report.json"), filepath.Join(*outDir, "report.html"))
	return 0
}

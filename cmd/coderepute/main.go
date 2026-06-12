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
		repos          = fs.String("repo", "", "repository to cover, owner/name (comma-separated for several)")
		org            = fs.String("org", "", "organization to cover: every repo visible to the token (alternative to -repo)")
		subject        = fs.String("subject", "", "GitHub username the report is about")
		token          = fs.String("token", "", "GitHub token (defaults to GITHUB_TOKEN)")
		appID          = fs.String("app-id", "", "GitHub App ID; with -app-key, mints an installation token instead of -token")
		appKey         = fs.String("app-key", "", "path to the GitHub App private key PEM")
		installationID = fs.Int64("installation-id", 0, "GitHub App installation to act as (default: the sole installation)")
		windowDays     = fs.Int("window-days", 365, "report window ending now, in days")
		outDir         = fs.String("out", ".", "output directory for report.json and report.html")
		apiBase        = fs.String("api-base", "https://api.github.com", "GitHub API base URL")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *token == "" {
		*token = getenv("GITHUB_TOKEN")
	}
	usingApp := *appID != "" || *appKey != ""
	switch {
	case usingApp && (*appID == "" || *appKey == ""):
		fmt.Fprintln(stderr, "coderepute: -app-id and -app-key must be given together")
		return 2
	case *repos == "" && *org == "" && !usingApp:
		fmt.Fprintln(stderr, "coderepute: -repo or -org is required")
		return 2
	case *subject == "":
		fmt.Fprintln(stderr, "coderepute: -subject is required")
		return 2
	case *token == "" && !usingApp:
		fmt.Fprintln(stderr, "coderepute: a token is required (-token, GITHUB_TOKEN, or -app-id/-app-key)")
		return 2
	case *windowDays <= 0:
		fmt.Fprintln(stderr, "coderepute: -window-days must be positive")
		return 2
	}

	ctx := context.Background()
	if usingApp {
		minted, err := mintInstallationToken(ctx, *appID, *appKey, *installationID, *apiBase)
		if err != nil {
			fmt.Fprintf(stderr, "coderepute: app token: %v\n", err)
			return 1
		}
		*token = minted
	}

	until := time.Now().UTC()
	window := provider.Window{Since: until.AddDate(0, 0, -*windowDays), Until: until}

	adapter := github.New(*token, github.WithBaseURL(*apiBase))
	repoList, err := resolveRepos(ctx, adapter, *repos, *org, usingApp)
	if err != nil {
		fmt.Fprintf(stderr, "coderepute: enumerate repos: %v\n", err)
		return 1
	}
	activity, err := adapter.FetchActivity(ctx, provider.FetchOptions{
		Repos:   repoList,
		Subject: *subject,
		Window:  window,
	})
	if err != nil {
		fmt.Fprintf(stderr, "coderepute: fetch: %v\n", err)
		return 1
	}

	result := metrics.Compute(activity)
	r := report.Build(activity, &result.Collaboration, &result.Cadence, time.Now(),
		report.WithTokenScopeClass(github.ClassifyToken(*token, activity.TokenScope)))
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


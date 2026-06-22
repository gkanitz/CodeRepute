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
	"github.com/grkanitz/coderepute/provider/gitlab"
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
		platform = fs.String("platform", "github", "platform to report on: github or gitlab")

		// GitHub flags
		repos          = fs.String("repo", "", "repository to cover, owner/name (comma-separated for several)")
		org            = fs.String("org", "", "GitHub organization to cover: every repo visible to the token (alternative to -repo)")
		token          = fs.String("token", "", "GitHub token (defaults to GITHUB_TOKEN)")
		appID          = fs.String("app-id", "", "GitHub App ID; with -app-key, mints an installation token instead of -token")
		appKey         = fs.String("app-key", "", "path to the GitHub App private key PEM")
		installationID = fs.Int64("installation-id", 0, "GitHub App installation to act as (default: the sole installation)")
		apiBase        = fs.String("api-base", "https://api.github.com", "GitHub API base URL")

		// GitLab flags
		gitlabToken   = fs.String("gitlab-token", "", "GitLab group access token (defaults to GITLAB_TOKEN)")
		group         = fs.String("group", "", "GitLab group to cover: every project visible to the token")
		gitlabAPIBase = fs.String("gitlab-api-base", "https://gitlab.com/api/v4", "GitLab API base URL (include /api/v4 suffix)")

		// Common flags
		subject    = fs.String("subject", "", "platform username the report is about")
		windowDays = fs.Int("window-days", 0, "report window ending now, in days (0 = all time / no lower bound)")
		outDir     = fs.String("out", ".", "output directory for report.json and report.html")
		cacheFile  = fs.String("cache", "", "path to cache file: write activity JSON on first run, read it on subsequent runs (skips all API calls)")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *windowDays < 0 {
		fmt.Fprintln(stderr, "coderepute: -window-days must be zero (all time) or positive")
		return 2
	}
	if *subject == "" {
		fmt.Fprintln(stderr, "coderepute: -subject is required")
		return 2
	}

	until := time.Now().UTC()
	var window provider.Window
	if *windowDays > 0 {
		window = provider.Window{Since: until.AddDate(0, 0, -*windowDays), Until: until}
	} else {
		// windowDays == 0: fetch all available history, no lower bound.
		window = provider.Window{Until: until}
	}

	switch *platform {
	case "gitlab":
		return runGitLab(stderr, *gitlabToken, *gitlabAPIBase, *group, *repos, *subject, *outDir, *cacheFile, window, getenv)
	case "github":
		return runGitHub(stderr, *token, *apiBase, *repos, *org, *subject, *outDir, *cacheFile, *appID, *appKey, *installationID, window, getenv)
	default:
		fmt.Fprintf(stderr, "coderepute: unknown -platform %q: must be github or gitlab\n", *platform)
		return 2
	}
}

func runGitHub(stderr io.Writer, token, apiBase, repos, org, subject, outDir, cacheFile, appID, appKey string, installationID int64, window provider.Window, getenv func(string) string) int {
	if token == "" {
		token = getenv("GITHUB_TOKEN")
	}
	usingApp := appID != "" || appKey != ""
	cacheHit := cacheFile != "" && func() bool { _, err := os.Stat(cacheFile); return err == nil }()
	switch {
	case usingApp && (appID == "" || appKey == ""):
		fmt.Fprintln(stderr, "coderepute: -app-id and -app-key must be given together")
		return 2
	case repos == "" && org == "" && !usingApp && !cacheHit:
		fmt.Fprintln(stderr, "coderepute: -repo or -org is required")
		return 2
	case token == "" && !usingApp && !cacheHit:
		fmt.Fprintln(stderr, "coderepute: a token is required (-token, GITHUB_TOKEN, or -app-id/-app-key)")
		return 2
	}

	ctx := context.Background()
	if usingApp {
		minted, err := mintInstallationToken(ctx, appID, appKey, installationID, apiBase)
		if err != nil {
			fmt.Fprintf(stderr, "coderepute: app token: %v\n", err)
			return 1
		}
		token = minted
	}

	adapter := github.New(token, github.WithBaseURL(apiBase))

	activity, err := loadOrFetch(stderr, cacheFile, func() (provider.ActivitySet, error) {
		repoList, err := resolveRepos(ctx, adapter, repos, org, usingApp)
		if err != nil {
			return provider.ActivitySet{}, fmt.Errorf("enumerate repos: %w", err)
		}
		return adapter.FetchActivity(ctx, provider.FetchOptions{
			Repos:   repoList,
			Subject: subject,
			Window:  window,
		})
	})
	if err != nil {
		fmt.Fprintf(stderr, "coderepute: fetch: %v\n", err)
		return 1
	}

	result := metrics.Compute(activity)
	r := report.Build(activity, &result.Collaboration, &result.Cadence, time.Now(),
		report.WithTokenScopeClass(github.ClassifyToken(token, activity.TokenScope)))
	if v := report.CIVerification(getenv); v != nil {
		r.Verification = v
	} else if v := report.GitLabVerification(getenv); v != nil {
		r.Verification = v
	}
	return writeReport(stderr, &r, outDir)
}

func runGitLab(stderr io.Writer, token, apiBase, group, repos, subject, outDir, cacheFile string, window provider.Window, getenv func(string) string) int {
	if token == "" {
		token = getenv("GITLAB_TOKEN")
	}
	switch {
	case token == "":
		fmt.Fprintln(stderr, "coderepute: a token is required (-gitlab-token or GITLAB_TOKEN)")
		return 2
	case repos == "" && group == "":
		fmt.Fprintln(stderr, "coderepute: -repo or -group is required for GitLab")
		return 2
	}

	ctx := context.Background()
	adapter := gitlab.New(token, gitlab.WithBaseURL(apiBase))

	activity, err := loadOrFetch(stderr, cacheFile, func() (provider.ActivitySet, error) {
		var repoList []string
		if repos != "" {
			for _, r := range strings.Split(repos, ",") {
				if r = strings.TrimSpace(r); r != "" {
					repoList = append(repoList, r)
				}
			}
		} else {
			var err error
			repoList, err = adapter.ListGroupProjects(ctx, group)
			if err != nil {
				return provider.ActivitySet{}, fmt.Errorf("enumerate projects: %w", err)
			}
		}
		return adapter.FetchActivity(ctx, provider.FetchOptions{
			Repos:   repoList,
			Subject: subject,
			Window:  window,
		})
	})
	if err != nil {
		fmt.Fprintf(stderr, "coderepute: fetch: %v\n", err)
		return 1
	}

	result := metrics.Compute(activity)
	r := report.Build(activity, &result.Collaboration, &result.Cadence, time.Now(),
		report.WithTokenScopeClass(gitlab.ClassifyToken(token, activity.TokenScope)))
	return writeReport(stderr, &r, outDir)
}

// loadOrFetch returns a cached ActivitySet from cachePath if the file exists,
// otherwise calls fetch(), writes the result to cachePath (when set), and
// returns it. Delete the cache file to force a fresh API fetch.
func loadOrFetch(stderr io.Writer, cachePath string, fetch func() (provider.ActivitySet, error)) (provider.ActivitySet, error) {
	if cachePath != "" {
		if data, err := os.ReadFile(cachePath); err == nil {
			var as provider.ActivitySet
			if err := json.Unmarshal(data, &as); err == nil {
				fmt.Fprintf(stderr, "using cached activity from %s\n", cachePath)
				return as, nil
			}
		}
	}

	as, err := fetch()
	if err != nil {
		return provider.ActivitySet{}, err
	}

	if cachePath != "" {
		data, merr := json.MarshalIndent(as, "", "  ")
		if merr == nil {
			if werr := os.WriteFile(cachePath, append(data, '\n'), 0o644); werr == nil {
				fmt.Fprintf(stderr, "cached activity to %s\n", cachePath)
			}
		}
	}
	return as, nil
}

func writeReport(stderr io.Writer, r *report.Report, outDir string) int {
	if err := r.Validate(); err != nil {
		fmt.Fprintf(stderr, "coderepute: built an invalid report: %v\n", err)
		return 1
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintf(stderr, "coderepute: %v\n", err)
		return 1
	}
	rawJSON, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "coderepute: encode report: %v\n", err)
		return 1
	}
	if err := os.WriteFile(filepath.Join(outDir, "report.json"), append(rawJSON, '\n'), 0o644); err != nil {
		fmt.Fprintf(stderr, "coderepute: %v\n", err)
		return 1
	}
	rawHTML, err := render.HTML(*r)
	if err != nil {
		fmt.Fprintf(stderr, "coderepute: render: %v\n", err)
		return 1
	}
	if err := os.WriteFile(filepath.Join(outDir, "report.html"), rawHTML, 0o644); err != nil {
		fmt.Fprintf(stderr, "coderepute: %v\n", err)
		return 1
	}

	fmt.Fprintf(stderr, "wrote %s and %s\n",
		filepath.Join(outDir, "report.json"), filepath.Join(outDir, "report.html"))
	return 0
}

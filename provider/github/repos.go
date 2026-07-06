package github

// Repository enumeration for org-scoped coverage. One run covers every
// repository the token can see; the resulting list feeds the report's
// coverage stamp so omissions stay visible.
//
// Like the rest of the adapter this file reads only API metadata — repo
// names — never repository contents.

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/gkanitz/coderepute/provider"
)

type apiRepo struct {
	FullName string `json:"full_name"`
}

// ListOrgRepos returns every repository of org visible to the token, as
// "owner/name", following pagination to exhaustion.
func (a *Adapter) ListOrgRepos(ctx context.Context, org string) ([]string, error) {
	var out []string
	url := fmt.Sprintf("%s/orgs/%s/repos?per_page=100", a.baseURL, org)
	for url != "" {
		var page []apiRepo
		resp, err := a.getJSON(ctx, url, &page)
		if err != nil {
			return nil, fmt.Errorf("github: list repos of org %q: %w", org, err)
		}
		for _, r := range page {
			out = append(out, r.FullName)
		}
		url = nextPage(resp.Header.Get("Link"))
	}
	return out, nil
}

// ListInstallationRepos returns every repository accessible to the App
// installation token the adapter authenticates with, as "owner/name",
// following pagination to exhaustion.
func (a *Adapter) ListInstallationRepos(ctx context.Context) ([]string, error) {
	var out []string
	url := a.baseURL + "/installation/repositories?per_page=100"
	for url != "" {
		var page struct {
			Repositories []apiRepo `json:"repositories"`
		}
		resp, err := a.getJSON(ctx, url, &page)
		if err != nil {
			return nil, fmt.Errorf("github: list installation repos: %w", err)
		}
		for _, r := range page.Repositories {
			out = append(out, r.FullName)
		}
		url = nextPage(resp.Header.Get("Link"))
	}
	return out, nil
}

// searchItem is a single result from the GitHub Search Issues/PRs API.
// Only repository_url is extracted — no PR body, title, diff, or patch
// content is ever read. This is consistent with the adapter's metadata-only
// constraint.
type searchItem struct {
	RepositoryURL string `json:"repository_url"`
}

// searchResponse is the top-level Search API response envelope.
type searchResponse struct {
	TotalCount        int          `json:"total_count"`
	IncompleteResults bool         `json:"incomplete_results"`
	Items             []searchItem `json:"items"`
}

// repoNameFromSearchURL extracts "owner/name" from a Search API item's
// repository_url field, which has the form
// "https://api.github.com/repos/owner/name".
func repoNameFromSearchURL(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	path := strings.TrimPrefix(u.Path, "/repos/")
	parts := strings.SplitN(path, "/", 3)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", fmt.Errorf("cannot extract repo name from %q", rawURL)
	}
	return parts[0] + "/" + parts[1], nil
}

// ListContributedRepos discovers repositories where the subject has authored
// a PR or given a review within the given window, using the GitHub Search API.
//
// It runs two queries:
//   - "type:pr author:{subject}" filtered by created date (exact — the PR
//     creation date is the authoritative timestamp for authorship).
//   - "type:pr reviewed-by:{subject}" filtered by updated date (approximate —
//     the Search API does not expose a "review submitted" date filter; using
//     updated: as a proxy is the closest available qualifier, and will include
//     PRs whose last update (any event) falls in the window).
//
// Results are de-duplicated across both queries and sorted for determinism.
// Only repo metadata (the repository_url field) is fetched — never PR titles,
// bodies, diffs, or patch content.
func (a *Adapter) ListContributedRepos(ctx context.Context, subject string, window provider.Window) ([]string, error) {
	seen := map[string]bool{}
	var out []string

	for _, q := range []string{buildAuthorQuery(subject, window), buildReviewQuery(subject, window)} {
		searchURL := fmt.Sprintf("%s/search/issues?q=%s&per_page=100", a.baseURL, url.QueryEscape(q))
		for searchURL != "" {
			var page searchResponse
			resp, err := a.getJSON(ctx, searchURL, &page)
			if err != nil {
				return nil, fmt.Errorf("github: search issues (%s): %w", q, err)
			}
			for _, item := range page.Items {
				repo, err := repoNameFromSearchURL(item.RepositoryURL)
				if err != nil {
					continue // skip malformed URLs
				}
				if !seen[repo] {
					seen[repo] = true
					out = append(out, repo)
				}
			}
			searchURL = nextPage(resp.Header.Get("Link"))
		}
	}

	sort.Strings(out)
	return out, nil
}

// buildAuthorQuery builds the Search API query for PRs authored by subject
// within the given window, filtered by PR creation date.
func buildAuthorQuery(subject string, window provider.Window) string {
	q := fmt.Sprintf("type:pr author:%s created:%s..%s", subject, dateStr(window.Since), dateStr(window.Until))
	if window.Since.IsZero() {
		q = fmt.Sprintf("type:pr author:%s created:<%s", subject, dateStr(window.Until))
	}
	return q
}

// buildReviewQuery builds the Search API query for PRs reviewed by subject
// within the given window, filtered by PR last-updated date.
//
// The Search API does not expose a "review submitted at" date qualifier, so
// updated: is used as an approximation: a PR that received a review in the
// window will generally have its updated_at timestamp bumped into the window.
func buildReviewQuery(subject string, window provider.Window) string {
	q := fmt.Sprintf("type:pr reviewed-by:%s updated:%s..%s", subject, dateStr(window.Since), dateStr(window.Until))
	if window.Since.IsZero() {
		q = fmt.Sprintf("type:pr reviewed-by:%s updated:<%s", subject, dateStr(window.Until))
	}
	return q
}

// dateStr formats a time.Time as YYYY-MM-DD for use in Search API date
// qualifiers. A zero time returns "1970-01-01" (far past, effectively no
// lower bound).
func dateStr(t time.Time) string {
	if t.IsZero() {
		return "1970-01-01"
	}
	return t.Format("2006-01-02")
}

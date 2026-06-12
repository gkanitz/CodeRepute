// Package github adapts the GitHub REST API to the provider port.
//
// The adapter reads only API metadata: user lookups and pull request
// listings. It never clones repositories or requests file contents, and it
// attributes activity by the immutable account ID, never git author emails.
package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/grkanitz/coderepute/provider"
)

const defaultBaseURL = "https://api.github.com"

// Adapter implements provider.Provider for GitHub.
type Adapter struct {
	baseURL string
	token   string
	httpc   *http.Client
}

// Option configures an Adapter.
type Option func(*Adapter)

// WithBaseURL points the adapter at a different API root (GHES, tests).
func WithBaseURL(url string) Option {
	return func(a *Adapter) { a.baseURL = url }
}

// WithHTTPClient replaces the underlying HTTP client.
func WithHTTPClient(c *http.Client) Option {
	return func(a *Adapter) { a.httpc = c }
}

// New returns a GitHub adapter authenticating with the given token.
func New(token string, opts ...Option) *Adapter {
	a := &Adapter{
		baseURL: defaultBaseURL,
		token:   token,
		httpc:   &http.Client{Timeout: 30 * time.Second},
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

type apiUser struct {
	Login string `json:"login"`
	ID    int64  `json:"id"`
}

type apiPull struct {
	User      apiUser    `json:"user"`
	CreatedAt time.Time  `json:"created_at"`
	MergedAt  *time.Time `json:"merged_at"`
	ClosedAt  *time.Time `json:"closed_at"`
}

// FetchActivity resolves the subject to an account ID and collects the
// subject's pull requests in the window across the given repos.
func (a *Adapter) FetchActivity(ctx context.Context, opts provider.FetchOptions) (provider.ActivitySet, error) {
	subject, tokenScope, err := a.resolveSubject(ctx, opts.Subject)
	if err != nil {
		return provider.ActivitySet{}, err
	}

	as := provider.ActivitySet{
		Subject:    subject,
		Window:     opts.Window,
		Repos:      opts.Repos,
		TokenScope: tokenScope,
	}

	subjectID, err := strconv.ParseInt(subject.AccountID, 10, 64)
	if err != nil {
		return provider.ActivitySet{}, fmt.Errorf("github: invalid account id %q: %w", subject.AccountID, err)
	}

	for _, repo := range opts.Repos {
		prs, err := a.fetchPulls(ctx, repo, subjectID, opts.Window)
		if err != nil {
			return provider.ActivitySet{}, err
		}
		as.PullRequests = append(as.PullRequests, prs...)
	}
	return as, nil
}

func (a *Adapter) resolveSubject(ctx context.Context, username string) (provider.Subject, string, error) {
	var user apiUser
	resp, err := a.getJSON(ctx, fmt.Sprintf("%s/users/%s", a.baseURL, username), &user)
	if err != nil {
		return provider.Subject{}, "", fmt.Errorf("github: resolve subject %q: %w", username, err)
	}
	return provider.Subject{
		Platform:  "github",
		Username:  user.Login,
		AccountID: strconv.FormatInt(user.ID, 10),
	}, resp.Header.Get("X-OAuth-Scopes"), nil
}

func (a *Adapter) fetchPulls(ctx context.Context, repo string, subjectID int64, window provider.Window) ([]provider.PullRequest, error) {
	var out []provider.PullRequest
	url := fmt.Sprintf("%s/repos/%s/pulls?state=all&per_page=100", a.baseURL, repo)
	for url != "" {
		var page []apiPull
		resp, err := a.getJSON(ctx, url, &page)
		if err != nil {
			return nil, fmt.Errorf("github: list pulls for %s: %w", repo, err)
		}
		for _, p := range page {
			if p.User.ID != subjectID {
				continue // bound to account ID, not login or email
			}
			if p.CreatedAt.Before(window.Since) || !p.CreatedAt.Before(window.Until) {
				continue
			}
			out = append(out, provider.PullRequest{
				Repo:      repo,
				CreatedAt: p.CreatedAt,
				MergedAt:  p.MergedAt,
				ClosedAt:  p.ClosedAt,
			})
		}
		url = nextPage(resp.Header.Get("Link"))
	}
	return out, nil
}

func (a *Adapter) getJSON(ctx context.Context, url string, v any) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+a.token)

	resp, err := a.httpc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s: %s", url, resp.Status, body)
	}
	if err := json.Unmarshal(body, v); err != nil {
		return nil, fmt.Errorf("GET %s: decode: %w", url, err)
	}
	return resp, nil
}

var nextLinkRe = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)

func nextPage(linkHeader string) string {
	if m := nextLinkRe.FindStringSubmatch(linkHeader); m != nil {
		return m[1]
	}
	return ""
}

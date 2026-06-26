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
	"strings"
	"time"

	"github.com/gkanitz/coderepute/provider"
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
	Number    int64      `json:"number"`
	User      apiUser    `json:"user"`
	CreatedAt time.Time  `json:"created_at"`
	MergedAt  *time.Time `json:"merged_at"`
	ClosedAt  *time.Time `json:"closed_at"`
}

type apiReview struct {
	User        apiUser   `json:"user"`
	State       string    `json:"state"`
	SubmittedAt time.Time `json:"submitted_at"`
}

type apiReviewComment struct {
	User           apiUser   `json:"user"`
	CreatedAt      time.Time `json:"created_at"`
	PullRequestURL string    `json:"pull_request_url"`
}

// FetchActivity resolves the subject to an account ID and collects the
// subject's activity in the window across the given repos: authored PRs
// with review timing, reviews given on others' PRs, and review comments
// written and received.
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
		if err := a.fetchRepoActivity(ctx, repo, subjectID, opts.Window, &as); err != nil {
			return provider.ActivitySet{}, err
		}
	}
	return as, nil
}

// pendingReview holds a subject review collected during the PR pass, before
// the per-PR inline comment counts are known.
type pendingReview struct {
	prNumber    int64
	submittedAt time.Time
	state       string
}

// fetchRepoActivity collects one repo's activity into the set. Everything
// is reduced to subject-only data before it leaves the adapter: colleague
// identities, comment bodies, and PR titles are never copied out of the
// API responses.
func (a *Adapter) fetchRepoActivity(ctx context.Context, repo string, subjectID int64, window provider.Window, as *provider.ActivitySet) error {
	pulls, err := a.fetchPulls(ctx, repo, window)
	if err != nil {
		return err
	}

	subjectPRs := map[int64]bool{}
	var pending []pendingReview
	for _, p := range pulls {
		reviews, err := a.fetchReviews(ctx, repo, p.Number)
		if err != nil {
			return err
		}
		if p.User.ID == subjectID { // bound to account ID, not login or email
			subjectPRs[p.Number] = true
			as.PullRequests = append(as.PullRequests, subjectPull(repo, p, reviews, subjectID))
			continue
		}
		// Someone else's PR: only the subject's in-window reviews matter.
		for _, rv := range reviews {
			if rv.User.ID != subjectID || !inWindow(rv.SubmittedAt, window) {
				continue
			}
			pending = append(pending, pendingReview{
				prNumber:    p.Number,
				submittedAt: rv.SubmittedAt,
				state:       rv.State,
			})
		}
	}

	// Fetch inline review comments; this also returns subject's comment count
	// per PR so we can annotate each review with its CommentCount.
	commentCounts, err := a.fetchReviewComments(ctx, repo, subjectID, subjectPRs, window, as)
	if err != nil {
		return err
	}

	for _, rv := range pending {
		as.ReviewsGiven = append(as.ReviewsGiven, provider.Review{
			Repo:         repo,
			SubmittedAt:  rv.submittedAt,
			State:        rv.state,
			CommentCount: commentCounts[rv.prNumber],
		})
	}
	return nil
}

// subjectPull reduces a subject-authored PR and its reviews to the
// provider model: when someone else first reviewed it and how many
// reviews requested changes. The subject's own reviews never count.
func subjectPull(repo string, p apiPull, reviews []apiReview, subjectID int64) provider.PullRequest {
	pr := provider.PullRequest{
		Repo:      repo,
		CreatedAt: p.CreatedAt,
		MergedAt:  p.MergedAt,
		ClosedAt:  p.ClosedAt,
	}
	for _, rv := range reviews {
		if rv.User.ID == subjectID {
			continue
		}
		if pr.FirstReviewAt == nil || rv.SubmittedAt.Before(*pr.FirstReviewAt) {
			at := rv.SubmittedAt
			pr.FirstReviewAt = &at
		}
		if rv.State == "CHANGES_REQUESTED" {
			pr.ChangesRequested++
		}
	}
	return pr
}

func inWindow(t time.Time, w provider.Window) bool {
	if !w.Since.IsZero() && t.Before(w.Since) {
		return false
	}
	return t.Before(w.Until)
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

// fetchPulls lists every PR created in the window, regardless of author:
// the subject's own PRs become activity entries, while colleagues' PRs
// are scanned for reviews the subject gave.
func (a *Adapter) fetchPulls(ctx context.Context, repo string, window provider.Window) ([]apiPull, error) {
	var out []apiPull
	url := fmt.Sprintf("%s/repos/%s/pulls?state=all&per_page=100", a.baseURL, repo)
	for url != "" {
		var page []apiPull
		resp, err := a.getJSON(ctx, url, &page)
		if err != nil {
			return nil, fmt.Errorf("github: list pulls for %s: %w", repo, err)
		}
		for _, p := range page {
			if inWindow(p.CreatedAt, window) {
				out = append(out, p)
			}
		}
		url = nextPage(resp.Header.Get("Link"))
	}
	return out, nil
}

func (a *Adapter) fetchReviews(ctx context.Context, repo string, number int64) ([]apiReview, error) {
	var out []apiReview
	url := fmt.Sprintf("%s/repos/%s/pulls/%d/reviews?per_page=100", a.baseURL, repo, number)
	for url != "" {
		var page []apiReview
		resp, err := a.getJSON(ctx, url, &page)
		if err != nil {
			return nil, fmt.Errorf("github: list reviews for %s#%d: %w", repo, number, err)
		}
		out = append(out, page...)
		url = nextPage(resp.Header.Get("Link"))
	}
	return out, nil
}

// fetchReviewComments walks the repo's review comments once and splits
// them into comments the subject wrote and comments others left on the
// subject's PRs. Bodies and authors are dropped; only repo and timestamp
// survive.
//
// It returns a map from PR number to the count of subject inline comments
// on colleague PRs (i.e. PRs not authored by the subject), so that callers
// can annotate review events with CommentCount.
func (a *Adapter) fetchReviewComments(ctx context.Context, repo string, subjectID int64, subjectPRs map[int64]bool, window provider.Window, as *provider.ActivitySet) (map[int64]int, error) {
	commentCounts := map[int64]int{}
	url := fmt.Sprintf("%s/repos/%s/pulls/comments?per_page=100", a.baseURL, repo)
	for url != "" {
		var page []apiReviewComment
		resp, err := a.getJSON(ctx, url, &page)
		if err != nil {
			return nil, fmt.Errorf("github: list review comments for %s: %w", repo, err)
		}
		for _, c := range page {
			if !inWindow(c.CreatedAt, window) {
				continue
			}
			prNum := pullNumberFromURL(c.PullRequestURL)
			comment := provider.ReviewComment{Repo: repo, CreatedAt: c.CreatedAt}
			switch {
			case c.User.ID == subjectID:
				as.ReviewCommentsWritten = append(as.ReviewCommentsWritten, comment)
				if !subjectPRs[prNum] {
					// Subject commented on a colleague's PR — count it toward
					// that PR's review comment count.
					commentCounts[prNum]++
				}
			case subjectPRs[prNum]:
				as.ReviewCommentsReceived = append(as.ReviewCommentsReceived, comment)
			}
		}
		url = nextPage(resp.Header.Get("Link"))
	}
	return commentCounts, nil
}

// pullNumberFromURL extracts the PR number from an API pull_request_url
// like ".../repos/owner/name/pulls/42". Returns 0 when unparsable.
func pullNumberFromURL(url string) int64 {
	idx := strings.LastIndexByte(url, '/')
	if idx < 0 {
		return 0
	}
	n, err := strconv.ParseInt(url[idx+1:], 10, 64)
	if err != nil {
		return 0
	}
	return n
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

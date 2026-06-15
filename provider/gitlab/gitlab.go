// Package gitlab adapts the GitLab REST API (v4) to the provider port.
//
// The adapter reads only API metadata: user lookups, merge request
// listings, and notes. It never clones repositories or requests file
// contents, and it attributes activity by the immutable account ID,
// never git author emails.
package gitlab

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

	"github.com/grkanitz/coderepute/provider"
)

const defaultBaseURL = "https://gitlab.com/api/v4"

// Adapter implements provider.Provider for GitLab.
type Adapter struct {
	baseURL string
	token   string
	httpc   *http.Client
}

// Option configures an Adapter.
type Option func(*Adapter)

// WithBaseURL points the adapter at a different API root (self-managed
// GitLab, tests). The URL must include the /api/v4 prefix.
func WithBaseURL(url string) Option {
	return func(a *Adapter) { a.baseURL = url }
}

// WithHTTPClient replaces the underlying HTTP client.
func WithHTTPClient(c *http.Client) Option {
	return func(a *Adapter) { a.httpc = c }
}

// New returns a GitLab adapter authenticating with the given token.
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
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

type apiMergeRequest struct {
	IID       int64      `json:"iid"`
	Author    apiUser    `json:"author"`
	CreatedAt time.Time  `json:"created_at"`
	MergedAt  *time.Time `json:"merged_at"`
	ClosedAt  *time.Time `json:"closed_at"`
}

// apiNote is one MR note. GitLab records both human comments and review
// lifecycle events ("approved this merge request", "requested changes")
// as notes; system marks the latter, Type marks diff-anchored comments.
type apiNote struct {
	Type      *string   `json:"type"`
	System    bool      `json:"system"`
	Author    apiUser   `json:"author"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

// Review semantics: GitLab has no first-class review object. Approval
// and requested-changes events surface as system notes with these exact
// bodies; the adapter normalizes them to the provider-neutral review
// states GitHub reviews carry ("APPROVED", "CHANGES_REQUESTED").
// GitLab has no comment-only review object, so no "COMMENTED" state is
// ever produced; diff notes count as review comments instead.
const (
	systemNoteApproved         = "approved this merge request"
	systemNoteRequestedChanges = "requested changes"
)

// reviewState maps a system note to a normalized review state, or ""
// when the note is not a review event (e.g. label changes, unapprovals —
// GitHub keeps dismissed approvals in its review list, and GitLab keeps
// the original "approved" note after an unapproval, so unapprove events
// are ignored for symmetry).
func reviewState(n apiNote) string {
	if !n.System {
		return ""
	}
	switch n.Body {
	case systemNoteApproved:
		return "APPROVED"
	case systemNoteRequestedChanges:
		return "CHANGES_REQUESTED"
	default:
		return ""
	}
}

// isDiffComment reports whether a note is a human diff-anchored comment —
// the GitLab equivalent of a GitHub pull request review comment. Plain
// discussion notes (type null or "DiscussionNote") map to GitHub issue
// comments on PRs, which the GitHub adapter does not count either.
func isDiffComment(n apiNote) bool {
	return !n.System && n.Type != nil && *n.Type == "DiffNote"
}

// FetchActivity resolves the subject to an account ID and collects the
// subject's activity in the window across the given projects.
func (a *Adapter) FetchActivity(ctx context.Context, opts provider.FetchOptions) (provider.ActivitySet, error) {
	subject, err := a.resolveSubject(ctx, opts.Subject)
	if err != nil {
		return provider.ActivitySet{}, err
	}

	as := provider.ActivitySet{
		Subject:    subject,
		Window:     opts.Window,
		Repos:      opts.Repos,
		TokenScope: a.tokenScope(ctx),
	}

	subjectID, err := strconv.ParseInt(subject.AccountID, 10, 64)
	if err != nil {
		return provider.ActivitySet{}, fmt.Errorf("gitlab: invalid account id %q: %w", subject.AccountID, err)
	}

	for _, repo := range opts.Repos {
		if err := a.fetchProjectActivity(ctx, repo, subjectID, opts.Window, &as); err != nil {
			return provider.ActivitySet{}, err
		}
	}
	return as, nil
}

// fetchProjectActivity collects one project's activity into the set.
// Everything is reduced to subject-only data before it leaves the
// adapter: colleague identities, note bodies, and MR titles are never
// copied out of the API responses.
func (a *Adapter) fetchProjectActivity(ctx context.Context, repo string, subjectID int64, window provider.Window, as *provider.ActivitySet) error {
	mrs, err := a.fetchMergeRequests(ctx, repo, window)
	if err != nil {
		return err
	}
	for _, mr := range mrs {
		notes, err := a.fetchNotes(ctx, repo, mr.IID)
		if err != nil {
			return err
		}
		subjectMR := mr.Author.ID == subjectID // bound to account ID, not username or email

		// Diff comments split into written (by the subject, anywhere)
		// and received (by others, on the subject's MRs). Bodies and
		// authors are dropped; only repo and timestamp survive.
		for _, n := range notes {
			if !isDiffComment(n) || !inWindow(n.CreatedAt, window) {
				continue
			}
			comment := provider.ReviewComment{Repo: repo, CreatedAt: n.CreatedAt}
			switch {
			case n.Author.ID == subjectID:
				as.ReviewCommentsWritten = append(as.ReviewCommentsWritten, comment)
			case subjectMR:
				as.ReviewCommentsReceived = append(as.ReviewCommentsReceived, comment)
			}
		}

		if subjectMR {
			as.PullRequests = append(as.PullRequests, subjectMergeRequest(repo, mr, notes, subjectID))
			continue
		}
		// Someone else's MR: only the subject's in-window review events
		// (approvals, requested changes) matter.
		for _, n := range notes {
			state := reviewState(n)
			if state == "" || n.Author.ID != subjectID || !inWindow(n.CreatedAt, window) {
				continue
			}
			as.ReviewsGiven = append(as.ReviewsGiven, provider.Review{
				Repo:        repo,
				SubmittedAt: n.CreatedAt,
				State:       state,
			})
		}
	}
	return nil
}

// subjectMergeRequest reduces a subject-authored MR and its notes to the
// provider model: when someone else first reviewed it and how many
// review events requested changes. The subject's own notes never count.
//
// FirstReviewAt considers others' review events (approvals, requested
// changes) and others' diff comments. The latter mirrors GitHub, where
// leaving diff comments creates a COMMENTED review that counts as a
// first review; on GitLab the diff note itself is the only trace.
func subjectMergeRequest(repo string, mr apiMergeRequest, notes []apiNote, subjectID int64) provider.PullRequest {
	closedAt := mr.ClosedAt
	if closedAt == nil {
		closedAt = mr.MergedAt // GitLab sets merged_at but not closed_at for merged MRs; normalise to GitHub semantics
	}
	pr := provider.PullRequest{
		Repo:      repo,
		CreatedAt: mr.CreatedAt,
		MergedAt:  mr.MergedAt,
		ClosedAt:  closedAt,
	}
	for _, n := range notes {
		if n.Author.ID == subjectID {
			continue
		}
		state := reviewState(n)
		if state == "" && !isDiffComment(n) {
			continue
		}
		if pr.FirstReviewAt == nil || n.CreatedAt.Before(*pr.FirstReviewAt) {
			at := n.CreatedAt
			pr.FirstReviewAt = &at
		}
		if state == "CHANGES_REQUESTED" {
			pr.ChangesRequested++
		}
	}
	return pr
}

func (a *Adapter) fetchNotes(ctx context.Context, repo string, iid int64) ([]apiNote, error) {
	var out []apiNote
	url := fmt.Sprintf("%s/projects/%s/merge_requests/%d/notes?per_page=100", a.baseURL, projectPath(repo), iid)
	for url != "" {
		var page []apiNote
		next, err := a.getJSONPage(ctx, url, &page)
		if err != nil {
			return nil, fmt.Errorf("gitlab: list notes for %s!%d: %w", repo, iid, err)
		}
		out = append(out, page...)
		url = next
	}
	return out, nil
}

// fetchMergeRequests lists every MR created in the window, regardless of
// author: the subject's own MRs become activity entries, while
// colleagues' MRs are scanned for the subject's review involvement.
func (a *Adapter) fetchMergeRequests(ctx context.Context, repo string, window provider.Window) ([]apiMergeRequest, error) {
	var out []apiMergeRequest
	url := fmt.Sprintf("%s/projects/%s/merge_requests?scope=all&per_page=100", a.baseURL, projectPath(repo))
	for url != "" {
		var page []apiMergeRequest
		next, err := a.getJSONPage(ctx, url, &page)
		if err != nil {
			return nil, fmt.Errorf("gitlab: list merge requests for %s: %w", repo, err)
		}
		for _, mr := range page {
			if inWindow(mr.CreatedAt, window) {
				out = append(out, mr)
			}
		}
		url = next
	}
	return out, nil
}

// projectPath URL-encodes a "group/project" repo name into GitLab's
// project path form ("group%2Fproject").
func projectPath(repo string) string {
	return strings.ReplaceAll(repo, "/", "%2F")
}

func inWindow(t time.Time, w provider.Window) bool {
	return !t.Before(w.Since) && t.Before(w.Until)
}

// resolveSubject binds the report subject to GitLab's immutable numeric
// account ID via the exact-match username lookup. GitLab has no
// per-username GET endpoint; /users?username= returns a list that is
// empty for unknown users.
func (a *Adapter) resolveSubject(ctx context.Context, username string) (provider.Subject, error) {
	var users []apiUser
	if err := a.getJSON(ctx, fmt.Sprintf("%s/users?username=%s", a.baseURL, username), &users); err != nil {
		return provider.Subject{}, fmt.Errorf("gitlab: resolve subject %q: %w", username, err)
	}
	if len(users) == 0 {
		return provider.Subject{}, fmt.Errorf("gitlab: subject %q: no such user", username)
	}
	return provider.Subject{
		Platform:  "gitlab",
		Username:  users[0].Username,
		AccountID: strconv.FormatInt(users[0].ID, 10),
	}, nil
}

// tokenScope reports the credential's scopes for the coverage stamp.
// Unlike GitHub, GitLab sends no scope header; personal, group, and
// project access tokens expose their scopes via
// /personal_access_tokens/self. Credentials without that endpoint
// (e.g. OAuth tokens) yield an empty scope string rather than an error —
// the coverage stamp stays honest about what is unknown.
func (a *Adapter) tokenScope(ctx context.Context) string {
	var info struct {
		Scopes []string `json:"scopes"`
	}
	if err := a.getJSON(ctx, a.baseURL+"/personal_access_tokens/self", &info); err != nil {
		return ""
	}
	return strings.Join(info.Scopes, ", ")
}

func (a *Adapter) getJSON(ctx context.Context, url string, v any) error {
	_, err := a.getJSONPage(ctx, url, v)
	return err
}

// getJSONPage fetches one page of a paginated listing and returns the
// next page's URL from the Link header, or "" on the last page.
func (a *Adapter) getJSONPage(ctx context.Context, url string, v any) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+a.token)

	resp, err := a.httpc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s: %s: %s", url, resp.Status, body)
	}
	if err := json.Unmarshal(body, v); err != nil {
		return "", fmt.Errorf("GET %s: decode: %w", url, err)
	}
	return nextPage(resp.Header.Get("Link")), nil
}

var nextLinkRe = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)

func nextPage(linkHeader string) string {
	if m := nextLinkRe.FindStringSubmatch(linkHeader); m != nil {
		return m[1]
	}
	return ""
}

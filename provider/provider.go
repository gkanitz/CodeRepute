// Package provider defines the provider-neutral activity model and the
// port interface that platform adapters implement.
package provider

import (
	"context"
	"errors"
	"time"
)

// Subject identifies the developer a report is about. AccountID is the
// platform's immutable account identifier; it, not git author emails, is
// the source of truth for attribution.
type Subject struct {
	Platform  string
	Username  string
	AccountID string
}

// Window is the half-open time window [Since, Until) a fetch covers.
type Window struct {
	Since time.Time
	Until time.Time
}

// ActivitySet is the provider-neutral normalization of one subject's
// activity in a scope and window. It carries the full v1 shape; adapters
// may leave fields unpopulated until the corresponding slice lands.
type ActivitySet struct {
	Subject    Subject
	Window     Window
	Repos      []string
	TokenScope string

	PullRequests           []PullRequest
	ReviewsGiven           []Review
	ReviewCommentsWritten  []ReviewComment
	ReviewCommentsReceived []ReviewComment
}

// PullRequest is a pull/merge request authored by the subject.
type PullRequest struct {
	Repo             string
	CreatedAt        time.Time
	MergedAt         *time.Time
	ClosedAt         *time.Time
	FirstReviewAt    *time.Time
	ChangesRequested int
	// Additions and Deletions hold the total lines changed across all files
	// in the PR, populated from diff-shape data when available. Zero means
	// "unknown / no diff data fetched."
	Additions int
	Deletions int
	// Files is the number of files touched in the PR, populated from
	// diff-shape data when available. Zero means unknown.
	Files int
	// FileStats holds per-file diff shape data for this PR, populated from
	// the GraphQL diff-stats query. Zero-length or nil means no data is
	// available. Each entry's Ext is already reduced to a canonical extension
	// (lowercase, no leading dot, "" for extensionless). Full paths never
	// leave the adapter package.
	FileStats []FileStat `json:"file_stats,omitempty"`
	// Number is the platform-level PR/MR number, used internally for
	// correlating reviews with diff data. Not exposed in reports.
	Number int64
}

// Review is a review the subject submitted on someone else's PR.
type Review struct {
	Repo         string
	SubmittedAt  time.Time
	State        string
	CommentCount int // number of diff/inline comments the subject left on the same PR
	// PRNumber is the platform-level PR/MR number, used internally for
	// correlating reviews with diff data. Not exposed in reports.
	PRNumber int64
	// PRLines is the total additions+deletions of the PR being reviewed,
	// populated from diff-shape data when available. Zero means "unknown /
	// no diff data" and triggers the fallback deep-review threshold.
	PRLines int
}

// ReviewComment is a single review comment written or received by the subject.
type ReviewComment struct {
	Repo      string
	CreatedAt time.Time
}

// FetchOptions scope a fetch to repos, a subject username, and a window.
type FetchOptions struct {
	Repos   []string // "owner/name"
	Subject string
	Window  Window
}

// FileStat holds per-file diff shape metadata for a PR/MR file. The path
// is reduced to an extension inside the adapter before this struct is
// populated, giving a type-level guarantee that full paths never leave
// the adapter package.
type FileStat struct {
	Ext       string // reduced canonical extension (lowercase, no leading dot)
	Additions int
	Deletions int
}

// ErrDiffShapeUnsupported is returned by DiffShape fetch methods when the
// GraphQL endpoint is unavailable (auth failure, network error, 404).
var ErrDiffShapeUnsupported = errors.New("diff shape: unsupported by provider")

// DiffShapeFetcher is an optional interface that platform adapters may
// implement to provide per-PR/MR file statistics without requesting
// patch content or repository file contents.
type DiffShapeFetcher interface {
	FetchDiffShape(ctx context.Context, repo string, number int64) ([]FileStat, error)
}

// Provider fetches the subject's activity using only API metadata.
// Implementations must never clone repositories or read file contents.
type Provider interface {
	FetchActivity(ctx context.Context, opts FetchOptions) (ActivitySet, error)
}

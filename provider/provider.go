// Package provider defines the provider-neutral activity model and the
// port interface that platform adapters implement.
package provider

import (
	"context"
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
}

// Review is a review the subject submitted on someone else's PR.
type Review struct {
	Repo        string
	SubmittedAt time.Time
	State       string
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

// Provider fetches the subject's activity using only API metadata.
// Implementations must never clone repositories or read file contents.
type Provider interface {
	FetchActivity(ctx context.Context, opts FetchOptions) (ActivitySet, error)
}

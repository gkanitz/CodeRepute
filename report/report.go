// Package report defines the canonical, versioned, provider-neutral
// report schema, its builder, and validation.
package report

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/grkanitz/coderepute/provider"
)

// SchemaVersion is the version stamped into every report this build emits.
const SchemaVersion = "v0"

// Verification statuses.
const (
	StatusUnverified = "unverified"
	StatusVerified   = "verified"
)

// Report is the canonical schema-v0 report. Coverage and Verification are
// mandatory; Collaboration and Cadence are optional sections that later
// slices populate.
type Report struct {
	SchemaVersion string         `json:"schema_version"`
	GeneratedAt   time.Time      `json:"generated_at"`
	Subject       Subject        `json:"subject"`
	Coverage      *Coverage      `json:"coverage"`
	Verification  *Verification  `json:"verification"`
	Collaboration *Collaboration `json:"collaboration,omitempty"`
	Cadence       *Cadence       `json:"cadence,omitempty"`
}

// Subject is the developer the report is about, bound to the platform's
// immutable account ID.
type Subject struct {
	Platform  string `json:"platform"`
	Username  string `json:"username"`
	AccountID string `json:"account_id"`
}

// Coverage is the mandatory coverage stamp: which repos, which window,
// and what the token could see. TokenScope carries the raw scopes the
// platform reported; TokenScopeClass names the kind of credential
// (e.g. "app-installation", "classic-pat") so omissions stay visible to
// any reader.
type Coverage struct {
	Repos           []string `json:"repos"`
	Window          Window   `json:"window"`
	TokenScope      string   `json:"token_scope"`
	TokenScopeClass string   `json:"token_scope_class,omitempty"`
}

// Window is the half-open time window [Since, Until) the report covers.
// Since is nil when the report covers all available history (no lower bound).
// Until is always set.
type Window struct {
	Since *time.Time `json:"since,omitempty"`
	Until time.Time  `json:"until"`
}

// Verification is the mandatory verification block. Local runs carry an
// explicit StatusUnverified; CI attestation upgrades it and records the
// producing workflow identity plus a pointer to the attestation.
type Verification struct {
	Status      string       `json:"status"`
	Reason      string       `json:"reason,omitempty"`
	Provider    string       `json:"provider,omitempty"`
	Repository  string       `json:"repository,omitempty"`
	WorkflowRef string       `json:"workflow_ref,omitempty"`
	RunID       string       `json:"run_id,omitempty"`
	RunURL      string       `json:"run_url,omitempty"`
	Attestation *Attestation `json:"attestation,omitempty"`
	// Note is an optional free-text explanation of the verification block,
	// used to document platform-specific attestation limitations honestly.
	Note string `json:"note,omitempty"`
}

// Collaboration holds collaboration metrics. Each sub-struct is owned by
// one metrics concern; follow-up slices add fields here.
type Collaboration struct {
	PullRequests   *PullRequestStats   `json:"pull_requests,omitempty"`
	ReviewsGiven   *ReviewStats        `json:"reviews_given,omitempty"`
	ReviewComments *ReviewCommentStats `json:"review_comments,omitempty"`
	TimeToMerge    *DurationStats      `json:"time_to_merge,omitempty"`

	// TimeToFirstReview covers only the subject's PRs that received at
	// least one review from someone else.
	TimeToFirstReview *DurationStats `json:"time_to_first_review,omitempty"`
	Rework            *ReworkStats   `json:"rework,omitempty"`
}

// ReworkStats describe how often the subject's reviewed PRs needed a
// rework cycle: at least one changes-requested review. The share's
// denominator is reviewed PRs only; the stat is omitted when no PR in
// the window received a review.
type ReworkStats struct {
	ReviewedPRs int     `json:"reviewed_prs"`
	ReworkedPRs int     `json:"reworked_prs"`
	Share       float64 `json:"share"`
}

// DurationStats summarizes a sample of durations in hours over Count
// observations. Omitted entirely when the window holds no observations.
type DurationStats struct {
	Count       int     `json:"count"`
	MedianHours float64 `json:"median_hours"`
}

// ReviewCommentStats are counts of review comments the subject wrote and
// received in the window.
type ReviewCommentStats struct {
	Written  int `json:"written"`
	Received int `json:"received"`
}

// ReviewStats are counts of reviews the subject submitted on other
// people's pull requests in the window, broken down by outcome.
type ReviewStats struct {
	Total            int `json:"total"`
	Approvals        int `json:"approvals"`
	ChangesRequested int `json:"changes_requested"`
	// DeepReviewCount is the number of reviews where the subject left ≥3
	// inline/diff comments (CommentCount ≥ 3 on the provider.Review).
	DeepReviewCount int `json:"deep_review_count,omitempty"`
}

// PullRequestStats are counts of PRs the subject authored in the window.
type PullRequestStats struct {
	Authored int `json:"authored"`
	Merged   int `json:"merged"`
}

// Cadence holds volume/cadence context: how much and how often the
// subject was active inside the coverage window. It is context only —
// never a headline number, and no composite score is derived from it.
type Cadence struct {
	ActiveDays    int           `json:"active_days"`
	Contributions int           `json:"contributions"`
	Trend         []TrendBucket `json:"trend,omitempty"`
}

// TrendBucket is one time bucket of the cadence trend series: a half-open
// [Start, End) slice of the coverage window with per-series event counts.
// First and last buckets may be partial when the window does not align
// with bucket boundaries.
type TrendBucket struct {
	Start  time.Time      `json:"start"`
	End    time.Time      `json:"end"`
	Counts map[string]int `json:"counts"`
}

// BuildOption customizes report assembly beyond what the ActivitySet
// carries.
type BuildOption func(*Report)

// WithTokenScopeClass stamps the coverage block with the credential's
// scope class (e.g. "app-installation").
func WithTokenScopeClass(class string) BuildOption {
	return func(r *Report) { r.Coverage.TokenScopeClass = class }
}

// Build assembles a report from a fetched ActivitySet and computed metric
// sections. Local builds always carry an explicit unverified block.
func Build(as provider.ActivitySet, collab *Collaboration, cadence *Cadence, generatedAt time.Time, opts ...BuildOption) Report {
	var windowSince *time.Time
	if !as.Window.Since.IsZero() {
		s := as.Window.Since
		windowSince = &s
	}
	r := Report{
		SchemaVersion: SchemaVersion,
		GeneratedAt:   generatedAt.UTC(),
		Subject: Subject{
			Platform:  as.Subject.Platform,
			Username:  as.Subject.Username,
			AccountID: as.Subject.AccountID,
		},
		Coverage: &Coverage{
			Repos:      as.Repos,
			Window:     Window{Since: windowSince, Until: as.Window.Until},
			TokenScope: as.TokenScope,
		},
		Verification: &Verification{
			Status: StatusUnverified,
			Reason: "local run; no CI attestation",
		},
		Collaboration: collab,
		Cadence:       cadence,
	}
	for _, opt := range opts {
		opt(&r)
	}
	return r
}

// Validate checks that the report is a well-formed schema-v0 document.
func (r Report) Validate() error {
	if r.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported schema_version %q (want %q)", r.SchemaVersion, SchemaVersion)
	}
	if r.Subject.Platform == "" || r.Subject.Username == "" || r.Subject.AccountID == "" {
		return errors.New("subject must carry platform, username, and immutable account_id")
	}
	if r.Coverage == nil {
		return errors.New("missing coverage stamp")
	}
	if len(r.Coverage.Repos) == 0 {
		return errors.New("coverage stamp must list at least one covered repo")
	}
	if r.Coverage.Window.Until.IsZero() {
		return errors.New("coverage stamp must carry a time window")
	}
	if r.Coverage.Window.Since != nil && !r.Coverage.Window.Since.Before(r.Coverage.Window.Until) {
		return errors.New("coverage window since must precede until")
	}
	if r.Verification == nil {
		return errors.New("missing verification block")
	}
	if r.Verification.Status != StatusUnverified && r.Verification.Status != StatusVerified {
		return fmt.Errorf("verification status %q is not one of %q, %q", r.Verification.Status, StatusUnverified, StatusVerified)
	}
	return nil
}

// Parse unmarshals and validates a report document.
func Parse(raw []byte) (Report, error) {
	var r Report
	if err := json.Unmarshal(raw, &r); err != nil {
		return Report{}, fmt.Errorf("parse report: %w", err)
	}
	if err := r.Validate(); err != nil {
		return Report{}, err
	}
	return r, nil
}

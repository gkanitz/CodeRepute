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
// and what the token could see.
type Coverage struct {
	Repos      []string `json:"repos"`
	Window     Window   `json:"window"`
	TokenScope string   `json:"token_scope"`
}

// Window is the half-open time window [Since, Until) the report covers.
type Window struct {
	Since time.Time `json:"since"`
	Until time.Time `json:"until"`
}

// Verification is the mandatory verification block. Local runs carry an
// explicit StatusUnverified; CI attestation upgrades it.
type Verification struct {
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

// Collaboration holds collaboration metrics. Each sub-struct is owned by
// one metrics concern; follow-up slices add fields here.
type Collaboration struct {
	PullRequests *PullRequestStats `json:"pull_requests,omitempty"`
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

// Build assembles a report from a fetched ActivitySet and computed metric
// sections. Local builds always carry an explicit unverified block.
func Build(as provider.ActivitySet, collab *Collaboration, cadence *Cadence, generatedAt time.Time) Report {
	return Report{
		SchemaVersion: SchemaVersion,
		GeneratedAt:   generatedAt.UTC(),
		Subject: Subject{
			Platform:  as.Subject.Platform,
			Username:  as.Subject.Username,
			AccountID: as.Subject.AccountID,
		},
		Coverage: &Coverage{
			Repos:      as.Repos,
			Window:     Window{Since: as.Window.Since, Until: as.Window.Until},
			TokenScope: as.TokenScope,
		},
		Verification: &Verification{
			Status: StatusUnverified,
			Reason: "local run; no CI attestation",
		},
		Collaboration: collab,
		Cadence:       cadence,
	}
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
	if r.Coverage.Window.Since.IsZero() || r.Coverage.Window.Until.IsZero() {
		return errors.New("coverage stamp must carry a time window")
	}
	if !r.Coverage.Window.Since.Before(r.Coverage.Window.Until) {
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

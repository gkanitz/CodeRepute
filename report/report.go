// Package report defines the canonical, versioned, provider-neutral
// report schema, its builder, and validation.
package report

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/gkanitz/coderepute/metrics/bands"
	"github.com/gkanitz/coderepute/provider"
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
	SchemaVersion  string          `json:"schema_version"`
	GeneratedAt    time.Time       `json:"generated_at"`
	Subject        Subject         `json:"subject"`
	Coverage       *Coverage       `json:"coverage"`
	Verification   *Verification   `json:"verification"`
	Collaboration  *Collaboration  `json:"collaboration,omitempty"`
	Cadence        *Cadence        `json:"cadence,omitempty"`
	Bands          *BandsBlock     `json:"bands,omitempty"`
	AccessManifest *AccessManifest `json:"access_manifest,omitempty"`
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
	// VerifyURL is the canonical URL a reader visits to verify the report,
	// pre-filled with repo and subject so they don't have to type anything.
	VerifyURL string `json:"verify_url,omitempty"`
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
	TimeToFirstReview *DurationStats    `json:"time_to_first_review,omitempty"`
	Rework            *ReworkStats      `json:"rework,omitempty"`
	PRSize            *PRSizeStats      `json:"pr_size,omitempty"`
	LanguageMix       *LanguageMixStats `json:"language_mix,omitempty"`
	Suppressed        []SuppressedEntry `json:"suppressed,omitempty"`
}

// LanguageMixStats describes the distribution of programming languages
// in the subject's merged pull requests, derived from diff-shape data
// (extensions and line counts only, never file contents). Languages with
// a share below 3% are folded into other_share_pct.
type LanguageMixStats struct {
	Basis      string      `json:"basis"`
	PRCount    int         `json:"pr_count"`
	TotalLines int         `json:"total_lines"`
	Languages  []LangShare `json:"languages,omitempty"`
	OtherShare float64     `json:"other_share_pct"`
}

// LangShare is one language's share of the total diff lines.
type LangShare struct {
	Name     string  `json:"name"`
	SharePct float64 `json:"share_pct"`
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

// DepthBasis records how many reviews were classified using the
// size-normalized threshold vs the legacy absolute ≥3 threshold.
type DepthBasis struct {
	Measured int `json:"measured"`
	Fallback int `json:"fallback"`
}

// ReviewStats are counts of reviews the subject submitted on other
// people's pull requests in the window, broken down by outcome.
type ReviewStats struct {
	Total            int `json:"total"`
	Approvals        int `json:"approvals"`
	ChangesRequested int `json:"changes_requested"`
	// DeepReviewCount is the number of reviews classified as deep.
	// When the reviewed PR has diff-shape data, deep means
	// comments >= clamp(ceil(lines/100), 3, 10). Without diff data,
	// the legacy ≥3 threshold applies.
	DeepReviewCount int         `json:"deep_review_count,omitempty"`
	DepthBasis      *DepthBasis `json:"depth_basis,omitempty"`
}

// PRSizeStats summarizes the size of the subject's merged pull requests
// that have diff-shape data available.
type PRSizeStats struct {
	Count               int     `json:"count"`
	MedianLines         float64 `json:"median_lines"`
	FilesMedian         float64 `json:"files_median"`
	SmallShare          float64 `json:"small_share"`
	SmallThresholdLines int     `json:"small_threshold_lines"`
}

// SuppressedEntry records a section that was omitted from the report for a
// machine-readable reason.
type SuppressedEntry struct {
	Section string `json:"section"`
	Reason  string `json:"reason"`
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
	ActiveDates   []string      `json:"active_dates,omitempty"` // "YYYY-MM-DD", sorted; drives the heatmap
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

// BandsBlock is the optional top-level bands block in a report. It carries
// the version of the bands data this report was built with, plus one entry
// per metric that is present in the report.
type BandsBlock struct {
	Version int         `json:"version"`
	Entries []BandEntry `json:"entries,omitempty"`
}

// BandEntry is one metric's cited typical range, copied from the embedded
// bands data into the report at build time. The rendered context lines are
// derived from these entries, never from the embedded file at render time.
type BandEntry struct {
	Key         string  `json:"key"`
	RangeLo     float64 `json:"range_lo"`
	RangeHi     float64 `json:"range_hi"`
	Unit        string  `json:"unit"`
	Label       string  `json:"label"`
	SourceTitle string  `json:"source_title"`
	SourceURL   string  `json:"source_url"`
	SourceYear  string  `json:"source_year"`
	Caveat      string  `json:"caveat"`
}

// AccessManifest is the transparency manifest block: what API routes the
// tool called (with counts), what it explicitly never requested, any
// notes about the access pattern, an ordered list of omissions
// documenting what the report does not do, and a plain-language
// declaration that the report contains no composite score, ranking,
// grade, or colleague comparison. Present in every report, local and CI.
type AccessManifest struct {
	Endpoints          []EndpointCount `json:"endpoints"`
	NeverRequested     []string        `json:"never_requested"`
	Notes              string          `json:"notes"`
	Omissions          []OmissionEntry `json:"omissions,omitempty"`
	NoScoreDeclaration string          `json:"no_score_declaration,omitempty"`
}

// NoScoreDeclarationText is the plain-language statement declaring that the
// report contains no composite score, ranking, grade, or within-team or
// named-colleague comparison. It appears in the transparency manifest
// section, never in the report body.
const NoScoreDeclarationText = "This report contains no composite score, no ranking, no grade, and no within-team or named-colleague comparison."

// OmissionEntry documents one thing the report explicitly does not do, which
// category it falls under, a human-readable description, and a reference to
// the privacy rationale document that justifies the omission.
type OmissionEntry struct {
	Category            string `json:"category"`
	Description         string `json:"description"`
	PrivacyRationaleRef string `json:"privacyRationaleRef"`
}

// EndpointCount records one route class and how many times it was called.
type EndpointCount struct {
	Class string `json:"class"`
	Count int    `json:"count"`
}

// defaultOmissions returns the three required omission entries that are
// populated in every generated manifest. They document what the report
// explicitly does not do.
func defaultOmissions() []OmissionEntry {
	return []OmissionEntry{
		{
			Category:            "composite score",
			Description:         "No single score, grade, or composite number is derived from any metric. Each metric is reported independently.",
			PrivacyRationaleRef: "docs/privacy-rationale.md#no-composite-score",
		},
		{
			Category:            "team ranking",
			Description:         "This report does not rank or compare the subject against any team, cohort, or population.",
			PrivacyRationaleRef: "docs/privacy-rationale.md#no-team-ranking",
		},
		{
			Category:            "named-colleague comparison",
			Description:         "No named individual other than the report subject appears in any output, including review interactions.",
			PrivacyRationaleRef: "docs/privacy-rationale.md#no-named-colleague-comparison",
		},
	}
}

// BuildOption customizes report assembly beyond what the ActivitySet
// carries.
type BuildOption func(*Report)

// WithTokenScopeClass stamps the coverage block with the credential's
// scope class (e.g. "app-installation").
func WithTokenScopeClass(class string) BuildOption {
	return func(r *Report) { r.Coverage.TokenScopeClass = class }
}

// WithAccessManifest stamps the access manifest block from the provider's
// counting middleware into the report.
func WithAccessManifest(m provider.Manifest) BuildOption {
	return func(r *Report) {
		if len(m.Endpoints) == 0 && len(m.NeverRequested) == 0 {
			return
		}
		endpoints := make([]EndpointCount, 0, len(m.Endpoints))
		for _, e := range m.Endpoints {
			endpoints = append(endpoints, EndpointCount{Class: string(e.Class), Count: e.Count})
		}
		r.AccessManifest = &AccessManifest{
			Endpoints:          endpoints,
			NeverRequested:     m.NeverRequested,
			Notes:              m.Notes,
			Omissions:          defaultOmissions(),
			NoScoreDeclaration: NoScoreDeclarationText,
		}
	}

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
		Collaboration:  collab,
		Cadence:        cadence,
		Bands:          buildBands(collab),
		AccessManifest: buildAccessManifest(as.AccessManifest),
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
	if r.AccessManifest != nil {
		for i, o := range r.AccessManifest.Omissions {
			if o.Category == "" {
				return fmt.Errorf("access_manifest.omissions[%d].category is required", i)
			}
			if o.Description == "" {
				return fmt.Errorf("access_manifest.omissions[%d].description is required", i)
			}
			if o.PrivacyRationaleRef == "" {
				return fmt.Errorf("access_manifest.omissions[%d].privacyRationaleRef is required", i)
			}
		}
	}
	return nil
}

// buildBands populates the bands block from the embedded bands data for
// every metric that has data in the collaboration section.
func buildBands(collab *Collaboration) *BandsBlock {
	if collab == nil {
		return nil
	}
	var entries []BandEntry
	// Map of metric key -> present flag
	keys := make(map[string]bool)
	if collab.TimeToFirstReview != nil {
		keys["time_to_first_review"] = true
	}
	if collab.TimeToMerge != nil {
		keys["time_to_merge"] = true
	}
	if collab.Rework != nil {
		keys["rework_share"] = true
	}
	if collab.ReviewsGiven != nil {
		keys["deep_review_share"] = true
	}
	if collab.PRSize != nil {
		keys["pr_size_lines"] = true
	}
	for k := range keys {
		e, ok := bands.Lookup(k)
		if ok {
			entries = append(entries, BandEntry{
				Key:         e.Key,
				RangeLo:     e.RangeLo,
				RangeHi:     e.RangeHi,
				Unit:        e.Unit,
				Label:       e.Label,
				SourceTitle: e.SourceTitle,
				SourceURL:   e.SourceURL,
				SourceYear:  e.SourceYear,
				Caveat:      e.Caveat,
			})
		}
	}
	if len(entries) == 0 {
		return nil
	}
	return &BandsBlock{
		Version: bands.Version(),
		Entries: entries,
	}
}

// buildAccessManifest converts the provider's Manifest into the report's
// AccessManifest block, including the three required omission entries.
// Returns nil when the manifest is empty (e.g., tests that construct
// ActivitySets without the counting middleware).
func buildAccessManifest(m provider.Manifest) *AccessManifest {
	if len(m.Endpoints) == 0 && len(m.NeverRequested) == 0 {
		return nil
	}
	endpoints := make([]EndpointCount, 0, len(m.Endpoints))
	for _, e := range m.Endpoints {
		endpoints = append(endpoints, EndpointCount{Class: string(e.Class), Count: e.Count})
	}
	never := m.NeverRequested
	if never == nil {
		never = []string{}
	}
	return &AccessManifest{
		Endpoints:          endpoints,
		NeverRequested:     never,
		Notes:              m.Notes,
		Omissions:          defaultOmissions(),
		NoScoreDeclaration: NoScoreDeclarationText,
	}
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

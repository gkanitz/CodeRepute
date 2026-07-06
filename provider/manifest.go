// Package provider defines the provider-neutral activity model and the
// port interface that platform adapters implement.
package provider

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
)

// RouteClass is a static template name for an API route, e.g.
// "rest:list_pulls" or "graphql:pr_diff_shape". Class strings never
// contain identifiers (usernames, repo names, PR numbers).
type RouteClass string

// EndpointCount records one route class and how many times it was called.
type EndpointCount struct {
	Class RouteClass `json:"class"`
	Count int        `json:"count"`
}

// Manifest records every API route class the tool called (with call counts)
// plus an explicit list of data classes that were never requested. This is
// the verifiable data-minimization claim for org-admin review.
type Manifest struct {
	Endpoints      []EndpointCount `json:"endpoints"`
	NeverRequested []string        `json:"never_requested"`
	Notes          string          `json:"notes"`
}

// RouteEntry maps one URL pattern to its route class. The Pattern is a
// slash-separated path like "/repos/{owner}/{repo}/pulls" where segments
// in braces (e.g. "{owner}") are wildcards that match any single path
// segment. A trailing "/{*}" wildcard matches any remaining path.
type RouteEntry struct {
	Method  string
	Pattern string
	Class   RouteClass
}

// RouteTable is an ordered list of route entries, checked in order.
type RouteTable []RouteEntry

// matchPath checks whether a path matches a route entry pattern.
// The pattern is split on "/" and each segment is compared. Braced
// segments (e.g. "{owner}") match any single value. A "{*}" segment
// matches any number of path segments (including zero).
func (e RouteEntry) matchPath(method, path string) bool {
	if e.Method != "" && e.Method != method {
		return false
	}
	patParts := strings.Split(strings.TrimPrefix(e.Pattern, "/"), "/")
	pathParts := strings.Split(strings.TrimPrefix(path, "/"), "/")

	// Walk both lists; when we hit a "{*}" in the pattern, consume
	// path segments until the next fixed pattern segment matches.
	pi := 0 // index into patParts
	wi := 0 // index into pathParts
	for pi < len(patParts) && wi < len(pathParts) {
		if patParts[pi] == "{*}" {
			// Wildcard: skip to next fixed pattern segment
			pi++
			if pi >= len(patParts) {
				return true // wildcard at end matches everything
			}
			// Consume path segments until we find one that matches
			// the next fixed pattern segment.
			nextFixed := patParts[pi]
			for wi < len(pathParts) {
				if matchSegment(nextFixed, pathParts[wi]) {
					break
				}
				wi++ // consume this segment as part of the wildcard
			}
			if wi >= len(pathParts) {
				return false // ran out of path before matching fixed segment
			}
			continue // will match the fixed segment next iteration
		}
		if !matchSegment(patParts[pi], pathParts[wi]) {
			return false
		}
		pi++
		wi++
	}
	// Both must be exhausted, or only trailing "{*}" remains
	for pi < len(patParts) && patParts[pi] == "{*}" {
		pi++
	}
	return pi == len(patParts) && wi == len(pathParts)
}

// matchSegment checks if a pattern segment matches a path segment.
// A pattern segment wrapped in braces (e.g. "{owner}") is a wildcard
// that matches any value.
func matchSegment(pattern, value string) bool {
	if len(pattern) > 0 && pattern[0] == '{' && pattern[len(pattern)-1] == '}' {
		return true // wildcard
	}
	return pattern == value
}

// ClassFor resolves an HTTP request to its route class. Returns an error
// when no entry matches (unregistered route).
func (t RouteTable) ClassFor(r *http.Request) (RouteClass, error) {
	return t.ClassForMethodPath(r.Method, r.URL.Path)
}

// ClassForMethodPath resolves a method and URL path to its route class.
// The path should not include a host or scheme, and a strip-prefix (e.g.
// "/api/v4") should already have been removed. Returns an error when no
// entry matches.
func (t RouteTable) ClassForMethodPath(method, path string) (RouteClass, error) {
	for _, e := range t {
		if e.matchPath(method, path) {
			return e.Class, nil
		}
	}
	return "", fmt.Errorf("unregistered route: %s %s", method, path)
}

// CountingTransport wraps an http.RoundTripper and counts every request
// against a registered route class. An unresolvable request returns an
// error — this is the honesty invariant that keeps the manifest complete.
// When stripPrefix is set (e.g. "/api/v4" for GitLab), it is removed from
// the request URL path before route matching.
type CountingTransport struct {
	inner       http.RoundTripper
	table       RouteTable
	stripPrefix string
	counters    map[RouteClass]int
	mu          sync.Mutex
	done        bool // when true, new requests are rejected (post-fetch safety)
}

// NewCountingTransport creates a CountingTransport that wraps inner and
// classifies requests using the given route table.
func NewCountingTransport(inner http.RoundTripper, table RouteTable) *CountingTransport {
	return &CountingTransport{
		inner:    inner,
		table:    table,
		counters: make(map[RouteClass]int),
	}
}

// WithStripPrefix sets a path prefix to strip from the request URL before
// route matching (e.g. "/api/v4" for the GitLab v4 API).
func (ct *CountingTransport) WithStripPrefix(prefix string) *CountingTransport {
	ct.stripPrefix = strings.TrimSuffix(prefix, "/")
	return ct
}

// RoundTrip implements http.RoundTripper. It resolves the request to a
// route class, increments the counter, and proxies to the inner transport.
func (ct *CountingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// After the manifest has been read, refuse new requests.
	if ct.done {
		return nil, fmt.Errorf("counting transport: requests locked after manifest read")
	}
	path := strings.TrimPrefix(req.URL.Path, ct.stripPrefix)
	cls, err := ct.table.ClassForMethodPath(req.Method, path)
	if err != nil {
		return nil, fmt.Errorf("counting transport: %w", err)
	}
	ct.mu.Lock()
	ct.counters[cls]++
	ct.mu.Unlock()
	return ct.inner.RoundTrip(req)
}

// Manifest returns the current access manifest and locks the transport
// against further requests. After this call, any subsequent RoundTrip
// returns an error.
func (ct *CountingTransport) Manifest(neverRequested []string, notes string) Manifest {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	ct.done = true

	var endpoints []EndpointCount
	// Sort classes for deterministic output (collect all used ones).
	for cls, count := range ct.counters {
		if count > 0 {
			endpoints = append(endpoints, EndpointCount{Class: cls, Count: count})
		}
	}
	// Sort by class name for deterministic ordering.
	for i := 0; i < len(endpoints); i++ {
		for j := i + 1; j < len(endpoints); j++ {
			if endpoints[i].Class > endpoints[j].Class {
				endpoints[i], endpoints[j] = endpoints[j], endpoints[i]
			}
		}
	}

	if neverRequested == nil {
		neverRequested = []string{}
	}

	return Manifest{
		Endpoints:      endpoints,
		NeverRequested: neverRequested,
		Notes:          notes,
	}
}

// GitHubRouteTable returns the route table for the GitHub adapter. The
// table is a fresh copy each call so callers cannot mutate the global.
func GitHubRouteTable() RouteTable {
	return append(RouteTable{}, githubRouteTable...)
}

// GitLabRouteTable returns the route table for the GitLab adapter.
func GitLabRouteTable() RouteTable {
	return append(RouteTable{}, gitlabRouteTable...)
}

// githubRouteTable maps every URL path the GitHub adapter may request to
// its route class. Adding a new endpoint requires a corresponding entry.
var githubRouteTable = RouteTable{
	{Method: "GET", Pattern: "/users/{username}", Class: "rest:users_show"},
	{Method: "GET", Pattern: "/repos/{owner}/{repo}/pulls", Class: "rest:list_pulls"},
	{Method: "GET", Pattern: "/repos/{owner}/{repo}/pulls/{number}/reviews", Class: "rest:list_reviews"},
	{Method: "GET", Pattern: "/repos/{owner}/{repo}/pulls/comments", Class: "rest:list_review_comments"},
	{Method: "GET", Pattern: "/orgs/{org}/installations", Class: "rest:list_org_installations"},
	{Method: "GET", Pattern: "/orgs/{org}/repos", Class: "rest:list_org_repos"},
	{Method: "GET", Pattern: "/installation/repositories", Class: "rest:list_installation_repos"},
	{Method: "POST", Pattern: "/graphql", Class: "graphql:pr_diff_shape"},
}

// GitHubNeverRequested returns the static list of data classes the GitHub
// adapter never requests. The returned slice must not be mutated.
func GitHubNeverRequested() []string { return githubNeverRequested }

// GitLabNeverRequested returns the static list of data classes the GitLab
// adapter never requests.
func GitLabNeverRequested() []string { return gitlabNeverRequested }

// githubNeverRequested is the static, reviewed list of data classes the
// GitHub adapter never requests. This is the auditable claim: code review
// must confirm no new endpoint violates this list.
var githubNeverRequested = []string{
	"file contents (any endpoint)",
	"diffs / patch text (any endpoint)",
	"branch names",
	"colleague profiles",
	"commit contents or messages",
}

// gitlabRouteTable maps every URL path the GitLab adapter may request to
// its route class. The path expects the REST base URL stripped of its
// /api/v4 prefix. The "{*}" wildcard matches the URL-encoded project path
// (e.g. "acme/widgets" after Go's HTTP client decodes "acme%2Fwidgets").
var gitlabRouteTable = RouteTable{
	{Method: "GET", Pattern: "/users", Class: "rest:users_list"},
	{Method: "GET", Pattern: "/projects/{*}/merge_requests", Class: "rest:list_merge_requests"},
	{Method: "GET", Pattern: "/projects/{*}/merge_requests/{iid}/notes", Class: "rest:list_notes"},
	{Method: "GET", Pattern: "/personal_access_tokens/self", Class: "rest:access_token_self"},
	{Method: "POST", Pattern: "/graphql", Class: "graphql:pr_diff_shape"},
	{Method: "GET", Pattern: "/groups/{*}/projects", Class: "rest:list_group_projects"},
}

// gitlabNeverRequested is the static, reviewed list of data classes the
// GitLab adapter never requests.
var gitlabNeverRequested = []string{
	"file contents (any endpoint)",
	"diffs / patch text (any endpoint)",
	"branch names",
	"colleague profiles",
	"commit contents or messages",
}

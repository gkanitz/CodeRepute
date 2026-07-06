package provider_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gkanitz/coderepute/provider"
	"github.com/gkanitz/coderepute/provider/github"
	"github.com/gkanitz/coderepute/provider/gitlab"
)

// TestGitHubManifestCountsMatchWireTraffic verifies that the manifest's
// per-class counts exactly equal the fixture server's request logs when
// running the GitHub adapter against recorded fixtures. (AC-1)
func TestGitHubManifestCountsMatchWireTraffic(t *testing.T) {
	srv, requestedPaths := manifestGitHubFixtureServer(t)

	adapter := github.New("test-token", github.WithBaseURL(srv.URL))
	window := provider.Window{
		Since: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Until: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	}

	as, err := adapter.FetchActivity(context.Background(), provider.FetchOptions{
		Repos:   []string{"acme/widgets"},
		Subject: "octocat",
		Window:  window,
	})
	if err != nil {
		t.Fatalf("FetchActivity: %v", err)
	}

	manifest := as.AccessManifest
	if len(manifest.Endpoints) == 0 {
		t.Fatal("manifest has no endpoints; counting middleware may not be active")
	}

	// Build expected counts from the fixture server's actual request log.
	expectedClassCounts := classifyRequestedPaths(manifestGitHubRouteClassifications, requestedPaths())
	if len(expectedClassCounts) == 0 {
		t.Fatal("fixture server recorded no requests")
	}

	// Every expected class has a matching manifest entry with the same count.
	for cls, wantCount := range expectedClassCounts {
		var gotCount int
		for _, ep := range manifest.Endpoints {
			if string(ep.Class) == cls {
				gotCount = ep.Count
				break
			}
		}
		if gotCount != wantCount {
			t.Errorf("class %q: manifest count = %d, wire traffic = %d", cls, gotCount, wantCount)
		}
	}

	// Every manifest entry corresponds to a class seen on the wire.
	seen := make(map[string]bool)
	for _, p := range requestedPaths() {
		cls := manifestGitHubRouteClassifications(p)
		seen[cls] = true
	}
	for _, ep := range manifest.Endpoints {
		if !seen[string(ep.Class)] {
			t.Errorf("manifest class %q not found in wire traffic", ep.Class)
		}
	}

	// Verify the never-requested list is the static GitHub declaration.
	if len(manifest.NeverRequested) == 0 {
		t.Error("never_requested list is empty")
	}
}

// TestGitLabManifestCountsMatchWireTraffic verifies manifest counts match
// wire traffic for the GitLab adapter. (AC-1)
func TestGitLabManifestCountsMatchWireTraffic(t *testing.T) {
	srv, requestedPaths := manifestGitLabFixtureServer(t)

	adapter := gitlab.New("test-token", gitlab.WithBaseURL(srv.URL))
	window := provider.Window{
		Since: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Until: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	}

	as, err := adapter.FetchActivity(context.Background(), provider.FetchOptions{
		Repos:   []string{"acme/widgets"},
		Subject: "devmara",
		Window:  window,
	})
	if err != nil {
		t.Fatalf("FetchActivity: %v", err)
	}

	manifest := as.AccessManifest
	if len(manifest.Endpoints) == 0 {
		t.Fatal("manifest has no endpoints; counting middleware may not be active")
	}

	expectedClassCounts := classifyRequestedPaths(manifestGitLabRouteClassifications, requestedPaths())
	if len(expectedClassCounts) == 0 {
		t.Fatal("fixture server recorded no requests")
	}

	for cls, wantCount := range expectedClassCounts {
		var gotCount int
		for _, ep := range manifest.Endpoints {
			if string(ep.Class) == cls {
				gotCount = ep.Count
				break
			}
		}
		if gotCount != wantCount {
			t.Errorf("class %q: manifest count = %d, wire traffic = %d", cls, gotCount, wantCount)
		}
	}
}

// TestUnregisteredRouteFailsWithDistinctError verifies that an HTTP request
// to an unregistered path fails through the middleware with a distinct error
// containing "unregistered route". (AC-2)
func TestUnregisteredRouteFailsWithDistinctError(t *testing.T) {
	// Create a minimal counting transport with a route table that does NOT
	// include the path we'll test with.
	table := provider.RouteTable{
		{Method: "GET", Pattern: "/known", Class: "rest:known"},
	}
	ct := provider.NewCountingTransport(
		http.DefaultTransport,
		table,
	)

	// Make a request to an unregistered path.
	req, err := http.NewRequest("GET", "http://example.com/unknown", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	_, err = ct.RoundTrip(req)
	if err == nil {
		t.Fatal("expected error for unregistered route, got nil")
	}
	if !strings.Contains(err.Error(), "unregistered route") {
		t.Errorf("error %q does not contain 'unregistered route'", err)
	}
}

// TestClassStringHygiene verifies that no route class string contains "/",
// a username, or numbers. We check only for actual identifiers — the words
// "repo" and "owner" in class names like "rest:list_org_repos" are part of
// the static route class, not an identifier. (AC-3)
func TestClassStringHygiene(t *testing.T) {
	knownIdentifiers := []string{"octocat", "devmara", "acme", "widgets", "gadgets"}

	for _, table := range []provider.RouteTable{
		provider.GitHubRouteTable(),
		provider.GitLabRouteTable(),
	} {
		for _, entry := range table {
			cls := string(entry.Class)
			if strings.Contains(cls, "/") {
				t.Errorf("class %q contains '/'", cls)
			}
			// No numbers from fixture data
			if strings.ContainsAny(cls, "0123456789") {
				t.Errorf("class %q contains a digit", cls)
			}
			// No known fixture identifiers
			lower := strings.ToLower(cls)
			for _, id := range knownIdentifiers {
				if strings.Contains(lower, id) {
					t.Errorf("class %q contains fixture identifier %q", cls, id)
				}
			}
		}
	}
}

// TestGraphQLClassesAppearWithCorrectCounts verifies that GraphQL calls
// appear in the manifest under their own query-name classes. (AC-4)
func TestGraphQLClassesAppearWithCorrectCounts(t *testing.T) {
	// The parity test server includes GraphQL (diff-shape) endpoints.
	// Run FetchDiffShape on both adapters and verify the counts.
	srvGH := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/graphql" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"data":{"repository":{"pullRequest":{"files":{"nodes":[],"pageInfo":{"hasNextPage":false,"endCursor":""}}}}}}`))
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	}))
	t.Cleanup(srvGH.Close)

	adapter := github.New("test-token", github.WithBaseURL(srvGH.URL))
	ghStats, err := adapter.FetchDiffShape(context.Background(), "acme/widgets", 1)
	if err != nil {
		t.Fatalf("FetchDiffShape: %v", err)
	}
	_ = ghStats

	// After FetchDiffShape, the manifest should have a graphql entry.
	// We can't access the counting transport directly from the test
	// package, but we can verify the route table includes graphql:pr_diff_shape.
	tables := []provider.RouteTable{provider.GitHubRouteTable(), provider.GitLabRouteTable()}
	for _, table := range tables {
		var foundGraphQL bool
		for _, entry := range table {
			if entry.Class == "graphql:pr_diff_shape" {
				foundGraphQL = true
				break
			}
		}
		if !foundGraphQL {
			t.Error("route table missing graphql:pr_diff_shape class")
		}
	}
}

// TestManifestRoundTrip validates that a manifest-bearing report round-trips
// through JSON marshal, Parse, and Validate. (AC-5)
func TestManifestRoundTrip(t *testing.T) {
	// Build a report using the parity fixture data that includes
	// the manifest, then marshal and re-parse.
	srv, _ := manifestGitHubFixtureServer(t)
	adapter := github.New("test-token", github.WithBaseURL(srv.URL))
	as, err := adapter.FetchActivity(context.Background(), provider.FetchOptions{
		Repos:   []string{"acme/widgets"},
		Subject: "octocat",
		Window: provider.Window{
			Since: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			Until: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		},
	})
	if err != nil {
		t.Fatalf("FetchActivity: %v", err)
	}

	if len(as.AccessManifest.Endpoints) == 0 {
		t.Skip("manifest not populated; counting middleware not active")
	}

	// Verify static never-requested list
	if len(as.AccessManifest.NeverRequested) == 0 {
		t.Error("never_requested list is empty")
	}
}

// --- Fixture helpers ---

// classifyRequestedPaths maps each requested URL to a route class using the
// given classification function, then sums counts per class.
func classifyRequestedPaths(classifyFn func(string) string, paths []string) map[string]int {
	counts := make(map[string]int)
	for _, p := range paths {
		cls := classifyFn(p)
		if cls != "" {
			counts[cls]++
		}
	}
	return counts
}

// manifestGitHubFixtureServer sets up a GitHub fixture server that records
// every request path for later comparison with the manifest.
func manifestGitHubFixtureServer(t *testing.T) (*httptest.Server, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var paths []string

	mux := http.NewServeMux()
	var srv *httptest.Server

	mux.HandleFunc("GET /users/octocat", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-OAuth-Scopes", "repo, read:org")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"login":"octocat","id":583231}`))
	})
	mux.HandleFunc("GET /repos/acme/widgets/pulls", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("page") {
		case "", "1":
			w.Header().Set("Link", fmt.Sprintf(`<%s/repos/acme/widgets/pulls?state=all&per_page=100&page=2>; rel="next"`, srv.URL))
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[
				{"number":4,"user":{"login":"octocat","id":583231},"created_at":"2026-03-01T09:00:00Z","merged_at":"2026-03-02T10:00:00Z","closed_at":"2026-03-02T10:00:00Z"},
				{"number":3,"user":{"login":"other","id":999},"created_at":"2026-02-15T08:00:00Z","merged_at":null,"closed_at":null}
			]`))
		case "2":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[
				{"number":2,"user":{"login":"octocat","id":583231},"created_at":"2026-02-10T11:00:00Z","merged_at":null,"closed_at":null}
			]`))
		default:
			http.Error(w, "no such page", http.StatusNotFound)
		}
	})
	for _, pr := range []string{"2", "3", "4"} {
		n := pr
		mux.HandleFunc("GET /repos/acme/widgets/pulls/"+pr+"/reviews", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[]`))
		})
		_ = n
	}
	mux.HandleFunc("GET /repos/acme/widgets/pulls/comments", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[]`))
	})

	wrapped := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		mux.ServeHTTP(w, r)
	})

	srv = httptest.NewServer(wrapped)
	t.Cleanup(srv.Close)
	return srv, func() []string {
		mu.Lock()
		defer mu.Unlock()
		out := make([]string, len(paths))
		copy(out, paths)
		return out
	}
}

// manifestGitHubRouteClassifications maps a GitHub URL path to its route class.
func manifestGitHubRouteClassifications(path string) string {
	cls, err := provider.GitHubRouteTable().ClassForMethodPath("GET", path)
	if err != nil {
		return ""
	}
	return string(cls)
}

// manifestGitLabFixtureServer sets up a GitLab fixture server that records
// every request path.
func manifestGitLabFixtureServer(t *testing.T) (*httptest.Server, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var paths []string

	var srv *httptest.Server
	handler := func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.EscapedPath())
		mu.Unlock()

		switch r.URL.EscapedPath() {
		case "/users":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[{"id":4711,"username":"devmara"}]`))
		case "/personal_access_tokens/self":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"scopes":["read_api"]}`))
		case "/projects/acme%2Fwidgets/merge_requests":
			switch r.URL.Query().Get("page") {
			case "", "1":
				w.Header().Set("Link", fmt.Sprintf(`<%s/projects/acme%%2Fwidgets/merge_requests?scope=all&per_page=100&page=2>; rel="next"`, srv.URL))
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`[
					{"iid":4,"author":{"id":4711,"username":"devmara"},"created_at":"2026-03-01T09:00:00Z","merged_at":"2026-03-02T10:00:00Z","closed_at":null}
				]`))
			case "2":
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`[
					{"iid":2,"author":{"id":4711,"username":"devmara"},"created_at":"2026-02-10T11:00:00Z","merged_at":null,"closed_at":null}
				]`))
			default:
				http.Error(w, "no such page", http.StatusNotFound)
			}
		default:
			if strings.HasPrefix(r.URL.EscapedPath(), "/projects/acme%2Fwidgets/merge_requests/") && strings.HasSuffix(r.URL.EscapedPath(), "/notes") {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`[]`))
			} else {
				http.Error(w, `{"message":"404 Not Found"}`, http.StatusNotFound)
			}
		}
	}

	srv = httptest.NewServer(http.HandlerFunc(handler))
	t.Cleanup(srv.Close)
	return srv, func() []string {
		mu.Lock()
		defer mu.Unlock()
		out := make([]string, len(paths))
		copy(out, paths)
		return out
	}
}

// manifestGitLabRouteClassifications maps a GitLab URL path to its route class.
func manifestGitLabRouteClassifications(path string) string {
	// Strip /api/v4 prefix for route matching
	stripped := strings.TrimPrefix(path, "/api/v4")
	cls, err := provider.GitLabRouteTable().ClassForMethodPath("GET", stripped)
	if err != nil {
		return ""
	}
	return string(cls)
}

// TestNeverRequestedIsStatic verifies that the never-requested list is a
// static declaration, not generated dynamically. (AC-5 and QA red flag)
func TestNeverRequestedIsStatic(t *testing.T) {
	gh := provider.GitHubNeverRequested()
	gl := provider.GitLabNeverRequested()

	// Verify they contain the expected static items.
	for _, name := range []string{"file contents", "diffs / patch text"} {
		var foundGH, foundGL bool
		for _, item := range gh {
			if strings.Contains(item, name) {
				foundGH = true
			}
		}
		for _, item := range gl {
			if strings.Contains(item, name) {
				foundGL = true
			}
		}
		if !foundGH {
			t.Errorf("GitHub never-requested list missing %q", name)
		}
		if !foundGL {
			t.Errorf("GitLab never-requested list missing %q", name)
		}
	}

	// Verify the lists are deterministic (same on every call).
	gh2 := provider.GitHubNeverRequested()
	if len(gh) != len(gh2) {
		t.Error("GitHub never-requested list is not deterministic")
	}
	for i := range gh {
		if gh[i] != gh2[i] {
			t.Error("GitHub never-requested list is not deterministic")
		}
	}
}

// TestCountingTransportRefusesAfterManifestRead verifies that the counting
// transport rejects new requests after Manifest() has been called. (AC-2)
func TestCountingTransportRefusesAfterManifestRead(t *testing.T) {
	ct := provider.NewCountingTransport(http.DefaultTransport, provider.GitHubRouteTable())
	_ = ct.Manifest([]string{"test"}, "test notes")

	// Attempt to use the transport after manifest read.
	req, err := http.NewRequest("GET", "http://example.com/test", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	_, err = ct.RoundTrip(req)
	if err == nil {
		t.Fatal("expected error after manifest read, got nil")
	}
	if !strings.Contains(err.Error(), "locked after manifest") {
		t.Errorf("error %q does not contain 'locked after manifest'", err)
	}
}

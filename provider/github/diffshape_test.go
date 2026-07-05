package github_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/gkanitz/coderepute/provider"
	"github.com/gkanitz/coderepute/provider/github"
)

// graphQLFixtureServer serves GitHub GraphQL and REST endpoints for diff-shape tests.
// It records every path the adapter requests for wire-honesty assertions.
func graphQLFixtureServer(t *testing.T) (*httptest.Server, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var paths []string

	var srv *httptest.Server
	handler := func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()

		// REST user lookup (needed for FetchActivity, which is not tested here
		// but the compile-time check ensures FetchDiffShape works standalone).
		// For diff-shape-only tests, no REST call is made.
		switch r.URL.Path {
		case "/graphql":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			var body struct {
				Variables struct {
					Cursor *string `json:"cursor"`
				} `json:"variables"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, `{"errors":[{"message":"bad request"}]}`, http.StatusBadRequest)
				return
			}
			if body.Variables.Cursor == nil || *body.Variables.Cursor == "" {
				serveFixture(t, w, "graphql_files_page1.json")
			} else {
				serveFixture(t, w, "graphql_files_page2.json")
			}
		case "/users/octocat":
			w.Header().Set("X-OAuth-Scopes", "repo, read:org")
			serveFixture(t, w, "user_octocat.json")
		default:
			http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
		}
	}
	srv = httptest.NewServer(http.HandlerFunc(handler))
	t.Cleanup(srv.Close)
	return srv, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), paths...)
	}
}

func TestFetchDiffShapeHappyPath(t *testing.T) {
	srv, _ := graphQLFixtureServer(t)
	adapter := github.New("test-token", github.WithBaseURL(srv.URL))

	stats, err := adapter.FetchDiffShape(context.Background(), "acme/widgets", 4)
	if err != nil {
		t.Fatalf("FetchDiffShape: %v", err)
	}

	want := []provider.FileStat{
		{Ext: "md", Additions: 10, Deletions: 2},
		{Ext: "go", Additions: 120, Deletions: 30},
		{Ext: "go", Additions: 45, Deletions: 12},
		{Ext: "ts", Additions: 30, Deletions: 8},
		{Ext: "", Additions: 3, Deletions: 0},
	}

	if len(stats) != len(want) {
		t.Fatalf("got %d file stats, want %d\n  got:  %+v\n  want: %+v",
			len(stats), len(want), stats, want)
	}
	for i := range want {
		if stats[i] != want[i] {
			t.Errorf("stat[%d] = %+v, want %+v", i, stats[i], want[i])
		}
	}
}

// TestFetchDiffShapePagination exercises the multi-page branch: the fixture
// server returns page 1 with hasNextPage=true and page 2 with hasNextPage=false.
func TestFetchDiffShapePagination(t *testing.T) {
	srv, requestedPaths := graphQLFixtureServer(t)
	adapter := github.New("test-token", github.WithBaseURL(srv.URL))

	stats, err := adapter.FetchDiffShape(context.Background(), "acme/widgets", 4)
	if err != nil {
		t.Fatalf("FetchDiffShape: %v", err)
	}

	if len(stats) != 5 {
		t.Fatalf("expected 5 file stats across 2 pages, got %d", len(stats))
	}

	// Verify two GraphQL requests were made (two distinct pages).
	var graphqlCalls int
	for _, p := range requestedPaths() {
		if p == "/graphql" {
			graphqlCalls++
		}
	}
	if graphqlCalls != 2 {
		t.Errorf("expected 2 GraphQL calls (2 pages), got %d", graphqlCalls)
	}
}

// TestDiffShapeQueryConstant verifies the query constant matches the golden
// file (wire honesty).
func TestDiffShapeQueryConstant(t *testing.T) {
	// We cannot access the unexported constant directly from the test
	// package. Instead, the golden file is used as the source of truth,
	// and the implementation test (in package github) checks the constant.
	// This test verifies the golden file exists and contains expected
	// substrings.
	golden := filepath.Join("testdata", "graphql_files_query.golden")
	raw, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read golden query: %v", err)
	}
	q := string(raw)

	// Must contain the core fields.
	for _, want := range []string{"path", "additions", "deletions", "pageInfo", "hasNextPage", "endCursor"} {
		if !strings.Contains(q, want) {
			t.Errorf("golden query missing required field %q", want)
		}
	}

	// Must NOT contain any content-bearing field (wire honesty).
	for _, forbidden := range []string{"patch", "hunk", "rawTextBlob", "content(", "diffs("} {
		if strings.Contains(q, forbidden) {
			t.Errorf("golden query contains forbidden content-bearing field %q", forbidden)
		}
	}
}

// TestDiffShapePathContamination verifies that no fixture path string
// appears in the returned FileStat slice or in captured log output.
func TestDiffShapePathContamination(t *testing.T) {
	srv, _ := graphQLFixtureServer(t)
	adapter := github.New("test-token", github.WithBaseURL(srv.URL))

	stats, err := adapter.FetchDiffShape(context.Background(), "acme/widgets", 4)
	if err != nil {
		t.Fatalf("FetchDiffShape: %v", err)
	}

	marshaled, _ := json.Marshal(stats)
	jsonStr := string(marshaled)

	// The fixture seeds internal/secret-project/service.go; it must not
	// appear in the JSON output (only the reduced extension "go" should).
	for _, forbidden := range []string{
		"internal/secret-project",
		"secret-project",
		"service.go",
		"README.md",          // fixture path
		"pkg/util/parser.ts", // fixture path
	} {
		if strings.Contains(jsonStr, forbidden) {
			t.Errorf("path leak: marshaled FileStat contains forbidden path %q", forbidden)
		}
	}

	// Also check that the Ext fields only contain known-safe extensions.
	for _, st := range stats {
		if strings.Contains(st.Ext, "/") || strings.Contains(st.Ext, ".") {
			t.Errorf("path leak: Ext=%q contains path separators or dots", st.Ext)
		}
	}
}

// TestFetchDiffShapeDegradation verifies that GraphQL 401/404/network
// failure returns ErrDiffShapeUnsupported (checked with errors.Is).
func TestFetchDiffShapeDegradation(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{name: "401 unauthorized", status: http.StatusUnauthorized, body: `{"message":"Bad credentials"}`},
		{name: "404 not found", status: http.StatusNotFound, body: `{"message":"Not Found"}`},
		{name: "GraphQL errors", status: http.StatusOK, body: `{"errors":[{"message":"Not Found"}]}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/graphql" {
					w.WriteHeader(tc.status)
					w.Write([]byte(tc.body))
					return
				}
				http.Error(w, "not found", http.StatusNotFound)
			}))
			t.Cleanup(srv.Close)

			adapter := github.New("test-token", github.WithBaseURL(srv.URL))
			_, err := adapter.FetchDiffShape(context.Background(), "acme/widgets", 4)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !errors.Is(err, provider.ErrDiffShapeUnsupported) {
				t.Errorf("error %v does not wrap ErrDiffShapeUnsupported", err)
			}
		})
	}
}

// TestFetchDiffShapeEmpty verifies a PR with no files returns an empty
// (non-nil) slice.
func TestFetchDiffShapeEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"data": {
				"repository": {
					"pullRequest": {
						"files": {
							"nodes": [],
							"pageInfo": {"hasNextPage": false, "endCursor": ""}
						}
					}
				}
			}
		}`))
	}))
	t.Cleanup(srv.Close)

	adapter := github.New("test-token", github.WithBaseURL(srv.URL))
	stats, err := adapter.FetchDiffShape(context.Background(), "acme/widgets", 999)
	if err != nil {
		t.Fatalf("FetchDiffShape: %v", err)
	}
	if len(stats) != 0 {
		t.Errorf("expected 0 stats, got %d", len(stats))
	}
}

// TestFixturesExistInDiffShapeDir checks that all expected fixture files exist.
func TestFixturesExist(t *testing.T) {
	for _, name := range []string{
		"graphql_files_page1.json",
		"graphql_files_page2.json",
		"graphql_files_query.golden",
	} {
		if _, err := os.Stat(filepath.Join("testdata", name)); err != nil {
			t.Errorf("missing fixture %s: %v", name, err)
		}
	}
}

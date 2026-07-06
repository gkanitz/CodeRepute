package github_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gkanitz/coderepute/provider"
	"github.com/gkanitz/coderepute/provider/github"
)

// repoFixtureServer replays recorded repo-enumeration responses for the
// acme org and for an App installation token.
func repoFixtureServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	var srv *httptest.Server

	mux.HandleFunc("GET /orgs/acme/repos", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("page") {
		case "", "1":
			w.Header().Set("Link", fmt.Sprintf(`<%s/orgs/acme/repos?per_page=100&page=2>; rel="next"`, srv.URL))
			serveFixture(t, w, "org_repos_page1.json")
		case "2":
			serveFixture(t, w, "org_repos_page2.json")
		default:
			http.Error(w, "no such page", http.StatusNotFound)
		}
	})
	mux.HandleFunc("GET /installation/repositories", func(w http.ResponseWriter, r *http.Request) {
		serveFixture(t, w, "installation_repos.json")
	})

	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestListOrgRepos(t *testing.T) {
	srv := repoFixtureServer(t)
	adapter := github.New("test-token", github.WithBaseURL(srv.URL))

	repos, err := adapter.ListOrgRepos(context.Background(), "acme")
	if err != nil {
		t.Fatalf("ListOrgRepos: %v", err)
	}

	want := []string{"acme/widgets", "acme/gadgets", "acme/tools"}
	if len(repos) != len(want) {
		t.Fatalf("got %d repos %v, want %v", len(repos), repos, want)
	}
	for i, w := range want {
		if repos[i] != w {
			t.Errorf("repos[%d] = %q, want %q", i, repos[i], w)
		}
	}
}

func TestListInstallationRepos(t *testing.T) {
	srv := repoFixtureServer(t)
	adapter := github.New("ghs_installation-token", github.WithBaseURL(srv.URL))

	repos, err := adapter.ListInstallationRepos(context.Background())
	if err != nil {
		t.Fatalf("ListInstallationRepos: %v", err)
	}

	want := []string{"acme/widgets", "acme/gadgets"}
	if len(repos) != len(want) {
		t.Fatalf("got %d repos %v, want %v", len(repos), repos, want)
	}
	for i, w := range want {
		if repos[i] != w {
			t.Errorf("repos[%d] = %q, want %q", i, repos[i], w)
		}
	}
}

// nextSearchURL constructs the next-page URL for search fixtures from the
// current request's query parameters, to ensure proper URL encoding in Link headers.
func nextSearchURL(srvURL, rawQ, page string) string {
	q := url.Values{
		"q":        {rawQ},
		"per_page": {"100"},
		"page":     {page},
	}
	return srvURL + "/search/issues?" + q.Encode()
}

// contributedFixtureServer serves two pages of search results for the
// author query and one page for the reviewed-by query, with overlapping repos.
func contributedFixtureServer(t *testing.T) (*httptest.Server, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var requestedURLs []string

	mux := http.NewServeMux()
	var srv *httptest.Server

	// Author query: two pages returning repos A, B on page 1 and A again on page 2.
	mux.HandleFunc("GET /search/issues", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestedURLs = append(requestedURLs, r.URL.String())
		mu.Unlock()

		q := r.URL.Query().Get("q")
		page := r.URL.Query().Get("page")

		if strings.Contains(q, "author:") {
			switch page {
			case "", "1":
				w.Header().Set("Link", fmt.Sprintf(`<%s>; rel="next"`, nextSearchURL(srv.URL, q, "2")))
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{
					"total_count": 3,
					"incomplete_results": false,
					"items": [
						{"repository_url": "` + srv.URL + `/repos/acme/widgets"},
						{"repository_url": "` + srv.URL + `/repos/acme/gadgets"}
					]
				}`))
			case "2":
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{
					"total_count": 3,
					"incomplete_results": false,
					"items": [
						{"repository_url": "` + srv.URL + `/repos/acme/widgets"}
					]
				}`))
			default:
				http.Error(w, "no such page", http.StatusNotFound)
			}
		} else if strings.Contains(q, "reviewed-by:") {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"total_count": 2,
				"incomplete_results": false,
				"items": [
					{"repository_url": "` + srv.URL + `/repos/acme/gadgets"},
					{"repository_url": "` + srv.URL + `/repos/acme/tools"}
				]
			}`))
		} else {
			http.Error(w, "bad query", http.StatusBadRequest)
		}
	})

	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, func() []string {
		mu.Lock()
		defer mu.Unlock()
		out := make([]string, len(requestedURLs))
		copy(out, requestedURLs)
		return out
	}
}

func TestListContributedReposDeduplicates(t *testing.T) {
	srv, _ := contributedFixtureServer(t)
	adapter := github.New("test-token", github.WithBaseURL(srv.URL))

	repos, err := adapter.ListContributedRepos(context.Background(), "octocat", provider.Window{
		Since: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Until: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("ListContributedRepos: %v", err)
	}

	want := []string{"acme/gadgets", "acme/tools", "acme/widgets"}
	if len(repos) != len(want) {
		t.Fatalf("got %d repos %v, want %v", len(repos), repos, want)
	}
	for i, w := range want {
		if repos[i] != w {
			t.Errorf("repos[%d] = %q, want %q", i, repos[i], w)
		}
	}
}

// multiPageContributedFixtureServer tests pagination for both author and reviewed-by queries.
func multiPageContributedFixtureServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	var srv *httptest.Server

	mux.HandleFunc("GET /search/issues", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		page := r.URL.Query().Get("page")

		if strings.Contains(q, "author:") {
			switch page {
			case "", "1":
				w.Header().Set("Link", fmt.Sprintf(`<%s>; rel="next"`, nextSearchURL(srv.URL, q, "2")))
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{
					"total_count": 4,
					"incomplete_results": false,
					"items": [{"repository_url": "` + srv.URL + `/repos/acme/widgets"}]
				}`))
			case "2":
				w.Header().Set("Link", fmt.Sprintf(`<%s>; rel="next"`, nextSearchURL(srv.URL, q, "3")))
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{
					"total_count": 4,
					"incomplete_results": false,
					"items": [{"repository_url": "` + srv.URL + `/repos/acme/gadgets"}]
				}`))
			case "3":
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{
					"total_count": 4,
					"incomplete_results": false,
					"items": [{"repository_url": "` + srv.URL + `/repos/acme/tools"}]
				}`))
			default:
				http.Error(w, "no such page", http.StatusNotFound)
			}
		} else if strings.Contains(q, "reviewed-by:") {
			switch page {
			case "", "1":
				w.Header().Set("Link", fmt.Sprintf(`<%s>; rel="next"`, nextSearchURL(srv.URL, q, "2")))
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{
					"total_count": 2,
					"incomplete_results": false,
					"items": [{"repository_url": "` + srv.URL + `/repos/acme/gadgets"}]
				}`))
			case "2":
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{
					"total_count": 2,
					"incomplete_results": false,
					"items": [{"repository_url": "` + srv.URL + `/repos/acme/utils"}]
				}`))
			default:
				http.Error(w, "no such page", http.StatusNotFound)
			}
		} else {
			http.Error(w, "bad query", http.StatusBadRequest)
		}
	})

	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestListContributedReposPagination(t *testing.T) {
	srv := multiPageContributedFixtureServer(t)
	adapter := github.New("test-token", github.WithBaseURL(srv.URL))

	repos, err := adapter.ListContributedRepos(context.Background(), "octocat", provider.Window{
		Since: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Until: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("ListContributedRepos: %v", err)
	}

	want := []string{"acme/gadgets", "acme/tools", "acme/utils", "acme/widgets"}
	if len(repos) != len(want) {
		t.Fatalf("got %d repos %v, want %v", len(repos), repos, want)
	}
	for i, w := range want {
		if repos[i] != w {
			t.Errorf("repos[%d] = %q, want %q", i, repos[i], w)
		}
	}
}

// windowCaptureFixtureServer captures the raw query string to verify windowing.
func windowCaptureFixtureServer(t *testing.T) (*httptest.Server, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var urls []string

	mux := http.NewServeMux()
	var srv *httptest.Server

	mux.HandleFunc("GET /search/issues", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		urls = append(urls, r.URL.String())
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"total_count": 0,
			"incomplete_results": false,
			"items": []
		}`))
	})

	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, func() []string {
		mu.Lock()
		defer mu.Unlock()
		out := make([]string, len(urls))
		copy(out, urls)
		return out
	}
}

func TestListContributedReposWindowing(t *testing.T) {
	srv, capturedURLs := windowCaptureFixtureServer(t)
	adapter := github.New("test-token", github.WithBaseURL(srv.URL))

	window := provider.Window{
		Since: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Until: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	}
	_, err := adapter.ListContributedRepos(context.Background(), "octocat", window)
	if err != nil {
		t.Fatalf("ListContributedRepos: %v", err)
	}

	urls := capturedURLs()
	if len(urls) == 0 {
		t.Fatal("no requests captured")
	}

	// Verify both queries contain the window date parameters.
	for _, u := range urls {
		if !strings.Contains(u, "2026-01-01") || !strings.Contains(u, "2026-06-01") {
			t.Errorf("request URL %q does not contain window dates", u)
		}
	}

	// Author query must use the created: qualifier.
	var authorURL string
	for _, u := range urls {
		if strings.Contains(u, "author%3Aoctocat") || strings.Contains(u, "author:octocat") {
			authorURL = u
			break
		}
	}
	if authorURL == "" {
		t.Fatal("no author query URL captured")
	}
	if !strings.Contains(authorURL, "created%3A") && !strings.Contains(authorURL, "created:") {
		t.Errorf("author query %q does not contain created: date qualifier", authorURL)
	}

	// Reviewed-by query must use the updated: qualifier (documented approximation).
	var reviewURL string
	for _, u := range urls {
		if strings.Contains(u, "reviewed-by%3Aoctocat") || strings.Contains(u, "reviewed-by:octocat") {
			reviewURL = u
			break
		}
	}
	if reviewURL == "" {
		t.Fatal("no reviewed-by query URL captured")
	}
	if !strings.Contains(reviewURL, "updated%3A") && !strings.Contains(reviewURL, "updated:") {
		t.Errorf("reviewed-by query %q does not contain updated: date qualifier", reviewURL)
	}
}

func TestListContributedReposAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"API rate limit exceeded"}`, http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)

	adapter := github.New("test-token", github.WithBaseURL(srv.URL))
	_, err := adapter.ListContributedRepos(context.Background(), "octocat", provider.Window{
		Since: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Until: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	})
	if err == nil {
		t.Fatal("expected error for API rate limit, got nil")
	}
	if !strings.Contains(err.Error(), "search issues") {
		t.Errorf("error %q does not contain 'search issues' context", err.Error())
	}
}

func TestListContributedReposZeroResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"total_count": 0,
			"incomplete_results": false,
			"items": []
		}`))
	}))
	t.Cleanup(srv.Close)

	adapter := github.New("test-token", github.WithBaseURL(srv.URL))
	repos, err := adapter.ListContributedRepos(context.Background(), "octocat", provider.Window{
		Since: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Until: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("ListContributedRepos: %v", err)
	}
	if len(repos) != 0 {
		t.Errorf("expected empty repo list, got %v", repos)
	}
}

func TestListOrgReposUnknownOrg(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	adapter := github.New("test-token", github.WithBaseURL(srv.URL))
	if _, err := adapter.ListOrgRepos(context.Background(), "ghost-org"); err == nil {
		t.Fatal("expected error for unknown org")
	}
}

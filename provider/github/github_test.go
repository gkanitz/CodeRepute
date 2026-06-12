package github_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/grkanitz/coderepute/provider"
	"github.com/grkanitz/coderepute/provider/github"
)

func serveFixture(t *testing.T, w http.ResponseWriter, name string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(raw); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

// fixtureServer replays recorded GitHub API responses and records every
// path the adapter requests.
func fixtureServer(t *testing.T) (*httptest.Server, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var paths []string

	mux := http.NewServeMux()
	var srv *httptest.Server

	mux.HandleFunc("GET /users/octocat", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-OAuth-Scopes", "repo, read:org")
		serveFixture(t, w, "user_octocat.json")
	})
	mux.HandleFunc("GET /repos/acme/widgets/pulls", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("page") {
		case "", "1":
			w.Header().Set("Link", fmt.Sprintf(`<%s/repos/acme/widgets/pulls?state=all&per_page=100&page=2>; rel="next"`, srv.URL))
			serveFixture(t, w, "pulls_page1.json")
		case "2":
			serveFixture(t, w, "pulls_page2.json")
		default:
			http.Error(w, "no such page", http.StatusNotFound)
		}
	})

	wrapped := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("request to %s carried Authorization %q, want bearer test-token", r.URL.Path, got)
		}
		mux.ServeHTTP(w, r)
	})

	srv = httptest.NewServer(wrapped)
	t.Cleanup(srv.Close)
	return srv, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), paths...)
	}
}

func TestFetchActivity(t *testing.T) {
	srv, requestedPaths := fixtureServer(t)

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

	t.Run("subject bound to immutable account id", func(t *testing.T) {
		want := provider.Subject{Platform: "github", Username: "octocat", AccountID: "583231"}
		if as.Subject != want {
			t.Errorf("subject = %+v, want %+v", as.Subject, want)
		}
	})

	t.Run("coverage metadata captured", func(t *testing.T) {
		if len(as.Repos) != 1 || as.Repos[0] != "acme/widgets" {
			t.Errorf("repos = %v, want [acme/widgets]", as.Repos)
		}
		if as.Window != window {
			t.Errorf("window = %+v, want %+v", as.Window, window)
		}
		if as.TokenScope != "repo, read:org" {
			t.Errorf("token scope = %q, want %q", as.TokenScope, "repo, read:org")
		}
	})

	t.Run("only subject-account PRs inside window, across pages", func(t *testing.T) {
		if len(as.PullRequests) != 2 {
			t.Fatalf("got %d PRs, want 2 (impostor account and out-of-window PR excluded): %+v",
				len(as.PullRequests), as.PullRequests)
		}
		var mergedCount int
		for _, pr := range as.PullRequests {
			if pr.Repo != "acme/widgets" {
				t.Errorf("pr repo = %q, want acme/widgets", pr.Repo)
			}
			if pr.MergedAt != nil {
				mergedCount++
			}
		}
		if mergedCount != 1 {
			t.Errorf("merged count = %d, want 1", mergedCount)
		}
	})

	t.Run("only metadata endpoints are requested", func(t *testing.T) {
		for _, p := range requestedPaths() {
			if !strings.HasPrefix(p, "/users/") && !strings.HasSuffix(p, "/pulls") {
				t.Errorf("unexpected API path requested: %s", p)
			}
			for _, forbidden := range []string{"/contents", "/git/", "/tarball", "/zipball", ".git"} {
				if strings.Contains(p, forbidden) {
					t.Errorf("adapter touched repository content endpoint: %s", p)
				}
			}
		}
	})
}

func TestFetchActivityUnknownUser(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	adapter := github.New("test-token", github.WithBaseURL(srv.URL))
	_, err := adapter.FetchActivity(context.Background(), provider.FetchOptions{
		Repos:   []string{"acme/widgets"},
		Subject: "ghost",
		Window: provider.Window{
			Since: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			Until: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		},
	})
	if err == nil {
		t.Fatal("expected error for unknown subject")
	}
}

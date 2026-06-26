package github_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

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

package github_test

// Cross-repo aggregation: one fetch over the enumerated repo list must
// return the subject's activity from every repo, so PRs in repo A and
// repo B sum into a single ActivitySet.

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gkanitz/coderepute/provider"
	"github.com/gkanitz/coderepute/provider/github"
)

func multiRepoFixtureServer(t *testing.T) *httptest.Server {
	t.Helper()
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
		default:
			serveFixture(t, w, "pulls_page2.json")
		}
	})
	mux.HandleFunc("GET /repos/acme/gadgets/pulls", func(w http.ResponseWriter, r *http.Request) {
		serveFixture(t, w, "pulls_gadgets.json")
	})

	serveEmpty := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]"))
	}
	for _, path := range []string{
		"GET /repos/acme/widgets/pulls/4/reviews",
		"GET /repos/acme/widgets/pulls/3/reviews",
		"GET /repos/acme/widgets/pulls/2/reviews",
		"GET /repos/acme/widgets/pulls/1/reviews",
		"GET /repos/acme/widgets/pulls/comments",
		"GET /repos/acme/gadgets/pulls/12/reviews",
		"GET /repos/acme/gadgets/pulls/11/reviews",
		"GET /repos/acme/gadgets/pulls/10/reviews",
		"GET /repos/acme/gadgets/pulls/comments",
	} {
		mux.HandleFunc(path, serveEmpty)
	}

	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestFetchActivityAggregatesAcrossRepos(t *testing.T) {
	srv := multiRepoFixtureServer(t)
	adapter := github.New("test-token", github.WithBaseURL(srv.URL))

	as, err := adapter.FetchActivity(context.Background(), provider.FetchOptions{
		Repos:   []string{"acme/widgets", "acme/gadgets"},
		Subject: "octocat",
		Window: provider.Window{
			Since: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			Until: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		},
	})
	if err != nil {
		t.Fatalf("FetchActivity: %v", err)
	}

	perRepo := map[string]int{}
	var merged int
	for _, pr := range as.PullRequests {
		perRepo[pr.Repo]++
		if pr.MergedAt != nil {
			merged++
		}
	}

	// widgets: 2 in-window subject PRs (1 merged); gadgets: 2 (1 merged).
	if perRepo["acme/widgets"] != 2 || perRepo["acme/gadgets"] != 2 {
		t.Errorf("per-repo PR counts = %v, want acme/widgets:2 acme/gadgets:2", perRepo)
	}
	if got := len(as.PullRequests); got != 4 {
		t.Errorf("total PRs = %d, want 4 (2 + 2 across repos)", got)
	}
	if merged != 2 {
		t.Errorf("merged PRs = %d, want 2 (1 + 1 across repos)", merged)
	}
	if len(as.Repos) != 2 {
		t.Errorf("activity covers repos %v, want both fetched repos for the coverage stamp", as.Repos)
	}
}

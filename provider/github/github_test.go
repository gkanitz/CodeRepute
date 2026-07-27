package github_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gkanitz/coderepute/provider"
	"github.com/gkanitz/coderepute/provider/github"
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
	for _, pr := range []string{"2", "3", "4"} {
		fixture := "reviews_pr" + pr + ".json"
		mux.HandleFunc("GET /repos/acme/widgets/pulls/"+pr+"/reviews", func(w http.ResponseWriter, r *http.Request) {
			serveFixture(t, w, fixture)
		})
	}
	mux.HandleFunc("GET /repos/acme/widgets/pulls/comments", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("page") {
		case "", "1":
			w.Header().Set("Link", fmt.Sprintf(`<%s/repos/acme/widgets/pulls/comments?per_page=100&page=2>; rel="next"`, srv.URL))
			serveFixture(t, w, "comments_page1.json")
		case "2":
			serveFixture(t, w, "comments_page2.json")
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

	t.Run("review timing on subject PRs is derived from others' reviews", func(t *testing.T) {
		byCreated := map[string]provider.PullRequest{}
		for _, pr := range as.PullRequests {
			byCreated[pr.CreatedAt.Format(time.RFC3339)] = pr
		}

		mergedPR, ok := byCreated["2026-03-01T09:00:00Z"]
		if !ok {
			t.Fatal("merged subject PR missing from activity set")
		}
		if mergedPR.FirstReviewAt == nil {
			t.Fatal("merged PR has no FirstReviewAt despite colleague review")
		}
		// The subject's own 10:00 comment-review must not count as the
		// first review; the colleague's 12:00 changes-requested does.
		if want := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC); !mergedPR.FirstReviewAt.Equal(want) {
			t.Errorf("FirstReviewAt = %v, want %v", mergedPR.FirstReviewAt, want)
		}
		if mergedPR.ChangesRequested != 1 {
			t.Errorf("ChangesRequested = %d, want 1", mergedPR.ChangesRequested)
		}

		openPR, ok := byCreated["2026-02-10T11:00:00Z"]
		if !ok {
			t.Fatal("open subject PR missing from activity set")
		}
		if openPR.FirstReviewAt != nil {
			t.Errorf("unreviewed PR carries FirstReviewAt = %v, want nil", openPR.FirstReviewAt)
		}
	})

	t.Run("reviews given are the subject's in-window reviews on others' PRs", func(t *testing.T) {
		if len(as.ReviewsGiven) != 1 {
			t.Fatalf("got %d reviews given, want 1 (self-review and out-of-window review excluded): %+v",
				len(as.ReviewsGiven), as.ReviewsGiven)
		}
		rv := as.ReviewsGiven[0]
		if rv.Repo != "acme/widgets" || rv.State != "APPROVED" {
			t.Errorf("review = %+v, want APPROVED on acme/widgets", rv)
		}
		if want := time.Date(2026, 2, 21, 9, 0, 0, 0, time.UTC); !rv.SubmittedAt.Equal(want) {
			t.Errorf("SubmittedAt = %v, want %v", rv.SubmittedAt, want)
		}
	})

	t.Run("review comment count reflects subject's inline comments on the reviewed PR", func(t *testing.T) {
		if len(as.ReviewsGiven) != 1 {
			t.Fatalf("got %d reviews given, want 1", len(as.ReviewsGiven))
		}
		rv := as.ReviewsGiven[0]
		// The subject left 2 review comments on PR3 (the colleague's PR they
		// approved): comment 7001 at 08:00 and 7003 at 08:30, both in window.
		if rv.CommentCount != 2 {
			t.Errorf("review CommentCount = %d, want 2 (subject left 2 inline comments on that PR)", rv.CommentCount)
		}
	})

	t.Run("review comments split into written and received, across pages", func(t *testing.T) {
		if len(as.ReviewCommentsWritten) != 2 {
			t.Errorf("got %d comments written, want 2 (out-of-window comment excluded): %+v",
				len(as.ReviewCommentsWritten), as.ReviewCommentsWritten)
		}
		if len(as.ReviewCommentsReceived) != 1 {
			t.Errorf("got %d comments received, want 1 (colleague comment on non-subject PR excluded): %+v",
				len(as.ReviewCommentsReceived), as.ReviewCommentsReceived)
		}
	})

	t.Run("activity carries no colleague identities or content", func(t *testing.T) {
		dump := fmt.Sprintf("%+v", as)
		for _, forbidden := range []string{
			"alice-reviewer", "bob-colleague", "777", "888",
			"OAuth secret", "Secret rotation",
		} {
			if strings.Contains(dump, forbidden) {
				t.Errorf("ActivitySet carries prohibited data %q", forbidden)
			}
		}
	})

	t.Run("only metadata endpoints are requested", func(t *testing.T) {
		reviewPath := regexp.MustCompile(`^/repos/[^/]+/[^/]+/pulls/(\d+/reviews|comments)$`)
		for _, p := range requestedPaths() {
			if !strings.HasPrefix(p, "/users/") && !strings.HasSuffix(p, "/pulls") && !reviewPath.MatchString(p) {
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

// TestFetchActivityAllTimeWindow verifies that a zero Since in the window
// means "no lower bound": PRs created before any fixed cutoff are included.
func TestFetchActivityAllTimeWindow(t *testing.T) {
	// Minimal server: one user, one PR list (includes an ancient PR), and
	// stub review/comment endpoints so no 404 errors interrupt the fetch.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /users/octocat", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-OAuth-Scopes", "repo")
		serveFixture(t, w, "user_octocat.json")
	})
	// Return a single page with two PRs: one recent, one ancient (2024).
	mux.HandleFunc("GET /repos/acme/widgets/pulls", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[
			{"number":2,"user":{"login":"octocat","id":583231},"created_at":"2026-02-10T11:00:00Z","merged_at":null,"closed_at":null},
			{"number":1,"user":{"login":"octocat","id":583231},"created_at":"2024-01-05T08:00:00Z","merged_at":"2024-01-06T08:00:00Z","closed_at":"2024-01-06T08:00:00Z"}
		]`))
	})
	mux.HandleFunc("GET /repos/acme/widgets/pulls/1/reviews", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[]`))
	})
	mux.HandleFunc("GET /repos/acme/widgets/pulls/2/reviews", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[]`))
	})
	mux.HandleFunc("GET /repos/acme/widgets/pulls/comments", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[]`))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	adapter := github.New("test-token", github.WithBaseURL(srv.URL))

	// Window with zero Since = fetch all history up to Until.
	as, err := adapter.FetchActivity(context.Background(), provider.FetchOptions{
		Repos:   []string{"acme/widgets"},
		Subject: "octocat",
		Window: provider.Window{
			Until: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		},
	})
	if err != nil {
		t.Fatalf("FetchActivity: %v", err)
	}

	// Both PRs are authored by octocat; without a Since lower bound, the
	// 2024 ancient PR must be included alongside the 2026 PR.
	if len(as.PullRequests) != 2 {
		t.Errorf("all-time window: got %d PRs, want 2 (ancient 2024 PR now in scope): %+v",
			len(as.PullRequests), as.PullRequests)
	}
	var hasAncient bool
	for _, pr := range as.PullRequests {
		if pr.CreatedAt.Year() == 2024 {
			hasAncient = true
		}
	}
	if !hasAncient {
		t.Errorf("all-time window: ancient 2024 PR not found in activity set")
	}
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

// TestFetchActivityWidenedReviewScan verifies that reviews the subject gave on
// PRs created before the coverage window are captured via the updated-at scan.
// See https://github.com/gkanitz/CodeRepute/issues/18
//
// Success criteria (all must pass):
//  1. PR created before Since, review submitted inside the window → counted
//  2. PR created before the window, review submitted before it → not counted
//  3. PR both created and updated inside the window → counted exactly once
//  4. Pagination: old PR updated recently on a late page is not lost
//  5. Authored-PR stats unchanged (created-in-window semantics)
//  6. Review comments logic untouched
func TestFetchActivityWidenedReviewScan(t *testing.T) {
	var mu sync.Mutex
	var requestLog []string
	var srv *httptest.Server

	mux := http.NewServeMux()
	mux.HandleFunc("GET /users/octocat", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-OAuth-Scopes", "repo, read:org")
		serveFixture(t, w, "user_octocat.json")
	})

	// Created-at scan (default sort): returns PRs created in the window.
	// PR4 is subject-authored and created in window.
	// PR3 is colleague-authored and created in window.
	mux.HandleFunc("GET /repos/acme/widgets/pulls", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("sort") == "updated" {
			// Updated-at scan with two pages to test pagination.
			// Page 1: PR4 (recently updated), PR5 (old, updated in window)
			// Page 2: PR6 (old, updated in window)
			switch r.URL.Query().Get("page") {
			case "", "1":
				w.Header().Set("Link", fmt.Sprintf(`<%s/repos/acme/widgets/pulls?state=all&per_page=100&sort=updated&direction=desc&page=2>; rel="next"`, srv.URL))
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`[
					{"number":4,"user":{"login":"octocat","id":583231},"created_at":"2026-03-01T09:00:00Z","updated_at":"2026-05-15T10:00:00Z","merged_at":"2026-03-02T15:30:00Z","closed_at":"2026-03-02T15:30:00Z"},
					{"number":5,"user":{"login":"bob-colleague","id":888},"created_at":"2024-06-01T08:00:00Z","updated_at":"2026-05-10T09:00:00Z","merged_at":null,"closed_at":null}
				]`))
			case "2":
				w.Write([]byte(`[
					{"number":6,"user":{"login":"bob-colleague","id":888},"created_at":"2024-01-15T10:00:00Z","updated_at":"2026-05-05T08:00:00Z","merged_at":"2024-02-01T10:00:00Z","closed_at":"2024-02-01T10:00:00Z"}
				]`))
			default:
				http.Error(w, "no such page", http.StatusNotFound)
			}
			return
		}
		// Created-at scan (default): existing fixture data.
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

	// Review endpoints for all PRs: 2, 3, 4 (existing) + 5, 6 (new old PRs)
	reviewHandlers := map[string]string{
		"2": "reviews_pr2.json",
		"3": "reviews_pr3.json",
		"4": "reviews_pr4.json",
		"5": "reviews_pr5.json",
		"6": "reviews_pr6.json",
	}
	for prNum, fixture := range reviewHandlers {
		// Capture loop variables
		fp := fixture
		pn := prNum
		mux.HandleFunc("GET /repos/acme/widgets/pulls/"+prNum+"/reviews", func(w http.ResponseWriter, r *http.Request) {
			raw, err := os.ReadFile(filepath.Join("testdata", fp))
			if err != nil {
				// Fallback: serve empty reviews if fixture not found
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`[]`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write(raw)
		})
		_ = pn
	}

	// Review comments endpoint (unchanged).
	mux.HandleFunc("GET /repos/acme/widgets/pulls/comments", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("page") {
		case "", "1":
			w.Header().Set("Link", fmt.Sprintf(`<%s/repos/acme/widgets/pulls/comments?per_page=100&page=2>; rel="next"`, srv.URL))
			serveFixture(t, w, "comments_page1.json")
		case "2":
			serveFixture(t, w, "comments_page2.json")
		default:
			http.Error(w, "no such page", http.StatusNotFound)
		}
	})

	wrapped := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestLog = append(requestLog, r.URL.Path)
		mu.Unlock()
		mux.ServeHTTP(w, r)
	})

	srv = httptest.NewServer(wrapped)
	t.Cleanup(srv.Close)

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

	t.Run("SC1: old PR with in-window review counted in reviews_given", func(t *testing.T) {
		// PR5 is created in 2024 (before window) but reviewed by subject
		// at 2026-02-21T10:00:00Z (in window). This should now be counted.
		var foundOldPR bool
		for _, rv := range as.ReviewsGiven {
			if rv.SubmittedAt.Equal(time.Date(2026, 2, 21, 10, 0, 0, 0, time.UTC)) {
				foundOldPR = true
				break
			}
		}
		if !foundOldPR {
			t.Errorf("review on old PR5 not found in reviews_given: %+v", as.ReviewsGiven)
		}
	})

	t.Run("SC2: old PR with out-of-window review not counted", func(t *testing.T) {
		// PR5 has a second COMMENTED review at 2024-12-01 (before window).
		for _, rv := range as.ReviewsGiven {
			if rv.State == "COMMENTED" && rv.SubmittedAt.Year() == 2024 {
				t.Errorf("out-of-window review on old PR counted: %+v", rv)
			}
		}
	})

	t.Run("SC3: no double counting for PRs matched by both scans", func(t *testing.T) {
		// PR4 is created AND updated in window. It appears in both scans
		// but should be processed once.
		if len(as.PullRequests) != 2 {
			t.Fatalf("got %d authored PRs, want 2 (no double counting from updated scan): %+v",
				len(as.PullRequests), as.PullRequests)
		}
	})

	t.Run("SC4: pagination — old PR on late page is still scanned", func(t *testing.T) {
		// PR6 is created in 2024, updated 2026-05-05 (in window), and
		// appears on page 2 of the updated scan. Its review should be counted.
		var foundPR6 bool
		for _, rv := range as.ReviewsGiven {
			if rv.State == "APPROVED" && rv.SubmittedAt.Equal(time.Date(2026, 3, 1, 14, 0, 0, 0, time.UTC)) {
				foundPR6 = true
				break
			}
		}
		if !foundPR6 {
			t.Errorf("review on late-page PR6 not found: %+v", as.ReviewsGiven)
		}
	})

	t.Run("SC5: authored-PR stats use created-in-window semantics", func(t *testing.T) {
		// Only PR4 (created in window) and PR2 (created in window) are
		// authored by the subject — existing behavior unchanged.
		var subjectPRCount int
		for _, pr := range as.PullRequests {
			if pr.Repo == "acme/widgets" {
				subjectPRCount++
			}
		}
		if subjectPRCount != 2 {
			t.Errorf("subject authored PRs = %d, want 2", subjectPRCount)
		}
	})

	t.Run("SC6: review comments logic untouched", func(t *testing.T) {
		// Review comments are fetched via pulls/comments and filtered by
		// timestamp — unchanged by this change.
		if len(as.ReviewCommentsWritten) != 2 {
			t.Errorf("got %d comments written, want 2 (existing fixture): %+v",
				len(as.ReviewCommentsWritten), as.ReviewCommentsWritten)
		}
		if len(as.ReviewCommentsReceived) != 1 {
			t.Errorf("got %d comments received, want 1 (existing fixture): %+v",
				len(as.ReviewCommentsReceived), as.ReviewCommentsReceived)
		}
	})

	t.Run("only metadata endpoints are requested", func(t *testing.T) {
		reviewPath := regexp.MustCompile(`^/repos/[^/]+/[^/]+/pulls/(\d+/reviews|comments)$`)
		for _, p := range requestLog {
			if !strings.HasPrefix(p, "/users/") && !strings.HasSuffix(p, "/pulls") && !reviewPath.MatchString(p) {
				t.Errorf("unexpected API path requested: %s", p)
			}
		}
	})
}

// TestFetchActivityWidenedReviewNoSince verifies that the widened scan is
// skipped when Since is zero (the created scan already captures everything).
func TestFetchActivityWidenedReviewAllTimeWindow(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /users/octocat", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-OAuth-Scopes", "repo")
		serveFixture(t, w, "user_octocat.json")
	})
	mux.HandleFunc("GET /repos/acme/widgets/pulls", func(w http.ResponseWriter, r *http.Request) {
		// Should ONLY be called once (created scan) since Since is zero.
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[
			{"number":2,"user":{"login":"octocat","id":583231},"created_at":"2026-02-10T11:00:00Z","merged_at":null,"closed_at":null}
		]`))
	})
	mux.HandleFunc("GET /repos/acme/widgets/pulls/2/reviews", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[]`))
	})
	mux.HandleFunc("GET /repos/acme/widgets/pulls/comments", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[]`))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	adapter := github.New("test-token", github.WithBaseURL(srv.URL))
	as, err := adapter.FetchActivity(context.Background(), provider.FetchOptions{
		Repos:   []string{"acme/widgets"},
		Subject: "octocat",
		Window: provider.Window{
			Until: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		},
	})
	if err != nil {
		t.Fatalf("FetchActivity: %v", err)
	}
	if len(as.PullRequests) != 1 {
		t.Errorf("got %d PRs, want 1", len(as.PullRequests))
	}
}

// TestFetchActivityAuthorClassification verifies that the author of a reviewed
// PR is classified against the recognition ruleset and the class string is
// recorded on the Review, without leaking the colleague's identity.
func TestFetchActivityAuthorClassification(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/users/octocat":
			w.Header().Set("X-OAuth-Scopes", "repo")
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"login":"octocat","id":583231}`))
		case "/repos/acme/widgets/pulls":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[
				{"number":1,"user":{"login":"copilot[bot]","id":999001,"type":"Bot"},"created_at":"2026-02-15T08:00:00Z","updated_at":"2026-02-20T08:00:00Z"},
				{"number":2,"user":{"login":"human-colleague","id":888001},"created_at":"2026-02-15T08:00:00Z","updated_at":"2026-02-20T08:00:00Z"}
			]`))
		case "/repos/acme/widgets/pulls/1/reviews":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[
				{"user":{"login":"octocat","id":583231},"state":"APPROVED","submitted_at":"2026-02-20T09:00:00Z"}
			]`))
		case "/repos/acme/widgets/pulls/2/reviews":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[
				{"user":{"login":"octocat","id":583231},"state":"COMMENTED","submitted_at":"2026-02-20T10:00:00Z"}
			]`))
		case "/repos/acme/widgets/pulls/comments":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[]`))
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

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

	// Expect two reviews: one on a copilot-authored PR, one on a human-authored PR.
	if len(as.ReviewsGiven) != 2 {
		t.Fatalf("got %d reviews given, want 2", len(as.ReviewsGiven))
	}

	// Review on copilot-authored PR should have AuthorClass = "copilot".
	var copilotReview, humanReview *provider.Review
	for i, rv := range as.ReviewsGiven {
		if rv.AuthorClass == "copilot" {
			copilotReview = &as.ReviewsGiven[i]
		} else if rv.AuthorClass == "" {
			humanReview = &as.ReviewsGiven[i]
		}
	}

	if copilotReview == nil {
		t.Errorf("no review with AuthorClass 'copilot' found; all reviews: %+v", as.ReviewsGiven)
	}
	if humanReview == nil {
		t.Errorf("no review with AuthorClass '' (human) found; all reviews: %+v", as.ReviewsGiven)
	}

	// Verify that no colleague identity (login, ID) appears in ReviewsGiven.
	dump := fmt.Sprintf("%+v", as.ReviewsGiven)
	for _, forbidden := range []string{"copilot[bot]", "999001", "888001"} {
		if strings.Contains(dump, forbidden) {
			t.Errorf("Review carries prohibited colleague identity %q", forbidden)
		}
	}
}

// TestFetchActivityBotAuthorClassification verifies that a PR authored by an
// unknown bot (matched only by type:"Bot" or *[bot] login) gets AuthorClass "bot".
func TestFetchActivityBotAuthorClassification(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/users/octocat":
			w.Header().Set("X-OAuth-Scopes", "repo")
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"login":"octocat","id":583231}`))
		case "/repos/acme/widgets/pulls":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[
				{"number":1,"user":{"login":"some-unknown-tool[bot]","id":999002,"type":"Bot"},"created_at":"2026-02-15T08:00:00Z","updated_at":"2026-02-20T08:00:00Z"}
			]`))
		case "/repos/acme/widgets/pulls/1/reviews":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[
				{"user":{"login":"octocat","id":583231},"state":"APPROVED","submitted_at":"2026-02-20T09:00:00Z"}
			]`))
		case "/repos/acme/widgets/pulls/comments":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[]`))
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

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

	if len(as.ReviewsGiven) != 1 {
		t.Fatalf("got %d reviews given, want 1", len(as.ReviewsGiven))
	}
	if as.ReviewsGiven[0].AuthorClass != "bot" {
		t.Errorf("AuthorClass = %q, want %q (unknown bot should get 'bot')", as.ReviewsGiven[0].AuthorClass, "bot")
	}
}

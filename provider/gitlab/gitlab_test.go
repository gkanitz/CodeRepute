package gitlab_test

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

	"github.com/grkanitz/coderepute/provider"
	"github.com/grkanitz/coderepute/provider/gitlab"
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

// fixtureServer replays recorded GitLab v4 API responses and records every
// path the adapter requests. GitLab addresses projects by URL-encoded path
// ("acme%2Fwidgets"), so routing dispatches on the escaped path rather
// than a ServeMux pattern.
func fixtureServer(t *testing.T) (*httptest.Server, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var paths []string

	var srv *httptest.Server
	srvURL := func() string { return srv.URL }
	handler := func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.EscapedPath())
		mu.Unlock()
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("request to %s carried Authorization %q, want bearer test-token", r.URL.Path, got)
		}

		q := r.URL.Query()
		switch r.URL.EscapedPath() {
		case "/users":
			if q.Get("username") != "devmara" {
				http.Error(w, `{"message":"unexpected username"}`, http.StatusBadRequest)
				return
			}
			serveFixture(t, w, "users_devmara.json")
		case "/personal_access_tokens/self":
			serveFixture(t, w, "token_self.json")
		case "/projects/acme%2Fwidgets/merge_requests/1/notes":
			serveFixture(t, w, "notes_mr1.json")
		case "/projects/acme%2Fwidgets/merge_requests/2/notes":
			serveFixture(t, w, "notes_mr2.json")
		case "/projects/acme%2Fwidgets/merge_requests/3/notes":
			serveFixture(t, w, "notes_mr3.json")
		case "/projects/acme%2Fwidgets/merge_requests/4/notes":
			serveFixture(t, w, "notes_mr4.json")
		case "/projects/acme%2Fwidgets/merge_requests":
			switch q.Get("page") {
			case "", "1":
				w.Header().Set("Link", `<`+srvURL()+`/projects/acme%2Fwidgets/merge_requests?scope=all&per_page=100&page=2>; rel="next"`)
				serveFixture(t, w, "mrs_widgets_page1.json")
			case "2":
				serveFixture(t, w, "mrs_widgets_page2.json")
			default:
				http.Error(w, "no such page", http.StatusNotFound)
			}
		default:
			http.Error(w, `{"message":"404 Not Found"}`, http.StatusNotFound)
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

func fetchWidgetsActivity(t *testing.T) (provider.ActivitySet, func() []string, provider.Window) {
	t.Helper()
	srv, requestedPaths := fixtureServer(t)

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
	return as, requestedPaths, window
}

func TestFetchActivitySubjectIdentity(t *testing.T) {
	as, _, window := fetchWidgetsActivity(t)

	t.Run("subject bound to immutable account id", func(t *testing.T) {
		want := provider.Subject{Platform: "gitlab", Username: "devmara", AccountID: "4711"}
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
		if as.TokenScope != "read_api" {
			t.Errorf("token scope = %q, want %q", as.TokenScope, "read_api")
		}
	})
}

func TestFetchActivityPullRequests(t *testing.T) {
	as, _, _ := fetchWidgetsActivity(t)

	t.Run("only subject-account MRs inside window, across pages", func(t *testing.T) {
		if len(as.PullRequests) != 2 {
			t.Fatalf("got %d MRs, want 2 (impostor account and out-of-window MR excluded): %+v",
				len(as.PullRequests), as.PullRequests)
		}
		var mergedCount int
		for _, pr := range as.PullRequests {
			if pr.Repo != "acme/widgets" {
				t.Errorf("MR repo = %q, want acme/widgets", pr.Repo)
			}
			if pr.MergedAt != nil {
				mergedCount++
			}
		}
		if mergedCount != 1 {
			t.Errorf("merged count = %d, want 1", mergedCount)
		}
	})

	t.Run("merge timestamps survive normalization", func(t *testing.T) {
		var merged *provider.PullRequest
		for i := range as.PullRequests {
			if as.PullRequests[i].MergedAt != nil {
				merged = &as.PullRequests[i]
			}
		}
		if merged == nil {
			t.Fatal("no merged MR in activity set")
		}
		if want := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC); !merged.CreatedAt.Equal(want) {
			t.Errorf("CreatedAt = %v, want %v", merged.CreatedAt, want)
		}
		if want := time.Date(2026, 3, 2, 15, 30, 0, 0, time.UTC); !merged.MergedAt.Equal(want) {
			t.Errorf("MergedAt = %v, want %v", merged.MergedAt, want)
		}
	})
}

func TestFetchActivityReviewTiming(t *testing.T) {
	as, _, _ := fetchWidgetsActivity(t)

	byCreated := map[string]provider.PullRequest{}
	for _, pr := range as.PullRequests {
		byCreated[pr.CreatedAt.Format(time.RFC3339)] = pr
	}

	t.Run("first review comes from others' review activity, never the subject's own", func(t *testing.T) {
		mergedMR, ok := byCreated["2026-03-01T09:00:00Z"]
		if !ok {
			t.Fatal("merged subject MR missing from activity set")
		}
		if mergedMR.FirstReviewAt == nil {
			t.Fatal("merged MR has no FirstReviewAt despite colleague review")
		}
		// The subject's own 10:00 diff note must not count as the first
		// review; the colleague's 12:00 requested-changes event does.
		if want := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC); !mergedMR.FirstReviewAt.Equal(want) {
			t.Errorf("FirstReviewAt = %v, want %v", mergedMR.FirstReviewAt, want)
		}
	})

	t.Run("rework signal counts others' requested-changes events", func(t *testing.T) {
		mergedMR := byCreated["2026-03-01T09:00:00Z"]
		if mergedMR.ChangesRequested != 1 {
			t.Errorf("ChangesRequested = %d, want 1", mergedMR.ChangesRequested)
		}
	})

	t.Run("unreviewed MR carries no first review", func(t *testing.T) {
		openMR, ok := byCreated["2026-02-10T11:00:00Z"]
		if !ok {
			t.Fatal("open subject MR missing from activity set")
		}
		if openMR.FirstReviewAt != nil {
			t.Errorf("unreviewed MR carries FirstReviewAt = %v, want nil", openMR.FirstReviewAt)
		}
	})
}

func TestFetchActivityReviewsGiven(t *testing.T) {
	as, _, _ := fetchWidgetsActivity(t)

	if len(as.ReviewsGiven) != 1 {
		t.Fatalf("got %d reviews given, want 1 (colleague's events and out-of-window event excluded): %+v",
			len(as.ReviewsGiven), as.ReviewsGiven)
	}
	rv := as.ReviewsGiven[0]
	if rv.Repo != "acme/widgets" || rv.State != "APPROVED" {
		t.Errorf("review = %+v, want APPROVED on acme/widgets", rv)
	}
	if want := time.Date(2026, 2, 21, 9, 0, 0, 0, time.UTC); !rv.SubmittedAt.Equal(want) {
		t.Errorf("SubmittedAt = %v, want %v", rv.SubmittedAt, want)
	}

	t.Run("review comment count reflects subject diff notes on the reviewed MR", func(t *testing.T) {
		// MR3 (owned by impostor, notes_mr3.json): subject has 1 in-window
		// APPROVED system note and no DiffNotes. CommentCount must be 0.
		if rv.CommentCount != 0 {
			t.Errorf("review CommentCount = %d, want 0 (no subject diff notes on MR3)", rv.CommentCount)
		}
	})
}

// TestFetchActivityAllTimeWindow verifies that a zero Since in the window
// means "no lower bound": MRs created before any fixed cutoff are included.
func TestFetchActivityAllTimeWindow(t *testing.T) {
	// Minimal server: one user, one MR list with an ancient MR, and empty
	// notes. Without a Since bound, the ancient MR must be included.
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.EscapedPath() {
		case "/users":
			w.Write([]byte(`[{"id":4711,"username":"devmara"}]`))
		case "/personal_access_tokens/self":
			w.Write([]byte(`{"scopes":["read_api"]}`))
		case "/projects/acme%2Fwidgets/merge_requests":
			w.Write([]byte(`[
				{"iid":2,"author":{"id":4711,"username":"devmara"},"created_at":"2026-02-10T11:00:00Z","merged_at":null,"closed_at":null},
				{"iid":1,"author":{"id":4711,"username":"devmara"},"created_at":"2024-01-05T08:00:00Z","merged_at":"2024-01-06T08:00:00Z","closed_at":null}
			]`))
		case "/projects/acme%2Fwidgets/merge_requests/1/notes",
			"/projects/acme%2Fwidgets/merge_requests/2/notes":
			w.Write([]byte(`[]`))
		default:
			http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(handler))
	t.Cleanup(srv.Close)

	adapter := gitlab.New("test-token", gitlab.WithBaseURL(srv.URL))
	as, err := adapter.FetchActivity(context.Background(), provider.FetchOptions{
		Repos:   []string{"acme/widgets"},
		Subject: "devmara",
		Window: provider.Window{
			Until: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		},
	})
	if err != nil {
		t.Fatalf("FetchActivity: %v", err)
	}

	// Both MRs are authored by devmara; without a Since lower bound, the
	// 2024 ancient MR must be included alongside the 2026 MR.
	if len(as.PullRequests) != 2 {
		t.Errorf("all-time window: got %d MRs, want 2 (ancient 2024 MR now in scope): %+v",
			len(as.PullRequests), as.PullRequests)
	}
	var hasAncient bool
	for _, pr := range as.PullRequests {
		if pr.CreatedAt.Year() == 2024 {
			hasAncient = true
		}
	}
	if !hasAncient {
		t.Errorf("all-time window: ancient 2024 MR not found in activity set")
	}
}

func TestFetchActivityReviewComments(t *testing.T) {
	as, _, _ := fetchWidgetsActivity(t)

	t.Run("written: subject diff notes in window, anywhere", func(t *testing.T) {
		// The subject's diff note on their own MR and on the colleague's
		// MR count; the out-of-window drive-by does not.
		if len(as.ReviewCommentsWritten) != 2 {
			t.Errorf("got %d comments written, want 2: %+v",
				len(as.ReviewCommentsWritten), as.ReviewCommentsWritten)
		}
	})

	t.Run("received: others' diff notes on subject MRs only", func(t *testing.T) {
		// The colleague's diff note on the subject's MR counts; their
		// diff note on their own MR and their plain (non-diff) comment
		// on the subject's MR do not.
		if len(as.ReviewCommentsReceived) != 1 {
			t.Fatalf("got %d comments received, want 1: %+v",
				len(as.ReviewCommentsReceived), as.ReviewCommentsReceived)
		}
		c := as.ReviewCommentsReceived[0]
		if want := time.Date(2026, 3, 1, 12, 30, 0, 0, time.UTC); !c.CreatedAt.Equal(want) {
			t.Errorf("received comment CreatedAt = %v, want %v", c.CreatedAt, want)
		}
	})
}

func TestFetchActivityPrivacyBoundary(t *testing.T) {
	as, requestedPaths, _ := fetchWidgetsActivity(t)

	t.Run("activity carries no colleague identities or content", func(t *testing.T) {
		dump := fmt.Sprintf("%+v", as)
		for _, forbidden := range []string{
			"nadia-colleague", "9301", "999999",
			"Add payment retry logic", "Refactor token vault", "Impostor change",
			"OAuth secret", "Secret rotation", "Looks good overall",
			"feature/payment-retries",
		} {
			if strings.Contains(dump, forbidden) {
				t.Errorf("ActivitySet carries prohibited data %q", forbidden)
			}
		}
	})

	t.Run("only metadata endpoints are requested", func(t *testing.T) {
		notesPath := regexp.MustCompile(`^/projects/[^/]+/merge_requests(/\d+/notes)?$`)
		for _, p := range requestedPaths() {
			if p != "/users" && p != "/personal_access_tokens/self" && !notesPath.MatchString(p) {
				t.Errorf("unexpected API path requested: %s", p)
			}
			for _, forbidden := range []string{"/repository", "/files", "/archive", ".git"} {
				if strings.Contains(p, forbidden) {
					t.Errorf("adapter touched repository content endpoint: %s", p)
				}
			}
		}
	})
}

func TestFetchActivityUnknownUser(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() == "/users" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("[]")) // GitLab returns an empty list, not 404
			return
		}
		http.Error(w, `{"message":"404 Not Found"}`, http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	adapter := gitlab.New("test-token", gitlab.WithBaseURL(srv.URL))
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
	if !strings.Contains(err.Error(), "ghost") {
		t.Errorf("error %q does not name the unknown subject", err)
	}
}

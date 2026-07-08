package provider_test

// Cross-adapter parity: the same real-world collaboration story, recorded
// once in GitHub API shape and once in GitLab API shape, must normalize
// to equivalent ActivitySets and produce identical metric values. This is
// the test that keeps the provider abstraction honest — platform
// divergences must be resolved inside the adapters, never leaked upward.
//
// The story (fixtures under testdata/parity_*):
//   - PR/MR 1, authored by the subject, created 2026-02-01 10:00, merged
//     2026-02-03 10:00. A colleague requested changes at 15:00, left a
//     diff comment at 16:00, and approved at 09:00 the next day.
//   - PR/MR 2, authored by the colleague. The subject left a diff comment
//     (a thread reply on GitHub, so no review object) at 11:00 and
//     approved at 12:00 on 2026-02-11.
//   - PR/MR 3, authored by the subject on 2026-03-05, still open and
//     unreviewed.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/gkanitz/coderepute/metrics"
	"github.com/gkanitz/coderepute/provider"
	"github.com/gkanitz/coderepute/provider/github"
	"github.com/gkanitz/coderepute/provider/gitlab"
)

func parityFixture(t *testing.T, w http.ResponseWriter, name string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(raw)
}

func githubParityServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/users/devmara":
			w.Header().Set("X-OAuth-Scopes", "repo, read:org")
			parityFixture(t, w, "parity_github_user.json")
		case "/repos/acme/widgets/pulls":
			parityFixture(t, w, "parity_github_pulls.json")
		case "/repos/acme/widgets/pulls/1/reviews":
			parityFixture(t, w, "parity_github_reviews_pr1.json")
		case "/repos/acme/widgets/pulls/2/reviews":
			parityFixture(t, w, "parity_github_reviews_pr2.json")
		case "/repos/acme/widgets/pulls/3/reviews":
			parityFixture(t, w, "parity_github_reviews_pr3.json")
		case "/repos/acme/widgets/pulls/comments":
			parityFixture(t, w, "parity_github_comments.json")
		default:
			http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func gitlabParityServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.EscapedPath() {
		case "/users":
			parityFixture(t, w, "parity_gitlab_users.json")
		case "/personal_access_tokens/self":
			parityFixture(t, w, "parity_gitlab_token.json")
		case "/projects/acme%2Fwidgets/merge_requests":
			parityFixture(t, w, "parity_gitlab_mrs.json")
		case "/projects/acme%2Fwidgets/merge_requests/1/notes":
			parityFixture(t, w, "parity_gitlab_notes_mr1.json")
		case "/projects/acme%2Fwidgets/merge_requests/2/notes":
			parityFixture(t, w, "parity_gitlab_notes_mr2.json")
		case "/projects/acme%2Fwidgets/merge_requests/3/notes":
			parityFixture(t, w, "parity_gitlab_notes_mr3.json")
		default:
			http.Error(w, `{"message":"404 Not Found"}`, http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func githubDiffShapeParityServer(t *testing.T) *httptest.Server {
	t.Helper()
	var page int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/graphql" {
			page++
			if page == 1 {
				parityFixture(t, w, "parity_github_diffshape_page1.json")
			} else {
				parityFixture(t, w, "parity_github_diffshape_page2.json")
			}
			return
		}
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func gitlabDiffShapeParityServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() == "/graphql" {
			parityFixture(t, w, "parity_gitlab_diffshape.json")
			return
		}
		http.Error(w, `{"message":"404 Not Found"}`, http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestCrossAdapterDiffShapeParity(t *testing.T) {
	ghAdapter := github.New("test-token", github.WithBaseURL(githubDiffShapeParityServer(t).URL))
	glAdapter := gitlab.New("test-token", gitlab.WithBaseURL(gitlabDiffShapeParityServer(t).URL))

	ghStats, err := ghAdapter.FetchDiffShape(context.Background(), "acme/widgets", 1)
	if err != nil {
		t.Fatalf("github FetchDiffShape: %v", err)
	}
	glStats, err := glAdapter.FetchDiffShape(context.Background(), "acme/widgets", 1)
	if err != nil {
		t.Fatalf("gitlab FetchDiffShape: %v", err)
	}

	if len(ghStats) != len(glStats) {
		t.Fatalf("github: %d stats, gitlab: %d stats", len(ghStats), len(glStats))
	}
	for i := range ghStats {
		if ghStats[i] != glStats[i] {
			t.Errorf("stat[%d] diverges: github %+v vs gitlab %+v", i, ghStats[i], glStats[i])
		}
	}
}

func TestCrossAdapterParity(t *testing.T) {
	window := provider.Window{
		Since: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Until: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	}
	opts := provider.FetchOptions{
		Repos:   []string{"acme/widgets"},
		Subject: "devmara",
		Window:  window,
	}

	ghAdapter := github.New("test-token", github.WithBaseURL(githubParityServer(t).URL))
	ghActivity, err := ghAdapter.FetchActivity(context.Background(), opts)
	if err != nil {
		t.Fatalf("github FetchActivity: %v", err)
	}

	glAdapter := gitlab.New("test-token", gitlab.WithBaseURL(gitlabParityServer(t).URL))
	glActivity, err := glAdapter.FetchActivity(context.Background(), opts)
	if err != nil {
		t.Fatalf("gitlab FetchActivity: %v", err)
	}

	t.Run("each adapter binds the subject to its own platform identity", func(t *testing.T) {
		wantGH := provider.Subject{Platform: "github", Username: "devmara", AccountID: "1001"}
		if ghActivity.Subject != wantGH {
			t.Errorf("github subject = %+v, want %+v", ghActivity.Subject, wantGH)
		}
		wantGL := provider.Subject{Platform: "gitlab", Username: "devmara", AccountID: "4711"}
		if glActivity.Subject != wantGL {
			t.Errorf("gitlab subject = %+v, want %+v", glActivity.Subject, wantGL)
		}
	})

	t.Run("equivalent fixtures normalize to equivalent activity sets", func(t *testing.T) {
		// Subject identity, raw token scopes, and access manifest are
		// inherently platform-specific; everything else must match exactly.
		gh, gl := ghActivity, glActivity
		gh.Subject, gl.Subject = provider.Subject{}, provider.Subject{}
		gh.TokenScope, gl.TokenScope = "", ""
		gh.AccessManifest, gl.AccessManifest = provider.Manifest{}, provider.Manifest{}
		if !reflect.DeepEqual(gh, gl) {
			t.Errorf("activity sets diverge:\ngithub: %+v\ngitlab: %+v", gh, gl)
		}
	})

	t.Run("equivalent activity yields identical metric values", func(t *testing.T) {
		ghMetrics := metrics.Compute(ghActivity)
		glMetrics := metrics.Compute(glActivity)
		if !reflect.DeepEqual(ghMetrics, glMetrics) {
			t.Errorf("metrics diverge:\ngithub: %+v\ngitlab: %+v", ghMetrics, glMetrics)
		}
	})

	t.Run("sanity: the story is non-trivial", func(t *testing.T) {
		if len(ghActivity.PullRequests) != 2 || len(ghActivity.ReviewsGiven) != 1 ||
			len(ghActivity.ReviewCommentsWritten) != 1 || len(ghActivity.ReviewCommentsReceived) != 1 {
			t.Errorf("github activity unexpectedly shaped: %+v", ghActivity)
		}
		var merged int
		for _, pr := range ghActivity.PullRequests {
			if pr.MergedAt != nil {
				merged++
			}
		}
		if merged != 1 {
			t.Errorf("merged PRs = %d, want 1", merged)
		}
	})
}

// TestCrossAdapterParityWidenedScan verifies that GitHub and GitLab adapters
// produce identical results for the widened review scan (SC5 from #18):
// equivalent old-PR-with-in-window-review fixtures produce identical
// reviews_given totals.
func TestCrossAdapterParityWidenedScan(t *testing.T) {
	window := provider.Window{
		Since: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Until: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	}

	// GitHub fixture: 1 colleague PR created before window, updated in window,
	// with one subject APPROVED review in window.
	ghSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/users/devmara":
			w.Header().Set("X-OAuth-Scopes", "repo, read:org")
			parityFixture(t, w, "parity_github_user.json")
		case "/repos/acme/widgets/pulls":
			// Returns old PR for both created and updated scans.
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[
				{"number":1,"user":{"login":"nadia-colleague","id":2002},"created_at":"2024-06-01T08:00:00Z","updated_at":"2026-02-20T10:00:00Z","merged_at":null,"closed_at":null}
			]`))
		case "/repos/acme/widgets/pulls/1/reviews":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[
				{"id":601,"user":{"login":"devmara","id":1001},"state":"APPROVED","body":"LGTM","submitted_at":"2026-02-20T12:00:00Z"}
			]`))
		case "/repos/acme/widgets/pulls/comments":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[]`))
		default:
			http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
		}
	}))
	t.Cleanup(ghSrv.Close)

	// GitLab fixture: 1 colleague MR created before window, updated in window,
	// with one subject APPROVED review in window.
	glSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.EscapedPath() {
		case "/users":
			parityFixture(t, w, "parity_gitlab_users.json")
		case "/personal_access_tokens/self":
			parityFixture(t, w, "parity_gitlab_token.json")
		case "/projects/acme%2Fwidgets/merge_requests":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[
				{"iid":1,"author":{"id":9301,"username":"nadia-colleague"},"created_at":"2024-06-01T08:00:00Z","updated_at":"2026-02-20T10:00:00Z","merged_at":null,"closed_at":null}
			]`))
		case "/projects/acme%2Fwidgets/merge_requests/1/notes":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[
				{"id":7801,"type":null,"system":true,"author":{"id":4711,"username":"devmara"},"body":"approved this merge request","created_at":"2026-02-20T12:00:00Z"}
			]`))
		default:
			http.Error(w, `{"message":"404 Not Found"}`, http.StatusNotFound)
		}
	}))
	t.Cleanup(glSrv.Close)

	ghAdapter := github.New("test-token", github.WithBaseURL(ghSrv.URL))
	ghActivity, err := ghAdapter.FetchActivity(context.Background(), provider.FetchOptions{
		Repos:   []string{"acme/widgets"},
		Subject: "devmara",
		Window:  window,
	})
	if err != nil {
		t.Fatalf("github FetchActivity: %v", err)
	}

	glAdapter := gitlab.New("test-token", gitlab.WithBaseURL(glSrv.URL))
	glActivity, err := glAdapter.FetchActivity(context.Background(), provider.FetchOptions{
		Repos:   []string{"acme/widgets"},
		Subject: "devmara",
		Window:  window,
	})
	if err != nil {
		t.Fatalf("gitlab FetchActivity: %v", err)
	}

	t.Run("both adapters capture the review on the old PR/MR", func(t *testing.T) {
		if len(ghActivity.ReviewsGiven) != 1 {
			t.Errorf("github: got %d reviews given, want 1: %+v", len(ghActivity.ReviewsGiven), ghActivity.ReviewsGiven)
		}
		if len(glActivity.ReviewsGiven) != 1 {
			t.Errorf("gitlab: got %d reviews given, want 1: %+v", len(glActivity.ReviewsGiven), glActivity.ReviewsGiven)
		}
	})

	t.Run("identical reviews_given totals across adapters", func(t *testing.T) {
		if len(ghActivity.ReviewsGiven) != len(glActivity.ReviewsGiven) {
			t.Errorf("reviews_given: github=%d gitlab=%d", len(ghActivity.ReviewsGiven), len(glActivity.ReviewsGiven))
		}
	})

	t.Run("no authored PRs from old PR (created-in-window semantics)", func(t *testing.T) {
		if len(ghActivity.PullRequests) != 0 {
			t.Errorf("github: got %d authored PRs, want 0 (old PR not subject-authored)", len(ghActivity.PullRequests))
		}
		if len(glActivity.PullRequests) != 0 {
			t.Errorf("gitlab: got %d authored MRs, want 0 (old MR not subject-authored)", len(glActivity.PullRequests))
		}
	})
}

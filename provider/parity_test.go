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

	"github.com/grkanitz/coderepute/metrics"
	"github.com/grkanitz/coderepute/provider"
	"github.com/grkanitz/coderepute/provider/github"
	"github.com/grkanitz/coderepute/provider/gitlab"
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
		// Subject identity and raw token scopes are inherently
		// platform-specific; everything else must match exactly.
		gh, gl := ghActivity, glActivity
		gh.Subject, gl.Subject = provider.Subject{}, provider.Subject{}
		gh.TokenScope, gl.TokenScope = "", ""
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

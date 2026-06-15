package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/grkanitz/coderepute/report"
)

func fixtureServer(t *testing.T) *httptest.Server {
	t.Helper()
	serve := func(name string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			raw, err := os.ReadFile(filepath.Join("testdata", name))
			if err != nil {
				t.Errorf("read fixture %s: %v", name, err)
				http.Error(w, "fixture missing", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write(raw)
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /users/octocat", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-OAuth-Scopes", "repo")
		serve("user_octocat.json")(w, r)
	})
	mux.HandleFunc("GET /repos/acme/widgets/pulls", serve("pulls_page1.json"))
	mux.HandleFunc("GET /repos/acme/widgets/pulls/4/reviews", serve("reviews_pr4.json"))
	mux.HandleFunc("GET /repos/acme/widgets/pulls/3/reviews", serve("reviews_pr3.json"))
	mux.HandleFunc("GET /repos/acme/widgets/pulls/comments", serve("comments_page1.json"))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestRunEndToEnd(t *testing.T) {
	srv := fixtureServer(t)
	out := t.TempDir()

	var stderr bytes.Buffer
	code := run([]string{
		"-repo", "acme/widgets",
		"-subject", "octocat",
		"-token", "test-token",
		"-window-days", "365",
		"-out", out,
		"-api-base", srv.URL,
	}, func(string) string { return "" }, &stderr)
	if code != 0 {
		t.Fatalf("run exited %d: %s", code, stderr.String())
	}

	rawJSON, err := os.ReadFile(filepath.Join(out, "report.json"))
	if err != nil {
		t.Fatalf("report.json not written: %v", err)
	}
	r, err := report.Parse(rawJSON)
	if err != nil {
		t.Fatalf("report.json invalid: %v", err)
	}
	if r.Subject.AccountID != "583231" {
		t.Errorf("subject account id = %q, want 583231", r.Subject.AccountID)
	}
	if r.Verification.Status != report.StatusUnverified {
		t.Errorf("local run verification = %q, want unverified", r.Verification.Status)
	}
	if r.Collaboration == nil || r.Collaboration.PullRequests == nil {
		t.Fatal("collaboration.pull_requests missing")
	}
	collab := r.Collaboration
	if collab.ReviewsGiven == nil || collab.ReviewsGiven.Total != 1 {
		t.Errorf("collaboration.reviews_given = %+v, want total 1", collab.ReviewsGiven)
	}
	if collab.ReviewComments == nil || collab.ReviewComments.Written != 1 || collab.ReviewComments.Received != 1 {
		t.Errorf("collaboration.review_comments = %+v, want written 1 received 1", collab.ReviewComments)
	}
	if collab.TimeToMerge == nil || collab.TimeToMerge.Count != 1 {
		t.Errorf("collaboration.time_to_merge = %+v, want count 1", collab.TimeToMerge)
	}
	if collab.TimeToFirstReview == nil || collab.TimeToFirstReview.Count != 1 {
		t.Errorf("collaboration.time_to_first_review = %+v, want count 1", collab.TimeToFirstReview)
	}
	if collab.Rework == nil || collab.Rework.ReviewedPRs != 1 || collab.Rework.ReworkedPRs != 1 {
		t.Errorf("collaboration.rework = %+v, want 1 reviewed, 1 reworked", collab.Rework)
	}
	if r.Cadence == nil {
		t.Fatal("cadence section missing")
	}
	if len(r.Cadence.Trend) == 0 {
		t.Error("cadence.trend has no buckets despite a non-empty coverage window")
	}
	if strings.Contains(strings.ToLower(string(rawJSON)), `"score"`) {
		t.Error("report.json contains a score field; no composite score is allowed")
	}

	rawHTML, err := os.ReadFile(filepath.Join(out, "report.html"))
	if err != nil {
		t.Fatalf("report.html not written: %v", err)
	}

	// Privacy boundary: strings seeded in the API fixtures must never
	// surface in any output.
	for _, doc := range []struct {
		name string
		raw  []byte
	}{
		{"report.json", rawJSON},
		{"report.html", rawHTML},
	} {
		for _, forbidden := range []string{
			"Add payment retry logic",      // PR title
			"Impostor change",              // PR title
			"feature/payment-retries",      // branch name
			"999999",                       // other account's ID
			"alice-reviewer",               // colleague username
			"bob-colleague",                // colleague username
			"OAuth secret",                 // colleague review body
			"Secret rotation",              // colleague comment body
			"This retry loop looks risky",  // subject's own comment body
			"Replying to my own PR thread", // subject's own review body
		} {
			if strings.Contains(string(doc.raw), forbidden) {
				t.Errorf("%s leaks prohibited data %q", doc.name, forbidden)
			}
		}
	}
}

func TestRunTrimsRepoListWhitespace(t *testing.T) {
	srv := fixtureServer(t)
	out := t.TempDir()

	var stderr bytes.Buffer
	code := run([]string{
		"-repo", " acme/widgets ,",
		"-subject", "octocat",
		"-token", "test-token",
		"-out", out,
		"-api-base", srv.URL,
	}, func(string) string { return "" }, &stderr)
	if code != 0 {
		t.Fatalf("run exited %d: %s", code, stderr.String())
	}

	rawJSON, err := os.ReadFile(filepath.Join(out, "report.json"))
	if err != nil {
		t.Fatalf("report.json not written: %v", err)
	}
	r, err := report.Parse(rawJSON)
	if err != nil {
		t.Fatalf("report.json invalid: %v", err)
	}
	if len(r.Coverage.Repos) != 1 || r.Coverage.Repos[0] != "acme/widgets" {
		t.Errorf("coverage repos = %v, want [acme/widgets]", r.Coverage.Repos)
	}
}

func TestRunInGitHubActionsUpgradesVerification(t *testing.T) {
	srv := fixtureServer(t)
	out := t.TempDir()

	env := map[string]string{
		"GITHUB_ACTIONS":      "true",
		"GITHUB_REPOSITORY":   "acme/widgets",
		"GITHUB_WORKFLOW_REF": "acme/widgets/.github/workflows/report.yml@refs/heads/main",
		"GITHUB_RUN_ID":       "9000000001",
		"GITHUB_SERVER_URL":   "https://github.com",
	}
	var stderr bytes.Buffer
	code := run([]string{
		"-repo", "acme/widgets",
		"-subject", "octocat",
		"-token", "test-token",
		"-out", out,
		"-api-base", srv.URL,
	}, func(key string) string { return env[key] }, &stderr)
	if code != 0 {
		t.Fatalf("run exited %d: %s", code, stderr.String())
	}

	rawJSON, err := os.ReadFile(filepath.Join(out, "report.json"))
	if err != nil {
		t.Fatalf("report.json not written: %v", err)
	}
	r, err := report.Parse(rawJSON)
	if err != nil {
		t.Fatalf("report.json invalid: %v", err)
	}
	if r.Verification.Status != report.StatusVerified {
		t.Errorf("CI run verification = %q, want verified", r.Verification.Status)
	}
	if want := "acme/widgets/.github/workflows/report.yml@refs/heads/main"; r.Verification.WorkflowRef != want {
		t.Errorf("workflow_ref = %q, want %q", r.Verification.WorkflowRef, want)
	}
	if r.Verification.Attestation == nil {
		t.Error("CI run verification block carries no attestation pointer")
	}
}

func TestRunRejectsMissingArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"no repo", []string{"-subject", "octocat", "-token", "t"}},
		{"no subject", []string{"-repo", "acme/widgets", "-token", "t"}},
		{"no token", []string{"-repo", "acme/widgets", "-subject", "octocat"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stderr bytes.Buffer
			if code := run(tt.args, func(string) string { return "" }, &stderr); code == 0 {
				t.Error("run succeeded despite missing required argument")
			}
		})
	}
}

func TestRunTokenFromEnv(t *testing.T) {
	srv := fixtureServer(t)
	out := t.TempDir()

	var stderr bytes.Buffer
	code := run([]string{
		"-repo", "acme/widgets",
		"-subject", "octocat",
		"-out", out,
		"-api-base", srv.URL,
	}, func(key string) string {
		if key == "GITHUB_TOKEN" {
			return "env-token"
		}
		return ""
	}, &stderr)
	if code != 0 {
		t.Fatalf("run exited %d: %s", code, stderr.String())
	}
}

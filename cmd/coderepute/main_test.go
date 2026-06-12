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
			"Add payment retry logic", // PR title
			"Impostor change",         // PR title
			"feature/payment-retries", // branch name
			"999999",                  // other account's ID
		} {
			if strings.Contains(string(doc.raw), forbidden) {
				t.Errorf("%s leaks prohibited data %q", doc.name, forbidden)
			}
		}
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

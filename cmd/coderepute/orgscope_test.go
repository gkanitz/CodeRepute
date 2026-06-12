package main

// End-to-end org-scoped runs: enumerate every repo the token can see,
// aggregate the subject's activity into one report, and stamp coverage
// with the full repo list and token scope class.

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"github.com/grkanitz/coderepute/report"
)

// orgFixtureServer serves org repo enumeration plus activity for the two
// enumerated repos, and records the Authorization header of API calls.
func orgFixtureServer(t *testing.T) (*httptest.Server, func() []string) {
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
	mux.HandleFunc("GET /orgs/acme/repos", serve("org_repos.json"))
	mux.HandleFunc("GET /users/octocat", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-OAuth-Scopes", "repo, read:org")
		serve("user_octocat.json")(w, r)
	})
	mux.HandleFunc("GET /repos/acme/widgets/pulls", serve("pulls_page1.json"))
	mux.HandleFunc("GET /repos/acme/gadgets/pulls", serve("pulls_gadgets.json"))

	var mu sync.Mutex
	var auths []string
	wrapped := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		auths = append(auths, r.Header.Get("Authorization"))
		mu.Unlock()
		mux.ServeHTTP(w, r)
	})

	srv := httptest.NewServer(wrapped)
	t.Cleanup(srv.Close)
	return srv, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), auths...)
	}
}

func TestRunOrgScoped(t *testing.T) {
	srv, _ := orgFixtureServer(t)
	out := t.TempDir()

	var stderr bytes.Buffer
	code := run([]string{
		"-org", "acme",
		"-subject", "octocat",
		"-token", "ghp_test-token",
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

	wantRepos := []string{"acme/widgets", "acme/gadgets"}
	if !reflect.DeepEqual(r.Coverage.Repos, wantRepos) {
		t.Errorf("coverage repos = %v, want %v", r.Coverage.Repos, wantRepos)
	}
	if r.Coverage.TokenScopeClass != "classic-pat" {
		t.Errorf("token scope class = %q, want classic-pat", r.Coverage.TokenScopeClass)
	}
	// Cross-repo sum: 1 in-window subject PR in widgets + 2 in gadgets.
	if got := r.Collaboration.PullRequests.Authored; got != 3 {
		t.Errorf("authored = %d, want 3 (1 + 2 across repos)", got)
	}
	if got := r.Collaboration.PullRequests.Merged; got != 2 {
		t.Errorf("merged = %d, want 2 (1 + 1 across repos)", got)
	}
}

// appServer extends the org fixture server with App token-exchange and
// installation repo enumeration endpoints.
func appServer(t *testing.T) (*httptest.Server, func() []string) {
	t.Helper()
	inner, auths := orgFixtureServer(t)
	t.Cleanup(inner.Close)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /app/installations", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"id": 42, "account": {"login": "acme"}}]`))
	})
	mux.HandleFunc("POST /app/installations/42/access_tokens", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"token": "ghs_e2e-installation-token"}`))
	})
	mux.HandleFunc("GET /installation/repositories", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"total_count": 2, "repositories": [{"full_name": "acme/widgets"}, {"full_name": "acme/gadgets"}]}`))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		inner.Config.Handler.ServeHTTP(w, r)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, auths
}

func writeTestKey(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	path := filepath.Join(t.TempDir(), "app.pem")
	raw := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	return path
}

func TestRunWithAppCredentials(t *testing.T) {
	srv, auths := appServer(t)
	out := t.TempDir()

	var stderr bytes.Buffer
	code := run([]string{
		"-app-id", "12345",
		"-app-key", writeTestKey(t),
		"-subject", "octocat",
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

	wantRepos := []string{"acme/widgets", "acme/gadgets"}
	if !reflect.DeepEqual(r.Coverage.Repos, wantRepos) {
		t.Errorf("coverage repos = %v, want %v", r.Coverage.Repos, wantRepos)
	}
	if r.Coverage.TokenScopeClass != "app-installation" {
		t.Errorf("token scope class = %q, want app-installation", r.Coverage.TokenScopeClass)
	}
	if got := r.Collaboration.PullRequests.Authored; got != 3 {
		t.Errorf("authored = %d, want 3 across repos", got)
	}

	// Activity calls must authenticate with the minted installation token.
	var sawMinted bool
	for _, a := range auths() {
		if a == "Bearer ghs_e2e-installation-token" {
			sawMinted = true
		}
	}
	if !sawMinted {
		t.Error("no API call carried the minted installation token")
	}
}

func TestRunRejectsBadScopeFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"neither repo nor org", []string{"-subject", "octocat", "-token", "t"}},
		{"app id without key", []string{"-org", "acme", "-subject", "octocat", "-app-id", "12345"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stderr bytes.Buffer
			if code := run(tt.args, func(string) string { return "" }, &stderr); code == 0 {
				t.Error("run succeeded despite invalid scope flags")
			}
		})
	}
}

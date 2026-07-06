package gitlab_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/gkanitz/coderepute/provider"
	"github.com/gkanitz/coderepute/provider/gitlab"
)

// graphQLFixtureServer serves GitLab GraphQL and REST endpoints for
// diff-shape tests.
func graphQLFixtureServer(t *testing.T) (*httptest.Server, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var paths []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.EscapedPath())
		mu.Unlock()

		switch r.URL.EscapedPath() {
		case "/graphql":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			serveFixture(t, w, "graphql_diffstats.json")
		case "/users":
			serveFixture(t, w, "users_devmara.json")
		case "/personal_access_tokens/self":
			serveFixture(t, w, "token_self.json")
		default:
			http.Error(w, `{"message":"404 Not Found"}`, http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), paths...)
	}
}

func TestFetchDiffShapeHappyPath(t *testing.T) {
	srv, _ := graphQLFixtureServer(t)
	adapter := gitlab.New("test-token", gitlab.WithBaseURL(srv.URL))

	stats, err := adapter.FetchDiffShape(context.Background(), "acme/widgets", 1)
	if err != nil {
		t.Fatalf("FetchDiffShape: %v", err)
	}

	want := []provider.FileStat{
		{Ext: "md", Additions: 10, Deletions: 2},
		{Ext: "go", Additions: 120, Deletions: 30},
		{Ext: "go", Additions: 45, Deletions: 12},
		{Ext: "ts", Additions: 30, Deletions: 8},
		{Ext: "", Additions: 3, Deletions: 0},
	}

	if len(stats) != len(want) {
		t.Fatalf("got %d file stats, want %d\n  got:  %+v\n  want: %+v",
			len(stats), len(want), stats, want)
	}
	for i := range want {
		if stats[i] != want[i] {
			t.Errorf("stat[%d] = %+v, want %+v", i, stats[i], want[i])
		}
	}
}

func TestDiffShapeQueryConstant(t *testing.T) {
	golden := filepath.Join("testdata", "graphql_diffstats_query.golden")
	raw, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read golden query: %v", err)
	}
	q := string(raw)

	for _, want := range []string{"diffStats", "path", "additions", "deletions"} {
		if !strings.Contains(q, want) {
			t.Errorf("golden query missing required field %q", want)
		}
	}

	for _, forbidden := range []string{"patch", "hunk", "rawTextBlob", "content(", "diffs("} {
		if strings.Contains(q, forbidden) {
			t.Errorf("golden query contains forbidden content-bearing field %q", forbidden)
		}
	}
}

func TestDiffShapePathContamination(t *testing.T) {
	srv, _ := graphQLFixtureServer(t)
	adapter := gitlab.New("test-token", gitlab.WithBaseURL(srv.URL))

	stats, err := adapter.FetchDiffShape(context.Background(), "acme/widgets", 1)
	if err != nil {
		t.Fatalf("FetchDiffShape: %v", err)
	}

	marshaled, _ := json.Marshal(stats)
	jsonStr := string(marshaled)

	for _, forbidden := range []string{
		"internal/secret-project",
		"secret-project",
		"service.go",
		"README.md",
		"pkg/util/parser.ts",
	} {
		if strings.Contains(jsonStr, forbidden) {
			t.Errorf("path leak: marshaled FileStat contains forbidden path %q", forbidden)
		}
	}

	for _, st := range stats {
		if strings.Contains(st.Ext, "/") || strings.Contains(st.Ext, ".") {
			t.Errorf("path leak: Ext=%q contains path separators or dots", st.Ext)
		}
	}
}

func TestFetchDiffShapeDegradation(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{name: "401 unauthorized", status: http.StatusUnauthorized, body: `{"message":"Unauthorized"}`},
		{name: "404 not found", status: http.StatusNotFound, body: `{"message":"Not Found"}`},
		{name: "GraphQL errors", status: http.StatusOK, body: `{"errors":[{"message":"Not Found"}]}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.EscapedPath() == "/graphql" {
					w.WriteHeader(tc.status)
					w.Write([]byte(tc.body))
					return
				}
				http.Error(w, "not found", http.StatusNotFound)
			}))
			t.Cleanup(srv.Close)

			adapter := gitlab.New("test-token", gitlab.WithBaseURL(srv.URL))
			_, err := adapter.FetchDiffShape(context.Background(), "acme/widgets", 1)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !errors.Is(err, provider.ErrDiffShapeUnsupported) {
				t.Errorf("error %v does not wrap ErrDiffShapeUnsupported", err)
			}
		})
	}
}

func TestFetchDiffShapeEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"data": {
				"project": {
					"mergeRequest": {
						"diffStats": []
					}
				}
			}
		}`))
	}))
	t.Cleanup(srv.Close)

	adapter := gitlab.New("test-token", gitlab.WithBaseURL(srv.URL))
	stats, err := adapter.FetchDiffShape(context.Background(), "acme/widgets", 999)
	if err != nil {
		t.Fatalf("FetchDiffShape: %v", err)
	}
	if len(stats) != 0 {
		t.Errorf("expected 0 stats, got %d", len(stats))
	}
}

func TestFixturesExist(t *testing.T) {
	for _, name := range []string{
		"graphql_diffstats.json",
		"graphql_diffstats_query.golden",
	} {
		if _, err := os.Stat(filepath.Join("testdata", name)); err != nil {
			t.Errorf("missing fixture %s: %v", name, err)
		}
	}
}

package gitlab_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gkanitz/coderepute/provider/gitlab"
)

func TestListGroupProjects(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != "/groups/acme/projects" {
			http.Error(w, `{"message":"404 Not Found"}`, http.StatusNotFound)
			return
		}
		if got := r.URL.Query().Get("include_subgroups"); got != "true" {
			t.Errorf("include_subgroups = %q, want true (group coverage must include subgroup projects)", got)
		}
		switch r.URL.Query().Get("page") {
		case "", "1":
			w.Header().Set("Link", `<`+srv.URL+`/groups/acme/projects?include_subgroups=true&per_page=100&page=2>; rel="next"`)
			serveFixture(t, w, "group_projects_page1.json")
		case "2":
			serveFixture(t, w, "group_projects_page2.json")
		default:
			http.Error(w, "no such page", http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	adapter := gitlab.New("test-token", gitlab.WithBaseURL(srv.URL))
	projects, err := adapter.ListGroupProjects(context.Background(), "acme")
	if err != nil {
		t.Fatalf("ListGroupProjects: %v", err)
	}

	want := []string{"acme/widgets", "acme/gadgets", "acme/platform/auth-service"}
	if len(projects) != len(want) {
		t.Fatalf("got %d projects %v, want %v", len(projects), projects, want)
	}
	for i := range want {
		if projects[i] != want[i] {
			t.Errorf("projects[%d] = %q, want %q", i, projects[i], want[i])
		}
	}
}

func TestListGroupProjectsEscapesNestedGroupPath(t *testing.T) {
	var requested string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = r.URL.EscapedPath()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]"))
	}))
	t.Cleanup(srv.Close)

	adapter := gitlab.New("test-token", gitlab.WithBaseURL(srv.URL))
	if _, err := adapter.ListGroupProjects(context.Background(), "acme/platform"); err != nil {
		t.Fatalf("ListGroupProjects: %v", err)
	}
	if requested != "/groups/acme%2Fplatform/projects" {
		t.Errorf("requested path %q, want URL-encoded nested group path", requested)
	}
}

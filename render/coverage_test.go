package render_test

import (
	"strings"
	"testing"

	"github.com/grkanitz/coderepute/render"
)

func TestHTMLShowsCoverageBreadth(t *testing.T) {
	r := reportFixture()
	r.Coverage.Repos = []string{"acme/widgets", "acme/gadgets"}
	r.Coverage.TokenScopeClass = "app-installation"

	out, err := render.HTML(r)
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	html := string(out)

	if !strings.Contains(html, "app-installation") {
		t.Error("rendered HTML missing the token scope class")
	}
	if !strings.Contains(html, "2 repositories") {
		t.Error("rendered HTML missing the covered-repo count")
	}
	if !strings.Contains(html, "org acme") {
		t.Error("rendered HTML missing the covering org name")
	}
	// Individual repo names are a privacy leak risk (unannounced products,
	// codenames) and must never appear in the coverage stamp — only the
	// org name and a count.
	for _, repo := range r.Coverage.Repos {
		if strings.Contains(html, repo) {
			t.Errorf("rendered HTML leaks individual covered repo name %q", repo)
		}
	}
}

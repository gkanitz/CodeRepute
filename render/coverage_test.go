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
	for _, repo := range r.Coverage.Repos {
		if !strings.Contains(html, repo) {
			t.Errorf("rendered HTML missing covered repo %q", repo)
		}
	}
}

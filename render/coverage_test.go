package render_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/grkanitz/coderepute/render"
)

// stripScriptJSON removes the embedded <script type="application/json"> block
// from an HTML string, returning only the visible rendered content. This lets
// privacy tests check that repo names and other machine-readable data do not
// appear in the human-visible portion of the page.
var scriptJSONRe = regexp.MustCompile(`(?s)<script type="application/json"[^>]*>.*?</script>`)

func stripScriptJSON(html string) string {
	return scriptJSONRe.ReplaceAllString(html, "")
}

// TestHTMLShowsAllTimeWhenSinceIsNil verifies that a nil Window.Since
// renders as "all time" in the coverage stamp.
func TestHTMLShowsAllTimeWhenSinceIsNil(t *testing.T) {
	r := reportFixture()
	r.Coverage.Window.Since = nil // no lower bound

	out, err := render.HTML(r)
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	html := string(out)

	if !strings.Contains(html, "all time") {
		t.Error("rendered HTML missing 'all time' for nil Since window")
	}
}

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
	// codenames) and must never appear in the visible rendered HTML — only the
	// org name and a count. The embedded machine-readable JSON (inside the
	// <script type="application/json"> tag) is excluded from this check because
	// it is an intentional machine-readable data carrier, not visible content.
	visibleHTML := stripScriptJSON(html)
	for _, repo := range r.Coverage.Repos {
		if strings.Contains(visibleHTML, repo) {
			t.Errorf("rendered HTML leaks individual covered repo name %q", repo)
		}
	}
}

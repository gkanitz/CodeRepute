package render_test

import (
	"bytes"
	"flag"
	"os"
	"strings"
	"testing"

	"github.com/gkanitz/coderepute/render"
	"github.com/gkanitz/coderepute/report"
)

// -update rewrites the golden file and the committed docs sample from the
// current renderer output:
//
//	go test ./render -run Golden -update
var update = flag.Bool("update", false, "rewrite golden files and the docs sample")

const (
	fixturePath = "testdata/sample-report.json"
	goldenPath  = "testdata/sample-report.golden.html"
	docsSample  = "../docs/examples/sample-report.html"
)

// renderSample renders the rich sample fixture and returns the raw fixture
// bytes alongside the rendered HTML.
func renderSample(t *testing.T) (raw, html []byte) {
	t.Helper()
	raw, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	r, err := report.Parse(raw)
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	html, err = render.HTML(r)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	return raw, html
}

// TestGoldenFile renders the sample fixture and either updates the committed
// golden file (with -update) or diffs against it. The golden file is the
// canonical reference for how the report looks; failures here mean the
// renderer changed in a way that wasn't explicitly approved.
func TestGoldenFile(t *testing.T) {
	_, html := renderSample(t)

	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if err := os.WriteFile(goldenPath, html, 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("updated %s", goldenPath)

		// Also update the docs sample so the committed example stays fresh.
		if err := os.MkdirAll("../docs/examples", 0o755); err != nil {
			t.Fatalf("mkdir docs/examples: %v", err)
		}
		if err := os.WriteFile(docsSample, html, 0o644); err != nil {
			t.Fatalf("write docs sample: %v", err)
		}
		t.Logf("updated %s", docsSample)
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden (run with -update to create): %v", err)
	}
	if !bytes.Equal(html, want) {
		// Show a concise diff: lines that changed.
		gotLines := strings.Split(string(html), "\n")
		wantLines := strings.Split(string(want), "\n")
		for i := 0; i < len(gotLines) && i < len(wantLines); i++ {
			if gotLines[i] != wantLines[i] {
				t.Errorf("line %d differs:\n  got:  %q\n  want: %q", i+1, gotLines[i], wantLines[i])
				if t.Failed() && i > 5 {
					// cap output
					break
				}
			}
		}
		if len(gotLines) != len(wantLines) {
			t.Errorf("line count differs: got %d, want %d", len(gotLines), len(wantLines))
		}
		t.Logf("re-run with -update to accept the new output")
	}
}

// TestRenderedReportNeverLeaksSeededColleagueData treats rendering as a
// privacy boundary: the fixture deliberately carries colleague usernames,
// PR titles, and branch names in unknown JSON fields (as a hostile or buggy
// upstream might), and none of them may ever surface in the rendered HTML.
func TestRenderedReportNeverLeaksSeededColleagueData(t *testing.T) {
	raw, html := renderSample(t)

	prohibited := []string{
		"mallory-reviewer", // colleague username
		"trent-teammate",   // colleague username
		"rocket telemetry", // PR title fragment
		"Megacorp",         // customer name from a PR title
		"feature/rocket",   // branch name
	}
	lower := strings.ToLower(string(html))
	for _, p := range prohibited {
		// The test is only honest if the fixture really carries the bait.
		if !bytes.Contains(raw, []byte(p)) {
			t.Fatalf("fixture no longer seeds prohibited string %q; re-seed it", p)
		}
		if strings.Contains(lower, strings.ToLower(p)) {
			t.Errorf("rendered HTML leaks prohibited data %q", p)
		}
	}
}

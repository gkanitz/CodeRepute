package render_test

import (
	"os"
	"strings"
	"testing"

	"github.com/gkanitz/coderepute/render"
	"github.com/gkanitz/coderepute/report"
)

func TestTransparencyManifestSectionRenders(t *testing.T) {
	r := reportFixture()
	r.AccessManifest = &report.AccessManifest{
		Endpoints: []report.EndpointCount{
			{Class: "rest:users_show", Count: 1},
			{Class: "rest:list_pulls", Count: 2},
			{Class: "rest:list_reviews", Count: 3},
			{Class: "graphql:pr_diff_shape", Count: 1},
		},
		NeverRequested: []string{
			"file contents (any endpoint)",
			"diffs / patch text (any endpoint)",
			"branch names",
			"colleague profiles",
			"commit contents or messages",
		},
		Notes: "All requests are to the API. No repository contents, file contents, diffs, branch names, colleague profiles, or commit data are ever requested.",
	}

	out, err := render.HTML(r)
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	html := string(out)

	// Section header
	if !strings.Contains(html, "What this tool read") {
		t.Error("rendered HTML missing 'What this tool read' section header")
	}

	// Endpoint labels and counts
	for _, want := range []string{
		"User profile lookup",
		"Pull request listing",
		"Review listing",
		"Diff shape (file paths reduced to extensions)",
		"1", // count for users_show
		"2", // count for list_pulls
		"3", // count for list_reviews
	} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered HTML missing %q", want)
		}
	}

	// Never-requested section
	if !strings.Contains(html, "Data never requested") {
		t.Error("rendered HTML missing 'Data never requested' section")
	}
	for _, want := range []string{
		"file contents (any endpoint)",
		"diffs / patch text (any endpoint)",
		"branch names",
		"colleague profiles",
		"commit contents or messages",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered HTML missing never-requested item %q", want)
		}
	}

	// Notes section
	if !strings.Contains(html, r.AccessManifest.Notes) {
		t.Error("rendered HTML missing manifest notes")
	}
}

func TestTransparencySectionOmitsWhenNil(t *testing.T) {
	r := reportFixture()
	r.AccessManifest = nil

	out, err := render.HTML(r)
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	html := string(out)

	if strings.Contains(html, "What this tool read") {
		t.Error("rendered HTML shows transparency section despite nil manifest")
	}
}

func TestTransparencyProhibitedStrings(t *testing.T) {
	// Verify that the "What this tool read" section does not leak
	// any prohibited strings (class strings with identifiers, etc.).
	r := reportFixture()
	r.AccessManifest = &report.AccessManifest{
		Endpoints: []report.EndpointCount{
			{Class: "rest:users_show", Count: 1},
			{Class: "rest:list_pulls", Count: 1},
		},
		NeverRequested: []string{"file contents"},
		Notes:          "test",
	}

	out, err := render.HTML(r)
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	html := string(out)

	// The rendered section should not contain any class strings
	// that look like identifiers (usernames, repo names, etc.).
	// Find the transparency section boundaries and check only there.
	for _, prohibited := range []string{
		"/users/octocat",
		"/repos/acme/widgets",
	} {
		if strings.Contains(html, prohibited) {
			t.Errorf("rendered HTML leaks prohibited string %q", prohibited)
		}
	}
}

func TestTransparencySectionGoldenFileCompatible(t *testing.T) {
	// Verify that the golden file fixture (which includes an access_manifest)
	// renders without error and produces the expected section.
	raw, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Skipf("skip: fixture not found: %v", err)
	}
	r, err := report.Parse(raw)
	if err != nil {
		t.Skipf("skip: fixture parse: %v", err)
	}
	if r.AccessManifest == nil {
		t.Skip("skip: fixture has no access_manifest block")
	}
	html, err := render.HTML(r)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	htmlStr := string(html)
	if !strings.Contains(htmlStr, "What this tool read") {
		t.Error("rendered fixture missing 'What this tool read' section")
	}
	if !strings.Contains(htmlStr, "Data never requested") {
		t.Error("rendered fixture missing 'Data never requested' section")
	}
	// Verify the never-requested list is rendered verbatim
	for _, item := range r.AccessManifest.NeverRequested {
		if !strings.Contains(htmlStr, item) {
			t.Errorf("rendered fixture missing never-requested item %q", item)
		}
	}

	// Verify prohibited strings from the fixture are not leaked
	for _, p := range []string{"mallory-reviewer", "trent-teammate", "rocket telemetry", "feature/rocket"} {
		if strings.Contains(strings.ToLower(htmlStr), strings.ToLower(p)) {
			t.Errorf("rendered fixture leaks prohibited string %q", p)
		}
	}
}

func TestManifestNeverRequestedAppearsInSparseFixture(t *testing.T) {
	// The sparse fixture (from reportFixture above) doesn't have an
	// access_manifest. We simulate a local unverified run with manifest.
	r := reportFixture()
	r.AccessManifest = &report.AccessManifest{
		Endpoints: []report.EndpointCount{
			{Class: "rest:users_show", Count: 1},
		},
		NeverRequested: []string{
			"file contents (any endpoint)",
			"diffs / patch text (any endpoint)",
			"branch names",
			"colleague profiles",
			"commit contents or messages",
		},
		Notes: "Test run manifest.",
	}

	out, err := render.HTML(r)
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	html := string(out)

	if !strings.Contains(html, "What this tool read") {
		t.Error("local unverified report missing 'What this tool read' section")
	}
	if !strings.Contains(html, "Data never requested") {
		t.Error("local unverified report missing 'Data never requested' section")
	}
}

func TestOmissionsSectionRendersAllEntries(t *testing.T) {
	r := reportFixture()
	r.AccessManifest = &report.AccessManifest{
		Endpoints: []report.EndpointCount{
			{Class: "rest:users_show", Count: 1},
		},
		NeverRequested: []string{"file contents"},
		Notes:          "test",
		Omissions: []report.OmissionEntry{
			{
				Category:            "composite score",
				Description:         "No single score, grade, or composite number is derived from any metric. Each metric is reported independently.",
				PrivacyRationaleRef: "docs/privacy-rationale.md#no-composite-score",
			},
			{
				Category:            "team ranking",
				Description:         "This report does not rank or compare the subject against any team, cohort, or population.",
				PrivacyRationaleRef: "docs/privacy-rationale.md#no-team-ranking",
			},
			{
				Category:            "named-colleague comparison",
				Description:         "No named individual other than the report subject appears in any output, including review interactions.",
				PrivacyRationaleRef: "docs/privacy-rationale.md#no-named-colleague-comparison",
			},
		},
	}

	out, err := render.HTML(r)
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	html := string(out)

	// Section heading
	if !strings.Contains(html, "What this report does not include and why") {
		t.Error("rendered HTML missing omissions section heading")
	}

	// Each entry: bold category label, description, and Privacy rationale link
	for _, want := range []string{
		"Composite score:</strong>",
		"No single score, grade, or composite number is derived from any metric.",
		`<a href="docs/privacy-rationale.md#no-composite-score">Privacy rationale</a>`,
		"Team ranking:</strong>",
		"This report does not rank or compare the subject against any team, cohort, or population.",
		`<a href="docs/privacy-rationale.md#no-team-ranking">Privacy rationale</a>`,
		"Named-colleague comparison:</strong>",
		"No named individual other than the report subject appears in any output, including review interactions.",
		`<a href="docs/privacy-rationale.md#no-named-colleague-comparison">Privacy rationale</a>`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered HTML missing omission entry content: %q", want)
		}
	}

	// "None declared" must NOT appear when omissions are present
	if strings.Contains(html, "None declared.") {
		t.Error("rendered HTML shows 'None declared' despite having omission entries")
	}
}

func TestOmissionsEmptyShowsNoneDeclared(t *testing.T) {
	r := reportFixture()
	r.AccessManifest = &report.AccessManifest{
		Endpoints: []report.EndpointCount{
			{Class: "rest:users_show", Count: 1},
		},
		NeverRequested: []string{"file contents"},
		Notes:          "test",
		Omissions:      []report.OmissionEntry{},
	}

	out, err := render.HTML(r)
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	html := string(out)

	if !strings.Contains(html, "What this report does not include and why") {
		t.Error("rendered HTML missing omissions section heading even when omissions are empty")
	}
	if !strings.Contains(html, "None declared.") {
		t.Error("rendered HTML missing 'None declared' when omissions are empty")
	}
}

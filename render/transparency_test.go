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

// TestNoScoreDeclarationRendersInManifest verifies that the no-score
// declaration line appears in the rendered HTML when the AccessManifest
// carries it. (Issue #103)
func TestNoScoreDeclarationRendersInManifest(t *testing.T) {
	r := reportFixture()
	r.AccessManifest = &report.AccessManifest{
		Endpoints: []report.EndpointCount{
			{Class: "rest:users_show", Count: 1},
		},
		NeverRequested:     []string{"file contents"},
		Notes:              "test",
		NoScoreDeclaration: report.NoScoreDeclarationText,
	}

	out, err := render.HTML(r)
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	html := string(out)

	// Must appear in the "What this report does not include and why" section
	if !strings.Contains(html, "No score or ranking:") {
		t.Error("rendered HTML missing 'No score or ranking:' heading")
	}
	if !strings.Contains(html, "no composite score, no ranking, no grade") {
		t.Error("rendered HTML missing no-score declaration text")
	}
	if !strings.Contains(html, "This report contains no composite score") {
		t.Error("rendered HTML missing full declaration line")
	}

	// The declaration must NOT appear in the report body; it must be in
	// the transparency annex section.
	manifestSection := strings.Index(html, "What this report does not include and why")
	if manifestSection < 0 {
		t.Fatal("manifest section not found")
	}
	declarationPos := strings.Index(html, "No score or ranking:")
	if declarationPos < 0 {
		t.Fatal("declaration not found")
	}
	if declarationPos < manifestSection {
		t.Error("declaration appears before the manifest section")
	}
}

// TestNoScoreDeclarationOmitsWhenEmpty verifies that the no-score
// declaration section is omitted when NoScoreDeclaration is empty.
func TestProvenanceSectionRenders(t *testing.T) {
	r := reportFixture()
	r.Verification = &report.Verification{
		Status:      report.StatusVerified,
		Provider:    "github-actions",
		Repository:  "gkanitz/CodeRepute",
		WorkflowRef: "gkanitz/CodeRepute/.github/workflows/coderepute-report.yml@refs/heads/main",
		RunID:       "12345678",
		RunURL:      "https://github.com/gkanitz/CodeRepute/actions/runs/12345678",
		Attestation: &report.Attestation{
			Type:          report.AttestationTypeSigstore,
			URL:           "https://github.com/gkanitz/CodeRepute/attestations",
			VerifyCommand: "gh attestation verify report.json --repo gkanitz/CodeRepute",
		},
	}
	r.AccessManifest = &report.AccessManifest{
		Endpoints: []report.EndpointCount{
			{Class: "rest:users_show", Count: 1},
		},
		NeverRequested: []string{"file contents"},
		Notes:          "test",
	}

	out, err := render.HTML(r)
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	html := string(out)

	// Section heading (AC1)
	if !strings.Contains(html, "Provenance") {
		t.Error("rendered HTML missing 'Provenance' section heading")
	}

	// 1. Workflow identity (AC1 item 1)
	if !strings.Contains(html, "gkanitz/CodeRepute/.github/workflows/coderepute-report.yml@refs/heads/main") {
		t.Error("rendered HTML missing workflow_ref")
	}

	// 2. Repository (AC1 item 2)
	if !strings.Contains(html, "gkanitz/CodeRepute") {
		t.Error("rendered HTML missing repository")
	}

	// 3. Actions run link (AC1 item 3)
	if !strings.Contains(html, "https://github.com/gkanitz/CodeRepute/actions/runs/12345678") {
		t.Error("rendered HTML missing run URL")
	}
	if !strings.Contains(html, `href="https://github.com/gkanitz/CodeRepute/actions/runs/12345678"`) {
		t.Error("run URL is not a clickable link")
	}

	// 4. Sigstore / Rekor prose (AC1 item 4)
	if !strings.Contains(html, "keyless Sigstore") {
		t.Error("rendered HTML missing 'keyless Sigstore' reference")
	}
	if !strings.Contains(html, "public Sigstore Rekor transparency log") {
		t.Error("rendered HTML missing 'public Sigstore Rekor transparency log' reference")
	}

	// 5. Verify command (AC1 item 5)
	if !strings.Contains(html, "gh attestation verify") {
		t.Error("rendered HTML missing verify command")
	}
	if !strings.Contains(html, "--signer-workflow gkanitz/CodeRepute/.github/workflows/coderepute-report.yml") {
		t.Error("rendered HTML missing --signer-workflow flag in verify command")
	}
	if !strings.Contains(html, "--repo gkanitz/CodeRepute") {
		t.Error("rendered HTML missing --repo flag in verify command")
	}
	if !strings.Contains(html, "&lt;file&gt;") {
		t.Error("rendered HTML missing &lt;file&gt; placeholder in verify command")
	}

	// The provenance section must appear inside the transparency manifest section
	manifestSection := strings.Index(html, "What this tool read")
	provenancePos := strings.Index(html, "Provenance")
	if manifestSection < 0 {
		t.Fatal("transparency manifest section not found")
	}
	if provenancePos < 0 {
		t.Fatal("provenance heading not found")
	}
	if provenancePos < manifestSection {
		t.Error("provenance appears before the transparency manifest section")
	}
}

func TestProvenanceSectionNoRekorLeak(t *testing.T) {
	r := reportFixture()
	r.Verification = &report.Verification{
		Status:      report.StatusVerified,
		Provider:    "github-actions",
		Repository:  "gkanitz/CodeRepute",
		WorkflowRef: "gkanitz/CodeRepute/.github/workflows/coderepute-report.yml@refs/heads/main",
		RunURL:      "https://github.com/gkanitz/CodeRepute/actions/runs/12345678",
	}
	r.AccessManifest = &report.AccessManifest{
		Endpoints: []report.EndpointCount{
			{Class: "rest:users_show", Count: 1},
		},
		NeverRequested: []string{"file contents"},
		Notes:          "test",
	}

	out, err := render.HTML(r)
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	html := string(out)

	// AC2: No Rekor log entry URL or log index number anywhere in the rendered output.
	// The word "Rekor" is expected in the prose statement; only URLs, API paths,
	// and log index references are prohibited.
	for _, prohibited := range []string{
		"rekor.sigstore.dev",
		"api/v1/log/entries",
		"log index",
		"log_index",
		"Rekor log entry",
	} {
		if strings.Contains(strings.ToLower(html), strings.ToLower(prohibited)) {
			t.Errorf("rendered HTML leaks prohibited Rekor reference %q", prohibited)
		}
	}
}

func TestProvenanceSectionPlaceholders(t *testing.T) {
	r := reportFixture()
	r.Verification = &report.Verification{
		Status:      report.StatusUnverified,
		Provider:    "github-actions",
		Repository:  "",
		WorkflowRef: "",
		RunURL:      "",
	}
	r.AccessManifest = &report.AccessManifest{
		Endpoints: []report.EndpointCount{
			{Class: "rest:users_show", Count: 1},
		},
		NeverRequested: []string{"file contents"},
		Notes:          "test",
	}

	out, err := render.HTML(r)
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	html := string(out)

	// AC3: When all provenance fields are absent, the section is not rendered at all
	// (the outer if condition fails since WorkflowRef, Repository, and RunURL are all empty)
	if strings.Contains(html, "Provenance") {
		t.Error("Provenance section rendered despite empty provenance fields")
	}
}

func TestProvenanceSectionPartialPlaceholders(t *testing.T) {
	r := reportFixture()
	r.Verification = &report.Verification{
		Status:      report.StatusVerified,
		Provider:    "github-actions",
		Repository:  "gkanitz/CodeRepute",
		WorkflowRef: "",
		RunURL:      "",
	}
	r.AccessManifest = &report.AccessManifest{
		Endpoints: []report.EndpointCount{
			{Class: "rest:users_show", Count: 1},
		},
		NeverRequested: []string{"file contents"},
		Notes:          "test",
	}

	out, err := render.HTML(r)
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	html := string(out)

	// Section is still rendered when Repository is present (AC3)
	if !strings.Contains(html, "Provenance") {
		t.Error("Provenance section not rendered when Repository is present")
	}

	// Missing fields show placeholders
	if !strings.Contains(html, "[not available]") {
		t.Error("missing provenance fields should render '[not available]'")
	}

	// The verify command uses Repository since WorkflowRef is absent
	if !strings.Contains(html, "--signer-workflow [not available]") {
		t.Error("missing workflow_ref should show placeholder in verify command")
	}
}

func TestProvenanceSectionOmitsWhenNil(t *testing.T) {
	r := reportFixture()
	r.Verification = nil
	r.AccessManifest = &report.AccessManifest{
		Endpoints: []report.EndpointCount{
			{Class: "rest:users_show", Count: 1},
		},
		NeverRequested: []string{"file contents"},
		Notes:          "test",
	}

	out, err := render.HTML(r)
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	html := string(out)

	// AC4: Provenance section omitted when no verification block
	if strings.Contains(html, "Provenance") {
		t.Error("Provenance section rendered despite nil Verification block")
	}

	// The rest of the manifest renders normally (AC4)
	if !strings.Contains(html, "What this tool read") {
		t.Error("transparency manifest missing despite nil Verification")
	}
	if !strings.Contains(html, "Data never requested") {
		t.Error("'Data never requested' section missing despite nil Verification")
	}
}

func TestNoScoreDeclarationOmitsWhenEmpty(t *testing.T) {
	r := reportFixture()
	r.AccessManifest = &report.AccessManifest{
		Endpoints: []report.EndpointCount{
			{Class: "rest:users_show", Count: 1},
		},
		NeverRequested:     []string{"file contents"},
		Notes:              "test",
		NoScoreDeclaration: "",
	}

	out, err := render.HTML(r)
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	html := string(out)

	if strings.Contains(html, "No score or ranking:") {
		t.Error("rendered HTML shows 'No score or ranking:' section when NoScoreDeclaration is empty")
	}
}

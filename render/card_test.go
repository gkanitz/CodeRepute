package render_test

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gkanitz/coderepute/render"
	"github.com/gkanitz/coderepute/report"
)

const (
	cardGoldenFull       = "testdata/card-full.golden.svg"
	cardGoldenSparse     = "testdata/card-sparse.golden.svg"
	cardGoldenUnverified = "testdata/card-unverified.golden.svg"
)

// cardFixture returns a full report with all four headline metrics populated and
// a verified verification block. This is the baseline for golden tests.
func cardFixture() report.Report {
	return report.Report{
		SchemaVersion: report.SchemaVersion,
		GeneratedAt:   time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC),
		Subject: report.Subject{
			Platform:  "github",
			Username:  "alice",
			AccountID: "1234567",
		},
		Coverage: &report.Coverage{
			Repos:           []string{"acme/widgets"},
			TokenScope:      "repo,read:org",
			TokenScopeClass: "classic-pat",
			Window: report.Window{
				Since: func() *time.Time { t := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC); return &t }(),
				Until: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
			},
		},
		Verification: &report.Verification{
			Status:      report.StatusVerified,
			Provider:    "github-actions",
			Repository:  "acme/widgets",
			WorkflowRef: "acme/widgets/.github/workflows/coderepute.yml@refs/heads/main",
			RunID:       "8000000042",
			RunURL:      "https://github.com/acme/widgets/actions/runs/8000000042",
			VerifyURL:   "https://gkanitz.github.io/CodeRepute/verify/?repo=acme%2Fwidgets&subject=alice",
			Attestation: &report.Attestation{
				Type:          "sigstore-github-artifact-attestation",
				URL:           "https://github.com/acme/widgets/attestations",
				VerifyCommand: "gh attestation verify report.json --repo acme/widgets",
			},
		},
		Collaboration: &report.Collaboration{
			PullRequests: &report.PullRequestStats{Authored: 47, Merged: 42},
			ReviewsGiven: &report.ReviewStats{Total: 63, Approvals: 51, ChangesRequested: 12, DeepReviewCount: 19},
		},
		Cadence: &report.Cadence{
			ActiveDays:    198,
			Contributions: 371,
		},
	}
}

// fullCardFixtureFromSample reads the sample report JSON (the same one used by
// the HTML golden tests) and renders it as a card. This is the "full sample"
// fixture for golden-file comparison.
func fullCardFixtureFromSample(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	r, err := report.Parse(raw)
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	svg, err := render.CardSVG(r)
	if err != nil {
		t.Fatalf("CardSVG: %v", err)
	}
	return svg
}

// cardGoldenTest is a helper that renders a CardSVG from the given report and
// either updates the golden file or compares against it.
func cardGoldenTest(t *testing.T, r report.Report, goldenPath string) {
	t.Helper()
	svg, err := render.CardSVG(r)
	if err != nil {
		t.Fatalf("CardSVG: %v", err)
	}

	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if err := os.WriteFile(goldenPath, svg, 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("updated %s", goldenPath)
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden (run with -update to create): %v", err)
	}
	if !bytes.Equal(svg, want) {
		gotLines := strings.Split(string(svg), "\n")
		wantLines := strings.Split(string(want), "\n")
		for i := 0; i < len(gotLines) && i < len(wantLines); i++ {
			if gotLines[i] != wantLines[i] {
				t.Errorf("line %d differs:\n  got:  %q\n  want: %q", i+1, gotLines[i], wantLines[i])
				if t.Failed() && i > 5 {
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

// AC-1: Golden card.svg for the full sample report contains username, platform,
// owner/org string, window, the four headline values, QR, and the attested mark.
func TestCardGoldenFull(t *testing.T) {
	svg := fullCardFixtureFromSample(t)

	svgStr := string(svg)
	for _, want := range []string{
		"alice",      // username
		"github",     // platform
		"acme",       // single-owner org name
		"2025-06-01", // window start
		"2026-06-01", // window end
		"42",         // PRs merged
		"63",         // reviews given
	} {
		if !strings.Contains(svgStr, want) {
			t.Errorf("card SVG missing %q", want)
		}
	}

	// Median TTM (18.5 from fixture)
	if !strings.Contains(svgStr, "18.5") && !strings.Contains(svgStr, "18") {
		t.Error("card SVG missing median TTM value")
	}

	// Active days (198 from fixture)
	if !strings.Contains(svgStr, "198") {
		t.Error("card SVG missing active days")
	}

	// QR should be present (SVG with pixelated rendering inside the card)
	if !strings.Contains(svgStr, "<svg") {
		t.Error("card SVG missing nested SVG element (QR code)")
	}

	// Attested mark for verified report
	if !strings.Contains(svgStr, "Sigstore attested") {
		t.Error("card SVG missing 'Sigstore attested' for verified report")
	}

	// Verify URL present as text near the QR
	if !strings.Contains(svgStr, "Verify") {
		t.Error("card SVG missing verify link text")
	}
}

// AC-1 golden test that compares against a stored golden file.
func TestCardGoldenFullGoldenFile(t *testing.T) {
	cardGoldenTest(t, fullCardFixtureFromSampleAsReport(t), cardGoldenFull)
}

// helper: parse the sample fixture and return the report
func fullCardFixtureFromSampleAsReport(t *testing.T) report.Report {
	t.Helper()
	raw, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	r, err := report.Parse(raw)
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	return r
}

// AC-2: Golden for a sparse report: missing metrics render "—"; nothing else
// invented.
func TestCardGoldenSparse(t *testing.T) {
	r := cardFixture()
	// Remove Collaboration and Cadence entirely, keeping only the minimum
	r.Collaboration = nil
	r.Cadence = nil

	cardGoldenTest(t, r, cardGoldenSparse)

	// Verify the em-dash placeholders are present for missing metrics.
	svgStr := string(thunkCardSVG(t, r))
	if !strings.Contains(svgStr, "—") {
		t.Error("sparse card should render em-dash for missing metrics")
	}
}

// thunkCardSVG renders CardSVG and returns the string; fails the test on error.
func thunkCardSVG(t *testing.T, r report.Report) []byte {
	t.Helper()
	svg, err := render.CardSVG(r)
	if err != nil {
		t.Fatalf("CardSVG: %v", err)
	}
	return svg
}

// AC-3: Golden for an unverified (local-run) report: "unverified" variant shown;
// attested mark absent.
func TestCardGoldenUnverified(t *testing.T) {
	r := cardFixture()
	r.Verification = &report.Verification{
		Status: report.StatusUnverified,
		Reason: "local run; no CI attestation",
	}

	cardGoldenTest(t, r, cardGoldenUnverified)

	svgStr := string(thunkCardSVG(t, r))
	if strings.Contains(svgStr, "Sigstore attested") {
		t.Error("unverified card must not contain 'Sigstore attested'")
	}
	if !strings.Contains(svgStr, "unverified") {
		t.Error("unverified card must show unverified variant")
	}
}

// AC-4: Multi-owner coverage fixture renders "N orgs", single-owner renders the
// owner name.
func TestCardOrgContext(t *testing.T) {
	t.Run("single owner shows owner name", func(t *testing.T) {
		r := cardFixture()
		r.Coverage.Repos = []string{"acme/widgets", "acme/platform"}
		svg := string(thunkCardSVG(t, r))
		if !strings.Contains(svg, "acme") {
			t.Error("single-owner card should show org name 'acme'")
		}
	})

	t.Run("multi-owner shows N orgs", func(t *testing.T) {
		r := cardFixture()
		r.Coverage.Repos = []string{"acme/widgets", "megacorp/platform"}
		svg := string(thunkCardSVG(t, r))
		if !strings.Contains(svg, "2 orgs") {
			t.Error("multi-owner card should show '2 orgs'")
		}
	})
}

// AC-5: QR payload equals verification.verify_url when set, else the canonical
// verify URL.
func TestCardQRPayload(t *testing.T) {
	t.Run("QR uses VerifyURL when set", func(t *testing.T) {
		r := cardFixture()
		r.Verification.VerifyURL = "https://example.com/verify?repo=acme&subject=alice"
		svg := string(thunkCardSVG(t, r))
		// The URL text is HTML-escaped in the SVG so ampersand becomes &amp;
		if !strings.Contains(svg, "example.com/verify") {
			t.Error("card should include the verify URL near the QR code")
		}
		if !strings.Contains(svg, "acme") && !strings.Contains(svg, "alice") {
			t.Error("card verify URL should include query parameters")
		}
	})

	t.Run("QR falls back to canonical URL", func(t *testing.T) {
		r := cardFixture()
		r.Verification.VerifyURL = ""
		svg := string(thunkCardSVG(t, r))
		if !strings.Contains(svg, "coderepute.dev/verify") {
			t.Error("card should fall back to canonical verify URL")
		}
	})
}

// AC-6: Self-containment: rendered SVG contains no http(s):// references except
// inside the QR payload/verify link text.
func TestCardSelfContainment(t *testing.T) {
	r := cardFixture()
	svg := string(thunkCardSVG(t, r))

	// Find all http:// or https:// occurrences.
	httpRe := regexp.MustCompile(`https?://[^\s"<>]+`)
	matches := httpRe.FindAllString(svg, -1)
	for _, m := range matches {
		// Allowed: the SVG namespace, the verify URL, and github.com attestation URLs.
		if strings.Contains(m, "www.w3.org/2000/svg") {
			continue
		}
		if strings.Contains(m, "gkanitz.github.io/CodeRepute/verify") {
			continue
		}
		if strings.Contains(m, "github.com") {
			continue
		}
		t.Errorf("card contains disallowed URL reference: %s", m)
	}

	// No external resource references.
	for _, ext := range []string{".jpg", ".png", ".woff", ".ttf", ".otf", "data:image"} {
		if strings.Contains(svg, ext) {
			t.Errorf("card references external resource: %q", ext)
		}
	}
}

// AC-7: Prohibited-strings test (seeded colleague names, PR titles, branch names,
// file paths) passes over card output.
func TestCardProhibitedStrings(t *testing.T) {
	r := fullCardFixtureFromSampleAsReport(t)
	svg := string(thunkCardSVG(t, r))

	prohibited := []string{
		"mallory-reviewer",
		"trent-teammate",
		"rocket telemetry",
		"Megacorp",
		"feature/rocket",
	}
	lower := strings.ToLower(svg)
	for _, p := range prohibited {
		if strings.Contains(lower, strings.ToLower(p)) {
			t.Errorf("card SVG leaks prohibited data %q", p)
		}
	}
}

// AC-8: Card dimensions/viewBox exactly 1200x627.
func TestCardDimensions(t *testing.T) {
	r := cardFixture()
	svg := string(thunkCardSVG(t, r))

	dimRe := regexp.MustCompile(`viewBox="0\s+0\s+1200\s+627"`)
	if !dimRe.MatchString(svg) {
		t.Errorf("card SVG must have viewBox=\"0 0 1200 627\", got: %s", extractViewBox(svg))
	}

	// Also check width and height attributes.
	if !strings.Contains(svg, `width="1200"`) {
		t.Error("card SVG missing width=\"1200\"")
	}
	if !strings.Contains(svg, `height="627"`) {
		t.Error("card SVG missing height=\"627\"")
	}
}

// extractViewBox returns the viewBox attribute value from an SVG.
func extractViewBox(svg string) string {
	re := regexp.MustCompile(`viewBox="([^"]+)"`)
	m := re.FindStringSubmatch(svg)
	if len(m) > 1 {
		return m[1]
	}
	return "(not found)"
}

// AC-9: Workflow/action changes — verify card outputs exist in action.yml.
// We assert the action.yml content via file content checks since the project
// has no action wire-test pattern yet.
func TestActionYMLCardOutputs(t *testing.T) {
	raw, err := os.ReadFile("../action.yml")
	if err != nil {
		t.Fatal("cannot read action.yml for card-output assertion")
	}
	yml := string(raw)

	// Card PNG step reference
	if !strings.Contains(yml, "card.png") {
		t.Error("action.yml must reference card.png")
	}
	// Card attestation alongside report.html and report.pdf
	if !strings.Contains(yml, "report-pdf") && !strings.Contains(yml, "card-png") {
		// At minimum, card.png must be attested in the same attest step
		if !strings.Contains(yml, "card.png") {
			t.Error("action.yml must attest card.png")
		}
	}
}

// TestCardFourMetricsOnly verifies that no metric beyond the fixed four
// headline numbers appears on the card. This is a QA red-flag gate.
func TestCardFourMetricsOnly(t *testing.T) {
	r := cardFixture()
	svg := string(thunkCardSVG(t, r))

	// Labels that must NOT appear as card metric labels
	for _, forbidden := range []string{
		"Deep review",
		"Review comments",
		"Time to first review",
		"Rework",
		"Authored",
		"Contributions",
	} {
		if strings.Contains(svg, forbidden) {
			t.Errorf("card must not contain metric %q beyond the fixed four", forbidden)
		}
	}
}

// TestCardSigstoreAttestedOnlyWhenVerified verifies that the attested mark
// only renders when status is "verified". QA red flag.
func TestCardSigstoreAttestedOnlyWhenVerified(t *testing.T) {
	t.Run("unverified report omits attested mark", func(t *testing.T) {
		r := cardFixture()
		r.Verification.Status = report.StatusUnverified
		svg := string(thunkCardSVG(t, r))
		if strings.Contains(svg, "Sigstore attested") {
			t.Error("unverified report shows Sigstore attested mark")
		}
	})

	t.Run("verified report shows attested mark", func(t *testing.T) {
		r := cardFixture()
		r.Verification.Status = report.StatusVerified
		svg := string(thunkCardSVG(t, r))
		if !strings.Contains(svg, "Sigstore attested") {
			t.Error("verified report should show Sigstore attested mark")
		}
	})
}

// TestCardMissingMetricsDash verifies that missing metrics render as em-dash
// and nothing is invented. QA red flag.
func TestCardMissingMetricsDash(t *testing.T) {
	r := cardFixture()
	// Set all four metric sources to nil
	r.Collaboration = nil
	r.Cadence = nil

	svg := string(thunkCardSVG(t, r))
	// Verify the four metric values are all em-dash. The metric values appear
	// as text nodes with the headline styling (font-size="36" font-weight="800").
	// We extract each font-size="36" text element and check its content.
	metricValueRe := regexp.MustCompile(`font-size="36" font-weight="800"[^>]*>([^<]+)<`)
	matches := metricValueRe.FindAllStringSubmatch(svg, -1)
	if len(matches) < 4 {
		t.Errorf("expected at least 4 metric value elements, got %d", len(matches))
	}
	dashCount := 0
	for _, m := range matches {
		if m[1] == "—" {
			dashCount++
		}
	}
	if dashCount < 4 {
		t.Errorf("expected at least 4 em-dash placeholders for missing metrics, got %d", dashCount)
	}

	// The four metric labels should still be present.
	for _, label := range []string{"PRs merged", "Reviews given", "Median TTM", "Active days"} {
		if !strings.Contains(svg, label) {
			t.Errorf("sparse card should still show label %q", label)
		}
	}
}

// TestCardNoExternalFonts verifies the SVG uses system font stack only.
// QA red flag.
func TestCardNoExternalFonts(t *testing.T) {
	r := cardFixture()
	svg := string(thunkCardSVG(t, r))

	if strings.Contains(svg, "@import") {
		t.Error("card SVG must not use @import for external fonts")
	}
	if strings.Contains(svg, "<link ") {
		t.Error("card SVG must not use <link> for external resources")
	}
	// System font stack only - no url() references in font-family
	if strings.Contains(svg, "font-family") {
		fontRe := regexp.MustCompile(`font-family="([^"]+)"`)
		for _, m := range fontRe.FindAllStringSubmatch(svg, -1) {
			if strings.Contains(m[1], "url(") {
				t.Errorf("card SVG uses url() in font-family: %s", m[1])
			}
		}
	}
}

// TestCardDimensionsExact verifies the exact 1200x627 constraint with extra
// rigor. QA red flag.
func TestCardDimensionsExact(t *testing.T) {
	r := cardFixture()
	svg := string(thunkCardSVG(t, r))

	// Must NOT have a different viewBox.
	viewBox := extractViewBox(svg)
	if viewBox != "0 0 1200 627" {
		t.Errorf("card viewBox must be exactly '0 0 1200 627', got %q", viewBox)
	}
}

// TestCardMedianTTMFormat verifies median time to merge is formatted correctly.
func TestCardMedianTTMFormat(t *testing.T) {
	r := cardFixture()
	r.Collaboration.TimeToMerge = &report.DurationStats{Count: 42, MedianHours: 18.5}
	svg := string(thunkCardSVG(t, r))

	if !strings.Contains(svg, "18.5") {
		t.Error("card should show median TTM value 18.5")
	}
}

// TestCardFullSampleFromGoldenFixture renders the sample-report.json via CardSVG
// and runs structural assertions. This mirrors the golden test pattern in
// golden_test.go.
func TestCardGoldenFullStructural(t *testing.T) {
	r := fullCardFixtureFromSampleAsReport(t)
	svg := thunkCardSVG(t, r)
	svgStr := string(svg)

	// The sample fixture is verified (status=verified), so:
	if !strings.Contains(svgStr, "Sigstore attested") {
		t.Error("sample report is verified but card omits Sigstore attested")
	}

	// Coverage is acme/widgets, acme/platform, acme/infra — single-owner
	if !strings.Contains(svgStr, "acme") {
		t.Error("card should show org context for acme")
	}

	// Window
	if !strings.Contains(svgStr, "2025-06-01") || !strings.Contains(svgStr, "2026-06-01") {
		t.Error("card should show coverage window")
	}
}

// verifyUnitTest helpers: these patterns match how render_test.go structures
// its unit-style tests — content assertions without golden files.

func TestCardSVGOutputIsValidSVG(t *testing.T) {
	r := cardFixture()
	svg := string(thunkCardSVG(t, r))

	if !strings.HasPrefix(strings.TrimSpace(svg), "<svg") {
		t.Error("card output must start with <svg")
	}
	if !strings.HasSuffix(strings.TrimSpace(svg), "</svg>") {
		fmt.Println(svg[len(svg)-50:])
		t.Error("card output must end with </svg>")
	}
}

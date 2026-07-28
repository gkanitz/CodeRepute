package report_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gkanitz/coderepute/report"
)

func TestSLSAProvenanceCI(t *testing.T) {
	env := map[string]string{
		"GITHUB_ACTIONS":      "true",
		"GITHUB_REPOSITORY":   "acme/widgets",
		"GITHUB_WORKFLOW_REF": "acme/widgets/.github/workflows/report.yml@refs/heads/main",
		"GITHUB_RUN_ID":       "9000000001",
		"GITHUB_SERVER_URL":   "https://github.com",
	}

	startedAt := time.Date(2025, 11, 1, 9, 0, 0, 0, time.UTC)
	finishedAt := time.Date(2025, 11, 1, 9, 2, 34, 0, time.UTC)
	p := report.CIProvenance(envFrom(env), startedAt, finishedAt, "v0.2.1")

	if p == nil {
		t.Fatal("CIProvenance with GITHUB_ACTIONS=true = nil, want populated block")
	}

	if p.BuildType != report.SLSABuildTypeURI {
		t.Errorf("BuildType = %q, want %q", p.BuildType, report.SLSABuildTypeURI)
	}

	wantBuilder := "https://github.com/acme/widgets/.github/workflows/report.yml@refs/heads/main"
	if p.BuilderID != wantBuilder {
		t.Errorf("BuilderID = %q, want %q", p.BuilderID, wantBuilder)
	}

	wantInvocation := "https://github.com/acme/widgets/actions/runs/9000000001"
	if p.InvocationID != wantInvocation {
		t.Errorf("InvocationID = %q, want %q", p.InvocationID, wantInvocation)
	}

	if p.StartedOn == nil {
		t.Fatal("StartedOn = nil, want the report-generation start timestamp")
	}
	if !p.StartedOn.Equal(startedAt) {
		t.Errorf("StartedOn = %v, want %v", p.StartedOn, startedAt)
	}

	if p.FinishedOn == nil {
		t.Fatal("FinishedOn = nil, want non-nil")
	}
	if !p.FinishedOn.Equal(finishedAt) {
		t.Errorf("FinishedOn = %v, want %v", p.FinishedOn, finishedAt)
	}

	if len(p.ResolvedDependencies) != 1 {
		t.Fatalf("len(ResolvedDependencies) = %d, want 1", len(p.ResolvedDependencies))
	}
	wantDepURI := "https://github.com/gkanitz/CodeRepute@v0.2.1"
	if p.ResolvedDependencies[0].URI != wantDepURI {
		t.Errorf("ResolvedDependencies[0].URI = %q, want %q", p.ResolvedDependencies[0].URI, wantDepURI)
	}

	// Serialize to JSON and verify the shape matches the expected literal.
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal SLSAProvenance: %v", err)
	}

	var doc struct {
		BuildType    string `json:"build_type"`
		BuilderID    string `json:"builder_id"`
		InvocationID string `json:"invocation_id"`
		StartedOn    string `json:"started_on"`
		FinishedOn   string `json:"finished_on"`
		Deps         []struct {
			URI string `json:"uri"`
		} `json:"resolved_dependencies"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal SLSAProvenance: %v", err)
	}

	if doc.BuildType != report.SLSABuildTypeURI {
		t.Errorf("JSON build_type = %q, want %q", doc.BuildType, report.SLSABuildTypeURI)
	}
	if doc.BuilderID != wantBuilder {
		t.Errorf("JSON builder_id = %q, want %q", doc.BuilderID, wantBuilder)
	}
	if doc.InvocationID != wantInvocation {
		t.Errorf("JSON invocation_id = %q, want %q", doc.InvocationID, wantInvocation)
	}
	if doc.StartedOn != "2025-11-01T09:00:00Z" {
		t.Errorf("JSON started_on = %q, want 2025-11-01T09:00:00Z", doc.StartedOn)
	}
	if doc.FinishedOn != "2025-11-01T09:02:34Z" {
		t.Errorf("JSON finished_on = %q, want 2025-11-01T09:02:34Z", doc.FinishedOn)
	}
	if len(doc.Deps) != 1 {
		t.Fatalf("JSON resolved_dependencies has %d entries, want 1", len(doc.Deps))
	}
	if doc.Deps[0].URI != wantDepURI {
		t.Errorf("JSON resolved_dependencies[0].uri = %q, want %q", doc.Deps[0].URI, wantDepURI)
	}

	// The started_on key must be present now that the start timestamp is recorded.
	if !strings.Contains(string(raw), "started_on") {
		t.Error("JSON missing started_on key, want the recorded start timestamp")
	}

	// Verify the provenance block survives a report marshal/parse round trip
	// when wired via Build.
	now := time.Date(2025, 11, 1, 9, 0, 0, 0, time.UTC)
	r := report.Build(activityFixture(), nil, nil, now)
	r.SLSAProvenance = p
	if err := r.Validate(); err != nil {
		t.Fatalf("Validate() with SLSAProvenance: %v", err)
	}

	reportJSON, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}

	parsed, err := report.Parse(reportJSON)
	if err != nil {
		t.Fatalf("Parse() report with SLSAProvenance: %v", err)
	}
	if parsed.SLSAProvenance == nil {
		t.Fatal("Parse() lost SLSAProvenance block")
	}
	if parsed.SLSAProvenance.BuildType != report.SLSABuildTypeURI {
		t.Errorf("round-trip BuildType = %q, want %q", parsed.SLSAProvenance.BuildType, report.SLSABuildTypeURI)
	}
	if parsed.SLSAProvenance.InvocationID != wantInvocation {
		t.Errorf("round-trip InvocationID = %q, want %q", parsed.SLSAProvenance.InvocationID, wantInvocation)
	}
}

func TestSLSAProvenanceCarriesStartedOn(t *testing.T) {
	env := map[string]string{
		"GITHUB_ACTIONS":      "true",
		"GITHUB_REPOSITORY":   "acme/widgets",
		"GITHUB_WORKFLOW_REF": "acme/widgets/.github/workflows/report.yml@refs/heads/main",
		"GITHUB_RUN_ID":       "9000000001",
		"GITHUB_SERVER_URL":   "https://github.com",
	}

	startedAt := time.Date(2025, 11, 1, 9, 0, 0, 0, time.UTC)
	finishedAt := time.Date(2025, 11, 1, 9, 2, 34, 0, time.UTC)
	p := report.CIProvenance(envFrom(env), startedAt, finishedAt, "v0.2.1")
	if p == nil {
		t.Fatal("CIProvenance with GITHUB_ACTIONS=true = nil, want populated block")
	}

	if p.StartedOn == nil {
		t.Fatal("StartedOn = nil, want the report-generation start timestamp")
	}
	if !p.StartedOn.Equal(startedAt) {
		t.Errorf("StartedOn = %v, want %v", p.StartedOn, startedAt)
	}

	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal SLSAProvenance: %v", err)
	}
	if !strings.Contains(string(raw), `"started_on": "2025-11-01T09:00:00Z"`) &&
		!strings.Contains(string(raw), `"started_on":"2025-11-01T09:00:00Z"`) {
		t.Errorf("JSON missing started_on = 2025-11-01T09:00:00Z; got %s", raw)
	}
}

func TestSLSAProvenanceNonCI(t *testing.T) {
	// No GITHUB_ACTIONS at all → nil.
	if p := report.CIProvenance(envFrom(nil), time.Now(), time.Now(), "dev"); p != nil {
		t.Errorf("CIProvenance with no env = %+v, want nil", p)
	}

	// GITHUB_ACTIONS=false → nil.
	if p := report.CIProvenance(envFrom(map[string]string{"GITHUB_ACTIONS": "false"}), time.Now(), time.Now(), "dev"); p != nil {
		t.Errorf("CIProvenance with GITHUB_ACTIONS=false = %+v, want nil", p)
	}

	// GITHUB_ACTIONS set to any non-"true" value → nil.
	if p := report.CIProvenance(envFrom(map[string]string{"GITHUB_ACTIONS": "0"}), time.Now(), time.Now(), "dev"); p != nil {
		t.Errorf("CIProvenance with GITHUB_ACTIONS=0 = %+v, want nil", p)
	}
}

func TestSLSAProvenanceOmittedInNonCIReportJSON(t *testing.T) {
	// Build a report normally (no CI env) and verify slsa_provenance is absent.
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	r := report.Build(activityFixture(), nil, nil, now)

	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var doc map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if _, ok := doc["slsa_provenance"]; ok {
		t.Error("non-CI report JSON contains slsa_provenance key, want omitted")
	}
}

func TestManifestRoundTripLegacy(t *testing.T) {
	// A manifest JSON without slsa_provenance (the old format before this
	// change was introduced) must unmarshal without error.
	previousReportJSON := `{
		"schema_version": "v0",
		"generated_at": "2025-11-01T09:00:00Z",
		"subject": {
			"platform": "github",
			"username": "octocat",
			"account_id": "583231"
		},
		"coverage": {
			"repos": ["acme/widgets"],
			"window": {
				"since": "2025-06-01T00:00:00Z",
				"until": "2026-06-01T00:00:00Z"
			},
			"token_scope": "repo"
		},
		"verification": {
			"status": "unverified",
			"reason": "local run; no CI attestation"
		}
	}`

	parsed, err := report.Parse([]byte(previousReportJSON))
	if err != nil {
		t.Fatalf("Parse() legacy report without slsa_provenance: %v", err)
	}

	if parsed.SLSAProvenance != nil {
		t.Error("Parse() produced non-nil SLSAProvenance for legacy JSON without the field")
	}
}

func TestSLSAProvenanceWithEmptyVersion(t *testing.T) {
	env := map[string]string{
		"GITHUB_ACTIONS":      "true",
		"GITHUB_REPOSITORY":   "acme/widgets",
		"GITHUB_WORKFLOW_REF": "acme/widgets/.github/workflows/report.yml@refs/heads/main",
		"GITHUB_RUN_ID":       "9000000001",
		"GITHUB_SERVER_URL":   "https://github.com",
	}

	// When the version is empty, resolved_dependencies should be omitted.
	p := report.CIProvenance(envFrom(env), time.Now(), time.Now(), "")
	if p == nil {
		t.Fatal("CIProvenance with GITHUB_ACTIONS=true = nil, want populated block")
	}
	if len(p.ResolvedDependencies) != 0 {
		t.Errorf("len(ResolvedDependencies) = %d, want 0 when version is empty", len(p.ResolvedDependencies))
	}
}

func TestSLSAProvenanceWithKnownVerificationValues(t *testing.T) {
	// Use the same env values that CIVerification tests use and verify that
	// the provenance builder_id matches what CIVerification sets as WorkflowRef,
	// and invocation_id matches CIVerification's RunURL.
	env := map[string]string{
		"GITHUB_ACTIONS":      "true",
		"GITHUB_REPOSITORY":   "acme/widgets",
		"GITHUB_WORKFLOW_REF": "acme/widgets/.github/workflows/report.yml@refs/heads/main",
		"GITHUB_RUN_ID":       "9000000001",
		"GITHUB_SERVER_URL":   "https://github.com",
	}

	// Get the verification block using these values.
	v := report.CIVerification(envFrom(env), "jsmith")
	if v == nil {
		t.Fatal("CIVerification returned nil")
	}

	// Get the provenance block using the same values.
	startedAt := time.Now()
	finishedAt := time.Now()
	p := report.CIProvenance(envFrom(env), startedAt, finishedAt, "v0.2.1")
	if p == nil {
		t.Fatal("CIProvenance returned nil")
	}

	// The builder_id should be server URL + "/" + workflow_ref.
	wantBuilder := "https://github.com/acme/widgets/.github/workflows/report.yml@refs/heads/main"
	if p.BuilderID != wantBuilder {
		t.Errorf("BuilderID = %q, want %q", p.BuilderID, wantBuilder)
	}

	// The invocation_id should match the verification RunURL.
	if p.InvocationID != v.RunURL {
		t.Errorf("InvocationID = %q, want verification.RunURL = %q", p.InvocationID, v.RunURL)
	}
}

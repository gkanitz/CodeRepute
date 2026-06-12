// Verification-block population: how a report run binds itself to a CI
// identity, and the honest unverified fallback everywhere else.
//
// Attestation only exists in CI. The CLI itself never claims more than the
// environment proves: outside a recognized CI environment the default
// unverified block from Build stands; inside GitHub Actions the block
// records the producing workflow identity and where its Sigstore
// attestation can be checked.
package report

import "fmt"

// AttestationTypeSigstore names the Sigstore/OIDC artifact attestation that
// GitHub's actions/attest-build-provenance produces over report.json.
const AttestationTypeSigstore = "sigstore-github-artifact-attestation"

// Attestation points at where the report's attestation lives and how a
// consumer checks it.
type Attestation struct {
	// Type names the attestation mechanism.
	Type string `json:"type"`
	// URL is the repository's attestations page on the platform.
	URL string `json:"url"`
	// VerifyCommand is the exact command a consumer runs against the
	// downloaded report.json to verify origin.
	VerifyCommand string `json:"verify_command"`
}

// CIVerification inspects the environment (via getenv) and returns the
// verification block for a recognized CI run, or nil when not running in
// CI so the caller keeps the explicit unverified default.
func CIVerification(getenv func(string) string) *Verification {
	if getenv("GITHUB_ACTIONS") != "true" {
		return nil
	}
	repo := getenv("GITHUB_REPOSITORY")
	runID := getenv("GITHUB_RUN_ID")
	server := getenv("GITHUB_SERVER_URL")
	return &Verification{
		Status:      StatusVerified,
		Provider:    "github-actions",
		Repository:  repo,
		WorkflowRef: getenv("GITHUB_WORKFLOW_REF"),
		RunID:       runID,
		RunURL:      fmt.Sprintf("%s/%s/actions/runs/%s", server, repo, runID),
		Attestation: &Attestation{
			Type:          AttestationTypeSigstore,
			URL:           fmt.Sprintf("%s/%s/attestations", server, repo),
			VerifyCommand: fmt.Sprintf("gh attestation verify report.json --repo %s", repo),
		},
	}
}

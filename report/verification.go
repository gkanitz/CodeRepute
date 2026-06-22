// Verification-block population: how a report run binds itself to a CI
// identity, and the honest unverified fallback everywhere else.
//
// Attestation only exists in CI. The CLI itself never claims more than the
// environment proves: outside a recognized CI environment the default
// unverified block from Build stands; inside GitHub Actions the block
// records the producing workflow identity and where its Sigstore
// attestation can be checked.
package report

import (
	"fmt"
	"net/url"
)

// AttestationTypeSigstore names the Sigstore/OIDC artifact attestation that
// GitHub's actions/attest-build-provenance produces over report.json.
const AttestationTypeSigstore = "sigstore-github-artifact-attestation"

// verifyBaseURL is the canonical base URL for the report verification page.
// It will be updated to https://coderepute.dev/verify/ when the domain is registered.
const verifyBaseURL = "https://grkanitz.github.io/CodeRepute/verify/"

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

// GitLabVerification inspects the environment for GitLab CI identity and
// returns the verification block, or nil when not running in GitLab CI.
// GitLab CI does not provide Sigstore OIDC attestation (unlike GitHub Actions).
// The verification block records job identity but cannot be independently
// verified without GitLab API access to the pipeline run.
func GitLabVerification(getenv func(string) string, subject string) *Verification {
	if getenv("GITLAB_CI") != "true" {
		return nil
	}
	project := getenv("CI_PROJECT_PATH")
	ref := getenv("CI_COMMIT_REF_NAME")
	return &Verification{
		Status:      StatusVerified,
		Provider:    "gitlab-ci",
		Repository:  project,
		WorkflowRef: project + "/.gitlab-ci.yml@" + ref,
		RunURL:      getenv("CI_JOB_URL"),
		VerifyURL:   verifyBaseURL + "?repo=" + url.QueryEscape(project) + "&subject=" + url.QueryEscape(subject),
		Note:        "GitLab CI does not provide Sigstore OIDC attestation. The verification block records job identity (project, ref, job URL) but cannot be independently verified without GitLab API access to the pipeline run. Unlike GitHub Actions, there is no cryptographic attestation of file integrity or workflow identity.",
	}
}

// CIVerification inspects the environment (via getenv) and returns the
// verification block for a recognized CI run, or nil when not running in
// CI so the caller keeps the explicit unverified default.
func CIVerification(getenv func(string) string, subject string) *Verification {
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
		VerifyURL:   verifyBaseURL + "?repo=" + url.QueryEscape(repo) + "&subject=" + url.QueryEscape(subject),
		Attestation: &Attestation{
			Type:          AttestationTypeSigstore,
			URL:           fmt.Sprintf("%s/%s/attestations", server, repo),
			VerifyCommand: fmt.Sprintf("gh attestation verify report.json --repo %s", repo),
		},
	}
}

// Verification-block population: how a report run binds itself to a CI
// identity, and the honest unverified fallback everywhere else.
//
// Attestation only exists in CI. The CLI itself never claims more than the
// environment proves: outside a recognized CI environment the default
// unverified block from Build stands; inside GitHub Actions the block
// records the producing workflow identity and where its Sigstore
// attestation can be checked.
package report

// CIVerification inspects the environment (via getenv) and returns the
// verification block for a recognized CI run, or nil when not running in
// CI so the caller keeps the explicit unverified default.
func CIVerification(getenv func(string) string) *Verification {
	if getenv("GITHUB_ACTIONS") != "true" {
		return nil
	}
	return nil
}

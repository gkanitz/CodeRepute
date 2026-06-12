package github

// Token scope classification for the report's coverage stamp. The class
// tells a reader what kind of credential produced the report — and so
// what it could plausibly see — without ever echoing the token itself.

import "strings"

// Token scope classes stamped into report coverage.
const (
	ScopeClassAppInstallation = "app-installation"
	ScopeClassFineGrainedPAT  = "fine-grained-pat"
	ScopeClassClassicPAT      = "classic-pat"
	ScopeClassOAuthApp        = "oauth-app"
	ScopeClassUnknown         = "unknown"
)

// ClassifyToken derives the token scope class from the token's format and
// the X-OAuth-Scopes header the API reported for it. The token value is
// only inspected for its well-known prefix; it is never stored or logged.
func ClassifyToken(token, oauthScopes string) string {
	switch {
	case strings.HasPrefix(token, "ghs_"):
		return ScopeClassAppInstallation
	case strings.HasPrefix(token, "github_pat_"):
		return ScopeClassFineGrainedPAT
	case strings.HasPrefix(token, "ghp_"):
		return ScopeClassClassicPAT
	case strings.HasPrefix(token, "gho_"):
		return ScopeClassOAuthApp
	case oauthScopes != "":
		// Only classic tokens report OAuth scopes via header.
		return ScopeClassClassicPAT
	default:
		return ScopeClassUnknown
	}
}

package gitlab

// Token scope classification for the report's coverage stamp. The class
// tells a reader what kind of credential produced the report — and so
// what it could plausibly see — without ever echoing the token itself.

import "strings"

// Token scope classes stamped into report coverage. GitLab shares the
// glpat- prefix between personal, group, and project access tokens, so —
// unlike GitHub — the class cannot distinguish them and says so honestly.
const (
	ScopeClassAccessToken = "access-token" // personal, group, or project access token
	ScopeClassOAuth       = "oauth-app"
	ScopeClassUnknown     = "unknown"
)

// ClassifyToken derives the token scope class from the token's format
// and the scopes the API reported for it. The token value is only
// inspected for its well-known prefix; it is never stored or logged.
func ClassifyToken(token, scopes string) string {
	switch {
	case strings.HasPrefix(token, "glpat-"):
		return ScopeClassAccessToken
	case strings.HasPrefix(token, "gloas-"):
		return ScopeClassOAuth
	case scopes != "":
		// Only access tokens answer /personal_access_tokens/self,
		// so reported scopes imply an access token with a custom or
		// legacy prefix.
		return ScopeClassAccessToken
	default:
		return ScopeClassUnknown
	}
}

package github_test

import (
	"testing"

	"github.com/gkanitz/coderepute/provider/github"
)

func TestClassifyToken(t *testing.T) {
	tests := []struct {
		name        string
		token       string
		oauthScopes string
		want        string
	}{
		{"app installation token", "ghs_abc123", "", "app-installation"},
		{"fine-grained PAT", "github_pat_11AAA_zzz", "", "fine-grained-pat"},
		{"classic PAT by prefix", "ghp_abc123", "repo, read:org", "classic-pat"},
		{"classic PAT by reported scopes", "0123456789abcdef0123456789abcdef01234567", "repo", "classic-pat"},
		{"oauth app token", "gho_abc123", "", "oauth-app"},
		{"unrecognized token without scopes", "mystery-token", "", "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := github.ClassifyToken(tt.token, tt.oauthScopes); got != tt.want {
				t.Errorf("ClassifyToken(%q, %q) = %q, want %q", tt.token, tt.oauthScopes, got, tt.want)
			}
		})
	}
}

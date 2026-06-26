package gitlab_test

import (
	"testing"

	"github.com/gkanitz/coderepute/provider/gitlab"
)

func TestClassifyToken(t *testing.T) {
	tests := []struct {
		name   string
		token  string
		scopes string
		want   string
	}{
		// GitLab uses the same glpat- prefix for personal, group, and
		// project access tokens; the class is honest about that ambiguity.
		{"group access token", "glpat-xxxxxxxxxxxxxxxxxxxx", "read_api", gitlab.ScopeClassAccessToken},
		{"personal access token without scope info", "glpat-yyyyyyyyyyyyyyyyyyyy", "", gitlab.ScopeClassAccessToken},
		{"oauth token", "gloas-zzzzzzzzzzzzzzzzzzzz", "", gitlab.ScopeClassOAuth},
		{"unprefixed token that reported scopes", "legacytokenvalue", "read_api", gitlab.ScopeClassAccessToken},
		{"unrecognizable token", "legacytokenvalue", "", gitlab.ScopeClassUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := gitlab.ClassifyToken(tt.token, tt.scopes); got != tt.want {
				t.Errorf("ClassifyToken(%q, %q) = %q, want %q", tt.token, tt.scopes, got, tt.want)
			}
		})
	}
}

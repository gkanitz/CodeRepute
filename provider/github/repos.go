package github

// Repository enumeration for org-scoped coverage. One run covers every
// repository the token can see; the resulting list feeds the report's
// coverage stamp so omissions stay visible.
//
// Like the rest of the adapter this file reads only API metadata — repo
// names — never repository contents.

import (
	"context"
	"fmt"
)

type apiRepo struct {
	FullName string `json:"full_name"`
}

// ListOrgRepos returns every repository of org visible to the token, as
// "owner/name", following pagination to exhaustion.
func (a *Adapter) ListOrgRepos(ctx context.Context, org string) ([]string, error) {
	var out []string
	url := fmt.Sprintf("%s/orgs/%s/repos?per_page=100", a.baseURL, org)
	for url != "" {
		var page []apiRepo
		resp, err := a.getJSON(ctx, url, &page)
		if err != nil {
			return nil, fmt.Errorf("github: list repos of org %q: %w", org, err)
		}
		for _, r := range page {
			out = append(out, r.FullName)
		}
		url = nextPage(resp.Header.Get("Link"))
	}
	return out, nil
}

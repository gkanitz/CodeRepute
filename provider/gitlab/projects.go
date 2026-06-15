package gitlab

// Project enumeration for group-scoped coverage. One run covers every
// project the token can see in the group and its subgroups; the
// resulting list feeds the report's coverage stamp so omissions stay
// visible.
//
// Like the rest of the adapter this file reads only API metadata —
// project paths — never repository contents.

import (
	"context"
	"fmt"
)

type apiProject struct {
	PathWithNamespace string `json:"path_with_namespace"`
}

// ListGroupProjects returns every project of the group (including
// subgroups) visible to the token, as "group/project" paths, following
// pagination to exhaustion. This is the GitLab counterpart of GitHub's
// org repo enumeration; a group access token with read_api scope is
// sufficient.
func (a *Adapter) ListGroupProjects(ctx context.Context, group string) ([]string, error) {
	var out []string
	url := fmt.Sprintf("%s/groups/%s/projects?include_subgroups=true&per_page=100", a.baseURL, projectPath(group))
	for url != "" {
		var page []apiProject
		next, err := a.getJSONPage(ctx, url, &page)
		if err != nil {
			return nil, fmt.Errorf("gitlab: list projects of group %q: %w", group, err)
		}
		for _, p := range page {
			out = append(out, p.PathWithNamespace)
		}
		url = next
	}
	return out, nil
}

package main

// Org-scoped coverage plumbing: minting GitHub App installation tokens
// and resolving the repo list a run covers. Token acquisition stays
// pluggable — the rest of the pipeline consumes "a token" and a repo
// list, never caring where either came from.

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/gkanitz/coderepute/provider"
	"github.com/gkanitz/coderepute/provider/github"
)

// mintInstallationToken exchanges App credentials (app ID + private key
// PEM file) for a short-lived installation token.
func mintInstallationToken(ctx context.Context, appID, keyPath string, installationID int64, apiBase string) (string, error) {
	raw, err := os.ReadFile(keyPath)
	if err != nil {
		return "", fmt.Errorf("read app key: %w", err)
	}
	key, err := github.ParseAppPrivateKey(raw)
	if err != nil {
		return "", err
	}
	auth := github.AppAuth{
		AppID:          appID,
		PrivateKey:     key,
		InstallationID: installationID,
		BaseURL:        apiBase,
	}
	return auth.InstallationToken(ctx)
}

// resolveRepos decides what one run covers: an explicit -repo list wins,
// then -org enumeration, then — under an App token — every repo of the
// installation. With a personal token and no scope flags, auto-discovers
// repos the subject contributed to via Search API.
func resolveRepos(ctx context.Context, adapter *github.Adapter, repoFlag, orgFlag, excludeRepoFlag, subject string, usingAppToken bool, window provider.Window) ([]string, error) {
	var repos []string

	switch {
	case repoFlag != "":
		for _, r := range strings.Split(repoFlag, ",") {
			if r = strings.TrimSpace(r); r != "" {
				repos = append(repos, r)
			}
		}
	case orgFlag != "":
		var err error
		repos, err = adapter.ListOrgRepos(ctx, orgFlag)
		if err != nil {
			return nil, err
		}
	case usingAppToken:
		var err error
		repos, err = adapter.ListInstallationRepos(ctx)
		if err != nil {
			return nil, err
		}
	default:
		var err error
		repos, err = adapter.ListContributedRepos(ctx, subject, window)
		if err != nil {
			return nil, err
		}
	}

	// Apply -exclude-repo filter regardless of which case produced the list.
	repos = filterRepos(repos, excludeRepoFlag)
	return repos, nil
}

// filterRepos removes entries matching the comma-separated exclude list from
// the repo list. Each entry in excludeFlag is trimmed; empty entries are
// ignored. Case-sensitive match on the full "owner/name" string.
func filterRepos(repos []string, excludeFlag string) []string {
	if excludeFlag == "" {
		return repos
	}
	exclude := map[string]bool{}
	for _, r := range strings.Split(excludeFlag, ",") {
		if r = strings.TrimSpace(r); r != "" {
			exclude[r] = true
		}
	}
	var out []string
	for _, r := range repos {
		if !exclude[r] {
			out = append(out, r)
		}
	}
	return out
}

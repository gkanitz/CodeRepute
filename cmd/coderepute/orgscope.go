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

	"github.com/grkanitz/coderepute/provider/github"
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
// installation.
func resolveRepos(ctx context.Context, adapter *github.Adapter, repoFlag, orgFlag string, usingAppToken bool) ([]string, error) {
	switch {
	case repoFlag != "":
		return strings.Split(repoFlag, ","), nil
	case orgFlag != "":
		return adapter.ListOrgRepos(ctx, orgFlag)
	case usingAppToken:
		return adapter.ListInstallationRepos(ctx)
	default:
		return nil, fmt.Errorf("no repos to cover: pass -repo or -org")
	}
}

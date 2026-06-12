package github

// GitHub App installation-token exchange: app ID + private key → signed
// app JWT → installation token. Token acquisition stays pluggable — the
// rest of the adapter consumes "a token" and never knows where it came
// from. Like everything else here, only API metadata is read.

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// AppAuth mints GitHub App installation tokens from the app's credentials
// (the app-manifest flow hands the org both). If InstallationID is zero,
// the sole installation of the app is discovered automatically; with more
// than one installation the caller must choose.
type AppAuth struct {
	AppID          string          // numeric App ID (or client ID) used as the JWT issuer
	PrivateKey     *rsa.PrivateKey // the app's private key
	InstallationID int64           // optional; 0 means auto-discover
	BaseURL        string          // API root; empty means api.github.com
	HTTPClient     *http.Client    // optional; nil means a default client

	now func() time.Time // test seam; nil means time.Now
}

// ParseAppPrivateKey parses a GitHub App private key PEM (GitHub issues
// PKCS#1; PKCS#8 is accepted too).
func ParseAppPrivateKey(raw []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, errors.New("github: app private key is not PEM")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("github: parse app private key: %w", err)
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("github: app private key is %T, want RSA", parsed)
	}
	return key, nil
}

// InstallationToken exchanges the app credentials for a short-lived
// installation token ("ghs_…").
func (a AppAuth) InstallationToken(ctx context.Context) (string, error) {
	jwt, err := a.signJWT()
	if err != nil {
		return "", err
	}

	id := a.InstallationID
	if id == 0 {
		id, err = a.discoverInstallation(ctx, jwt)
		if err != nil {
			return "", err
		}
	}

	var minted struct {
		Token string `json:"token"`
	}
	url := fmt.Sprintf("%s/app/installations/%d/access_tokens", a.baseURL(), id)
	if err := a.doJSON(ctx, http.MethodPost, url, jwt, http.StatusCreated, &minted); err != nil {
		return "", fmt.Errorf("github: mint installation token: %w", err)
	}
	if minted.Token == "" {
		return "", errors.New("github: installation token response carried no token")
	}
	return minted.Token, nil
}

// discoverInstallation returns the ID of the app's sole installation, or
// an error listing the candidates when there are several.
func (a AppAuth) discoverInstallation(ctx context.Context, jwt string) (int64, error) {
	var installations []struct {
		ID      int64 `json:"id"`
		Account struct {
			Login string `json:"login"`
		} `json:"account"`
	}
	url := a.baseURL() + "/app/installations?per_page=100"
	if err := a.doJSON(ctx, http.MethodGet, url, jwt, http.StatusOK, &installations); err != nil {
		return 0, fmt.Errorf("github: list app installations: %w", err)
	}
	switch len(installations) {
	case 0:
		return 0, errors.New("github: the app has no installations; install it on the org first")
	case 1:
		return installations[0].ID, nil
	default:
		var candidates []string
		for _, in := range installations {
			candidates = append(candidates, fmt.Sprintf("%d (%s)", in.ID, in.Account.Login))
		}
		return 0, fmt.Errorf("github: the app has %d installations — pick one with an explicit installation id: %s",
			len(installations), strings.Join(candidates, ", "))
	}
}

// signJWT builds the RS256 app JWT GitHub expects: issued slightly in the
// past to absorb clock drift, valid for ten minutes at most.
func (a AppAuth) signJWT() (string, error) {
	if a.AppID == "" {
		return "", errors.New("github: app id is required")
	}
	if a.PrivateKey == nil {
		return "", errors.New("github: app private key is required")
	}

	nowFn := a.now
	if nowFn == nil {
		nowFn = time.Now
	}
	now := nowFn().UTC()

	encode := func(v any) (string, error) {
		raw, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return base64.RawURLEncoding.EncodeToString(raw), nil
	}
	header, err := encode(map[string]string{"alg": "RS256", "typ": "JWT"})
	if err != nil {
		return "", err
	}
	claims, err := encode(map[string]any{
		"iss": a.AppID,
		"iat": now.Add(-60 * time.Second).Unix(),
		"exp": now.Add(9 * time.Minute).Unix(),
	})
	if err != nil {
		return "", err
	}

	signingInput := header + "." + claims
	digest := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, a.PrivateKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("github: sign app jwt: %w", err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func (a AppAuth) baseURL() string {
	if a.BaseURL != "" {
		return a.BaseURL
	}
	return defaultBaseURL
}

func (a AppAuth) doJSON(ctx context.Context, method, url, jwt string, wantStatus int, v any) error {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+jwt)

	httpc := a.HTTPClient
	if httpc == nil {
		httpc = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := httpc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != wantStatus {
		return fmt.Errorf("%s %s: %s: %s", method, url, resp.Status, body)
	}
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("%s %s: decode: %w", method, url, err)
	}
	return nil
}

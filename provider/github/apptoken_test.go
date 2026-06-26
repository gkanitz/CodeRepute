package github_test

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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gkanitz/coderepute/provider/github"
)

// appFixtureServer replays the GitHub App token-exchange endpoints. Every
// request must carry a JWT signed by key and issued by appID; the server
// verifies the signature against the real public key.
func appFixtureServer(t *testing.T, key *rsa.PrivateKey, appID string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /app/installations", func(w http.ResponseWriter, r *http.Request) {
		requireAppJWT(t, r, &key.PublicKey, appID)
		serveFixture(t, w, "app_installations.json")
	})
	mux.HandleFunc("POST /app/installations/42/access_tokens", func(w http.ResponseWriter, r *http.Request) {
		requireAppJWT(t, r, &key.PublicKey, appID)
		w.WriteHeader(http.StatusCreated)
		serveFixture(t, w, "installation_token.json")
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// requireAppJWT checks the request carries a bearer JWT: RS256, signature
// valid for pub, issuer claim equal to appID.
func requireAppJWT(t *testing.T, r *http.Request, pub *rsa.PublicKey, appID string) {
	t.Helper()
	raw := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		t.Fatalf("Authorization %q is not a JWT", r.Header.Get("Authorization"))
	}
	decode := func(s string) []byte {
		b, err := base64.RawURLEncoding.DecodeString(s)
		if err != nil {
			t.Fatalf("JWT segment not base64url: %v", err)
		}
		return b
	}

	var header struct {
		Alg string `json:"alg"`
		Typ string `json:"typ"`
	}
	if err := json.Unmarshal(decode(parts[0]), &header); err != nil {
		t.Fatalf("JWT header: %v", err)
	}
	if header.Alg != "RS256" {
		t.Fatalf("JWT alg = %q, want RS256", header.Alg)
	}

	var claims struct {
		Iss string `json:"iss"`
		Iat int64  `json:"iat"`
		Exp int64  `json:"exp"`
	}
	if err := json.Unmarshal(decode(parts[1]), &claims); err != nil {
		t.Fatalf("JWT claims: %v", err)
	}
	if claims.Iss != appID {
		t.Errorf("JWT iss = %q, want %q", claims.Iss, appID)
	}
	if claims.Exp <= claims.Iat {
		t.Errorf("JWT exp %d not after iat %d", claims.Exp, claims.Iat)
	}

	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, digest[:], decode(parts[2])); err != nil {
		t.Errorf("JWT signature invalid: %v", err)
	}
}

func testRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return key
}

func TestAppAuthInstallationToken(t *testing.T) {
	key := testRSAKey(t)
	srv := appFixtureServer(t, key, "12345")

	auth := github.AppAuth{
		AppID:      "12345",
		PrivateKey: key,
		BaseURL:    srv.URL,
	}
	token, err := auth.InstallationToken(context.Background())
	if err != nil {
		t.Fatalf("InstallationToken: %v", err)
	}
	if token != "ghs_fixture-installation-token" {
		t.Errorf("token = %q, want ghs_fixture-installation-token", token)
	}
}

func TestAppAuthExplicitInstallationSkipsDiscovery(t *testing.T) {
	key := testRSAKey(t)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /app/installations", func(w http.ResponseWriter, r *http.Request) {
		t.Error("discovery endpoint called despite explicit installation id")
		http.NotFound(w, r)
	})
	mux.HandleFunc("POST /app/installations/7/access_tokens", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		serveFixture(t, w, "installation_token.json")
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	auth := github.AppAuth{
		AppID:          "12345",
		PrivateKey:     key,
		InstallationID: 7,
		BaseURL:        srv.URL,
	}
	token, err := auth.InstallationToken(context.Background())
	if err != nil {
		t.Fatalf("InstallationToken: %v", err)
	}
	if token != "ghs_fixture-installation-token" {
		t.Errorf("token = %q, want ghs_fixture-installation-token", token)
	}
}

func TestAppAuthAmbiguousInstallations(t *testing.T) {
	key := testRSAKey(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveFixture(t, w, "app_installations_multi.json")
	}))
	t.Cleanup(srv.Close)

	auth := github.AppAuth{AppID: "12345", PrivateKey: key, BaseURL: srv.URL}
	_, err := auth.InstallationToken(context.Background())
	if err == nil {
		t.Fatal("expected error when the app has several installations and none was chosen")
	}
	if !strings.Contains(err.Error(), "42") || !strings.Contains(err.Error(), "43") {
		t.Errorf("error %q should list the candidate installation ids", err)
	}
}

func TestParseAppPrivateKey(t *testing.T) {
	key := testRSAKey(t)

	t.Run("pkcs1", func(t *testing.T) {
		raw := pem.EncodeToMemory(&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(key),
		})
		parsed, err := github.ParseAppPrivateKey(raw)
		if err != nil {
			t.Fatalf("ParseAppPrivateKey(pkcs1): %v", err)
		}
		if !parsed.Equal(key) {
			t.Error("parsed key differs from original")
		}
	})

	t.Run("pkcs8", func(t *testing.T) {
		der, err := x509.MarshalPKCS8PrivateKey(key)
		if err != nil {
			t.Fatalf("marshal pkcs8: %v", err)
		}
		raw := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
		parsed, err := github.ParseAppPrivateKey(raw)
		if err != nil {
			t.Fatalf("ParseAppPrivateKey(pkcs8): %v", err)
		}
		if !parsed.Equal(key) {
			t.Error("parsed key differs from original")
		}
	})

	t.Run("garbage", func(t *testing.T) {
		if _, err := github.ParseAppPrivateKey([]byte("not a pem")); err == nil {
			t.Error("expected error for non-PEM input")
		}
	})
}

package ui

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/go-logr/logr"
	"golang.org/x/oauth2"
)

type tokenRequest struct {
	authHeader string
	form       url.Values
}

// newFakeOIDCProvider serves discovery and a token endpoint that records requests.
func newFakeOIDCProvider(t *testing.T) (*httptest.Server, func() []tokenRequest) {
	t.Helper()
	var (
		mu   sync.Mutex
		seen []tokenRequest
	)
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                srv.URL,
			"authorization_endpoint":                srv.URL + "/auth",
			"token_endpoint":                        srv.URL + "/token",
			"jwks_uri":                              srv.URL + "/keys",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mu.Lock()
		seen = append(seen, tokenRequest{authHeader: r.Header.Get("Authorization"), form: r.PostForm})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "at", "token_type": "Bearer"})
	})

	return srv, func() []tokenRequest {
		mu.Lock()
		defer mu.Unlock()
		return append([]tokenRequest(nil), seen...)
	}
}

func newPublicClientTestAuth(t *testing.T, issuer, secret string, pkce bool) (*OIDCAuth, error) {
	t.Helper()
	return NewOIDCAuth(context.Background(), OIDCConfig{
		IssuerURL:    issuer,
		ClientID:     "renovate-operator",
		ClientSecret: secret,
		RedirectURL:  "https://renovate.example.org/auth/callback",
		PKCEEnabled:  pkce,
	}, testEncryptionKey(t), logr.Discard(), NewMemorySessionStore())
}

func TestNewOIDCAuth_PublicClientRequiresPKCE(t *testing.T) {
	// Fails before discovery, so the issuer is never contacted.
	_, err := newPublicClientTestAuth(t, "http://127.0.0.1:0", "", false)
	if !errors.Is(err, ErrOIDCPublicClientWithoutPKCE) {
		t.Fatalf("expected ErrOIDCPublicClientWithoutPKCE, got %v", err)
	}
}

func TestNewOIDCAuth_PublicClientExchangeSendsNoSecret(t *testing.T) {
	srv, requests := newFakeOIDCProvider(t)

	auth, err := newPublicClientTestAuth(t, srv.URL, "", true)
	if err != nil {
		t.Fatalf("NewOIDCAuth: %v", err)
	}
	if auth.oauth2Config.Endpoint.AuthStyle != oauth2.AuthStyleInParams {
		t.Fatalf("public client must use AuthStyleInParams, got %v", auth.oauth2Config.Endpoint.AuthStyle)
	}

	verifier := oauth2.GenerateVerifier()
	if _, err := auth.oauth2Config.Exchange(context.Background(), "code", oauth2.VerifierOption(verifier)); err != nil {
		t.Fatalf("Exchange: %v", err)
	}

	got := requests()
	if len(got) != 1 {
		t.Fatalf("expected exactly one token request, got %d", len(got))
	}
	req := got[0]
	if req.authHeader != "" {
		t.Errorf("public client must not send an Authorization header, got %q", req.authHeader)
	}
	if _, ok := req.form["client_secret"]; ok {
		t.Errorf("public client must not send client_secret, got %q", req.form.Get("client_secret"))
	}
	if req.form.Get("client_id") != "renovate-operator" {
		t.Errorf("expected client_id in the request body, got %q", req.form.Get("client_id"))
	}
	if req.form.Get("code_verifier") != verifier {
		t.Errorf("expected the PKCE code_verifier in the request body")
	}
}

func TestNewOIDCAuth_ConfidentialClientKeepsSecret(t *testing.T) {
	srv, requests := newFakeOIDCProvider(t)

	auth, err := newPublicClientTestAuth(t, srv.URL, "s3cret", false)
	if err != nil {
		t.Fatalf("NewOIDCAuth: %v", err)
	}
	if _, err := auth.oauth2Config.Exchange(context.Background(), "code"); err != nil {
		t.Fatalf("Exchange: %v", err)
	}

	got := requests()
	if len(got) != 1 {
		t.Fatalf("expected exactly one token request, got %d", len(got))
	}
	user, pass, ok := (&http.Request{Header: http.Header{"Authorization": {got[0].authHeader}}}).BasicAuth()
	if !ok || user != "renovate-operator" || pass != "s3cret" {
		t.Errorf("confidential client must authenticate with the secret via Basic auth, got header %q", got[0].authHeader)
	}
}

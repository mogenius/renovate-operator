package ui

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-logr/logr"
	"golang.org/x/oauth2"
)

const (
	sessionCookieName = "renovate_session"
	stateCookieName   = "oidc_state"
	sessionDuration   = 24 * time.Hour
)

type OIDCConfig struct {
	IssuerURL          string
	ClientID           string
	ClientSecret       string
	RedirectURL        string
	SessionSecret      string
	InsecureSkipVerify bool
	LogoutURL          string
}

type sessionData struct {
	Email  string `json:"email"`
	Name   string `json:"name"`
	Expiry int64  `json:"exp"`
}

type OIDCAuth struct {
	provider         *oidc.Provider
	oauth2Config     oauth2.Config
	verifier         *oidc.IDTokenVerifier
	encryptionKey    [32]byte
	httpClient       *http.Client
	endSessionURL    string
	postLogoutRedirect string
	logger           logr.Logger
}

func NewOIDCAuth(ctx context.Context, cfg OIDCConfig, logger logr.Logger) (*OIDCAuth, error) {
	// Build a custom HTTP client for OIDC operations (discovery, token exchange)
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
		TLSHandshakeTimeout:  10 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
	}
	if cfg.InsecureSkipVerify {
		logger.Info("WARNING: OIDC TLS verification is disabled. Do not use this in production!")
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	}
	httpClient := &http.Client{Transport: transport}

	// Use the custom client for OIDC provider discovery
	oidcCtx := oidc.ClientContext(ctx, httpClient)
	provider, err := oidc.NewProvider(oidcCtx, cfg.IssuerURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create OIDC provider: %w", err)
	}

	oauth2Cfg := oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       []string{oidc.ScopeOpenID, "email", "profile"},
	}

	verifier := provider.Verifier(&oidc.Config{ClientID: cfg.ClientID})

	// Discover end_session_endpoint from provider if available
	endSessionURL := cfg.LogoutURL
	if endSessionURL == "" {
		var providerClaims struct {
			EndSessionEndpoint string `json:"end_session_endpoint"`
		}
		if err := provider.Claims(&providerClaims); err == nil && providerClaims.EndSessionEndpoint != "" {
			endSessionURL = providerClaims.EndSessionEndpoint
		}
	}
	if endSessionURL != "" {
		logger.Info("OIDC logout endpoint configured", "url", endSessionURL)
	}

	// Derive post-logout redirect from the redirect URL (strip the /auth/callback path)
	postLogoutRedirect := cfg.RedirectURL
	if idx := strings.Index(postLogoutRedirect, "/auth/"); idx > 0 {
		postLogoutRedirect = postLogoutRedirect[:idx]
	}

	var key [32]byte
	if cfg.SessionSecret != "" {
		key = sha256.Sum256([]byte(cfg.SessionSecret))
	} else {
		if _, err := io.ReadFull(rand.Reader, key[:]); err != nil {
			return nil, fmt.Errorf("failed to generate encryption key: %w", err)
		}
	}

	return &OIDCAuth{
		provider:           provider,
		oauth2Config:       oauth2Cfg,
		verifier:           verifier,
		encryptionKey:      key,
		httpClient:         httpClient,
		endSessionURL:      endSessionURL,
		postLogoutRedirect: postLogoutRedirect,
		logger:             logger,
	}, nil
}

func (o *OIDCAuth) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Always allow health endpoint
		if path == "/health" {
			next.ServeHTTP(w, r)
			return
		}

		// Always allow auth endpoints
		if strings.HasPrefix(path, "/auth/") {
			next.ServeHTTP(w, r)
			return
		}

		// Check session
		session, err := o.getSession(r)
		if err != nil || session == nil {
			// API requests get 401
			if strings.HasPrefix(path, "/api/") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
				return
			}
			// UI requests get redirected to login
			http.Redirect(w, r, "/auth/login", http.StatusFound)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (o *OIDCAuth) handleLogin(w http.ResponseWriter, r *http.Request) {
	state, err := generateRandomString(32)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     stateCookieName,
		Value:    state,
		Path:     "/",
		MaxAge:   300,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, o.oauth2Config.AuthCodeURL(state), http.StatusFound)
}

func (o *OIDCAuth) handleCallback(w http.ResponseWriter, r *http.Request) {
	// Validate state
	stateCookie, err := r.Cookie(stateCookieName)
	if err != nil {
		http.Error(w, "missing state cookie", http.StatusBadRequest)
		return
	}

	if r.URL.Query().Get("state") != stateCookie.Value {
		http.Error(w, "invalid state", http.StatusBadRequest)
		return
	}

	// Clear state cookie
	http.SetCookie(w, &http.Cookie{
		Name:     stateCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})

	// Exchange code for token (use custom HTTP client for TLS config)
	exchangeCtx := oidc.ClientContext(r.Context(), o.httpClient)
	oauth2Token, err := o.oauth2Config.Exchange(exchangeCtx, r.URL.Query().Get("code"))
	if err != nil {
		o.logger.Error(err, "failed to exchange code for token")
		http.Error(w, "failed to exchange token", http.StatusInternalServerError)
		return
	}

	// Extract and verify ID token
	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		http.Error(w, "no id_token in response", http.StatusInternalServerError)
		return
	}

	idToken, err := o.verifier.Verify(exchangeCtx, rawIDToken)
	if err != nil {
		o.logger.Error(err, "failed to verify ID token")
		http.Error(w, "failed to verify token", http.StatusInternalServerError)
		return
	}

	// Extract claims
	var claims struct {
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := idToken.Claims(&claims); err != nil {
		o.logger.Error(err, "failed to parse claims")
		http.Error(w, "failed to parse claims", http.StatusInternalServerError)
		return
	}

	// Create session
	session := sessionData{
		Email:  claims.Email,
		Name:   claims.Name,
		Expiry: time.Now().Add(sessionDuration).Unix(),
	}

	encrypted, err := o.encryptSession(session)
	if err != nil {
		o.logger.Error(err, "failed to encrypt session")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    encrypted,
		Path:     "/",
		MaxAge:   int(sessionDuration.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, "/", http.StatusFound)
}

func (o *OIDCAuth) handleLogout(w http.ResponseWriter, r *http.Request) {
	// Clear local session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})

	// Redirect to OIDC provider's end_session_endpoint if available
	if o.endSessionURL != "" {
		logoutURL := o.endSessionURL + "?client_id=" + o.oauth2Config.ClientID
		if o.postLogoutRedirect != "" {
			logoutURL += "&post_logout_redirect_uri=" + o.postLogoutRedirect
		}
		http.Redirect(w, r, logoutURL, http.StatusFound)
		return
	}

	// No end_session_endpoint configured, just redirect home
	http.Redirect(w, r, "/", http.StatusFound)
}

func (o *OIDCAuth) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	session, _ := o.getSession(r)

	result := map[string]any{
		"enabled": true,
	}

	if session != nil {
		result["authenticated"] = true
		result["email"] = session.Email
		result["name"] = session.Name
	} else {
		result["authenticated"] = false
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (o *OIDCAuth) getSession(r *http.Request) (*sessionData, error) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return nil, err
	}

	session, err := o.decryptSession(cookie.Value)
	if err != nil {
		return nil, err
	}

	if time.Now().Unix() > session.Expiry {
		return nil, fmt.Errorf("session expired")
	}

	return session, nil
}

func (o *OIDCAuth) encryptSession(session sessionData) (string, error) {
	plaintext, err := json.Marshal(session)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(o.encryptionKey[:])
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.URLEncoding.EncodeToString(ciphertext), nil
}

func (o *OIDCAuth) decryptSession(encrypted string) (*sessionData, error) {
	data, err := base64.URLEncoding.DecodeString(encrypted)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(o.encryptionKey[:])
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	var session sessionData
	if err := json.Unmarshal(plaintext, &session); err != nil {
		return nil, err
	}

	return &session, nil
}

func generateRandomString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

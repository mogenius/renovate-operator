package ui

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-logr/logr"
	"golang.org/x/oauth2"
)

type OIDCConfig struct {
	IssuerURL           string
	ClientID            string
	ClientSecret        string
	RedirectURL         string
	SessionSecret       string
	InsecureSkipVerify  bool
	LogoutURL           string
	AllowedGroupPrefix  string
	AllowedGroupPattern string
	AdditionalScopes    []string
}

type OIDCAuth struct {
	baseAuth
	provider           *oidc.Provider
	oauth2Config       oauth2.Config
	verifier           *oidc.IDTokenVerifier
	httpClient         *http.Client
	endSessionURL      string
	postLogoutRedirect string
	groupFilterConfig  GroupFilterConfig
}

func NewOIDCAuth(ctx context.Context, cfg OIDCConfig, logger logr.Logger) (*OIDCAuth, error) {
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
	}
	if cfg.InsecureSkipVerify {
		logger.Info("WARNING: OIDC TLS verification is disabled. Do not use this in production!")
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	}
	httpClient := &http.Client{Transport: transport}

	oidcCtx := oidc.ClientContext(ctx, httpClient)
	provider, err := oidc.NewProvider(oidcCtx, cfg.IssuerURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create OIDC provider: %w", err)
	}

	scopes := buildOIDCScopes(cfg.AdditionalScopes)
	if len(cfg.AdditionalScopes) > 0 {
		logger.Info("requesting additional OIDC scopes", "scopes", cfg.AdditionalScopes)
	}

	if cfg.AllowedGroupPrefix != "" || cfg.AllowedGroupPattern != "" {
		hasGroupsScope := false
		for _, s := range scopes {
			if s == "groups" {
				hasGroupsScope = true
				break
			}
		}
		if !hasGroupsScope {
			logger.Info("WARNING: group filtering is configured but 'groups' is not in additionalScopes. " +
				"If your OIDC provider requires the 'groups' scope to include group claims, add it to additionalScopes.")
		}
	}

	oauth2Cfg := oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       scopes,
	}

	verifier := provider.Verifier(&oidc.Config{ClientID: cfg.ClientID})

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

	postLogoutRedirect := cfg.RedirectURL
	if idx := strings.Index(postLogoutRedirect, "/auth/"); idx > 0 {
		postLogoutRedirect = postLogoutRedirect[:idx]
	}

	key, err := newEncryptionKey(cfg.SessionSecret)
	if err != nil {
		return nil, err
	}

	// Parse group filter pattern if provided
	var groupFilterConfig GroupFilterConfig
	groupFilterConfig.AllowedPrefix = cfg.AllowedGroupPrefix
	if cfg.AllowedGroupPattern != "" {
		pattern, err := regexp.Compile(cfg.AllowedGroupPattern)
		if err != nil {
			return nil, fmt.Errorf("invalid group pattern regex: %w", err)
		}
		groupFilterConfig.AllowedPattern = pattern
	}

	return &OIDCAuth{
		baseAuth:           baseAuth{encryptionKey: key, logger: logger},
		provider:           provider,
		oauth2Config:       oauth2Cfg,
		verifier:           verifier,
		httpClient:         httpClient,
		endSessionURL:      endSessionURL,
		postLogoutRedirect: postLogoutRedirect,
		groupFilterConfig:  groupFilterConfig,
	}, nil
}

func (o *OIDCAuth) AuthMiddleware(next http.Handler) http.Handler {
	return o.authMiddleware(next)
}

func (o *OIDCAuth) HandleLogin(w http.ResponseWriter, r *http.Request) {
	state, err := o.setStateCookie(w, r)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, o.oauth2Config.AuthCodeURL(state), http.StatusFound)
}

func (o *OIDCAuth) HandleCallback(w http.ResponseWriter, r *http.Request) {
	// Prevent proxies from caching this response (it contains Set-Cookie headers)
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")

	if err := o.validateStateCookie(r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	o.clearStateCookie(w)

	exchangeCtx := oidc.ClientContext(r.Context(), o.httpClient)
	oauth2Token, err := o.oauth2Config.Exchange(exchangeCtx, r.URL.Query().Get("code"))
	if err != nil {
		o.logger.Error(err, "failed to exchange code for token")
		http.Error(w, "failed to exchange token", http.StatusInternalServerError)
		return
	}

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

	var claims struct {
		Email  string   `json:"email"`
		Name   string   `json:"name"`
		Groups []string `json:"groups,omitempty"`
	}
	if err := idToken.Claims(&claims); err != nil {
		o.logger.Error(err, "failed to parse claims")
		http.Error(w, "failed to parse claims", http.StatusInternalServerError)
		return
	}

	// Log received groups for debugging
	o.logger.V(1).Info("OIDC claims received",
		"user", claims.Email,
		"name", claims.Name,
		"groups", claims.Groups)

	// Apply 3-layer group validation
	validatedGroups := ValidateAndNormalizeGroups(claims.Groups, o.groupFilterConfig, o.logger)

	if len(claims.Groups) > 0 && len(validatedGroups) == 0 {
		o.logger.Info("WARNING: User authenticated but all groups filtered out",
			"user", claims.Email,
			"original_groups", claims.Groups)
	}

	completeURL, err := o.buildCompleteURL(claims.Email, claims.Name, func(s *sessionData) {
		s.Groups = validatedGroups
	})
	if err != nil {
		o.logger.Error(err, "failed to build complete URL")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, completeURL, http.StatusFound)
}

func (o *OIDCAuth) HandleComplete(w http.ResponseWriter, r *http.Request) {
	o.handleComplete(w, r)
}

func (o *OIDCAuth) HandleLogout(w http.ResponseWriter, r *http.Request) {
	o.clearSessionCookie(w)

	if o.endSessionURL != "" {
		logoutURL := o.endSessionURL + "?client_id=" + o.oauth2Config.ClientID
		if o.postLogoutRedirect != "" {
			logoutURL += "&post_logout_redirect_uri=" + o.postLogoutRedirect
		}
		http.Redirect(w, r, logoutURL, http.StatusFound)
		return
	}

	http.Redirect(w, r, "/auth/logged-out", http.StatusFound)
}

func (o *OIDCAuth) HandleAuthStatus(w http.ResponseWriter, r *http.Request) {
	o.handleAuthStatus(w, r)
}

func (o *OIDCAuth) SupportsGroups() bool {
	return true
}

// buildOIDCScopes returns the base OIDC scopes plus any additional scopes, deduplicated.
func buildOIDCScopes(additionalScopes []string) []string {
	seen := map[string]struct{}{
		oidc.ScopeOpenID: {},
		"email":          {},
		"profile":        {},
	}
	scopes := []string{oidc.ScopeOpenID, "email", "profile"}
	for _, s := range additionalScopes {
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
			scopes = append(scopes, s)
		}
	}
	return scopes
}

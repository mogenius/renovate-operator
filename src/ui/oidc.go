package ui

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"regexp"
	"renovate-operator/internal/telemetry"
	"renovate-operator/metricStore"
	"slices"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-logr/logr"
	"golang.org/x/oauth2"
)

const pkceCookieName = "pkce_verifier"

type OIDCConfig struct {
	IssuerURL           string
	ClientID            string
	ClientSecret        string
	RedirectURL         string
	InsecureSkipVerify  bool
	CACertPath          string
	LogoutURL           string
	AllowedGroupPrefix  string
	AllowedGroupPattern string
	AdditionalScopes    []string
	FetchUserInfoGroups bool
	PKCEEnabled         bool
}

type OIDCAuth struct {
	baseAuth
	provider            *oidc.Provider
	oauth2Config        oauth2.Config
	verifier            *oidc.IDTokenVerifier
	httpClient          *http.Client
	endSessionURL       string
	postLogoutRedirect  string
	groupFilterConfig   GroupFilterConfig
	fetchUserInfoGroups bool
	pkceEnabled         bool
}

func NewOIDCAuth(ctx context.Context, cfg OIDCConfig, encryptionKey [32]byte, logger logr.Logger, sessionStore SessionStore) (*OIDCAuth, error) {
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
	}
	if cfg.InsecureSkipVerify {
		logger.Info("WARNING: OIDC TLS verification is disabled. Do not use this in production!")
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	} else if cfg.CACertPath != "" {
		pemData, err := os.ReadFile(cfg.CACertPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read OIDC CA cert %q: %w", cfg.CACertPath, err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pemData) {
			return nil, fmt.Errorf("no valid PEM certificates found in %q", cfg.CACertPath)
		}
		transport.TLSClientConfig = &tls.Config{RootCAs: pool}
		logger.Info("Using custom CA certificate for OIDC TLS", "path", cfg.CACertPath)
	}
	// Reflect the actual TLS posture in the SecOps gauge (CISO target = 0).
	metricStore.SetOIDCTLSVerificationDisabled(cfg.InsecureSkipVerify)
	httpClient := &http.Client{Transport: telemetry.WrapTransport(transport)}

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
		hasGroupsScope := slices.Contains(scopes, "groups")
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

	base, err := newBaseAuth(encryptionKey, logger, sessionStore)
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

	if cfg.FetchUserInfoGroups {
		logger.Info("OIDC userinfo group fetching enabled -- groups will be fetched from both ID token and userinfo endpoint; userinfo failures will block login")
	}

	return &OIDCAuth{
		baseAuth:            base,
		provider:            provider,
		oauth2Config:        oauth2Cfg,
		verifier:            verifier,
		httpClient:          httpClient,
		endSessionURL:       endSessionURL,
		postLogoutRedirect:  postLogoutRedirect,
		groupFilterConfig:   groupFilterConfig,
		fetchUserInfoGroups: cfg.FetchUserInfoGroups,
		pkceEnabled:         cfg.PKCEEnabled,
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
	var authOpts []oauth2.AuthCodeOption
	if o.pkceEnabled {
		verifier := oauth2.GenerateVerifier()
		http.SetCookie(w, &http.Cookie{
			Name:     pkceCookieName,
			Value:    verifier,
			Path:     "/",
			MaxAge:   300,
			HttpOnly: true,
			Secure:   isHTTPS(r),
			SameSite: http.SameSiteLaxMode,
		})
		authOpts = append(authOpts, oauth2.S256ChallengeOption(verifier))
	}
	http.Redirect(w, r, o.oauth2Config.AuthCodeURL(state, authOpts...), http.StatusFound)
}

func (o *OIDCAuth) HandleCallback(w http.ResponseWriter, r *http.Request) {
	// Prevent proxies from caching this response (it contains Set-Cookie headers)
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")

	ctx := r.Context()

	if err := o.validateStateCookie(r); err != nil {
		metricStore.IncAuthStateValidationFailure(ctx, "oidc")
		metricStore.IncUIAuthAttempt(ctx, "oidc", "failure")
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	o.clearStateCookie(w)

	exchangeCtx := oidc.ClientContext(r.Context(), o.httpClient)
	var exchangeOpts []oauth2.AuthCodeOption
	if o.pkceEnabled {
		verifierCookie, err := r.Cookie(pkceCookieName)
		if err != nil {
			o.logger.Error(err, "missing PKCE verifier cookie")
			metricStore.IncUIAuthAttempt(ctx, "oidc", "failure")
			http.Error(w, "invalid PKCE state", http.StatusBadRequest)
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:     pkceCookieName,
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
		})
		exchangeOpts = append(exchangeOpts, oauth2.VerifierOption(verifierCookie.Value))
	}
	oauth2Token, err := o.oauth2Config.Exchange(exchangeCtx, r.URL.Query().Get("code"), exchangeOpts...)
	if err != nil {
		o.logger.Error(err, "failed to exchange code for token")
		metricStore.IncOAuthTokenExchangeFailure(ctx, "oidc", classifyOAuthExchangeError(err))
		metricStore.IncUIAuthAttempt(ctx, "oidc", "failure")
		http.Error(w, "failed to exchange token", http.StatusInternalServerError)
		return
	}

	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		metricStore.IncOIDCTokenVerificationFailure(ctx, "claims")
		metricStore.IncUIAuthAttempt(ctx, "oidc", "failure")
		http.Error(w, "no id_token in response", http.StatusInternalServerError)
		return
	}

	idToken, err := o.verifier.Verify(exchangeCtx, rawIDToken)
	if err != nil {
		o.logger.Error(err, "failed to verify ID token")
		metricStore.IncOIDCTokenVerificationFailure(ctx, classifyOIDCVerificationError(err))
		metricStore.IncUIAuthAttempt(ctx, "oidc", "failure")
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
		metricStore.IncOIDCTokenVerificationFailure(ctx, "claims")
		metricStore.IncUIAuthAttempt(ctx, "oidc", "failure")
		http.Error(w, "failed to parse claims", http.StatusInternalServerError)
		return
	}

	// Log received groups for debugging
	o.logger.V(1).Info("OIDC claims received",
		"user", claims.Email,
		"name", claims.Name,
		"groups", claims.Groups)

	idTokenGroups := claims.Groups

	if o.fetchUserInfoGroups {
		o.logger.V(1).Info("fetching groups from userinfo endpoint", "user", claims.Email)
		userInfoCtx, cancel := context.WithTimeout(exchangeCtx, 15*time.Second)
		defer cancel()
		userInfo, err := o.provider.UserInfo(userInfoCtx, oauth2.StaticTokenSource(oauth2Token))
		if err != nil {
			o.logger.Error(err, "failed to fetch userinfo")
			metricStore.IncUIAuthAttempt(ctx, "oidc", "failure")
			http.Error(w, "failed to fetch userinfo", http.StatusInternalServerError)
			return
		}
		var userInfoClaims struct {
			Groups []string `json:"groups,omitempty"`
		}
		if err := userInfo.Claims(&userInfoClaims); err != nil {
			o.logger.Error(err, "failed to parse userinfo claims")
			metricStore.IncOIDCTokenVerificationFailure(ctx, "claims")
			metricStore.IncUIAuthAttempt(ctx, "oidc", "failure")
			http.Error(w, "failed to parse userinfo claims", http.StatusInternalServerError)
			return
		}
		o.logger.V(1).Info("userinfo groups received",
			"user", claims.Email,
			"id_token_groups", claims.Groups,
			"userinfo_groups", userInfoClaims.Groups)
		claims.Groups = mergeGroups(claims.Groups, userInfoClaims.Groups)
	}

	// Apply 3-layer group validation
	validatedGroups := ValidateAndNormalizeGroups(claims.Groups, o.groupFilterConfig, o.logger)

	// Record the authorization outcome and group-filtering posture. A group
	// filter is only consequential when one is configured; without a filter
	// every authenticated user is allowed.
	groupFilterConfigured := o.groupFilterConfig.AllowedPrefix != "" || o.groupFilterConfig.AllowedPattern != nil
	if groupFilterConfigured && len(validatedGroups) == 0 {
		if len(claims.Groups) > 0 {
			// User presented groups but none survived the allowlist filter.
			metricStore.IncAuthzGroupsFiltered(ctx, "empty_after_filter")
			o.logger.Info("WARNING: User authenticated but all groups filtered out",
				"user", claims.Email,
				"id_token_groups", idTokenGroups,
				"groups_before_validation", claims.Groups)
		} else {
			// User presented no groups at all to match against the allowlist.
			metricStore.IncAuthzGroupsFiltered(ctx, "not_in_allowlist")
		}
		metricStore.IncAuthzDecision(ctx, "denied")
	} else {
		metricStore.IncAuthzDecision(ctx, "allowed")
	}

	completeURL, err := o.buildCompleteURL(r.Context(), claims.Email, claims.Name, func(s *sessionData) {
		s.Groups = validatedGroups
	})
	if err != nil {
		o.logger.Error(err, "failed to build complete URL")
		metricStore.IncUIAuthAttempt(ctx, "oidc", "failure")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	metricStore.IncUIAuthAttempt(ctx, "oidc", "success")
	http.Redirect(w, r, completeURL, http.StatusFound)
}

func (o *OIDCAuth) HandleComplete(w http.ResponseWriter, r *http.Request) {
	o.handleComplete(w, r)
}

func (o *OIDCAuth) HandleLogout(w http.ResponseWriter, r *http.Request) {
	o.deleteSession(r)
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

// mergeGroups combines groups from the ID token and userinfo endpoint,
// deduplicating entries so that the merged result doesn't inflate the
// count before the downstream maxGroupsPerUser truncation.
func mergeGroups(idTokenGroups, userInfoGroups []string) []string {
	seen := make(map[string]struct{}, len(idTokenGroups))
	merged := make([]string, 0, len(idTokenGroups)+len(userInfoGroups))
	for _, g := range idTokenGroups {
		seen[g] = struct{}{}
		merged = append(merged, g)
	}
	for _, g := range userInfoGroups {
		if _, ok := seen[g]; !ok {
			seen[g] = struct{}{}
			merged = append(merged, g)
		}
	}
	return merged
}

// classifyOAuthExchangeError maps an OAuth2 code->token exchange error to one
// of the bounded SecOps reason enum values: "invalid_code", "timeout" or
// "network" (default). It never inspects user-controlled values, only error
// types/shapes, so it is safe to use as a metric label.
func classifyOAuthExchangeError(err error) string {
	if err == nil {
		return "network"
	}
	// The provider returned an OAuth2 error response (e.g. invalid_grant),
	// which means the authorization code itself was rejected.
	var retrieveErr *oauth2.RetrieveError
	if errors.As(err, &retrieveErr) {
		return "invalid_code"
	}
	// Timeouts: context deadline or any net.Error reporting a timeout.
	if errors.Is(err, context.DeadlineExceeded) || os.IsTimeout(err) {
		return "timeout"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "timeout"
	}
	return "network"
}

// classifyOIDCVerificationError maps an ID-token verification error to one of
// the bounded SecOps reason enum values: "expired", "claims" or "signature"
// (default). Classification is based on the error text shape only; no token
// contents or user identifiers are used as label values.
func classifyOIDCVerificationError(err error) string {
	if err == nil {
		return "signature"
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "expired") || strings.Contains(msg, "expiry"):
		return "expired"
	case strings.Contains(msg, "signature"):
		return "signature"
	case strings.Contains(msg, "claim") || strings.Contains(msg, "audience") ||
		strings.Contains(msg, "issuer") || strings.Contains(msg, "nonce") ||
		strings.Contains(msg, "subject"):
		return "claims"
	default:
		return "signature"
	}
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

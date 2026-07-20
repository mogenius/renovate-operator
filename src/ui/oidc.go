package ui

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"regexp"
	"renovate-operator/internal/telemetry"
	"slices"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-logr/logr"
	"golang.org/x/oauth2"
)

const pkceCookieName = "pkce_verifier"

// defaultGroupsClaim is the OIDC claim that group memberships are read from when
// OIDC_GROUPS_CLAIM is not set. Providers that emit groups under a namespaced
// claim (e.g. "https://example.com/groups") can override it.
const defaultGroupsClaim = "groups"

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
	GroupsClaim         string
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
	groupsClaim         string
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

	groupsClaim := cfg.GroupsClaim
	if groupsClaim == "" {
		groupsClaim = defaultGroupsClaim
	}
	if groupsClaim != defaultGroupsClaim {
		logger.Info("OIDC groups claim name overridden", "claim", groupsClaim)
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
		groupsClaim:         groupsClaim,
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
			Path:     cookiePath(),
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

	if err := o.validateStateCookie(r); err != nil {
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
			http.Error(w, "invalid PKCE state", http.StatusBadRequest)
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:     pkceCookieName,
			Value:    "",
			Path:     cookiePath(),
			MaxAge:   -1,
			HttpOnly: true,
		})
		exchangeOpts = append(exchangeOpts, oauth2.VerifierOption(verifierCookie.Value))
	}
	oauth2Token, err := o.oauth2Config.Exchange(exchangeCtx, r.URL.Query().Get("code"), exchangeOpts...)
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
		Groups []string `json:"-"` // read from the configurable claim below
	}
	if err := idToken.Claims(&claims); err != nil {
		o.logger.Error(err, "failed to parse claims")
		http.Error(w, "failed to parse claims", http.StatusInternalServerError)
		return
	}
	claims.Groups, err = extractGroupsClaim(idToken, o.groupsClaim)
	if err != nil {
		o.logger.Error(err, "failed to parse group claim")
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
			http.Error(w, "failed to fetch userinfo", http.StatusInternalServerError)
			return
		}
		userInfoGroups, err := extractGroupsClaim(userInfo, o.groupsClaim)
		if err != nil {
			o.logger.Error(err, "failed to parse userinfo claims")
			http.Error(w, "failed to parse userinfo claims", http.StatusInternalServerError)
			return
		}
		o.logger.V(1).Info("userinfo groups received",
			"user", claims.Email,
			"id_token_groups", claims.Groups,
			"userinfo_groups", userInfoGroups)
		claims.Groups = mergeGroups(claims.Groups, userInfoGroups)
	}

	// Apply 3-layer group validation
	validatedGroups := ValidateAndNormalizeGroups(claims.Groups, o.groupFilterConfig, o.logger)

	filterActive := o.groupFilterConfig.AllowedPrefix != "" || o.groupFilterConfig.AllowedPattern != nil
	if filterActive && len(validatedGroups) == 0 {
		o.logger.Info("Access denied: no groups matching configured filter",
			"user", claims.Email,
			"id_token_groups", idTokenGroups,
			"groups_before_validation", claims.Groups)
		http.Redirect(w, r, withBase("/auth/unauthorized"), http.StatusFound)
		return
	}

	completeURL, err := o.buildCompleteURL(r.Context(), claims.Email, claims.Name, func(s *sessionData) {
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

	http.Redirect(w, r, withBase("/auth/logged-out"), http.StatusFound)
}

func (o *OIDCAuth) HandleAuthStatus(w http.ResponseWriter, r *http.Request) {
	o.handleAuthStatus(w, r)
}

func (o *OIDCAuth) SupportsGroups() bool {
	return true
}

// claimReader is implemented by *oidc.IDToken and *oidc.UserInfo; both expose
// the raw claim set via Claims(v).
type claimReader interface {
	Claims(v any) error
}

// extractGroupsClaim reads group memberships from an OIDC claim set using the
// configured claim name (see defaultGroupsClaim). It tolerates the claim being
// absent, a JSON array of strings, or a single string.
func extractGroupsClaim(r claimReader, claimName string) ([]string, error) {
	var raw map[string]json.RawMessage
	if err := r.Claims(&raw); err != nil {
		return nil, err
	}
	return parseGroupsClaim(raw[claimName]), nil
}

// parseGroupsClaim decodes a single OIDC claim value into a list of groups,
// accepting either a JSON array of strings or a single string; anything else
// (absent, null, empty, or a non-string type) yields no groups.
func parseGroupsClaim(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var groups []string
	if err := json.Unmarshal(raw, &groups); err == nil {
		return groups
	}
	var single string
	if err := json.Unmarshal(raw, &single); err == nil && single != "" {
		return []string{single}
	}
	return nil
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

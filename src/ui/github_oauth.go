package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
)

type GitHubOAuthConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

type GitHubOAuth struct {
	baseAuth
	oauth2Config oauth2.Config
	httpClient   *http.Client
}

func NewGitHubOAuth(cfg GitHubOAuthConfig, encryptionKey [32]byte, logger logr.Logger, sessionStore SessionStore) (*GitHubOAuth, error) {
	oauth2Cfg := oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Endpoint:     github.Endpoint,
		Scopes:       []string{"read:user", "user:email"},
	}

	base, err := newBaseAuth(encryptionKey, logger, sessionStore)
	if err != nil {
		return nil, err
	}

	return &GitHubOAuth{
		baseAuth:     base,
		oauth2Config: oauth2Cfg,
		httpClient:   &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)},
	}, nil
}

func (g *GitHubOAuth) AuthMiddleware(next http.Handler) http.Handler {
	return g.authMiddleware(next)
}

func (g *GitHubOAuth) HandleLogin(w http.ResponseWriter, r *http.Request) {
	g.logger.Info("login initiated", "remoteAddr", r.RemoteAddr)
	state, err := g.setStateCookie(w, r)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	authURL := g.oauth2Config.AuthCodeURL(state)
	g.logger.Info("redirecting to GitHub", "url", authURL)
	http.Redirect(w, r, authURL, http.StatusFound)
}

func (g *GitHubOAuth) HandleCallback(w http.ResponseWriter, r *http.Request) {
	// Prevent proxies from caching this response (it contains Set-Cookie headers)
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")

	g.logger.Info("callback received", "hasCode", r.URL.Query().Get("code") != "", "hasState", r.URL.Query().Get("state") != "")

	if err := g.validateStateCookie(r); err != nil {
		g.logger.Error(err, "state cookie validation failed")
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	g.clearStateCookie(w)

	oauth2Token, err := g.oauth2Config.Exchange(r.Context(), r.URL.Query().Get("code"))
	if err != nil {
		g.logger.Error(err, "failed to exchange code for token")
		http.Error(w, "failed to exchange token", http.StatusInternalServerError)
		return
	}
	g.logger.Info("token exchange successful")

	// Fetch user info from GitHub API
	email, name, err := g.fetchGitHubUser(oauth2Token.AccessToken)
	if err != nil {
		g.logger.Error(err, "failed to fetch GitHub user info")
		http.Error(w, "failed to fetch user info", http.StatusInternalServerError)
		return
	}
	g.logger.Info("user info fetched", "email", email, "name", name)

	// Redirect to /auth/complete with the encrypted session token.
	// The cookie is set there, not here, because some reverse proxies strip
	// Set-Cookie headers from OAuth callback responses.
	completeURL, err := g.buildCompleteURL(r.Context(), email, name, func(s *sessionData) {
		s.AccessToken = oauth2Token.AccessToken
	})
	if err != nil {
		g.logger.Error(err, "failed to build complete URL")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, completeURL, http.StatusFound)
}

func (g *GitHubOAuth) HandleComplete(w http.ResponseWriter, r *http.Request) {
	g.handleComplete(w, r)
}

func (g *GitHubOAuth) HandleLogout(w http.ResponseWriter, r *http.Request) {
	if session, err := g.getSession(r); err == nil && session.AccessToken != "" {
		g.revokeGitHubToken(session.AccessToken)
	}
	g.deleteSession(r)
	g.clearSessionCookie(w)
	http.Redirect(w, r, "/auth/logged-out", http.StatusFound)
}

func (g *GitHubOAuth) revokeGitHubToken(accessToken string) {
	url := fmt.Sprintf("https://api.github.com/applications/%s/token", g.oauth2Config.ClientID)
	body := fmt.Sprintf(`{"access_token":"%s"}`, accessToken)
	req, err := http.NewRequest("DELETE", url, strings.NewReader(body))
	if err != nil {
		g.logger.Error(err, "failed to create token revocation request")
		return
	}
	req.SetBasicAuth(g.oauth2Config.ClientID, g.oauth2Config.ClientSecret)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		g.logger.Error(err, "failed to revoke GitHub token")
		return
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			g.logger.Error(err, "failed to close response body")
		}
	}()

	if resp.StatusCode == http.StatusNoContent {
		g.logger.Info("GitHub OAuth token revoked successfully")
	} else {
		g.logger.Info("GitHub token revocation returned unexpected status", "status", resp.StatusCode)
	}
}

func (g *GitHubOAuth) HandleAuthStatus(w http.ResponseWriter, r *http.Request) {
	g.handleAuthStatus(w, r)
}

func (g *GitHubOAuth) SupportsGroups() bool {
	return false
}

func (g *GitHubOAuth) fetchGitHubUser(accessToken string) (email, name string, err error) {
	req, err := http.NewRequest("GET", "https://api.github.com/user", nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			g.logger.Error(err, "failed to close response body")
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var user struct {
		Login string `json:"login"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return "", "", err
	}

	name = user.Name
	if name == "" {
		name = user.Login
	}
	email = user.Email
	if email == "" {
		// Email might be private, try the emails endpoint
		email, _ = g.fetchPrimaryEmail(accessToken)
	}
	if email == "" {
		email = user.Login + "@github"
	}

	return email, name, nil
}

func (g *GitHubOAuth) fetchPrimaryEmail(accessToken string) (string, error) {
	req, err := http.NewRequest("GET", "https://api.github.com/user/emails", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			g.logger.Error(err, "failed to close response body")
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub emails API returned status %d", resp.StatusCode)
	}

	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&emails); err != nil {
		return "", err
	}

	for _, e := range emails {
		if e.Primary && e.Verified {
			return e.Email, nil
		}
	}
	for _, e := range emails {
		if e.Verified {
			return e.Email, nil
		}
	}

	return "", fmt.Errorf("no verified email found")
}

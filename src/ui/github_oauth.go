package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"renovate-operator/internal/telemetry"
	"strings"

	"github.com/go-logr/logr"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
)

type GitHubOAuthConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	OrgGroups    bool
}

type GitHubOAuth struct {
	baseAuth
	oauth2Config oauth2.Config
	httpClient   *http.Client
	orgGroups    bool
}

func NewGitHubOAuth(cfg GitHubOAuthConfig, encryptionKey [32]byte, logger logr.Logger, sessionStore SessionStore) (*GitHubOAuth, error) {
	scopes := []string{"read:user", "user:email"}
	if cfg.OrgGroups {
		scopes = append(scopes, "read:org")
	}

	oauth2Cfg := oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Endpoint:     github.Endpoint,
		Scopes:       scopes,
	}

	base, err := newBaseAuth(encryptionKey, logger, sessionStore)
	if err != nil {
		return nil, err
	}

	return &GitHubOAuth{
		baseAuth:     base,
		oauth2Config: oauth2Cfg,
		httpClient:   &http.Client{Transport: telemetry.WrapTransport(http.DefaultTransport)},
		orgGroups:    cfg.OrgGroups,
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
	email, name, login, err := g.fetchGitHubUser(oauth2Token.AccessToken)
	if err != nil {
		g.logger.Error(err, "failed to fetch GitHub user info")
		http.Error(w, "failed to fetch user info", http.StatusInternalServerError)
		return
	}
	g.logger.Info("user info fetched", "email", email, "name", name, "login", login)

	var groups []string
	if g.orgGroups {
		fetched, groupsErr := g.fetchGitHubGroups(oauth2Token.AccessToken)
		if groupsErr != nil {
			// Half an authorization set is worse than none: the session would
			// live for 24h quietly missing whatever the failed call would have
			// granted, with nothing to distinguish it from legitimately having
			// no access.
			g.logger.Error(groupsErr, "failed to fetch GitHub group membership, refusing to create a session with incomplete authorization",
				"email", email, "login", login)
			http.Error(w, "could not determine your organization and team membership, so no session was created. "+
				"If your organization restricts OAuth apps, it has to approve this app for the read:org scope.",
				http.StatusForbidden)
			return
		}
		groups = ValidateAndNormalizeGroups(fetched, GroupFilterConfig{}, g.logger)
		g.logger.V(1).Info("GitHub groups fetched", "email", email, "groups", groups)
	}

	// Redirect to /auth/complete with the encrypted session token.
	// The cookie is set there, not here, because some reverse proxies strip
	// Set-Cookie headers from OAuth callback responses.
	completeURL, err := g.buildCompleteURL(r.Context(), email, name, func(s *sessionData) {
		s.AccessToken = oauth2Token.AccessToken
		s.Groups = groups
		s.Username = login
		s.EmailVerified = true
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
	http.Redirect(w, r, withBase("/auth/logged-out"), http.StatusFound)
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
	return g.orgGroups
}

func (g *GitHubOAuth) fetchGitHubGroups(accessToken string) ([]string, error) {
	groups := make([]string, 0, 8)

	type githubOrg struct {
		Login string `json:"login"`
	}
	orgs, err := fetchAllPages[githubOrg](g, accessToken, "https://api.github.com/user/orgs?per_page=100")
	if err != nil {
		return nil, fmt.Errorf("fetching organizations: %w", err)
	}
	for _, org := range orgs {
		groups = append(groups, org.Login)
	}

	type githubTeam struct {
		Slug         string `json:"slug"`
		Organization struct {
			Login string `json:"login"`
		} `json:"organization"`
	}
	teams, err := fetchAllPages[githubTeam](g, accessToken, "https://api.github.com/user/teams?per_page=100")
	if err != nil {
		return nil, fmt.Errorf("fetching teams: %w", err)
	}
	for _, team := range teams {
		if team.Organization.Login == "" || team.Slug == "" {
			continue
		}
		groups = append(groups, team.Organization.Login+"/"+team.Slug)
	}

	return groups, nil
}

// maxGitHubPages bounds pagination so one account cannot make a single login
// issue an unbounded number of API calls. At 100 per page this reaches well past
// the group cap sanitizeGroups applies anyway.
const maxGitHubPages = 20

// fetchAllPages follows GitHub's Link-header pagination and returns every item.
func fetchAllPages[T any](g *GitHubOAuth, accessToken, url string) ([]T, error) {
	var all []T
	for page := 0; url != ""; page++ {
		if page >= maxGitHubPages {
			return nil, fmt.Errorf("more than %d pages of results, refusing to continue", maxGitHubPages)
		}

		var batch []T
		next, err := g.getJSON(accessToken, url, &batch)
		if err != nil {
			return nil, err
		}
		all = append(all, batch...)
		url = next
	}
	return all, nil
}

func (g *GitHubOAuth) getJSON(accessToken, url string, target any) (nextURL string, err error) {
	req, err := http.NewRequest("GET", url, nil)
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
		return "", fmt.Errorf("GitHub API %s returned status %d", url, resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return "", err
	}

	return nextPageURL(resp.Header.Get("Link")), nil
}

// nextPageURL extracts the rel="next" target from an RFC 8288 Link header as
// GitHub sends it, or "" when there is no next page.
func nextPageURL(link string) string {
	for entry := range strings.SplitSeq(link, ",") {
		parts := strings.Split(entry, ";")
		if len(parts) < 2 {
			continue
		}
		target := strings.TrimSpace(parts[0])
		if !strings.HasPrefix(target, "<") || !strings.HasSuffix(target, ">") {
			continue
		}
		for _, param := range parts[1:] {
			// The quoted form is what GitHub sends; accept both spellings.
			switch strings.TrimSpace(param) {
			case `rel="next"`, "rel=next":
				return strings.TrimSuffix(strings.TrimPrefix(target, "<"), ">")
			}
		}
	}
	return ""
}

func (g *GitHubOAuth) fetchGitHubUser(accessToken string) (email, name, login string, err error) {
	req, err := http.NewRequest("GET", "https://api.github.com/user", nil)
	if err != nil {
		return "", "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return "", "", "", err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			g.logger.Error(err, "failed to close response body")
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return "", "", "", fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var user struct {
		Login string `json:"login"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return "", "", "", err
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

	return email, name, user.Login, nil
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

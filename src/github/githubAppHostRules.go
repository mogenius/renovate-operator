package github

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-logr/logr"
)

// HostRulesConfigFileKey is the key of the generated hostRules Secret that holds
// the Renovate additional config file. Renovate picks the parser by extension.
const HostRulesConfigFileKey = "host-rules.json"

// InstallationTokenMinter mints installation access tokens of a GitHub App.
type InstallationTokenMinter interface {
	MintInstallationToken(appID, installationID, pem, githubApi string) (token string, expiresAt time.Time, err error)
}

// NewInstallationTokenMinter returns a minter that calls the GitHub API with
// httpClient.
func NewInstallationTokenMinter(httpClient *http.Client) InstallationTokenMinter {
	return &githubappToken{httpClient: httpClient, logger: logr.Discard()}
}

func (g *githubappToken) MintInstallationToken(appID, installationID, pemString, githubApi string) (string, time.Time, error) {
	return g.createGithubAppTokenDetailed(appID, installationID, pemString, githubApi)
}

// InstallationToken is a minted token of the installation on Account.
type InstallationToken struct {
	Account string
	Token   string
}

type hostRule struct {
	MatchHost string `json:"matchHost"`
	HostType  string `json:"hostType"`
	Token     string `json:"token"`
}

type hostRulesConfig struct {
	HostRules []hostRule `json:"hostRules"`
}

// BuildHostRulesConfig renders a Renovate config holding one hostRule per
// token, each scoped to the repositories of its account on githubApi.
// Renovate prefers the longest matching matchHost, so "<api>/repos/<account>/"
// lets every account's token apply only to that account's repositories while
// all of them share the API host.
func BuildHostRulesConfig(githubApi string, tokens []InstallationToken) ([]byte, error) {
	base := strings.TrimRight(githubApi, "/")
	cfg := hostRulesConfig{HostRules: make([]hostRule, 0, len(tokens))}
	for _, t := range tokens {
		cfg.HostRules = append(cfg.HostRules, hostRule{
			MatchHost: base + "/repos/" + t.Account + "/",
			HostType:  "github",
			Token:     t.Token,
		})
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal host rules: %w", err)
	}
	return data, nil
}

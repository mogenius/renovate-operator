package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// installationsPerPage is the GitHub API's maximum page size.
const installationsPerPage = 100

// AppInstallation is one installation of a GitHub App.
type AppInstallation struct {
	ID          int64
	Account     string
	AccountType string
	Suspended   bool
}

// InstallationLister lists every installation of a GitHub App.
type InstallationLister interface {
	ListInstallations(ctx context.Context, appID, pem, githubApi string) ([]AppInstallation, error)
}

type installationLister struct {
	httpClient *http.Client
}

func NewInstallationLister(httpClient *http.Client) InstallationLister {
	return &installationLister{httpClient: httpClient}
}

type installationResponse struct {
	ID      int64 `json:"id"`
	Account struct {
		Login string `json:"login"`
		Type  string `json:"type"`
	} `json:"account"`
	SuspendedAt *time.Time `json:"suspended_at"`
}

func (l *installationLister) ListInstallations(ctx context.Context, appID, pemString, githubApi string) ([]AppInstallation, error) {
	signedJWT, err := signAppJWT(appID, pemString)
	if err != nil {
		return nil, err
	}

	var result []AppInstallation
	for page := 1; ; page++ {
		items, err := l.listPage(ctx, signedJWT, githubApi, page)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			// Only user and organization accounts carry a login. An enterprise
			// account has a slug instead, and an installation on it grants no
			// repository access.
			if item.Account.Login == "" {
				continue
			}
			result = append(result, AppInstallation{
				ID:          item.ID,
				Account:     item.Account.Login,
				AccountType: item.Account.Type,
				Suspended:   item.SuspendedAt != nil,
			})
		}
		if len(items) < installationsPerPage {
			return result, nil
		}
	}
}

func (l *installationLister) listPage(ctx context.Context, signedJWT, githubApi string, page int) ([]installationResponse, error) {
	url := fmt.Sprintf("%s/app/installations?per_page=%d&page=%d", githubApi, installationsPerPage, page)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create list installations request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+signedJWT)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := l.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to list installations: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read installations response: %w", err)
	}
	if resp.StatusCode > 299 {
		return nil, fmt.Errorf("GitHub error listing installations (status %d): %s", resp.StatusCode, string(body))
	}

	var items []installationResponse
	if err := json.Unmarshal(body, &items); err != nil {
		return nil, fmt.Errorf("failed to decode installations response: %w", err)
	}
	return items, nil
}

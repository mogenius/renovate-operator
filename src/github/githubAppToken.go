package github

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	api "renovate-operator/api/v1alpha1"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type GithubAppToken interface {
	CreateGithubAppTokenFromJob(job *api.RenovateJob) (string, error)
	CreateGithubAppToken(appID, installationID, pem, githubApi string) (string, error)
}
type githubappToken struct {
	client     client.Client
	httpClient *http.Client
}

func NewGitHubAppTokenCreator(client client.Client) *githubappToken {
	return NewGitHubAppTokenCreatorWithHTTPClient(client, http.DefaultClient)
}

func NewGitHubAppTokenCreatorWithHTTPClient(client client.Client, httpClient *http.Client) *githubappToken {
	return &githubappToken{
		client:     client,
		httpClient: httpClient,
	}
}

func (github *githubappToken) CreateGithubAppTokenFromJob(job *api.RenovateJob) (string, error) {
	ctx := context.Background()

	if job.Spec.GithubAppReference == nil {
		return "", fmt.Errorf("github App Reference needs to be defined")
	}

	secretName := job.Spec.GithubAppReference.SecretName
	pemSecretKey := job.Spec.GithubAppReference.PemSecretKey
	appIdSecretKey := job.Spec.GithubAppReference.AppIdSecretKey
	installationIdSecretKey := job.Spec.GithubAppReference.InstallationIdSecretKey

	secret := &corev1.Secret{}
	err := github.client.Get(ctx, types.NamespacedName{
		Name:      secretName,
		Namespace: job.Namespace,
	}, secret)

	if err != nil {
		// todo format?
		return "", err
	}

	pem, exists := secret.Data[pemSecretKey]
	if !exists {
		return "", fmt.Errorf("the given key does not exist in the secret: key=%s, secret=%s", pemSecretKey, secretName)
	}

	appId, exists := secret.Data[appIdSecretKey]
	if !exists {
		return "", fmt.Errorf("the given key does not exist in the secret: key=%s, secret=%s", appIdSecretKey, secretName)
	}

	installId, exists := secret.Data[installationIdSecretKey]
	if !exists {
		return "", fmt.Errorf("the given key does not exist in the secret: key=%s, secret=%s", installationIdSecretKey, secretName)
	}

	githubApi := "https://api.github.com"
	if job.Spec.ExtraEnv != nil {
		for _, env := range job.Spec.ExtraEnv {
			if env.Name == "RENOVATE_ENDPOINT" {
				githubApi = env.Value
				break
			}
		}
	}

	return github.CreateGithubAppToken(string(appId), string(installId), string(pem), githubApi)
}

func (github *githubappToken) CreateGithubAppToken(appID, installationID, pemString, githubApi string) (string, error) {

	// Clean and parse PEM-encoded private key
	// Trim whitespace and ensure proper formatting
	pemString = strings.TrimSpace(pemString)

	// Ensure the PEM string starts with the header
	if !strings.HasPrefix(pemString, "-----BEGIN") {
		return "", fmt.Errorf("PEM data does not start with BEGIN marker (starts with: %s...)", pemString[:min(50, len(pemString))])
	}

	block, _ := pem.Decode([]byte(pemString))
	if block == nil {
		return "", fmt.Errorf("failed to parse PEM block")
	}

	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		// Try PKCS8 format
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return "", fmt.Errorf("failed to parse private key: %w", err)
		}
		var ok bool
		privateKey, ok = key.(*rsa.PrivateKey)
		if !ok {
			return "", fmt.Errorf("not an RSA private key")
		}
	}

	// Create JWT for GitHub App
	jwtToken := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iat": time.Now().Unix() - 60,
		"exp": time.Now().Unix() + (10 * 60),
		"iss": appID,
	})

	signedJWT, err := jwtToken.SignedString(privateKey)
	if err != nil {
		return "", err
	}

	// Exchange JWT for installation token
	url := fmt.Sprintf("%s/app/installations/%s/access_tokens", githubApi, installationID)
	req, _ := http.NewRequest("POST", url, nil)
	req.Header.Set("Authorization", "Bearer "+signedJWT)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := github.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() {
		// do not fail
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode > 299 {
		return "", fmt.Errorf("GitHub error: %s", string(body))
	}

	// Extract token
	type tokenResponse struct {
		Token string `json:"token"`
	}

	var tr tokenResponse
	err = json.Unmarshal(body, &tr)
	if err != nil {
		return "", err
	}

	return tr.Token, nil
}

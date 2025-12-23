package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	api "renovate-operator/api/v1alpha1"
	"time"

	"github.com/golang-jwt/jwt/v5"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type GithubAppToken interface {
	CreateGithubAppTokenFromJob(job *api.RenovateJob) (string, error)
	CreateGithubAppToken(appID, installationID, pem string) (string, error)
}
type githubappToken struct {
	client client.Client
}

func NewGitHubAppTokenCreator(client client.Client) *githubappToken {
	return &githubappToken{
		client: client,
	}
}

func (github *githubappToken) CreateGithubAppTokenFromJob(job *api.RenovateJob) (string, error) {
	ctx := context.Background()

	if job.Spec.GithubAppReference == nil {
		return "", fmt.Errorf("Github App Reference needs to be defined")
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

	return github.CreateGithubAppToken(string(appId), string(installId), string(pem))
}

func (github *githubappToken) CreateGithubAppToken(appID, installationID, pem string) (string, error) {

	// Create JWT for GitHub App
	jwtToken := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iat": time.Now().Unix() - 60,
		"exp": time.Now().Unix() + (10 * 60),
		"iss": appID,
	})

	signedJWT, err := jwtToken.SignedString(pem)
	if err != nil {
		return "", err
	}

	// Exchange JWT for installation token
	url := fmt.Sprintf("https://api.github.com/app/installations/%s/access_tokens", installationID)
	req, _ := http.NewRequest("POST", url, nil)
	req.Header.Set("Authorization", "Bearer "+signedJWT)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

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

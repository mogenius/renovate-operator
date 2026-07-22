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
	"renovate-operator/internal/utils"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/golang-jwt/jwt/v5"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const tokenExpiresAtAnnotation = "renovate-operator.mogenius.com/token-expires-at"

func parsePEMKey(pemStr string) (*rsa.PrivateKey, error) {
	pemStr = strings.TrimSpace(pemStr)
	if !strings.HasPrefix(pemStr, "-----BEGIN") {
		return nil, fmt.Errorf("PEM data does not start with BEGIN marker (starts with: %s...)", pemStr[:min(50, len(pemStr))])
	}
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("failed to parse PEM block")
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		raw, err2 := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err2 != nil {
			return nil, fmt.Errorf("failed to parse private key: %w", err2)
		}
		var ok bool
		key, ok = raw.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("not an RSA private key")
		}
	}
	return key, nil
}

func mintJWT(appID string, key *rsa.PrivateKey) (string, error) {
	if key == nil {
		return "", fmt.Errorf("private key must not be nil")
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iat": time.Now().Unix() - 60,
		"exp": time.Now().Unix() + (10 * 60),
		"iss": appID,
	})
	return tok.SignedString(key)
}

type GithubAppToken interface {
	// EnsureToken creates or renews the GitHub App token secret for the job when
	// the existing token has less than 30 minutes remaining. The secret is owned
	// by the RenovateJob so Kubernetes GC cleans it up on deletion.
	EnsureToken(ctx context.Context, job *api.RenovateJob) error
	CreateGithubAppTokenFromJob(job *api.RenovateJob) (string, error)
	CreateGithubAppToken(appID, installationID, pem, githubApi string) (string, error)
}

type githubappToken struct {
	client     client.Client
	httpClient *http.Client
	logger     logr.Logger
}

func NewGitHubAppTokenCreator(client client.Client) *githubappToken {
	return NewGitHubAppTokenCreatorWithHTTPClient(client, http.DefaultClient)
}

func NewGitHubAppTokenCreatorWithHTTPClient(client client.Client, httpClient *http.Client) *githubappToken {
	return &githubappToken{
		client:     client,
		httpClient: httpClient,
		logger:     logr.Discard(),
	}
}

func NewGitHubAppTokenCreatorWithLogger(client client.Client, logger logr.Logger) *githubappToken {
	return &githubappToken{
		client:     client,
		httpClient: http.DefaultClient,
		logger:     logger,
	}
}

func (g *githubappToken) EnsureToken(ctx context.Context, job *api.RenovateJob) error {
	if job.Spec.GithubAppReference == nil {
		return nil
	}

	secretName := GetNameForGithubAppSecret(job)
	existing := &corev1.Secret{}
	err := g.client.Get(ctx, client.ObjectKey{Name: secretName, Namespace: job.Namespace}, existing)
	if err == nil {
		if expiresAtStr, ok := existing.Annotations[tokenExpiresAtAnnotation]; ok {
			if expiresAt, parseErr := time.Parse(time.RFC3339, expiresAtStr); parseErr == nil {
				if time.Until(expiresAt) > 30*time.Minute {
					return nil
				}
			}
		}
		g.logger.Info("github app token expiring soon, renewing", "job", job.Fullname())
	} else if !k8serrors.IsNotFound(err) {
		return fmt.Errorf("failed to get github app token secret: %w", err)
	} else {
		g.logger.Info("creating github app token", "job", job.Fullname())
	}

	appID, installID, pemStr, githubApi, err := g.readJobCredentials(ctx, job)
	if err != nil {
		return err
	}

	token, expiresAt, err := g.createGithubAppTokenDetailed(appID, installID, pemStr, githubApi)
	if err != nil {
		return err
	}

	secret := &corev1.Secret{}
	secret.Namespace = job.Namespace
	secret.Name = secretName
	_, err = controllerutil.CreateOrUpdate(ctx, g.client, secret, func() error {
		secret.Data = map[string][]byte{
			"RENOVATE_TOKEN": []byte(token),
		}
		if secret.Annotations == nil {
			secret.Annotations = make(map[string]string)
		}
		secret.Annotations[tokenExpiresAtAnnotation] = expiresAt.Format(time.RFC3339)
		return controllerutil.SetControllerReference(job, secret, g.client.Scheme())
	})

	return err
}

func (g *githubappToken) readJobCredentials(ctx context.Context, job *api.RenovateJob) (appID, installID, pemStr, githubApi string, err error) {
	if job.Spec.GithubAppReference == nil {
		return "", "", "", "", fmt.Errorf("github App Reference needs to be defined")
	}

	ref := job.Spec.GithubAppReference
	secret := &corev1.Secret{}
	if err = g.client.Get(ctx, types.NamespacedName{Name: ref.SecretName, Namespace: job.Namespace}, secret); err != nil {
		return "", "", "", "", fmt.Errorf("failed to get github app secret %s: %w", ref.SecretName, err)
	}

	pemBytes, exists := secret.Data[ref.PemSecretKey]
	if !exists {
		return "", "", "", "", fmt.Errorf("the given key does not exist in the secret: key=%s, secret=%s", ref.PemSecretKey, ref.SecretName)
	}
	appIDBytes, exists := secret.Data[ref.AppIdSecretKey]
	if !exists {
		return "", "", "", "", fmt.Errorf("the given key does not exist in the secret: key=%s, secret=%s", ref.AppIdSecretKey, ref.SecretName)
	}
	installIDBytes, exists := secret.Data[ref.InstallationIdSecretKey]
	if !exists {
		return "", "", "", "", fmt.Errorf("the given key does not exist in the secret: key=%s, secret=%s", ref.InstallationIdSecretKey, ref.SecretName)
	}

	_, githubApi = utils.GetPlatformAndEndpoint(job.Spec.Provider)
	return string(appIDBytes), string(installIDBytes), string(pemBytes), githubApi, nil
}

func (g *githubappToken) CreateGithubAppTokenFromJob(job *api.RenovateJob) (string, error) {
	ctx := context.Background()
	appID, installID, pemStr, githubApi, err := g.readJobCredentials(ctx, job)
	if err != nil {
		return "", err
	}
	token, _, err := g.createGithubAppTokenDetailed(appID, installID, pemStr, githubApi)
	return token, err
}

func (g *githubappToken) CreateGithubAppToken(appID, installationID, pemString, githubApi string) (string, error) {
	token, _, err := g.createGithubAppTokenDetailed(appID, installationID, pemString, githubApi)
	return token, err
}

func (g *githubappToken) createGithubAppTokenDetailed(appID, installationID, pemString, githubApi string) (string, time.Time, error) {
	privateKey, err := parsePEMKey(pemString)
	if err != nil {
		return "", time.Time{}, err
	}

	signedJWT, err := mintJWT(appID, privateKey)
	if err != nil {
		return "", time.Time{}, err
	}

	url := fmt.Sprintf("%s/app/installations/%s/access_tokens", githubApi, installationID)
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to create installation token request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+signedJWT)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return "", time.Time{}, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", time.Time{}, err
	}

	if resp.StatusCode > 299 {
		return "", time.Time{}, fmt.Errorf("GitHub error: %s", string(body))
	}

	type tokenResponse struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}

	var tr tokenResponse
	if err = json.Unmarshal(body, &tr); err != nil {
		return "", time.Time{}, err
	}

	if tr.ExpiresAt.IsZero() {
		tr.ExpiresAt = time.Now().Add(1 * time.Hour)
	}

	return tr.Token, tr.ExpiresAt, nil
}

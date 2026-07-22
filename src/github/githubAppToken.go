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
	"strconv"
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
	EnsureTokensForEnterpriseApp(ctx context.Context, job *api.RenovateJob) ([]string, error)
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

// isTokenFresh returns true if the secret exists and its token has >30 min remaining.
// Returns (false, nil) when missing or expiring. Returns (false, err) on unexpected k8s errors.
func (g *githubappToken) isTokenFresh(ctx context.Context, namespace, secretName string) (bool, error) {
	existing := &corev1.Secret{}
	err := g.client.Get(ctx, client.ObjectKey{Name: secretName, Namespace: namespace}, existing)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to get token secret %s: %w", secretName, err)
	}
	expiresAtStr, ok := existing.Annotations[tokenExpiresAtAnnotation]
	if !ok {
		return false, nil
	}
	expiresAt, err := time.Parse(time.RFC3339, expiresAtStr)
	if err != nil {
		return false, nil
	}
	return time.Until(expiresAt) > 30*time.Minute, nil
}

// readBaseCredentials fetches the Kubernetes Secret referenced by creds and returns the
// appID, PEM string, resolved GitHub API endpoint, and the secret itself (so callers can
// read additional keys without a second Get).
func (g *githubappToken) readBaseCredentials(ctx context.Context, creds api.GithubAppCredentials, namespace string, provider *api.RenovateProvider) (appID, pemStr, githubApi string, secret *corev1.Secret, err error) {
	secret = &corev1.Secret{}
	if err = g.client.Get(ctx, types.NamespacedName{Name: creds.SecretName, Namespace: namespace}, secret); err != nil {
		return "", "", "", nil, fmt.Errorf("failed to get github app secret %s: %w", creds.SecretName, err)
	}
	pemBytes, ok := secret.Data[creds.PemSecretKey]
	if !ok {
		return "", "", "", nil, fmt.Errorf("the given key does not exist in the secret: key=%s, secret=%s", creds.PemSecretKey, creds.SecretName)
	}
	appIDBytes, ok := secret.Data[creds.AppIdSecretKey]
	if !ok {
		return "", "", "", nil, fmt.Errorf("the given key does not exist in the secret: key=%s, secret=%s", creds.AppIdSecretKey, creds.SecretName)
	}
	_, githubApi = utils.GetPlatformAndEndpoint(provider)
	return string(appIDBytes), string(pemBytes), githubApi, secret, nil
}

// upsertTokenSecret creates or updates a Secret holding RENOVATE_TOKEN and its expiry annotation,
// owned by job so Kubernetes GC cleans it up on deletion.
func (g *githubappToken) upsertTokenSecret(ctx context.Context, job *api.RenovateJob, secretName, token string, expiresAt time.Time) error {
	s := &corev1.Secret{}
	s.Namespace = job.Namespace
	s.Name = secretName
	_, err := controllerutil.CreateOrUpdate(ctx, g.client, s, func() error {
		s.Data = map[string][]byte{"RENOVATE_TOKEN": []byte(token)}
		if s.Annotations == nil {
			s.Annotations = make(map[string]string)
		}
		s.Annotations[tokenExpiresAtAnnotation] = expiresAt.Format(time.RFC3339)
		return controllerutil.SetControllerReference(job, s, g.client.Scheme())
	})
	return err
}

func (g *githubappToken) EnsureToken(ctx context.Context, job *api.RenovateJob) error {
	if job.Spec.GithubAppReference == nil {
		return nil
	}

	secretName := GetNameForGithubAppSecret(job)
	fresh, err := g.isTokenFresh(ctx, job.Namespace, secretName)
	if err != nil {
		return err
	}
	if fresh {
		return nil
	}
	g.logger.Info("github app token expiring soon, renewing", "job", job.Fullname())

	appID, installID, pemStr, githubApi, err := g.readJobCredentials(ctx, job)
	if err != nil {
		return err
	}

	token, expiresAt, err := g.createGithubAppTokenDetailed(appID, installID, pemStr, githubApi)
	if err != nil {
		return err
	}

	return g.upsertTokenSecret(ctx, job, secretName, token, expiresAt)
}

func (g *githubappToken) readJobCredentials(ctx context.Context, job *api.RenovateJob) (appID, installID, pemStr, githubApi string, err error) {
	if job.Spec.GithubAppReference == nil {
		return "", "", "", "", fmt.Errorf("github App Reference needs to be defined")
	}
	ref := job.Spec.GithubAppReference
	appID, pemStr, githubApi, secret, err := g.readBaseCredentials(ctx, ref.GithubAppCredentials, job.Namespace, job.Spec.Provider)
	if err != nil {
		return "", "", "", "", err
	}
	installIDBytes, exists := secret.Data[ref.InstallationIdSecretKey]
	if !exists {
		return "", "", "", "", fmt.Errorf("the given key does not exist in the secret: key=%s, secret=%s", ref.InstallationIdSecretKey, ref.SecretName)
	}
	return appID, string(installIDBytes), pemStr, githubApi, nil
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

// doGithubAppRequest executes an authenticated GitHub App API request and returns the response
// body. Returns an error for transport failures or non-2xx status codes.
func (g *githubappToken) doGithubAppRequest(method, url, signedJWT string) ([]byte, error) {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create %s request: %w", method, err)
	}
	req.Header.Set("Authorization", "Bearer "+signedJWT)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode > 299 {
		return nil, fmt.Errorf("GitHub error: %s", string(body))
	}
	return body, nil
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
	return g.createInstallationTokenWithJWT(signedJWT, installationID, githubApi)
}

// createInstallationTokenWithJWT mints an installation access token given an already-signed
// app JWT, letting callers that mint many tokens in a loop (e.g. EnsureTokensForEnterpriseApp)
// reuse a single JWT instead of re-parsing the PEM and re-signing per call.
func (g *githubappToken) createInstallationTokenWithJWT(signedJWT, installationID, githubApi string) (string, time.Time, error) {
	url := fmt.Sprintf("%s/app/installations/%s/access_tokens", githubApi, installationID)
	body, err := g.doGithubAppRequest("POST", url, signedJWT)
	if err != nil {
		return "", time.Time{}, err
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

func (g *githubappToken) listInstallationIDs(appID, pemStr, githubApi string) ([]string, error) {
	privateKey, err := parsePEMKey(pemStr)
	if err != nil {
		return nil, err
	}
	signedJWT, err := mintJWT(appID, privateKey)
	if err != nil {
		return nil, err
	}
	return g.listInstallationIDsWithJWT(signedJWT, githubApi)
}

// listInstallationIDsWithJWT lists installations given an already-signed app JWT, so
// EnsureTokensForEnterpriseApp can reuse the same JWT it mints for the token-creation loop.
func (g *githubappToken) listInstallationIDsWithJWT(signedJWT, githubApi string) ([]string, error) {
	url := fmt.Sprintf("%s/app/installations", githubApi)
	body, err := g.doGithubAppRequest("GET", url, signedJWT)
	if err != nil {
		return nil, err
	}

	var installations []struct {
		ID int64 `json:"id"`
	}
	if err = json.Unmarshal(body, &installations); err != nil {
		return nil, err
	}
	ids := make([]string, len(installations))
	for i, inst := range installations {
		ids[i] = strconv.FormatInt(inst.ID, 10)
	}
	return ids, nil
}

// Secret names are deterministic: GetNameForGithubAppInstallationSecret(job, id) can regenerate
// each name from the job and installation ID alone, so the returned slice need not be persisted.
func (g *githubappToken) EnsureTokensForEnterpriseApp(ctx context.Context, job *api.RenovateJob) ([]string, error) {
	if job.Spec.GithubEnterpriseAppReference == nil {
		return nil, fmt.Errorf("GithubEnterpriseAppReference is not defined")
	}
	ref := job.Spec.GithubEnterpriseAppReference
	appID, pemStr, githubApi, _, err := g.readBaseCredentials(ctx, ref.GithubAppCredentials, job.Namespace, job.Spec.Provider)
	if err != nil {
		return nil, err
	}

	// Parse the PEM and mint the JWT once, then reuse both across the installation-listing
	// call and every per-installation token mint below (the JWT's 10-minute window comfortably
	// covers the whole loop) instead of re-parsing and re-signing per installation.
	privateKey, err := parsePEMKey(pemStr)
	if err != nil {
		return nil, err
	}
	signedJWT, err := mintJWT(appID, privateKey)
	if err != nil {
		return nil, err
	}

	installIDs, err := g.listInstallationIDsWithJWT(signedJWT, githubApi)
	if err != nil {
		return nil, fmt.Errorf("failed to list installation IDs: %w", err)
	}

	secretNames := make([]string, 0, len(installIDs))
	for _, id := range installIDs {
		secretName := GetNameForGithubAppInstallationSecret(job, id)
		fresh, err := g.isTokenFresh(ctx, job.Namespace, secretName)
		if err != nil {
			return nil, err
		}
		if fresh {
			secretNames = append(secretNames, secretName)
			continue
		}
		g.logger.Info("creating/renewing enterprise github app token", "job", job.Fullname(), "installationID", id)

		token, expiresAt, err := g.createInstallationTokenWithJWT(signedJWT, id, githubApi)
		if err != nil {
			return nil, fmt.Errorf("failed to create token for installation %s: %w", id, err)
		}
		if err = g.upsertTokenSecret(ctx, job, secretName, token, expiresAt); err != nil {
			return nil, fmt.Errorf("failed to upsert token secret %s: %w", secretName, err)
		}
		secretNames = append(secretNames, secretName)
	}
	return secretNames, nil
}

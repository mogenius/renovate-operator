package gitProviderClientFactory

import (
	"context"
	"fmt"
	"net/http"
	api "renovate-operator/api/v1alpha1"
	"renovate-operator/gitProviderClients"
	"renovate-operator/gitProviderClients/bitbucketProvider"
	"renovate-operator/gitProviderClients/forgejoProvider"
	"renovate-operator/gitProviderClients/giteaProvider"
	"renovate-operator/gitProviderClients/githubProvider"
	"renovate-operator/gitProviderClients/gitlabProvider"
	"renovate-operator/internal/telemetry"
	"renovate-operator/internal/utils"
	"time"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type GitProviderClientFactory interface {
	// NewClient creates a GitProviderClient for the given RenovateJob by reading
	// platform credentials from the referenced Kubernetes secret.
	NewClient(ctx context.Context, job *api.RenovateJob) (gitProviderClients.GitProviderClient, error)
}

type gitProviderClientFactory struct {
	client client.Client
}

func NewGitProviderClientFactory(c client.Client) GitProviderClientFactory {
	return &gitProviderClientFactory{client: c}
}

// NewClientFactory creates a ClientFactory that reads platform credentials from
// the Kubernetes secret referenced by the RenovateJob and returns the appropriate
// platform client.
func (f *gitProviderClientFactory) NewClient(ctx context.Context, job *api.RenovateJob) (gitProviderClients.GitProviderClient, error) {

	platform, endpoint := utils.GetPlatformAndEndpoint(job.Spec.Provider)
	if platform == "" {
		return nil, fmt.Errorf("skipForks requires a provider to be configured")
	}

	token, err := readToken(ctx, f.client, job)
	if err != nil {
		return nil, fmt.Errorf("failed to read platform token for fork filtering: %w", err)
	}

	httpClient := &http.Client{
		Timeout:   10 * time.Second,
		Transport: telemetry.WrapTransport(http.DefaultTransport),
	}

	switch platform {
	case "github":
		return &githubProvider.GitHubClient{Endpoint: endpoint, Token: token, HTTPClient: httpClient}, nil
	case "gitlab":
		return &gitlabProvider.GitLabClient{Endpoint: endpoint, Token: token, HTTPClient: httpClient}, nil
	case "gitea":
		return &giteaProvider.GiteaClient{Endpoint: endpoint, Token: token, HTTPClient: httpClient}, nil
	case "forgejo":
		return &forgejoProvider.ForgejoClient{Endpoint: endpoint, Token: token, HTTPClient: httpClient}, nil
	case "bitbucket":
		return &bitbucketProvider.BitbucketClient{Endpoint: endpoint, Token: token, HTTPClient: httpClient}, nil
	default:
		return nil, fmt.Errorf("skipForks is not supported for platform %q", platform)
	}

}

// readToken reads the platform API token from the Kubernetes secret referenced
// by the RenovateJob. It checks common key names used by Renovate.
func readToken(ctx context.Context, c client.Client, job *api.RenovateJob) (string, error) {
	if job.Spec.SecretRef == "" {
		return "", fmt.Errorf("secretRef must be set when skipForks is enabled")
	}

	secret := &corev1.Secret{}
	err := c.Get(ctx, client.ObjectKey{
		Name:      job.Spec.SecretRef,
		Namespace: job.Namespace,
	}, secret)
	if err != nil {
		return "", fmt.Errorf("failed to get secret %s: %w", job.Spec.SecretRef, err)
	}

	// Try common token key names in order of preference
	for _, key := range []string{"RENOVATE_TOKEN", "GITHUB_COM_TOKEN", "GITLAB_TOKEN", "BITBUCKET_TOKEN", "GITEA_TOKEN", "FORGEJO_TOKEN"} {
		if val, ok := secret.Data[key]; ok && len(val) > 0 {
			return string(val), nil
		}
	}

	return "", fmt.Errorf("no platform token found in secret %s (expected one of: RENOVATE_TOKEN, GITHUB_COM_TOKEN, GITLAB_TOKEN, BITBUCKET_TOKEN, GITEA_TOKEN, FORGEJO_TOKEN)", job.Spec.SecretRef)
}

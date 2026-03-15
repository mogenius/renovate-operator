package gitprovider

import (
	"context"
	"fmt"
	"net/http"
	api "renovate-operator/api/v1alpha1"
	"renovate-operator/internal/utils"
	"sync"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GitProviderClient provides platform-specific repository operations.
type GitProviderClient interface {
	// IsFork returns true if the given project is a fork.
	IsFork(ctx context.Context, project string) (bool, error)
}

// ClientFactory creates a GitProviderClient for a given RenovateJob.
type ClientFactory func(ctx context.Context, job *api.RenovateJob) (GitProviderClient, error)

// NewClientFactory creates a ClientFactory that reads platform credentials from
// the Kubernetes secret referenced by the RenovateJob and returns the appropriate
// platform client.
func NewClientFactory(c client.Client) ClientFactory {
	return func(ctx context.Context, job *api.RenovateJob) (GitProviderClient, error) {
		platform, endpoint := utils.GetPlatformAndEndpoint(job.Spec.Provider)
		if platform == "" {
			return nil, fmt.Errorf("skipForks requires a provider to be configured")
		}

		token, err := readToken(ctx, c, job)
		if err != nil {
			return nil, fmt.Errorf("failed to read platform token for fork filtering: %w", err)
		}

		httpClient := &http.Client{Timeout: 10 * time.Second}

		switch platform {
		case "github":
			return &GitHubClient{endpoint: endpoint, token: token, httpClient: httpClient}, nil
		case "gitlab":
			return &GitLabClient{endpoint: endpoint, token: token, httpClient: httpClient}, nil
		case "gitea", "forgejo":
			return &GiteaClient{endpoint: endpoint, token: token, httpClient: httpClient}, nil
		case "bitbucket":
			return &BitbucketClient{endpoint: endpoint, token: token, httpClient: httpClient}, nil
		default:
			return nil, fmt.Errorf("skipForks is not supported for platform %q", platform)
		}
	}
}

// FilterForks filters forked repositories from a list of discovered projects
// using the given client. On API errors for individual repos, the repo is kept
// (fail-open) to avoid accidentally excluding valid projects.
func FilterForks(ctx context.Context, providerClient GitProviderClient, logger logr.Logger, projects []string) ([]string, error) {
	if len(projects) == 0 {
		return projects, nil
	}

	const maxConcurrency = 10

	type result struct {
		project string
		isFork  bool
		err     error
	}

	results := make([]result, len(projects))
	sem := make(chan struct{}, maxConcurrency)
	var wg sync.WaitGroup

	for i, project := range projects {
		wg.Add(1)
		go func(idx int, proj string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			isFork, err := providerClient.IsFork(ctx, proj)
			results[idx] = result{project: proj, isFork: isFork, err: err}
		}(i, project)
	}
	wg.Wait()

	filtered := make([]string, 0, len(projects))
	for _, r := range results {
		if r.err != nil {
			logger.V(1).Info("Failed to check if repo is fork, keeping it", "project", r.project, "error", r.err)
			filtered = append(filtered, r.project)
			continue
		}
		if !r.isFork {
			filtered = append(filtered, r.project)
		} else {
			logger.V(2).Info("Excluding forked repository", "project", r.project)
		}
	}

	removed := len(projects) - len(filtered)
	if removed > 0 {
		logger.Info("Filtered forked repositories", "removed", removed, "remaining", len(filtered))
	}
	return filtered, nil
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

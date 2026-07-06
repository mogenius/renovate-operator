package utils

import (
	"fmt"

	api "renovate-operator/api/v1alpha1"
)

func GetPlatformAndEndpoint(provider *api.RenovateProvider) (string, string) {
	if provider == nil {
		return "", ""
	}
	endpoint := provider.Endpoint
	if endpoint == "" {
		switch provider.Name {
		case "github":
			endpoint = "https://api.github.com"
		case "gitlab":
			endpoint = "https://gitlab.com/api/v4"
		}
	}
	return provider.Name, endpoint
}

// WebhookEndpointPath returns the operator's webhook server path for the given
// platform.
func WebhookEndpointPath(platform string) (string, error) {
	switch platform {
	case "github":
		return "/webhook/v1/github", nil
	case "gitlab":
		return "/webhook/v1/gitlab", nil
	case "forgejo":
		return "/webhook/v1/forgejo", nil
	case "gitea":
		return "/webhook/v1/gitea", nil
	default:
		return "", fmt.Errorf("no webhook endpoint for platform %q", platform)
	}
}

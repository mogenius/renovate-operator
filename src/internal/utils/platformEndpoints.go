package utils

import (
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

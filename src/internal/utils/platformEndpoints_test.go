package utils

import (
	api "renovate-operator/api/v1alpha1"
	"testing"
)

func TestGetPlatformAndEndpoint(t *testing.T) {
	tests := []struct {
		name             string
		provider         *api.RenovateProvider
		expectedPlatform string
		expectedEndpoint string
	}{
		{
			name: "github provider with custom endpoint",
			provider: &api.RenovateProvider{
				Name:     "github",
				Endpoint: "https://github.example.com/api/v3",
			},
			expectedPlatform: "github",
			expectedEndpoint: "https://github.example.com/api/v3",
		},
		{
			name: "github provider with default endpoint",
			provider: &api.RenovateProvider{
				Name: "github",
			},
			expectedPlatform: "github",
			expectedEndpoint: "https://api.github.com",
		},
		{
			name: "gitlab provider with custom endpoint",
			provider: &api.RenovateProvider{
				Name:     "gitlab",
				Endpoint: "https://gitlab.example.com/api/v4",
			},
			expectedPlatform: "gitlab",
			expectedEndpoint: "https://gitlab.example.com/api/v4",
		},
		{
			name: "gitlab provider with default endpoint",
			provider: &api.RenovateProvider{
				Name: "gitlab",
			},
			expectedPlatform: "gitlab",
			expectedEndpoint: "https://gitlab.com/api/v4",
		},
		{
			name:             "nil provider",
			provider:         nil,
			expectedPlatform: "",
			expectedEndpoint: "",
		},
		{
			name: "unknown provider with endpoint",
			provider: &api.RenovateProvider{
				Name:     "unknown",
				Endpoint: "https://unknown.example.com/api",
			},
			expectedPlatform: "unknown",
			expectedEndpoint: "https://unknown.example.com/api",
		},
		{
			name: "unknown provider with no endpoint",
			provider: &api.RenovateProvider{
				Name: "unknown",
			},
			expectedPlatform: "unknown",
			expectedEndpoint: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			platform, endpoint := GetPlatformAndEndpoint(tt.provider)
			if platform != tt.expectedPlatform {
				t.Errorf("expected platform %s, got %s", tt.expectedPlatform, platform)
			}
			if endpoint != tt.expectedEndpoint {
				t.Errorf("expected endpoint %s, got %s", tt.expectedEndpoint, endpoint)
			}
		})
	}
}

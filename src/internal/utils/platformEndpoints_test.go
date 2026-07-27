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

func TestGetPublicEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		provider *api.RenovateProvider
		expected string
	}{
		{
			name: "publicEndpoint set — returned as-is",
			provider: &api.RenovateProvider{
				Name:           "gitea",
				Endpoint:       "http://gitea.internal",
				PublicEndpoint: "https://gitea.example.com",
			},
			expected: "https://gitea.example.com",
		},
		{
			name: "publicEndpoint not set — falls back to endpoint",
			provider: &api.RenovateProvider{
				Name:     "gitea",
				Endpoint: "http://gitea.internal",
			},
			expected: "http://gitea.internal",
		},
		{
			name: "publicEndpoint not set, github default — falls back to default API endpoint",
			provider: &api.RenovateProvider{
				Name: "github",
			},
			expected: "https://api.github.com",
		},
		{
			name:     "nil provider",
			provider: nil,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			endpoint := GetPublicEndpoint(tt.provider)
			if endpoint != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, endpoint)
			}
		})
	}
}

func TestWebhookEndpointPath(t *testing.T) {
	tests := []struct {
		platform string
		path     string
		wantErr  bool
	}{
		{platform: "github", path: "/webhook/v1/github"},
		{platform: "gitlab", path: "/webhook/v1/gitlab"},
		{platform: "forgejo", path: "/webhook/v1/forgejo"},
		{platform: "gitea", path: "/webhook/v1/gitea"},
		{platform: "bitbucket", wantErr: true},
		{platform: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.platform, func(t *testing.T) {
			path, err := WebhookEndpointPath(tt.platform)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for platform %q", tt.platform)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if path != tt.path {
				t.Errorf("expected %s, got %s", tt.path, path)
			}
		})
	}
}

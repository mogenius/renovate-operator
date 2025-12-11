package github

import (
	"testing"

	api "renovate-operator/api/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetNameForGithubAppSecretFromJobName(t *testing.T) {
	tests := []struct {
		name     string
		jobName  string
		expected string
	}{
		{
			name:     "short job name",
			jobName:  "test-job",
			expected: "test-job-github-app-4990bd27",
		},
		{
			name:     "long job name gets truncated",
			jobName:  "this-is-a-very-long-job-name-that-exceeds-forty-three-characters",
			expected: "this-is-a-very-long-job-name-that-exceeds-f-github-app-51c9e812",
		},
		{
			name:     "job name with special characters",
			jobName:  "my-job-with-dots.and.dashes",
			expected: "my-job-with-dots-and-dashes-github-app-8c17e7d4",
		},
		{
			name:     "empty job name",
			jobName:  "",
			expected: "-github-app-e3b0c442",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetNameForGithubAppSecretFromJobName(tt.jobName)
			if result != tt.expected {
				t.Errorf("GetNameForGithubAppSecretFromJobName(%q) = %q, want %q", tt.jobName, result, tt.expected)
			}
		})
	}
}

func TestGetNameForGithubAppSecret(t *testing.T) {
	tests := []struct {
		name     string
		job      *api.RenovateJob
		expected string
	}{
		{
			name: "basic job",
			job: &api.RenovateJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-renovate-job",
					Namespace: "default",
				},
			},
			expected: "my-renovate-job-github-app-3bff5513",
		},
		{
			name: "job with long name",
			job: &api.RenovateJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "this-is-a-really-long-renovate-job-name-that-needs-truncation",
					Namespace: "production",
				},
			},
			expected: "this-is-a-really-long-renovate-job-name-tha-github-app-e09ef13c",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetNameForGithubAppSecret(tt.job)
			if result != tt.expected {
				t.Errorf("GetNameForGithubAppSecret() = %q, want %q", result, tt.expected)
			}
		})
	}
}

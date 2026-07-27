package utils

import (
	"reflect"
	"renovate-operator/config"
	"testing"
)

func initPodLabelTemplatesConfig(t *testing.T, templates string) {
	t.Helper()
	err := config.InitializeConfigModule([]config.ConfigItemDescription{
		{Key: "POD_LABEL_TEMPLATES", Optional: true, Default: "{}"},
	})
	if err != nil {
		t.Fatalf("expected to initialize config module without error, got %v", err)
	}
	if templates != "" {
		t.Setenv("POD_LABEL_TEMPLATES", templates)
		err := config.InitializeConfigModule([]config.ConfigItemDescription{
			{Key: "POD_LABEL_TEMPLATES", Optional: true, Default: "{}"},
		})
		if err != nil {
			t.Fatalf("expected to initialize config module without error, got %v", err)
		}
	}
}

func TestConfiguredPodLabels(t *testing.T) {
	tests := []struct {
		name      string
		templates string
		job       string
		project   string
		jobType   string
		namespace string
		expected  map[string]string
	}{
		{
			name:      "no templates configured",
			templates: "",
			job:       "github-renovate",
			jobType:   "executor",
			namespace: "my-organization",
			project:   "foo",
			expected:  nil,
		},
		{
			name:      "executor with all placeholders filled",
			templates: `{"perfectscale.io/workload-grouping-workload-name":"{jobType}-{job}-{namespace}-{project}"}`,
			job:       "github-renovate",
			jobType:   "executor",
			namespace: "my-organization",
			project:   "foo",
			expected: map[string]string{
				"perfectscale.io/workload-grouping-workload-name": "executor-github-renovate-my-organization-foo",
			},
		},
		{
			name:      "discovery job collapses empty project placeholder",
			templates: `{"perfectscale.io/workload-grouping-workload-name":"{jobType}-{job}-{namespace}-{project}"}`,
			job:       "github-renovate",
			jobType:   "discovery",
			namespace: "my-organization",
			project:   "",
			expected: map[string]string{
				"perfectscale.io/workload-grouping-workload-name": "discovery-github-renovate-my-organization",
			},
		},
		{
			name:      "project slug with slash is sanitized",
			templates: `{"perfectscale.io/workload-grouping-workload-name":"{jobType}-{project}"}`,
			job:       "github-renovate",
			jobType:   "executor",
			namespace: "my-organization",
			project:   "org/repo",
			expected: map[string]string{
				"perfectscale.io/workload-grouping-workload-name": "executor-org-repo",
			},
		},
		{
			name:      "malformed json falls back to no labels",
			templates: `{not-json`,
			job:       "github-renovate",
			jobType:   "executor",
			namespace: "my-organization",
			project:   "foo",
			expected:  nil,
		},
		{
			name:      "long rendered value is truncated to 63 chars and trimmed",
			templates: `{"example.com/name":"{jobType}-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`,
			job:       "github-renovate",
			jobType:   "executor",
			namespace: "my-organization",
			project:   "foo",
			expected: map[string]string{
				"example.com/name": "executor-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			initPodLabelTemplatesConfig(t, tt.templates)

			got := ConfiguredPodLabels(tt.job, tt.project, tt.jobType, tt.namespace)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("ConfiguredPodLabels() = %#v, want %#v", got, tt.expected)
			}
		})
	}
}

func TestSanitizeLabelValue(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "collapses dangling separator from empty placeholder", input: "discovery-github-renovate--", expected: "discovery-github-renovate"},
		{name: "collapses mixed separators", input: "a--_.--b", expected: "a-b"},
		{name: "trims leading and trailing separators", input: "-.-foo-.-", expected: "foo"},
		{name: "replaces invalid characters", input: "org/repo {weird}", expected: "org-repo-weird"},
		{name: "preserves case and underscores/dots", input: "Foo_Bar.Baz", expected: "Foo_Bar.Baz"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeLabelValue(tt.input)
			if got != tt.expected {
				t.Errorf("sanitizeLabelValue(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

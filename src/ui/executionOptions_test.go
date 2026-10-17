package ui

import (
	"strings"
	"testing"

	api "renovate-operator/api/v1alpha1"
)

func TestValidateExecutionOptions(t *testing.T) {
	validBranches := []string{
		"main",
		"test/renovate-config",
		"feature/JIRA-123_new.config",
		"release-1.2",
		strings.Repeat("a", maxBaseBranchLength),
	}
	for _, branch := range validBranches {
		if err := validateExecutionOptions(&api.RenovateExecutionOptions{BaseBranch: branch, DryRun: true}); err != nil {
			t.Errorf("expected %q to be accepted, got %v", branch, err)
		}
	}

	invalidBranches := []string{
		"/regex/",
		"!main",
		"-flag",
		".hidden",
		"feature/../main",
		"feature//double",
		"feature/.hidden",
		"feature/",
		"feature.",
		"feature.lock",
		"feature.lock/sub",
		"with space",
		"refs@{0}",
		"quote\"",
		"new\nline",
		"ümlaut",
		strings.Repeat("a", maxBaseBranchLength+1),
	}
	for _, branch := range invalidBranches {
		if err := validateExecutionOptions(&api.RenovateExecutionOptions{BaseBranch: branch, DryRun: true}); err == nil {
			t.Errorf("expected %q to be rejected", branch)
		}
	}

	// a real run against a custom branch would take over the default branch's Renovate branches
	for _, executionOptions := range []*api.RenovateExecutionOptions{{BaseBranch: "main"}, {BaseBranch: "main", Debug: true}} {
		if err := validateExecutionOptions(executionOptions); err == nil {
			t.Errorf("expected %+v without dryRun to be rejected", executionOptions)
		}
	}

	for _, executionOptions := range []*api.RenovateExecutionOptions{nil, {}, {Debug: true, DryRun: true}} {
		if err := validateExecutionOptions(executionOptions); err != nil {
			t.Errorf("expected %+v without a base branch to be accepted, got %v", executionOptions, err)
		}
	}
}

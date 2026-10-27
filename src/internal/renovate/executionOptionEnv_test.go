package renovate

import (
	"testing"

	api "renovate-operator/api/v1alpha1"
	"renovate-operator/config"

	v1 "k8s.io/api/core/v1"
)

func TestNewRenovateJob_CustomBranchExecutionOptions(t *testing.T) {
	_ = config.InitializeConfigModule([]config.ConfigItemDescription{
		{Key: "JOB_TIMEOUT_SECONDS", Optional: true, Default: "10"},
	})

	t.Run("base branch selects the branch and its config, always as a dry run", func(t *testing.T) {
		job := &api.RenovateJob{Name: "rj", Namespace: "ns", Spec: api.RenovateJobSpec{Image: "img"}}

		container := expectContainer(t, newRenovateJob(job, "org/repository",
			withExecutionOptions(&api.RenovateExecutionOptions{BaseBranch: "test/renovate-config"})))

		expectEnvVar(t, container, "RENOVATE_BASE_BRANCH_PATTERNS", `["test/renovate-config"]`)
		expectEnvVar(t, container, "RENOVATE_BASE_BRANCHES", `["test/renovate-config"]`)
		expectEnvVar(t, container, "RENOVATE_USE_BASE_BRANCH_CONFIG", "merge")
		// the status carries no dryRun here, as when written without the API's validation
		expectEnvVar(t, container, "RENOVATE_DRY_RUN", "full")
	})

	t.Run("dry run is passed as full dry run", func(t *testing.T) {
		job := &api.RenovateJob{Name: "rj", Namespace: "ns", Spec: api.RenovateJobSpec{Image: "img"}}

		container := expectContainer(t, newRenovateJob(job, "org/repository",
			withExecutionOptions(&api.RenovateExecutionOptions{BaseBranch: "feature", DryRun: true})))

		expectEnvVar(t, container, "RENOVATE_DRY_RUN", "full")
	})

	t.Run("no execution options add no per-run env", func(t *testing.T) {
		job := &api.RenovateJob{Name: "rj", Namespace: "ns", Spec: api.RenovateJobSpec{Image: "img"}}

		container := expectContainer(t, newRenovateJob(job, "org/repository"))

		for _, name := range []string{"RENOVATE_BASE_BRANCH_PATTERNS", "RENOVATE_BASE_BRANCHES", "RENOVATE_USE_BASE_BRANCH_CONFIG", "RENOVATE_DRY_RUN", "RENOVATE_LOG_LEVEL"} {
			expectEnvVarAbsent(t, container, name)
		}
	})

	t.Run("per-run options win over extra env without duplicating it", func(t *testing.T) {
		job := &api.RenovateJob{Name: "rj", Namespace: "ns", Spec: api.RenovateJobSpec{
			Image: "img",
			ExtraEnv: []v1.EnvVar{
				{Name: "RENOVATE_LOG_LEVEL", Value: "warn"},
				{Name: "RENOVATE_DRY_RUN", Value: "lookup"},
				{Name: "RENOVATE_BASE_BRANCH_PATTERNS", Value: `["main"]`},
			},
		}}

		container := expectContainer(t, newRenovateJob(job, "org/repository",
			withExecutionOptions(&api.RenovateExecutionOptions{Debug: true, DryRun: true, BaseBranch: "feature"})))

		expectEnvVar(t, container, "RENOVATE_LOG_LEVEL", "debug")
		expectEnvVar(t, container, "RENOVATE_DRY_RUN", "full")
		expectEnvVar(t, container, "RENOVATE_BASE_BRANCH_PATTERNS", `["feature"]`)
		for _, name := range []string{"RENOVATE_LOG_LEVEL", "RENOVATE_DRY_RUN", "RENOVATE_BASE_BRANCH_PATTERNS"} {
			expectEnvVarCount(t, container, name, 1)
		}
	})

	t.Run("discovery job ignores execution options", func(t *testing.T) {
		job := &api.RenovateJob{Name: "rj", Namespace: "ns", Spec: api.RenovateJobSpec{Image: "img"}}

		container := expectContainer(t, newDiscoveryJob(job))

		expectEnvVarAbsent(t, container, "RENOVATE_BASE_BRANCH_PATTERNS")
	})
}

func TestNewRenovateJob_DryRunAnnotation(t *testing.T) {
	_ = config.InitializeConfigModule([]config.ConfigItemDescription{
		{Key: "JOB_TIMEOUT_SECONDS", Optional: true, Default: "10"},
	})

	tests := []struct {
		name             string
		executionOptions *api.RenovateExecutionOptions
		wantDryRun       bool
	}{
		{name: "no options", executionOptions: nil, wantDryRun: false},
		{name: "debug only", executionOptions: &api.RenovateExecutionOptions{Debug: true}, wantDryRun: false},
		{name: "dry run", executionOptions: &api.RenovateExecutionOptions{DryRun: true}, wantDryRun: true},
		{name: "custom branch without dryRun in the status", executionOptions: &api.RenovateExecutionOptions{BaseBranch: "feature"}, wantDryRun: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			specAnnotations := map[string]string{"team": "platform"}
			job := &api.RenovateJob{Name: "rj", Namespace: "ns", Spec: api.RenovateJobSpec{
				Image:    "img",
				Metadata: &api.RenovateJobMetadata{Annotations: specAnnotations},
			}}

			batchJob := newRenovateJob(job, "org/repository", withExecutionOptions(tt.executionOptions))

			_, marked := batchJob.Annotations[api.DryRunAnnotationKey]
			if marked != tt.wantDryRun {
				t.Errorf("dry-run annotation present = %v, want %v", marked, tt.wantDryRun)
			}
			wantBaseBranch := ""
			if tt.executionOptions != nil {
				wantBaseBranch = tt.executionOptions.BaseBranch
			}
			if got := batchJob.Annotations[api.BaseBranchAnnotationKey]; got != wantBaseBranch {
				t.Errorf("base-branch annotation = %q, want %q", got, wantBaseBranch)
			}
			if batchJob.Annotations["team"] != "platform" {
				t.Errorf("expected the spec's annotations on the Job, got %v", batchJob.Annotations)
			}
			// the Job builder must not write into the RenovateJob spec's own map
			if _, leaked := specAnnotations[api.DryRunAnnotationKey]; leaked {
				t.Error("dry-run annotation was written into the RenovateJob spec's annotations")
			}
		})
	}
}

func expectEnvVarAbsent(t *testing.T, container *v1.Container, name string) {
	t.Helper()
	expectEnvVarCount(t, container, name, 0)
}

func expectEnvVarCount(t *testing.T, container *v1.Container, name string, expectedCount int) {
	t.Helper()
	count := 0
	for _, env := range container.Env {
		if env.Name == name {
			count++
		}
	}
	if count != expectedCount {
		t.Fatalf("expected env var %s %d time(s), found it %d time(s)", name, expectedCount, count)
	}
}

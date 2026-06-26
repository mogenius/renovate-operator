package metricStore

import (
	"context"
	api "renovate-operator/api/v1alpha1"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestSetRunFailed(t *testing.T) {
	tests := []struct {
		name     string
		failed   bool
		expected float64
	}{
		{
			name:     "set to failed",
			failed:   true,
			expected: 1.0,
		},
		{
			name:     "set to success",
			failed:   false,
			expected: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SetRunFailed("test-ns", "test-job", "test-project", tt.failed)

			value := testutil.ToFloat64(runFailed.WithLabelValues("test-ns", "test-job", "test-project"))
			if value != tt.expected {
				t.Errorf("SetRunFailed() = %v, want %v", value, tt.expected)
			}

			// Cleanup
			runFailed.DeleteLabelValues("test-ns", "test-job", "test-project")
		})
	}
}

func TestSetDependencyIssues(t *testing.T) {
	tests := []struct {
		name      string
		hasIssues bool
		expected  float64
	}{
		{
			name:      "has issues",
			hasIssues: true,
			expected:  1.0,
		},
		{
			name:      "no issues",
			hasIssues: false,
			expected:  0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SetDependencyIssues("test-ns", "test-job", "test-project", tt.hasIssues)

			value := testutil.ToFloat64(dependencyIssues.WithLabelValues("test-ns", "test-job", "test-project"))
			if value != tt.expected {
				t.Errorf("SetDependencyIssues() = %v, want %v", value, tt.expected)
			}

			// Cleanup
			dependencyIssues.DeleteLabelValues("test-ns", "test-job", "test-project")
		})
	}
}

func TestCaptureRenovateProjectExecution(t *testing.T) {
	// Capture a completed execution
	CaptureRenovateProjectExecution(context.Background(), "test-ns", "test-job", "test-project", api.JobStatusCompleted)

	value := testutil.ToFloat64(projectRuns.WithLabelValues("test-ns", "test-job", "test-project", "completed"))
	if value != 1.0 {
		t.Errorf("CaptureRenovateProjectExecution() = %v, want 1.0", value)
	}

	// Capture another one - counter should increment
	CaptureRenovateProjectExecution(context.Background(), "test-ns", "test-job", "test-project", api.JobStatusCompleted)

	value = testutil.ToFloat64(projectRuns.WithLabelValues("test-ns", "test-job", "test-project", "completed"))
	if value != 2.0 {
		t.Errorf("CaptureRenovateProjectExecution() after second call = %v, want 2.0", value)
	}

	// Cleanup
	projectRuns.DeleteLabelValues("test-ns", "test-job", "test-project", "completed")
}

func TestDeleteProjectMetrics(t *testing.T) {
	// Use unique labels to avoid interference with other tests
	ns, job, proj := "delete-test-ns", "delete-test-job", "delete-test-project"

	// Setup: create metrics for a project
	SetRunFailed(ns, job, proj, true)
	SetDependencyIssues(ns, job, proj, true)
	CaptureRenovateProjectExecution(context.Background(), ns, job, proj, api.JobStatusCompleted)
	CaptureRenovateProjectExecution(context.Background(), ns, job, proj, api.JobStatusFailed)

	// Verify metrics exist
	if testutil.ToFloat64(runFailed.WithLabelValues(ns, job, proj)) != 1.0 {
		t.Error("runFailed metric should exist before deletion")
	}
	if testutil.ToFloat64(dependencyIssues.WithLabelValues(ns, job, proj)) != 1.0 {
		t.Error("dependencyIssues metric should exist before deletion")
	}
	if testutil.ToFloat64(projectRuns.WithLabelValues(ns, job, proj, "completed")) != 1.0 {
		t.Error("projectRuns completed metric should exist before deletion")
	}
	if testutil.ToFloat64(projectRuns.WithLabelValues(ns, job, proj, "failed")) != 1.0 {
		t.Error("projectRuns failed metric should exist before deletion")
	}

	// Count metrics before deletion
	runFailedCountBefore := testutil.CollectAndCount(runFailed)
	dependencyIssuesCountBefore := testutil.CollectAndCount(dependencyIssues)
	projectRunsCountBefore := testutil.CollectAndCount(projectRuns)

	// Delete metrics
	DeleteProjectMetrics(ns, job, proj)

	// Count metrics after deletion - should be fewer
	runFailedCountAfter := testutil.CollectAndCount(runFailed)
	dependencyIssuesCountAfter := testutil.CollectAndCount(dependencyIssues)
	projectRunsCountAfter := testutil.CollectAndCount(projectRuns)

	if runFailedCountAfter >= runFailedCountBefore {
		t.Errorf("runFailed metric count should decrease after deletion: before=%d, after=%d", runFailedCountBefore, runFailedCountAfter)
	}
	if dependencyIssuesCountAfter >= dependencyIssuesCountBefore {
		t.Errorf("dependencyIssues metric count should decrease after deletion: before=%d, after=%d", dependencyIssuesCountBefore, dependencyIssuesCountAfter)
	}
	if projectRunsCountAfter >= projectRunsCountBefore {
		t.Errorf("projectRuns metric count should decrease after deletion: before=%d, after=%d", projectRunsCountBefore, projectRunsCountAfter)
	}
}

func TestIncJobDispatched(t *testing.T) {
	ctx := context.Background()
	IncJobDispatched(ctx, "ns", "job", "executor")
	IncJobDispatched(ctx, "ns", "job", "executor")
	IncJobDispatched(ctx, "ns", "job", "discovery")

	if v := testutil.ToFloat64(jobsDispatched.WithLabelValues("ns", "job", "executor")); v != 2.0 {
		t.Errorf("jobsDispatched{executor} = %v, want 2", v)
	}
	if v := testutil.ToFloat64(jobsDispatched.WithLabelValues("ns", "job", "discovery")); v != 1.0 {
		t.Errorf("jobsDispatched{discovery} = %v, want 1", v)
	}

	jobsDispatched.DeleteLabelValues("ns", "job", "executor")
	jobsDispatched.DeleteLabelValues("ns", "job", "discovery")
}

func TestObserveJobDuration(t *testing.T) {
	ctx := context.Background()
	ObserveJobDuration(ctx, "ns", "job", "executor", api.JobStatusCompleted, 42.0)

	// A HistogramVec exposes a _count series; one observation means count == 1.
	if c := testutil.CollectAndCount(jobDuration); c == 0 {
		t.Errorf("jobDuration should have at least one series after observation, got %d", c)
	}
	jobDuration.DeleteLabelValues("ns", "job", "executor", string(api.JobStatusCompleted))
}

func TestSetSaturationGauges(t *testing.T) {
	SetProjectsScheduled("ns", "job", 5)
	SetProjectsRunning("ns", "job", 3)
	SetGlobalRunningProjects(8)
	SetGlobalParallelismLimit(10)

	if v := testutil.ToFloat64(projectsScheduled.WithLabelValues("ns", "job")); v != 5.0 {
		t.Errorf("projectsScheduled = %v, want 5", v)
	}
	if v := testutil.ToFloat64(projectsRunning.WithLabelValues("ns", "job")); v != 3.0 {
		t.Errorf("projectsRunning = %v, want 3", v)
	}
	if v := testutil.ToFloat64(globalRunningProjects); v != 8.0 {
		t.Errorf("globalRunningProjects = %v, want 8", v)
	}
	if v := testutil.ToFloat64(globalParallelismLimit); v != 10.0 {
		t.Errorf("globalParallelismLimit = %v, want 10", v)
	}

	projectsScheduled.DeleteLabelValues("ns", "job")
	projectsRunning.DeleteLabelValues("ns", "job")
}

func TestPullRequestCounters(t *testing.T) {
	ctx := context.Background()
	AddPullRequestsCreated(ctx, "ns", "job", 3)
	AddPullRequestsMerged(ctx, "ns", "job", 2)
	AddPullRequestsUpdated(ctx, "ns", "job", 1)
	// count<=0 must be a no-op
	AddPullRequestsCreated(ctx, "ns", "job", 0)

	if v := testutil.ToFloat64(pullRequestsCreated.WithLabelValues("ns", "job")); v != 3.0 {
		t.Errorf("pullRequestsCreated = %v, want 3", v)
	}
	if v := testutil.ToFloat64(pullRequestsMerged.WithLabelValues("ns", "job")); v != 2.0 {
		t.Errorf("pullRequestsMerged = %v, want 2", v)
	}
	if v := testutil.ToFloat64(pullRequestsUpdated.WithLabelValues("ns", "job")); v != 1.0 {
		t.Errorf("pullRequestsUpdated = %v, want 1", v)
	}

	pullRequestsCreated.DeleteLabelValues("ns", "job")
	pullRequestsMerged.DeleteLabelValues("ns", "job")
	pullRequestsUpdated.DeleteLabelValues("ns", "job")
}

func TestSecOpsCountersAndPosture(t *testing.T) {
	ctx := context.Background()
	IncWebhookSignatureFailure(ctx, "github")
	IncWebhookSignatureFailure(ctx, "github")
	IncUIAuthAttempt(ctx, "oidc", "failure")
	SetOIDCTLSVerificationDisabled(true)

	if v := testutil.ToFloat64(webhookSignatureFailures.WithLabelValues("github")); v != 2.0 {
		t.Errorf("webhookSignatureFailures{github} = %v, want 2", v)
	}
	if v := testutil.ToFloat64(uiAuthAttempts.WithLabelValues("oidc", "failure")); v != 1.0 {
		t.Errorf("uiAuthAttempts{oidc,failure} = %v, want 1", v)
	}
	if v := testutil.ToFloat64(oidcTLSVerificationDisabled); v != 1.0 {
		t.Errorf("oidcTLSVerificationDisabled = %v, want 1", v)
	}

	webhookSignatureFailures.DeleteLabelValues("github")
	uiAuthAttempts.DeleteLabelValues("oidc", "failure")
	SetOIDCTLSVerificationDisabled(false)
}

func TestDeleteProjectMetricsRemovesNewSeries(t *testing.T) {
	ns, job, proj := "del2-ns", "del2-job", "del2-proj"

	SetOpenPullRequests(ns, job, proj, 4)
	SetPullRequestsAwaitingApproval(ns, job, proj, 2)
	SetLastExecutionDuration(ns, job, proj, 12.5)
	SetLogIssues(ns, job, proj, "warn", 1)
	SetLogIssues(ns, job, proj, "error", 1)

	if testutil.CollectAndCount(openPullRequests) == 0 {
		t.Fatal("openPullRequests should exist before deletion")
	}

	DeleteProjectMetrics(ns, job, proj)

	if v := testutil.CollectAndCount(openPullRequests); v != 0 {
		t.Errorf("openPullRequests should be empty after deletion, got %d series", v)
	}
	if v := testutil.CollectAndCount(lastExecutionDuration); v != 0 {
		t.Errorf("lastExecutionDuration should be empty after deletion, got %d series", v)
	}
	if v := testutil.CollectAndCount(logIssues); v != 0 {
		t.Errorf("logIssues should be empty after deletion, got %d series", v)
	}
}

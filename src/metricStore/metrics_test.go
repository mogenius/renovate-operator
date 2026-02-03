package metricStore

import (
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
	CaptureRenovateProjectExecution("test-ns", "test-job", "test-project", "completed")

	value := testutil.ToFloat64(projectRuns.WithLabelValues("test-ns", "test-job", "test-project", "completed"))
	if value != 1.0 {
		t.Errorf("CaptureRenovateProjectExecution() = %v, want 1.0", value)
	}

	// Capture another one - counter should increment
	CaptureRenovateProjectExecution("test-ns", "test-job", "test-project", "completed")

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
	CaptureRenovateProjectExecution(ns, job, proj, "completed")
	CaptureRenovateProjectExecution(ns, job, proj, "failed")

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

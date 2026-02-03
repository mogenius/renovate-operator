package metricStore

import (
	"github.com/prometheus/client_golang/prometheus"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	projectRuns = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "renovate_operator_project_executions_total",
			Help: "Total number of executed Renovate projects",
		},
		[]string{"renovate-namespace", "renovate-job", "project", "status"})

	runFailed = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "renovate_operator_run_failed",
			Help: "Whether the last Renovate run for this project failed (1=failed, 0=success)",
		},
		[]string{"renovate-namespace", "renovate-job", "project"})

	dependencyIssues = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "renovate_operator_dependency_issues",
			Help: "Whether the last Renovate run had WARN/ERROR log entries (1=issues found, 0=clean)",
		},
		[]string{"renovate-namespace", "renovate-job", "project"})
)

func Register(registry ctrlmetrics.RegistererGatherer) {
	registry.MustRegister(projectRuns)
	registry.MustRegister(runFailed)
	registry.MustRegister(dependencyIssues)
}

func CaptureRenovateProjectExecution(namespace, job, project, status string) {
	projectRuns.WithLabelValues(
		namespace,
		job,
		project,
		status,
	).Inc()
}

// SetRunFailed sets the run_failed gauge for a project
func SetRunFailed(namespace, job, project string, failed bool) {
	value := 0.0
	if failed {
		value = 1.0
	}
	runFailed.WithLabelValues(namespace, job, project).Set(value)
}

// SetDependencyIssues sets the dependency_issues gauge for a project
func SetDependencyIssues(namespace, job, project string, hasIssues bool) {
	value := 0.0
	if hasIssues {
		value = 1.0
	}
	dependencyIssues.WithLabelValues(namespace, job, project).Set(value)
}

// DeleteProjectMetrics removes all metrics for a project that was removed from discovery
func DeleteProjectMetrics(namespace, job, project string) {
	runFailed.DeleteLabelValues(namespace, job, project)
	dependencyIssues.DeleteLabelValues(namespace, job, project)
	// Note: projectRuns counter has an additional "status" label, so we delete both possible values
	projectRuns.DeleteLabelValues(namespace, job, project, "completed")
	projectRuns.DeleteLabelValues(namespace, job, project, "failed")
}

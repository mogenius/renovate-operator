package metricStore

import (
	"context"
	"time"

	api "renovate-operator/api/v1alpha1"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

var otelMeter = otel.Meter("renovate-operator/metrics")

// Prometheus metrics
var (
	executorLoopDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "renovate_operator_executor_loop_duration_seconds",
			Help:    "Duration of a single executor loop tick in seconds",
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		},
	)

	projectRuns = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "renovate_operator_project_executions_total",
			Help: "Total number of executed Renovate projects",
		},
		[]string{"renovate_namespace", "renovate_job", "project", "status"})

	runFailed = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "renovate_operator_run_failed",
			Help: "Whether the last Renovate run for this project failed (1=failed, 0=success)",
		},
		[]string{"renovate_namespace", "renovate_job", "project"})

	dependencyIssues = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "renovate_operator_dependency_issues",
			Help: "Whether the last Renovate run had WARN/ERROR log entries (1=issues found, 0=clean)",
		},
		[]string{"renovate_namespace", "renovate_job", "project"})

	approvalsNeeded = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "renovate_operator_approvals_needed",
			Help: "Number of dependency updates awaiting approval after the last Renovate run",
		},
		[]string{"renovate_namespace", "renovate_job", "project"})
)

// OTel metrics — no-ops when OTel is not configured.
var (
	otelExecutorLoopDuration, _ = otelMeter.Float64Histogram(
		"renovate_operator.executor.loop.duration",
		metric.WithUnit("s"),
		metric.WithDescription("Duration of each executor loop tick"),
	)
	otelProjectExecutions, _ = otelMeter.Int64Counter(
		"renovate_operator.project.executions",
		metric.WithUnit("{execution}"),
		metric.WithDescription("Total executed Renovate project runs"),
	)
)

func Register(registry ctrlmetrics.RegistererGatherer) {
	registry.MustRegister(executorLoopDuration)
	registry.MustRegister(projectRuns)
	registry.MustRegister(runFailed)
	registry.MustRegister(dependencyIssues)
	registry.MustRegister(approvalsNeeded)
}

func ObserveExecutorLoopDuration(ctx context.Context, duration time.Duration) {
	executorLoopDuration.Observe(duration.Seconds())
	if otelExecutorLoopDuration.Enabled(ctx) {
		otelExecutorLoopDuration.Record(ctx, duration.Seconds())
	}
}

func CaptureRenovateProjectExecution(ctx context.Context, namespace, job, project string, status api.RenovateProjectStatus) {
	projectRuns.WithLabelValues(
		namespace,
		job,
		project,
		string(status),
	).Inc()
	if otelProjectExecutions.Enabled(ctx) {
		otelProjectExecutions.Add(ctx, 1,
			metric.WithAttributes(
				semconv.K8SNamespaceName(namespace),
				semconv.CICDPipelineName(job),
				MapPipelineResult(status),
			),
		)
	}
}

// MapPipelineResult maps internal RenovateProjectStatus values to OTel semconv
// cicd.pipeline.result enum values for interoperability with tracing backends.
func MapPipelineResult(status api.RenovateProjectStatus) attribute.KeyValue {
	switch status {
	case api.JobStatusCompleted:
		return semconv.CICDPipelineResultSuccess
	case api.JobStatusFailed:
		return semconv.CICDPipelineResultFailure
	default:
		return semconv.CICDPipelineResultKey.String(string(status))
	}
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

// SetApprovalsNeeded sets the count of dependency updates awaiting approval for a project.
func SetApprovalsNeeded(namespace, job, project string, count int) {
	approvalsNeeded.WithLabelValues(namespace, job, project).Set(float64(count))
}

// DeleteProjectMetrics removes all metrics for a project that was removed from discovery
func DeleteProjectMetrics(namespace, job, project string) {
	runFailed.DeleteLabelValues(namespace, job, project)
	dependencyIssues.DeleteLabelValues(namespace, job, project)
	approvalsNeeded.DeleteLabelValues(namespace, job, project)
	// Note: projectRuns counter has an additional "status" label, so we delete both possible values
	projectRuns.DeleteLabelValues(namespace, job, project, "completed")
	projectRuns.DeleteLabelValues(namespace, job, project, "failed")
}

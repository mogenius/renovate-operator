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

// Label-key constants shared between Prometheus label names and OTel attribute keys so
// the two emission paths expose identical dimensions.
const (
	labelNamespace = "renovate_namespace"
	labelJob       = "renovate_job"
	labelProject   = "project"
	labelStatus    = "status"
	labelKind      = "kind"
	labelReason    = "reason"
	labelResult    = "result"
	labelLevel     = "level"
	labelProvider  = "provider"
	labelErrorType = "error_type"
)

// Prometheus metrics — existing.
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
		[]string{labelNamespace, labelJob, labelProject, labelStatus})

	runFailed = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "renovate_operator_run_failed",
			Help: "Whether the last Renovate run for this project failed (1=failed, 0=success)",
		},
		[]string{labelNamespace, labelJob, labelProject})

	dependencyIssues = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "renovate_operator_dependency_issues",
			Help: "Whether the last Renovate run had WARN/ERROR log entries (1=issues found, 0=clean)",
		},
		[]string{labelNamespace, labelJob, labelProject})

	approvalsNeeded = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "renovate_operator_approvals_needed",
			Help: "Number of dependency updates awaiting approval after the last Renovate run",
		},
		[]string{labelNamespace, labelJob, labelProject})
)

// Prometheus metrics — SRE: job lifecycle & latency (Group A).
var (
	jobsDispatched = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "renovate_operator_jobs_dispatched_total",
			Help: "Total number of Kubernetes Jobs launched by the operator",
		},
		[]string{labelNamespace, labelJob, labelKind})

	jobDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "renovate_operator_job_duration_seconds",
			Help:    "Wall-clock duration of a Renovate Kubernetes Job in seconds",
			Buckets: []float64{10, 30, 60, 120, 300, 600, 1200, 1800, 3600},
		},
		[]string{labelNamespace, labelJob, labelKind, labelStatus})

	queueWait = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "renovate_operator_project_queue_wait_seconds",
			Help:    "Time a project spent in Scheduled before being dispatched, in seconds",
			Buckets: []float64{1, 5, 15, 30, 60, 300, 900, 3600},
		},
		[]string{labelNamespace, labelJob})

	jobFailures = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "renovate_operator_job_failures_total",
			Help: "Total Renovate Job failures by failure mode",
		},
		[]string{labelNamespace, labelJob, labelKind, labelReason})
)

// Prometheus metrics — SRE: saturation & queue depth (Group B).
var (
	projectsScheduled = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "renovate_operator_projects_scheduled",
			Help: "Number of projects currently in Scheduled state (queue depth) per job",
		},
		[]string{labelNamespace, labelJob})

	projectsRunning = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "renovate_operator_projects_running",
			Help: "Number of projects currently Running (in-flight) per job",
		},
		[]string{labelNamespace, labelJob})

	globalRunningProjects = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "renovate_operator_global_running_projects",
			Help: "Total number of Running projects across all jobs",
		})

	globalParallelismLimit = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "renovate_operator_global_parallelism_limit",
			Help: "Configured global parallelism limit (0 = unlimited)",
		})
)

// Prometheus metrics — SRE: discovery (Group C).
var (
	discoveryJobs = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "renovate_operator_discovery_jobs_total",
			Help: "Total discovery Jobs completed by status",
		},
		[]string{labelNamespace, labelJob, labelStatus})

	discoveredRepositories = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "renovate_operator_discovered_repositories",
			Help: "Number of repositories seen by the last discovery run",
		},
		[]string{labelNamespace, labelJob})

	repositoriesFiltered = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "renovate_operator_repositories_filtered_total",
			Help: "Total repositories dropped by discovery filters by reason",
		},
		[]string{labelNamespace, labelJob, labelReason})
)

// Prometheus metrics — SRE: scheduler (Group D).
var (
	scheduleRuns = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "renovate_operator_schedule_runs_total",
			Help: "Total cron schedule firings executed by result",
		},
		[]string{labelNamespace, labelJob, labelResult})

	scheduleNextRun = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "renovate_operator_schedule_next_run_timestamp_seconds",
			Help: "Unix timestamp of the next planned scheduled run",
		},
		[]string{labelNamespace, labelJob})
)

// Prometheus metrics — SRE: log quality (Group E).
var (
	logIssues = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "renovate_operator_log_issues",
			Help: "Count of WARN/ERROR log entries in the last run, by level",
		},
		[]string{labelNamespace, labelJob, labelProject, labelLevel})
)

// Prometheus metrics — Results & outcomes (Group L).
var (
	openPullRequests = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "renovate_operator_open_pull_requests",
			Help: "Number of open Renovate-managed pull requests after the last run",
		},
		[]string{labelNamespace, labelJob, labelProject})

	pullRequestsAwaitingApproval = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "renovate_operator_pull_requests_awaiting_approval",
			Help: "Number of pull requests awaiting human approval after the last run",
		},
		[]string{labelNamespace, labelJob, labelProject})

	repositoriesByStatus = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "renovate_operator_repositories_by_status",
			Help: "Number of repositories per Renovate result status (coverage)",
		},
		[]string{labelNamespace, labelJob, labelStatus})

	pullRequestsCreated = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "renovate_operator_pull_requests_created_total",
			Help: "Total Renovate pull requests created",
		},
		[]string{labelNamespace, labelJob})

	pullRequestsMerged = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "renovate_operator_pull_requests_merged_total",
			Help: "Total Renovate pull requests automerged (updates that landed)",
		},
		[]string{labelNamespace, labelJob})

	pullRequestsUpdated = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "renovate_operator_pull_requests_updated_total",
			Help: "Total Renovate pull requests updated",
		},
		[]string{labelNamespace, labelJob})

	lastExecutionDuration = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "renovate_operator_last_execution_duration_seconds",
			Help: "Duration of the most recent Renovate run for this project, in seconds",
		},
		[]string{labelNamespace, labelJob, labelProject})
)

// Prometheus metrics — SecOps: webhook integrity (Group H).
var (
	webhookRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "renovate_operator_webhook_requests_total",
			Help: "Total webhook requests by provider and outcome",
		},
		[]string{labelProvider, labelResult})

	webhookSignatureFailures = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "renovate_operator_webhook_signature_verification_failures_total",
			Help: "Total webhook HMAC signature verification failures by provider",
		},
		[]string{labelProvider})

	webhookAuthFailures = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "renovate_operator_webhook_auth_failures_total",
			Help: "Total webhook authentication failures by provider and error type",
		},
		[]string{labelProvider, labelErrorType})

	webhookPayloadDecodeFailures = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "renovate_operator_webhook_payload_decode_failures_total",
			Help: "Total webhook payloads that failed to decode by provider",
		},
		[]string{labelProvider})
)

// Prometheus metrics — SecOps: credential resolution (Group I).
var (
	secretResolutionErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "renovate_operator_secret_resolution_errors_total",
			Help: "Total Kubernetes Secret resolution errors by error type",
		},
		[]string{labelErrorType})
)

// OTel metrics — no-ops when OTel is not configured. Mirrors the Prometheus
// counters/histograms above. Gauges are intentionally Prometheus-only, matching the
// existing run_failed/dependency_issues/approvals_needed convention.
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

	otelJobsDispatched, _    = otelMeter.Int64Counter("renovate_operator.jobs.dispatched", metric.WithDescription("Kubernetes Jobs launched"))
	otelJobDuration, _       = otelMeter.Float64Histogram("renovate_operator.job.duration", metric.WithUnit("s"), metric.WithDescription("Renovate Job wall-clock duration"))
	otelQueueWait, _         = otelMeter.Float64Histogram("renovate_operator.project.queue_wait", metric.WithUnit("s"), metric.WithDescription("Time a project waited in Scheduled"))
	otelJobFailures, _       = otelMeter.Int64Counter("renovate_operator.job.failures", metric.WithDescription("Renovate Job failures by mode"))
	otelDiscoveryJobs, _     = otelMeter.Int64Counter("renovate_operator.discovery.jobs", metric.WithDescription("Discovery Jobs by status"))
	otelReposFiltered, _     = otelMeter.Int64Counter("renovate_operator.repositories.filtered", metric.WithDescription("Repositories dropped by filters"))
	otelScheduleRuns, _      = otelMeter.Int64Counter("renovate_operator.schedule.runs", metric.WithDescription("Cron schedule firings by result"))
	otelPRsCreated, _        = otelMeter.Int64Counter("renovate_operator.pull_requests.created", metric.WithDescription("Pull requests created"))
	otelPRsMerged, _         = otelMeter.Int64Counter("renovate_operator.pull_requests.merged", metric.WithDescription("Pull requests automerged"))
	otelPRsUpdated, _        = otelMeter.Int64Counter("renovate_operator.pull_requests.updated", metric.WithDescription("Pull requests updated"))
	otelWebhookRequests, _   = otelMeter.Int64Counter("renovate_operator.webhook.requests", metric.WithDescription("Webhook requests by outcome"))
	otelWebhookSigFail, _    = otelMeter.Int64Counter("renovate_operator.webhook.signature_verification.failures", metric.WithDescription("Webhook signature failures"))
	otelWebhookAuthFail, _   = otelMeter.Int64Counter("renovate_operator.webhook.auth.failures", metric.WithDescription("Webhook auth failures"))
	otelWebhookDecodeFail, _ = otelMeter.Int64Counter("renovate_operator.webhook.payload_decode.failures", metric.WithDescription("Webhook payload decode failures"))
	otelSecretResolErrors, _ = otelMeter.Int64Counter("renovate_operator.secret.resolution.errors", metric.WithDescription("Secret resolution errors"))
)

func Register(registry ctrlmetrics.RegistererGatherer) {
	registry.MustRegister(
		// existing
		executorLoopDuration,
		projectRuns,
		runFailed,
		dependencyIssues,
		approvalsNeeded,
		// Group A
		jobsDispatched,
		jobDuration,
		queueWait,
		jobFailures,
		// Group B
		projectsScheduled,
		projectsRunning,
		globalRunningProjects,
		globalParallelismLimit,
		// Group C
		discoveryJobs,
		discoveredRepositories,
		repositoriesFiltered,
		// Group D
		scheduleRuns,
		scheduleNextRun,
		// Group E
		logIssues,
		// Group L
		openPullRequests,
		pullRequestsAwaitingApproval,
		repositoriesByStatus,
		pullRequestsCreated,
		pullRequestsMerged,
		pullRequestsUpdated,
		lastExecutionDuration,
		// Group H
		webhookRequests,
		webhookSignatureFailures,
		webhookAuthFailures,
		webhookPayloadDecodeFailures,
		// Group I
		secretResolutionErrors,
	)
}

// addOtel records an OTel counter increment when an OTel meter is configured.
func addOtel(ctx context.Context, c metric.Int64Counter, n int64, attrs ...attribute.KeyValue) {
	if c.Enabled(ctx) {
		c.Add(ctx, n, metric.WithAttributes(attrs...))
	}
}

// recordOtel records an OTel histogram observation when an OTel meter is configured.
func recordOtel(ctx context.Context, h metric.Float64Histogram, v float64, attrs ...attribute.KeyValue) {
	if h.Enabled(ctx) {
		h.Record(ctx, v, metric.WithAttributes(attrs...))
	}
}

func boolToFloat(b bool) float64 {
	if b {
		return 1.0
	}
	return 0.0
}

// ---------------------------------------------------------------------------
// Existing helpers
// ---------------------------------------------------------------------------

func ObserveExecutorLoopDuration(ctx context.Context, duration time.Duration) {
	executorLoopDuration.Observe(duration.Seconds())
	recordOtel(ctx, otelExecutorLoopDuration, duration.Seconds())
}

func CaptureRenovateProjectExecution(ctx context.Context, namespace, job, project string, status api.RenovateProjectStatus) {
	projectRuns.WithLabelValues(namespace, job, project, string(status)).Inc()
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
	runFailed.WithLabelValues(namespace, job, project).Set(boolToFloat(failed))
}

// SetDependencyIssues sets the dependency_issues gauge for a project
func SetDependencyIssues(namespace, job, project string, hasIssues bool) {
	dependencyIssues.WithLabelValues(namespace, job, project).Set(boolToFloat(hasIssues))
}

// SetApprovalsNeeded sets the count of dependency updates awaiting approval for a project.
func SetApprovalsNeeded(namespace, job, project string, count int) {
	approvalsNeeded.WithLabelValues(namespace, job, project).Set(float64(count))
}

// ---------------------------------------------------------------------------
// Group A — job lifecycle & latency
// ---------------------------------------------------------------------------

// IncJobDispatched counts a Kubernetes Job launch. kind is "executor" or "discovery".
func IncJobDispatched(ctx context.Context, namespace, job, kind string) {
	jobsDispatched.WithLabelValues(namespace, job, kind).Inc()
	addOtel(ctx, otelJobsDispatched, 1,
		attribute.String(labelNamespace, namespace), attribute.String(labelJob, job), attribute.String(labelKind, kind))
}

// ObserveJobDuration records the wall-clock duration of a finished Renovate Job.
func ObserveJobDuration(ctx context.Context, namespace, job, kind string, status api.RenovateProjectStatus, seconds float64) {
	jobDuration.WithLabelValues(namespace, job, kind, string(status)).Observe(seconds)
	recordOtel(ctx, otelJobDuration, seconds,
		attribute.String(labelNamespace, namespace), attribute.String(labelJob, job),
		attribute.String(labelKind, kind), attribute.String(labelStatus, string(status)))
}

// ObserveQueueWait records how long a project waited in Scheduled before dispatch.
func ObserveQueueWait(ctx context.Context, namespace, job string, seconds float64) {
	queueWait.WithLabelValues(namespace, job).Observe(seconds)
	recordOtel(ctx, otelQueueWait, seconds,
		attribute.String(labelNamespace, namespace), attribute.String(labelJob, job))
}

// IncJobFailure counts a Job failure by mode (timeout/backoff_exceeded/job_not_found/pod_error/unknown).
func IncJobFailure(ctx context.Context, namespace, job, kind, reason string) {
	jobFailures.WithLabelValues(namespace, job, kind, reason).Inc()
	addOtel(ctx, otelJobFailures, 1,
		attribute.String(labelNamespace, namespace), attribute.String(labelJob, job),
		attribute.String(labelKind, kind), attribute.String(labelReason, reason))
}

// ---------------------------------------------------------------------------
// Group B — saturation & queue depth (gauges, Prometheus-only)
// ---------------------------------------------------------------------------

func SetProjectsScheduled(namespace, job string, count int) {
	projectsScheduled.WithLabelValues(namespace, job).Set(float64(count))
}

func SetProjectsRunning(namespace, job string, count int) {
	projectsRunning.WithLabelValues(namespace, job).Set(float64(count))
}

func SetGlobalRunningProjects(count int) {
	globalRunningProjects.Set(float64(count))
}

func SetGlobalParallelismLimit(limit int) {
	globalParallelismLimit.Set(float64(limit))
}

// ---------------------------------------------------------------------------
// Group C — discovery
// ---------------------------------------------------------------------------

// IncDiscoveryJob counts a finished discovery Job. status is "completed" or "failed".
func IncDiscoveryJob(ctx context.Context, namespace, job, status string) {
	discoveryJobs.WithLabelValues(namespace, job, status).Inc()
	addOtel(ctx, otelDiscoveryJobs, 1,
		attribute.String(labelNamespace, namespace), attribute.String(labelJob, job), attribute.String(labelStatus, status))
}

func SetDiscoveredRepositories(namespace, job string, count int) {
	discoveredRepositories.WithLabelValues(namespace, job).Set(float64(count))
}

// AddRepositoriesFiltered counts repositories dropped by a filter. reason is "fork" or "pending_deletion".
func AddRepositoriesFiltered(ctx context.Context, namespace, job, reason string, count int) {
	if count <= 0 {
		return
	}
	repositoriesFiltered.WithLabelValues(namespace, job, reason).Add(float64(count))
	addOtel(ctx, otelReposFiltered, int64(count),
		attribute.String(labelNamespace, namespace), attribute.String(labelJob, job), attribute.String(labelReason, reason))
}

// ---------------------------------------------------------------------------
// Group D — scheduler
// ---------------------------------------------------------------------------

// IncScheduleRun counts a cron firing. result is "success" or "error".
func IncScheduleRun(ctx context.Context, namespace, job, result string) {
	scheduleRuns.WithLabelValues(namespace, job, result).Inc()
	addOtel(ctx, otelScheduleRuns, 1,
		attribute.String(labelNamespace, namespace), attribute.String(labelJob, job), attribute.String(labelResult, result))
}

// SetScheduleNextRun sets the Unix timestamp (seconds) of the next planned run.
func SetScheduleNextRun(namespace, job string, unixSeconds float64) {
	scheduleNextRun.WithLabelValues(namespace, job).Set(unixSeconds)
}

// ---------------------------------------------------------------------------
// Group E — log quality (gauge, Prometheus-only)
// ---------------------------------------------------------------------------

// SetLogIssues sets the count of log entries for a level ("warn" or "error").
func SetLogIssues(namespace, job, project, level string, count int) {
	logIssues.WithLabelValues(namespace, job, project, level).Set(float64(count))
}

// ---------------------------------------------------------------------------
// Group L — results & outcomes
// ---------------------------------------------------------------------------

func SetOpenPullRequests(namespace, job, project string, count int) {
	openPullRequests.WithLabelValues(namespace, job, project).Set(float64(count))
}

func SetPullRequestsAwaitingApproval(namespace, job, project string, count int) {
	pullRequestsAwaitingApproval.WithLabelValues(namespace, job, project).Set(float64(count))
}

// SetRepositoriesByStatus sets the count of repositories in a given Renovate result
// status (e.g. active/onboarding/disabled/no_config/unknown) for a job.
func SetRepositoriesByStatus(namespace, job, status string, count int) {
	repositoriesByStatus.WithLabelValues(namespace, job, status).Set(float64(count))
}

func AddPullRequestsCreated(ctx context.Context, namespace, job string, count int) {
	if count <= 0 {
		return
	}
	pullRequestsCreated.WithLabelValues(namespace, job).Add(float64(count))
	addOtel(ctx, otelPRsCreated, int64(count),
		attribute.String(labelNamespace, namespace), attribute.String(labelJob, job))
}

func AddPullRequestsMerged(ctx context.Context, namespace, job string, count int) {
	if count <= 0 {
		return
	}
	pullRequestsMerged.WithLabelValues(namespace, job).Add(float64(count))
	addOtel(ctx, otelPRsMerged, int64(count),
		attribute.String(labelNamespace, namespace), attribute.String(labelJob, job))
}

func AddPullRequestsUpdated(ctx context.Context, namespace, job string, count int) {
	if count <= 0 {
		return
	}
	pullRequestsUpdated.WithLabelValues(namespace, job).Add(float64(count))
	addOtel(ctx, otelPRsUpdated, int64(count),
		attribute.String(labelNamespace, namespace), attribute.String(labelJob, job))
}

// SetLastExecutionDuration sets the most-recent run duration for a project, in seconds.
func SetLastExecutionDuration(namespace, job, project string, seconds float64) {
	lastExecutionDuration.WithLabelValues(namespace, job, project).Set(seconds)
}

// ---------------------------------------------------------------------------
// Group H — webhook integrity
// ---------------------------------------------------------------------------

// IncWebhookRequest counts a webhook request. provider is github/gitlab/forgejo/schedule;
// result is accepted/rejected/ignored.
func IncWebhookRequest(ctx context.Context, provider, result string) {
	webhookRequests.WithLabelValues(provider, result).Inc()
	addOtel(ctx, otelWebhookRequests, 1, attribute.String(labelProvider, provider), attribute.String(labelResult, result))
}

func IncWebhookSignatureFailure(ctx context.Context, provider string) {
	webhookSignatureFailures.WithLabelValues(provider).Inc()
	addOtel(ctx, otelWebhookSigFail, 1, attribute.String(labelProvider, provider))
}

// IncWebhookAuthFailure counts a webhook auth failure. errorType is no_matching_job/auth_failed/secret_error.
func IncWebhookAuthFailure(ctx context.Context, provider, errorType string) {
	webhookAuthFailures.WithLabelValues(provider, errorType).Inc()
	addOtel(ctx, otelWebhookAuthFail, 1, attribute.String(labelProvider, provider), attribute.String(labelErrorType, errorType))
}

func IncWebhookPayloadDecodeFailure(ctx context.Context, provider string) {
	webhookPayloadDecodeFailures.WithLabelValues(provider).Inc()
	addOtel(ctx, otelWebhookDecodeFail, 1, attribute.String(labelProvider, provider))
}

// ---------------------------------------------------------------------------
// Group I — credential resolution
// ---------------------------------------------------------------------------

// IncSecretResolutionError counts a Secret resolution error. errorType is not_found/key_missing/api_error.
func IncSecretResolutionError(ctx context.Context, errorType string) {
	secretResolutionErrors.WithLabelValues(errorType).Inc()
	addOtel(ctx, otelSecretResolErrors, 1, attribute.String(labelErrorType, errorType))
}

// ---------------------------------------------------------------------------
// Cleanup
// ---------------------------------------------------------------------------

// DeleteProjectMetrics removes all per-project metrics for a project that was removed from discovery.
func DeleteProjectMetrics(namespace, job, project string) {
	runFailed.DeleteLabelValues(namespace, job, project)
	dependencyIssues.DeleteLabelValues(namespace, job, project)
	approvalsNeeded.DeleteLabelValues(namespace, job, project)
	openPullRequests.DeleteLabelValues(namespace, job, project)
	pullRequestsAwaitingApproval.DeleteLabelValues(namespace, job, project)
	lastExecutionDuration.DeleteLabelValues(namespace, job, project)
	logIssues.DeleteLabelValues(namespace, job, project, "warn")
	logIssues.DeleteLabelValues(namespace, job, project, "error")
	// Note: projectRuns counter has an additional "status" label, so we delete both possible values
	projectRuns.DeleteLabelValues(namespace, job, project, "completed")
	projectRuns.DeleteLabelValues(namespace, job, project, "failed")
}

package renovate

import (
	context "context"
	"fmt"
	"sort"
	"strconv"
	"time"

	api "renovate-operator/api/v1alpha1"
	"renovate-operator/config"
	"renovate-operator/health"
	"renovate-operator/metricStore"

	crdManager "renovate-operator/internal/crdManager"
	"renovate-operator/internal/logStore"
	"renovate-operator/internal/parser"
	"renovate-operator/internal/podLogs"
	"renovate-operator/internal/telemetry"
	"renovate-operator/internal/types"
	"renovate-operator/internal/utils"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
	"go.opentelemetry.io/otel/trace"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var executorTracer = otel.Tracer("renovate-operator/executor")

/*
RenovateExecutor is the interface that periodically executes RenovateJob CRDs.
It checks the status of each project and starts new jobs as needed based on the specified parameters.
*/
type RenovateExecutor interface {
	// Start begins the periodic execution of RenovateJob CRDs.
	Start(ctx context.Context) error
	// ProcessProjectJobResult handles the status transition of a single Running project given its
	// k8s Job. A nil k8sJob means the job was not found and is treated as failed.
	// Updates metrics, logs, and CRD status. Returns true if the project is still running.
	ProcessProjectJobResult(ctx context.Context, k8sJob *batchv1.Job, project string, jobId crdManager.RenovateJobIdentifier) error
}

type renovateExecutor struct {
	scheme    *runtime.Scheme
	client    client.Client
	logger    logr.Logger
	health    health.HealthCheck
	manager   crdManager.RenovateJobManager
	logStore  logStore.LogStore
	logReader podLogs.PodLogReader
}

type executionOptions struct {
	globalParallelism int
}

func NewRenovateExecutor(scheme *runtime.Scheme, manager crdManager.RenovateJobManager, client client.Client, logger logr.Logger, health health.HealthCheck, ls logStore.LogStore, lr podLogs.PodLogReader) RenovateExecutor {
	return &renovateExecutor{
		client:    client,
		scheme:    scheme,
		manager:   manager,
		logger:    logger,
		health:    health,
		logStore:  ls,
		logReader: lr,
	}
}

func (e *renovateExecutor) Start(ctx context.Context) error {
	globalParallelism, err := strconv.Atoi(config.GetValue("GLOBAL_PARALLELISM_LIMIT"))
	if err != nil {
		return fmt.Errorf("failed to parse GLOBAL_PARALLELISM_LIMIT: %w", err)
	}
	options := executionOptions{
		globalParallelism: globalParallelism,
	}

	go func() {
		e.health.SetExecutorHealth(func(eHealth *health.ExecutorHealth) *health.ExecutorHealth {
			eHealth.Running = true
			return eHealth
		})
		e.logger.Info("starting renovate executor loop")
		for {
			select {
			case <-ctx.Done():
				e.logger.Info("executor loop stopped due to context cancellation")
				return
			default:
				err := e.execute(ctx, options)
				if err != nil {
					e.logger.Error(err, "an error occurred in execution loop")
				}
				select {
				case <-ctx.Done():
					e.logger.Info("executor loop stopped during sleep due to context cancellation")
					return
				case <-time.After(10 * time.Second):
				}
			}
		}
	}()
	return nil
}

func (e *renovateExecutor) execute(ctx context.Context, options executionOptions) error {
	ctx, span := executorTracer.Start(ctx, "executor.tick")
	defer span.End()
	ctx = telemetry.ContextWithTraceLogger(ctx, e.logger)

	start := time.Now()
	defer func() {
		duration := time.Since(start)
		metricStore.ObserveExecutorLoopDuration(ctx, duration)
		log.FromContext(ctx).V(2).Info("Executed renovate executor loop", "duration", duration)
	}()

	renovateJobs, err := e.manager.ListRenovateJobsFull(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("failed to list renovate jobs: %w", err)
	}
	span.SetAttributes(attribute.Int("renovate_operator.jobs.count", len(renovateJobs)))
	log.FromContext(ctx).V(2).Info("Executing renovate executor loop for jobs", "count", len(renovateJobs))

	metricStore.SetGlobalParallelismLimit(options.globalParallelism)

	// Pass 1: check all currently running projects across all jobs, update their statuses,
	// and count how many are still running globally and per job.
	globalRunning, perJobRunning := e.countRunningProjects(renovateJobs)

	// Pass 2: collect all scheduled projects across all jobs, sort for fairness,
	// and dispatch new jobs up to the global and per-job limits.
	err = e.dispatchScheduled(ctx, renovateJobs, globalRunning, perJobRunning, options)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return err
}

// countRunningProjects returns the number of still-running projects globally and per job
// (keyed by job fullname). It also emits per-tick saturation gauges (running/scheduled
// per job, global running) and the repositories-by-status gauge aggregated per job.
func (e *renovateExecutor) countRunningProjects(renovateJobs []api.RenovateJob) (int, map[string]int) {
	globalRunning := 0
	perJobRunning := make(map[string]int, len(renovateJobs))

	// Clear stale repositories_by_status series before repopulating for every job this tick.
	metricStore.ResetRepositoriesByStatus()

	for i := range renovateJobs {
		renovateJob := &renovateJobs[i]
		jobId := crdManager.RenovateJobIdentifier{Name: renovateJob.Name, Namespace: renovateJob.Namespace}
		key := jobId.Fullname()
		perJobRunning[key] = 0

		scheduled := 0
		byResultStatus := make(map[string]int)
		for j := range renovateJob.Status.Projects {
			project := &renovateJob.Status.Projects[j]
			if project.RenovateResultStatus != nil {
				byResultStatus[metricStore.NormalizeRepositoryStatus(*project.RenovateResultStatus)]++
			}
			switch project.Status {
			case api.JobStatusRunning:
				globalRunning++
				perJobRunning[key]++
			case api.JobStatusScheduled:
				scheduled++
			}
		}

		// Saturation gauges (set 0 when none so stale values are cleared).
		metricStore.SetProjectsRunning(renovateJob.Namespace, renovateJob.Name, perJobRunning[key])
		metricStore.SetProjectsScheduled(renovateJob.Namespace, renovateJob.Name, scheduled)

		// repositories_by_status aggregated at the job level by persisted Renovate result.
		for status, count := range byResultStatus {
			metricStore.SetRepositoriesByStatus(renovateJob.Namespace, renovateJob.Name, status, count)
		}
	}

	metricStore.SetGlobalRunningProjects(globalRunning)

	return globalRunning, perJobRunning
}

// ProcessProjectJobResult handles the status transition of a single Running project given its
// k8s Job. A nil k8sJob means the job was not found and is treated as failed.
// Updates metrics, logs, and CRD status. Returns true if the project is still running.
func (e *renovateExecutor) ProcessProjectJobResult(ctx context.Context, k8sJob *batchv1.Job, project string, jobId crdManager.RenovateJobIdentifier) error {
	var newStatus api.RenovateProjectStatus
	var durationStr string
	var duration time.Duration
	if k8sJob == nil {
		newStatus = api.JobStatusFailed
	} else {
		var err error
		newStatus, durationStr, duration, err = getJobStatus(k8sJob)
		if err != nil {
			return err
		}
	}

	if newStatus == api.JobStatusRunning {
		return nil
	}

	// Guard: only proceed if the project is still Running in the CRD.
	// Without this, the job controller replays all existing Jobs on startup and
	// tries to fetch logs for pods that are long gone.
	renovateJob, err := e.manager.GetRenovateJob(ctx, jobId.Name, jobId.Namespace)
	if err != nil {
		return fmt.Errorf("failed to load RenovateJob for status check: %w", err)
	}
	projectIsRunning := false
	for _, p := range renovateJob.Status.Projects {
		if p.Name == project && p.Status == api.JobStatusRunning {
			projectIsRunning = true
			break
		}
	}
	if !projectIsRunning {
		return nil
	}

	// Job finished — collect metrics and update CRD status.
	newProjectStatus := &types.RenovateStatusUpdate{
		Status:   newStatus,
		Duration: &durationStr,
	}
	hasIssues := false
	if k8sJob != nil {
		if logs, err := e.logReader.GetLastJobLog(ctx, k8sJob); err == nil {
			e.logStore.Save(jobId.Namespace, jobId.Name, project, logs)
			parseResult := parser.ParseRenovateLogs(logs)
			hasIssues = parseResult.HasIssues
			newProjectStatus.RenovateResultStatus = parseResult.RenovateResultStatus
			newProjectStatus.PRActivity = parseResult.PRActivity
			newProjectStatus.LogIssues = parseResult.LogIssues
		} else {
			log.FromContext(ctx).Error(err, "failed to get logs for metrics parsing", "project", project)
		}
	}

	metricStore.SetRunFailed(jobId.Namespace, jobId.Name, project, newStatus == api.JobStatusFailed)
	metricStore.SetDependencyIssues(jobId.Namespace, jobId.Name, project, hasIssues)
	approvalsNeeded := 0
	if newProjectStatus.PRActivity != nil {
		approvalsNeeded = newProjectStatus.PRActivity.NeedsApproval
	}
	metricStore.SetApprovalsNeeded(jobId.Namespace, jobId.Name, project, approvalsNeeded)
	metricStore.CaptureRenovateProjectExecution(ctx, jobId.Namespace, jobId.Name, project, newStatus)

	// Execution duration (Group A/E). Only emit when the k8s Job reported a StartTime.
	if k8sJob != nil && k8sJob.Status.StartTime != nil {
		seconds := duration.Seconds()
		metricStore.ObserveJobDuration(ctx, jobId.Namespace, jobId.Name, "executor", newStatus, seconds)
		metricStore.SetLastExecutionDuration(jobId.Namespace, jobId.Name, project, seconds)
	}

	// Failure breakdown by reason (Group A).
	if newStatus == api.JobStatusFailed {
		metricStore.IncJobFailure(ctx, jobId.Namespace, jobId.Name, "executor", jobFailureReason(k8sJob))
	}

	// Log issue counts (Group L).
	if newProjectStatus.LogIssues != nil {
		metricStore.SetLogIssues(jobId.Namespace, jobId.Name, project, "warn", newProjectStatus.LogIssues.WarnCount)
		metricStore.SetLogIssues(jobId.Namespace, jobId.Name, project, "error", newProjectStatus.LogIssues.ErrorCount)
	}

	// Pull request activity (Group E/L).
	if pr := newProjectStatus.PRActivity; pr != nil {
		metricStore.AddPullRequestsCreated(ctx, jobId.Namespace, jobId.Name, pr.Created)
		metricStore.AddPullRequestsMerged(ctx, jobId.Namespace, jobId.Name, pr.Automerged)
		metricStore.AddPullRequestsUpdated(ctx, jobId.Namespace, jobId.Name, pr.Updated)
		metricStore.SetPullRequestsAwaitingApproval(jobId.Namespace, jobId.Name, project, pr.NeedsApproval)
		// Open managed PRs: automerged ones are closed, so exclude them.
		metricStore.SetOpenPullRequests(jobId.Namespace, jobId.Name, project, pr.Created+pr.Updated+pr.NeedsApproval+pr.Unchanged)
	} else {
		metricStore.SetPullRequestsAwaitingApproval(jobId.Namespace, jobId.Name, project, 0)
		metricStore.SetOpenPullRequests(jobId.Namespace, jobId.Name, project, 0)
	}

	if span := trace.SpanFromContext(ctx); span.IsRecording() {
		span.AddEvent("project.completed", trace.WithAttributes(
			semconv.K8SNamespaceName(jobId.Namespace),
			semconv.K8SJobName(k8sJob.GetName()),
			metricStore.MapPipelineResult(newStatus),
		))
	}

	if err := e.manager.UpdateProjectStatus(ctx, project, jobId, newProjectStatus); err != nil {
		return err
	}

	if k8sJob != nil {
		if err := crdManager.MarkJobProcessed(ctx, e.client, k8sJob); err != nil {
			log.FromContext(ctx).Error(err, "failed to mark executor job as processed", "job", k8sJob.Name)
		}
	}

	if newStatus == api.JobStatusCompleted && config.GetValue("DELETE_SUCCESSFUL_JOBS") == "true" && k8sJob != nil {
		if err := crdManager.DeleteJob(ctx, e.client, k8sJob); err != nil {
			return err
		}
	}

	return nil
}

// scheduledCandidate is a project ready to be dispatched together with its parent RenovateJob.
type scheduledCandidate struct {
	project     api.ProjectStatus
	renovateJob *api.RenovateJob
	// jobOldestWait is the smallest LastRun time among all Scheduled projects in the same
	// RenovateJob. Used as the fairness key: jobs whose projects have been waiting the longest
	// are dispatched first to prevent starvation.
	jobOldestWait time.Time
}

// dispatchScheduled collects all Scheduled projects across all RenovateJobs, sorts them for
// fairness, and launches Kubernetes Jobs until the global or per-job parallelism limits are reached.
func (e *renovateExecutor) dispatchScheduled(ctx context.Context, renovateJobs []api.RenovateJob, globalRunning int, perJobRunning map[string]int, options executionOptions) error {
	if options.globalParallelism > 0 && globalRunning >= options.globalParallelism {
		log.FromContext(ctx).V(2).Info("global parallelism limit reached, skipping dispatch", "limit", options.globalParallelism)
		return nil
	}

	var candidates []scheduledCandidate

	for i := range renovateJobs {
		renovateJob := &renovateJobs[i]
		oldestWait := time.Now()
		startIdx := len(candidates)

		for _, p := range renovateJob.Status.Projects {
			if p.Status != api.JobStatusScheduled {
				continue
			}
			if p.LastRun.Time.Before(oldestWait) {
				oldestWait = p.LastRun.Time
			}
			candidates = append(candidates, scheduledCandidate{
				project:     p,
				renovateJob: renovateJob,
			})
		}

		// Back-fill jobOldestWait now that we know the oldest wait for this job.
		for k := startIdx; k < len(candidates); k++ {
			candidates[k].jobOldestWait = oldestWait
		}
	}

	// Primary sort: higher priority projects go first.
	// Secondary sort: among equal-priority candidates, the RenovateJob that has been waiting longest goes first.
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].project.Priority != candidates[j].project.Priority {
			return candidates[i].project.Priority > candidates[j].project.Priority
		}
		return candidates[i].jobOldestWait.Before(candidates[j].jobOldestWait)
	})

	// Ensure the Redis URL secret exists in each namespace that has candidates,
	// once per namespace instead of once per candidate.
	seenNamespaces := make(map[string]struct{}, len(renovateJobs))
	for _, c := range candidates {
		seenNamespaces[c.renovateJob.Namespace] = struct{}{}
	}
	for ns := range seenNamespaces {
		if err := ensureRedisURLSecret(ctx, e.client, ns); err != nil {
			return fmt.Errorf("failed to ensure redis url secret: %w", err)
		}
	}

	for _, candidate := range candidates {
		renovateJob := candidate.renovateJob
		project := candidate.project
		jobId := crdManager.RenovateJobIdentifier{Name: renovateJob.Name, Namespace: renovateJob.Namespace}
		key := jobId.Fullname()

		// Stop entirely if the global limit is reached.
		if options.globalParallelism > 0 && globalRunning >= options.globalParallelism {
			log.FromContext(ctx).V(2).Info("global parallelism limit reached, stopping dispatch", "limit", options.globalParallelism)
			break
		}

		// Skip this candidate if its job has reached its per-job limit.
		if perJobRunning[key] >= int(renovateJob.Spec.Parallelism) {
			continue
		}

		carrier := propagation.MapCarrier{}
		otel.GetTextMapPropagator().Inject(ctx, carrier)
		k8sJob := newRenovateJob(renovateJob, project.Name, carrier.Get("traceparent"))
		if err := controllerutil.SetControllerReference(renovateJob, k8sJob, e.scheme); err != nil {
			return fmt.Errorf("failed to set controller reference: %w", err)
		}

		_, err := crdManager.CreateJobWithGeneration(ctx, e.client, k8sJob, crdManager.JobSelector{
			JobType:         crdManager.ExecutorJobType,
			Namespace:       renovateJob.Namespace,
			RenovateJobName: renovateJob.Name,
			Project:         project.Name,
		})
		if err != nil {
			return fmt.Errorf("failed to create RenovateJob for project %s: %w", project.Name, err)
		}

		metricStore.IncJobDispatched(ctx, renovateJob.Namespace, renovateJob.Name, "executor")

		// Queue wait: time the project spent in Scheduled before this dispatch.
		if project.ScheduledAt != nil {
			metricStore.ObserveQueueWait(ctx, renovateJob.Namespace, renovateJob.Name, time.Since(project.ScheduledAt.Time).Seconds())
		}

		if span := trace.SpanFromContext(ctx); span.IsRecording() {
			span.AddEvent("job.created", trace.WithAttributes(
				semconv.K8SNamespaceName(renovateJob.Namespace),
				semconv.K8SJobName(utils.ExecutorJobName(renovateJob, project.Name)),
			))
		}

		if err := e.manager.UpdateProjectStatus(ctx, project.Name, jobId, &types.RenovateStatusUpdate{
			Status: api.JobStatusRunning,
		}); err != nil {
			return err
		}

		globalRunning++
		perJobRunning[key]++
	}

	return nil
}

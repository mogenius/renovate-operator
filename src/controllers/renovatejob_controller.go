package controllers

import (
	context "context"
	api "renovate-operator/api/v1alpha1"
	"renovate-operator/github"
	"renovate-operator/internal/renovate"
	"renovate-operator/internal/telemetry"
	"renovate-operator/internal/types"
	"renovate-operator/internal/webhookSync"
	"renovate-operator/scheduler"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
	"go.opentelemetry.io/otel/trace"
	batchv1 "k8s.io/api/batch/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	crdManager "renovate-operator/internal/crdManager"
)

var reconcilerTracer = otel.Tracer("renovate-operator/reconciler")

/*
Reconciler for RenovateJob resources
Watching for create/update/delete events and managing the schedules accordingly
*/
type RenovateJobReconciler struct {
	Discovery   renovate.DiscoveryAgent
	Manager     crdManager.RenovateJobManager
	Scheduler   scheduler.Scheduler
	K8sClient   client.Client
	WebhookSync webhookSync.WebhookSyncManager
	GithubApp   github.GithubAppToken
}

func (r *RenovateJobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctx, span := reconcilerTracer.Start(ctx, "RenovateJob.Reconcile",
		trace.WithAttributes(
			semconv.K8SNamespaceName(req.Namespace),
			semconv.CICDPipelineName(req.Name),
		),
	)
	defer span.End()
	ctx = telemetry.ContextWithTraceLogger(ctx, log.FromContext(ctx).WithName("renovatejob-controller"))

	logger := log.FromContext(ctx)
	renovateJob, err := r.Manager.GetRenovateJob(ctx, req.Name, req.Namespace)

	if err == nil {
		// renovatejob object read without problem -> create the schedule
		r.resetOrphanedRunning(ctx, renovateJob)
		r.WebhookSync.EnsureSyncer(ctx, logger, renovateJob)
		createScheduler(logger, renovateJob, r)
		if err := r.GithubApp.EnsureToken(ctx, renovateJob); err != nil {
			logger.Error(err, "failed to ensure github app token")
		}
		r.handleAnnotationTriggers(ctx, logger, renovateJob)
		return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
	} else if errors.IsNotFound(err) {
		// renovatejob cannot be found -> delete the schedule
		// the github app token secret is owned by the RenovateJob and cleaned up by Kubernetes GC
		name := req.Name + "-" + req.Namespace
		r.Scheduler.RemoveSchedule(req.Namespace, req.Name)
		r.WebhookSync.RemoveSyncer(name)
		return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
	} else {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error(err, "Failed to get RenovateJob")
		return ctrl.Result{RequeueAfter: 1 * time.Minute}, err
	}
}

func createScheduler(logger logr.Logger, renovateJob *api.RenovateJob, reconciler *RenovateJobReconciler) {
	name := renovateJob.Fullname()
	expr := renovateJob.Spec.Schedule
	jobName := renovateJob.Name
	jobNamespace := renovateJob.Namespace
	f := func() {
		ctx := context.Background()
		ctx, span := reconcilerTracer.Start(ctx, "RenovateJob.ScheduledRun",
			trace.WithAttributes(
				semconv.K8SNamespaceName(jobNamespace),
				semconv.CICDPipelineName(jobName),
			),
		)
		defer span.End()
		ctx = telemetry.ContextWithTraceLogger(ctx, logger.WithName(name))
		logger := log.FromContext(ctx)

		logger.V(2).Info("Executing schedule for RenovateJob")

		// Re-fetch the RenovateJob to get the latest spec (e.g. updated container image)
		currentJob, err := reconciler.Manager.GetRenovateJob(ctx, jobName, jobNamespace)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			logger.Error(err, "Failed to get current RenovateJob")
			return
		}

		_, err = reconciler.Discovery.CreateDiscoveryJob(ctx, *currentJob, renovate.DiscoveryJobOptions{TriggerAllProjects: true})
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			logger.Error(err, "Failed to create discovery job for RenovateJob")
			return
		}
		logger.V(2).Info("Discovery job created, completion handled by job controller")
	}

	// adding the schedule if it does not exist
	// if the expression is different it will be updated
	err := reconciler.Scheduler.AddScheduleReplaceExisting(expr, renovateJob.Namespace, renovateJob.Name, f)
	if err != nil {
		logger.Error(err, "Failed to add schedule for RenovateJob")
		return
	}
	logger.V(2).Info("Added schedule for RenovateJob", "schedule", expr)
}

// resetOrphanedRunning resets Running projects whose k8s Job no longer exists (e.g. deleted
// while the operator was scaled down). Uses a single list call to avoid per-project API calls.
func (r *RenovateJobReconciler) resetOrphanedRunning(ctx context.Context, renovateJob *api.RenovateJob) {
	hasRunning := false
	for _, p := range renovateJob.Status.Projects {
		if p.Status == api.JobStatusRunning {
			hasRunning = true
			break
		}
	}
	if !hasRunning {
		return
	}

	logger := log.FromContext(ctx)
	jobId := crdManager.RenovateJobIdentifier{Name: renovateJob.Name, Namespace: renovateJob.Namespace}

	existingJobs, err := crdManager.GetJobsByLabel(ctx, r.K8sClient, crdManager.JobSelector{
		RenovateJobName: renovateJob.Name,
		JobType:         crdManager.ExecutorJobType,
		Namespace:       renovateJob.Namespace,
	})
	if err != nil {
		logger.Error(err, "failed to list executor jobs for orphan check")
		return
	}

	activeProjects := make(map[string]struct{}, len(existingJobs))
	for _, j := range existingJobs {
		if name := j.Annotations[crdManager.JOB_ANNOTATION_PROJECT]; name != "" {
			activeProjects[name] = struct{}{}
		}
	}

	isOrphaned := func(p api.ProjectStatus) bool {
		if p.Status != api.JobStatusRunning {
			return false
		}
		_, active := activeProjects[p.Name]
		return !active
	}

	if err := r.Manager.UpdateProjectStatusBatched(ctx, isOrphaned, jobId, &types.RenovateStatusUpdate{Status: api.JobStatusFailed}); err != nil {
		logger.Error(err, "failed to reset orphaned running projects")
	}
}

// handleAnnotationTriggers checks for one-shot trigger annotations on the RenovateJob and acts on them:
//   - renovate-operator.mogenius.com/discovery: "true"           → start a discovery run
//   - renovate-operator.mogenius.com/schedule-all: "true"        → set all non-running projects to Scheduled
//   - renovate-operator.mogenius.com/schedule: "org/a,org/b"     → set specific non-running projects to Scheduled
//
// Each annotation is removed once its action succeeds, making triggers idempotent one-shots.
// Note: these are annotations (not labels) because project names may contain slashes.
func (r *RenovateJobReconciler) handleAnnotationTriggers(ctx context.Context, logger logr.Logger, renovateJob *api.RenovateJob) {
	annotations := renovateJob.Annotations
	if len(annotations) == 0 {
		return
	}

	toRemove := make([]string, 0, 3)
	jobId := crdManager.RenovateJobIdentifier{Name: renovateJob.Name, Namespace: renovateJob.Namespace}

	if annotations[crdManager.RENOVATEJOB_ANNOTATION_TRIGGER_DISCOVERY] == "true" {
		if _, err := r.Discovery.CreateDiscoveryJob(ctx, *renovateJob, renovate.DiscoveryJobOptions{}); err != nil {
			logger.Error(err, "failed to trigger discovery")
		} else {
			logger.V(1).Info("discovery triggered via annotation")
			toRemove = append(toRemove, crdManager.RENOVATEJOB_ANNOTATION_TRIGGER_DISCOVERY)
		}
	}

	if annotations[crdManager.RENOVATEJOB_ANNOTATION_TRIGGER_SCHEDULE_ALL] == "true" {
		isNotRunning := func(p api.ProjectStatus) bool { return p.Status != api.JobStatusRunning }
		if err := r.Manager.UpdateProjectStatusBatched(ctx, isNotRunning, jobId, &types.RenovateStatusUpdate{Status: api.JobStatusScheduled}); err != nil {
			logger.Error(err, "failed to schedule all projects")
		} else {
			logger.V(1).Info("all projects scheduled via annotation")
			toRemove = append(toRemove, crdManager.RENOVATEJOB_ANNOTATION_TRIGGER_SCHEDULE_ALL)
		}
	}

	if projectsStr := annotations[crdManager.RENOVATEJOB_ANNOTATION_TRIGGER_SCHEDULE]; projectsStr != "" {
		projectSet := parseAnnotationProjectList(projectsStr)
		isTargeted := func(p api.ProjectStatus) bool {
			_, ok := projectSet[p.Name]
			return ok && p.Status != api.JobStatusRunning
		}
		if err := r.Manager.UpdateProjectStatusBatched(ctx, isTargeted, jobId, &types.RenovateStatusUpdate{Status: api.JobStatusScheduled}); err != nil {
			logger.Error(err, "failed to schedule projects from annotation")
		} else {
			logger.V(1).Info("projects scheduled via annotation", "projects", projectsStr)
			toRemove = append(toRemove, crdManager.RENOVATEJOB_ANNOTATION_TRIGGER_SCHEDULE)
		}
	}

	if len(toRemove) == 0 {
		return
	}

	patch := client.MergeFrom(renovateJob.DeepCopyObject().(client.Object))
	for _, key := range toRemove {
		delete(renovateJob.Annotations, key)
	}
	if err := r.K8sClient.Patch(ctx, renovateJob, patch); err != nil {
		logger.Error(err, "failed to remove trigger annotations from RenovateJob")
	}
}

func parseAnnotationProjectList(s string) map[string]struct{} {
	parts := strings.Split(s, ",")
	result := make(map[string]struct{}, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			result[p] = struct{}{}
		}
	}
	return result
}

func (r *RenovateJobReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&api.RenovateJob{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}

package controllers

import (
	context "context"
	api "renovate-operator/api/v1alpha1"
	"renovate-operator/github"
	"renovate-operator/internal/policy"
	"renovate-operator/internal/renovate"
	"renovate-operator/internal/telemetry"
	"renovate-operator/internal/types"
	"renovate-operator/metricStore"
	"renovate-operator/scheduler"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"
	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	apitypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	crdManager "renovate-operator/internal/crdManager"
)

// templateRefIndexKey indexes RenovateJobs by the "<kind>/<name>" of their
// spec.templateRef, so a template change can be mapped back to its dependents.
const templateRefIndexKey = ".spec.templateRef"

var reconcilerTracer = otel.Tracer("renovate-operator/reconciler")

/*
Reconciler for RenovateJob resources
Watching for create/update/delete events and managing the schedules accordingly
*/
type RenovateJobReconciler struct {
	Discovery renovate.DiscoveryAgent
	Manager   crdManager.RenovateJobManager
	Scheduler scheduler.Scheduler
	K8sClient client.Client
	GithubApp github.GithubAppToken
	Policy    policy.Policy
}

func (r *RenovateJobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctx, span := telemetry.StartSpan(ctx, reconcilerTracer, "RenovateJob.Reconcile",
		log.FromContext(ctx).WithName("renovatejob-controller"),
		trace.WithAttributes(
			semconv.K8SNamespaceName(req.Namespace),
			attribute.String("renovate_operator.renovatejob.name", req.Name),
		),
	)
	defer span.End()

	logger := log.FromContext(ctx)
	// Raw object: written back below (finalizers, annotations), so it must never
	// carry an inherited spec.
	renovateJob, err := r.Manager.GetRawRenovateJob(ctx, req.Name, req.Namespace)

	if err == nil {
		if !renovateJob.DeletionTimestamp.IsZero() {
			return r.handleDeletion(ctx, logger, renovateJob)
		}

		// Merge the template in once. Every derived action (scheduling, token, config
		// map, discovery) reads the effective spec; a dangling templateRef leaves
		// effectiveJob nil and is reported by acceptJob.
		effectiveJob, resolveErr := r.Manager.ResolveEffective(ctx, renovateJob)

		jobId := crdManager.RenovateJobIdentifier{Name: renovateJob.Name, Namespace: renovateJob.Namespace}
		if projects, projErr := r.Manager.GetProjectsForRenovateJob(ctx, jobId); projErr != nil {
			logger.Error(projErr, "failed to get projects for metric rehydration")
		} else {
			projectMap := make(map[string]api.RenovateProjectState, len(projects))
			for _, p := range projects {
				projectMap[p.Name] = p.RenovateProjectState
			}
			metricStore.RehydrateMetrics(renovateJob.Namespace, renovateJob.Name, projectMap)
		}

		// Gate before anything is scheduled or created.
		if !r.acceptJob(ctx, logger, jobId, effectiveJob, resolveErr) {
			return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
		}

		r.ensureWebhookCleanupFinalizer(ctx, logger, renovateJob, effectiveJob)
		r.resetOrphanedRunning(ctx, renovateJob)
		createScheduler(logger, effectiveJob, r)
		if err := r.GithubApp.EnsureToken(ctx, effectiveJob); err != nil {
			logger.Error(err, "failed to ensure github app token")
		}
		if err := renovate.EnsureRenovateConfigMap(ctx, r.K8sClient, effectiveJob); err != nil {
			logger.Error(err, "failed to ensure renovate config configmap")
		}
		r.handleAnnotationTriggers(ctx, logger, renovateJob, effectiveJob)
		span.SetStatus(codes.Ok, "")
		return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
	} else if errors.IsNotFound(err) {
		// renovatejob cannot be found -> delete the schedule
		// the github app token secret is owned by the RenovateJob and cleaned up by Kubernetes GC
		r.Scheduler.RemoveSchedule(req.Namespace, req.Name)
		span.SetStatus(codes.Ok, "")
		return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
	} else {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error(err, "Failed to get RenovateJob")
		return ctrl.Result{RequeueAfter: 1 * time.Minute}, err
	}
}

// acceptJob validates the effective RenovateJob against the operator's policy and
// records the outcome as the Accepted condition. It returns false when the job must
// not run: a dangling templateRef, a required field still missing after the
// template is merged in, or a policy violation each fail the job closed.
func (r *RenovateJobReconciler) acceptJob(ctx context.Context, logger logr.Logger, jobID crdManager.RenovateJobIdentifier, effectiveJob *api.RenovateJob, resolveErr error) bool {
	if resolveErr != nil {
		return r.refuseJob(ctx, logger, jobID, policy.ReasonTemplateNotFound, resolveErr.Error())
	}

	if msg := missingRequiredSpecFields(effectiveJob.Spec); msg != "" {
		return r.refuseJob(ctx, logger, jobID, policy.ReasonIncompleteSpec, msg)
	}

	err := r.Policy.ValidateJob(effectiveJob)
	if err == nil {
		message := "RenovateJob satisfies the operator's policy"
		if r.Policy.Disabled {
			message = "the policy engine is disabled, so this RenovateJob was not checked"
		}
		if _, condErr := r.Manager.SetAcceptedCondition(ctx, jobID, true, r.Policy.AcceptedReason(), message); condErr != nil {
			logger.Error(condErr, "failed to record the Accepted condition")
		}
		return true
	}

	reason := policy.ReasonFor(err)
	if reason == "" {
		reason = policy.ReasonDestinationNotAllowed
	}

	metricStore.IncPolicyDenial(ctx, "destination")
	return r.refuseJob(ctx, logger, jobID, reason, err.Error())
}

// refuseJob records an Accepted=False condition, removes the schedule and returns
// false, so a refused job stops running until the cause is fixed.
func (r *RenovateJobReconciler) refuseJob(ctx context.Context, logger logr.Logger, jobID crdManager.RenovateJobIdentifier, reason, message string) bool {
	r.Scheduler.RemoveSchedule(jobID.Namespace, jobID.Name)

	// Log only when the refusal is new or its reason changed, not on every reconcile
	// tick, or a persistently-refused job floods the log.
	changed, condErr := r.Manager.SetAcceptedCondition(ctx, jobID, false, reason, message)
	if condErr != nil {
		logger.Error(condErr, "failed to record the Accepted condition")
	} else if changed {
		logger.Info("RenovateJob refused, nothing will run for it until this is fixed",
			"renovateJob", jobID.Name, "namespace", jobID.Namespace, "reason", reason, "message", message)
	}
	return false
}

// missingRequiredSpecFields reports the fields a RenovateJob must carry once its
// template is merged in, or "" when the effective spec is complete. These are
// enforced here, not by the CRD schema, because a template may supply them (so the
// schema marks them optional).
func missingRequiredSpecFields(spec api.RenovateJobSpec) string {
	var missing []string
	if spec.Schedule == "" {
		missing = append(missing, "spec.schedule")
	}
	if spec.Image == "" {
		missing = append(missing, "spec.image")
	}
	if spec.Provider == nil || spec.Provider.Name == "" {
		missing = append(missing, "spec.provider.name")
	}
	if spec.Parallelism <= 0 {
		missing = append(missing, "spec.parallelism")
	}
	if len(missing) == 0 {
		return ""
	}
	return strings.Join(missing, ", ") + " must be set, on the RenovateJob or on the template it references"
}

func (r *RenovateJobReconciler) ensureWebhookCleanupFinalizer(ctx context.Context, logger logr.Logger, renovateJob *api.RenovateJob, effectiveJob *api.RenovateJob) {
	// A dangling templateRef leaves the effective spec unknown; leave the finalizer
	// as-is rather than guess whether webhook sync is configured.
	if effectiveJob == nil {
		return
	}
	webhook := effectiveJob.Spec.Webhook
	syncEnabled := webhook != nil && webhook.Enabled && webhook.Sync != nil && webhook.Sync.Enabled

	if syncEnabled == controllerutil.ContainsFinalizer(renovateJob, api.FinalizerWebhookCleanup) {
		return
	}
	if syncEnabled {
		controllerutil.AddFinalizer(renovateJob, api.FinalizerWebhookCleanup)
	} else {
		controllerutil.RemoveFinalizer(renovateJob, api.FinalizerWebhookCleanup)
	}
	if err := r.K8sClient.Update(ctx, renovateJob); err != nil {
		logger.Error(err, "failed to update webhook cleanup finalizer")
	}
}

func (r *RenovateJobReconciler) handleDeletion(ctx context.Context, logger logr.Logger, renovateJob *api.RenovateJob) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(renovateJob, api.FinalizerWebhookCleanup) {
		return ctrl.Result{}, nil
	}

	jobId := crdManager.RenovateJobIdentifier{Name: renovateJob.Name, Namespace: renovateJob.Namespace}
	if err := r.Manager.CleanupWebhooks(ctx, jobId); err != nil {
		logger.Error(err, "failed to clean up webhooks during deletion, hooks may remain on the platform")
	}

	controllerutil.RemoveFinalizer(renovateJob, api.FinalizerWebhookCleanup)
	if err := r.K8sClient.Update(ctx, renovateJob); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func createScheduler(logger logr.Logger, renovateJob *api.RenovateJob, reconciler *RenovateJobReconciler) {
	name := renovateJob.Fullname()
	expr := renovateJob.Spec.Schedule
	jobName := renovateJob.Name
	jobNamespace := renovateJob.Namespace
	f := func() {
		ctx := context.Background()
		ctx, span := telemetry.StartSpan(ctx, reconcilerTracer, "RenovateJob.ScheduledRun",
			logger.WithName(name),
			trace.WithAttributes(
				semconv.K8SNamespaceName(jobNamespace),
				attribute.String("renovate_operator.renovatejob.name", jobName),
			),
		)
		defer span.End()
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
		span.SetStatus(codes.Ok, "")
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
	logger := log.FromContext(ctx)
	jobId := crdManager.RenovateJobIdentifier{Name: renovateJob.Name, Namespace: renovateJob.Namespace}

	runningProjects, err := r.Manager.GetProjectsByStatus(ctx, jobId, api.JobStatusRunning)
	if err != nil {
		logger.Error(err, "failed to get running projects for orphan check")
		return
	}
	if len(runningProjects) == 0 {
		return
	}

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
		if name := j.Annotations[api.ProjectAnnotationKey]; name != "" {
			activeProjects[name] = struct{}{}
		}
	}

	isOrphaned := func(p crdManager.RenovateProjectStatus) bool {
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
// effectiveJob carries the template-resolved spec discovery must run with; the raw
// renovateJob carries the trigger annotations to read and remove.
func (r *RenovateJobReconciler) handleAnnotationTriggers(ctx context.Context, logger logr.Logger, renovateJob *api.RenovateJob, effectiveJob *api.RenovateJob) {
	annotations := renovateJob.Annotations
	if len(annotations) == 0 {
		return
	}

	toRemove := make([]string, 0, 3)
	jobId := crdManager.RenovateJobIdentifier{Name: renovateJob.Name, Namespace: renovateJob.Namespace}

	if annotations[api.TriggerDiscoveryAnnotationKey] == "true" {
		if _, err := r.Discovery.CreateDiscoveryJob(ctx, *effectiveJob, renovate.DiscoveryJobOptions{}); err != nil {
			logger.Error(err, "failed to trigger discovery")
		} else {
			logger.V(1).Info("discovery triggered via annotation")
			toRemove = append(toRemove, api.TriggerDiscoveryAnnotationKey)
		}
	}

	if annotations[api.TriggerScheduleAllAnnotationKey] == "true" {
		isNotRunning := func(p crdManager.RenovateProjectStatus) bool { return p.Status != api.JobStatusRunning }
		if err := r.Manager.UpdateProjectStatusBatched(ctx, isNotRunning, jobId, &types.RenovateStatusUpdate{Status: api.JobStatusScheduled}); err != nil {
			logger.Error(err, "failed to schedule all projects")
		} else {
			logger.V(1).Info("all projects scheduled via annotation")
			toRemove = append(toRemove, api.TriggerScheduleAllAnnotationKey)
		}
	}

	if projectsStr := annotations[api.TriggerScheduleAnnotationKey]; projectsStr != "" {
		projectSet := parseAnnotationProjectList(projectsStr)
		isTargeted := func(p crdManager.RenovateProjectStatus) bool {
			_, ok := projectSet[p.Name]
			return ok && p.Status != api.JobStatusRunning
		}
		if err := r.Manager.UpdateProjectStatusBatched(ctx, isTargeted, jobId, &types.RenovateStatusUpdate{Status: api.JobStatusScheduled}); err != nil {
			logger.Error(err, "failed to schedule projects from annotation")
		} else {
			logger.V(1).Info("projects scheduled via annotation", "projects", projectsStr)
			toRemove = append(toRemove, api.TriggerScheduleAnnotationKey)
		}
	}

	if len(toRemove) == 0 {
		return
	}

	if err := crdManager.RemoveAnnotation(ctx, r.K8sClient, renovateJob, toRemove...); err != nil {
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

// templateRefKey renders the index/lookup key for a templateRef, defaulting the
// kind to the namespaced RenovateJobTemplate.
func templateRefKey(kind, name string) string {
	if kind == "" {
		kind = api.KindRenovateJobTemplate
	}
	return kind + "/" + name
}

func (r *RenovateJobReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &api.RenovateJob{}, templateRefIndexKey,
		func(obj client.Object) []string {
			ref := obj.(*api.RenovateJob).Spec.TemplateRef
			if ref == nil {
				return nil
			}
			return []string{templateRefKey(ref.Kind, ref.Name)}
		}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&api.RenovateJob{}).
		Owns(&corev1.ConfigMap{}).
		Watches(&api.RenovateJobTemplate{}, handler.EnqueueRequestsFromMapFunc(r.jobsForNamespacedTemplate)).
		Watches(&api.ClusterRenovateJobTemplate{}, handler.EnqueueRequestsFromMapFunc(r.jobsForClusterTemplate)).
		Complete(r)
}

// jobsForNamespacedTemplate enqueues every RenovateJob in the template's namespace
// that references it, so an edit propagates immediately.
func (r *RenovateJobReconciler) jobsForNamespacedTemplate(ctx context.Context, obj client.Object) []reconcile.Request {
	return r.requestsForTemplateKey(ctx,
		templateRefKey(api.KindRenovateJobTemplate, obj.GetName()),
		client.InNamespace(obj.GetNamespace()))
}

// jobsForClusterTemplate enqueues every RenovateJob in any namespace that
// references the changed cluster-scoped template.
func (r *RenovateJobReconciler) jobsForClusterTemplate(ctx context.Context, obj client.Object) []reconcile.Request {
	return r.requestsForTemplateKey(ctx,
		templateRefKey(api.KindClusterRenovateJobTemplate, obj.GetName()))
}

func (r *RenovateJobReconciler) requestsForTemplateKey(ctx context.Context, key string, opts ...client.ListOption) []reconcile.Request {
	var jobs api.RenovateJobList
	listOpts := append([]client.ListOption{client.MatchingFields{templateRefIndexKey: key}}, opts...)
	if err := r.K8sClient.List(ctx, &jobs, listOpts...); err != nil {
		log.FromContext(ctx).Error(err, "failed to list RenovateJobs for template change", "templateRef", key)
		return nil
	}
	requests := make([]reconcile.Request, 0, len(jobs.Items))
	for i := range jobs.Items {
		requests = append(requests, reconcile.Request{NamespacedName: apitypes.NamespacedName{
			Name:      jobs.Items[i].Name,
			Namespace: jobs.Items[i].Namespace,
		}})
	}
	return requests
}

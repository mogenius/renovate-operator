package controllers

import (
	context "context"
	"encoding/json"
	"fmt"
	"net/url"
	api "renovate-operator/api/v1alpha1"
	"renovate-operator/gitProviderClients/forgejoProvider"
	"renovate-operator/internal/renovate"
	"renovate-operator/internal/telemetry"
	"renovate-operator/internal/types"
	"renovate-operator/internal/utils"
	"renovate-operator/internal/webhookSync"
	"renovate-operator/scheduler"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
	"go.opentelemetry.io/otel/trace"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	crdManager "renovate-operator/internal/crdManager"
)

var reconcilerTracer = otel.Tracer("renovate-operator/reconciler")

const webhookSyncStateAnnotation = "renovate-operator.mogenius.com/webhook-sync-managed-repos"

/*
Reconciler for RenovateJob resources
Watching for create/update/delete events and managing the schedules accordingly
*/
type RenovateJobReconciler struct {
	Discovery      renovate.DiscoveryAgent
	Manager        crdManager.RenovateJobManager
	Scheduler      scheduler.Scheduler
	K8sClient      client.Client
	webhookSyncers map[string]*webhookSyncerEntry
}

type webhookSyncerEntry struct {
	syncer      *webhookSync.WebhookSyncer
	fingerprint string
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
		r.ensureWebhookSyncer(ctx, logger, renovateJob)
		createScheduler(logger, renovateJob, r)
		return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
	} else if errors.IsNotFound(err) {
		// renovatejob cannot be found -> delete the schedule
		name := req.Name + "-" + req.Namespace
		r.Scheduler.RemoveSchedule(name)
		delete(r.webhookSyncers, name)
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

		projects, err := reconciler.Discovery.Discover(ctx, currentJob)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			logger.Error(err, "Failed to discover projects for RenovateJob")
			return
		}
		span.AddEvent("discovery.complete", trace.WithAttributes(
			attribute.Int("project.count", len(projects)),
		))
		logger.V(2).Info("Successfully discovered projects", "count", len(projects))

		err = reconciler.Manager.ReconcileProjects(ctx, currentJob, projects)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			logger.Error(err, "failed to reconcile projects")
			return
		}
		logger.V(2).Info("Successfully reconciled Projects")

		isNotRunning := func(p api.ProjectStatus) bool {
			return p.Status != api.JobStatusRunning
		}
		err = reconciler.Manager.UpdateProjectStatusBatched(ctx, isNotRunning, crdManager.RenovateJobIdentifier{
			Name:      jobName,
			Namespace: jobNamespace,
		}, &types.RenovateStatusUpdate{
			Status: api.JobStatusScheduled,
		})

		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			logger.Error(err, "failed to schedule projects")
			return
		}
		logger.V(2).Info("Successfully scheduled RenovateJob")

		// Run Forgejo webhook sync after discovery completes
		if entry, ok := reconciler.webhookSyncers[name]; ok {
			state, err := entry.syncer.RunOnce(ctx)
			if err != nil {
				logger.Error(err, "webhook sync failed")
			}
			if state != nil {
				reconciler.saveWebhookSyncState(ctx, logger, jobName, jobNamespace, entry.syncer, state)
			}
		}
	}

	// adding the schedule if it does not exist
	// if the expression is different it will be updated
	err := reconciler.Scheduler.AddScheduleReplaceExisting(expr, name, f)
	if err != nil {
		logger.Error(err, "Failed to add schedule for RenovateJob")
		return
	}
	logger.V(2).Info("Added schedule for RenovateJob", "schedule", expr)
}

// ensureWebhookSyncer creates, updates, or removes the WebhookSyncer for a RenovateJob
// based on the webhook.forgejo.sync configuration.
func (r *RenovateJobReconciler) ensureWebhookSyncer(ctx context.Context, logger logr.Logger, renovateJob *api.RenovateJob) {
	name := renovateJob.Fullname()

	if renovateJob.Spec.Webhook == nil || renovateJob.Spec.Webhook.Forgejo == nil || renovateJob.Spec.Webhook.Forgejo.Sync == nil || !renovateJob.Spec.Webhook.Forgejo.Sync.Enabled {
		delete(r.webhookSyncers, name)
		return
	}

	syncCfg := renovateJob.Spec.Webhook.Forgejo.Sync
	_, providerEndpoint := utils.GetPlatformAndEndpoint(renovateJob.Spec.Provider)
	fp := syncFingerprint(syncCfg, providerEndpoint, renovateJob.Spec.DiscoverTopics, renovateJob.Namespace, renovateJob.Name)

	// Config unchanged — nothing to do
	if entry, exists := r.webhookSyncers[name]; exists && entry.fingerprint == fp {
		return
	}

	jobNamespace := renovateJob.Namespace

	if syncCfg.TokenSecretRef == nil {
		logger.Error(fmt.Errorf("tokenSecretRef is required when webhook sync is enabled"), "cannot initialize webhook syncer without a Forgejo API token")
		return
	}

	forgejoToken, err := r.readSecretKey(ctx, syncCfg.TokenSecretRef, jobNamespace)
	if err != nil {
		logger.Error(err, "failed to read Forgejo API token for webhook sync")
		return
	}

	var authToken string
	if syncCfg.AuthTokenSecretRef != nil {
		authToken, err = r.readSecretKey(ctx, syncCfg.AuthTokenSecretRef, jobNamespace)
		if err != nil {
			logger.Error(err, "failed to read auth token for webhook sync")
			return
		}
	}

	topic := syncCfg.Topic
	if topic == "" && len(renovateJob.Spec.DiscoverTopics) > 0 {
		topic = renovateJob.Spec.DiscoverTopics[0]
	}

	webhookURL := syncCfg.WebhookURL
	parsed, err := url.Parse(webhookURL)
	if err != nil {
		logger.Error(err, "failed to parse webhookURL")
		return
	}
	q := parsed.Query()
	q.Set("namespace", renovateJob.Namespace)
	q.Set("job", renovateJob.Name)
	parsed.RawQuery = q.Encode()
	webhookURL = parsed.String()

	if providerEndpoint == "" {
		logger.Error(fmt.Errorf("provider endpoint is required when webhook sync is enabled"), "cannot initialize webhook syncer without a Forgejo endpoint")
		return
	}

	forgejoClient := forgejoProvider.NewClient(providerEndpoint, forgejoToken)
	syncer := webhookSync.NewWebhookSyncer(
		forgejoClient,
		webhookURL,
		authToken,
		topic,
		syncCfg.Events,
		logger.WithName("webhook-sync"),
	)

	// Transfer managed repos state: prefer in-memory state from old syncer,
	// fall back to persisted annotation (covers operator restart).
	if oldEntry, exists := r.webhookSyncers[name]; exists {
		syncer.SetManagedRepos(oldEntry.syncer.ManagedRepos())
	} else {
		state := r.loadWebhookSyncState(renovateJob)
		if len(state) > 0 {
			syncer.SetManagedRepos(state)
			logger.V(2).Info("restored webhook sync state from annotation", "repos", len(state))
		}
	}

	r.webhookSyncers[name] = &webhookSyncerEntry{syncer: syncer, fingerprint: fp}
}

// syncFingerprint produces a string that changes when any sync-relevant config changes.
func syncFingerprint(cfg *api.RenovateWebhookForgejoSync, endpoint string, defaultTopic []string, namespace, jobName string) string {
	topic := cfg.Topic
	if topic == "" && len(defaultTopic) > 0 {
		topic = defaultTopic[0]
	}
	tokenRef := ""
	if cfg.TokenSecretRef != nil {
		tokenRef = cfg.TokenSecretRef.Name + "/" + cfg.TokenSecretRef.Key
	}
	authRef := ""
	if cfg.AuthTokenSecretRef != nil {
		authRef = cfg.AuthTokenSecretRef.Name + "/" + cfg.AuthTokenSecretRef.Key
	}
	events := strings.Join(cfg.Events, ",")
	return fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s", endpoint, cfg.WebhookURL, topic, events, tokenRef, authRef, namespace+"/"+jobName)
}

func (r *RenovateJobReconciler) loadWebhookSyncState(renovateJob *api.RenovateJob) map[string]int64 {
	if renovateJob.Annotations == nil {
		return nil
	}
	raw, ok := renovateJob.Annotations[webhookSyncStateAnnotation]
	if !ok || raw == "" {
		return nil
	}
	var state map[string]int64
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return nil
	}
	return state
}

func (r *RenovateJobReconciler) saveWebhookSyncState(ctx context.Context, logger logr.Logger, jobName, jobNamespace string, syncer *webhookSync.WebhookSyncer, state map[string]int64) {
	renovateJob, err := r.Manager.GetRenovateJob(ctx, jobName, jobNamespace)
	if err != nil {
		logger.Error(err, "failed to fetch RenovateJob for saving webhook sync state")
		return
	}

	data, err := json.Marshal(state)
	if err != nil {
		logger.Error(err, "failed to marshal webhook sync state")
		return
	}

	newVal := string(data)
	if renovateJob.Annotations != nil && renovateJob.Annotations[webhookSyncStateAnnotation] == newVal {
		return // no change
	}

	// Remember what was previously persisted so we can roll back on failure
	var previousState map[string]int64
	if raw, ok := renovateJob.Annotations[webhookSyncStateAnnotation]; ok && raw != "" {
		if err := json.Unmarshal([]byte(raw), &previousState); err != nil {
			previousState = nil
		}
	}

	if renovateJob.Annotations == nil {
		renovateJob.Annotations = make(map[string]string)
	}
	renovateJob.Annotations[webhookSyncStateAnnotation] = newVal

	if err := r.K8sClient.Update(ctx, renovateJob); err != nil {
		logger.Error(err, "failed to save webhook sync state annotation, rolling back in-memory state to last persisted value")
		if previousState != nil {
			syncer.SetManagedRepos(previousState)
		}
	}
}

func (r *RenovateJobReconciler) readSecretKey(ctx context.Context, ref *api.RenovateSecretKeyReference, namespace string) (string, error) {
	if ref == nil {
		return "", fmt.Errorf("secret reference is nil")
	}
	secret := &corev1.Secret{}
	err := r.K8sClient.Get(ctx, client.ObjectKey{
		Name:      ref.Name,
		Namespace: namespace,
	}, secret)
	if err != nil {
		return "", fmt.Errorf("reading secret %s: %w", ref.Name, err)
	}
	data, ok := secret.Data[ref.Key]
	if !ok {
		return "", fmt.Errorf("key %s not found in secret %s", ref.Key, ref.Name)
	}
	return string(data), nil
}

func (r *RenovateJobReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.webhookSyncers = make(map[string]*webhookSyncerEntry)
	return ctrl.NewControllerManagedBy(mgr).
		For(&api.RenovateJob{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}

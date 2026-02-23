package controllers

import (
	context "context"
	"encoding/json"
	"fmt"
	api "renovate-operator/api/v1alpha1"
	"renovate-operator/internal/forgejo"
	"renovate-operator/internal/renovate"
	"renovate-operator/scheduler"
	"strings"
	"time"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	crdManager "renovate-operator/internal/crdManager"
)

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
	syncer      *forgejo.WebhookSyncer
	fingerprint string
}

func (r *RenovateJobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	if r.webhookSyncers == nil {
		r.webhookSyncers = make(map[string]*webhookSyncerEntry)
	}
	logger := log.FromContext(ctx).WithName("renovatejob-controller")
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
		logger = logger.WithName(name)
		ctx := context.Background()
		logger.V(2).Info("Executing schedule for RenovateJob")

		// Re-fetch the RenovateJob to get the latest spec (e.g. updated container image)
		currentJob, err := reconciler.Manager.GetRenovateJob(ctx, jobName, jobNamespace)
		if err != nil {
			logger.Error(err, "Failed to get current RenovateJob")
			return
		}

		projects, err := reconciler.Discovery.Discover(ctx, currentJob)
		if err != nil {
			logger.Error(err, "Failed to discover projects for RenovateJob")
			return
		}
		logger.V(2).Info("Successfully discovered projects", "count", len(projects))

		jobIdentifier := crdManager.RenovateJobIdentifier{
			Name:      jobName,
			Namespace: jobNamespace,
		}
		err = reconciler.Manager.ReconcileProjects(ctx, jobIdentifier, projects)
		if err != nil {
			logger.Error(err, "failed to reconcile projects")
			return
		}
		logger.V(2).Info("Successfully reconciled Projects")

		isNotRunning := func(p api.ProjectStatus) bool {
			return p.Status != api.JobStatusRunning
		}
		err = reconciler.Manager.UpdateProjectStatusBatched(ctx, isNotRunning, jobIdentifier, api.JobStatusScheduled)

		if err != nil {
			logger.Error(err, "failed to schedule projects")
		}
		logger.V(2).Info("Successfully scheduled RenovateJob")

		// Run Forgejo webhook sync after discovery completes
		if entry, ok := reconciler.webhookSyncers[name]; ok {
			if err := entry.syncer.RunOnce(ctx); err != nil {
				logger.Error(err, "webhook sync failed")
			}
			// Persist managed repos state to annotation
			reconciler.saveWebhookSyncState(ctx, logger, jobName, jobNamespace, entry.syncer.ManagedRepos())
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
	fp := syncFingerprint(syncCfg, renovateJob.Spec.DiscoverTopics, renovateJob.Namespace, renovateJob.Name)

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
	if topic == "" {
		topic = renovateJob.Spec.DiscoverTopics
	}

	webhookURL := syncCfg.WebhookURL
	sep := "?"
	if strings.Contains(webhookURL, "?") {
		sep = "&"
	}
	webhookURL = fmt.Sprintf("%s%snamespace=%s&job=%s", webhookURL, sep, renovateJob.Namespace, renovateJob.Name)

	forgejoClient := forgejo.NewClient(syncCfg.ForgejoURL, forgejoToken)
	syncer := forgejo.NewWebhookSyncer(
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
func syncFingerprint(cfg *api.RenovateWebhookForgejoSync, defaultTopic, namespace, jobName string) string {
	topic := cfg.Topic
	if topic == "" {
		topic = defaultTopic
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
	return fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s", cfg.ForgejoURL, cfg.WebhookURL, topic, events, tokenRef, authRef, namespace+"/"+jobName)
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

func (r *RenovateJobReconciler) saveWebhookSyncState(ctx context.Context, logger logr.Logger, jobName, jobNamespace string, state map[string]int64) {
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

	if renovateJob.Annotations == nil {
		renovateJob.Annotations = make(map[string]string)
	}
	renovateJob.Annotations[webhookSyncStateAnnotation] = newVal

	if err := r.K8sClient.Update(ctx, renovateJob); err != nil {
		logger.Error(err, "failed to save webhook sync state annotation")
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
	return ctrl.NewControllerManagedBy(mgr).
		For(&api.RenovateJob{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}

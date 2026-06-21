package webhookSync

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	api "renovate-operator/api/v1alpha1"
	"renovate-operator/gitProviderClients/forgejoProvider"
	crdManager "renovate-operator/internal/crdManager"
	"renovate-operator/internal/utils"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const SyncStateAnnotation = "renovate-operator.mogenius.com/webhook-sync-managed-repos"

type WebhookSyncManager interface {
	// EnsureSyncer creates, updates, or removes the WebhookSyncer for a RenovateJob
	// based on the webhook.forgejo.sync configuration.
	EnsureSyncer(ctx context.Context, logger logr.Logger, renovateJob *api.RenovateJob)
	// RunSync executes one webhook sync cycle for the given job, if a syncer is configured.
	RunSync(ctx context.Context, logger logr.Logger, jobId crdManager.RenovateJobIdentifier)
	// RemoveSyncer removes the syncer entry for the given job (called on RenovateJob deletion).
	RemoveSyncer(name string)
}

type syncerEntry struct {
	syncer      *WebhookSyncer
	fingerprint string
}

type webhookSyncManager struct {
	syncers    map[string]*syncerEntry
	k8sClient  client.Client
	jobManager crdManager.RenovateJobManager
}

func NewWebhookSyncManager(k8sClient client.Client, jobManager crdManager.RenovateJobManager) WebhookSyncManager {
	return &webhookSyncManager{
		syncers:    make(map[string]*syncerEntry),
		k8sClient:  k8sClient,
		jobManager: jobManager,
	}
}

func (m *webhookSyncManager) RemoveSyncer(name string) {
	delete(m.syncers, name)
}

func (m *webhookSyncManager) EnsureSyncer(ctx context.Context, logger logr.Logger, renovateJob *api.RenovateJob) {
	name := renovateJob.Fullname()

	if renovateJob.Spec.Webhook == nil || renovateJob.Spec.Webhook.Forgejo == nil ||
		renovateJob.Spec.Webhook.Forgejo.Sync == nil || !renovateJob.Spec.Webhook.Forgejo.Sync.Enabled {
		delete(m.syncers, name)
		return
	}

	syncCfg := renovateJob.Spec.Webhook.Forgejo.Sync
	_, providerEndpoint := utils.GetPlatformAndEndpoint(renovateJob.Spec.Provider)
	fp := syncFingerprint(syncCfg, providerEndpoint, renovateJob.Spec.DiscoverTopics, renovateJob.Namespace, renovateJob.Name)

	if entry, exists := m.syncers[name]; exists && entry.fingerprint == fp {
		return
	}

	jobNamespace := renovateJob.Namespace

	if syncCfg.TokenSecretRef == nil {
		logger.Error(fmt.Errorf("tokenSecretRef is required when webhook sync is enabled"), "cannot initialize webhook syncer without a Forgejo API token")
		return
	}

	forgejoToken, err := m.readSecretKey(ctx, syncCfg.TokenSecretRef, jobNamespace)
	if err != nil {
		logger.Error(err, "failed to read Forgejo API token for webhook sync")
		return
	}

	var authToken string
	if syncCfg.AuthTokenSecretRef != nil {
		authToken, err = m.readSecretKey(ctx, syncCfg.AuthTokenSecretRef, jobNamespace)
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
	syncer := NewWebhookSyncer(
		forgejoClient,
		webhookURL,
		authToken,
		topic,
		syncCfg.Events,
		logger.WithName("webhook-sync"),
	)

	// Transfer managed repos state: prefer in-memory from old syncer,
	// fall back to persisted annotation (covers operator restart).
	if oldEntry, exists := m.syncers[name]; exists {
		syncer.SetManagedRepos(oldEntry.syncer.ManagedRepos())
	} else {
		state := loadWebhookSyncState(renovateJob)
		if len(state) > 0 {
			syncer.SetManagedRepos(state)
			logger.V(2).Info("restored webhook sync state from annotation", "repos", len(state))
		}
	}

	m.syncers[name] = &syncerEntry{syncer: syncer, fingerprint: fp}
}

func (m *webhookSyncManager) RunSync(ctx context.Context, logger logr.Logger, jobId crdManager.RenovateJobIdentifier) {
	name := jobId.Name + "-" + jobId.Namespace
	entry, ok := m.syncers[name]
	if !ok {
		return
	}
	state, err := entry.syncer.RunOnce(ctx)
	if err != nil {
		logger.Error(err, "webhook sync failed")
	}
	if state != nil {
		m.saveWebhookSyncState(ctx, logger, jobId.Name, jobId.Namespace, entry.syncer, state)
	}
}

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

func loadWebhookSyncState(renovateJob *api.RenovateJob) map[string]int64 {
	if renovateJob.Annotations == nil {
		return nil
	}
	raw, ok := renovateJob.Annotations[SyncStateAnnotation]
	if !ok || raw == "" {
		return nil
	}
	var state map[string]int64
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return nil
	}
	return state
}

func (m *webhookSyncManager) saveWebhookSyncState(ctx context.Context, logger logr.Logger, jobName, jobNamespace string, syncer *WebhookSyncer, state map[string]int64) {
	renovateJob, err := m.jobManager.GetRenovateJob(ctx, jobName, jobNamespace)
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
	if renovateJob.Annotations != nil && renovateJob.Annotations[SyncStateAnnotation] == newVal {
		return
	}

	var previousState map[string]int64
	if raw, ok := renovateJob.Annotations[SyncStateAnnotation]; ok && raw != "" {
		if err := json.Unmarshal([]byte(raw), &previousState); err != nil {
			previousState = nil
		}
	}

	if renovateJob.Annotations == nil {
		renovateJob.Annotations = make(map[string]string)
	}
	renovateJob.Annotations[SyncStateAnnotation] = newVal

	if err := m.k8sClient.Update(ctx, renovateJob); err != nil {
		logger.Error(err, "failed to save webhook sync state annotation, rolling back in-memory state to last persisted value")
		if previousState != nil {
			syncer.SetManagedRepos(previousState)
		}
	}
}

func (m *webhookSyncManager) readSecretKey(ctx context.Context, ref *api.RenovateSecretKeyReference, namespace string) (string, error) {
	if ref == nil {
		return "", fmt.Errorf("secret reference is nil")
	}
	secret := &corev1.Secret{}
	err := m.k8sClient.Get(ctx, client.ObjectKey{
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

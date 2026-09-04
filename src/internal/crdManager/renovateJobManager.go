package crdmanager

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	api "renovate-operator/api/v1alpha1"
	"renovate-operator/config"
	"renovate-operator/gitProviderClients"
	gitProviderClientFactory "renovate-operator/gitProviderClients/factory"
	"renovate-operator/internal/logStore"
	"renovate-operator/internal/podLogs"
	"renovate-operator/internal/policy"
	"renovate-operator/internal/types"
	"renovate-operator/internal/utils"
	"renovate-operator/internal/webhookSync"
	"renovate-operator/metricStore"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

/*
RenovateJobManager is the interface for managing RenovateJob CRDs.
It provides methods to list, get, and update RenovateJob CRDs and their associated projects.
This should be the only component interacting with RenovateJob CRDs directly.
*/
type RenovateJobManager interface {
	// ListRenovateJobs lists all RenovateJob CRDs in the cluster.
	ListRenovateJobs(ctx context.Context) ([]RenovateJobIdentifier, error)
	// ListRenovateJobsFull lists all RenovateJob CRDs in the cluster with full object data.
	ListRenovateJobsFull(ctx context.Context) ([]api.RenovateJob, error)
	// GetRenovateJob retrieves a specific RenovateJob CRD by name and namespace.
	GetRenovateJob(ctx context.Context, name string, namespace string) (*api.RenovateJob, error)
	// GetProjectsForRenovateJob retrieves all projects associated with a specific RenovateJob CRD.
	GetProjectsForRenovateJob(ctx context.Context, job RenovateJobIdentifier) ([]RenovateProjectStatus, error)
	// UpdateProjectStatus updates the status of a specific project within a RenovateJob CRD.
	UpdateProjectStatus(ctx context.Context, project string, job RenovateJobIdentifier, status *types.RenovateStatusUpdate) error
	// UpdateProjectStatusBatched updates the status of multiple projects within a RenovateJob CRD based on a filter function.
	UpdateProjectStatusBatched(ctx context.Context, fn func(p RenovateProjectStatus) bool, job RenovateJobIdentifier, status *types.RenovateStatusUpdate) error
	// GetProjectsByStatus retrieves all projects with a specific status within a RenovateJob CRD.
	GetProjectsByStatus(ctx context.Context, job RenovateJobIdentifier, status api.RenovateProjectStatus) ([]RenovateProjectStatus, error)
	// ReconcileProjects reconciles the list of projects in a RenovateJob CRD
	// with the provided list. It returns the names of the projects that were
	// removed (present before, absent now).
	ReconcileProjects(ctx context.Context, job *api.RenovateJob, projects []string) ([]string, error)
	// SyncWebhooks ensures the operator's webhook exists on every project of
	// the RenovateJob and removes it from the given removed projects (the diff
	// reported by ReconcileProjects). Stateless: hooks are identified by their
	// delivery URL on the platform.
	SyncWebhooks(ctx context.Context, job RenovateJobIdentifier, removedProjects []string) error
	// CleanupWebhooks removes the operator's webhook from every project of the
	// RenovateJob. Called by the deletion finalizer.
	CleanupWebhooks(ctx context.Context, job RenovateJobIdentifier) error
	// StreamLogsForProject returns an io.ReadCloser that streams NDJSON log lines for the given
	// project. For running pods Follow is true so the stream stays open until the container exits.
	// For completed pods or log-store fallback the stream closes after all content is delivered.
	// The lock is released before returning — callers read outside the lock.
	StreamLogsForProject(ctx context.Context, job RenovateJobIdentifier, project string) (io.ReadCloser, error)
	// IsWebhookTokenValid checks if the provided token is valid for the webhook of the specified RenovateJob CRD.
	IsWebhookTokenValid(ctx context.Context, job RenovateJobIdentifier, token string) (bool, error)
	// IsWebhookSignatureValid checks if the provided signature is valid for the webhook of the specified RenovateJob CRD.
	IsWebhookSignatureValid(ctx context.Context, job RenovateJobIdentifier, signature string, body []byte) (bool, error)
	// IsWebhookStandardSignatureValid checks a Standard Webhooks signature (https://www.standardwebhooks.com/)
	// against the webhook signing keys configured for the specified RenovateJob CRD. Standard Webhooks is a
	// vendor-neutral signing scheme; GitLab "signing tokens" are one implementation of it, not the only one.
	IsWebhookStandardSignatureValid(ctx context.Context, job RenovateJobIdentifier, msgID, timestamp, signature string, body []byte) (bool, error)
	// SetAcceptedCondition records whether the RenovateJob satisfies the operator's
	// policy, so a refusal is visible on the resource rather than only in the log.
	SetAcceptedCondition(ctx context.Context, job RenovateJobIdentifier, accepted bool, reason string, message string) error
	// CancelProjectJob deletes the running executor Kubernetes Job for the given project and
	// transitions its CRD status to cancelled, freeing the slot for the next dispatch.
	CancelProjectJob(ctx context.Context, project string, job RenovateJobIdentifier) error
}

var ErrProjectNotFound = errors.New("project not found")

type renovateJobManager struct {
	client                   client.Client
	gitProviderClientFactory gitProviderClientFactory.GitProviderClientFactory
	logger                   logr.Logger
	lock                     *sync.RWMutex
	logStore                 logStore.LogStore
	logReader                podLogs.PodLogReader
	policy                   policy.Policy
}

type RenovateJobIdentifier struct {
	Name      string
	Namespace string
}

func (in *RenovateJobIdentifier) Fullname() string {
	return in.Name + "-" + in.Namespace
}

type RenovateProjectStatus struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	api.RenovateProjectState
}

func NewRenovateJobManager(client client.Client, gitProviderClientFactory gitProviderClientFactory.GitProviderClientFactory, logger logr.Logger, ls logStore.LogStore, lr podLogs.PodLogReader, p policy.Policy) RenovateJobManager {
	return &renovateJobManager{
		client:                   client,
		gitProviderClientFactory: gitProviderClientFactory,
		logger:                   logger,
		lock:                     &sync.RWMutex{},
		logStore:                 ls,
		logReader:                lr,
		policy:                   p,
	}
}

// globally lock the manager, if parameter is true, lock in read mode
func (r *renovateJobManager) globalManagerLock(readonly bool) func() {
	if readonly {
		r.lock.RLock()
		return func() {
			r.lock.RUnlock()
		}
	}

	r.lock.Lock()
	return func() {
		r.lock.Unlock()
	}
}

func (r *renovateJobManager) GetRenovateJob(ctx context.Context, name string, namespace string) (*api.RenovateJob, error) {
	defer r.globalManagerLock(true)()

	return loadRenovateJob(ctx, name, namespace, r.client)
}

// toRenovateProjectStatus converts a RenovateProject CRD object into the internal DTO.
func toRenovateProjectStatus(p *api.RenovateProject) RenovateProjectStatus {
	return RenovateProjectStatus{
		Name:                 p.Spec.Project,
		Namespace:            p.Namespace,
		RenovateProjectState: p.Status,
	}
}

func (r *renovateJobManager) GetProjectsByStatus(ctx context.Context, job RenovateJobIdentifier, status api.RenovateProjectStatus) ([]RenovateProjectStatus, error) {
	defer r.globalManagerLock(true)()

	var projectList api.RenovateProjectList
	if err := r.client.List(ctx, &projectList,
		client.InNamespace(job.Namespace),
		client.MatchingLabels{api.LabelRenovateJob: job.Name},
	); err != nil {
		return nil, err
	}

	result := make([]RenovateProjectStatus, 0)
	for i := range projectList.Items {
		if projectList.Items[i].Status.Status == status {
			result = append(result, toRenovateProjectStatus(&projectList.Items[i]))
		}
	}
	return result, nil
}

func (r *renovateJobManager) GetProjectsForRenovateJob(ctx context.Context, job RenovateJobIdentifier) ([]RenovateProjectStatus, error) {
	defer r.globalManagerLock(true)()

	var projectList api.RenovateProjectList
	if err := r.client.List(ctx, &projectList,
		client.InNamespace(job.Namespace),
		client.MatchingLabels{api.LabelRenovateJob: job.Name},
	); err != nil {
		return nil, err
	}

	result := make([]RenovateProjectStatus, 0, len(projectList.Items))
	for i := range projectList.Items {
		result = append(result, toRenovateProjectStatus(&projectList.Items[i]))
	}
	return result, nil
}

func (r *renovateJobManager) ListRenovateJobs(ctx context.Context) ([]RenovateJobIdentifier, error) {
	defer r.globalManagerLock(true)()

	var renovateJobs api.RenovateJobList
	err := r.client.List(ctx, &renovateJobs)
	if err != nil {
		return nil, err
	}

	result := make([]RenovateJobIdentifier, 0)
	for _, renovateJob := range renovateJobs.Items {
		result = append(result, RenovateJobIdentifier{
			Name:      renovateJob.Name,
			Namespace: renovateJob.Namespace,
		})
	}

	return result, nil
}

func (r *renovateJobManager) ListRenovateJobsFull(ctx context.Context) ([]api.RenovateJob, error) {
	defer r.globalManagerLock(true)()

	var renovateJobs api.RenovateJobList
	err := r.client.List(ctx, &renovateJobs)
	if err != nil {
		return nil, err
	}

	return renovateJobs.Items, nil
}

func (r *renovateJobManager) UpdateProjectStatus(ctx context.Context, project string, job RenovateJobIdentifier, status *types.RenovateStatusUpdate) error {
	defer r.globalManagerLock(false)()

	rpName := utils.RenovateProjectCRDName(job.Name, project)

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var rp api.RenovateProject
		if err := r.client.Get(ctx, client.ObjectKey{
			Name:      rpName,
			Namespace: job.Namespace,
		}, &rp); err != nil {
			if apierrors.IsNotFound(err) {
				return ErrProjectNotFound
			}
			return err
		}

		utils.GetUpdateStatusForProject(&rp.Status, status)

		return r.client.Status().Update(ctx, &rp)
	})
}

func (r *renovateJobManager) UpdateProjectStatusBatched(ctx context.Context, fn func(p RenovateProjectStatus) bool, job RenovateJobIdentifier, status *types.RenovateStatusUpdate) error {
	defer r.globalManagerLock(false)()

	var projectList api.RenovateProjectList
	if err := r.client.List(ctx, &projectList,
		client.InNamespace(job.Namespace),
		client.MatchingLabels{api.LabelRenovateJob: job.Name},
	); err != nil {
		return err
	}

	for i := range projectList.Items {
		rp := &projectList.Items[i]
		ps := toRenovateProjectStatus(rp)

		if !fn(ps) {
			continue
		}

		rpName := rp.Name
		rpNamespace := rp.Namespace
		if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			var fresh api.RenovateProject
			if err := r.client.Get(ctx, client.ObjectKey{
				Name:      rpName,
				Namespace: rpNamespace,
			}, &fresh); err != nil {
				return err
			}
			utils.GetUpdateStatusForProject(&fresh.Status, status)
			return r.client.Status().Update(ctx, &fresh)
		}); err != nil {
			return err
		}
	}

	return nil
}

// buildRenovateProject constructs a new RenovateProject object owned by the given RenovateJob.
func buildRenovateProject(job *api.RenovateJob, project string) *api.RenovateProject {
	controller := true
	blockOwnerDeletion := true
	return &api.RenovateProject{
		Name:      utils.RenovateProjectCRDName(job.Name, project),
		Namespace: job.Namespace,
		Labels: map[string]string{
			api.LabelRenovateJob:  job.Name,
			api.LabelProject:      utils.KubernetesCompatibleProjectName(project),
			api.LabelAppManagedBy: api.LabelValueManagedBy,
		},
		OwnerReferences: []v1.OwnerReference{
			{
				APIVersion:         api.GroupVersion.String(),
				Kind:               "RenovateJob",
				Name:               job.Name,
				UID:                job.UID,
				Controller:         &controller,
				BlockOwnerDeletion: &blockOwnerDeletion,
			},
		},
		Spec: api.RenovateProjectSpec{
			Project: project,
		},
	}
}

func (r *renovateJobManager) ReconcileProjects(ctx context.Context, renovateJob *api.RenovateJob, projects []string) ([]string, error) {

	if (renovateJob.Spec.SkipForks || renovateJob.Spec.SkipPendingDeletion) && r.gitProviderClientFactory != nil {
		providerClient, err := r.gitProviderClientFactory.NewClient(ctx, renovateJob)
		if err != nil {
			r.logger.Error(err, "Failed to create git provider client for project filtering")
		} else {
			newProjects, stats, err := gitProviderClients.FilterProjects(ctx, providerClient, r.logger, projects, renovateJob.Spec.SkipForks, renovateJob.Spec.SkipPendingDeletion)
			if err != nil {
				r.logger.Error(err, "Failed to filter discovered repositories")
			} else {
				r.logger.V(2).Info("Filtered discovered repositories", "remaining", len(newProjects))
				projects = newProjects
				metricStore.AddRepositoriesFiltered(ctx, renovateJob.Namespace, renovateJob.Name, "fork", stats.ForksRemoved)
				metricStore.AddRepositoriesFiltered(ctx, renovateJob.Namespace, renovateJob.Name, "pending_deletion", stats.PendingRemoved)
			}
		}
	}

	defer r.globalManagerLock(false)()

	// List existing RenovateProject objects for this job.
	var existing api.RenovateProjectList
	if err := r.client.List(ctx, &existing,
		client.InNamespace(renovateJob.Namespace),
		client.MatchingLabels{api.LabelRenovateJob: renovateJob.Name},
	); err != nil {
		return nil, fmt.Errorf("failed to list RenovateProjects: %w", err)
	}

	existingByName := make(map[string]*api.RenovateProject, len(existing.Items))
	for i := range existing.Items {
		existingByName[existing.Items[i].Spec.Project] = &existing.Items[i]
	}

	newProjectSet := make(map[string]struct{}, len(projects))
	for _, p := range projects {
		newProjectSet[p] = struct{}{}
	}

	// Delete RenovateProject objects for repos that are no longer discovered.
	var removed []string
	for name, rp := range existingByName {
		if _, exists := newProjectSet[name]; !exists {
			removed = append(removed, name)
			metricStore.DeleteProjectMetrics(renovateJob.Namespace, renovateJob.Name, name)
			if err := r.client.Delete(ctx, rp); err != nil && !apierrors.IsNotFound(err) {
				r.logger.Error(err, "failed to delete RenovateProject", "project", name)
			}
		}
	}

	// Create RenovateProject objects for newly discovered repos.
	for _, project := range projects {
		if _, exists := existingByName[project]; exists {
			continue
		}
		rp := buildRenovateProject(renovateJob, project)
		if err := r.client.Create(ctx, rp); err != nil {
			if apierrors.IsAlreadyExists(err) {
				continue
			}
			return nil, fmt.Errorf("failed to create RenovateProject for %s: %w", project, err)
		}
		rp.Status = api.RenovateProjectState{
			Status:         api.JobStatusScheduled,
			LastTransition: v1.Now(),
		}
		if err := r.client.Status().Update(ctx, rp); err != nil {
			r.logger.Error(err, "failed to set initial status for RenovateProject", "project", project)
		}
	}

	return removed, nil
}

func (r *renovateJobManager) SyncWebhooks(ctx context.Context, job RenovateJobIdentifier, removedProjects []string) error {
	unlock := r.globalManagerLock(true)
	renovateJob, err := loadRenovateJob(ctx, job.Name, job.Namespace, r.client)
	var projectList api.RenovateProjectList
	var listErr error
	if err == nil {
		listErr = r.client.List(ctx, &projectList,
			client.InNamespace(job.Namespace),
			client.MatchingLabels{api.LabelRenovateJob: job.Name},
		)
	}
	unlock()
	if err != nil {
		return fmt.Errorf("failed to load renovate job: %w", err)
	}
	if listErr != nil {
		r.logger.Error(listErr, "failed to list RenovateProjects for webhook sync")
	}

	webhook := renovateJob.Spec.Webhook
	if webhook == nil || webhook.Sync == nil {
		return nil
	}
	syncEnabled := webhook.Enabled && webhook.Sync.Enabled

	current := make([]string, 0, len(projectList.Items))
	for _, p := range projectList.Items {
		current = append(current, p.Spec.Project)
	}
	var desired, removed []string
	if syncEnabled {
		desired = current
		removed = removedProjects
	} else {
		removed = append(current, removedProjects...)
	}
	if len(desired) == 0 && len(removed) == 0 {
		return nil
	}

	return r.runWebhookSync(ctx, renovateJob, job, desired, removed)
}

func (r *renovateJobManager) CleanupWebhooks(ctx context.Context, job RenovateJobIdentifier) error {
	unlock := r.globalManagerLock(true)
	renovateJob, err := loadRenovateJob(ctx, job.Name, job.Namespace, r.client)
	var projectList api.RenovateProjectList
	var listErr error
	if err == nil {
		listErr = r.client.List(ctx, &projectList,
			client.InNamespace(job.Namespace),
			client.MatchingLabels{api.LabelRenovateJob: job.Name},
		)
	}
	unlock()
	if err != nil {
		return fmt.Errorf("failed to load renovate job: %w", err)
	}
	if listErr != nil {
		r.logger.Error(listErr, "failed to list RenovateProjects for webhook cleanup")
	}

	removed := make([]string, 0, len(projectList.Items))
	for _, p := range projectList.Items {
		removed = append(removed, p.Spec.Project)
	}
	if len(removed) == 0 {
		return nil
	}
	return r.runWebhookSync(ctx, renovateJob, job, nil, removed)
}

// runWebhookSync builds the provider client and delivery URL, then runs one
// webhook sync cycle over the given project sets.
func (r *renovateJobManager) runWebhookSync(ctx context.Context, renovateJob *api.RenovateJob, job RenovateJobIdentifier, desired, removed []string) error {
	webhook := renovateJob.Spec.Webhook

	var gitProvider gitProviderClients.GitProviderClient
	var err error
	if webhook != nil && webhook.Sync != nil && webhook.Sync.SecretRef != nil {
		gitProvider, err = r.gitProviderClientFactory.NewClientWithTokenRef(ctx, renovateJob, webhook.Sync.SecretRef)
	} else {
		gitProvider, err = r.gitProviderClientFactory.NewClient(ctx, renovateJob)
	}
	if err != nil {
		return fmt.Errorf("failed to create git provider client: %w", err)
	}

	rawURL, err := webhookURLForJob(renovateJob)
	if err != nil {
		return err
	}
	// Only writes are gated. For a removal the delivery URL is matching input, not
	// a destination, and refusing it would strand exactly the hook that a hostile
	// baseUrl created.
	if len(desired) > 0 {
		if err := r.policy.ValidateDestination(rawURL, "spec.webhook.baseUrl"); err != nil {
			metricStore.IncPolicyDenial(ctx, "destination")
			return fmt.Errorf("refusing to write webhooks: %w", err)
		}
	}
	webhookURL, err := buildWebhookURL(rawURL, job)
	if err != nil {
		return fmt.Errorf("failed to parse webhookURL: %w", err)
	}

	var authToken string
	if len(desired) > 0 && webhook != nil && webhook.Authentication != nil && webhook.Authentication.Enabled && webhook.Authentication.SecretRef != nil {
		tokens, err := r.getRenovateJobTokens(ctx, renovateJob)
		if err != nil {
			return fmt.Errorf("failed to read webhook auth token: %w", err)
		}
		if len(tokens) > 0 {
			authToken = tokens[0]
		}
	}

	opts := webhookSync.Options{
		WebhookURL: webhookURL,
		AuthToken:  authToken,
	}
	webhookSync.Sync(ctx, r.logger.WithName("webhook-sync"), gitProvider, opts, desired, removed)
	return nil
}

func webhookURLForJob(renovateJob *api.RenovateJob) (string, error) {
	baseURL := ""
	if renovateJob.Spec.Webhook != nil {
		baseURL = renovateJob.Spec.Webhook.BaseURL
	}
	if baseURL == "" {
		baseURL = config.GetValue("WEBHOOK_BASE_URL")
	}
	if baseURL == "" {
		return "", fmt.Errorf("webhook delivery URL is unknown: set spec.webhook.baseUrl on the RenovateJob, or expose the webhook via the chart's webhook.route/webhook.ingress (WEBHOOK_BASE_URL)")
	}
	platform, _ := utils.GetPlatformAndEndpoint(renovateJob.Spec.Provider)
	path, err := utils.WebhookEndpointPath(platform)
	if err != nil {
		return "", fmt.Errorf("failed to derive webhook URL: %w", err)
	}
	return strings.TrimSuffix(baseURL, "/") + path, nil
}

// buildWebhookURL appends the namespace/job query parameters that the webhook
// server uses to route incoming events to the right RenovateJob.
func buildWebhookURL(rawURL string, job RenovateJobIdentifier) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	q := parsed.Query()
	q.Set("namespace", job.Namespace)
	q.Set("job", job.Name)
	parsed.RawQuery = q.Encode()
	return parsed.String(), nil
}

func (r *renovateJobManager) StreamLogsForProject(ctx context.Context, job RenovateJobIdentifier, project string) (io.ReadCloser, error) {
	// Phase 1: hold the read lock only for CRD + k8s Job metadata lookup.
	unlock := r.globalManagerLock(true)

	rpName := utils.RenovateProjectCRDName(job.Name, project)
	var rp api.RenovateProject
	projectRunning := false
	if err := r.client.Get(ctx, client.ObjectKey{Name: rpName, Namespace: job.Namespace}, &rp); err == nil {
		projectRunning = rp.Status.Status == api.JobStatusRunning
	}

	executorJob, jobErr := GetJobByLabel(ctx, r.client, JobSelector{
		JobType:         ExecutorJobType,
		Namespace:       job.Namespace,
		RenovateJobName: job.Name,
		Project:         project,
	})

	unlock() // released before any streaming I/O so the lock is never held across a long-lived connection

	// Phase 2: open the stream (no lock held).
	if jobErr == nil {
		stream, err := r.logReader.StreamJobLogs(ctx, executorJob, projectRunning)
		if err == nil {
			return stream, nil
		}
		// Pod is gone — fall through to log store only if the job is not running.
		if projectRunning {
			return nil, fmt.Errorf("failed to get pod logs for running project: %w", err)
		}
	} else if projectRunning {
		return nil, fmt.Errorf("failed to get job for running project: %w", jobErr)
	}

	// Job or pod not available and project is not running — try the log store.
	if logs, ok := r.logStore.Get(job.Namespace, job.Name, project); ok {
		return io.NopCloser(strings.NewReader(logs)), nil
	}

	return nil, fmt.Errorf("logs not available: pod has been cleaned up and no cached logs found")
}

func (r *renovateJobManager) getRenovateJobTokens(ctx context.Context, job *api.RenovateJob) ([]string, error) {
	secret := &corev1.Secret{}
	err := r.client.Get(ctx, client.ObjectKey{
		Name:      job.Spec.Webhook.Authentication.SecretRef.Name,
		Namespace: job.Namespace,
	}, secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			metricStore.IncSecretResolutionError(ctx, "not_found")
		} else {
			metricStore.IncSecretResolutionError(ctx, "api_error")
		}
		return nil, err
	}

	if err := r.policy.ValidateReferencedSecret(secret); err != nil {
		metricStore.IncPolicyDenial(ctx, "secret_ref")
		return nil, err
	}

	authData, exists := secret.Data[job.Spec.Webhook.Authentication.SecretRef.Key]
	if !exists {
		metricStore.IncSecretResolutionError(ctx, "key_missing")
		return nil, fmt.Errorf("secret key %s not found in secret %s", job.Spec.Webhook.Authentication.SecretRef.Key, job.Spec.Webhook.Authentication.SecretRef.Name)
	}

	allTokens := string(authData)
	tokens := strings.Split(allTokens, ",")
	return tokens, nil
}

func (r *renovateJobManager) IsWebhookTokenValid(ctx context.Context, job RenovateJobIdentifier, token string) (bool, error) {
	defer r.globalManagerLock(true)()

	renovateJob, err := loadRenovateJob(ctx, job.Name, job.Namespace, r.client)
	if err != nil {
		return false, err
	}

	if renovateJob.Spec.Webhook == nil ||
		renovateJob.Spec.Webhook.Authentication == nil ||
		!renovateJob.Spec.Webhook.Authentication.Enabled {
		// Webhook authentication is not enabled
		return false, nil
	}
	tokens, err := r.getRenovateJobTokens(ctx, renovateJob)
	if err != nil {
		return false, err
	}
	if slices.Contains(tokens, token) {
		return true, nil
	}

	return false, nil
}

func (r *renovateJobManager) IsWebhookSignatureValid(ctx context.Context, job RenovateJobIdentifier, signature string, body []byte) (bool, error) {
	defer r.globalManagerLock(true)()

	renovateJob, err := loadRenovateJob(ctx, job.Name, job.Namespace, r.client)
	if err != nil {
		return false, err
	}

	if renovateJob.Spec.Webhook == nil ||
		renovateJob.Spec.Webhook.Authentication == nil ||
		!renovateJob.Spec.Webhook.Authentication.Enabled {
		// Webhook authentication is not enabled
		return false, nil
	}

	tokens, err := r.getRenovateJobTokens(ctx, renovateJob)
	if err != nil {
		return false, err
	}
	for _, token := range tokens {
		expectedSignature := computeHMAC256(body, token)

		if hmac.Equal([]byte(signature), []byte(expectedSignature)) {
			return true, nil
		}
	}

	return false, nil
}

// IsWebhookStandardSignatureValid validates a Standard Webhooks signature against the keys configured for
// the RenovateJob. Standard Webhooks (https://www.standardwebhooks.com/) is a vendor-neutral webhook
// signing scheme implemented by multiple providers — GitLab "signing tokens" among them — so this path is
// not GitLab-specific and works for any compliant sender. The signed content is "{msgID}.{timestamp}.{body}",
// keyed by the HMAC-SHA256 key decoded from each configured secret. The signature header is a space-separated
// list of "v1,<base64>" entries; a match against any entry for any configured key authenticates the request.
// The timestamp must be within standardWebhookTimestampTolerance of now to reject replayed requests.
func (r *renovateJobManager) IsWebhookStandardSignatureValid(ctx context.Context, job RenovateJobIdentifier, msgID, timestamp, signature string, body []byte) (bool, error) {
	defer r.globalManagerLock(true)()

	renovateJob, err := loadRenovateJob(ctx, job.Name, job.Namespace, r.client)
	if err != nil {
		return false, err
	}

	if renovateJob.Spec.Webhook == nil ||
		renovateJob.Spec.Webhook.Authentication == nil ||
		!renovateJob.Spec.Webhook.Authentication.Enabled {
		// Webhook authentication is not enabled
		return false, nil
	}

	if msgID == "" || signature == "" {
		return false, nil
	}
	if !isStandardWebhookTimestampFresh(timestamp, time.Now()) {
		r.logger.V(1).Info("rejecting webhook: signature timestamp outside tolerance", "namespace", job.Namespace, "name", job.Name, "timestamp", timestamp)
		return false, nil
	}

	tokens, err := r.getRenovateJobTokens(ctx, renovateJob)
	if err != nil {
		return false, err
	}

	signedContent := msgID + "." + timestamp + "." + string(body)
	for _, token := range tokens {
		key, ok := decodeStandardWebhookSigningKey(token)
		if !ok {
			continue
		}
		expected := computeStandardWebhookSignature(key, signedContent)
		if matchesAnyStandardWebhookSignature(signature, expected) {
			return true, nil
		}
	}

	return false, nil
}

func (r *renovateJobManager) CancelProjectJob(ctx context.Context, project string, job RenovateJobIdentifier) error {
	executorJob, err := GetJobByLabel(ctx, r.client, JobSelector{
		JobType:         ExecutorJobType,
		Namespace:       job.Namespace,
		Project:         project,
		RenovateJobName: job.Name,
	})
	if err == nil && executorJob != nil {
		if delErr := DeleteJob(ctx, r.client, executorJob); delErr != nil {
			return fmt.Errorf("failed to delete executor job: %w", delErr)
		}
	}

	return r.UpdateProjectStatus(ctx, project, job, &types.RenovateStatusUpdate{
		Status: api.JobStatusCancelled,
	})
}

func (r *renovateJobManager) SetAcceptedCondition(ctx context.Context, job RenovateJobIdentifier, accepted bool, reason string, message string) error {
	defer r.globalManagerLock(false)()

	status := v1.ConditionTrue
	if !accepted {
		status = v1.ConditionFalse
	}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		renovateJob, err := loadRenovateJob(ctx, job.Name, job.Namespace, r.client)
		if err != nil {
			return err
		}

		condition := v1.Condition{
			Type:               api.ConditionAccepted,
			Status:             status,
			Reason:             reason,
			Message:            message,
			ObservedGeneration: renovateJob.Generation,
		}

		// The reconciler runs on a one-minute requeue, so writing unconditionally
		// would rewrite the status and bump resourceVersion on every tick forever.
		// SetStatusCondition reports whether anything actually changed.
		if !meta.SetStatusCondition(&renovateJob.Status.Conditions, condition) {
			return nil
		}
		return r.client.Status().Update(ctx, renovateJob)
	})
}

func computeHMAC256(message []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(message)
	expectedMAC := mac.Sum(nil)
	return "sha256=" + fmt.Sprintf("%x", expectedMAC)
}

// standardWebhookTimestampTolerance bounds how far a webhook-timestamp may drift from the current
// time before the request is rejected as a potential replay. Matches the Standard Webhooks default.
const standardWebhookTimestampTolerance = 5 * time.Minute

// decodeStandardWebhookSigningKey returns the raw HMAC key for a Standard Webhooks signing secret. The
// canonical form is "whsec_" + base64(key), as issued by Standard Webhooks senders (GitLab among them).
// A bare value is base64-decoded when possible, otherwise used verbatim as the key.
func decodeStandardWebhookSigningKey(secret string) ([]byte, bool) {
	if secret == "" {
		return nil, false
	}
	if rest, found := strings.CutPrefix(secret, "whsec_"); found {
		decoded, err := base64.StdEncoding.DecodeString(rest)
		if err != nil {
			return nil, false
		}
		return decoded, true
	}
	if decoded, err := base64.StdEncoding.DecodeString(secret); err == nil {
		return decoded, true
	}
	return []byte(secret), true
}

// computeStandardWebhookSignature returns the base64 HMAC-SHA256 of signedContent, without the
// "v1," version prefix.
func computeStandardWebhookSignature(key []byte, signedContent string) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(signedContent))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// matchesAnyStandardWebhookSignature reports whether expected (raw base64) matches any "v1" entry
// in a space-separated webhook-signature header value. Comparison is constant-time.
func matchesAnyStandardWebhookSignature(header, expected string) bool {
	for part := range strings.FieldsSeq(header) {
		version, sig, found := strings.Cut(part, ",")
		if !found || version != "v1" {
			continue
		}
		if hmac.Equal([]byte(sig), []byte(expected)) {
			return true
		}
	}
	return false
}

// isStandardWebhookTimestampFresh reports whether a unix-seconds timestamp is within the replay
// tolerance of now.
func isStandardWebhookTimestampFresh(timestamp string, now time.Time) bool {
	secs, err := strconv.ParseInt(strings.TrimSpace(timestamp), 10, 64)
	if err != nil {
		return false
	}
	delta := now.Sub(time.Unix(secs, 0))
	if delta < 0 {
		delta = -delta
	}
	return delta <= standardWebhookTimestampTolerance
}

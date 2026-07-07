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
	"renovate-operator/internal/types"
	"renovate-operator/internal/utils"
	"renovate-operator/internal/webhookSync"
	"renovate-operator/metricStore"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
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
	UpdateProjectStatusBatched(ctx context.Context, fn func(p api.ProjectStatus) bool, job RenovateJobIdentifier, status *types.RenovateStatusUpdate) error
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
	// UpdateExecutionOptions updates the execution options for the specified RenovateJob CRD.
	UpdateExecutionOptions(ctx context.Context, job RenovateJobIdentifier, options *api.RenovateExecutionOptions) error
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
}

type RenovateJobIdentifier struct {
	Name      string
	Namespace string
}

func (in *RenovateJobIdentifier) Fullname() string {
	return in.Name + "-" + in.Namespace
}

type RenovateProjectStatus struct {
	Name                 string                    `json:"name"`
	Status               api.RenovateProjectStatus `json:"status"`
	LastRun              time.Time                 `json:"lastRun"`
	Priority             int32                     `json:"priority,omitempty"`
	RenovateResultStatus *string                   `json:"renovateResultStatus,omitempty"`
	Duration             *string                   `json:"duration,omitempty"`
	PRActivity           *api.PRActivity           `json:"prActivity,omitempty"`
	LogIssues            *api.LogIssues            `json:"logIssues,omitempty"`
}

func NewRenovateJobManager(client client.Client, gitProviderClientFactory gitProviderClientFactory.GitProviderClientFactory, logger logr.Logger, ls logStore.LogStore, lr podLogs.PodLogReader) RenovateJobManager {
	return &renovateJobManager{
		client:                   client,
		gitProviderClientFactory: gitProviderClientFactory,
		logger:                   logger,
		lock:                     &sync.RWMutex{},
		logStore:                 ls,
		logReader:                lr,
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

func (r *renovateJobManager) GetProjectsByStatus(ctx context.Context, job RenovateJobIdentifier, status api.RenovateProjectStatus) ([]RenovateProjectStatus, error) {
	defer r.globalManagerLock(true)()

	renovateJob, err := loadRenovateJob(ctx, job.Name, job.Namespace, r.client)
	if err != nil {
		return nil, err
	}
	result := make([]RenovateProjectStatus, 0)
	for _, project := range renovateJob.Status.Projects {
		if project.Status == status {
			result = append(result, RenovateProjectStatus{
				Name:                 project.Name,
				Status:               project.Status,
				LastRun:              project.LastRun.Time,
				Priority:             project.Priority,
				RenovateResultStatus: project.RenovateResultStatus,
				Duration:             project.Duration,
				PRActivity:           project.PRActivity,
				LogIssues:            project.LogIssues,
			})
		}
	}
	return result, nil
}

func (r *renovateJobManager) GetProjectsForRenovateJob(ctx context.Context, job RenovateJobIdentifier) ([]RenovateProjectStatus, error) {
	defer r.globalManagerLock(true)()

	renovateJob, err := loadRenovateJob(ctx, job.Name, job.Namespace, r.client)
	if err != nil {
		return nil, err
	}
	result := make([]RenovateProjectStatus, 0)
	for _, project := range renovateJob.Status.Projects {
		result = append(result, RenovateProjectStatus{
			Name:                 project.Name,
			Status:               project.Status,
			LastRun:              project.LastRun.Time,
			Priority:             project.Priority,
			RenovateResultStatus: project.RenovateResultStatus,
			Duration:             project.Duration,
			PRActivity:           project.PRActivity,
			LogIssues:            project.LogIssues,
		})
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

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		renovateJob, err := loadRenovateJob(ctx, job.Name, job.Namespace, r.client)
		if err != nil {
			return err
		}
		index := -1
		for i := range renovateJob.Status.Projects {
			projectStatus := renovateJob.Status.Projects[i]
			if projectStatus.Name == project {
				index = i
				break
			}
		}
		if index == -1 {
			return ErrProjectNotFound
		}

		projectStatus := renovateJob.Status.Projects[index]
		renovateJob.Status.Projects[index] = *utils.GetUpdateStatusForProject(&projectStatus, status)

		return r.client.Status().Update(ctx, renovateJob)
	})
}

func (r *renovateJobManager) UpdateProjectStatusBatched(ctx context.Context, fn func(p api.ProjectStatus) bool, job RenovateJobIdentifier, status *types.RenovateStatusUpdate) error {
	defer r.globalManagerLock(false)()

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		renovateJob, err := loadRenovateJob(ctx, job.Name, job.Namespace, r.client)
		if err != nil {
			return err
		}

		for i := range renovateJob.Status.Projects {
			p := renovateJob.Status.Projects[i]

			if fn(p) {
				renovateJob.Status.Projects[i] = *utils.GetUpdateStatusForProject(&p, status)
			}
		}

		return r.client.Status().Update(ctx, renovateJob)
	})
}

func (r *renovateJobManager) ReconcileProjects(ctx context.Context, renovateJob *api.RenovateJob, projects []string) ([]string, error) {

	if (renovateJob.Spec.SkipForks || renovateJob.Spec.SkipPendingDeletion) && r.gitProviderClientFactory != nil {
		providerClient, err := r.gitProviderClientFactory.NewClient(ctx, renovateJob)
		if err != nil {
			r.logger.Error(err, "Failed to create git provider client for project filtering")
		} else {
			newProjects, err := gitProviderClients.FilterProjects(ctx, providerClient, r.logger, projects, renovateJob.Spec.SkipForks, renovateJob.Spec.SkipPendingDeletion)
			if err != nil {
				r.logger.Error(err, "Failed to filter discovered repositories")
			} else {
				r.logger.V(2).Info("Filtered discovered repositories", "remaining", len(newProjects))
				projects = newProjects
			}
		}
	}

	defer r.globalManagerLock(false)()

	var removed []string
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		renovateJob, err := loadRenovateJob(ctx, renovateJob.Name, renovateJob.Namespace, r.client)
		if err != nil {
			return err
		}
		// Build a set of current CRD projects
		crdProjectSet := make(map[string]api.ProjectStatus, len(renovateJob.Status.Projects))
		for i, crdProject := range renovateJob.Status.Projects {
			crdProjectSet[crdProject.Name] = renovateJob.Status.Projects[i]
		}

		// Build a set of new projects for quick lookup
		newProjectSet := make(map[string]struct{}, len(projects))
		for _, project := range projects {
			newProjectSet[project] = struct{}{}
		}

		// Collect removed projects (for webhook cleanup) and drop their metrics
		removed = removed[:0]
		for projectName := range crdProjectSet {
			if _, exists := newProjectSet[projectName]; !exists {
				removed = append(removed, projectName)
				metricStore.DeleteProjectMetrics(renovateJob.Namespace, renovateJob.Name, projectName)
			}
		}

		newProjects := make([]api.ProjectStatus, 0, len(projects))
		for _, project := range projects {
			if crdProject, exists := crdProjectSet[project]; exists {
				// add project that exist in the new project list
				newProjects = append(newProjects, crdProject)
			} else {
				// add new project to the list
				newProjects = append(newProjects, api.ProjectStatus{
					Name:    project,
					Status:  api.JobStatusScheduled,
					LastRun: v1.Now(),
				})
			}
		}
		renovateJob.Status.Projects = newProjects

		return r.client.Status().Update(ctx, renovateJob)
	})
	return removed, err
}

func (r *renovateJobManager) SyncWebhooks(ctx context.Context, job RenovateJobIdentifier, removedProjects []string) error {
	unlock := r.globalManagerLock(true)
	renovateJob, err := loadRenovateJob(ctx, job.Name, job.Namespace, r.client)
	unlock()
	if err != nil {
		return fmt.Errorf("failed to load renovate job: %w", err)
	}

	webhook := renovateJob.Spec.Webhook
	if webhook == nil || webhook.Sync == nil {
		return nil
	}
	syncEnabled := webhook.Enabled && webhook.Sync.Enabled

	// Reconcile toward the desired state: with sync enabled, hooks exist on all
	// current projects and are removed from projects that dropped out; with
	// sync disabled, hooks are removed from every project.
	current := make([]string, 0, len(renovateJob.Status.Projects))
	for _, project := range renovateJob.Status.Projects {
		current = append(current, project.Name)
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
	unlock()
	if err != nil {
		return fmt.Errorf("failed to load renovate job: %w", err)
	}

	removed := make([]string, 0, len(renovateJob.Status.Projects))
	for _, project := range renovateJob.Status.Projects {
		removed = append(removed, project.Name)
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
	baseURL := config.GetValue("WEBHOOK_BASE_URL")
	if baseURL == "" {
		return "", fmt.Errorf("webhook delivery URL is unknown: expose the webhook via the chart's webhook.route/webhook.ingress (WEBHOOK_BASE_URL)")
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

	renovateJob, err := loadRenovateJob(ctx, job.Name, job.Namespace, r.client)
	if err != nil {
		unlock()
		return nil, fmt.Errorf("failed to load renovate job: %w", err)
	}

	projectRunning := false
	for _, p := range renovateJob.Status.Projects {
		if p.Name == project && p.Status == api.JobStatusRunning {
			projectRunning = true
			break
		}
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
		return nil, err
	}

	authData, exists := secret.Data[job.Spec.Webhook.Authentication.SecretRef.Key]
	if !exists {
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

func (r *renovateJobManager) UpdateExecutionOptions(ctx context.Context, job RenovateJobIdentifier, options *api.RenovateExecutionOptions) error {
	defer r.globalManagerLock(false)()

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		renovateJob, err := loadRenovateJob(ctx, job.Name, job.Namespace, r.client)
		if err != nil {
			return err
		}
		renovateJob.Status.ExecutionOptions = options

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
	for _, part := range strings.Fields(header) {
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

package crdmanager

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	api "renovate-operator/api/v1alpha1"
	"renovate-operator/clientProvider"
	"renovate-operator/gitProviderClients"
	gitProviderClientFactory "renovate-operator/gitProviderClients/factory"
	"renovate-operator/internal/logStore"
	"renovate-operator/internal/types"
	"renovate-operator/internal/utils"
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
	// ReconcileProjects reconciles the list of projects in a RenovateJob CRD with the provided list.
	ReconcileProjects(ctx context.Context, job *api.RenovateJob, projects []string) error
	// GetLogsForProject retrieves the logs for a specific project within a RenovateJob CRD.
	GetLogsForProject(ctx context.Context, job RenovateJobIdentifier, project string) (string, error)
	// IsWebhookTokenValid checks if the provided token is valid for the webhook of the specified RenovateJob CRD.
	IsWebhookTokenValid(ctx context.Context, job RenovateJobIdentifier, token string) (bool, error)
	// IsWebhookSignatureValid checks if the provided signature is valid for the webhook of the specified RenovateJob CRD.
	IsWebhookSignatureValid(ctx context.Context, job RenovateJobIdentifier, signature string, body []byte) (bool, error)
	// UpdateExecutionOptions updates the execution options for the specified RenovateJob CRD.
	UpdateExecutionOptions(ctx context.Context, job RenovateJobIdentifier, options *api.RenovateExecutionOptions) error
	// CancelProjectJob deletes the running executor Kubernetes Job for the given project and
	// transitions its CRD status to cancelled, freeing the slot for the next dispatch.
	CancelProjectJob(ctx context.Context, project string, job RenovateJobIdentifier) error
}

type renovateJobManager struct {
	client                   client.Client
	gitProviderClientFactory gitProviderClientFactory.GitProviderClientFactory
	logger                   logr.Logger
	lock                     *sync.RWMutex
	logStore                 logStore.LogStore
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
	LastRun              time.Time                 `json:"lastRun,omitempty"`
	Priority             int32                     `json:"priority,omitempty"`
	RenovateResultStatus *string                   `json:"renovateResultStatus,omitempty"`
	Duration             *string                   `json:"duration,omitempty"`
	PRActivity           *api.PRActivity           `json:"prActivity,omitempty"`
	LogIssues            *api.LogIssues            `json:"logIssues,omitempty"`
}

func NewRenovateJobManager(client client.Client, gitProviderClientFactory gitProviderClientFactory.GitProviderClientFactory, logger logr.Logger, ls logStore.LogStore) RenovateJobManager {
	return &renovateJobManager{
		client:                   client,
		gitProviderClientFactory: gitProviderClientFactory,
		logger:                   logger,
		lock:                     &sync.RWMutex{},
		logStore:                 ls,
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
			projectStatus := &api.ProjectStatus{
				Name:     project,
				Status:   status.Status,
				Priority: status.Priority,
			}
			renovateJob.Status.Projects = append(renovateJob.Status.Projects, *projectStatus)
		} else {
			projectStatus := renovateJob.Status.Projects[index]
			renovateJob.Status.Projects[index] = *utils.GetUpdateStatusForProject(&projectStatus, status)
		}
		_, err = updateRenovateJobStatus(ctx, renovateJob, r.client)
		return err
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

		_, err = updateRenovateJobStatus(ctx, renovateJob, r.client)
		return err
	})
}

func (r *renovateJobManager) ReconcileProjects(ctx context.Context, renovateJob *api.RenovateJob, projects []string) error {

	if renovateJob.Spec.SkipForks && r.gitProviderClientFactory != nil {
		providerClient, err := r.gitProviderClientFactory.NewClient(ctx, renovateJob)
		if err != nil {
			r.logger.Error(err, "Failed to create git provider client for fork filtering")
		} else {
			newProjects, err := gitProviderClients.FilterForks(ctx, providerClient, r.logger, projects)
			if err != nil {
				r.logger.Error(err, "Failed to filter forked repositories")
			} else {
				r.logger.V(2).Info("Filtered forked repositories", "remaining", len(newProjects))
				projects = newProjects
			}
		}
	}

	defer r.globalManagerLock(false)()

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
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

		// Delete metrics for projects that are being removed
		for projectName := range crdProjectSet {
			if _, exists := newProjectSet[projectName]; !exists {
				// Project is being removed, clean up its metrics
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

		_, err = updateRenovateJobStatus(ctx, renovateJob, r.client)
		return err
	})
}

func (r *renovateJobManager) GetLogsForProject(ctx context.Context, job RenovateJobIdentifier, project string) (string, error) {
	defer r.globalManagerLock(true)()
	renovateJob, err := loadRenovateJob(ctx, job.Name, job.Namespace, r.client)
	if err != nil {
		return "", fmt.Errorf("failed to load renovate job: %w", err)
	}

	// Determine whether the project is currently running.
	projectRunning := false
	for _, p := range renovateJob.Status.Projects {
		if p.Name == project && p.Status == api.JobStatusRunning {
			projectRunning = true
			break
		}
	}

	executorJobName := utils.ExecutorJobName(renovateJob, project)

	executorJob, jobErr := GetJobByLabel(ctx, r.client, JobSelector{
		JobName:   executorJobName,
		JobType:   ExecutorJobType,
		Namespace: job.Namespace,
	})

	if jobErr == nil {
		cp := clientProvider.StaticClientProvider()
		clientset, err := cp.K8sClientSet()
		if err != nil {
			return "", fmt.Errorf("failed to create client: %w", err)
		}
		logs, err := GetLastJobLog(ctx, clientset, executorJob)
		if err == nil {
			return logs, nil
		}
		// Pod is gone — fall through to store only if job is not running.
		if projectRunning {
			return "", fmt.Errorf("failed to get pod logs for running project: %w", err)
		}
	} else if projectRunning {
		return "", fmt.Errorf("failed to get job for running project: %w", jobErr)
	}

	// Job or pod not available and project is not running — try the log store.
	if logs, ok := r.logStore.Get(job.Namespace, job.Name, project); ok {
		return logs, nil
	}

	return "", fmt.Errorf("logs not available: pod has been cleaned up and no cached logs found")
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

func (r *renovateJobManager) CancelProjectJob(ctx context.Context, project string, job RenovateJobIdentifier) error {
	stub := &api.RenovateJob{ObjectMeta: v1.ObjectMeta{Name: job.Name, Namespace: job.Namespace}}
	executorJob, err := GetJobByLabel(ctx, r.client, JobSelector{
		JobName:   utils.ExecutorJobName(stub, project),
		JobType:   ExecutorJobType,
		Namespace: job.Namespace,
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
		_, err = updateRenovateJobStatus(ctx, renovateJob, r.client)
		return err
	})
}

func computeHMAC256(message []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(message)
	expectedMAC := mac.Sum(nil)
	return "sha256=" + fmt.Sprintf("%x", expectedMAC)
}

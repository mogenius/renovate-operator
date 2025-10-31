package crdmanager

import (
	"context"
	"sync"

	api "renovate-operator/api/v1alpha1"
	"renovate-operator/clientProvider"
	"renovate-operator/internal/utils"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	// GetRenovateJob retrieves a specific RenovateJob CRD by name and namespace.
	GetRenovateJob(ctx context.Context, name string, namespace string) (*api.RenovateJob, error)
	// GetProjectsForRenovateJob retrieves all projects associated with a specific RenovateJob CRD.
	GetProjectsForRenovateJob(ctx context.Context, job RenovateJobIdentifier) ([]RenovateProjectStatus, error)
	// UpdateProjectStatus updates the status of a specific project within a RenovateJob CRD.
	UpdateProjectStatus(ctx context.Context, project string, job RenovateJobIdentifier, status api.RenovateProjectStatus) error
	// UpdateProjectStatusBatched updates the status of multiple projects within a RenovateJob CRD based on a filter function.
	UpdateProjectStatusBatched(ctx context.Context, fn func(p api.ProjectStatus) bool, job RenovateJobIdentifier, status api.RenovateProjectStatus) error
	// GetProjectsByStatus retrieves all projects with a specific status within a RenovateJob CRD.
	GetProjectsByStatus(ctx context.Context, job RenovateJobIdentifier, status api.RenovateProjectStatus) ([]RenovateProjectStatus, error)
	// ReconcileProjects reconciles the list of projects in a RenovateJob CRD with the provided list.
	ReconcileProjects(ctx context.Context, job RenovateJobIdentifier, projects []string) error
	// GetLogsForProject retrieves the logs for a specific project within a RenovateJob CRD.
	GetLogsForProject(ctx context.Context, job RenovateJobIdentifier, project string) (string, error)
}

type renovateJobManager struct {
	client client.Client
	lock   *sync.RWMutex
}

type RenovateJobIdentifier struct {
	Name      string
	Namespace string
}

func (in *RenovateJobIdentifier) Fullname() string {
	return in.Name + "-" + in.Namespace
}

type RenovateProjectStatus struct {
	Name   string                    `json:"name"`
	Status api.RenovateProjectStatus `json:"status"`
}

func NewRenovateJobManager(client client.Client) RenovateJobManager {
	return &renovateJobManager{
		client: client,
		lock:   &sync.RWMutex{},
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
				Name:   project.Name,
				Status: project.Status,
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
			Name:   project.Name,
			Status: project.Status,
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

func (r *renovateJobManager) UpdateProjectStatus(ctx context.Context, project string, job RenovateJobIdentifier, status api.RenovateProjectStatus) error {
	defer r.globalManagerLock(false)()

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
			Name:   project,
			Status: status,
		}
		renovateJob.Status.Projects = append(renovateJob.Status.Projects, *projectStatus)
	} else {
		projectStatus := renovateJob.Status.Projects[index]
		projectStatus.Status = utils.GetUpdateStatusForProject(projectStatus.Status, status)
		renovateJob.Status.Projects[index] = projectStatus
	}
	_, err = updateRenovateJobStatus(ctx, renovateJob, r.client)
	return err
}

func (r *renovateJobManager) UpdateProjectStatusBatched(ctx context.Context, fn func(p api.ProjectStatus) bool, job RenovateJobIdentifier, status api.RenovateProjectStatus) error {
	defer r.globalManagerLock(false)()

	renovateJob, err := loadRenovateJob(ctx, job.Name, job.Namespace, r.client)
	if err != nil {
		return err
	}

	for i := range renovateJob.Status.Projects {
		p := renovateJob.Status.Projects[i]

		if fn(p) {
			p.Status = utils.GetUpdateStatusForProject(p.Status, status)
			renovateJob.Status.Projects[i] = p
		}
	}

	_, err = updateRenovateJobStatus(ctx, renovateJob, r.client)
	return err
}

func (r *renovateJobManager) ReconcileProjects(ctx context.Context, job RenovateJobIdentifier, projects []string) error {
	defer r.globalManagerLock(false)()

	renovateJob, err := loadRenovateJob(ctx, job.Name, job.Namespace, r.client)
	if err != nil {
		return err
	}

	// Build a set of current CRD projects
	crdProjectSet := make(map[string]api.ProjectStatus, len(renovateJob.Status.Projects))
	for i, crdProject := range renovateJob.Status.Projects {
		crdProjectSet[crdProject.Name] = renovateJob.Status.Projects[i]
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
	if err != nil {
		return err
	}
	return nil
}

func (r *renovateJobManager) GetLogsForProject(ctx context.Context, job RenovateJobIdentifier, project string) (string, error) {
	defer r.globalManagerLock(true)()
	renovateJob, err := loadRenovateJob(ctx, job.Name, job.Namespace, r.client)
	if err != nil {
		return "failed to load renovate job", err
	}

	executorJobName := renovateJob.ExecutorJobName(project)

	executorJob, err := GetJob(ctx, r.client, executorJobName, job.Namespace)
	if err != nil {
		return "failed to get job", err
	}

	cp := clientProvider.StaticClientProvider()
	client, err := cp.K8sClientSet()
	if err != nil {
		return "failed to create client", err
	}

	logs, err := getLastJobLog(ctx, client, executorJob)

	return logs, err
}

package renovate

import (
	context "context"
	"fmt"
	api "renovate-operator/api/v1alpha1"
	crdManager "renovate-operator/internal/crdManager"
	"sync"
	"time"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type DiscoveryAgent interface {
	Discover(ctx context.Context, job *api.RenovateJob) ([]string, error)
	CreateDiscoveryJob(ctx context.Context, renovateJob api.RenovateJob) (*batchv1.Job, error)
	GetDiscoveryJobStatus(ctx context.Context, job *api.RenovateJob) (api.RenovateProjectStatus, error)
	WaitForDiscoveryJob(ctx context.Context, job *api.RenovateJob) ([]string, error)
}

type discoveryAgent struct {
	client client.Client
	logger logr.Logger
	scheme *runtime.Scheme
	syncer map[string]*sync.RWMutex
	// allow tests to override how logs are extracted
	getDiscoveredProjectsFromJobLogsFn func(ctx context.Context, c client.Client, job *batchv1.Job) ([]string, error)
	// allow tests to override how status is checked
	getDiscoveryJobStatusFn func(ctx context.Context, job *api.RenovateJob) (api.RenovateProjectStatus, error)
}

func NewDiscoveryAgent(scheme *runtime.Scheme, client client.Client, logger logr.Logger) DiscoveryAgent {
	da := &discoveryAgent{
		client: client,
		logger: logger,
		scheme: scheme,
		syncer: make(map[string]*sync.RWMutex),
	}
	// default to the internal implementation
	da.getDiscoveredProjectsFromJobLogsFn = da.getDiscoveredProjectsFromJobLogs
	da.getDiscoveryJobStatusFn = da.getDiscoveryJobStatusInternal
	return da
}

func (e *discoveryAgent) Discover(ctx context.Context, job *api.RenovateJob) ([]string, error) {
	name := job.Fullname()

	e.logger.Info("Discovering projects for RenovateJob", "job", name)
	return e.discoverIntern(ctx, job)
}

func (e *discoveryAgent) discoverIntern(ctx context.Context, job *api.RenovateJob) ([]string, error) {
	// 1. Create the discovery job - replaces existing job
	_, err := e.CreateDiscoveryJob(ctx, *job)
	if err != nil {
		return nil, fmt.Errorf("failed to create or get discovery job: %w", err)
	}

	return e.WaitForDiscoveryJob(ctx, job)
}

func (e *discoveryAgent) WaitForDiscoveryJob(ctx context.Context, job *api.RenovateJob) ([]string, error) {
	// 2. Wait for discovery job completion
	for {
		status, err := e.getDiscoveryJobStatusFn(ctx, job)

		if err != nil {
			return nil, fmt.Errorf("failed to get discovery job status: %w", err)
		}

		if status == api.JobStatusRunning {
			time.Sleep(5 * time.Second)
		} else if status == api.JobStatusCompleted {
			break
		} else if status == api.JobStatusFailed {
			return nil, fmt.Errorf("discovery job failed")
		}
	}

	// 3. Extract discovered projects from stdout
	existingDiscoveryJob := &batchv1.Job{}
	err := e.client.Get(ctx, types.NamespacedName{
		Name:      job.Name + "-discovery",
		Namespace: job.Namespace,
	}, existingDiscoveryJob)
	if err != nil {
		return nil, fmt.Errorf("failed to get discovery job status: %w", err)
	}
	projects, err := e.getDiscoveredProjectsFromJobLogsFn(ctx, e.client, existingDiscoveryJob)
	if err != nil {
		return nil, fmt.Errorf("failed to get discovered projects from job logs: %w", err)
	}
	e.logger.Info("Discovered projects", "count", len(projects))

	return projects, nil
}

// GetDiscoveryJobStatus implements DiscoveryAgent.
func (e *discoveryAgent) GetDiscoveryJobStatus(ctx context.Context, job *api.RenovateJob) (api.RenovateProjectStatus, error) {
	return e.getDiscoveryJobStatusFn(ctx, job)
}

// getDiscoveryJobStatusInternal is the internal implementation of GetDiscoveryJobStatus.
func (e *discoveryAgent) getDiscoveryJobStatusInternal(ctx context.Context, job *api.RenovateJob) (api.RenovateProjectStatus, error) {
	// lock based on the renovatejob
	name := job.Fullname()
	lock := e.syncer[name]
	if lock == nil {
		lock = &sync.RWMutex{}
		e.syncer[name] = lock
	}
	lock.RLock()
	defer lock.RUnlock()

	existingDiscoveryJob := &batchv1.Job{}
	err := e.client.Get(ctx, types.NamespacedName{
		Name:      job.Name + "-discovery",
		Namespace: job.Namespace,
	}, existingDiscoveryJob)
	if err != nil {
		return api.JobStatusFailed, fmt.Errorf("failed to get discovery job status: %w", err)
	}
	if existingDiscoveryJob.Status.Failed > 0 {
		return api.JobStatusFailed, nil
	}
	if existingDiscoveryJob.Status.Succeeded > 0 {
		return api.JobStatusCompleted, nil
	}
	return api.JobStatusRunning, nil
}
func (e *discoveryAgent) CreateDiscoveryJob(ctx context.Context, renovateJob api.RenovateJob) (*batchv1.Job, error) {
	// lock based on the renovatejob
	name := renovateJob.Fullname()
	lock := e.syncer[name]
	if lock == nil {
		lock = &sync.RWMutex{}
		e.syncer[name] = lock
	}
	lock.Lock()
	defer lock.Unlock()

	discoveryJob := newDiscoveryJob(&renovateJob)
	if err := controllerutil.SetControllerReference(&renovateJob, discoveryJob, e.scheme); err != nil {
		return &batchv1.Job{}, fmt.Errorf("failed to set controller reference: %w", err)
	}

	// check if the job exists, if so, delete it
	existingJob, err := crdManager.GetJob(ctx, e.client, discoveryJob.Name, discoveryJob.Namespace)
	if err == nil || !errors.IsNotFound(err) {
		_ = crdManager.DeleteJob(ctx, e.client, existingJob)
	}

	// Create the discovery job
	err = crdManager.CreateJob(ctx, e.client, discoveryJob)
	if err != nil {
		return &batchv1.Job{}, fmt.Errorf("failed to create discovery job: %w", err)
	}

	// Reload the job to ensure we have the latest state
	existingJob, err = crdManager.GetJob(ctx, e.client, discoveryJob.Name, discoveryJob.Namespace)
	if err != nil {
		return &batchv1.Job{}, fmt.Errorf("failed to get existing discovery job: %w", err)
	}
	return existingJob, nil
}

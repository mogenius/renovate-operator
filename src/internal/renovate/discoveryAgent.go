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
}

type discoveryAgent struct {
	client client.Client
	logger logr.Logger
	scheme *runtime.Scheme
	syncer map[string]*sync.Mutex
}

func NewDiscoveryAgent(scheme *runtime.Scheme, client client.Client, logger logr.Logger) DiscoveryAgent {
	return discoveryAgent{
		client: client,
		logger: logger,
		scheme: scheme,
		syncer: make(map[string]*sync.Mutex),
	}
}

func (e discoveryAgent) Discover(ctx context.Context, job *api.RenovateJob) ([]string, error) {
	// lock based on the renovatejob
	name := job.Fullname()
	lock := e.syncer[name]
	if lock == nil {
		lock = &sync.Mutex{}
		e.syncer[name] = lock
	}

	lock.Lock()
	defer lock.Unlock()

	e.logger.Info("Discovering projects for RenovateJob", "job", name)
	return e.discoverIntern(ctx, job)
}

func (e *discoveryAgent) discoverIntern(ctx context.Context, job *api.RenovateJob) ([]string, error) {
	// 1. Create the discovery job - replaces existing job
	existingDiscoveryJob, err := e.createOrReplaceDiscoveryJob(ctx, *job)
	if err != nil {
		return nil, fmt.Errorf("failed to create or get discovery job: %w", err)
	}

	// 2. Wait for discovery job completion
	for {
		if existingDiscoveryJob.Status.Succeeded > 0 {
			break
		}
		time.Sleep(5 * time.Second) // Wait before checking again
		err = e.client.Get(ctx, types.NamespacedName{
			Name:      existingDiscoveryJob.Name,
			Namespace: existingDiscoveryJob.Namespace,
		}, existingDiscoveryJob)
		if err != nil {
			return nil, fmt.Errorf("failed to get discovery job status: %w", err)
		}
		if existingDiscoveryJob.Status.Failed > 0 {
			return nil, fmt.Errorf("discovery job %s failed", existingDiscoveryJob.Name)
		}
	}

	// 3. Extract discovered projects from stdout
	projects, err := e.getDiscoveredProjectsFromJobLogs(ctx, e.client, existingDiscoveryJob)
	if err != nil {
		return nil, fmt.Errorf("failed to get discovered projects from job logs: %w", err)
	}
	e.logger.Info("Discovered projects", "count", len(projects))

	return projects, nil
}

func (e *discoveryAgent) createOrReplaceDiscoveryJob(ctx context.Context, renovateJob api.RenovateJob) (*batchv1.Job, error) {
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

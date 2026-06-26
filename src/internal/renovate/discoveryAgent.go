package renovate

import (
	context "context"
	"fmt"
	api "renovate-operator/api/v1alpha1"
	"renovate-operator/config"
	crdManager "renovate-operator/internal/crdManager"
	"renovate-operator/internal/podLogs"
	"renovate-operator/internal/types"
	"renovate-operator/metricStore"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

/*
DiscoveryAgent is the interface for discovering projects for a RenovateJob CRD.
*/
type DiscoveryAgent interface {
	// CreateDiscoveryJob creates and starts a discovery job for the given RenovateJob.
	// scheduleAfterCompletion controls whether ProcessDiscoveryJobResult will schedule all
	// non-running projects once the job completes (true for cron, false for UI-triggered).
	// Completion is handled reactively by the job controller via ProcessDiscoveryJobResult.
	CreateDiscoveryJob(ctx context.Context, renovateJob api.RenovateJob, options DiscoveryJobOptions) (string, error)
	// GetDiscoveryJobStatus retrieves the current status of the discovery job for the given RenovateJob CRD.
	GetDiscoveryJobStatus(ctx context.Context, job *api.RenovateJob) (api.RenovateProjectStatus, error)
	// ProcessDiscoveryJobResult handles completion of a discovery k8s Job: extracts discovered projects,
	// reconciles them into the RenovateJob CRD, schedules all projects, and optionally deletes the job.
	// A nil k8sJob or a still-running job is a no-op.
	ProcessDiscoveryJobResult(ctx context.Context, k8sJob *batchv1.Job, jobId crdManager.RenovateJobIdentifier) error
}

type DiscoveryJobOptions struct {
	// Wether to trigger all projects once the discovery is fnished
	TriggerAllProjects bool
}

type discoveryAgent struct {
	client    client.Client
	logger    logr.Logger
	scheme    *runtime.Scheme
	manager   crdManager.RenovateJobManager
	syncer    map[string]*sync.RWMutex
	logReader podLogs.PodLogReader
}

func NewDiscoveryAgent(scheme *runtime.Scheme, client client.Client, logger logr.Logger, manager crdManager.RenovateJobManager, lr podLogs.PodLogReader) DiscoveryAgent {
	return &discoveryAgent{
		client:    client,
		logger:    logger,
		scheme:    scheme,
		manager:   manager,
		syncer:    make(map[string]*sync.RWMutex),
		logReader: lr,
	}
}

// GetDiscoveryJobStatus implements DiscoveryAgent.
func (e *discoveryAgent) GetDiscoveryJobStatus(ctx context.Context, job *api.RenovateJob) (api.RenovateProjectStatus, error) {
	name := job.Fullname()
	lock := e.syncer[name]
	if lock == nil {
		lock = &sync.RWMutex{}
		e.syncer[name] = lock
	}
	lock.RLock()
	defer lock.RUnlock()

	existingDiscoveryJob, err := crdManager.GetJobByLabel(ctx, e.client, crdManager.JobSelector{
		JobType:         crdManager.DiscoveryJobType,
		Namespace:       job.Namespace,
		RenovateJobName: job.Name,
	})
	if err != nil && errors.IsNotFound(err) {
		time.Sleep(1 * time.Second)

		tries := 5
		for errors.IsNotFound(err) {
			tries--
			if tries <= 0 {
				return api.JobStatusFailed, fmt.Errorf("discovery job not found: %w", err)
			}
			existingDiscoveryJob, err = crdManager.GetJobByLabel(ctx, e.client, crdManager.JobSelector{
				JobType:         crdManager.DiscoveryJobType,
				Namespace:       job.Namespace,
				RenovateJobName: job.Name,
			})
		}
	} else if err != nil {
		return api.JobStatusFailed, fmt.Errorf("failed to get discovery job: %w", err)
	}

	if existingDiscoveryJob.Status.Failed > 0 {
		return api.JobStatusFailed, nil
	}
	if existingDiscoveryJob.Status.Succeeded > 0 {
		return api.JobStatusCompleted, nil
	}
	return api.JobStatusRunning, nil
}

// ProcessDiscoveryJobResult handles completion of a discovery k8s Job: extracts discovered
// projects from its logs, reconciles them into the RenovateJob CRD, and optionally schedules
// all non-running projects (controlled by the JOB_ANNOTATION_SCHEDULE_AFTER_DISCOVERY annotation).
// A nil k8sJob or a still-running job is a no-op.
func (e *discoveryAgent) ProcessDiscoveryJobResult(ctx context.Context, k8sJob *batchv1.Job, jobId crdManager.RenovateJobIdentifier) error {
	if k8sJob == nil {
		return nil
	}

	status, _, _, err := getJobStatus(k8sJob)
	if err != nil {
		return err
	}
	if status == api.JobStatusRunning {
		return nil
	}

	if status == api.JobStatusFailed {
		log.FromContext(ctx).Info("discovery job failed", "renovateJob", jobId.Name)
		metricStore.IncDiscoveryJob(ctx, jobId.Namespace, jobId.Name, "failed")
		_ = crdManager.MarkJobProcessed(ctx, e.client, k8sJob)
		return nil
	}

	renovateJob, err := e.manager.GetRenovateJob(ctx, jobId.Name, jobId.Namespace)
	if err != nil {
		return fmt.Errorf("failed to load RenovateJob: %w", err)
	}

	rawLogs, err := e.logReader.GetSucceededJobLog(ctx, k8sJob)
	if err != nil {
		// Pod may already be gone (operator restart replaying old jobs, or TTL cleanup).
		// Skip reconciliation — the next scheduled discovery will correct any drift.
		log.FromContext(ctx).V(1).Info("skipping discovery result: could not read job logs", "renovateJob", jobId.Name, "error", err)
		return nil
	}
	projects, err := parseAndSortDiscoveredProjects(rawLogs)
	if err != nil {
		log.FromContext(ctx).V(1).Info("skipping discovery result: could not parse job logs", "renovateJob", jobId.Name, "error", err)
		return nil
	}
	log.FromContext(ctx).V(2).Info("Discovered projects", "count", len(projects), "job", renovateJob.Fullname())

	metricStore.IncDiscoveryJob(ctx, jobId.Namespace, jobId.Name, "completed")
	metricStore.SetDiscoveredRepositories(jobId.Namespace, jobId.Name, len(projects))

	if err := e.manager.ReconcileProjects(ctx, renovateJob, projects); err != nil {
		return fmt.Errorf("failed to reconcile projects: %w", err)
	}

	if k8sJob.Annotations[crdManager.JOB_ANNOTATION_SCHEDULE_AFTER_DISCOVERY] == "true" {
		isNotRunning := func(p api.ProjectStatus) bool {
			return p.Status != api.JobStatusRunning
		}
		if err := e.manager.UpdateProjectStatusBatched(ctx, isNotRunning, jobId, &types.RenovateStatusUpdate{
			Status: api.JobStatusScheduled,
		}); err != nil {
			return fmt.Errorf("failed to schedule projects: %w", err)
		}
	}

	// Stamp before the optional delete: if deletion fails the annotation prevents re-processing.
	if err := crdManager.MarkJobProcessed(ctx, e.client, k8sJob); err != nil {
		log.FromContext(ctx).Error(err, "failed to mark discovery job as processed", "job", k8sJob.Name)
	}

	if config.GetValue("DELETE_SUCCESSFUL_JOBS") == "true" {
		if err := crdManager.DeleteJob(ctx, e.client, k8sJob); err != nil {
			log.FromContext(ctx).Error(err, "failed to delete successful discovery job", "job", renovateJob.Fullname())
		}
	}

	return nil
}

func (e *discoveryAgent) CreateDiscoveryJob(ctx context.Context, renovateJob api.RenovateJob, options DiscoveryJobOptions) (string, error) {
	name := renovateJob.Fullname()
	lock := e.syncer[name]
	if lock == nil {
		lock = &sync.RWMutex{}
		e.syncer[name] = lock
	}
	lock.Lock()
	defer lock.Unlock()

	existingJob, err := crdManager.GetJobByLabel(ctx, e.client, crdManager.JobSelector{
		JobType:         crdManager.DiscoveryJobType,
		Namespace:       renovateJob.Namespace,
		RenovateJobName: renovateJob.Name,
	})
	if err != nil && !errors.IsNotFound(err) {
		return "", fmt.Errorf("failed to check for existing discovery job: %w", err)
	}
	if existingJob != nil && existingJob.Status.Succeeded == 0 && existingJob.Status.Failed == 0 {
		log.FromContext(ctx).V(1).Info("discovery job already running, skipping", "renovateJob", renovateJob.Fullname())
		if options.TriggerAllProjects && existingJob.Annotations[crdManager.JOB_ANNOTATION_SCHEDULE_AFTER_DISCOVERY] != "true" {
			patch := client.MergeFrom(existingJob.DeepCopy())
			if existingJob.Annotations == nil {
				existingJob.Annotations = make(map[string]string)
			}
			existingJob.Annotations[crdManager.JOB_ANNOTATION_SCHEDULE_AFTER_DISCOVERY] = "true"
			if err := e.client.Patch(ctx, existingJob, patch); err != nil {
				return "", fmt.Errorf("failed to set schedule-after-discovery annotation on running job: %w", err)
			}
		}
		return "", nil
	}

	if err := ensureRedisURLSecret(ctx, e.client, renovateJob.Namespace); err != nil {
		return "", fmt.Errorf("failed to ensure redis url secret: %w", err)
	}

	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	discoveryJob := newDiscoveryJob(&renovateJob, carrier.Get("traceparent"))
	if options.TriggerAllProjects {
		if discoveryJob.Annotations == nil {
			discoveryJob.Annotations = make(map[string]string)
		}
		discoveryJob.Annotations[crdManager.JOB_ANNOTATION_SCHEDULE_AFTER_DISCOVERY] = "true"
	}
	if err := controllerutil.SetControllerReference(&renovateJob, discoveryJob, e.scheme); err != nil {
		return "", fmt.Errorf("failed to set controller reference: %w", err)
	}

	generation, err := crdManager.CreateJobWithGeneration(ctx, e.client, discoveryJob, crdManager.JobSelector{
		JobType:         crdManager.DiscoveryJobType,
		Namespace:       renovateJob.Namespace,
		RenovateJobName: renovateJob.Name,
	})
	if err != nil {
		return "", fmt.Errorf("failed to create discovery job: %w", err)
	}

	metricStore.IncJobDispatched(ctx, renovateJob.Namespace, renovateJob.Name, "discovery")

	return generation, nil
}

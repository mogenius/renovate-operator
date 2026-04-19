package renovate

import (
	context "context"
	"fmt"
	api "renovate-operator/api/v1alpha1"
	crdManager "renovate-operator/internal/crdManager"
	"renovate-operator/internal/telemetry"
	"renovate-operator/internal/utils"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
	"go.opentelemetry.io/otel/trace"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var discoveryTracer = otel.Tracer("renovate-operator/discovery")

/*
DiscoveryAgent is the interface for discovering projects for a RenovateJob CRD.
*/
type DiscoveryAgent interface {
	// Discover runs the discovery process for the given RenovateJob CRD and returns the list of discovered projects.
	Discover(ctx context.Context, job *api.RenovateJob) ([]string, error)
	// Only create and start the discovery job, do not wait for completion.
	CreateDiscoveryJob(ctx context.Context, renovateJob api.RenovateJob) (string, error)
	// GetDiscoveryJobStatus retrieves the current status of the discovery job for the given RenovateJob CRD.
	GetDiscoveryJobStatus(ctx context.Context, job *api.RenovateJob, generation string) (api.RenovateProjectStatus, error)
	// WaitForDiscoveryJob waits for the discovery job to complete and returns the list of discovered projects.
	WaitForDiscoveryJob(ctx context.Context, job *api.RenovateJob, generation string) ([]string, error)
}

type discoveryAgent struct {
	client client.Client
	logger logr.Logger
	scheme *runtime.Scheme
	syncer map[string]*sync.RWMutex
	// allow tests to override how logs are extracted
	getDiscoveredProjectsFromJobLogsFn func(ctx context.Context, c client.Client, job *batchv1.Job) ([]string, error)
	// allow tests to override how status is checked
	getDiscoveryJobStatusFn func(ctx context.Context, job *api.RenovateJob, generation string) (api.RenovateProjectStatus, error)
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

	ctx, span := discoveryTracer.Start(ctx, "discovery.run",
		trace.WithAttributes(
			semconv.K8SNamespaceName(job.Namespace),
			semconv.CICDPipelineName(job.Name),
		),
	)
	defer span.End()
	ctx = telemetry.ContextWithTraceLogger(ctx, e.logger)

	log.FromContext(ctx).V(2).Info("Discovering projects for RenovateJob", "job", name)
	projects, err := e.discoverIntern(ctx, job)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	span.AddEvent("discovery.complete", trace.WithAttributes(
		attribute.Int("project.count", len(projects)),
	))
	return projects, nil
}

func (e *discoveryAgent) discoverIntern(ctx context.Context, job *api.RenovateJob) ([]string, error) {
	// 1. Create the discovery job - replaces existing job
	generation, err := e.CreateDiscoveryJob(ctx, *job)
	if err != nil {
		return nil, fmt.Errorf("failed to create or get discovery job: %w", err)
	}

	return e.WaitForDiscoveryJob(ctx, job, generation)
}

func (e *discoveryAgent) WaitForDiscoveryJob(ctx context.Context, job *api.RenovateJob, generation string) ([]string, error) {
	// 2. Wait for discovery job completion
	for {
		status, err := e.getDiscoveryJobStatusFn(ctx, job, generation)

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
	existingDiscoveryJob, err := crdManager.GetJobByLabel(ctx, e.client, crdManager.JobSelector{
		JobName:   utils.DiscoveryJobName(job),
		JobType:   crdManager.DiscoveryJobType,
		Namespace: job.Namespace,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get discovery job: %w", err)
	}
	projects, err := e.getDiscoveredProjectsFromJobLogsFn(ctx, e.client, existingDiscoveryJob)
	if err != nil {
		return nil, fmt.Errorf("failed to get discovered projects from job logs: %w", err)
	}
	e.logger.V(2).Info("Discovered projects", "count", len(projects), "job", job.Fullname())

	return projects, nil
}

// GetDiscoveryJobStatus implements DiscoveryAgent.
func (e *discoveryAgent) GetDiscoveryJobStatus(ctx context.Context, job *api.RenovateJob, generation string) (api.RenovateProjectStatus, error) {
	return e.getDiscoveryJobStatusFn(ctx, job, generation)
}

// getDiscoveryJobStatusInternal is the internal implementation of GetDiscoveryJobStatus.
func (e *discoveryAgent) getDiscoveryJobStatusInternal(ctx context.Context, job *api.RenovateJob, generation string) (api.RenovateProjectStatus, error) {
	// lock based on the renovatejob
	name := job.Fullname()
	lock := e.syncer[name]
	if lock == nil {
		lock = &sync.RWMutex{}
		e.syncer[name] = lock
	}
	lock.RLock()
	defer lock.RUnlock()

	// if generation is not provided, get the latest job and return its status
	existingDiscoveryJob, err := crdManager.GetJobByLabel(ctx, e.client, crdManager.JobSelector{
		JobName:    utils.DiscoveryJobName(job),
		JobType:    crdManager.DiscoveryJobType,
		Namespace:  job.Namespace,
		Generation: &generation,
	})
	// retry getting the job if not found
	if err != nil && errors.IsNotFound(err) {
		time.Sleep(1 * time.Second)

		tries := 5
		for errors.IsNotFound(err) {
			tries--
			if tries <= 0 {
				return api.JobStatusFailed, fmt.Errorf("discovery job not found: %w", err)
			}
			existingDiscoveryJob, err = crdManager.GetJobByLabel(ctx, e.client, crdManager.JobSelector{
				JobName:    utils.DiscoveryJobName(job),
				JobType:    crdManager.DiscoveryJobType,
				Namespace:  job.Namespace,
				Generation: &generation,
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
func (e *discoveryAgent) CreateDiscoveryJob(ctx context.Context, renovateJob api.RenovateJob) (string, error) {
	// lock based on the renovatejob
	name := renovateJob.Fullname()
	lock := e.syncer[name]
	if lock == nil {
		lock = &sync.RWMutex{}
		e.syncer[name] = lock
	}
	lock.Lock()
	defer lock.Unlock()

	if err := ensureRedisURLSecret(ctx, e.client, renovateJob.Namespace); err != nil {
		return "", fmt.Errorf("failed to ensure redis url secret: %w", err)
	}

	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	discoveryJob := newDiscoveryJob(&renovateJob, carrier.Get("traceparent"))
	if err := controllerutil.SetControllerReference(&renovateJob, discoveryJob, e.scheme); err != nil {
		return "", fmt.Errorf("failed to set controller reference: %w", err)
	}

	// Create the discovery job
	generation, err := crdManager.CreateJobWithGeneration(ctx, e.client, discoveryJob, crdManager.JobSelector{
		JobName:   utils.DiscoveryJobName(&renovateJob),
		JobType:   crdManager.DiscoveryJobType,
		Namespace: renovateJob.Namespace,
	})
	if err != nil {
		return "", fmt.Errorf("failed to create discovery job: %w", err)
	}
	return generation, nil
}

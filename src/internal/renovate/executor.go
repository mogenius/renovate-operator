package renovate

import (
	context "context"
	"fmt"
	api "renovate-operator/api/v1alpha1"
	"renovate-operator/config"
	"renovate-operator/health"
	"sync"
	"time"

	crdManager "renovate-operator/internal/crdManager"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type RenovateExecutor interface {
	Start(ctx context.Context) error
}

type RenovateJobInfo struct {
	Name      string
	Namespace string
	Projects  []api.ProjectStatus
}
type renovateExecutor struct {
	syncer        map[string]*sync.Mutex
	scheme        *runtime.Scheme
	updateJobSync map[string]*sync.Mutex
	client        client.Client
	logger        logr.Logger
	health        health.HealthCheck
	manager       crdManager.RenovateJobManager
}

func NewRenovateExecutor(scheme *runtime.Scheme, manager crdManager.RenovateJobManager, client client.Client, logger logr.Logger, health health.HealthCheck) RenovateExecutor {
	return &renovateExecutor{
		syncer:        make(map[string]*sync.Mutex),
		updateJobSync: make(map[string]*sync.Mutex),
		client:        client,
		scheme:        scheme,
		manager:       manager,
		logger:        logger,
		health:        health,
	}
}

func (e *renovateExecutor) Start(ctx context.Context) error {
	go func() {
		// set healthcheck to running
		e.health.SetExecutorHealth(func(eHealth *health.ExecutorHealth) *health.ExecutorHealth {
			eHealth.Running = true
			return eHealth
		})

		for {
			select {
			case <-ctx.Done():
				e.logger.Info("executor loop stopped due to context cancellation")
				return
			default:
				err := e.execute()
				if err != nil {
					e.logger.Error(err, "an error occured in execution loop")
				}
				// Wait for 10 seconds or until context is done
				select {
				case <-ctx.Done():
					e.logger.Info("executor loop stopped during sleep due to context cancellation")
					return
				case <-time.After(10 * time.Second):
				}
			}
		}
	}()
	return nil
}

func (e *renovateExecutor) execute() error {
	ctx := context.Background()

	renovateJobs, err := e.manager.ListRenovateJobs(ctx)
	if err != nil {
		return nil
	}
	e.logger.Info("Executing renovate loop for projects", "count", len(renovateJobs))
	for _, job := range renovateJobs {
		err := e.executeRenovateJob(ctx, &job)
		if err != nil {
			e.logger.Error(err, "renovate loop execution failed for job")
		}
	}
	return nil
}

func (e *renovateExecutor) syncOnJobExecution(name string) (bool, func()) {
	lock := e.syncer[name]
	if lock == nil {
		lock = &sync.Mutex{}
		e.syncer[name] = lock
	}

	locker := lock.TryLock()
	e.health.SetExecutorHealth(func(eHealth *health.ExecutorHealth) *health.ExecutorHealth {
		eHealth.Executor[name] = health.SingleExecutorHealth{
			IsRunning: locker,
		}
		return eHealth
	})

	if !locker {
		return false, func() {}
	}

	return true, func() {
		e.health.SetExecutorHealth(func(eHealth *health.ExecutorHealth) *health.ExecutorHealth {
			eHealth.Executor[name] = health.SingleExecutorHealth{
				IsRunning: false,
			}
			return eHealth
		})
		lock.Unlock()
	}
}

func (e *renovateExecutor) executeRenovateJob(ctx context.Context, job *crdManager.RenovateJobIdentifier) error {
	name := job.Fullname()
	locked, unlock := e.syncOnJobExecution(name)
	if !locked {
		e.logger.Info("another renovate execution is still running - skipping")
		return nil
	}
	defer unlock()

	e.logger.Info("Executing RenovateJob", "job", name)

	renovateJob, err := e.manager.GetRenovateJob(ctx, job.Name, job.Namespace)
	if err != nil {
		return err
	}
	return e.reconcileProjects(ctx, renovateJob)
}

func (e *renovateExecutor) reconcileProjects(ctx context.Context, renovateJob *api.RenovateJob) error {
	// determine how many projects are currently running
	runningProjects := 0
	for i := range renovateJob.Status.Projects {
		project := &renovateJob.Status.Projects[i]
		if project.Status == api.JobStatusRunning {
			runningProjects++
		}
	}

	for i := range renovateJob.Status.Projects {
		project := &renovateJob.Status.Projects[i]

		jobId := crdManager.RenovateJobIdentifier{
			Name:      renovateJob.Name,
			Namespace: renovateJob.Namespace,
		}

		switch project.Status {
		//Job is completed -> do nothing
		case api.JobStatusCompleted:

		// Job is failed -> do nothing
		case api.JobStatusFailed:

		// Job is running -> verify thats true
		case api.JobStatusRunning:
			newStatus, err := getJobStatus(renovateJob.ExecutorJobName(project.Name), renovateJob.Namespace, e.client)
			if err != nil {
				return err
			}
			if newStatus != api.JobStatusRunning {
				err = e.manager.UpdateProjectStatus(ctx, project.Name, jobId, newStatus)
				// one project less is currently running
				runningProjects--
				if err != nil {
					return err
				}

				// if DELETE_SUCCESSFULL_JOBS is set -> delete the job
				deleteSuccessfullJobs := config.GetValue("DELETE_SUCCESSFULL_JOBS")
				if newStatus == api.JobStatusCompleted && deleteSuccessfullJobs == "true" {
					err = deleteJob(renovateJob.ExecutorJobName(project.Name), renovateJob.Namespace, e.client)
					if err != nil {
						return err
					}
				}
			}

		// Job is scheduled -> execute (if possible)
		case api.JobStatusScheduled:
			// are there already enough projects running?
			if runningProjects < int(renovateJob.Spec.Parallelism) {
				// Create a new job for this project
				job := newRenovateJob(renovateJob, project.Name)
				if err := controllerutil.SetControllerReference(renovateJob, job, e.scheme); err != nil {
					return fmt.Errorf("failed to set controller reference: %w", err)
				}

				// Check if the job already exists
				existingJob, err := crdManager.GetJob(ctx, e.client, job.Name, job.Namespace)
				// delete job if it exists
				if err == nil {
					err = crdManager.DeleteJob(ctx, e.client, existingJob)
					if err != nil {
						return fmt.Errorf("failed to delete existing RenovateJob for project %s: %w", project.Name, err)
					}
				}
				// recreate the job
				if err := crdManager.CreateJob(ctx, e.client, job); err != nil {
					return fmt.Errorf("failed to create RenovateJob for project %s: %w", project.Name, err)
				}
				runningProjects++

				jobId := crdManager.RenovateJobIdentifier{Name: renovateJob.Name, Namespace: renovateJob.Namespace}
				err = e.manager.UpdateProjectStatus(ctx, project.Name, jobId, api.JobStatusRunning)
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

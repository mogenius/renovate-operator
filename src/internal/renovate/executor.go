package renovate

import (
	context "context"
	"fmt"
	"sort"
	"strconv"
	"time"

	api "renovate-operator/api/v1alpha1"
	"renovate-operator/clientProvider"
	"renovate-operator/config"
	"renovate-operator/health"
	"renovate-operator/metricStore"

	crdManager "renovate-operator/internal/crdManager"
	"renovate-operator/internal/logStore"
	"renovate-operator/internal/parser"
	"renovate-operator/internal/types"
	"renovate-operator/internal/utils"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

/*
RenovateExecutor is the interface that periodically executes RenovateJob CRDs.
It checks the status of each project and starts new jobs as needed based on the specified parameters.
*/
type RenovateExecutor interface {
	// Start begins the periodic execution of RenovateJob CRDs.
	Start(ctx context.Context) error
}

type renovateExecutor struct {
	scheme   *runtime.Scheme
	client   client.Client
	logger   logr.Logger
	health   health.HealthCheck
	manager  crdManager.RenovateJobManager
	logStore logStore.LogStore
}

type executionOptions struct {
	globalParallelism int
}

func NewRenovateExecutor(scheme *runtime.Scheme, manager crdManager.RenovateJobManager, client client.Client, logger logr.Logger, health health.HealthCheck, ls logStore.LogStore) RenovateExecutor {
	return &renovateExecutor{
		client:   client,
		scheme:   scheme,
		manager:  manager,
		logger:   logger,
		health:   health,
		logStore: ls,
	}
}

func (e *renovateExecutor) Start(ctx context.Context) error {
	globalParallelism, err := strconv.Atoi(config.GetValue("GLOBAL_PARALLELISM_LIMIT"))
	if err != nil {
		return fmt.Errorf("failed to parse GLOBAL_PARALLELISM_LIMIT: %w", err)
	}
	options := executionOptions{
		globalParallelism: globalParallelism,
	}

	go func() {
		e.health.SetExecutorHealth(func(eHealth *health.ExecutorHealth) *health.ExecutorHealth {
			eHealth.Running = true
			return eHealth
		})
		e.logger.Info("starting renovate executor loop")
		for {
			select {
			case <-ctx.Done():
				e.logger.Info("executor loop stopped due to context cancellation")
				return
			default:
				err := e.execute(options)
				if err != nil {
					e.logger.Error(err, "an error occured in execution loop")
				}
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

func (e *renovateExecutor) execute(options executionOptions) error {
	ctx := context.Background()

	renovateJobs, err := e.manager.ListRenovateJobsFull(ctx)
	if err != nil {
		return nil
	}
	e.logger.V(2).Info("Executing renovate loop for projects", "count", len(renovateJobs))

	// Pass 1: check all currently running projects across all jobs, update their statuses,
	// and count how many are still running globally and per job.
	globalRunning, perJobRunning, err := e.reconcileRunning(ctx, renovateJobs)
	if err != nil {
		return err
	}

	// Pass 2: collect all scheduled projects across all jobs, sort for fairness,
	// and dispatch new jobs up to the global and per-job limits.
	return e.dispatchScheduled(ctx, renovateJobs, globalRunning, perJobRunning, options)
}

// reconcileRunning iterates over all Running projects across all RenovateJobs, checks their
// Kubernetes Job status, updates finished ones, and returns the number of still-running projects
// globally and per job (keyed by job fullname).
func (e *renovateExecutor) reconcileRunning(ctx context.Context, renovateJobs []api.RenovateJob) (int, map[string]int, error) {
	globalRunning := 0
	perJobRunning := make(map[string]int, len(renovateJobs))

	for i := range renovateJobs {
		renovateJob := &renovateJobs[i]
		jobId := crdManager.RenovateJobIdentifier{Name: renovateJob.Name, Namespace: renovateJob.Namespace}
		key := jobId.Fullname()
		perJobRunning[key] = 0

		for j := range renovateJob.Status.Projects {
			project := &renovateJob.Status.Projects[j]
			if project.Status != api.JobStatusRunning {
				continue
			}

			k8sJob, err := crdManager.GetJobByLabel(ctx, e.client, crdManager.JobSelector{
				JobName:   utils.ExecutorJobName(renovateJob, project.Name),
				JobType:   crdManager.ExecutorJobType,
				Namespace: renovateJob.Namespace,
			})

			var newStatus api.RenovateProjectStatus
			var durationStr string
			if err != nil {
				if errors.IsNotFound(err) {
					newStatus = api.JobStatusFailed
				} else {
					return 0, nil, err
				}
			} else {
				newStatus, durationStr, err = getJobStatus(k8sJob)
				if err != nil {
					return 0, nil, err
				}
			}

			if newStatus == api.JobStatusRunning {
				globalRunning++
				perJobRunning[key]++
				continue
			}

			// Job finished — collect metrics and update CRD status.
			newProjectStatus := &types.RenovateStatusUpdate{
				Status:   newStatus,
				Duration: &durationStr,
			}
			hasIssues := false
			if k8sJob != nil {
				cp := clientProvider.StaticClientProvider()
				if clientset, err := cp.K8sClientSet(); err == nil {
					if logs, err := crdManager.GetLastJobLog(ctx, clientset, k8sJob); err == nil {
						e.logStore.Save(renovateJob.Namespace, renovateJob.Name, project.Name, logs)
						parseResult := parser.ParseRenovateLogs(logs)
						hasIssues = parseResult.HasIssues
						newProjectStatus.RenovateResultStatus = parseResult.RenovateResultStatus
						newProjectStatus.PRActivity = parseResult.PRActivity
					} else {
						e.logger.Error(err, "failed to get logs for metrics parsing", "project", project.Name)
					}
				} else {
					e.logger.Error(err, "failed to create Kubernetes clientset for metrics parsing", "project", project.Name)
				}
			}

			metricStore.SetRunFailed(renovateJob.Namespace, renovateJob.Name, project.Name, newStatus == api.JobStatusFailed)
			metricStore.SetDependencyIssues(renovateJob.Namespace, renovateJob.Name, project.Name, hasIssues)
			metricStore.CaptureRenovateProjectExecution(renovateJob.Namespace, renovateJob.Name, project.Name, string(newStatus))

			if err := e.manager.UpdateProjectStatus(ctx, project.Name, jobId, newProjectStatus); err != nil {
				return 0, nil, err
			}

			if newStatus == api.JobStatusCompleted && config.GetValue("DELETE_SUCCESSFUL_JOBS") == "true" && k8sJob != nil {
				if err := crdManager.DeleteJob(ctx, e.client, k8sJob); err != nil {
					return 0, nil, err
				}
			}
		}
	}

	return globalRunning, perJobRunning, nil
}

// scheduledCandidate is a project ready to be dispatched together with its parent RenovateJob.
type scheduledCandidate struct {
	project     api.ProjectStatus
	renovateJob *api.RenovateJob
	// jobOldestWait is the smallest LastRun time among all Scheduled projects in the same
	// RenovateJob. Used as the fairness key: jobs whose projects have been waiting the longest
	// are dispatched first to prevent starvation.
	jobOldestWait time.Time
}

// dispatchScheduled collects all Scheduled projects across all RenovateJobs, sorts them for
// fairness, and launches Kubernetes Jobs until the global or per-job parallelism limits are reached.
func (e *renovateExecutor) dispatchScheduled(ctx context.Context, renovateJobs []api.RenovateJob, globalRunning int, perJobRunning map[string]int, options executionOptions) error {
	var candidates []scheduledCandidate

	for i := range renovateJobs {
		renovateJob := &renovateJobs[i]

		// Find the oldest LastRun among this job's Scheduled projects to use as
		// the fairness sort key for all candidates from this RenovateJob.
		oldestWait := time.Now()
		for _, p := range renovateJob.Status.Projects {
			if p.Status == api.JobStatusScheduled && p.LastRun.Time.Before(oldestWait) {
				oldestWait = p.LastRun.Time
			}
		}

		for _, p := range renovateJob.Status.Projects {
			if p.Status != api.JobStatusScheduled {
				continue
			}
			candidates = append(candidates, scheduledCandidate{
				project:       p,
				renovateJob:   renovateJob,
				jobOldestWait: oldestWait,
			})
		}
	}

	// Primary sort: higher priority projects go first.
	// Secondary sort: among equal-priority candidates, the RenovateJob that has been waiting longest goes first.
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].project.Priority != candidates[j].project.Priority {
			return candidates[i].project.Priority > candidates[j].project.Priority
		}
		return candidates[i].jobOldestWait.Before(candidates[j].jobOldestWait)
	})

	for _, candidate := range candidates {
		renovateJob := candidate.renovateJob
		project := candidate.project
		jobId := crdManager.RenovateJobIdentifier{Name: renovateJob.Name, Namespace: renovateJob.Namespace}
		key := jobId.Fullname()

		// Stop entirely if the global limit is reached.
		if options.globalParallelism > 0 && globalRunning >= options.globalParallelism {
			e.logger.V(2).Info("global parallelism limit reached, stopping dispatch", "limit", options.globalParallelism)
			break
		}

		// Skip this candidate if its job has reached its per-job limit.
		if perJobRunning[key] >= int(renovateJob.Spec.Parallelism) {
			continue
		}

		k8sJob := newRenovateJob(renovateJob, project.Name)
		if err := controllerutil.SetControllerReference(renovateJob, k8sJob, e.scheme); err != nil {
			return fmt.Errorf("failed to set controller reference: %w", err)
		}

		_, err := crdManager.CreateJobWithGeneration(ctx, e.client, k8sJob, crdManager.JobSelector{
			JobName:   utils.ExecutorJobName(renovateJob, project.Name),
			JobType:   crdManager.ExecutorJobType,
			Namespace: renovateJob.Namespace,
		})
		if err != nil {
			return fmt.Errorf("failed to create RenovateJob for project %s: %w", project.Name, err)
		}

		if err := e.manager.UpdateProjectStatus(ctx, project.Name, jobId, &types.RenovateStatusUpdate{
			Status: api.JobStatusRunning,
		}); err != nil {
			return err
		}

		globalRunning++
		perJobRunning[key]++
	}

	return nil
}

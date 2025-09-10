package controllers

import (
	context "context"
	api "renovate-operator/api/v1alpha1"
	"renovate-operator/internal/renovate"
	"renovate-operator/scheduler"
	"time"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	crdManager "renovate-operator/internal/crdManager"
)

type RenovateJobReconciler struct {
	Discovery renovate.DiscoveryAgent
	Manager   crdManager.RenovateJobManager
	Scheduler scheduler.Scheduler
}

func (r *RenovateJobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithName("renovatejob-controller")
	renovateJob, err := r.Manager.GetRenovateJob(ctx, req.NamespacedName.Name, req.NamespacedName.Namespace)

	if err == nil {
		// renovatejob object read without problem -> create the schedule
		createScheduler(logger, renovateJob, r)
		return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
	} else if errors.IsNotFound(err) {
		// renovatejob cannot be found -> delete the schedule
		name := req.NamespacedName.Name + "-" + req.NamespacedName.Namespace
		r.Scheduler.RemoveSchedule(name)
		return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
	} else {
		logger.Error(err, "Failed to get RenovateJob")
		return ctrl.Result{RequeueAfter: 1 * time.Minute}, err
	}
}

func createScheduler(logger logr.Logger, renovateJob *api.RenovateJob, reconciler *RenovateJobReconciler) {
	name := renovateJob.Fullname()
	expr := renovateJob.Spec.Schedule
	f := func() {
		logger = logger.WithName(name)
		ctx := context.Background()
		logger.Info("Executing schedule for RenovateJob")
		projects, err := reconciler.Discovery.Discover(ctx, renovateJob)
		if err != nil {
			logger.Error(err, "Failed to discover projects for RenovateJob")
			return
		}
		logger.Info("Successfully discovered projects", "count", len(projects))

		jobIdentifier := crdManager.RenovateJobIdentifier{
			Name:      renovateJob.Name,
			Namespace: renovateJob.Namespace,
		}
		err = reconciler.Manager.ReconcileProjects(ctx, jobIdentifier, projects)
		if err != nil {
			logger.Error(err, "failed to reconcile projects")
			return
		}
		logger.Info("Successfully reconciled Projects")

		a, _ := reconciler.Manager.GetRenovateJob(ctx, renovateJob.Name, renovateJob.Namespace)

		logger.Info("DEBUG", "count", len(a.Status.Projects))

		isNotRunning := func(p api.ProjectStatus) bool {
			return p.Status != api.JobStatusRunning
		}
		err = reconciler.Manager.UpdateProjectStatusBatched(ctx, isNotRunning, jobIdentifier, api.JobStatusScheduled)

		if err != nil {
			logger.Error(err, "failed to schedule projects")
		}
		logger.Info("Successfully scheduled RenovateJob")
	}

	// adding the schedule if it does not exist
	// if the expression is different it will be updated
	err := reconciler.Scheduler.AddScheduleReplaceExisting(expr, name, f)
	if err != nil {
		logger.Error(err, "Failed to add schedule for RenovateJob")
		return
	}
	logger.Info("Added schedule for RenovateJob", "schedule", expr)
}

func (r *RenovateJobReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&api.RenovateJob{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}

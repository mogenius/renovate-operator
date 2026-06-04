package controllers

import (
	context "context"
	"renovate-operator/internal/renovate"

	batchv1 "k8s.io/api/batch/v1"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	crdManager "renovate-operator/internal/crdManager"
)

/*
Reconciler for RenovateJob resources
Watching for create/update/delete events and managing the schedules accordingly
*/
type JobReconciler struct {
	Executor  renovate.RenovateExecutor
	K8sClient client.Client
}

func (r *JobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	job := &batchv1.Job{}
	err := r.K8sClient.Get(ctx, req.NamespacedName, job)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if job.Labels == nil {
		return ctrl.Result{}, nil
	}
	jobType := job.Labels[crdManager.JOB_LABEL_TYPE]
	renovateJobName := job.Labels[crdManager.JOB_LABEL_RENOVATEJOB]

	// only handle jobs that are managed by us (identified by the presence of our labels)
	if renovateJobName == "" || jobType == "" {
		return ctrl.Result{}, nil
	}

	switch jobType {
	case string(crdManager.DiscoveryJobType):
	case string(crdManager.ExecutorJobType):
		project := job.Annotations[crdManager.JOB_ANNOTATION_PROJECT]
		jobId := crdManager.RenovateJobIdentifier{
			Namespace: job.Namespace,
			Name:      renovateJobName,
		}
		err := r.Executor.ProcessProjectJobResult(ctx, job, project, jobId)
		if err != nil {
			logger.Error(err, "Error processing job result", "jobName", job.Name, "project", project)
			return ctrl.Result{}, err
		}
	default:
		logger.Info("Ignoring job with unrecognized type", "jobName", job.Name, "jobType", jobType)
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}

func (r *JobReconciler) SetupWithJobManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&batchv1.Job{}).
		Complete(r)
}

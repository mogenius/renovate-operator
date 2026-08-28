package controllers

import (
	context "context"

	api "renovate-operator/api/v1alpha1"
	crdManager "renovate-operator/internal/crdManager"
	"renovate-operator/internal/types"

	"k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// RenovateProjectReconciler watches RenovateProject resources and reacts to
// one-shot trigger annotations set by users.
type RenovateProjectReconciler struct {
	Manager   crdManager.RenovateJobManager
	K8sClient client.Client
}

func (r *RenovateProjectReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var project api.RenovateProject
	if err := r.K8sClient.Get(ctx, req.NamespacedName, &project); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if project.Annotations[api.TriggerScheduleAnnotationKey] != "true" {
		return ctrl.Result{}, nil
	}

	jobName := project.Labels[api.LabelRenovateJob]
	if jobName == "" {
		logger.Error(nil, "RenovateProject missing renovatejob label, cannot schedule", "project", project.Spec.Project)
		return ctrl.Result{}, nil
	}

	jobId := crdManager.RenovateJobIdentifier{Name: jobName, Namespace: project.Namespace}
	if err := r.Manager.UpdateProjectStatus(ctx, project.Spec.Project, jobId, &types.RenovateStatusUpdate{Status: api.JobStatusScheduled}); err != nil {
		if err == crdManager.ErrProjectNotFound {
			if remErr := crdManager.RemoveAnnotation(ctx, r.K8sClient, &project, api.TriggerScheduleAnnotationKey); remErr != nil {
				logger.Error(remErr, "failed to remove schedule annotation from RenovateProject")
			}
			return ctrl.Result{}, nil
		}
		logger.Error(err, "failed to schedule project via annotation, will retry", "project", project.Spec.Project)
		return ctrl.Result{}, err
	}

	logger.V(1).Info("project scheduled via annotation", "project", project.Spec.Project)
	if err := crdManager.RemoveAnnotation(ctx, r.K8sClient, &project, api.TriggerScheduleAnnotationKey); err != nil {
		logger.Error(err, "failed to remove schedule annotation from RenovateProject")
	}
	return ctrl.Result{}, nil
}

func (r *RenovateProjectReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&api.RenovateProject{}).
		Complete(r)
}

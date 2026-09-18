package crdmanager

import (
	"context"
	"fmt"
	api "renovate-operator/api/v1alpha1"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// reload a given renovatejob
func reloadRenovateJob(ctx context.Context, renovateJob *api.RenovateJob, client client.Client) (*api.RenovateJob, error) {
	return loadRenovateJob(ctx, renovateJob.Name, renovateJob.Namespace, client)
}

// load a renovatejob by its name and namespace
func loadRenovateJob(ctx context.Context, name string, namespace string, client client.Client) (*api.RenovateJob, error) {
	renovateJob := &api.RenovateJob{}
	err := client.Get(ctx, types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, renovateJob)
	if err != nil {
		return nil, err
	}

	return renovateJob, nil
}

// resolveEffectiveJob returns a copy of job with its template merged in. A dangling
// ref returns the underlying (IsNotFound) error, so the caller fails the job closed.
// Templates are read through the cache: they are watched, so edits propagate to
// dependents.
func resolveEffectiveJob(ctx context.Context, job *api.RenovateJob, c client.Client) (*api.RenovateJob, error) {
	ref := job.Spec.TemplateRef
	if ref == nil {
		return job.DeepCopy(), nil
	}

	var templateSpec api.RenovateJobSpec
	if ref.IsCluster() {
		var template api.ClusterRenovateJobTemplate
		if err := c.Get(ctx, client.ObjectKey{Name: ref.Name}, &template); err != nil {
			return nil, fmt.Errorf("resolving ClusterRenovateJobTemplate %q: %w", ref.Name, err)
		}
		templateSpec = template.Spec
	} else {
		var template api.RenovateJobTemplate
		if err := c.Get(ctx, client.ObjectKey{Name: ref.Name, Namespace: job.Namespace}, &template); err != nil {
			return nil, fmt.Errorf("resolving RenovateJobTemplate %q: %w", ref.Name, err)
		}
		templateSpec = template.Spec
	}

	effective := job.DeepCopy()
	effective.Spec = api.MergeTemplateSpec(templateSpec, job.Spec)
	return effective, nil
}

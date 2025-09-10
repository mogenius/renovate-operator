package crdmanager

import (
	"context"
	api "renovate-operator/api/v1alpha1"
	"renovate-operator/internal/utils"

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
	err := utils.Retry(utils.DefaultRetryAttempts, utils.DefaultRetrySleep, func() error {
		return client.Get(ctx, types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		}, renovateJob)
	})
	if err != nil {
		return nil, err
	}

	return renovateJob, nil
}

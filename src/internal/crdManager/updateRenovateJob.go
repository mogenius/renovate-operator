package crdmanager

import (
	"context"
	api "renovate-operator/api/v1alpha1"
	"renovate-operator/internal/utils"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// update the provided renovate job and return a reloaded version
func updateRenovateJobStatus(ctx context.Context, renovateJob *api.RenovateJob, client client.Client) (*api.RenovateJob, error) {

	err := utils.Retry(utils.DefaultRetryAttempts, utils.DefaultRetrySleep, func() error {
		return client.Status().Update(ctx, renovateJob)
	})
	if err != nil {
		return nil, err
	}
	// Reload the object to ensure we have the latest state
	return reloadRenovateJob(ctx, renovateJob, client)
}

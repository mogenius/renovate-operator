package webhook

import (
	"context"
	api "renovate-operator/api/v1alpha1"
	crdmanager "renovate-operator/internal/crdManager"

	batchv1 "k8s.io/api/batch/v1"
)

// Mock RenovateJobManager for webhook integration tests
type mockWebhookManager struct {
	updateProjectStatusFunc     func(ctx context.Context, project string, jobId crdmanager.RenovateJobIdentifier, status api.RenovateProjectStatus) error
	isWebhookTokenValidFunc     func(ctx context.Context, job crdmanager.RenovateJobIdentifier, token string) (bool, error)
	isWebhookSignatureValidFunc func(ctx context.Context, job crdmanager.RenovateJobIdentifier, signature string, body []byte) (bool, error)
}

func (m *mockWebhookManager) UpdateProjectStatus(ctx context.Context, project string, jobId crdmanager.RenovateJobIdentifier, status api.RenovateProjectStatus) error {
	if m.updateProjectStatusFunc != nil {
		return m.updateProjectStatusFunc(ctx, project, jobId, status)
	}
	return nil
}

func (m *mockWebhookManager) IsWebhookTokenValid(ctx context.Context, job crdmanager.RenovateJobIdentifier, token string) (bool, error) {
	if m.isWebhookTokenValidFunc != nil {
		return m.isWebhookTokenValidFunc(ctx, job, token)
	}
	return true, nil
}

func (m *mockWebhookManager) IsWebhookSignatureValid(ctx context.Context, job crdmanager.RenovateJobIdentifier, signature string, body []byte) (bool, error) {
	if m.isWebhookSignatureValidFunc != nil {
		return m.isWebhookSignatureValidFunc(ctx, job, signature, body)
	}
	return true, nil
}

// Implement remaining interface methods as no-ops for webhook tests
func (m *mockWebhookManager) ListRenovateJobs(ctx context.Context) ([]crdmanager.RenovateJobIdentifier, error) {
	return nil, nil
}
func (m *mockWebhookManager) GetProjectsForRenovateJob(ctx context.Context, jobId crdmanager.RenovateJobIdentifier) ([]crdmanager.RenovateProjectStatus, error) {
	return nil, nil
}
func (m *mockWebhookManager) GetLogsForProject(ctx context.Context, jobId crdmanager.RenovateJobIdentifier, project string) (string, error) {
	return "", nil
}
func (m *mockWebhookManager) GetRenovateJob(ctx context.Context, name, namespace string) (*api.RenovateJob, error) {
	return nil, nil
}
func (m *mockWebhookManager) ReconcileProjects(ctx context.Context, jobId crdmanager.RenovateJobIdentifier, projects []string) error {
	return nil
}
func (m *mockWebhookManager) LoadRenovateJob(ctx context.Context, name, namespace string) (*api.RenovateJob, error) {
	return nil, nil
}
func (m *mockWebhookManager) ReloadRenovateJob(ctx context.Context, job *api.RenovateJob) error {
	return nil
}
func (m *mockWebhookManager) GetProjects(ctx context.Context, jobId crdmanager.RenovateJobIdentifier, filter func(crdmanager.RenovateProjectStatus) bool) ([]string, error) {
	return nil, nil
}
func (m *mockWebhookManager) GetProjectsByStatus(ctx context.Context, job crdmanager.RenovateJobIdentifier, status api.RenovateProjectStatus) ([]crdmanager.RenovateProjectStatus, error) {
	return nil, nil
}
func (m *mockWebhookManager) UpdateProjectStatusBatched(ctx context.Context, fn func(p api.ProjectStatus) bool, jobId crdmanager.RenovateJobIdentifier, status api.RenovateProjectStatus) error {
	return nil
}
func (m *mockWebhookManager) GetJob(jobId crdmanager.RenovateJobIdentifier, projectName string) (*batchv1.Job, error) {
	return nil, nil
}

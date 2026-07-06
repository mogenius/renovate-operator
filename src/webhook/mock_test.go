package webhook

import (
	"context"
	"io"
	"strings"

	api "renovate-operator/api/v1alpha1"
	crdmanager "renovate-operator/internal/crdManager"
	"renovate-operator/internal/types"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// mockWebhookManager is used by webhook integration tests.
type mockWebhookManager struct {
	listRenovateJobsFullFunc    func(ctx context.Context) ([]api.RenovateJob, error)
	updateProjectStatusFunc     func(ctx context.Context, project string, jobId crdmanager.RenovateJobIdentifier, status *types.RenovateStatusUpdate) error
	isWebhookTokenValidFunc     func(ctx context.Context, job crdmanager.RenovateJobIdentifier, token string) (bool, error)
	isWebhookSignatureValidFunc func(ctx context.Context, job crdmanager.RenovateJobIdentifier, signature string, body []byte) (bool, error)
}

func (m *mockWebhookManager) ListRenovateJobsFull(ctx context.Context) ([]api.RenovateJob, error) {
	if m.listRenovateJobsFullFunc != nil {
		return m.listRenovateJobsFullFunc(ctx)
	}
	return nil, nil
}

func (m *mockWebhookManager) UpdateProjectStatus(ctx context.Context, project string, jobId crdmanager.RenovateJobIdentifier, status *types.RenovateStatusUpdate) error {
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

func (m *mockWebhookManager) UpdateExecutionOptions(ctx context.Context, jobId crdmanager.RenovateJobIdentifier, options *api.RenovateExecutionOptions) error {
	return nil
}

func (m *mockWebhookManager) CancelProjectJob(ctx context.Context, project string, jobId crdmanager.RenovateJobIdentifier) error {
	return nil
}

func (m *mockWebhookManager) ListRenovateJobs(ctx context.Context) ([]crdmanager.RenovateJobIdentifier, error) {
	return nil, nil
}

func (m *mockWebhookManager) GetProjectsForRenovateJob(ctx context.Context, jobId crdmanager.RenovateJobIdentifier) ([]crdmanager.RenovateProjectStatus, error) {
	return nil, nil
}
func (m *mockWebhookManager) StreamLogsForProject(ctx context.Context, jobId crdmanager.RenovateJobIdentifier, project string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}

func (m *mockWebhookManager) GetRenovateJob(ctx context.Context, name, namespace string) (*api.RenovateJob, error) {
	return nil, nil
}

func (m *mockWebhookManager) SyncWebhooks(ctx context.Context, job crdmanager.RenovateJobIdentifier, removedProjects []string) error {
	return nil
}

func (m *mockWebhookManager) CleanupWebhooks(ctx context.Context, job crdmanager.RenovateJobIdentifier) error {
	return nil
}

func (m *mockWebhookManager) ReconcileProjects(ctx context.Context, jobId *api.RenovateJob, projects []string) ([]string, error) {
	return nil, nil
}

func (m *mockWebhookManager) UpdateProjectConfigStatus(ctx context.Context, project string, jobId crdmanager.RenovateJobIdentifier, status *string) error {
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

func (m *mockWebhookManager) UpdateProjectStatusBatched(ctx context.Context, fn func(p api.ProjectStatus) bool, jobId crdmanager.RenovateJobIdentifier, status *types.RenovateStatusUpdate) error {
	return nil
}

// makeTestRenovateJob builds a RenovateJob with webhook enabled (auth disabled) and a single project,
// used by integration tests to configure the resolver mock.
func makeTestRenovateJob(namespace, name, project string) api.RenovateJob {
	return api.RenovateJob{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: api.RenovateJobSpec{
			Webhook: &api.RenovateWebhook{
				Enabled: true,
			},
		},
		Status: api.RenovateJobStatus{
			Projects: []api.ProjectStatus{
				{Name: project},
			},
		},
	}
}

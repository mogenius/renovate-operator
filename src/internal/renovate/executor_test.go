package renovate

import (
	"context"
	"strings"
	"testing"

	api "renovate-operator/api/v1alpha1"
	"renovate-operator/config"
	crdManager "renovate-operator/internal/crdManager"
	"renovate-operator/internal/policy"
	"renovate-operator/internal/types"
	"renovate-operator/metricStore"

	"github.com/prometheus/client_golang/prometheus"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// fakeLogStore is a no-op LogStore for executor tests.
type fakeLogStore struct{}

func (fakeLogStore) Save(namespace, renovateJob, project, logs string) {}
func (fakeLogStore) Get(namespace, renovateJob, project string) (string, bool) {
	return "", false
}

// TestProcessProjectJobResultAbortedRunKeepsPRGauges reproduces the "metric drops to
// zero" incident: a run aborted with "repository-changed" (repo changed while Renovate
// was running) completes successfully but carries no branch data. Its result must not
// overwrite the last known approval/open-PR gauges or the persisted PRActivity.
func TestProcessProjectJobResultAbortedRunKeepsPRGauges(t *testing.T) {
	const (
		ns      = "renovate-operator"
		jobName = "gitlab"
		project = "org/repo"
	)

	_ = config.InitializeConfigModule([]config.ConfigItemDescription{
		{Key: "DELETE_SUCCESSFUL_JOBS", Optional: true, Default: "false"},
	})

	reg := prometheus.NewRegistry()
	metricStore.Register(reg)

	// Last clean run left these values behind.
	metricStore.SetApprovalsNeeded(ns, jobName, project, 7)
	metricStore.SetOpenPullRequests(ns, jobName, project, 6)

	abortedLogs := strings.Join([]string{
		`{"level":30,"msg":"Renovate started"}`,
		`{"level":30,"msg":"Repository started"}`,
		`{"level":30,"msg":"Dependency extraction complete"}`,
		`{"level":30,"msg":"Repository has changed during renovation - aborting"}`,
		`{"level":30,"msg":"Repository finished","result":"repository-changed"}`,
		`{"level":30,"msg":"Printing report","report":{"repositories":{"org/repo":{"branches":[]}}}}`,
	}, "\n")

	var captured *types.RenovateStatusUpdate
	manager := &fakeJobManager{
		getProjectsByStatusFn: func(ctx context.Context, job crdManager.RenovateJobIdentifier, status api.RenovateProjectStatus) ([]crdManager.RenovateProjectStatus, error) {
			return []crdManager.RenovateProjectStatus{{Name: project}}, nil
		},
		updateProjectStatusFn: func(ctx context.Context, p string, job crdManager.RenovateJobIdentifier, status *types.RenovateStatusUpdate) error {
			captured = status
			return nil
		},
	}
	logReader := &fakePodLogReader{
		getLastJobLogFn: func(ctx context.Context, job *batchv1.Job) (string, error) {
			return abortedLogs, nil
		},
	}

	scheme := runtime.NewScheme()
	if err := batchv1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add batch scheme: %v", err)
	}
	now := metav1.Now()
	k8sJob := &batchv1.Job{}
	k8sJob.Name = "gitlab-org-repo-abc123"
	k8sJob.Namespace = ns
	k8sJob.Status = batchv1.JobStatus{
		Conditions:     []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}},
		StartTime:      &now,
		CompletionTime: &now,
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(k8sJob).Build()

	executor := NewRenovateExecutor(scheme, manager, k8sClient, testLogger, nil, fakeLogStore{}, logReader, policy.Policy{})

	jobId := crdManager.RenovateJobIdentifier{Name: jobName, Namespace: ns}
	if err := executor.ProcessProjectJobResult(context.Background(), k8sJob, project, jobId); err != nil {
		t.Fatalf("ProcessProjectJobResult() error = %v", err)
	}

	if captured == nil {
		t.Fatal("UpdateProjectStatus was not called")
	}
	if captured.PRActivity != nil {
		t.Errorf("persisted PRActivity = %+v, want nil for an aborted run", captured.PRActivity)
	}

	labels := map[string]string{"renovate_namespace": ns, "renovate_job": jobName, "project": project}
	if got := gaugeValue(t, reg, "renovate_operator_approvals_needed", labels); got != 7 {
		t.Errorf("renovate_operator_approvals_needed = %v, want 7 (kept from last clean run)", got)
	}
	if got := gaugeValue(t, reg, "renovate_operator_open_pull_requests", labels); got != 6 {
		t.Errorf("renovate_operator_open_pull_requests = %v, want 6 (kept from last clean run)", got)
	}
}

// gaugeValue reads a single gauge with the given labels from the registry.
func gaugeValue(t *testing.T, reg *prometheus.Registry, name string, labels map[string]string) float64 {
	t.Helper()
	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}
	for _, mf := range families {
		if mf.GetName() != name {
			continue
		}
	metric:
		for _, m := range mf.GetMetric() {
			for k, v := range labels {
				found := false
				for _, lp := range m.GetLabel() {
					if lp.GetName() == k && lp.GetValue() == v {
						found = true
						break
					}
				}
				if !found {
					continue metric
				}
			}
			return m.GetGauge().GetValue()
		}
	}
	t.Fatalf("metric %s with labels %v not found", name, labels)
	return 0
}

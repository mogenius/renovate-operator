package renovate

import (
	"context"
	"strings"
	"sync"
	"testing"

	api "renovate-operator/api/v1alpha1"
	"renovate-operator/config"
	crdManager "renovate-operator/internal/crdManager"
	"renovate-operator/internal/logStore"
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

// recordingLogStore remembers which projects had their logs saved.
type recordingLogStore struct {
	mutex sync.Mutex
	saved map[string]string
}

func (s *recordingLogStore) Save(namespace, renovateJob, project, logs string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.saved == nil {
		s.saved = map[string]string{}
	}
	s.saved[project] = logs
}

func (s *recordingLogStore) Get(namespace, renovateJob, project string) (string, bool) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	logs, ok := s.saved[project]
	return logs, ok
}

// The log of a dry run of a custom branch whose config disables one of the default
// branch's PRs: Renovate's report still lists the remaining branches as "pr-edited",
// exactly like a real run would.
var dryRunLogs = strings.Join([]string{
	`{"level":30,"msg":"Renovate started"}`,
	`{"level":40,"msg":"Found renovate config warnings","baseBranch":"test/renovate-685"}`,
	`{"level":30,"msg":"DRY-RUN: Would update PR #3","baseBranch":"test/renovate-685"}`,
	`{"level":30,"msg":"DRY-RUN: Would autoclose PR"}`,
	`{"level":30,"msg":"Repository finished","result":"done","status":"onboarded"}`,
	`{"level":30,"msg":"Printing report","report":{"repositories":{"org/repo":{"branches":[` +
		`{"branchName":"renovate/backend","result":"pr-edited","prNo":3},` +
		`{"branchName":"renovate/frontend","result":"pr-edited","prNo":4}]}}}}`,
}, "\n")

// processFinishedRun feeds dryRunLogs through ProcessProjectJobResult for a completed
// Job with the given annotations, and returns the status update the executor persisted.
func processFinishedRun(t *testing.T, jobName, project string, annotations map[string]string, store logStore.LogStore) *types.RenovateStatusUpdate {
	t.Helper()
	const ns = "renovate-operator"

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
			return dryRunLogs, nil
		},
	}

	scheme := runtime.NewScheme()
	if err := batchv1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add batch scheme: %v", err)
	}
	now := metav1.Now()
	k8sJob := &batchv1.Job{}
	k8sJob.Name = jobName + "-org-repo-abc123"
	k8sJob.Namespace = ns
	k8sJob.Annotations = annotations
	k8sJob.Status = batchv1.JobStatus{
		Conditions:     []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}},
		StartTime:      &now,
		CompletionTime: &now,
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(k8sJob).Build()

	executor := NewRenovateExecutor(scheme, manager, k8sClient, testLogger, nil, store, logReader, policy.Policy{})
	jobId := crdManager.RenovateJobIdentifier{Name: jobName, Namespace: ns}
	if err := executor.ProcessProjectJobResult(context.Background(), k8sJob, project, jobId); err != nil {
		t.Fatalf("ProcessProjectJobResult() error = %v", err)
	}
	if captured == nil {
		t.Fatal("UpdateProjectStatus was not called")
	}
	return captured
}

func TestProcessProjectJobResultDryRunKeepsProjectDataAndMetrics(t *testing.T) {
	_ = config.InitializeConfigModule([]config.ConfigItemDescription{
		{Key: "DELETE_SUCCESSFUL_JOBS", Optional: true, Default: "false"},
	})
	reg := prometheus.NewRegistry()
	metricStore.Register(reg)

	const (
		dryRunJob  = "dry-run-job"
		realRunJob = "real-run-job"
		project    = "org/repo"
	)
	for _, jobName := range []string{dryRunJob, realRunJob} {
		// what the project's last real run left behind
		metricStore.SetOpenPullRequests("renovate-operator", jobName, project, 5)
		metricStore.SetApprovalsNeeded("renovate-operator", jobName, project, 2)
	}

	t.Run("a dry run only saves its log and finishes the run", func(t *testing.T) {
		savedLogs := &recordingLogStore{}
		status := processFinishedRun(t, dryRunJob, project, map[string]string{api.DryRunAnnotationKey: "true"}, savedLogs)

		if status.Status != api.JobStatusCompleted {
			t.Errorf("status = %q, want the run to finish as completed", status.Status)
		}
		if status.PRActivity != nil || status.LogIssues != nil || status.RenovateResultStatus != nil {
			t.Errorf("a dry run must not replace the project's data, got PRActivity=%+v LogIssues=%+v result=%v",
				status.PRActivity, status.LogIssues, status.RenovateResultStatus)
		}
		if logs, saved := savedLogs.Get("renovate-operator", dryRunJob, project); !saved || logs != dryRunLogs {
			t.Error("expected the dry run's log to be saved, it is where the result is read")
		}

		labels := map[string]string{"renovate_namespace": "renovate-operator", "renovate_job": dryRunJob, "project": project}
		if got := gaugeValue(t, reg, "renovate_operator_open_pull_requests", labels); got != 5 {
			t.Errorf("renovate_operator_open_pull_requests = %v, want 5 from the last real run", got)
		}
		if got := gaugeValue(t, reg, "renovate_operator_approvals_needed", labels); got != 2 {
			t.Errorf("renovate_operator_approvals_needed = %v, want 2 from the last real run", got)
		}
		jobLabels := map[string]string{"renovate_namespace": "renovate-operator", "renovate_job": dryRunJob}
		if got := counterValue(t, reg, "renovate_operator_pull_requests_updated_total", jobLabels); got != 0 {
			t.Errorf("renovate_operator_pull_requests_updated_total = %v, want 0: a dry run updates no PR", got)
		}
		if got := counterValue(t, reg, "renovate_operator_project_executions_total", labels); got != 0 {
			t.Errorf("renovate_operator_project_executions_total = %v, want 0: a dry run is not a project run", got)
		}
	})

	// The same log from a Job without the annotation is processed as a real run, which is
	// what the dry run case above is guarding against.
	t.Run("the same log from a real run replaces the project's data", func(t *testing.T) {
		status := processFinishedRun(t, realRunJob, project, nil, &recordingLogStore{})

		if status.PRActivity == nil || status.PRActivity.Updated != 2 {
			t.Fatalf("PRActivity = %+v, want the 2 updated PRs from the report", status.PRActivity)
		}
		labels := map[string]string{"renovate_namespace": "renovate-operator", "renovate_job": realRunJob, "project": project}
		if got := gaugeValue(t, reg, "renovate_operator_open_pull_requests", labels); got != 2 {
			t.Errorf("renovate_operator_open_pull_requests = %v, want 2 from this run", got)
		}
		jobLabels := map[string]string{"renovate_namespace": "renovate-operator", "renovate_job": realRunJob}
		if got := counterValue(t, reg, "renovate_operator_pull_requests_updated_total", jobLabels); got != 2 {
			t.Errorf("renovate_operator_pull_requests_updated_total = %v, want 2", got)
		}
	})
}

// counterValue sums the counter samples matching the given labels; 0 when there are none.
func counterValue(t *testing.T, reg *prometheus.Registry, name string, labels map[string]string) float64 {
	t.Helper()
	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}
	total := 0.0
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
	metric:
		for _, sample := range family.GetMetric() {
			for key, value := range labels {
				matched := false
				for _, pair := range sample.GetLabel() {
					if pair.GetName() == key && pair.GetValue() == value {
						matched = true
						break
					}
				}
				if !matched {
					continue metric
				}
			}
			total += sample.GetCounter().GetValue()
		}
	}
	return total
}

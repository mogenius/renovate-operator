package controllers

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	api "renovate-operator/api/v1alpha1"
	crdManager "renovate-operator/internal/crdManager"
	"renovate-operator/internal/types"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func makeRenovateProject(name, namespace, jobName, project string, annotations map[string]string) *api.RenovateProject {
	return &api.RenovateProject{
		TypeMeta: metav1.TypeMeta{
			APIVersion: api.GroupVersion.String(),
			Kind:       "RenovateProject",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				api.LabelRenovateJob: jobName,
			},
			Annotations: annotations,
		},
		Spec:   api.RenovateProjectSpec{Project: project},
		Status: api.RenovateProjectState{Status: api.JobStatusCompleted},
	}
}

// TestRenovateProjectReconciler_NoAnnotation verifies that reconcile is a no-op
// when the trigger annotation is absent.
func TestRenovateProjectReconciler_NoAnnotation(t *testing.T) {
	rp := makeRenovateProject("test-project-abc1", "default", "test-job", "org/repo", nil)
	k8s := buildFakeK8sClient(t, rp)

	var updateCalled bool
	mgr := &fakeProjectManager{
		updateStatusFn: func(_ context.Context, _ string, _ crdManager.RenovateJobIdentifier, _ *types.RenovateStatusUpdate) error {
			updateCalled = true
			return nil
		},
	}

	r := &RenovateProjectReconciler{Manager: mgr, K8sClient: k8s}
	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: crclient.ObjectKey{Name: rp.Name, Namespace: rp.Namespace}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updateCalled {
		t.Fatal("expected UpdateProjectStatus not to be called when annotation is absent")
	}
}

// TestRenovateProjectReconciler_ScheduleAnnotation verifies that the schedule annotation
// triggers UpdateProjectStatus and is removed from the project on success.
func TestRenovateProjectReconciler_ScheduleAnnotation(t *testing.T) {
	rp := makeRenovateProject("test-project-abc1", "default", "test-job", "org/repo", map[string]string{
		api.TriggerScheduleAnnotationKey: "true",
	})
	k8s := buildFakeK8sClient(t, rp)

	var scheduledProject string
	var scheduledJob crdManager.RenovateJobIdentifier
	mgr := &fakeProjectManager{
		updateStatusFn: func(_ context.Context, project string, job crdManager.RenovateJobIdentifier, status *types.RenovateStatusUpdate) error {
			scheduledProject = project
			scheduledJob = job
			return nil
		},
	}

	r := &RenovateProjectReconciler{Manager: mgr, K8sClient: k8s}
	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: crclient.ObjectKey{Name: rp.Name, Namespace: rp.Namespace}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if scheduledProject != "org/repo" {
		t.Fatalf("expected project org/repo to be scheduled, got %q", scheduledProject)
	}
	if scheduledJob.Name != "test-job" || scheduledJob.Namespace != "default" {
		t.Fatalf("unexpected job identifier: %+v", scheduledJob)
	}

	var updated api.RenovateProject
	if err := k8s.Get(context.Background(), crclient.ObjectKey{Name: rp.Name, Namespace: rp.Namespace}, &updated); err != nil {
		t.Fatalf("failed to re-read project: %v", err)
	}
	if updated.Annotations[api.TriggerScheduleAnnotationKey] != "" {
		t.Fatal("expected schedule annotation to be removed after reconcile")
	}
}

// TestRenovateProjectReconciler_ProjectNotFound verifies that ErrProjectNotFound removes
// the annotation without returning an error (stale project — no retry needed).
func TestRenovateProjectReconciler_ProjectNotFound(t *testing.T) {
	rp := makeRenovateProject("test-project-abc1", "default", "test-job", "org/repo", map[string]string{
		api.TriggerScheduleAnnotationKey: "true",
	})
	k8s := buildFakeK8sClient(t, rp)

	mgr := &fakeProjectManager{
		updateStatusFn: func(_ context.Context, _ string, _ crdManager.RenovateJobIdentifier, _ *types.RenovateStatusUpdate) error {
			return crdManager.ErrProjectNotFound
		},
	}

	r := &RenovateProjectReconciler{Manager: mgr, K8sClient: k8s}
	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: crclient.ObjectKey{Name: rp.Name, Namespace: rp.Namespace}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var updated api.RenovateProject
	if err := k8s.Get(context.Background(), crclient.ObjectKey{Name: rp.Name, Namespace: rp.Namespace}, &updated); err != nil {
		t.Fatalf("failed to re-read project: %v", err)
	}
	if updated.Annotations[api.TriggerScheduleAnnotationKey] != "" {
		t.Fatal("expected annotation to be removed after ErrProjectNotFound")
	}
}

// TestRenovateProjectReconciler_MissingJobLabel verifies that a project without the
// renovatejob label causes no scheduling and no error.
func TestRenovateProjectReconciler_MissingJobLabel(t *testing.T) {
	rp := &api.RenovateProject{
		TypeMeta: metav1.TypeMeta{APIVersion: api.GroupVersion.String(), Kind: "RenovateProject"},
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-project-abc1",
			Namespace:   "default",
			Annotations: map[string]string{api.TriggerScheduleAnnotationKey: "true"},
		},
		Spec:   api.RenovateProjectSpec{Project: "org/repo"},
		Status: api.RenovateProjectState{Status: api.JobStatusCompleted},
	}
	k8s := buildFakeK8sClient(t, rp)

	var updateCalled bool
	mgr := &fakeProjectManager{
		updateStatusFn: func(_ context.Context, _ string, _ crdManager.RenovateJobIdentifier, _ *types.RenovateStatusUpdate) error {
			updateCalled = true
			return nil
		},
	}

	r := &RenovateProjectReconciler{Manager: mgr, K8sClient: k8s}
	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: crclient.ObjectKey{Name: rp.Name, Namespace: rp.Namespace}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updateCalled {
		t.Fatal("expected UpdateProjectStatus not to be called when job label is missing")
	}
}

// fakeProjectManager implements the RenovateJobManager interface for project reconciler tests.
type fakeProjectManager struct {
	updateStatusFn func(ctx context.Context, project string, job crdManager.RenovateJobIdentifier, status *types.RenovateStatusUpdate) error
}

func (f *fakeProjectManager) UpdateProjectStatus(ctx context.Context, project string, job crdManager.RenovateJobIdentifier, status *types.RenovateStatusUpdate) error {
	if f.updateStatusFn != nil {
		return f.updateStatusFn(ctx, project, job, status)
	}
	return nil
}

func (f *fakeProjectManager) ListRenovateJobs(_ context.Context) ([]crdManager.RenovateJobIdentifier, error) {
	return nil, fmt.Errorf("not implemented")
}
func (f *fakeProjectManager) ListRenovateJobsFull(_ context.Context) ([]api.RenovateJob, error) {
	return nil, fmt.Errorf("not implemented")
}
func (f *fakeProjectManager) GetRenovateJob(_ context.Context, _, _ string) (*api.RenovateJob, error) {
	return nil, fmt.Errorf("not implemented")
}
func (f *fakeProjectManager) GetProjectsForRenovateJob(_ context.Context, _ crdManager.RenovateJobIdentifier) ([]crdManager.RenovateProjectStatus, error) {
	return nil, fmt.Errorf("not implemented")
}
func (f *fakeProjectManager) UpdateProjectStatusBatched(_ context.Context, _ func(crdManager.RenovateProjectStatus) bool, _ crdManager.RenovateJobIdentifier, _ *types.RenovateStatusUpdate) error {
	return fmt.Errorf("not implemented")
}
func (f *fakeProjectManager) GetProjectsByStatus(_ context.Context, _ crdManager.RenovateJobIdentifier, _ api.RenovateProjectStatus) ([]crdManager.RenovateProjectStatus, error) {
	return nil, fmt.Errorf("not implemented")
}
func (f *fakeProjectManager) ReconcileProjects(_ context.Context, _ *api.RenovateJob, _ []string) ([]string, error) {
	return nil, fmt.Errorf("not implemented")
}
func (f *fakeProjectManager) SyncWebhooks(_ context.Context, _ crdManager.RenovateJobIdentifier, _ []string) error {
	return fmt.Errorf("not implemented")
}
func (f *fakeProjectManager) CleanupWebhooks(_ context.Context, _ crdManager.RenovateJobIdentifier) error {
	return fmt.Errorf("not implemented")
}
func (f *fakeProjectManager) StreamLogsForProject(_ context.Context, _ crdManager.RenovateJobIdentifier, _ string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}
func (f *fakeProjectManager) IsWebhookTokenValid(_ context.Context, _ crdManager.RenovateJobIdentifier, _ string) (bool, error) {
	return false, fmt.Errorf("not implemented")
}
func (f *fakeProjectManager) IsWebhookSignatureValid(_ context.Context, _ crdManager.RenovateJobIdentifier, _ string, _ []byte) (bool, error) {
	return false, fmt.Errorf("not implemented")
}
func (f *fakeProjectManager) IsWebhookStandardSignatureValid(_ context.Context, _ crdManager.RenovateJobIdentifier, _, _, _ string, _ []byte) (bool, error) {
	return false, fmt.Errorf("not implemented")
}
func (f *fakeProjectManager) SetAcceptedCondition(_ context.Context, _ crdManager.RenovateJobIdentifier, _ bool, _, _ string) error {
	return fmt.Errorf("not implemented")
}
func (f *fakeProjectManager) CancelProjectJob(_ context.Context, _ string, _ crdManager.RenovateJobIdentifier) error {
	return fmt.Errorf("not implemented")
}

package crdmanager

import (
	"context"
	"testing"

	api "renovate-operator/api/v1alpha1"
	"renovate-operator/testhelpers"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestLoadRenovateJob_Success(t *testing.T) {
	job := &api.RenovateJob{}
	job.Name = "my-job"
	job.Namespace = "default"
	job.ObjectMeta = metav1.ObjectMeta{Name: job.Name, Namespace: job.Namespace}

	// register CRD scheme so fake client knows RenovateJob GVK
	scheme := runtime.NewScheme()
	if err := api.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add api scheme: %v", err)
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(job).Build()

	got, err := loadRenovateJob(context.Background(), "my-job", "default", cl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name != "my-job" || got.Namespace != "default" {
		t.Fatalf("unexpected renovate job returned: %v", got)
	}
}

func TestLoadRenovateJob_NotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := api.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add api scheme: %v", err)
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()

	testhelpers.WithShortRetries(t, 1, 1, func() {
		// inside this closure retries are shortened

		_, err := loadRenovateJob(context.Background(), "missing", "default", cl)
		if err == nil {
			t.Fatalf("expected error when renovatejob not found")
		}
	})
}

func TestReloadRenovateJob(t *testing.T) {
	job := &api.RenovateJob{}
	job.Name = "my-job"
	job.Namespace = "default"
	job.ObjectMeta = metav1.ObjectMeta{Name: job.Name, Namespace: job.Namespace}

	// register CRD scheme so fake client knows RenovateJob GVK
	scheme := runtime.NewScheme()
	if err := api.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add api scheme: %v", err)
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(job).Build()

	got, err := reloadRenovateJob(context.Background(), job, cl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name != "my-job" {
		t.Fatalf("unexpected job: %v", got)
	}
}

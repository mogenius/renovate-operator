package crdmanager

import (
	"context"
	"testing"

	api "renovate-operator/api/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func resolveClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := api.AddToScheme(scheme); err != nil {
		t.Fatalf("add api scheme: %v", err)
	}
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
}

func TestResolveEffectiveJob_MergesNamespacedTemplate(t *testing.T) {
	template := &api.RenovateJobTemplate{}
	template.ObjectMeta = metav1.ObjectMeta{Name: "base", Namespace: "renovate"}
	template.Spec = api.RenovateJobSpec{
		Schedule:    "0 */6 * * *",
		Image:       "renovate/renovate:41",
		Parallelism: 2,
		Provider:    &api.RenovateProvider{Name: "forgejo"},
	}

	job := &api.RenovateJob{}
	job.ObjectMeta = metav1.ObjectMeta{Name: "forgejo-homelab", Namespace: "renovate"}
	job.Spec = api.RenovateJobSpec{
		TemplateRef:      &api.RenovateJobTemplateRef{Name: "base"},
		DiscoveryFilters: []string{"homelab/*"},
	}

	c := resolveClient(t, template, job)

	got, err := resolveEffectiveJob(context.Background(), job, c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Spec.Schedule != "0 */6 * * *" {
		t.Errorf("schedule not inherited: %q", got.Spec.Schedule)
	}
	if got.Spec.Image != "renovate/renovate:41" {
		t.Errorf("image not inherited: %q", got.Spec.Image)
	}
	if got.Spec.Parallelism != 2 {
		t.Errorf("parallelism not inherited: %d", got.Spec.Parallelism)
	}
	if got.Spec.Provider == nil || got.Spec.Provider.Name != "forgejo" {
		t.Errorf("provider not inherited: %v", got.Spec.Provider)
	}
}

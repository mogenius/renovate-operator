package renovate

import (
	"reflect"
	"testing"

	api "renovate-operator/api/v1alpha1"
	"renovate-operator/config"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewDiscoveryJob(t *testing.T) {
	err := config.InitializeConfigModule([]config.ConfigItemDescription{
		{
			Key:      "JOB_TIMEOUT_SECONDS",
			Optional: true,
			Default:  "1800",
		},
	})
	if err != nil {
		t.Errorf("expected to initialize config module without error, got %v", err)
	}

	job := &api.RenovateJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testjob",
			Namespace: "default",
		},
		Spec: api.RenovateJobSpec{
			Image:           "renovate:latest",
			DiscoveryFilter: "org/*",
			SecretRef:       "mysecret",
			ExtraEnv: []v1.EnvVar{
				{Name: "FOO", Value: "BAR"},
			},
			Resources: v1.ResourceRequirements{
				Limits: v1.ResourceList{
					v1.ResourceCPU:    resource.MustParse("100m"),
					v1.ResourceMemory: resource.MustParse("128Mi"),
				},
			},
		},
	}

	got := newDiscoveryJob(job)
	if got.Name != "testjob-discovery" {
		t.Errorf("expected job name %q, got %q", "testjob-discovery", got.Name)
	}
	if got.Namespace != "default" {
		t.Errorf("expected namespace %q, got %q", "default", got.Namespace)
	}
	container := got.Spec.Template.Spec.Containers[0]
	if container.Image != "renovate:latest" {
		t.Errorf("expected image %q, got %q", "renovate:latest", container.Image)
	}
	found := false
	for _, env := range container.Env {
		if env.Name == "RENOVATE_AUTODISCOVER_FILTER" && env.Value == "org/*" {
			found = true
		}
	}
	if !found {
		t.Errorf("RENOVATE_AUTODISCOVER_FILTER env var not found or incorrect")
	}
	found = false
	for _, env := range container.Env {
		if env.Name == "FOO" && env.Value == "BAR" {
			found = true
		}
	}
	if !found {
		t.Errorf("ExtraEnv FOO=BAR not found")
	}
	if len(container.EnvFrom) != 1 || container.EnvFrom[0].SecretRef == nil || container.EnvFrom[0].SecretRef.Name != "mysecret" {
		t.Errorf("expected secretRef mysecret, got %+v", container.EnvFrom)
	}
	if got.Spec.Template.Spec.RestartPolicy != v1.RestartPolicyOnFailure {
		t.Errorf("expected RestartPolicyOnFailure, got %v", got.Spec.Template.Spec.RestartPolicy)
	}
	if len(got.Spec.Template.Spec.Volumes) != 1 || got.Spec.Template.Spec.Volumes[0].Name != "tmp" {
		t.Errorf("expected tmp volume, got %+v", got.Spec.Template.Spec.Volumes)
	}
}

func TestNewDiscoveryJob_NoFilterOrSecret(t *testing.T) {
	job := &api.RenovateJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nofilter",
			Namespace: "ns",
		},
		Spec: api.RenovateJobSpec{
			Image: "renovate:dev",
		},
	}
	got := newDiscoveryJob(job)
	container := got.Spec.Template.Spec.Containers[0]
	for _, env := range container.Env {
		if env.Name == "RENOVATE_AUTODISCOVER_FILTER" {
			t.Errorf("did not expect RENOVATE_AUTODISCOVER_FILTER env var")
		}
	}
	if len(container.EnvFrom) != 0 {
		t.Errorf("expected no EnvFrom, got %+v", container.EnvFrom)
	}
}

func TestNewRenovateJob(t *testing.T) {
	job := &api.RenovateJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "execjob",
			Namespace: "renovate-ns",
		},
		Spec: api.RenovateJobSpec{
			Image: "renovate:prod",
			ExtraEnv: []v1.EnvVar{
				{Name: "BAR", Value: "BAZ"},
			},
		},
	}
	project := "my/repo"
	expectedName := job.ExecutorJobName(project)
	got := newRenovateJob(job, project)
	if got.Name != expectedName {
		t.Errorf("expected job name %q, got %q", expectedName, got.Name)
	}
	if got.Namespace != "renovate-ns" {
		t.Errorf("expected namespace %q, got %q", "renovate-ns", got.Namespace)
	}
	container := got.Spec.Template.Spec.Containers[0]
	if container.Image != "renovate:prod" {
		t.Errorf("expected image %q, got %q", "renovate:prod", container.Image)
	}
	if !reflect.DeepEqual(container.Command, []string{"renovate"}) {
		t.Errorf("expected command [renovate], got %v", container.Command)
	}
	if !reflect.DeepEqual(container.Args, []string{"--base-dir", "/tmp", project}) {
		t.Errorf("expected args [%s], got %v", project, container.Args)
	}
	found := false
	for _, env := range container.Env {
		if env.Name == "BAR" && env.Value == "BAZ" {
			found = true
		}
	}
	if !found {
		t.Errorf("ExtraEnv BAR=BAZ not found")
	}
}

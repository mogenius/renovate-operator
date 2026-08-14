package renovate

import (
	"context"
	"slices"
	"strings"
	"testing"

	api "renovate-operator/api/v1alpha1"
	"renovate-operator/config"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func configJob(cfg *api.RenovateJobConfig) *api.RenovateJob {
	job := new(api.RenovateJob)
	job.ObjectMeta = metav1.ObjectMeta{Name: "myjob", Namespace: "default", UID: "uid-1"}
	job.Spec = api.RenovateJobSpec{RenovateConfig: cfg}
	return job
}

// configClient builds a fake client holding the job (so the marker annotation can
// be updated) and refreshes the job's resourceVersion from it.
func configClient(t *testing.T, job *api.RenovateJob, objs ...client.Object) client.Client {
	t.Helper()
	c := fake.NewClientBuilder().WithScheme(policyScheme(t)).WithObjects(append([]client.Object{job}, objs...)...).Build()
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(job), job); err != nil {
		t.Fatalf("get job: %v", err)
	}
	return c
}

func jobHasMarker(t *testing.T, c client.Client, job *api.RenovateJob) bool {
	t.Helper()
	fresh := new(api.RenovateJob)
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(job), fresh); err != nil {
		t.Fatalf("get job: %v", err)
	}
	return fresh.Annotations[api.RenovateConfigMapAnnotationKey] != ""
}

func getOwnedConfigMap(t *testing.T, c client.Client, job *api.RenovateJob) (*corev1.ConfigMap, bool) {
	t.Helper()
	cm := new(corev1.ConfigMap)
	err := c.Get(context.Background(), client.ObjectKey{Name: renovateConfigMapName(job), Namespace: job.Namespace}, cm)
	if err != nil {
		return nil, false
	}
	return cm, true
}

func TestEnsureRenovateConfigMap_Inline(t *testing.T) {
	job := configJob(&api.RenovateJobConfig{Inline: "module.exports = {};"})
	c := configClient(t, job)

	if err := EnsureRenovateConfigMap(context.Background(), c, job); err != nil {
		t.Fatalf("ensure failed: %v", err)
	}

	cm, ok := getOwnedConfigMap(t, c, job)
	if !ok {
		t.Fatal("expected the config ConfigMap to be created")
	}
	if cm.Data["config.js"] != "module.exports = {};" {
		t.Errorf("unexpected data: %v", cm.Data)
	}
	if len(cm.OwnerReferences) != 1 || cm.OwnerReferences[0].Name != "myjob" {
		t.Errorf("expected the RenovateJob as owner, got %v", cm.OwnerReferences)
	}
	if cm.Labels[api.LabelAppManagedBy] != api.LabelValueManagedBy {
		t.Errorf("expected managed-by label, got %v", cm.Labels)
	}
	if !jobHasMarker(t, c, job) {
		t.Error("expected the marker annotation on the RenovateJob")
	}
}

func TestEnsureRenovateConfigMap_UpdateReplacesFileName(t *testing.T) {
	job := configJob(&api.RenovateJobConfig{Inline: "{}", FileName: "config.json"})
	c := configClient(t, job)

	if err := EnsureRenovateConfigMap(context.Background(), c, job); err != nil {
		t.Fatalf("ensure failed: %v", err)
	}
	job.Spec.RenovateConfig.FileName = "config.json5"
	if err := EnsureRenovateConfigMap(context.Background(), c, job); err != nil {
		t.Fatalf("second ensure failed: %v", err)
	}

	cm, _ := getOwnedConfigMap(t, c, job)
	if _, stale := cm.Data["config.json"]; stale {
		t.Error("expected the old key to be removed")
	}
	if cm.Data["config.json5"] != "{}" {
		t.Errorf("unexpected data: %v", cm.Data)
	}
}

func TestEnsureRenovateConfigMap_DeletesWhenInlineRemoved(t *testing.T) {
	job := configJob(&api.RenovateJobConfig{Inline: "{}"})
	c := configClient(t, job)

	if err := EnsureRenovateConfigMap(context.Background(), c, job); err != nil {
		t.Fatalf("ensure failed: %v", err)
	}
	job.Spec.RenovateConfig = &api.RenovateJobConfig{ConfigMapRef: &api.RenovateConfigMapKeyReference{Name: "user-cm", Key: "config.js"}}
	if err := EnsureRenovateConfigMap(context.Background(), c, job); err != nil {
		t.Fatalf("second ensure failed: %v", err)
	}

	if _, ok := getOwnedConfigMap(t, c, job); ok {
		t.Error("expected the owned ConfigMap to be deleted when switching to configMapRef")
	}
	if jobHasMarker(t, c, job) {
		t.Error("expected the marker annotation to be removed with the ConfigMap")
	}
}

func TestEnsureRenovateConfigMap_SkipsCleanupWithoutMarker(t *testing.T) {
	job := configJob(&api.RenovateJobConfig{Inline: "{}"})
	c := configClient(t, job)

	if err := EnsureRenovateConfigMap(context.Background(), c, job); err != nil {
		t.Fatalf("ensure failed: %v", err)
	}
	delete(job.Annotations, api.RenovateConfigMapAnnotationKey)
	if err := c.Update(context.Background(), job); err != nil {
		t.Fatalf("stripping the marker failed: %v", err)
	}
	job.Spec.RenovateConfig = nil
	if err := EnsureRenovateConfigMap(context.Background(), c, job); err != nil {
		t.Fatalf("second ensure failed: %v", err)
	}

	if _, ok := getOwnedConfigMap(t, c, job); !ok {
		t.Error("expected cleanup to be skipped for a job without the marker annotation")
	}
}

func foreignConfigMap(job *api.RenovateJob) *corev1.ConfigMap {
	cm := new(corev1.ConfigMap)
	cm.Name = renovateConfigMapName(job)
	cm.Namespace = job.Namespace
	cm.Data = map[string]string{"config.js": "user data"}
	return cm
}

func TestEnsureRenovateConfigMap_LeavesForeignConfigMapAlone(t *testing.T) {
	job := configJob(nil)
	job.Annotations = map[string]string{api.RenovateConfigMapAnnotationKey: "true"}
	c := configClient(t, job, foreignConfigMap(job))

	if err := EnsureRenovateConfigMap(context.Background(), c, job); err != nil {
		t.Fatalf("ensure failed: %v", err)
	}
	if _, ok := getOwnedConfigMap(t, c, job); !ok {
		t.Error("expected the foreign ConfigMap to survive cleanup")
	}
	if jobHasMarker(t, c, job) {
		t.Error("expected the marker annotation to be removed once there is nothing to clean")
	}
}

func TestEnsureRenovateConfigMap_RefusesToAdoptForeignConfigMap(t *testing.T) {
	job := configJob(&api.RenovateJobConfig{Inline: "{}"})
	c := configClient(t, job, foreignConfigMap(job))

	if err := EnsureRenovateConfigMap(context.Background(), c, job); err == nil {
		t.Fatal("expected an error refusing to adopt the foreign ConfigMap")
	}
	cm, _ := getOwnedConfigMap(t, c, job)
	if cm.Data["config.js"] != "user data" {
		t.Errorf("expected the foreign ConfigMap to be untouched, got %v", cm.Data)
	}
}

func TestEnsureRenovateConfigMap_NoopWithoutConfig(t *testing.T) {
	job := configJob(nil)
	c := configClient(t, job)

	if err := EnsureRenovateConfigMap(context.Background(), c, job); err != nil {
		t.Fatalf("ensure failed: %v", err)
	}
	if _, ok := getOwnedConfigMap(t, c, job); ok {
		t.Error("expected no ConfigMap without renovateConfig")
	}
	if jobHasMarker(t, c, job) {
		t.Error("expected no marker annotation without renovateConfig")
	}
}

func TestJobDefinitions_RenovateConfigInline(t *testing.T) {
	job := configJob(&api.RenovateJobConfig{Inline: "{}"})
	job.Spec.Image = "renovate/renovate:latest"

	k8sJob := newRenovateJob(job, "org/repo", nil, "")
	pod := k8sJob.Spec.Template.Spec

	if !hasEnv(pod.Containers[0].Env, "RENOVATE_CONFIG_FILE", "/etc/renovate/config.js") {
		t.Errorf("expected RENOVATE_CONFIG_FILE env, got %v", pod.Containers[0].Env)
	}
	vol := find(pod.Volumes, byName[corev1.Volume](renovateConfigVolumeName))
	if vol == nil || vol.ConfigMap == nil || vol.ConfigMap.Name != renovateConfigMapName(job) {
		t.Fatalf("expected volume from the owned ConfigMap, got %+v", vol)
	}
	if len(vol.ConfigMap.Items) != 1 || vol.ConfigMap.Items[0].Key != "config.js" {
		t.Errorf("expected the config file projected by key, got %+v", vol.ConfigMap.Items)
	}
	if mount := find(pod.Containers[0].VolumeMounts, byName[corev1.VolumeMount](renovateConfigVolumeName)); mount == nil || mount.MountPath != renovateConfigMountPath || !mount.ReadOnly {
		t.Errorf("expected a read-only mount at %s, got %+v", renovateConfigMountPath, mount)
	}
}

func TestJobDefinitions_RenovateConfigMapRef(t *testing.T) {
	job := configJob(&api.RenovateJobConfig{ConfigMapRef: &api.RenovateConfigMapKeyReference{Name: "user-cm", Key: "renovate.json"}})
	job.Spec.Image = "renovate/renovate:latest"

	k8sJob := newDiscoveryJob(job, "")
	pod := k8sJob.Spec.Template.Spec

	if !hasEnv(pod.Containers[0].Env, "RENOVATE_CONFIG_FILE", "/etc/renovate/renovate.json") {
		t.Errorf("expected RENOVATE_CONFIG_FILE env, got %v", pod.Containers[0].Env)
	}
	vol := find(pod.Volumes, byName[corev1.Volume](renovateConfigVolumeName))
	if vol == nil || vol.ConfigMap == nil || vol.ConfigMap.Name != "user-cm" {
		t.Fatalf("expected volume from the referenced ConfigMap, got %+v", vol)
	}
	if len(vol.ConfigMap.Items) != 1 || vol.ConfigMap.Items[0].Key != "renovate.json" {
		t.Errorf("expected the config file projected by key, got %+v", vol.ConfigMap.Items)
	}
}

func TestJobDefinitions_NoRenovateConfig(t *testing.T) {
	job := configJob(nil)
	job.Spec.Image = "renovate/renovate:latest"

	pod := newRenovateJob(job, "org/repo", nil, "").Spec.Template.Spec
	for _, env := range pod.Containers[0].Env {
		if env.Name == "RENOVATE_CONFIG_FILE" {
			t.Error("expected no RENOVATE_CONFIG_FILE env without renovateConfig")
		}
	}
	if find(pod.Volumes, byName[corev1.Volume](renovateConfigVolumeName)) != nil {
		t.Error("expected no config volume without renovateConfig")
	}
}

func TestJobDefinitions_ExtraEnvOverridesRenovateConfigFile(t *testing.T) {
	err := config.InitializeConfigModule([]config.ConfigItemDescription{})
	if err != nil {
		t.Fatalf("expected to initialize config module without error, got %v", err)
	}

	job := configJob(&api.RenovateJobConfig{Inline: "{}"})
	job.Spec.Image = "renovate/renovate:latest"
	job.Spec.ExtraEnv = []corev1.EnvVar{{Name: "RENOVATE_CONFIG_FILE", Value: "/custom/config.js"}}

	pod := newRenovateJob(job, "org/repo", nil, "").Spec.Template.Spec
	var values []string
	for _, env := range pod.Containers[0].Env {
		if env.Name == "RENOVATE_CONFIG_FILE" {
			values = append(values, env.Value)
		}
	}
	if !slices.Equal(values, []string{"/custom/config.js"}) {
		t.Errorf("expected only the extraEnv value, got %v", values)
	}
}

func hasEnv(envs []corev1.EnvVar, name, value string) bool {
	return find(envs, func(e corev1.EnvVar) bool { return e.Name == name && e.Value == value }) != nil
}

func find[T any](items []T, pred func(T) bool) *T {
	if i := slices.IndexFunc(items, pred); i >= 0 {
		return &items[i]
	}
	return nil
}

func byName[T interface {
	corev1.Volume | corev1.VolumeMount
}](name string) func(T) bool {
	return func(item T) bool {
		switch v := any(item).(type) {
		case corev1.Volume:
			return v.Name == name
		case corev1.VolumeMount:
			return v.Name == name
		}
		return false
	}
}

func TestRenovateConfigMapName(t *testing.T) {
	job := new(api.RenovateJob)
	job.Name = strings.Repeat("a", 80)
	name := renovateConfigMapName(job)
	if len(name) > 63 {
		t.Errorf("name exceeds 63 chars: %d", len(name))
	}
	if !strings.Contains(name, "-renovate-config-") {
		t.Errorf("unexpected name: %s", name)
	}
}

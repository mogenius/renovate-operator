package renovate

import (
	"reflect"
	"testing"

	api "renovate-operator/api/v1alpha1"
	"renovate-operator/config"

	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

var (
	defaultPodSecurityContext = &v1.PodSecurityContext{
		RunAsUser:    ptr.To(int64(12021)),
		RunAsGroup:   ptr.To(int64(12021)),
		FSGroup:      ptr.To(int64(12021)),
		RunAsNonRoot: ptr.To(true),
		SeccompProfile: &v1.SeccompProfile{
			Type: v1.SeccompProfileTypeRuntimeDefault,
		},
	}

	defaultContainerSecurityContext = &v1.SecurityContext{
		RunAsUser:                ptr.To(int64(12021)),
		RunAsGroup:               ptr.To(int64(12021)),
		RunAsNonRoot:             ptr.To(true),
		ReadOnlyRootFilesystem:   ptr.To(false),
		Privileged:               ptr.To(false),
		AllowPrivilegeEscalation: ptr.To(false),
		SeccompProfile: &v1.SeccompProfile{
			Type: v1.SeccompProfileTypeRuntimeDefault,
		},
	}
)

func TestSecurityContextHelpers(t *testing.T) {
	var spec api.RenovateJobSpec

	podCtx := getPodSecurityContext(spec)
	if podCtx == nil || podCtx.RunAsUser == nil {
		t.Fatalf("expected default pod security context set")
	}

	contCtx := getContainerSecurityContext(spec)
	if contCtx == nil || contCtx.RunAsUser == nil {
		t.Fatalf("expected default container security context set")
	}

	// ServiceAccount token default
	if got := getAutoMountServiceAccountToken(spec); got == nil || *got != false {
		t.Fatalf("expected default automount false, got %v", got)
	}

	if name := getServiceAccountName(spec); name != "" {
		t.Fatalf("expected empty service account name, got %s", name)
	}
}
func TestNewJobs_WithSettings(t *testing.T) {
	job := &api.RenovateJob{
		ObjectMeta: metav1.ObjectMeta{Name: "rj", Namespace: "ns"},
		Spec: api.RenovateJobSpec{
			Image:           "img",
			SecretRef:       "sref",
			DiscoveryFilter: "org/*",
			DiscoverTopics:  "renovate",
			Metadata: &api.RenovateJobMetadata{
				Labels: map[string]string{"a": "b"},
			},
			ExtraVolumes: []v1.Volume{
				{
					Name: "extra-vol",
					VolumeSource: v1.VolumeSource{
						EmptyDir: &v1.EmptyDirVolumeSource{},
					},
				},
			},
			ExtraVolumeMounts: []v1.VolumeMount{
				{
					Name:      "extra-vol",
					MountPath: "/extra",
				},
			},
			ServiceAccount: &api.RenovateJobServiceAccount{
				AutomountServiceAccountToken: ptr.To(true),
				Name:                         "test",
			},
			NodeSelector: map[string]string{"disktype": "ssd"},
			Tolerations: []v1.Toleration{
				{
					Key:      "key1",
					Operator: v1.TolerationOpEqual,
					Value:    "value1",
					Effect:   v1.TaintEffectNoSchedule,
				},
			},
			Affinity: &v1.Affinity{
				NodeAffinity: &v1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
						NodeSelectorTerms: []v1.NodeSelectorTerm{
							{
								MatchExpressions: []v1.NodeSelectorRequirement{
									{
										Key:      "kubernetes.io/e2e-az-name",
										Operator: v1.NodeSelectorOpIn,
										Values:   []string{"e2e-az1", "e2e-az2"},
									},
								},
							},
						},
					},
				},
			},
			TopologySpreadConstraints: []v1.TopologySpreadConstraint{
				{
					MaxSkew:           1,
					TopologyKey:       "kubernetes.io/hostname",
					WhenUnsatisfiable: v1.ScheduleAnyway,
				},
			},
			ImagePullSecrets: []v1.LocalObjectReference{
				{
					Name: "my-pull-secret",
				},
			},
			Resources: v1.ResourceRequirements{
				Limits: v1.ResourceList{
					v1.ResourceCPU:    resource.MustParse("100m"),
					v1.ResourceMemory: resource.MustParse("128Mi"),
				},
			},
			SecurityContext: &api.RenovateJobSecurityContext{
				Pod: &v1.PodSecurityContext{
					RunAsUser: ptr.To(int64(15000)),
				},
				Container: &v1.SecurityContext{
					RunAsUser: ptr.To(int64(16000)),
				},
			},
		},
	}
	err := config.InitializeConfigModule([]config.ConfigItemDescription{
		{Key: "JOB_TIMEOUT_SECONDS", Optional: true, Default: "10"},
		{Key: "JOB_TTL_SECONDS_AFTER_FINISHED", Optional: true, Default: "360"},
	})
	if err != nil {
		t.Fatalf("expected to initialize config module without error, got %v", err)
	}

	// test discovery job
	dj := newDiscoveryJob(job)
	djContainer := expectContainer(t, dj)
	// basic fields
	expectJobName(t, dj, "rj-discovery-6987b484")
	expectJobNamespace(t, dj, "ns")
	expectLabels(t, dj, map[string]string{"a": "b"}, "discovery", "rj-discovery-6987b484")
	expectImage(t, djContainer, "img")
	expectRestartPolicy(t, dj, v1.RestartPolicyOnFailure)
	expectActiveDeadlineSeconds(t, dj, 10)
	expectTtlSecondsAfterFinished(t, dj, nil)

	// env vars
	expectEnvVar(t, djContainer, "RENOVATE_AUTODISCOVER_FILTER", "org/*")
	expectEnvVar(t, djContainer, "RENOVATE_AUTODISCOVER_TOPICS", "renovate")
	expectEnvFromSecret(t, djContainer, "sref")
	// volumes
	expectVolumeMounts(t, djContainer, []v1.VolumeMount{{Name: "tmp", MountPath: "/tmp"}, {Name: "extra-vol", MountPath: "/extra"}})
	expectVolumes(t, dj, []v1.Volume{{Name: "tmp"}, {Name: "extra-vol"}})
	// other
	expectServiceAccountSettings(t, dj, "test", ptr.To(true))
	expectSecurityContext(t, dj, djContainer, job.Spec.SecurityContext.Pod, job.Spec.SecurityContext.Container)
	expectImagePullSecrets(t, dj, []v1.LocalObjectReference{{Name: "my-pull-secret"}})
	// scheduling
	expectAffinity(t, dj, job.Spec.Affinity)
	expectNodeSelector(t, dj, map[string]string{"disktype": "ssd"})
	expectTolerations(t, dj, job.Spec.Tolerations)
	expectTopologySpreadConstraints(t, dj, job.Spec.TopologySpreadConstraints)

	// test renovate job
	rj := newRenovateJob(job, "proj")
	rjContainer := expectContainer(t, rj)
	// basic fields
	expectJobName(t, rj, "rj-proj-701b9b0a")
	expectJobNamespace(t, rj, "ns")
	expectLabels(t, rj, map[string]string{"a": "b"}, "executor", "rj-proj-701b9b0a")
	expectImage(t, rjContainer, "img")
	expectRestartPolicy(t, rj, v1.RestartPolicyOnFailure)
	expectActiveDeadlineSeconds(t, rj, 10)
	expectTtlSecondsAfterFinished(t, rj, ptr.To(int32(360)))

	// env vars
	expectEnvFromSecret(t, rjContainer, "sref")
	// volumes
	expectVolumeMounts(t, rjContainer, []v1.VolumeMount{{Name: "tmp", MountPath: "/tmp"}, {Name: "extra-vol", MountPath: "/extra"}})
	expectVolumes(t, rj, []v1.Volume{{Name: "tmp"}, {Name: "extra-vol"}})
	// other
	expectServiceAccountSettings(t, rj, "test", ptr.To(true))
	expectSecurityContext(t, rj, rjContainer, job.Spec.SecurityContext.Pod, job.Spec.SecurityContext.Container)
	expectImagePullSecrets(t, rj, []v1.LocalObjectReference{{Name: "my-pull-secret"}})
	// scheduling
	expectAffinity(t, rj, job.Spec.Affinity)
	expectNodeSelector(t, rj, map[string]string{"disktype": "ssd"})
	expectTolerations(t, rj, job.Spec.Tolerations)
	expectTopologySpreadConstraints(t, rj, job.Spec.TopologySpreadConstraints)
}

func TestNewJob_WithoutSettings(t *testing.T) {
	job := &api.RenovateJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nofilter",
			Namespace: "ns",
		},
		Spec: api.RenovateJobSpec{
			Image: "renovate:dev",
		},
	}
	err := config.InitializeConfigModule([]config.ConfigItemDescription{{Key: "JOB_TIMEOUT_SECONDS", Optional: true, Default: "10"}})
	if err != nil {
		t.Fatalf("expected to initialize config module without error, got %v", err)
	}

	// test discovery job
	dj := newDiscoveryJob(job)
	djContainer := expectContainer(t, dj)
	// basic fields
	expectJobName(t, dj, "nofilter-discovery-3006fe8c")
	expectJobNamespace(t, dj, "ns")
	expectImage(t, djContainer, "renovate:dev")
	expectTtlSecondsAfterFinished(t, dj, nil)

	// no env vars
	for _, env := range djContainer.Env {
		if env.Name == "RENOVATE_AUTODISCOVER_FILTER" {
			t.Errorf("did not expect RENOVATE_AUTODISCOVER_FILTER env var")
		}

		if env.Name == "RENOVATE_AUTODISCOVER_TOPICS" {
			t.Errorf("did not expect RENOVATE_AUTODISCOVER_TOPICS env var")
		}
	}
	if len(djContainer.EnvFrom) != 0 {
		t.Errorf("expected no EnvFrom, got %+v", djContainer.EnvFrom)
	}

	// volumes
	expectVolumeMounts(t, djContainer, []v1.VolumeMount{{Name: "tmp", MountPath: "/tmp"}})
	expectVolumes(t, dj, []v1.Volume{{Name: "tmp"}})

	expectServiceAccountSettings(t, dj, "", ptr.To(false))
	expectSecurityContext(t, dj, djContainer, defaultPodSecurityContext, defaultContainerSecurityContext)
	expectImagePullSecrets(t, dj, nil)

	// scheduling
	expectAffinity(t, dj, nil)
	expectNodeSelector(t, dj, nil)
	expectTolerations(t, dj, nil)
	expectTopologySpreadConstraints(t, dj, nil)

	// test renovate job
	rj := newRenovateJob(job, "myproj")
	rjContainer := expectContainer(t, rj)
	// basic fields
	expectJobName(t, rj, "nofilter-myproj-496e220d")
	expectJobNamespace(t, rj, "ns")
	expectImage(t, rjContainer, "renovate:dev")
	expectTtlSecondsAfterFinished(t, rj, nil)

	// no env vars
	if len(rjContainer.EnvFrom) != 0 {
		t.Errorf("expected no EnvFrom, got %+v", rjContainer.EnvFrom)
	}

	// volumes
	expectVolumeMounts(t, rjContainer, []v1.VolumeMount{{Name: "tmp", MountPath: "/tmp"}})
	expectVolumes(t, rj, []v1.Volume{{Name: "tmp"}})

	expectServiceAccountSettings(t, rj, "", ptr.To(false))
	expectSecurityContext(t, rj, rjContainer, defaultPodSecurityContext, defaultContainerSecurityContext)
	expectImagePullSecrets(t, rj, nil)

	// scheduling
	expectAffinity(t, rj, nil)
	expectNodeSelector(t, rj, nil)
	expectTolerations(t, rj, nil)
	expectTopologySpreadConstraints(t, rj, nil)
}

// ##### HELPERS #####
func expectContainer(t *testing.T, job *batchv1.Job) *v1.Container {
	containers := job.Spec.Template.Spec.Containers
	if len(containers) != 1 {
		t.Fatalf("expected exactly one container in job")
	}
	return &containers[0]
}

func expectVolumeMounts(t *testing.T, container *v1.Container, expectedMounts []v1.VolumeMount) {
	if len(container.VolumeMounts) != len(expectedMounts) {
		t.Fatalf("expected %d volume mounts, got %d", len(expectedMounts), len(container.VolumeMounts))
	}
	for _, expected := range expectedMounts {
		found := false
		for _, actual := range container.VolumeMounts {
			if actual.Name == expected.Name && actual.MountPath == expected.MountPath {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected volume mount %s at %s not found", expected.Name, expected.MountPath)
		}
	}
}
func expectVolumes(t *testing.T, job *batchv1.Job, expectedVolumes []v1.Volume) {
	if len(job.Spec.Template.Spec.Volumes) != len(expectedVolumes) {
		t.Fatalf("expected %d volumes, got %d", len(expectedVolumes), len(job.Spec.Template.Spec.Volumes))
	}
	for _, expected := range expectedVolumes {
		found := false
		for _, actual := range job.Spec.Template.Spec.Volumes {
			if actual.Name == expected.Name {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected volume %s not found", expected.Name)
		}
	}
}

func expectJobName(t *testing.T, job *batchv1.Job, expectedName string) {
	if job.Name != expectedName {
		t.Fatalf("expected job name %s, got %s", expectedName, job.Name)
	}
}

func expectJobNamespace(t *testing.T, job *batchv1.Job, expectedNamespace string) {
	if job.Namespace != expectedNamespace {
		t.Fatalf("expected job namespace %s, got %s", expectedNamespace, job.Namespace)
	}
}

func expectEnvFromSecret(t *testing.T, container *v1.Container, expectedSecret string) {
	envFrom := container.EnvFrom
	if len(envFrom) == 0 || envFrom[0].SecretRef == nil || envFrom[0].SecretRef.Name != expectedSecret {
		t.Fatalf("expected envFrom SecretRef %s, got %+v", expectedSecret, envFrom)
	}
}

func expectLabels(t *testing.T, job *batchv1.Job, expectedLabels map[string]string, jobType, jobName string) {
	for k, v := range expectedLabels {
		if job.Spec.Template.Labels[k] != v {
			t.Fatalf("expected template label %s=%s, got %s", k, v, job.Spec.Template.Labels[k])
		}
	}
	defaultLabels := map[string]string{
		"renovate-operator.mogenius.com/job-type": jobType,
		"renovate-operator.mogenius.com/job-name": jobName,
	}
	for k, v := range defaultLabels {
		if job.Spec.Template.Labels[k] != v {
			t.Fatalf("expected default template label %s=%s, got %s", k, v, job.Spec.Template.Labels[k])
		}
	}
}

func expectImage(t *testing.T, container *v1.Container, expectedImage string) {
	if container.Image != expectedImage {
		t.Fatalf("expected image %s, got %s", expectedImage, container.Image)
	}
}

func expectEnvVar(t *testing.T, container *v1.Container, name, expectedValue string) {
	for _, env := range container.Env {
		if env.Name == name {
			if env.Value != expectedValue {
				t.Fatalf("expected env var %s=%s, got %s", name, expectedValue, env.Value)
			}
			return
		}
	}
	t.Fatalf("expected env var %s not found", name)
}

func expectRestartPolicy(t *testing.T, job *batchv1.Job, expectedPolicy v1.RestartPolicy) {
	if job.Spec.Template.Spec.RestartPolicy != expectedPolicy {
		t.Fatalf("expected restart policy %s, got %s", expectedPolicy, job.Spec.Template.Spec.RestartPolicy)
	}
}

func expectActiveDeadlineSeconds(t *testing.T, job *batchv1.Job, expectedSeconds int64) {
	if job.Spec.ActiveDeadlineSeconds == nil || *job.Spec.ActiveDeadlineSeconds != expectedSeconds {
		t.Fatalf("expected active deadline seconds %d, got %v", expectedSeconds, job.Spec.ActiveDeadlineSeconds)
	}
}

func expectTtlSecondsAfterFinished(t *testing.T, job *batchv1.Job, expectedSeconds *int32) {
	if job.Spec.TTLSecondsAfterFinished != nil && expectedSeconds == nil {
		t.Fatalf("expected no TTL seconds after finished %d, got %v", expectedSeconds, job.Spec.TTLSecondsAfterFinished)
	}
	if job.Spec.TTLSecondsAfterFinished == nil && expectedSeconds != nil {
		t.Fatalf("expected TTL seconds after finished %d, got nil", *expectedSeconds)
	}
	if job.Spec.TTLSecondsAfterFinished != nil && expectedSeconds != nil && *job.Spec.TTLSecondsAfterFinished != *expectedSeconds {
		t.Fatalf("expected TTL seconds after finished %d, got %d", *expectedSeconds, *job.Spec.TTLSecondsAfterFinished)
	}
}

func expectServiceAccountSettings(t *testing.T, job *batchv1.Job, expectedName string, expectedAutoMount *bool) {
	if job.Spec.Template.Spec.ServiceAccountName != expectedName {
		t.Fatalf("expected service account name %s, got %s", expectedName, job.Spec.Template.Spec.ServiceAccountName)
	}
	if job.Spec.Template.Spec.AutomountServiceAccountToken == nil && expectedAutoMount != nil {
		t.Fatalf("expected automount service account token %v, got nil", *expectedAutoMount)
	}
	if job.Spec.Template.Spec.AutomountServiceAccountToken != nil && expectedAutoMount == nil {
		t.Fatalf("expected automount service account token nil, got %v", *job.Spec.Template.Spec.AutomountServiceAccountToken)
	}
	if job.Spec.Template.Spec.AutomountServiceAccountToken != nil && expectedAutoMount != nil && *job.Spec.Template.Spec.AutomountServiceAccountToken != *expectedAutoMount {
		t.Fatalf("expected automount service account token %v, got %v", *expectedAutoMount, *job.Spec.Template.Spec.AutomountServiceAccountToken)
	}
}

func expectNodeSelector(t *testing.T, job *batchv1.Job, expectedSelector map[string]string) {
	if len(job.Spec.Template.Spec.NodeSelector) != len(expectedSelector) {
		t.Fatalf("expected node selector %v, got %v", expectedSelector, job.Spec.Template.Spec.NodeSelector)
	}
	for k, v := range expectedSelector {
		if job.Spec.Template.Spec.NodeSelector[k] != v {
			t.Fatalf("expected node selector %s=%s, got %s", k, v, job.Spec.Template.Spec.NodeSelector[k])
		}
	}
}

func expectImagePullSecrets(t *testing.T, job *batchv1.Job, expectedSecrets []v1.LocalObjectReference) {
	if len(job.Spec.Template.Spec.ImagePullSecrets) != len(expectedSecrets) {
		t.Fatalf("expected image pull secrets %v, got %v", expectedSecrets, job.Spec.Template.Spec.ImagePullSecrets)
	}
	for i, sec := range expectedSecrets {
		if job.Spec.Template.Spec.ImagePullSecrets[i].Name != sec.Name {
			t.Fatalf("expected image pull secret %s, got %s", sec.Name, job.Spec.Template.Spec.ImagePullSecrets[i].Name)
		}
	}
}

func expectSecurityContext(t *testing.T, job *batchv1.Job, container *v1.Container, expectedPodCtx *v1.PodSecurityContext, expectedContCtx *v1.SecurityContext) {
	t.Helper()

	podCtx := job.Spec.Template.Spec.SecurityContext
	if !reflect.DeepEqual(podCtx, expectedPodCtx) {
		t.Fatalf("pod security context mismatch:\nexpected: %+v\ngot:      %+v", expectedPodCtx, podCtx)
	}

	contCtx := container.SecurityContext
	if !reflect.DeepEqual(contCtx, expectedContCtx) {
		t.Fatalf("container security context mismatch:\nexpected: %+v\ngot:      %+v", expectedContCtx, contCtx)
	}
}

func expectAffinity(t *testing.T, job *batchv1.Job, expectedAffinity *v1.Affinity) {
	if !reflect.DeepEqual(job.Spec.Template.Spec.Affinity, expectedAffinity) {
		t.Fatalf("affinity mismatch:\nexpected: %+v\ngot:      %+v", expectedAffinity, job.Spec.Template.Spec.Affinity)
	}
}

func expectTolerations(t *testing.T, job *batchv1.Job, expectedTolerations []v1.Toleration) {
	if !reflect.DeepEqual(job.Spec.Template.Spec.Tolerations, expectedTolerations) {
		t.Fatalf("tolerations mismatch:\nexpected: %+v\ngot:      %+v", expectedTolerations, job.Spec.Template.Spec.Tolerations)
	}
}

func expectTopologySpreadConstraints(t *testing.T, job *batchv1.Job, expectedConstraints []v1.TopologySpreadConstraint) {
	if !reflect.DeepEqual(job.Spec.Template.Spec.TopologySpreadConstraints, expectedConstraints) {
		t.Fatalf("topology spread constraints mismatch:\nexpected: %+v\ngot:      %+v", expectedConstraints, job.Spec.Template.Spec.TopologySpreadConstraints)
	}
}

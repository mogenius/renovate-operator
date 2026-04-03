package renovate

import (
	"reflect"
	"strings"
	"testing"

	api "renovate-operator/api/v1alpha1"
	"renovate-operator/config"
	crdManager "renovate-operator/internal/crdManager"

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
		Capabilities: &v1.Capabilities{
			Drop: []v1.Capability{"ALL"},
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

func TestGetDNSPolicy(t *testing.T) {
	t.Run("returns DNSClusterFirst when DNSPolicy is empty", func(t *testing.T) {
		spec := api.RenovateJobSpec{}
		if got := getDNSPolicy(spec); got != v1.DNSClusterFirst {
			t.Fatalf("expected %s, got %s", v1.DNSClusterFirst, got)
		}
	})

	t.Run("returns spec DNSPolicy when set", func(t *testing.T) {
		spec := api.RenovateJobSpec{DNSPolicy: v1.DNSClusterFirst}
		if got := getDNSPolicy(spec); got != v1.DNSClusterFirst {
			t.Fatalf("expected %s, got %s", v1.DNSClusterFirst, got)
		}
	})
}

func TestNewJobs_CustomRenovateBaseDir_ViaExtraVolumes(t *testing.T) {
	job := &api.RenovateJob{
		ObjectMeta: metav1.ObjectMeta{Name: "rj", Namespace: "ns"},
		Spec: api.RenovateJobSpec{
			Image: "img",
			ExtraEnv: []v1.EnvVar{
				{Name: "RENOVATE_BASE_DIR", Value: "/var/renovate"},
			},
			ExtraVolumes: []v1.Volume{
				{Name: "work", VolumeSource: v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}}},
			},
			ExtraVolumeMounts: []v1.VolumeMount{
				{Name: "work", MountPath: "/var/renovate"},
			},
		},
	}
	err := config.InitializeConfigModule([]config.ConfigItemDescription{{Key: "JOB_TIMEOUT_SECONDS", Optional: true, Default: "10"}})
	if err != nil {
		t.Fatalf("config init: %v", err)
	}

	discoveryBatchJob := newDiscoveryJob(job)
	discoveryContainer := expectContainer(t, discoveryBatchJob)
	expectEnvVar(t, discoveryContainer, "RENOVATE_BASE_DIR", "/var/renovate")
	expectVolumeMounts(t, discoveryContainer, []v1.VolumeMount{{Name: "work", MountPath: "/var/renovate"}})
	if !strings.Contains(discoveryContainer.Args[0], `"$RENOVATE_BASE_DIR/repos.json"`) {
		t.Fatalf("discovery args should use RENOVATE_BASE_DIR for repos path, got %q", discoveryContainer.Args[0])
	}

	renovateBatchJob := newRenovateJob(job, "proj")
	renovateContainer := expectContainer(t, renovateBatchJob)
	expectEnvVar(t, renovateContainer, "RENOVATE_BASE_DIR", "/var/renovate")
	expectVolumeMounts(t, renovateContainer, []v1.VolumeMount{{Name: "work", MountPath: "/var/renovate"}})
}

func TestNewJobs_ExtraEnv_RenovateBaseDirOverridesDefault(t *testing.T) {
	job := &api.RenovateJob{
		ObjectMeta: metav1.ObjectMeta{Name: "rj", Namespace: "ns"},
		Spec: api.RenovateJobSpec{
			Image: "img",
			ExtraEnv: []v1.EnvVar{
				{Name: "RENOVATE_BASE_DIR", Value: "/from-extra"},
				{Name: "OTHER", Value: "x"},
			},
		},
	}
	err := config.InitializeConfigModule([]config.ConfigItemDescription{{Key: "JOB_TIMEOUT_SECONDS", Optional: true, Default: "10"}})
	if err != nil {
		t.Fatalf("config init: %v", err)
	}

	discoveryContainer := expectContainer(t, newDiscoveryJob(job))
	expectEnvVar(t, discoveryContainer, "RENOVATE_BASE_DIR", "/from-extra")
	expectEnvVar(t, discoveryContainer, "OTHER", "x")
}

func TestNewJobs_ContainerResources(t *testing.T) {
	job := &api.RenovateJob{
		ObjectMeta: metav1.ObjectMeta{Name: "rj", Namespace: "ns"},
		Spec: api.RenovateJobSpec{
			Image: "img",
			Resources: v1.ResourceRequirements{
				Requests: v1.ResourceList{
					v1.ResourceEphemeralStorage: resource.MustParse("1Gi"),
				},
				Limits: v1.ResourceList{
					v1.ResourceCPU:              resource.MustParse("500m"),
					v1.ResourceMemory:           resource.MustParse("256Mi"),
					v1.ResourceEphemeralStorage: resource.MustParse("3Gi"),
				},
			},
		},
	}
	err := config.InitializeConfigModule([]config.ConfigItemDescription{{Key: "JOB_TIMEOUT_SECONDS", Optional: true, Default: "10"}})
	if err != nil {
		t.Fatalf("config init: %v", err)
	}

	for _, testCase := range []struct {
		name     string
		batchJob *batchv1.Job
	}{
		{"discovery", newDiscoveryJob(job)},
		{"renovate", newRenovateJob(job, "proj")},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			container := expectContainer(t, testCase.batchJob)
			ephemReq := container.Resources.Requests[v1.ResourceEphemeralStorage]
			if ephemReq.Cmp(resource.MustParse("1Gi")) != 0 {
				t.Fatalf("expected ephemeral-storage request 1Gi, got %v", ephemReq)
			}
			ephemLim := container.Resources.Limits[v1.ResourceEphemeralStorage]
			if ephemLim.Cmp(resource.MustParse("3Gi")) != 0 {
				t.Fatalf("expected ephemeral-storage limit 3Gi, got %v", ephemLim)
			}
			cpuLim := container.Resources.Limits[v1.ResourceCPU]
			if cpuLim.Cmp(resource.MustParse("500m")) != 0 {
				t.Fatalf("expected CPU limit preserved from spec.resources")
			}
		})
	}
}

func TestNewJobs_WithSettings(t *testing.T) {
	job := &api.RenovateJob{
		ObjectMeta: metav1.ObjectMeta{Name: "rj", Namespace: "ns"},
		Spec: api.RenovateJobSpec{
			Image:     "img",
			SecretRef: "sref",
			Provider: &api.RenovateProvider{
				Name:     "gitlab",
				Endpoint: "gitlab.example.com",
			},
			DiscoveryFilters: []string{"org1/*", "org2/repo1"},
			DiscoverTopics:   []string{"renovate", "!skipRenovate"},
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
			ExtraEnv: []v1.EnvVar{
				{
					Name:  "LOG_FORMAT",
					Value: "console",
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
		Status: api.RenovateJobStatus{
			ExecutionOptions: &api.RenovateExecutionOptions{
				Debug: true,
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
	discoveryBatchJob := newDiscoveryJob(job)
	discoveryContainer := expectContainer(t, discoveryBatchJob)
	// basic fields
	expectJobName(t, discoveryBatchJob, "rj-discovery-6987b484")
	expectJobNamespace(t, discoveryBatchJob, "ns")
	expectLabels(t, discoveryBatchJob, map[string]string{"a": "b"}, string(crdManager.DiscoveryJobType), "rj-discovery-6987b484")
	expectImage(t, discoveryContainer, "img")
	expectRestartPolicy(t, discoveryBatchJob, v1.RestartPolicyNever)
	expectActiveDeadlineSeconds(t, discoveryBatchJob, 10)
	expectTtlSecondsAfterFinished(t, discoveryBatchJob, ptr.To(int32(360)))

	// env vars
	expectEnvVar(t, discoveryContainer, "LOG_FORMAT", "console")
	expectEnvVar(t, discoveryContainer, "RENOVATE_AUTODISCOVER_FILTER", "org1/*,org2/repo1")
	expectEnvVar(t, discoveryContainer, "RENOVATE_AUTODISCOVER_TOPICS", "renovate,!skipRenovate")
	expectEnvVar(t, discoveryContainer, "RENOVATE_ENDPOINT", "gitlab.example.com")
	expectEnvVar(t, discoveryContainer, "RENOVATE_PLATFORM", "gitlab")
	expectEnvVar(t, discoveryContainer, "LOG_LEVEL", "debug")
	expectEnvVar(t, discoveryContainer, "RENOVATE_BASE_DIR", "/tmp")
	expectEnvFromSecret(t, discoveryContainer, "sref")
	// volumes
	expectVolumeMounts(t, discoveryContainer, []v1.VolumeMount{{Name: "extra-vol", MountPath: "/extra"}})
	expectVolumes(t, discoveryBatchJob, []v1.Volume{{Name: "extra-vol"}})
	// other
	expectServiceAccountSettings(t, discoveryBatchJob, "test", ptr.To(true))
	expectSecurityContext(t, discoveryBatchJob, discoveryContainer, job.Spec.SecurityContext.Pod, job.Spec.SecurityContext.Container)
	expectImagePullSecrets(t, discoveryBatchJob, []v1.LocalObjectReference{{Name: "my-pull-secret"}})
	// scheduling
	expectAffinity(t, discoveryBatchJob, job.Spec.Affinity)
	expectNodeSelector(t, discoveryBatchJob, map[string]string{"disktype": "ssd"})
	expectTolerations(t, discoveryBatchJob, job.Spec.Tolerations)
	expectTopologySpreadConstraints(t, discoveryBatchJob, job.Spec.TopologySpreadConstraints)

	// test renovate job
	renovateBatchJob := newRenovateJob(job, "proj")
	renovateContainer := expectContainer(t, renovateBatchJob)
	// basic fields
	expectJobName(t, renovateBatchJob, "rj-proj-701b9b0a")
	expectJobNamespace(t, renovateBatchJob, "ns")
	expectLabels(t, renovateBatchJob, map[string]string{"a": "b"}, string(crdManager.ExecutorJobType), "rj-proj-701b9b0a")
	expectImage(t, renovateContainer, "img")
	expectRestartPolicy(t, renovateBatchJob, v1.RestartPolicyNever)
	expectActiveDeadlineSeconds(t, renovateBatchJob, 10)
	expectTtlSecondsAfterFinished(t, renovateBatchJob, ptr.To(int32(360)))

	// env vars
	expectEnvVar(t, renovateContainer, "LOG_FORMAT", "console")
	expectEnvFromSecret(t, renovateContainer, "sref")
	expectEnvVar(t, renovateContainer, "RENOVATE_BASE_DIR", "/tmp")
	// volumes
	expectVolumeMounts(t, renovateContainer, []v1.VolumeMount{{Name: "extra-vol", MountPath: "/extra"}})
	expectVolumes(t, renovateBatchJob, []v1.Volume{{Name: "extra-vol"}})
	// other
	expectServiceAccountSettings(t, renovateBatchJob, "test", ptr.To(true))
	expectSecurityContext(t, renovateBatchJob, renovateContainer, job.Spec.SecurityContext.Pod, job.Spec.SecurityContext.Container)
	expectImagePullSecrets(t, renovateBatchJob, []v1.LocalObjectReference{{Name: "my-pull-secret"}})
	// scheduling
	expectAffinity(t, renovateBatchJob, job.Spec.Affinity)
	expectNodeSelector(t, renovateBatchJob, map[string]string{"disktype": "ssd"})
	expectTolerations(t, renovateBatchJob, job.Spec.Tolerations)
	expectTopologySpreadConstraints(t, renovateBatchJob, job.Spec.TopologySpreadConstraints)
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
	discoveryBatchJob := newDiscoveryJob(job)
	discoveryContainer := expectContainer(t, discoveryBatchJob)
	// basic fields
	expectJobName(t, discoveryBatchJob, "nofilter-discovery-3006fe8c")
	expectJobNamespace(t, discoveryBatchJob, "ns")
	expectImage(t, discoveryContainer, "renovate:dev")
	expectTtlSecondsAfterFinished(t, discoveryBatchJob, nil)

	// env vars - only defaults, no optional ones
	expectEnvVar(t, discoveryContainer, "LOG_FORMAT", "json")
	expectEnvVar(t, discoveryContainer, "RENOVATE_BASE_DIR", "/tmp")
	for _, env := range discoveryContainer.Env {
		if env.Name == "RENOVATE_AUTODISCOVER_FILTER" {
			t.Errorf("did not expect RENOVATE_AUTODISCOVER_FILTER env var")
		}

		if env.Name == "RENOVATE_AUTODISCOVER_TOPICS" {
			t.Errorf("did not expect RENOVATE_AUTODISCOVER_TOPICS env var")
		}
	}
	if len(discoveryContainer.EnvFrom) != 0 {
		t.Errorf("expected no EnvFrom, got %+v", discoveryContainer.EnvFrom)
	}

	// volumes
	expectVolumeMounts(t, discoveryContainer, nil)
	expectVolumes(t, discoveryBatchJob, nil)

	expectServiceAccountSettings(t, discoveryBatchJob, "", ptr.To(false))
	expectSecurityContext(t, discoveryBatchJob, discoveryContainer, defaultPodSecurityContext, defaultContainerSecurityContext)
	expectImagePullSecrets(t, discoveryBatchJob, nil)

	// scheduling
	expectAffinity(t, discoveryBatchJob, nil)
	expectNodeSelector(t, discoveryBatchJob, nil)
	expectTolerations(t, discoveryBatchJob, nil)
	expectTopologySpreadConstraints(t, discoveryBatchJob, nil)

	// test renovate job
	renovateBatchJob := newRenovateJob(job, "myproj")
	renovateContainer := expectContainer(t, renovateBatchJob)
	// basic fields
	expectJobName(t, renovateBatchJob, "nofilter-myproj-496e220d")
	expectJobNamespace(t, renovateBatchJob, "ns")
	expectImage(t, renovateContainer, "renovate:dev")
	expectTtlSecondsAfterFinished(t, renovateBatchJob, nil)

	// env vars - only defaults
	expectEnvVar(t, renovateContainer, "LOG_FORMAT", "json")
	expectEnvVar(t, renovateContainer, "RENOVATE_BASE_DIR", "/tmp")
	if len(renovateContainer.EnvFrom) != 0 {
		t.Errorf("expected no EnvFrom, got %+v", renovateContainer.EnvFrom)
	}

	// volumes
	expectVolumeMounts(t, renovateContainer, nil)
	expectVolumes(t, renovateBatchJob, nil)

	expectServiceAccountSettings(t, renovateBatchJob, "", ptr.To(false))
	expectSecurityContext(t, renovateBatchJob, renovateContainer, defaultPodSecurityContext, defaultContainerSecurityContext)
	expectImagePullSecrets(t, renovateBatchJob, nil)

	// scheduling
	expectAffinity(t, renovateBatchJob, nil)
	expectNodeSelector(t, renovateBatchJob, nil)
	expectTolerations(t, renovateBatchJob, nil)
	expectTopologySpreadConstraints(t, renovateBatchJob, nil)
}

func TestNewJobs_WithDefaultImagePullSecrets(t *testing.T) {
	err := config.InitializeConfigModule([]config.ConfigItemDescription{
		{Key: "JOB_TIMEOUT_SECONDS", Optional: true, Default: "10"},
		{Key: "IMAGE_PULL_SECRETS", Optional: true, Default: `[{"name":"default-secret"}]`},
	})
	if err != nil {
		t.Fatalf("expected to initialize config module without error, got %v", err)
	}

	t.Run("default secret applied when spec has none", func(t *testing.T) {
		job := &api.RenovateJob{
			ObjectMeta: metav1.ObjectMeta{Name: "rj", Namespace: "ns"},
			Spec:       api.RenovateJobSpec{Image: "img"},
		}
		discoveryBatchJob := newDiscoveryJob(job)
		expectImagePullSecrets(t, discoveryBatchJob, []v1.LocalObjectReference{{Name: "default-secret"}})

		renovateBatchJob := newRenovateJob(job, "proj")
		expectImagePullSecrets(t, renovateBatchJob, []v1.LocalObjectReference{{Name: "default-secret"}})
	})

	t.Run("spec and default secrets are combined", func(t *testing.T) {
		job := &api.RenovateJob{
			ObjectMeta: metav1.ObjectMeta{Name: "rj", Namespace: "ns"},
			Spec: api.RenovateJobSpec{
				Image:            "img",
				ImagePullSecrets: []v1.LocalObjectReference{{Name: "spec-secret"}},
			},
		}
		discoveryBatchJob := newDiscoveryJob(job)
		expectImagePullSecrets(t, discoveryBatchJob, []v1.LocalObjectReference{{Name: "spec-secret"}, {Name: "default-secret"}})

		renovateBatchJob := newRenovateJob(job, "proj")
		expectImagePullSecrets(t, renovateBatchJob, []v1.LocalObjectReference{{Name: "spec-secret"}, {Name: "default-secret"}})
	})
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
	if job.GenerateName != expectedName {
		t.Fatalf("expected job generate name %s, got %s", expectedName, job.GenerateName)
	}
	if job.Name != "" {
		t.Fatalf("expected job name to be empty, got %s", job.Name)
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
		if job.Labels[k] != v {
			t.Fatalf("expected job label %s=%s, got %s", k, v, job.Labels[k])
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
		if job.Labels[k] != v {
			t.Fatalf("expected default job label %s=%s, got %s", k, v, job.Labels[k])
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

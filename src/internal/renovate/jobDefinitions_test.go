package renovate

import (
	"reflect"
	"strings"
	"testing"

	api "renovate-operator/api/v1alpha1"
	"renovate-operator/config"
	crdManager "renovate-operator/internal/crdManager"

	"go.opentelemetry.io/otel/propagation"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	defaultPodSecurityContext = &v1.PodSecurityContext{
		RunAsUser:    new(int64(12021)),
		RunAsGroup:   new(int64(12021)),
		FSGroup:      new(int64(12021)),
		RunAsNonRoot: new(true),
		SeccompProfile: &v1.SeccompProfile{
			Type: v1.SeccompProfileTypeRuntimeDefault,
		},
	}

	defaultContainerSecurityContext = &v1.SecurityContext{
		RunAsUser:                new(int64(12021)),
		RunAsGroup:               new(int64(12021)),
		RunAsNonRoot:             new(true),
		ReadOnlyRootFilesystem:   new(false),
		Privileged:               new(false),
		AllowPrivilegeEscalation: new(false),
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
					Name:  "RENOVATE_LOG_FORMAT",
					Value: "console",
				},
			},
			ServiceAccount: &api.RenovateJobServiceAccount{
				AutomountServiceAccountToken: new(true),
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
			PriorityClassName: "renovate-low-priority",
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
					RunAsUser: new(int64(15000)),
				},
				Container: &v1.SecurityContext{
					RunAsUser: new(int64(16000)),
				},
			},
		},
	}
	err := config.InitializeConfigModule([]config.ConfigItemDescription{
		{Key: "JOB_TIMEOUT_SECONDS", Optional: true, Default: "10"},
		{Key: "JOB_TTL_SECONDS_AFTER_FINISHED", Optional: true, Default: "360"},
		{Key: "VALKEY_URL", Optional: true, Default: "redis://redis.svc.cluster.local:6379/0"},
		{Key: "VALKEY_HOST", Optional: true, Default: ""},
		{Key: "VALKEY_PORT", Optional: true, Default: "6379"},
		{Key: "VALKEY_PASSWORD", Optional: true, Default: ""},
		{Key: "VALKEY_FORWARD_CACHE_TO_JOBS", Optional: true, Default: "true"},
	})
	if err != nil {
		t.Fatalf("expected to initialize config module without error, got %v", err)
	}

	// test discovery job
	dj := newDiscoveryJob(job, nil)
	djContainer := expectContainer(t, dj)
	// basic fields
	expectJobName(t, dj, "rj-discovery-6987b484")
	expectJobNamespace(t, dj, "ns")
	expectLabels(t, dj, map[string]string{"a": "b"}, string(crdManager.DiscoveryJobType), "rj-discovery-6987b484")
	expectImage(t, djContainer, "img")
	expectRestartPolicy(t, dj, v1.RestartPolicyNever)
	expectActiveDeadlineSeconds(t, dj, 10)
	expectTtlSecondsAfterFinished(t, dj, new(int32(360)))

	// env vars
	expectEnvVar(t, djContainer, "RENOVATE_LOG_FORMAT", "console")
	expectEnvVar(t, djContainer, "RENOVATE_AUTODISCOVER_FILTER", "org1/*,org2/repo1")
	expectEnvVar(t, djContainer, "RENOVATE_AUTODISCOVER_TOPICS", "renovate,!skipRenovate")
	expectEnvVar(t, djContainer, "RENOVATE_ENDPOINT", "gitlab.example.com")
	expectEnvVar(t, djContainer, "RENOVATE_PLATFORM", "gitlab")
	expectEnvFromSecret(t, djContainer, "sref")
	expectEnvVarFromSecretKey(t, djContainer, "RENOVATE_REDIS_URL", redisURLSecretName, "redis-url")

	// volumes
	expectVolumeMounts(t, djContainer, []v1.VolumeMount{{Name: "tmp", MountPath: "/tmp"}, {Name: "extra-vol", MountPath: "/extra"}})
	expectVolumes(t, dj, []v1.Volume{{Name: "tmp"}, {Name: "extra-vol"}})
	// other
	expectServiceAccountSettings(t, dj, "test", new(true))
	// The fixture overrides runAsUser only, so the expectation is the merge: the spec's
	// uid wins and every other hardened default is still in place.
	expectSecurityContext(t, dj, djContainer, mergedPodSecurityContext(15000), mergedContainerSecurityContext(16000))
	expectImagePullSecrets(t, dj, []v1.LocalObjectReference{{Name: "my-pull-secret"}})
	// scheduling
	expectAffinity(t, dj, job.Spec.Affinity)
	expectNodeSelector(t, dj, map[string]string{"disktype": "ssd"})
	expectTolerations(t, dj, job.Spec.Tolerations)
	expectTopologySpreadConstraints(t, dj, job.Spec.TopologySpreadConstraints)
	expectPriorityClassName(t, dj, "renovate-low-priority")

	// test renovate job
	rj := newRenovateJob(job, "proj", &api.RenovateExecutionOptions{Debug: true}, nil)
	rjContainer := expectContainer(t, rj)
	// basic fields
	expectJobName(t, rj, "rj-proj-701b9b0a")
	expectJobNamespace(t, rj, "ns")
	expectLabels(t, rj, map[string]string{"a": "b"}, string(crdManager.ExecutorJobType), "rj-proj-701b9b0a")
	expectImage(t, rjContainer, "img")
	expectRestartPolicy(t, rj, v1.RestartPolicyNever)
	expectActiveDeadlineSeconds(t, rj, 10)
	expectTtlSecondsAfterFinished(t, rj, new(int32(360)))

	// env vars
	expectEnvVar(t, rjContainer, "RENOVATE_LOG_FORMAT", "console")
	expectEnvVar(t, rjContainer, "RENOVATE_LOG_LEVEL", "debug")
	expectEnvVarFromSecretKey(t, rjContainer, "RENOVATE_REDIS_URL", redisURLSecretName, "redis-url")
	expectEnvFromSecret(t, rjContainer, "sref")
	// volumes
	expectVolumeMounts(t, rjContainer, []v1.VolumeMount{{Name: "tmp", MountPath: "/tmp"}, {Name: "extra-vol", MountPath: "/extra"}})
	expectVolumes(t, rj, []v1.Volume{{Name: "tmp"}, {Name: "extra-vol"}})
	// other
	expectServiceAccountSettings(t, rj, "test", new(true))
	expectSecurityContext(t, rj, rjContainer, mergedPodSecurityContext(15000), mergedContainerSecurityContext(16000))
	expectImagePullSecrets(t, rj, []v1.LocalObjectReference{{Name: "my-pull-secret"}})
	// scheduling
	expectAffinity(t, rj, job.Spec.Affinity)
	expectNodeSelector(t, rj, map[string]string{"disktype": "ssd"})
	expectTolerations(t, rj, job.Spec.Tolerations)
	expectTopologySpreadConstraints(t, rj, job.Spec.TopologySpreadConstraints)
	expectPriorityClassName(t, rj, "renovate-low-priority")
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
	dj := newDiscoveryJob(job, nil)
	djContainer := expectContainer(t, dj)
	// basic fields
	expectJobName(t, dj, "nofilter-discovery-3006fe8c")
	expectJobNamespace(t, dj, "ns")
	expectImage(t, djContainer, "renovate:dev")
	expectTtlSecondsAfterFinished(t, dj, nil)

	// env vars - only defaults, no optional ones
	expectEnvVar(t, djContainer, "RENOVATE_LOG_FORMAT", "json")
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

	expectServiceAccountSettings(t, dj, "", new(false))
	expectSecurityContext(t, dj, djContainer, defaultPodSecurityContext, defaultContainerSecurityContext)
	expectImagePullSecrets(t, dj, nil)

	// scheduling
	expectAffinity(t, dj, nil)
	expectNodeSelector(t, dj, nil)
	expectTolerations(t, dj, nil)
	expectTopologySpreadConstraints(t, dj, nil)
	expectPriorityClassName(t, dj, "")

	// test renovate job
	rj := newRenovateJob(job, "myproj", nil, nil)
	rjContainer := expectContainer(t, rj)
	// basic fields
	expectJobName(t, rj, "nofilter-myproj-496e220d")
	expectJobNamespace(t, rj, "ns")
	expectImage(t, rjContainer, "renovate:dev")
	expectTtlSecondsAfterFinished(t, rj, nil)

	// env vars - only defaults
	expectEnvVar(t, rjContainer, "RENOVATE_LOG_FORMAT", "json")
	if len(rjContainer.EnvFrom) != 0 {
		t.Errorf("expected no EnvFrom, got %+v", rjContainer.EnvFrom)
	}

	// volumes
	expectVolumeMounts(t, rjContainer, []v1.VolumeMount{{Name: "tmp", MountPath: "/tmp"}})
	expectVolumes(t, rj, []v1.Volume{{Name: "tmp"}})

	expectServiceAccountSettings(t, rj, "", new(false))
	expectSecurityContext(t, rj, rjContainer, defaultPodSecurityContext, defaultContainerSecurityContext)
	expectImagePullSecrets(t, rj, nil)

	// scheduling
	expectAffinity(t, rj, nil)
	expectNodeSelector(t, rj, nil)
	expectTolerations(t, rj, nil)
	expectTopologySpreadConstraints(t, rj, nil)
	expectPriorityClassName(t, rj, "")
}

func TestNewJobs_Autodiscovery(t *testing.T) {
	_ = config.InitializeConfigModule([]config.ConfigItemDescription{
		{Key: "JOB_TIMEOUT_SECONDS", Optional: true, Default: "10"},
	})

	t.Run("executor disables autodiscovery without mutating extra env", func(t *testing.T) {
		extraEnv := []v1.EnvVar{
			{Name: "RENOVATE_AUTODISCOVER", Value: "true"},
			{Name: "RENOVATE_REQUIRE_CONFIG", Value: "required"},
		}
		job := &api.RenovateJob{
			ObjectMeta: metav1.ObjectMeta{Name: "rj", Namespace: "ns"},
			Spec: api.RenovateJobSpec{
				Image:    "img",
				ExtraEnv: extraEnv,
			},
		}

		djContainer := expectContainer(t, newDiscoveryJob(job, nil))
		rjContainer := expectContainer(t, newRenovateJob(job, "org/configured-repository", nil, nil))

		if !reflect.DeepEqual(djContainer.Command, []string{"/bin/sh", "-c"}) {
			t.Fatalf("expected discovery command to use the shell, got %v", djContainer.Command)
		}
		if len(djContainer.Args) != 1 || !strings.Contains(djContainer.Args[0], "renovate --autodiscover ") {
			t.Fatalf("expected discovery job to run Renovate with autodiscovery, got %v", djContainer.Args)
		}
		if !reflect.DeepEqual(rjContainer.Command, []string{"renovate"}) {
			t.Fatalf("expected executor command to run Renovate, got %v", rjContainer.Command)
		}
		expectedArgs := []string{"--autodiscover=false", "org/configured-repository"}
		if !reflect.DeepEqual(rjContainer.Args, expectedArgs) {
			t.Fatalf("expected executor args %v, got %v", expectedArgs, rjContainer.Args)
		}

		expectEnvVar(t, djContainer, "RENOVATE_AUTODISCOVER", "true")
		expectEnvVar(t, rjContainer, "RENOVATE_AUTODISCOVER", "true")
		expectEnvVar(t, rjContainer, "RENOVATE_REQUIRE_CONFIG", "required")
		if !reflect.DeepEqual(job.Spec.ExtraEnv, extraEnv) {
			t.Fatalf("expected extra env to remain unchanged, got %v", job.Spec.ExtraEnv)
		}
	})

	t.Run("executor disables autodiscovery when extra env omits it", func(t *testing.T) {
		job := &api.RenovateJob{
			ObjectMeta: metav1.ObjectMeta{Name: "rj", Namespace: "ns"},
			Spec:       api.RenovateJobSpec{Image: "img"},
		}

		container := expectContainer(t, newRenovateJob(job, "org/repository", nil, nil))
		expectedArgs := []string{"--autodiscover=false", "org/repository"}
		if !reflect.DeepEqual(container.Args, expectedArgs) {
			t.Fatalf("expected executor args %v, got %v", expectedArgs, container.Args)
		}
	})
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
		dj := newDiscoveryJob(job, nil)
		expectImagePullSecrets(t, dj, []v1.LocalObjectReference{{Name: "default-secret"}})

		rj := newRenovateJob(job, "proj", nil, nil)
		expectImagePullSecrets(t, rj, []v1.LocalObjectReference{{Name: "default-secret"}})
	})

	t.Run("spec and default secrets are combined", func(t *testing.T) {
		job := &api.RenovateJob{
			ObjectMeta: metav1.ObjectMeta{Name: "rj", Namespace: "ns"},
			Spec: api.RenovateJobSpec{
				Image:            "img",
				ImagePullSecrets: []v1.LocalObjectReference{{Name: "spec-secret"}},
			},
		}
		dj := newDiscoveryJob(job, nil)
		expectImagePullSecrets(t, dj, []v1.LocalObjectReference{{Name: "spec-secret"}, {Name: "default-secret"}})

		rj := newRenovateJob(job, "proj", nil, nil)
		expectImagePullSecrets(t, rj, []v1.LocalObjectReference{{Name: "spec-secret"}, {Name: "default-secret"}})
	})
}

func TestScratchVolume(t *testing.T) {
	_ = config.InitializeConfigModule([]config.ConfigItemDescription{
		{Key: "JOB_TIMEOUT_SECONDS", Optional: true, Default: "10"},
	})

	baseJob := func(sv *api.RenovateJobScratchVolume) *api.RenovateJob {
		return &api.RenovateJob{
			ObjectMeta: metav1.ObjectMeta{Name: "rj", Namespace: "ns"},
			Spec:       api.RenovateJobSpec{Image: "img", ScratchVolume: sv},
		}
	}

	t.Run("nil scratchVolume creates default emptyDir at /tmp", func(t *testing.T) {
		job := baseJob(nil)
		for _, bj := range []*batchv1.Job{newDiscoveryJob(job, nil), newRenovateJob(job, "proj", nil, nil)} {
			c := expectContainer(t, bj)
			expectVolumes(t, bj, []v1.Volume{{Name: "tmp"}})
			expectVolumeMounts(t, c, []v1.VolumeMount{{Name: "tmp", MountPath: "/tmp"}})
			expectEnvVar(t, c, "RENOVATE_BASE_DIR", "/tmp")
			vol := bj.Spec.Template.Spec.Volumes[0]
			if vol.EmptyDir == nil {
				t.Fatalf("expected emptyDir volume source")
			}
		}
	})

	t.Run("enabled=true explicitly creates scratch volume", func(t *testing.T) {
		job := baseJob(&api.RenovateJobScratchVolume{Enabled: true})
		for _, bj := range []*batchv1.Job{newDiscoveryJob(job, nil), newRenovateJob(job, "proj", nil, nil)} {
			c := expectContainer(t, bj)
			expectVolumes(t, bj, []v1.Volume{{Name: "tmp"}})
			expectVolumeMounts(t, c, []v1.VolumeMount{{Name: "tmp", MountPath: "/tmp"}})
			expectEnvVar(t, c, "RENOVATE_BASE_DIR", "/tmp")
		}
	})

	t.Run("enabled=false disables scratch volume and RENOVATE_BASE_DIR", func(t *testing.T) {
		job := baseJob(&api.RenovateJobScratchVolume{Enabled: false})
		for _, bj := range []*batchv1.Job{newDiscoveryJob(job, nil), newRenovateJob(job, "proj", nil, nil)} {
			c := expectContainer(t, bj)
			if len(bj.Spec.Template.Spec.Volumes) != 0 {
				t.Fatalf("expected no volumes, got %v", bj.Spec.Template.Spec.Volumes)
			}
			if len(c.VolumeMounts) != 0 {
				t.Fatalf("expected no volume mounts, got %v", c.VolumeMounts)
			}
			for _, env := range c.Env {
				if env.Name == "RENOVATE_BASE_DIR" {
					t.Fatalf("expected no RENOVATE_BASE_DIR env var when scratch disabled")
				}
			}
		}
	})

	t.Run("custom path sets mount and RENOVATE_BASE_DIR", func(t *testing.T) {
		job := baseJob(&api.RenovateJobScratchVolume{Enabled: true, Path: "/workspace"})
		for _, bj := range []*batchv1.Job{newDiscoveryJob(job, nil), newRenovateJob(job, "proj", nil, nil)} {
			c := expectContainer(t, bj)
			expectVolumeMounts(t, c, []v1.VolumeMount{{Name: "tmp", MountPath: "/workspace"}})
			expectEnvVar(t, c, "RENOVATE_BASE_DIR", "/workspace")
		}
	})

	t.Run("memory medium and sizeLimit applied to emptyDir", func(t *testing.T) {
		sl := resource.MustParse("512Mi")
		job := baseJob(&api.RenovateJobScratchVolume{
			Enabled:   true,
			Medium:    v1.StorageMediumMemory,
			SizeLimit: &sl,
		})
		for _, bj := range []*batchv1.Job{newDiscoveryJob(job, nil), newRenovateJob(job, "proj", nil, nil)} {
			vol := bj.Spec.Template.Spec.Volumes[0]
			if vol.EmptyDir == nil {
				t.Fatalf("expected emptyDir volume source")
			}
			if vol.EmptyDir.Medium != v1.StorageMediumMemory {
				t.Fatalf("expected Memory medium, got %s", vol.EmptyDir.Medium)
			}
			if vol.EmptyDir.SizeLimit == nil || vol.EmptyDir.SizeLimit.Cmp(sl) != 0 {
				t.Fatalf("expected sizeLimit 512Mi, got %v", vol.EmptyDir.SizeLimit)
			}
		}
	})

	t.Run("ephemeral volume source used when set", func(t *testing.T) {
		storageClass := "fast"
		job := baseJob(&api.RenovateJobScratchVolume{
			Enabled: true,
			Ephemeral: &v1.EphemeralVolumeSource{
				VolumeClaimTemplate: &v1.PersistentVolumeClaimTemplate{
					Spec: v1.PersistentVolumeClaimSpec{
						StorageClassName: &storageClass,
					},
				},
			},
		})
		for _, bj := range []*batchv1.Job{newDiscoveryJob(job, nil), newRenovateJob(job, "proj", nil, nil)} {
			vol := bj.Spec.Template.Spec.Volumes[0]
			if vol.Ephemeral == nil {
				t.Fatalf("expected ephemeral volume source")
			}
			if vol.EmptyDir != nil {
				t.Fatalf("expected no emptyDir when ephemeral is set")
			}
		}
	})
}

func TestOtelEnvVarsForJobs(t *testing.T) {
	otelConfigKeys := []config.ConfigItemDescription{
		{Key: "RENOVATE_FORWARD_OTEL", Optional: true, Default: "false"},
		{Key: "RENOVATE_JOB_OTEL_ENDPOINT", Optional: true, Default: ""},
		{Key: "OTEL_EXPORTER_OTLP_ENDPOINT", Optional: true, Default: ""},
		{Key: "OTEL_SERVICE_NAMESPACE", Optional: true, Default: ""},
	}

	t.Run("returns OTEL vars when forwarding enabled with endpoint", func(t *testing.T) {
		t.Setenv("RENOVATE_FORWARD_OTEL", "true")
		t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://collector:4318")
		_ = config.InitializeConfigModule(otelConfigKeys)

		envs := otelEnvVarsForJobs()

		var hasEndpoint bool
		for _, env := range envs {
			if env.Name == "OTEL_EXPORTER_OTLP_ENDPOINT" {
				hasEndpoint = true
			}
			if env.Name == "OTEL_EXPORTER_OTLP_PROTOCOL" {
				t.Fatal("OTEL_EXPORTER_OTLP_PROTOCOL should not be forwarded to Renovate jobs")
			}
			if env.Name == "TRACEPARENT" {
				t.Fatal("TRACEPARENT should not be in otelEnvVarsForJobs")
			}
		}
		if !hasEndpoint {
			t.Fatal("expected OTEL_EXPORTER_OTLP_ENDPOINT to be present")
		}
	})

	t.Run("returns nil when forwarding disabled", func(t *testing.T) {
		t.Setenv("RENOVATE_FORWARD_OTEL", "false")
		_ = config.InitializeConfigModule(otelConfigKeys)

		envs := otelEnvVarsForJobs()
		if envs != nil {
			t.Fatalf("expected nil when forwarding disabled, got %v", envs)
		}
	})

	t.Run("returns nil when no endpoint resolved", func(t *testing.T) {
		t.Setenv("RENOVATE_FORWARD_OTEL", "true")
		_ = config.InitializeConfigModule(otelConfigKeys)

		envs := otelEnvVarsForJobs()
		if envs != nil {
			t.Fatalf("expected nil when no endpoint, got %v", envs)
		}
	})
}

func TestTraceCarrierEnvVars(t *testing.T) {
	t.Run("injects TRACEPARENT only when tracestate absent", func(t *testing.T) {
		carrier := propagation.MapCarrier{"traceparent": "00-abc123-def456-01"}
		envs := traceCarrierEnvVars(carrier)
		if len(envs) != 1 || envs[0].Name != "TRACEPARENT" || envs[0].Value != "00-abc123-def456-01" {
			t.Fatalf("expected single TRACEPARENT env var, got %v", envs)
		}
	})

	t.Run("injects both TRACEPARENT and TRACESTATE when present", func(t *testing.T) {
		carrier := propagation.MapCarrier{
			"traceparent": "00-abc123-def456-01",
			"tracestate":  "vendor=value",
		}
		envs := traceCarrierEnvVars(carrier)
		if len(envs) != 2 {
			t.Fatalf("expected 2 env vars, got %v", envs)
		}
		names := map[string]string{}
		for _, e := range envs {
			names[e.Name] = e.Value
		}
		if names["TRACEPARENT"] != "00-abc123-def456-01" {
			t.Errorf("TRACEPARENT mismatch: %v", names["TRACEPARENT"])
		}
		if names["TRACESTATE"] != "vendor=value" {
			t.Errorf("TRACESTATE mismatch: %v", names["TRACESTATE"])
		}
	})

	t.Run("returns nil for nil carrier", func(t *testing.T) {
		envs := traceCarrierEnvVars(nil)
		if envs != nil {
			t.Fatalf("expected nil for nil carrier, got %v", envs)
		}
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

func expectEnvVarFromSecretKey(t *testing.T, container *v1.Container, name, secretName, secretKey string) {
	for _, env := range container.Env {
		if env.Name != name {
			continue
		}
		if env.ValueFrom == nil || env.ValueFrom.SecretKeyRef == nil {
			t.Fatalf("expected env var %s to use secret key ref, got %+v", name, env)
		}
		if env.ValueFrom.SecretKeyRef.Name != secretName || env.ValueFrom.SecretKeyRef.Key != secretKey {
			t.Fatalf("expected env var %s to use %s/%s, got %+v", name, secretName, secretKey, env.ValueFrom.SecretKeyRef)
		}
		return
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

func expectPriorityClassName(t *testing.T, job *batchv1.Job, expectedPriorityClassName string) {
	if job.Spec.Template.Spec.PriorityClassName != expectedPriorityClassName {
		t.Fatalf("expected priority class name %q, got %q", expectedPriorityClassName, job.Spec.Template.Spec.PriorityClassName)
	}
}

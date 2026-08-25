package renovate

import (
	"encoding/json"
	"fmt"
	api "renovate-operator/api/v1alpha1"
	"renovate-operator/config"
	"renovate-operator/github"
	"renovate-operator/internal/utils"
	"strconv"
	"strings"

	"go.opentelemetry.io/otel/propagation"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
)

// create job spec for a discovery job
func newDiscoveryJob(job *api.RenovateJob, carrier propagation.MapCarrier) *batchv1.Job {
	predefinedEnvVars := getDefaultEnvVars(job)
	predefinedEnvVars = append(predefinedEnvVars, otelEnvVarsForJobs()...)
	predefinedEnvVars = append(predefinedEnvVars, traceCarrierEnvVars(carrier)...)

	if len(job.Spec.DiscoveryFilters) > 0 {
		filter := strings.Join(job.Spec.DiscoveryFilters, ",")
		predefinedEnvVars = append(predefinedEnvVars, v1.EnvVar{
			Name:  "RENOVATE_AUTODISCOVER_FILTER",
			Value: filter,
		})
	}
	if len(job.Spec.DiscoverTopics) > 0 {
		filter := strings.Join(job.Spec.DiscoverTopics, ",")
		predefinedEnvVars = append(predefinedEnvVars, v1.EnvVar{
			Name:  "RENOVATE_AUTODISCOVER_TOPICS",
			Value: filter,
		})
	}

	envFromSecrets := []v1.EnvFromSource{}
	if job.Spec.SecretRef != "" {
		envFromSecrets = append(envFromSecrets, v1.EnvFromSource{
			SecretRef: &v1.SecretEnvSource{
				LocalObjectReference: v1.LocalObjectReference{
					Name: job.Spec.SecretRef,
				},
			},
		})
	}
	if job.Spec.ExtraEnvFrom != nil {
		envFromSecrets = append(envFromSecrets, job.Spec.ExtraEnvFrom...)
	}

	if job.Spec.GithubAppReference != nil {
		envFromSecrets = append(envFromSecrets, v1.EnvFromSource{
			SecretRef: &v1.SecretEnvSource{
				LocalObjectReference: v1.LocalObjectReference{
					Name: github.GetNameForGithubAppSecret(job),
				},
			},
		})
	}

	volumes, volumeMounts := getVolumeAndMounts(job)

	discoveryCmd := `BASE_DIR="${RENOVATE_BASE_DIR:-/tmp}"; renovate --autodiscover --write-discovered-repos "$BASE_DIR/repos.json" >> "$BASE_DIR/logs.json" 2>&1 && cat "$BASE_DIR/repos.json" || cat "$BASE_DIR/logs.json"`

	batchJob := &batchv1.Job{
		Spec: batchv1.JobSpec{
			ActiveDeadlineSeconds:   getJobTimeoutSeconds(),
			BackoffLimit:            getJobBackOffLimit(),
			TTLSecondsAfterFinished: getJobTTLSecondsAfterFinished(),
			Template: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					ServiceAccountName:            getServiceAccountName(job.Spec),
					ImagePullSecrets:              append(job.Spec.ImagePullSecrets, getDefaultImagePullSecrets()...),
					TerminationGracePeriodSeconds: new(int64(0)),
					Containers: []v1.Container{
						{
							Name:            "discovery",
							Command:         []string{"/bin/sh", "-c"},
							Args:            []string{discoveryCmd},
							Image:           job.Spec.Image,
							Env:             mergeEnvVars(job.Spec.ExtraEnv, predefinedEnvVars),
							EnvFrom:         envFromSecrets,
							Resources:       job.Spec.Resources,
							VolumeMounts:    volumeMounts,
							SecurityContext: getContainerSecurityContext(job.Spec),
						},
					},
					SecurityContext:              getPodSecurityContext(job.Spec),
					AutomountServiceAccountToken: getAutoMountServiceAccountToken(job.Spec),
					RestartPolicy:                v1.RestartPolicyNever,
					DNSPolicy:                    getDNSPolicy(job.Spec),
					NodeSelector:                 job.Spec.NodeSelector,
					Affinity:                     job.Spec.Affinity,
					Tolerations:                  job.Spec.Tolerations,
					TopologySpreadConstraints:    job.Spec.TopologySpreadConstraints,
					RuntimeClassName:             job.Spec.RuntimeClassName,
					PriorityClassName:            job.Spec.PriorityClassName,
					Volumes:                      volumes,
				},
			},
		},
	}

	jobName := utils.DiscoveryJobName(job)
	batchJob.GenerateName = jobName
	batchJob.Namespace = job.Namespace
	if job.Spec.Metadata != nil {
		batchJob.Spec.Template.Annotations = job.Spec.Metadata.Annotations
		batchJob.Annotations = job.Spec.Metadata.Annotations
		batchJob.Labels = job.Spec.Metadata.Labels
		batchJob.Spec.Template.Labels = job.Spec.Metadata.Labels
	}
	return batchJob
}

// create a Job spec for renovate run on project...
func newRenovateJob(job *api.RenovateJob, project string, executionOptions *api.RenovateExecutionOptions, carrier propagation.MapCarrier) *batchv1.Job {
	predefinedEnvVars := getDefaultEnvVars(job)
	predefinedEnvVars = append(predefinedEnvVars, otelEnvVarsForJobs()...)
	predefinedEnvVars = append(predefinedEnvVars, traceCarrierEnvVars(carrier)...)

	if executionOptions != nil && executionOptions.Debug {
		predefinedEnvVars = append(predefinedEnvVars, v1.EnvVar{
			Name:  "RENOVATE_LOG_LEVEL",
			Value: "debug",
		})
	}

	envFromSecrets := []v1.EnvFromSource{}
	if job.Spec.SecretRef != "" {
		envFromSecrets = append(envFromSecrets, v1.EnvFromSource{
			SecretRef: &v1.SecretEnvSource{
				LocalObjectReference: v1.LocalObjectReference{
					Name: job.Spec.SecretRef,
				},
			},
		})
	}
	if job.Spec.ExtraEnvFrom != nil {
		envFromSecrets = append(envFromSecrets, job.Spec.ExtraEnvFrom...)
	}

	if job.Spec.GithubAppReference != nil {
		envFromSecrets = append(envFromSecrets, v1.EnvFromSource{
			SecretRef: &v1.SecretEnvSource{
				LocalObjectReference: v1.LocalObjectReference{
					Name: github.GetNameForGithubAppSecret(job),
				},
			},
		})
	}

	volumes, volumeMounts := getVolumeAndMounts(job)

	command := []string{"renovate"}
	args := []string{"--autodiscover=false", project}

	batchJob := &batchv1.Job{
		Spec: batchv1.JobSpec{
			ActiveDeadlineSeconds:   getJobTimeoutSeconds(),
			BackoffLimit:            getJobBackOffLimit(),
			TTLSecondsAfterFinished: getJobTTLSecondsAfterFinished(),
			Template: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					ServiceAccountName:            getServiceAccountName(job.Spec),
					ImagePullSecrets:              append(job.Spec.ImagePullSecrets, getDefaultImagePullSecrets()...),
					TerminationGracePeriodSeconds: new(int64(0)),
					Containers: []v1.Container{
						{
							Name:            "renovate",
							Command:         command,
							Args:            args,
							Image:           job.Spec.Image,
							Env:             mergeEnvVars(job.Spec.ExtraEnv, predefinedEnvVars),
							EnvFrom:         envFromSecrets,
							Resources:       job.Spec.Resources,
							VolumeMounts:    volumeMounts,
							SecurityContext: getContainerSecurityContext(job.Spec),
						},
					},
					SecurityContext:              getPodSecurityContext(job.Spec),
					AutomountServiceAccountToken: getAutoMountServiceAccountToken(job.Spec),
					RestartPolicy:                v1.RestartPolicyNever,
					DNSPolicy:                    getDNSPolicy(job.Spec),
					NodeSelector:                 job.Spec.NodeSelector,
					Affinity:                     job.Spec.Affinity,
					Tolerations:                  job.Spec.Tolerations,
					TopologySpreadConstraints:    job.Spec.TopologySpreadConstraints,
					RuntimeClassName:             job.Spec.RuntimeClassName,
					PriorityClassName:            job.Spec.PriorityClassName,
					Volumes:                      volumes,
				},
			},
		},
	}

	jobName := utils.ExecutorJobName(job, project)
	batchJob.GenerateName = jobName
	batchJob.Namespace = job.Namespace
	if job.Spec.Metadata != nil {
		batchJob.Spec.Template.Annotations = job.Spec.Metadata.Annotations
		batchJob.Annotations = job.Spec.Metadata.Annotations
		batchJob.Labels = job.Spec.Metadata.Labels
		batchJob.Spec.Template.Labels = job.Spec.Metadata.Labels
	}
	return batchJob
}

func getDefaultEnvVars(job *api.RenovateJob) []v1.EnvVar {

	predefinedEnvVars := []v1.EnvVar{
		{
			Name:  "RENOVATE_LOG_FORMAT",
			Value: "json",
		},
		{
			Name:  "RENOVATE_REPORT_TYPE",
			Value: "logging",
		},
		{
			Name:  "NODE_NO_WARNINGS",
			Value: "1",
		},
	}

	if job.Spec.ScratchVolume == nil || job.Spec.ScratchVolume.Enabled {
		path := getScratchVolumePath(job.Spec.ScratchVolume)

		predefinedEnvVars = append(predefinedEnvVars, v1.EnvVar{
			Name:  "RENOVATE_BASE_DIR",
			Value: path,
		})
	}

	if job.Spec.RenovateConfig != nil {
		predefinedEnvVars = append(predefinedEnvVars, v1.EnvVar{
			Name:  "RENOVATE_CONFIG_FILE",
			Value: renovateConfigMountPath + "/" + renovateConfigFileName(job.Spec.RenovateConfig),
		})
	}

	if job.Spec.Provider != nil {
		platform, endpoint := utils.GetPlatformAndEndpoint(job.Spec.Provider)
		predefinedEnvVars = append(predefinedEnvVars, v1.EnvVar{
			Name:  "RENOVATE_ENDPOINT",
			Value: endpoint,
		}, v1.EnvVar{
			Name:  "RENOVATE_PLATFORM",
			Value: platform,
		})
	}

	if config.GetValue("VALKEY_FORWARD_CACHE_TO_JOBS") == "true" && getRenovateCacheURL() != "" {
		predefinedEnvVars = append(predefinedEnvVars, v1.EnvVar{
			Name: "RENOVATE_REDIS_URL",
			ValueFrom: &v1.EnvVarSource{
				SecretKeyRef: &v1.SecretKeySelector{
					LocalObjectReference: v1.LocalObjectReference{
						Name: redisURLSecretName,
					},
					Key: "redis-url",
				},
			},
		})
	}

	if config.GetValue("S3_FORWARD_CACHE_TO_JOBS") == "true" && config.GetValue("S3_BUCKET") != "" {
		s3CacheType := fmt.Sprintf("s3://%s/%s/", config.GetValue("S3_BUCKET"), config.GetValue("S3_CACHE_PREFIX"))
		predefinedEnvVars = append(predefinedEnvVars,
			v1.EnvVar{Name: "RENOVATE_REPOSITORY_CACHE", Value: "enabled"},
			v1.EnvVar{Name: "RENOVATE_REPOSITORY_CACHE_TYPE", Value: s3CacheType},
			v1.EnvVar{Name: "AWS_REGION", Value: config.GetValue("S3_REGION")},
		)
		if ep := config.GetValue("S3_ENDPOINT"); ep != "" {
			predefinedEnvVars = append(predefinedEnvVars,
				v1.EnvVar{Name: "RENOVATE_S3_ENDPOINT", Value: ep},
			)
		}
		if config.GetValue("S3_FORCE_PATH_STYLE") == "true" {
			predefinedEnvVars = append(predefinedEnvVars,
				v1.EnvVar{Name: "RENOVATE_S3_PATH_STYLE", Value: "true"},
			)
		}
		if secretName := config.GetValue("S3_CREDENTIALS_SECRET_NAME"); secretName != "" {
			predefinedEnvVars = append(predefinedEnvVars,
				v1.EnvVar{
					Name: "AWS_ACCESS_KEY_ID",
					ValueFrom: &v1.EnvVarSource{
						SecretKeyRef: &v1.SecretKeySelector{
							LocalObjectReference: v1.LocalObjectReference{Name: secretName},
							Key:                  "access-key-id",
						},
					},
				},
				v1.EnvVar{
					Name: "AWS_SECRET_ACCESS_KEY",
					ValueFrom: &v1.EnvVarSource{
						SecretKeyRef: &v1.SecretKeySelector{
							LocalObjectReference: v1.LocalObjectReference{Name: secretName},
							Key:                  "secret-access-key",
						},
					},
				},
			)
		}
	}

	return predefinedEnvVars
}

func hardenedPodSecurityContext() *v1.PodSecurityContext {
	return &v1.PodSecurityContext{
		RunAsUser:    new(int64(12021)),
		RunAsGroup:   new(int64(12021)),
		FSGroup:      new(int64(12021)),
		RunAsNonRoot: new(true),
		SeccompProfile: &v1.SeccompProfile{
			Type: v1.SeccompProfileTypeRuntimeDefault,
		},
	}
}

func hardenedContainerSecurityContext() *v1.SecurityContext {
	return &v1.SecurityContext{
		RunAsUser:    new(int64(12021)),
		RunAsGroup:   new(int64(12021)),
		RunAsNonRoot: new(true),
		SeccompProfile: &v1.SeccompProfile{
			Type: v1.SeccompProfileTypeRuntimeDefault,
		},
		ReadOnlyRootFilesystem:   new(false),
		Privileged:               new(false),
		AllowPrivilegeEscalation: new(false),
		Capabilities: &v1.Capabilities{
			Drop: []v1.Capability{"ALL"},
		},
	}
}

// getPodSecurityContext merges the spec over the hardened defaults rather than
// replacing them: overriding one field (fsGroup, say) must not silently drop
// runAsNonRoot and the seccomp profile with it.
func getPodSecurityContext(spec api.RenovateJobSpec) *v1.PodSecurityContext {
	defaults := hardenedPodSecurityContext()
	if spec.SecurityContext == nil || spec.SecurityContext.Pod == nil {
		return defaults
	}

	merged := spec.SecurityContext.Pod.DeepCopy()
	if merged.RunAsUser == nil {
		merged.RunAsUser = defaults.RunAsUser
	}
	if merged.RunAsGroup == nil {
		merged.RunAsGroup = defaults.RunAsGroup
	}
	if merged.FSGroup == nil {
		merged.FSGroup = defaults.FSGroup
	}
	if merged.RunAsNonRoot == nil {
		merged.RunAsNonRoot = defaults.RunAsNonRoot
	}
	if merged.SeccompProfile == nil {
		merged.SeccompProfile = defaults.SeccompProfile
	}
	return merged
}

// getContainerSecurityContext merges the spec over the hardened defaults; see
// getPodSecurityContext.
func getContainerSecurityContext(spec api.RenovateJobSpec) *v1.SecurityContext {
	defaults := hardenedContainerSecurityContext()
	if spec.SecurityContext == nil || spec.SecurityContext.Container == nil {
		return defaults
	}

	merged := spec.SecurityContext.Container.DeepCopy()
	if merged.RunAsUser == nil {
		merged.RunAsUser = defaults.RunAsUser
	}
	if merged.RunAsGroup == nil {
		merged.RunAsGroup = defaults.RunAsGroup
	}
	if merged.RunAsNonRoot == nil {
		merged.RunAsNonRoot = defaults.RunAsNonRoot
	}
	if merged.SeccompProfile == nil {
		merged.SeccompProfile = defaults.SeccompProfile
	}
	if merged.ReadOnlyRootFilesystem == nil {
		merged.ReadOnlyRootFilesystem = defaults.ReadOnlyRootFilesystem
	}
	if merged.Privileged == nil {
		merged.Privileged = defaults.Privileged
	}
	if merged.AllowPrivilegeEscalation == nil {
		merged.AllowPrivilegeEscalation = defaults.AllowPrivilegeEscalation
	}
	if merged.Capabilities == nil {
		merged.Capabilities = defaults.Capabilities
	}
	return merged
}

func getAutoMountServiceAccountToken(spec api.RenovateJobSpec) *bool {
	if spec.ServiceAccount != nil && spec.ServiceAccount.AutomountServiceAccountToken != nil {
		return spec.ServiceAccount.AutomountServiceAccountToken
	}
	return new(false)
}

func getServiceAccountName(spec api.RenovateJobSpec) string {
	if spec.ServiceAccount != nil {
		return spec.ServiceAccount.Name
	}
	return ""
}

func getJobTimeoutSeconds() *int64 {
	timeoutString := config.GetValue("JOB_TIMEOUT_SECONDS")
	val, err := strconv.ParseInt(timeoutString, 10, 64)
	if err != nil {
		return new(int64(1800))
	}
	return new(val)
}

func getJobBackOffLimit() *int32 {
	timeoutString := config.GetValue("JOB_BACKOFF_LIMIT")
	val, err := strconv.ParseInt(timeoutString, 10, 32)
	if err != nil {
		return new(int32(1800))
	}
	return new(int32(val))
}

func getJobTTLSecondsAfterFinished() *int32 {
	timeoutString := config.GetValue("JOB_TTL_SECONDS_AFTER_FINISHED")

	if timeoutString == "-1" {
		return nil
	}
	val, err := strconv.ParseInt(timeoutString, 10, 32)
	if err != nil {
		return nil
	}
	return new(int32(val))
}

// imagePullSecrets configured at the operator level via IMAGE_PULL_SECRETS env var
func getDefaultImagePullSecrets() []v1.LocalObjectReference {
	raw := config.GetValue("IMAGE_PULL_SECRETS")
	if raw == "" || raw == "[]" {
		return nil
	}
	var secrets []v1.LocalObjectReference
	if err := json.Unmarshal([]byte(raw), &secrets); err != nil {
		return nil
	}
	return secrets
}

func getDNSPolicy(spec api.RenovateJobSpec) v1.DNSPolicy {
	if spec.DNSPolicy != "" {
		return spec.DNSPolicy
	}

	return v1.DNSClusterFirst
}

// mergeEnvVars combines extraEnv and predefinedEnv, giving priority to extraEnv
// If there are duplicate env var names, the one from extraEnv is used
func mergeEnvVars(extraEnv []v1.EnvVar, predefinedEnv []v1.EnvVar) []v1.EnvVar {
	// Create a map of env var names from extraEnv
	extraNames := make(map[string]bool)
	for _, env := range extraEnv {
		extraNames[env.Name] = true
	}

	// Start with extraEnv (these take priority)
	result := make([]v1.EnvVar, len(extraEnv))
	copy(result, extraEnv)

	// Add predefined vars that don't conflict
	for _, env := range predefinedEnv {
		if !extraNames[env.Name] {
			result = append(result, env)
		}
	}

	return result
}

// otelEnvVarsForJobs returns OTEL_* env vars to forward to Renovate Jobs when
// RENOVATE_FORWARD_OTEL is set to "true". This enables Renovate's built-in OTLP
// export in Job containers. Returns nil when forwarding is disabled or no OTLP
// endpoint is resolved.
// Note: OTEL_EXPORTER_OTLP_PROTOCOL is intentionally not forwarded because
// Renovate uses OTLP/HTTP while the operator uses gRPC. The endpoint is read
// from RENOVATE_JOB_OTEL_ENDPOINT first (allows pointing Jobs at the HTTP port),
// falling back to OTEL_EXPORTER_OTLP_ENDPOINT.
func otelEnvVarsForJobs() []v1.EnvVar {
	if config.GetValue("RENOVATE_FORWARD_OTEL") != "true" {
		return nil
	}

	// Prefer dedicated job endpoint (HTTP/4318) over operator endpoint (gRPC/4317)
	jobEndpoint := config.GetValue("RENOVATE_JOB_OTEL_ENDPOINT")
	if jobEndpoint == "" {
		jobEndpoint = config.GetValue("OTEL_EXPORTER_OTLP_ENDPOINT")
	}
	if jobEndpoint == "" {
		return nil
	}

	envs := []v1.EnvVar{
		{Name: "OTEL_EXPORTER_OTLP_ENDPOINT", Value: jobEndpoint},
	}
	if val := config.GetValue("OTEL_SERVICE_NAMESPACE"); val != "" {
		envs = append(envs, v1.EnvVar{Name: "OTEL_SERVICE_NAMESPACE", Value: val})
	}
	envs = append(envs,
		v1.EnvVar{Name: "OTEL_SERVICE_NAME", Value: "renovate"},
	)
	return envs
}

// traceCarrierEnvVars converts a W3C trace context carrier into TRACEPARENT
// and TRACESTATE env vars for Renovate Jobs. Both headers are forwarded so
// vendor-specific tracestate extensions (e.g. Datadog, Jaeger adaptive
// sampling) survive the operator→Job boundary.
func traceCarrierEnvVars(carrier propagation.MapCarrier) []v1.EnvVar {
	var envs []v1.EnvVar
	if v := carrier.Get("traceparent"); v != "" {
		envs = append(envs, v1.EnvVar{Name: "TRACEPARENT", Value: v})
	}
	if v := carrier.Get("tracestate"); v != "" {
		envs = append(envs, v1.EnvVar{Name: "TRACESTATE", Value: v})
	}
	return envs
}

func getVolumeAndMounts(job *api.RenovateJob) ([]v1.Volume, []v1.VolumeMount) {
	volumes := []v1.Volume{}
	volumeMounts := []v1.VolumeMount{}

	if job.Spec.ScratchVolume == nil || job.Spec.ScratchVolume.Enabled {
		volumeName := "tmp"
		path := getScratchVolumePath(job.Spec.ScratchVolume)
		volumeSource := v1.VolumeSource{}

		if job.Spec.ScratchVolume != nil {

			if job.Spec.ScratchVolume.Ephemeral != nil {
				volumeSource.Ephemeral = job.Spec.ScratchVolume.Ephemeral
			} else {
				volumeSource.EmptyDir = &v1.EmptyDirVolumeSource{
					Medium:    job.Spec.ScratchVolume.Medium,
					SizeLimit: job.Spec.ScratchVolume.SizeLimit,
				}
			}
		} else {
			volumeSource.EmptyDir = &v1.EmptyDirVolumeSource{}
		}

		volume := v1.Volume{
			Name:         volumeName,
			VolumeSource: volumeSource,
		}
		mount := v1.VolumeMount{
			Name:      volumeName,
			MountPath: path,
		}
		volumes = append(volumes, volume)
		volumeMounts = append(volumeMounts, mount)
	}

	if cfg := job.Spec.RenovateConfig; cfg != nil {
		configMapName := renovateConfigMapName(job)
		if cfg.ConfigMapRef != nil {
			configMapName = cfg.ConfigMapRef.Name
		}
		fileName := renovateConfigFileName(cfg)
		volumes = append(volumes, v1.Volume{
			Name: renovateConfigVolumeName,
			VolumeSource: v1.VolumeSource{
				ConfigMap: &v1.ConfigMapVolumeSource{
					LocalObjectReference: v1.LocalObjectReference{Name: configMapName},
					Items:                []v1.KeyToPath{{Key: fileName, Path: fileName}},
				},
			},
		})
		volumeMounts = append(volumeMounts, v1.VolumeMount{
			Name:      renovateConfigVolumeName,
			MountPath: renovateConfigMountPath,
			ReadOnly:  true,
		})
	}

	if job.Spec.ExtraVolumes != nil {
		volumes = append(volumes, job.Spec.ExtraVolumes...)
	}
	if job.Spec.ExtraVolumeMounts != nil {
		volumeMounts = append(volumeMounts, job.Spec.ExtraVolumeMounts...)
	}

	return volumes, volumeMounts
}

func getScratchVolumePath(scratch *api.RenovateJobScratchVolume) string {
	if scratch != nil && scratch.Path != "" {
		return scratch.Path
	}
	return "/tmp"
}

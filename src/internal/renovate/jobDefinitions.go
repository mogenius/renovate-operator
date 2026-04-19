package renovate

import (
	"encoding/json"
	"maps"
	api "renovate-operator/api/v1alpha1"
	"renovate-operator/config"
	crdmanager "renovate-operator/internal/crdManager"
	"renovate-operator/internal/utils"
	"strconv"
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"

	"k8s.io/utils/ptr"
)

// create job spec for a discovery job
func newDiscoveryJob(job *api.RenovateJob, traceparent string) *batchv1.Job {
	predefinedEnvVars := getDefaultEnvVars(job)

	if traceparent != "" {
		predefinedEnvVars = append(predefinedEnvVars, v1.EnvVar{
			Name:  "TRACEPARENT",
			Value: traceparent,
		})
	}

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

	volumes, volumeMounts := getVolumeAndMounts(job)

	batchJob := &batchv1.Job{
		Spec: batchv1.JobSpec{
			ActiveDeadlineSeconds:   getJobTimeoutSeconds(),
			BackoffLimit:            getJobBackOffLimit(),
			TTLSecondsAfterFinished: getJobTTLSecondsAfterFinished(),
			Template: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					ServiceAccountName:            getServiceAccountName(job.Spec),
					ImagePullSecrets:              append(job.Spec.ImagePullSecrets, getDefaultImagePullSecrets()...),
					TerminationGracePeriodSeconds: ptr.To(int64(0)),
					Containers: []v1.Container{
						{
							Name:            "discovery",
							Command:         []string{"/bin/sh", "-c"},
							Args:            []string{`BASE_DIR="${RENOVATE_BASE_DIR:-/tmp}"; renovate --autodiscover --write-discovered-repos "$BASE_DIR/repos.json" >> "$BASE_DIR/logs.json" 2>&1 && cat "$BASE_DIR/repos.json" || cat "$BASE_DIR/logs.json"`},
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
	}
	labels := getJobLabels(job.Spec.Metadata, crdmanager.DiscoveryJobType, jobName)
	batchJob.Spec.Template.Labels = labels
	batchJob.Labels = labels
	return batchJob
}

// create a Job spec for renovate run on project...
func newRenovateJob(job *api.RenovateJob, project string, traceparent string) *batchv1.Job {
	predefinedEnvVars := getDefaultEnvVars(job)

	if traceparent != "" {
		predefinedEnvVars = append(predefinedEnvVars, v1.EnvVar{
			Name:  "TRACEPARENT",
			Value: traceparent,
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

	volumes, volumeMounts := getVolumeAndMounts(job)

	batchJob := &batchv1.Job{
		Spec: batchv1.JobSpec{
			ActiveDeadlineSeconds:   getJobTimeoutSeconds(),
			BackoffLimit:            getJobBackOffLimit(),
			TTLSecondsAfterFinished: getJobTTLSecondsAfterFinished(),
			Template: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					ServiceAccountName:            getServiceAccountName(job.Spec),
					ImagePullSecrets:              append(job.Spec.ImagePullSecrets, getDefaultImagePullSecrets()...),
					TerminationGracePeriodSeconds: ptr.To(int64(0)),
					Containers: []v1.Container{
						{
							Name:            "renovate",
							Command:         []string{"renovate"},
							Args:            []string{project},
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
	}
	labels := getJobLabels(job.Spec.Metadata, crdmanager.ExecutorJobType, jobName)
	batchJob.Labels = labels
	batchJob.Spec.Template.Labels = labels
	return batchJob
}

func getDefaultEnvVars(job *api.RenovateJob) []v1.EnvVar {

	predefinedEnvVars := []v1.EnvVar{
		{
			Name:  "LOG_FORMAT",
			Value: "json",
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

	if job.Status.ExecutionOptions != nil && job.Status.ExecutionOptions.Debug {
		predefinedEnvVars = append(predefinedEnvVars, v1.EnvVar{
			Name:  "LOG_LEVEL",
			Value: "debug",
		})
	}

	if config.GetValue("VALKEY_FORWARD_CACHE_TO_JOBS") == "true" && getValkeyURL() != "" {
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
	return predefinedEnvVars
}

func getPodSecurityContext(spec api.RenovateJobSpec) *v1.PodSecurityContext {
	if spec.SecurityContext != nil && spec.SecurityContext.Pod != nil {
		return spec.SecurityContext.Pod
	}

	return &v1.PodSecurityContext{
		RunAsUser:    ptr.To(int64(12021)),
		RunAsGroup:   ptr.To(int64(12021)),
		FSGroup:      ptr.To(int64(12021)),
		RunAsNonRoot: ptr.To(true),
		SeccompProfile: &v1.SeccompProfile{
			Type: v1.SeccompProfileTypeRuntimeDefault,
		},
	}
}
func getContainerSecurityContext(spec api.RenovateJobSpec) *v1.SecurityContext {
	if spec.SecurityContext != nil && spec.SecurityContext.Container != nil {
		return spec.SecurityContext.Container
	}

	return &v1.SecurityContext{
		RunAsUser:    ptr.To(int64(12021)),
		RunAsGroup:   ptr.To(int64(12021)),
		RunAsNonRoot: ptr.To(true),
		SeccompProfile: &v1.SeccompProfile{
			Type: v1.SeccompProfileTypeRuntimeDefault,
		},
		ReadOnlyRootFilesystem:   ptr.To(false),
		Privileged:               ptr.To(false),
		AllowPrivilegeEscalation: ptr.To(false),
		Capabilities: &v1.Capabilities{
			Drop: []v1.Capability{"ALL"},
		},
	}
}

func getAutoMountServiceAccountToken(spec api.RenovateJobSpec) *bool {
	if spec.ServiceAccount != nil && spec.ServiceAccount.AutomountServiceAccountToken != nil {
		return spec.ServiceAccount.AutomountServiceAccountToken
	}
	return ptr.To(false)
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
		return ptr.To(int64(1800))
	}
	return ptr.To(val)
}

func getJobBackOffLimit() *int32 {
	timeoutString := config.GetValue("JOB_BACKOFF_LIMIT")
	val, err := strconv.ParseInt(timeoutString, 10, 32)
	if err != nil {
		return ptr.To(int32(1800))
	}
	return ptr.To(int32(val))
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
	return ptr.To(int32(val))
}

func getJobLabels(metadata *api.RenovateJobMetadata, jobType crdmanager.JobType, jobName string) map[string]string {
	labels := map[string]string{
		crdmanager.JOB_LABEL_TYPE: string(jobType),
		crdmanager.JOB_LABEL_NAME: jobName,
	}
	if metadata != nil {
		maps.Copy(labels, metadata.Labels)
	}
	return labels
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

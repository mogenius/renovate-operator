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

// builtInScratchVolumeName is the Kubernetes volume name for the emptyDir backing RENOVATE_BASE_DIR.
const builtInScratchVolumeName = "scratch"

// create job spec for a discovery job
func newDiscoveryJob(job *api.RenovateJob) *batchv1.Job {
	predefinedEnvVars := getDefaultEnvVars(job)

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

	containerEnv := withCanonicalRenovateBaseDir(mergeEnvVars(job.Spec.ExtraEnv, predefinedEnvVars), job.Spec)
	scratchPath := defaultRenovateBaseDir(job.Spec)

	scratchVolSrc := scratchEmptyDirVolumeSource(job.Spec)
	volumes := []v1.Volume{
		{
			Name: builtInScratchVolumeName,
			VolumeSource: v1.VolumeSource{
				EmptyDir: &scratchVolSrc,
			},
		},
	}

	volumeMounts := []v1.VolumeMount{
		{
			Name:      builtInScratchVolumeName,
			MountPath: scratchPath,
		},
	}

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
							Args:            []string{`renovate --autodiscover --write-discovered-repos "$RENOVATE_BASE_DIR/repos.json" >> "$RENOVATE_BASE_DIR/logs.json" 2>&1 && cat "$RENOVATE_BASE_DIR/repos.json" || cat "$RENOVATE_BASE_DIR/logs.json"`},
							Image:           job.Spec.Image,
							Env:             containerEnv,
							EnvFrom:         envFromSecrets,
							Resources:       mergeResourcesWithScratchVolume(job.Spec.Resources, job.Spec.ScratchVolume),
							VolumeMounts:    append(volumeMounts, job.Spec.ExtraVolumeMounts...),
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
					Volumes:                      append(volumes, job.Spec.ExtraVolumes...),
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
func newRenovateJob(job *api.RenovateJob, project string) *batchv1.Job {
	predefinedEnvVars := getDefaultEnvVars(job)

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

	containerEnv := withCanonicalRenovateBaseDir(mergeEnvVars(job.Spec.ExtraEnv, predefinedEnvVars), job.Spec)
	scratchPath := defaultRenovateBaseDir(job.Spec)

	scratchVolSrc := scratchEmptyDirVolumeSource(job.Spec)
	volumes := []v1.Volume{
		{
			Name: builtInScratchVolumeName,
			VolumeSource: v1.VolumeSource{
				EmptyDir: &scratchVolSrc,
			},
		},
	}

	volumeMounts := []v1.VolumeMount{
		{
			Name:      builtInScratchVolumeName,
			MountPath: scratchPath,
		},
	}

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
							Env:             containerEnv,
							EnvFrom:         envFromSecrets,
							Resources:       mergeResourcesWithScratchVolume(job.Spec.Resources, job.Spec.ScratchVolume),
							VolumeMounts:    append(volumeMounts, job.Spec.ExtraVolumeMounts...),
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
					Volumes:                      append(volumes, job.Spec.ExtraVolumes...),
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

func scratchEmptyDirVolumeSource(spec api.RenovateJobSpec) v1.EmptyDirVolumeSource {
	src := v1.EmptyDirVolumeSource{}
	if spec.ScratchVolume == nil {
		return src
	}
	if spec.ScratchVolume.Medium != "" {
		src.Medium = spec.ScratchVolume.Medium
	}
	if spec.ScratchVolume.SizeLimit != nil {
		src.SizeLimit = spec.ScratchVolume.SizeLimit
	}
	return src
}

func mergeResourcesWithScratchVolume(base v1.ResourceRequirements, scratch *api.RenovateJobScratchVolume) v1.ResourceRequirements {
	if scratch == nil || (scratch.EphemeralStorageRequest == nil && scratch.EphemeralStorageLimit == nil) {
		return base
	}
	merged := base
	if scratch.EphemeralStorageRequest != nil {
		if merged.Requests == nil {
			merged.Requests = v1.ResourceList{}
		} else {
			merged.Requests = maps.Clone(merged.Requests)
		}
		merged.Requests[v1.ResourceEphemeralStorage] = *scratch.EphemeralStorageRequest
	}
	if scratch.EphemeralStorageLimit != nil {
		if merged.Limits == nil {
			merged.Limits = v1.ResourceList{}
		} else {
			merged.Limits = maps.Clone(merged.Limits)
		}
		merged.Limits[v1.ResourceEphemeralStorage] = *scratch.EphemeralStorageLimit
	}
	return merged
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
		{
			Name:  "RENOVATE_BASE_DIR",
			Value: defaultRenovateBaseDir(job.Spec),
		},
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

func defaultRenovateBaseDir(spec api.RenovateJobSpec) string {
	if spec.RenovateBaseDir != "" {
		return spec.RenovateBaseDir
	}
	return "/tmp"
}

// withCanonicalRenovateBaseDir drops any RENOVATE_BASE_DIR from env (e.g. extraEnv) and appends
// the value from spec.renovateBaseDir so the scratch mount and env stay a single source of truth.
func withCanonicalRenovateBaseDir(env []v1.EnvVar, spec api.RenovateJobSpec) []v1.EnvVar {
	dir := defaultRenovateBaseDir(spec)
	envExcludingRenovateBaseDir := make([]v1.EnvVar, 0, len(env)+1)
	for _, e := range env {
		if e.Name == "RENOVATE_BASE_DIR" {
			continue
		}
		envExcludingRenovateBaseDir = append(envExcludingRenovateBaseDir, e)
	}
	return append(envExcludingRenovateBaseDir, v1.EnvVar{Name: "RENOVATE_BASE_DIR", Value: dir})
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

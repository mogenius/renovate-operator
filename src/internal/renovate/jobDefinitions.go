package renovate

import (
	"maps"
	api "renovate-operator/api/v1alpha1"
	"renovate-operator/config"
	"renovate-operator/internal/utils"
	"strconv"

	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
)

// create job spec for a discovery job
func newDiscoveryJob(job *api.RenovateJob) *batchv1.Job {
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

	if job.Spec.DiscoveryFilter != "" {
		predefinedEnvVars = append(predefinedEnvVars, v1.EnvVar{
			Name:  "RENOVATE_AUTODISCOVER_FILTER",
			Value: job.Spec.DiscoveryFilter,
		})
	}
	if job.Spec.DiscoverTopics != "" {
		predefinedEnvVars = append(predefinedEnvVars, v1.EnvVar{
			Name:  "RENOVATE_AUTODISCOVER_TOPICS",
			Value: job.Spec.DiscoverTopics,
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

	volumes := []v1.Volume{
		{
			Name: "tmp",
			VolumeSource: v1.VolumeSource{
				EmptyDir: &v1.EmptyDirVolumeSource{},
			},
		},
	}

	volumeMounts := []v1.VolumeMount{
		{
			Name:      "tmp",
			MountPath: "/tmp",
		},
	}

	batchJob := &batchv1.Job{
		Spec: batchv1.JobSpec{
			ActiveDeadlineSeconds: getJobTimeoutSeconds(),
			BackoffLimit:          getJobBackOffLimit(),
			Template: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					ServiceAccountName:            getServiceAccountName(job.Spec),
					ImagePullSecrets:              job.Spec.ImagePullSecrets,
					TerminationGracePeriodSeconds: ptr.To(int64(0)),
					Containers: []v1.Container{
						{
							Name:            "discovery",
							Command:         []string{"/bin/sh", "-c"},
							Args:            []string{"renovate --autodiscover --write-discovered-repos /tmp/repos.json >> /tmp/logs.json && cat /tmp/repos.json || cat /tmp/logs.json"},
							Image:           job.Spec.Image,
							Env:             append(predefinedEnvVars, job.Spec.ExtraEnv...),
							EnvFrom:         envFromSecrets,
							Resources:       job.Spec.Resources,
							VolumeMounts:    append(volumeMounts, job.Spec.ExtraVolumeMounts...),
							SecurityContext: getContainerSecurityContext(job.Spec),
						},
					},
					SecurityContext:              getPodSecurityContext(job.Spec),
					AutomountServiceAccountToken: getAutoMountServiceAccountToken(job.Spec),
					RestartPolicy:                v1.RestartPolicyOnFailure,
					NodeSelector:                 job.Spec.NodeSelector,
					Affinity:                     job.Spec.Affinity,
					Tolerations:                  job.Spec.Tolerations,
					TopologySpreadConstraints:    job.Spec.TopologySpreadConstraints,
					Volumes:                      append(volumes, job.Spec.ExtraVolumes...),
				},
			},
		},
	}
	batchJob.Name = utils.DiscoveryJobName(job)
	batchJob.Namespace = job.Namespace
	if job.Spec.Metadata != nil {
		batchJob.Spec.Template.Annotations = job.Spec.Metadata.Annotations
	}
	batchJob.Spec.Template.Labels = getJobLabels(job.Spec.Metadata, "discovery", batchJob.Name)
	return batchJob
}

// create a Job spec for renovate run on project...
func newRenovateJob(job *api.RenovateJob, project string) *batchv1.Job {
	// Default env vars - user can override via ExtraEnv since these are prepended
	predefinedEnvVars := []v1.EnvVar{
		{
			Name:  "LOG_FORMAT",
			Value: "json",
		},
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

	volumes := []v1.Volume{
		{
			Name: "tmp",
			VolumeSource: v1.VolumeSource{
				EmptyDir: &v1.EmptyDirVolumeSource{},
			},
		},
	}

	volumeMounts := []v1.VolumeMount{
		{
			Name:      "tmp",
			MountPath: "/tmp",
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
					ImagePullSecrets:              job.Spec.ImagePullSecrets,
					TerminationGracePeriodSeconds: ptr.To(int64(0)),
					Containers: []v1.Container{
						{
							Name:            "renovate",
							Command:         []string{"renovate"},
							Args:            []string{"--base-dir", "/tmp", project},
							Image:           job.Spec.Image,
							Env:             append(predefinedEnvVars, job.Spec.ExtraEnv...),
							EnvFrom:         envFromSecrets,
							Resources:       job.Spec.Resources,
							VolumeMounts:    append(volumeMounts, job.Spec.ExtraVolumeMounts...),
							SecurityContext: getContainerSecurityContext(job.Spec),
						},
					},
					SecurityContext:              getPodSecurityContext(job.Spec),
					AutomountServiceAccountToken: getAutoMountServiceAccountToken(job.Spec),
					RestartPolicy:                v1.RestartPolicyOnFailure,
					NodeSelector:                 job.Spec.NodeSelector,
					Affinity:                     job.Spec.Affinity,
					Tolerations:                  job.Spec.Tolerations,
					TopologySpreadConstraints:    job.Spec.TopologySpreadConstraints,
					Volumes:                      append(volumes, job.Spec.ExtraVolumes...),
				},
			},
		},
	}

	batchJob.Name = utils.ExecutorJobName(job, project)
	batchJob.Namespace = job.Namespace
	if job.Spec.Metadata != nil {
		batchJob.Spec.Template.Annotations = job.Spec.Metadata.Annotations
	}
	batchJob.Spec.Template.Labels = getJobLabels(job.Spec.Metadata, "executor", batchJob.Name)
	return batchJob
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

func getJobLabels(metadata *api.RenovateJobMetadata, jobType, jobName string) map[string]string {
	labels := map[string]string{
		"renovate-operator.mogenius.com/job-type": jobType,
		"renovate-operator.mogenius.com/job-name": jobName,
	}
	if metadata != nil {
		maps.Copy(labels, metadata.Labels)
	}
	return labels
}

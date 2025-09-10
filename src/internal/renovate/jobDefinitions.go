package renovate

import (
	api "renovate-operator/api/v1alpha1"

	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
)

// create job spec for a discovery job
func newDiscoveryJob(job *api.RenovateJob) *batchv1.Job {
	predefinedEnvVars := []v1.EnvVar{}
	if job.Spec.DiscoveryFilter != "" {
		predefinedEnvVars = append(predefinedEnvVars, v1.EnvVar{
			Name:  "RENOVATE_AUTODISCOVER_FILTER",
			Value: job.Spec.DiscoveryFilter,
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
	batchJob := &batchv1.Job{
		Spec: batchv1.JobSpec{
			Template: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					ServiceAccountName: getServiceAccountName(job.Spec),
					Containers: []v1.Container{
						{
							Name:      "discovery",
							Command:   []string{"/bin/sh", "-c"},
							Args:      []string{"renovate --autodiscover --write-discovered-repos /tmp/repos.json >> /tmp/logs.json && cat /tmp/repos.json"},
							Image:     job.Spec.Image,
							Env:       append(predefinedEnvVars, job.Spec.ExtraEnv...),
							EnvFrom:   envFromSecrets,
							Resources: job.Spec.Resources,
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "tmp",
									MountPath: "/tmp",
								},
							},
							SecurityContext: getContainerSecurityContext(job.Spec),
						},
					},
					SecurityContext:              getPodSecurityContext(job.Spec),
					AutomountServiceAccountToken: getAutoMountServiceAccountToken(job.Spec),
					RestartPolicy:                v1.RestartPolicyOnFailure,
					NodeSelector:                 job.Spec.NodeSelector,
					Volumes: []v1.Volume{
						{
							Name: "tmp",
							VolumeSource: v1.VolumeSource{
								EmptyDir: &v1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},
	}
	batchJob.Name = job.Name + "-discovery"
	batchJob.Namespace = job.Namespace
	if job.Spec.Metadata != nil {
		batchJob.Spec.Template.Labels = job.Spec.Metadata.Labels
		batchJob.Spec.Template.Annotations = job.Spec.Metadata.Annotations
	}
	return batchJob
}

// create a Job spec for renovate run on project...
func newRenovateJob(job *api.RenovateJob, project string) *batchv1.Job {
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

	batchJob := &batchv1.Job{
		Spec: batchv1.JobSpec{
			Template: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					ServiceAccountName: getServiceAccountName(job.Spec),
					Containers: []v1.Container{
						{
							Name:      "renovate",
							Command:   []string{"renovate"},
							Args:      []string{"--base-dir", "/tmp", project},
							Image:     job.Spec.Image,
							Env:       job.Spec.ExtraEnv,
							EnvFrom:   envFromSecrets,
							Resources: job.Spec.Resources,
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "tmp",
									MountPath: "/tmp",
								},
							},
							SecurityContext: getContainerSecurityContext(job.Spec),
						},
					},
					SecurityContext:              getPodSecurityContext(job.Spec),
					AutomountServiceAccountToken: getAutoMountServiceAccountToken(job.Spec),
					RestartPolicy:                v1.RestartPolicyOnFailure,
					NodeSelector:                 job.Spec.NodeSelector,
					Volumes: []v1.Volume{
						{
							Name: "tmp",
							VolumeSource: v1.VolumeSource{
								EmptyDir: &v1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},
	}

	batchJob.Name = job.ExecutorJobName(project)
	batchJob.Namespace = job.Namespace
	if job.Spec.Metadata != nil {
		batchJob.Spec.Template.Labels = job.Spec.Metadata.Labels
		batchJob.Spec.Template.Annotations = job.Spec.Metadata.Annotations
	}
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

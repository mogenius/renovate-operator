package renovate

import (
	"testing"

	api "renovate-operator/api/v1alpha1"
	"renovate-operator/github"
	"renovate-operator/internal/policy"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
)

func hostRulesJob(hostRulesSecret string) *api.RenovateJob {
	job := configJob(nil)
	job.Spec.Image = "renovate/renovate:latest"
	job.Spec.Provider = &api.RenovateProvider{Name: "github"}
	job.Spec.GithubAppReference = &api.GithubAppReference{SecretName: "app", HostRulesSecretName: hostRulesSecret}
	return job
}

func TestJobDefinitions_HostRulesMountedIntoDiscoveryAndExecutor(t *testing.T) {
	job := hostRulesJob("club-github-app-host-rules-1234")
	for name, k8sJob := range map[string]*batchv1.Job{
		"discovery": newDiscoveryJob(job),
		"executor":  newRenovateJob(job, "org/repo"),
	} {
		t.Run(name, func(t *testing.T) {
			pod := k8sJob.Spec.Template.Spec
			want := hostRulesMountPath + "/" + github.HostRulesConfigFileKey
			if !hasEnv(pod.Containers[0].Env, policy.AdditionalConfigFileEnvVar, want) {
				t.Errorf("expected %s=%s, got %v", policy.AdditionalConfigFileEnvVar, want, pod.Containers[0].Env)
			}
			vol := find(pod.Volumes, byName[corev1.Volume](hostRulesVolumeName))
			if vol == nil || vol.Secret == nil || vol.Secret.SecretName != "club-github-app-host-rules-1234" {
				t.Fatalf("expected a volume from the hostRules secret, got %+v", vol)
			}
			if len(vol.Secret.Items) != 1 || vol.Secret.Items[0].Key != github.HostRulesConfigFileKey {
				t.Errorf("expected the config file projected by key, got %+v", vol.Secret.Items)
			}
			mount := find(pod.Containers[0].VolumeMounts, byName[corev1.VolumeMount](hostRulesVolumeName))
			if mount == nil || mount.MountPath != hostRulesMountPath || !mount.ReadOnly {
				t.Errorf("expected a read-only mount at %s, got %+v", hostRulesMountPath, mount)
			}
		})
	}
}

func TestJobDefinitions_NoHostRulesWithoutSecretName(t *testing.T) {
	for name, job := range map[string]*api.RenovateJob{
		"githubAppReference without hostRules": hostRulesJob(""),
		"no githubAppReference":                configJob(nil),
	} {
		t.Run(name, func(t *testing.T) {
			pod := newRenovateJob(job, "org/repo").Spec.Template.Spec
			for _, env := range pod.Containers[0].Env {
				if env.Name == policy.AdditionalConfigFileEnvVar {
					t.Errorf("expected no %s without hostRules", policy.AdditionalConfigFileEnvVar)
				}
			}
			if find(pod.Volumes, byName[corev1.Volume](hostRulesVolumeName)) != nil {
				t.Error("expected no hostRules volume without hostRules")
			}
		})
	}
}

func TestJobDefinitions_HostRulesKeepUserHostRulesEnv(t *testing.T) {
	job := hostRulesJob("club-github-app-host-rules-1234")
	job.Spec.ExtraEnv = []corev1.EnvVar{{Name: "RENOVATE_HOST_RULES", Value: `[{"matchHost":"registry.example.com"}]`}}

	pod := newRenovateJob(job, "org/repo").Spec.Template.Spec

	if !hasEnv(pod.Containers[0].Env, "RENOVATE_HOST_RULES", `[{"matchHost":"registry.example.com"}]`) {
		t.Error("user RENOVATE_HOST_RULES must reach Renovate unchanged, which merges it with the additional config file")
	}
	if !hasEnv(pod.Containers[0].Env, policy.AdditionalConfigFileEnvVar, hostRulesMountPath+"/"+github.HostRulesConfigFileKey) {
		t.Error("the shared hostRules must be mounted next to the user's RENOVATE_HOST_RULES")
	}
}

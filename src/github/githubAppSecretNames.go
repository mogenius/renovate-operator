package github

import (
	"crypto/sha256"
	"fmt"
	api "renovate-operator/api/v1alpha1"
	"renovate-operator/internal/utils"
)

func GetNameForGithubAppSecret(job *api.RenovateJob) string {
	return GetNameForGithubAppSecretFromJobName(job.Name)
}

func GetNameForGithubAppSecretFromJobName(name string) string {
	return githubAppSecretName(name, "")
}

func GetNameForGithubAppInstallationSecret(job *api.RenovateJob, installationID string) string {
	return githubAppSecretName(job.Name, installationID)
}

// githubAppSecretName builds a Kubernetes-safe secret name of the form
// {name}-github-app-{suffix}-{sha256[:4]} (or without -{suffix} when empty).
// Total length is guaranteed ≤ 63 characters.
func githubAppSecretName(rawName, suffix string) string {
	name := utils.KubernetesCompatibleName(rawName)
	hash := sha256.Sum256([]byte(name + suffix))
	hashStr := fmt.Sprintf("%x", hash[:4])

	sep := ""
	if suffix != "" {
		sep = "-"
	}

	maxLen := max(43-len(suffix)-len(sep), 1)
	if len(name) > maxLen {
		name = name[:maxLen]
	}

	return name + "-github-app-" + suffix + sep + hashStr
}

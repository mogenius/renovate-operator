package github

import (
	"crypto/sha256"
	"fmt"
	api "renovate-operator/api/v1alpha1"
)

func GetNameForGithubAppSecret(job *api.RenovateJob) string {
	return GetNameForGithubAppSecretFromJobName(job.Name)
}
func GetNameForGithubAppSecretFromJobName(name string) string {

	hash := sha256.Sum256([]byte(name))
	hashStr := fmt.Sprintf("%x", hash[:4])

	if len(name) > 43 {
		name = name[:43]
	}

	return fmt.Sprintf("%s-github-app-%s", name, hashStr)
}

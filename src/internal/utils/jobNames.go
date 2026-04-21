package utils

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	api "renovate-operator/api/v1alpha1"
	"strings"
)

var (
	invalidChars    = regexp.MustCompile(`[^a-z0-9-]+`)
	multipleHyphens = regexp.MustCompile(`-{2,}`)
)

// jobname for the executor job for a project. normalized for kubernetes resourcenames
func ExecutorJobName(in *api.RenovateJob, project string) string {
	fullName := in.Name + "-" + project
	fullName = kubernetesCompatibleName(fullName)

	// Generate hash of the full name
	hash := sha256.Sum256([]byte(fullName))
	hashStr := fmt.Sprintf("%x", hash[:4]) // Use first 4 bytes (8 hex chars)

	// Trim to 54 chars and append hash
	if len(fullName) > 54 {
		fullName = fullName[:54]
	}

	return fullName + "-" + hashStr
}

func kubernetesCompatibleName(name string) string {
	name = strings.ToLower(name) // Ensure lowercase for consistency
	name = invalidChars.ReplaceAllString(name, "-")
	name = multipleHyphens.ReplaceAllString(name, "-")
	name = strings.Trim(name, "-")

	return name
}

func DiscoveryJobName(in *api.RenovateJob) string {
	baseName := in.Name
	baseName = kubernetesCompatibleName(baseName)

	fullName := baseName + "-discovery"

	// Generate hash of the full name
	hash := sha256.Sum256([]byte(fullName))
	hashStr := fmt.Sprintf("%x", hash[:4]) // Use first 4 bytes (8 hex chars)

	// Trim base name to fit: 54 - len("-discovery") = 44 chars max
	if len(baseName) > 44 {
		baseName = baseName[:44]
	}

	return baseName + "-discovery-" + hashStr
}

// jobname for the preUpgrade hook job for a project. normalized for kubernetes resourcenames
func PreUpgradeJobName(in *api.RenovateJob, project string) string {
	fullName := in.Name + "-" + project + "-preupgrade"
	fullName = kubernetesCompatibleName(fullName)

	// Generate hash of the full name
	hash := sha256.Sum256([]byte(fullName))
	hashStr := fmt.Sprintf("%x", hash[:4]) // Use first 4 bytes (8 hex chars)

	// Trim to 54 chars and append hash
	if len(fullName) > 54 {
		fullName = fullName[:54]
	}

	return fullName + "-" + hashStr
}

// jobname for the postUpgrade hook job for a project. normalized for kubernetes resourcenames
func PostUpgradeJobName(in *api.RenovateJob, project string) string {
	fullName := in.Name + "-" + project + "-postupgrade"
	fullName = kubernetesCompatibleName(fullName)

	// Generate hash of the full name
	hash := sha256.Sum256([]byte(fullName))
	hashStr := fmt.Sprintf("%x", hash[:4]) // Use first 4 bytes (8 hex chars)

	// Trim to 54 chars and append hash
	if len(fullName) > 54 {
		fullName = fullName[:54]
	}

	return fullName + "-" + hashStr
}

// LEGACY functions - to be removed February 2026
func LegacyExecutorJobName(in *api.RenovateJob, project string) string {
	jobName := in.Name + "-" + project
	jobName = strings.ReplaceAll(jobName, "/", "-") // Replace slashes to avoid issues with Kubernetes naming
	jobName = strings.ReplaceAll(jobName, "_", "-")
	jobName = strings.ReplaceAll(jobName, ".", "-")
	jobName = strings.ToLower(jobName) // Ensure lowercase for consistency
	return jobName
}

// LEGACY functions - to be removed February 2026
func LegacyDiscoveryJobName(in *api.RenovateJob) string {
	jobName := in.Name + "-discovery"
	return jobName
}

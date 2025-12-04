package utils

import (
	"crypto/sha256"
	"fmt"
	api "renovate-operator/api/v1alpha1"
	"strings"
)

// jobname for the executor job for a project. normalized for kubernetes resourcenames
func ExecutorJobName(in *api.RenovateJob, project string) string {
	fullName := in.Name + "-" + project
	fullName = strings.ReplaceAll(fullName, "/", "-") // Replace slashes to avoid issues with Kubernetes naming
	fullName = strings.ReplaceAll(fullName, "_", "-")
	fullName = strings.ReplaceAll(fullName, ".", "-")
	fullName = strings.ToLower(fullName) // Ensure lowercase for consistency

	// Generate hash of the full name
	hash := sha256.Sum256([]byte(fullName))
	hashStr := fmt.Sprintf("%x", hash[:4]) // Use first 4 bytes (8 hex chars)

	// Trim to 54 chars and append hash
	if len(fullName) > 54 {
		fullName = fullName[:54]
	}

	return fullName + "-" + hashStr
}

func DiscoveryJobName(in *api.RenovateJob) string {
	baseName := in.Name
	baseName = strings.ReplaceAll(baseName, "/", "-")
	baseName = strings.ReplaceAll(baseName, "_", "-")
	baseName = strings.ReplaceAll(baseName, ".", "-")
	baseName = strings.ToLower(baseName)

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

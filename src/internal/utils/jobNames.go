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
	fullName = KubernetesCompatibleName(fullName)

	// Generate hash of the full name
	hash := sha256.Sum256([]byte(fullName))
	hashStr := fmt.Sprintf("%x", hash[:4]) // Use first 4 bytes (8 hex chars)

	// Trim to 54 chars and append hash
	if len(fullName) > 54 {
		fullName = fullName[:54]
	}

	return fullName + "-" + hashStr
}

func KubernetesCompatibleName(name string) string {
	name = strings.ToLower(name) // Ensure lowercase for consistency
	name = invalidChars.ReplaceAllString(name, "-")
	name = multipleHyphens.ReplaceAllString(name, "-")
	name = strings.Trim(name, "-")

	return name
}

func DiscoveryJobName(in *api.RenovateJob) string {
	baseName := in.Name
	baseName = KubernetesCompatibleName(baseName)

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

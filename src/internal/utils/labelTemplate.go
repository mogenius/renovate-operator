package utils

import (
	"encoding/json"
	"regexp"
	"renovate-operator/config"
	"strings"
)

var (
	invalidLabelValueChars  = regexp.MustCompile(`[^A-Za-z0-9_.-]+`)
	repeatedLabelSeparators = regexp.MustCompile(`[-_.]{2,}`)
)

// ConfiguredPodLabels renders the operator-wide POD_LABEL_TEMPLATES config (a JSON
// object mapping label key to template string) against the given job values,
// substituting {job}, {project}, {jobType} and {namespace} placeholders.
func ConfiguredPodLabels(job, project, jobType, namespace string) map[string]string {
	raw := config.GetValue("POD_LABEL_TEMPLATES")
	if raw == "" || raw == "{}" {
		return nil
	}

	var templates map[string]string
	if err := json.Unmarshal([]byte(raw), &templates); err != nil || len(templates) == 0 {
		return nil
	}

	replacer := strings.NewReplacer(
		"{job}", job,
		"{project}", project,
		"{jobType}", jobType,
		"{namespace}", namespace,
	)

	rendered := make(map[string]string, len(templates))
	for key, tmpl := range templates {
		rendered[key] = sanitizeLabelValue(replacer.Replace(tmpl))
	}
	return rendered
}

// sanitizeLabelValue turns a rendered template into a valid Kubernetes label value.
// Unlike KubernetesCompatibleName (resource names: lowercase, hyphen-only), label
// values permit mixed case, "_" and ".", so it uses its own charset but shares the
// same collapse/trim logic - see collapseSeparators.
func sanitizeLabelValue(value string) string {
	value = collapseSeparators(value, invalidLabelValueChars, repeatedLabelSeparators, "-_.")
	if len(value) > 63 {
		value = strings.Trim(value[:63], "-_.")
	}
	return value
}

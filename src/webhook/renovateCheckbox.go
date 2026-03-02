package webhook

import "strings"

// isRenovateContent checks if the description is from Renovate (either MR/PR or Dependency Dashboard).
// It relies on HTML comment markers that Renovate embeds for its own checkbox-parsing logic,
// rather than user-configurable text like dependencyDashboardHeader or prFooter which can be
// overridden in renovate.json.
func isRenovateContent(description string) bool {
	if description == "" {
		return false
	}

	patternList := []string{
		"## Detected Dependencies",
		"<!-- rebase-check -->",
		"<!--renovate-debug:",
		"<!-- rebase-all-open-prs -->",
		"<!-- rebase-branch=",
		"<!-- approve-all-pending-prs -->",
		"<!-- approvePr-branch=",
		"<!-- approve-branch=",
		"<!-- recreate-branch=",
		"<!-- unschedule-branch=",
		"<!-- create-config-migration-pr -->",
		"<!-- create-all-awaiting-schedule-prs -->",
		"<!-- create-all-rate-limited-prs -->",
		"<!-- unlimit-branch=",
		"<!-- manual job -->",
	}

	for _, pattern := range patternList {
		if strings.Contains(description, pattern) {
			return true
		}
	}
	return false
}

// hasCheckboxBeenChecked checks if there's a checked Renovate checkbox in the current description
func hasCheckboxBeenChecked(current string) bool {
	if current == "" {
		return false
	}

	return strings.Contains(current, "- [x]") ||
		strings.Contains(current, "- [X]")
}

// verifyRenovateDescriptionChange verifies if the description change is from Renovate and has a checked checkbox
func verifyRenovateDescriptionChange(current string) bool {
	// Verify it's Renovate content
	if !isRenovateContent(current) {
		return false
	}

	// Verify a checkbox was checked
	return hasCheckboxBeenChecked(current)
}

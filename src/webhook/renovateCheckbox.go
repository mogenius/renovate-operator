package webhook

import "strings"

// isRenovateContent checks if the description is from Renovate (either MR or Dependency Dashboard)
func isRenovateContent(description string) bool {
	if description == "" {
		return false
	}

	patternList := []string{
		"## Detected Dependencies",
		"<!-- rebase-check -->",
		"<!-- rebase-all-open-prs -->",
		"<!-- rebase-branch=",
		"<!-- approve-all-pending-prs -->",
		"<!-- approvePr-branch=",
		"<!-- approve-branch",
		"<!-- create-config-migration-pr -->",
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

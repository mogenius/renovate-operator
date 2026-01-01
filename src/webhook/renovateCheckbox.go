package webhook

import "strings"

// isRenovateContent checks if the description is from Renovate (either MR or Dependency Dashboard)
func isRenovateContent(description string) bool {
	if description == "" {
		return false
	}

	// merge requests created by Renovate contain these specific comments
	if strings.Contains(description, "<!-- rebase-check -->If you want to rebase/retry this") {
		return true
	}

	// Dependency Dashboards created by Renovate contain this specific comment
	if strings.Contains(description, "<!-- rebase-all-open-prs -->**Click on this checkbox to rebase all") {
		return true
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

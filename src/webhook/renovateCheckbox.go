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

	// Pull requests / merge requests created by Renovate embed a rebase-check comment
	// and a renovate-debug trailer.
	if strings.Contains(description, "<!-- rebase-check -->") {
		return true
	}
	if strings.Contains(description, "<!--renovate-debug:") {
		return true
	}

	// Dependency Dashboards use HTML comment markers on their interactive checkboxes.
	if strings.Contains(description, "<!-- manual job -->") {
		return true
	}
	if strings.Contains(description, "<!-- rebase-all-open-prs -->") {
		return true
	}
	if strings.Contains(description, "<!-- approve-all-pending-prs -->") {
		return true
	}
	if strings.Contains(description, "<!-- approvePr-branch=") {
		return true
	}
	if strings.Contains(description, "<!-- approve-branch=") {
		return true
	}
	if strings.Contains(description, "<!-- create-all-rate-limited-prs -->") {
		return true
	}
	if strings.Contains(description, "<!-- unlimit-branch=") {
		return true
	}
	if strings.Contains(description, "<!-- create-all-awaiting-schedule-prs -->") {
		return true
	}
	if strings.Contains(description, "<!-- unschedule-branch=") {
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

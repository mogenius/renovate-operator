package ui

import (
	"fmt"
	"regexp"
	api "renovate-operator/api/v1alpha1"
	"strings"
)

const maxBaseBranchLength = 250

// baseBranchCharacters is a conservative subset of git's check-ref-format rules.
// It also keeps Renovate's pattern syntax ("/regex/", "!negation") out of the value,
// so the branch passed to the run is always taken literally.
var baseBranchCharacters = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)

// validateExecutionOptions rejects per-run options that cannot be turned into a
// well-formed Renovate run. A nil value is valid and means "no overrides".
func validateExecutionOptions(executionOptions *api.RenovateExecutionOptions) error {
	if executionOptions == nil || executionOptions.BaseBranch == "" {
		return nil
	}
	return validateBaseBranch(executionOptions.BaseBranch)
}

func validateBaseBranch(branch string) error {
	if len(branch) > maxBaseBranchLength {
		return fmt.Errorf("baseBranch must not be longer than %d characters", maxBaseBranchLength)
	}
	if !baseBranchCharacters.MatchString(branch) {
		return fmt.Errorf("baseBranch must start with a letter or digit and contain only letters, digits, '.', '_', '-' and '/'")
	}
	for _, forbidden := range []string{"..", "//", "/.", ".lock/"} {
		if strings.Contains(branch, forbidden) {
			return fmt.Errorf("baseBranch must not contain %q", forbidden)
		}
	}
	for _, forbiddenSuffix := range []string{"/", ".", ".lock"} {
		if strings.HasSuffix(branch, forbiddenSuffix) {
			return fmt.Errorf("baseBranch must not end with %q", forbiddenSuffix)
		}
	}
	return nil
}

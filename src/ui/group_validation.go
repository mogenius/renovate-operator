package ui

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/go-logr/logr"
)

const (
	maxGroupNameLength = 256
	maxGroupsPerUser   = 100
)

var (
	// validGroupNamePattern allows alphanumeric, dash, underscore, @, dot, forward slash, comma, equals, space, colon
	// This covers most OIDC providers: Azure AD (including DN format and display names), Okta, Google, Keycloak, Forgejo, etc.
	validGroupNamePattern = regexp.MustCompile(`^[a-zA-Z0-9._@/,= :\-]+$`)
)

// validateGroupName performs basic format validation on a single group name.
// Prevents injection attacks and DoS via excessively long names.
func validateGroupName(name string) error {
	if len(name) == 0 {
		return fmt.Errorf("empty group name")
	}

	if len(name) > maxGroupNameLength {
		return fmt.Errorf("group name too long: %d characters (max: %d)", len(name), maxGroupNameLength)
	}

	if !validGroupNamePattern.MatchString(name) {
		return fmt.Errorf("invalid characters in group name (allowed: a-z A-Z 0-9 . _ @ / , = - : space)")
	}

	return nil
}

// sanitizeGroups performs LAYER 1 validation: basic format checking and DoS prevention.
// Filters out invalid groups and limits total count to prevent memory exhaustion.
func sanitizeGroups(groups []string, logger logr.Logger) []string {
	// Prevent memory DoS by limiting total groups
	if len(groups) > maxGroupsPerUser {
		logger.Info("User has excessive groups, truncating for safety",
			"original_count", len(groups),
			"limit", maxGroupsPerUser)
		groups = groups[:maxGroupsPerUser]
	}

	validated := make([]string, 0, len(groups))
	for _, group := range groups {
		// Trim whitespace before validation (formatting is not part of the group name)
		group = strings.TrimSpace(group)

		if err := validateGroupName(group); err != nil {
			logger.V(1).Info("Skipping invalid group from OIDC claim",
				"group", group,
				"error", err.Error())
			continue
		}
		validated = append(validated, group)
	}

	return validated
}

// normalizeGroups normalizes all groups by removing whitespace and using lowercase
func normalizeGroups(groups []string) []string {
	if groups == nil {
		return nil
	}
	normalized := make([]string, 0, len(groups))
	for _, group := range groups {
		normalized = append(normalized, strings.ToLower(strings.TrimSpace(group)))
	}
	return normalized
}

// normalizeAndValidateGroups performs LAYER 2 validation: normalization and deduplication.
// Handles case sensitivity, whitespace, and duplicate groups.
func normalizeAndValidateGroups(groups []string, logger logr.Logger) []string {
	seen := make(map[string]bool, len(groups))
	validated := make([]string, 0, len(groups))

	normalizedGroups := normalizeGroups(groups)

	for _, group := range normalizedGroups {

		if group == "" {
			continue
		}

		// Deduplicate (e.g., "Team-A" and "team-a" become one)
		if seen[group] {
			logger.V(2).Info("Skipping duplicate group", group)
			continue
		}

		seen[group] = true

		validated = append(validated, group)
	}

	return validated
}

// GroupFilterConfig defines optional filtering rules for LAYER 3 validation.
type GroupFilterConfig struct {
	// AllowedPrefix filters groups to only those starting with this prefix.
	// Example: "renovate-" only allows "renovate-team-a", "renovate-admin"
	AllowedPrefix string

	// AllowedPattern is a regex pattern that groups must match.
	// Example: "^(team-|platform-|admin-).*" allows team-*, platform-*, admin-*
	AllowedPattern *regexp.Regexp
}

// filterGroupsByPolicy performs LAYER 3 validation: policy-based filtering.
// Applies optional prefix and pattern restrictions for high-security environments.
func filterGroupsByPolicy(groups []string, config GroupFilterConfig, logger logr.Logger) []string {
	// If no policy configured, return all groups
	if config.AllowedPrefix == "" && config.AllowedPattern == nil {
		return groups
	}

	filtered := make([]string, 0, len(groups))

	for _, group := range groups {
		// Check prefix requirement
		if config.AllowedPrefix != "" {
			if !strings.HasPrefix(group, config.AllowedPrefix) {
				logger.V(2).Info("Filtering group without required prefix",
					"group", group,
					"required_prefix", config.AllowedPrefix)
				continue
			}
		}

		// Check pattern requirement
		if config.AllowedPattern != nil {
			if !config.AllowedPattern.MatchString(group) {
				logger.V(2).Info("Filtering group not matching pattern",
					"group", group,
					"pattern", config.AllowedPattern.String())
				continue
			}
		}

		filtered = append(filtered, group)
	}

	return filtered
}

// ValidateAndNormalizeGroups performs all 3 layers of group validation.
// Returns sanitized, normalized, and policy-filtered groups safe for authorization.
func ValidateAndNormalizeGroups(groups []string, config GroupFilterConfig, logger logr.Logger) []string {
	// LAYER 1: Basic sanitization - remove malformed groups, limit count
	sanitized := sanitizeGroups(groups, logger)

	// LAYER 2: Normalization - lowercase, trim, deduplicate
	normalized := normalizeAndValidateGroups(sanitized, logger)

	// LAYER 3: Policy filtering - apply prefix/pattern restrictions
	validated := filterGroupsByPolicy(normalized, config, logger)

	// Warn if all groups were filtered out (potential misconfiguration)
	if len(groups) > 0 && len(validated) == 0 {
		logger.Info("WARNING: All user groups filtered out by validation",
			"original_count", len(groups),
			"after_sanitization", len(sanitized),
			"after_normalization", len(normalized),
			"after_policy", len(validated))
	} else if len(groups) != len(validated) {
		logger.V(1).Info("Group validation filtered some groups",
			"original", len(groups),
			"final", len(validated))
	}

	return validated
}

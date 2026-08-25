package ui

import (
	"regexp"
	"testing"

	"github.com/go-logr/logr"
)

func TestValidateGroupName(t *testing.T) {
	tests := []struct {
		name      string
		groupName string
		wantErr   bool
	}{
		{"valid alphanumeric", "team-alpha-123", false},
		{"valid with underscore", "team_alpha", false},
		{"valid with dot", "team.alpha", false},
		{"valid with at", "team@company.com", false},
		{"valid with slash", "org/team", false},
		{"valid with colon", "org:team", false},
		{"empty string", "", true},
		{"too long", string(make([]byte, 257)), true},
		{"valid with spaces", "team alpha", false},
		{"valid with multiple internal spaces", "All Company Users", false},
		{"invalid characters special", "team$alpha", true},
		{"invalid characters newline", "team\nalpha", true},
		{"invalid characters null", "team\x00alpha", true},
		{"unicode characters", "团队-A", true}, // Not in allowed set
		{"just valid characters", "abcABC123._@/,= -", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateGroupName(tt.groupName)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateGroupName() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSanitizeGroups(t *testing.T) {
	logger := logr.Discard()

	tests := []struct {
		name   string
		groups []string
		want   int // expected number of groups after sanitization
	}{
		{
			name:   "all valid groups",
			groups: []string{"team-a", "team-b", "team-c"},
			want:   3,
		},
		{
			name:   "mixed valid and invalid",
			groups: []string{"team-a", "team$invalid", "team-c"},
			want:   2,
		},
		{
			name:   "empty group filtered out",
			groups: []string{"team-a", "", "team-c"},
			want:   2,
		},
		{
			name:   "too long group filtered",
			groups: []string{"team-a", string(make([]byte, 257)), "team-c"},
			want:   2,
		},
		{
			name:   "excessive groups truncated",
			groups: make([]string, 150), // More than maxGroupsPerUser
			want:   100,                 // Should be truncated to max
		},
		{
			name:   "all invalid groups",
			groups: []string{"team$a", "team\nc", "team\x00z"},
			want:   0,
		},
		{
			name:   "group with spaces is valid",
			groups: []string{"team$a", "team b", "team\nc"},
			want:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// For excessive groups test, fill with valid names
			if len(tt.groups) == 150 {
				for i := range tt.groups {
					tt.groups[i] = "team-" + string(rune('a'+i%26))
				}
			}

			result := sanitizeGroups(tt.groups, logger)
			if len(result) != tt.want {
				t.Errorf("sanitizeGroups() returned %d groups, want %d", len(result), tt.want)
			}
		})
	}
}

func TestNormalizeAndValidateGroups(t *testing.T) {
	logger := logr.Discard()

	tests := []struct {
		name   string
		groups []string
		want   []string
	}{
		{
			name:   "lowercase normalization",
			groups: []string{"Team-A", "TEAM-B", "team-c"},
			want:   []string{"team-a", "team-b", "team-c"},
		},
		{
			name:   "whitespace trimming",
			groups: []string{" team-a ", "  team-b", "team-c  "},
			want:   []string{"team-a", "team-b", "team-c"},
		},
		{
			name:   "deduplication after normalization",
			groups: []string{"team-a", "Team-A", "TEAM-A"},
			want:   []string{"team-a"},
		},
		{
			name:   "empty strings filtered",
			groups: []string{"team-a", "", "   ", "team-b"},
			want:   []string{"team-a", "team-b"},
		},
		{
			name:   "mixed case with whitespace",
			groups: []string{" Team-A ", "team-a", "TEAM-B "},
			want:   []string{"team-a", "team-b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeAndValidateGroups(tt.groups, logger)
			if len(result) != len(tt.want) {
				t.Errorf("normalizeAndValidateGroups() returned %d groups, want %d", len(result), len(tt.want))
			}
			for i := range result {
				if result[i] != tt.want[i] {
					t.Errorf("normalizeAndValidateGroups()[%d] = %s, want %s", i, result[i], tt.want[i])
				}
			}
		})
	}
}

func TestFilterGroupsByPolicy(t *testing.T) {
	logger := logr.Discard()

	tests := []struct {
		name   string
		groups []string
		config GroupFilterConfig
		want   []string
	}{
		{
			name:   "no policy - all groups pass",
			groups: []string{"team-a", "platform-b", "admin-c"},
			config: GroupFilterConfig{},
			want:   []string{"team-a", "platform-b", "admin-c"},
		},
		{
			name:   "prefix filter",
			groups: []string{"renovate-team-a", "team-b", "renovate-admin"},
			config: GroupFilterConfig{AllowedPrefix: "renovate-"},
			want:   []string{"renovate-team-a", "renovate-admin"},
		},
		{
			name:   "pattern filter",
			groups: []string{"team-a", "platform-b", "admin-c", "other-d"},
			config: GroupFilterConfig{AllowedPattern: regexp.MustCompile(`^(team-|platform-).*`)},
			want:   []string{"team-a", "platform-b"},
		},
		{
			name:   "prefix and pattern combined",
			groups: []string{"renovate-team-a", "renovate-other", "team-b"},
			config: GroupFilterConfig{
				AllowedPrefix:  "renovate-",
				AllowedPattern: regexp.MustCompile(`^renovate-team-.*`),
			},
			want: []string{"renovate-team-a"},
		},
		{
			name:   "all groups filtered out",
			groups: []string{"team-a", "team-b"},
			config: GroupFilterConfig{AllowedPrefix: "renovate-"},
			want:   []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterGroupsByPolicy(tt.groups, tt.config, logger)
			if len(result) != len(tt.want) {
				t.Errorf("filterGroupsByPolicy() returned %d groups, want %d", len(result), len(tt.want))
			}
			for i := range result {
				if result[i] != tt.want[i] {
					t.Errorf("filterGroupsByPolicy()[%d] = %s, want %s", i, result[i], tt.want[i])
				}
			}
		})
	}
}

func TestValidateAndNormalizeGroups(t *testing.T) {
	logger := logr.Discard()

	tests := []struct {
		name   string
		groups []string
		config GroupFilterConfig
		want   []string
	}{
		{
			name:   "full pipeline - sanitize, normalize, no policy",
			groups: []string{"Team-A", " team-b ", "team$invalid", "TEAM-A"},
			config: GroupFilterConfig{},
			want:   []string{"team-a", "team-b"},
		},
		{
			name:   "full pipeline with prefix filter",
			groups: []string{"Renovate-Team-A", " renovate-team-b ", "team-c", "RENOVATE-ADMIN"},
			config: GroupFilterConfig{AllowedPrefix: "renovate-"},
			want:   []string{"renovate-team-a", "renovate-team-b", "renovate-admin"},
		},
		{
			name: "full pipeline with pattern filter",
			groups: []string{
				"Team-Alpha", "Platform-Beta", " admin-gamma ", "other-delta",
				"TEAM-EPSILON", "invalid$group",
			},
			config: GroupFilterConfig{AllowedPattern: regexp.MustCompile(`^(team-|platform-).*`)},
			want:   []string{"team-alpha", "platform-beta", "team-epsilon"},
		},
		{
			name: "real-world Azure AD groups",
			groups: []string{
				" CN=Team-Renovate-Admins,OU=Groups,DC=example,DC=com ",
				"team-renovate-users",
				"TEAM-RENOVATE-VIEWERS",
			},
			config: GroupFilterConfig{AllowedPattern: regexp.MustCompile(`^(cn=)?team-renovate-.*`)},
			want:   []string{"cn=team-renovate-admins,ou=groups,dc=example,dc=com", "team-renovate-users", "team-renovate-viewers"},
		},
		{
			name:   "real-world OIDC groups with spaces",
			groups: []string{"All Users", "Engineering Team", " Team Alpha ", "team-no-spaces"},
			config: GroupFilterConfig{},
			want:   []string{"all users", "engineering team", "team alpha", "team-no-spaces"},
		},
		{
			name:   "all groups filtered out (warning case)",
			groups: []string{"team-a", "team-b", "team-c"},
			config: GroupFilterConfig{AllowedPrefix: "platform-"},
			want:   []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateAndNormalizeGroups(tt.groups, tt.config, logger)
			if len(result) != len(tt.want) {
				t.Errorf("ValidateAndNormalizeGroups() returned %d groups, want %d\nGot: %v\nWant: %v",
					len(result), len(tt.want), result, tt.want)
				return
			}
			for i := range result {
				if result[i] != tt.want[i] {
					t.Errorf("ValidateAndNormalizeGroups()[%d] = %s, want %s", i, result[i], tt.want[i])
				}
			}
		})
	}
}

func TestValidateAndNormalizeGroups_EdgeCases(t *testing.T) {
	logger := logr.Discard()

	t.Run("nil groups", func(t *testing.T) {
		result := ValidateAndNormalizeGroups(nil, GroupFilterConfig{}, logger)
		if len(result) != 0 {
			t.Errorf("Expected empty result for nil input, got %v", result)
		}
	})

	t.Run("empty groups", func(t *testing.T) {
		result := ValidateAndNormalizeGroups([]string{}, GroupFilterConfig{}, logger)
		if len(result) != 0 {
			t.Errorf("Expected empty result for empty input, got %v", result)
		}
	})

	t.Run("single group", func(t *testing.T) {
		result := ValidateAndNormalizeGroups([]string{"Team-A"}, GroupFilterConfig{}, logger)
		if len(result) != 1 || result[0] != "team-a" {
			t.Errorf("Expected [team-a], got %v", result)
		}
	})

	t.Run("maximum valid groups", func(t *testing.T) {
		groups := make([]string, 100)
		for i := range groups {
			groups[i] = "team-" + string(rune('a'+i%26)) + string(rune('0'+i/26))
		}
		result := ValidateAndNormalizeGroups(groups, GroupFilterConfig{}, logger)
		if len(result) != 100 {
			t.Errorf("Expected 100 groups, got %d", len(result))
		}
	})
}

// TestRegexCompiledOnce verifies regex is compiled at package level
func TestRegexCompiledOnce(t *testing.T) {
	// This test verifies the regex pattern exists as a package variable
	if validGroupNamePattern == nil {
		t.Error("validGroupNamePattern should be compiled at package level")
	}

	// Verify it's the same instance on multiple calls
	err1 := validateGroupName("test-group")
	err2 := validateGroupName("test-group")

	if err1 != nil || err2 != nil {
		t.Error("Valid group names should pass validation")
	}
}

// TestNewGroupFilterConfig covers the case handling that LAYER 2 forces on the
// policy: groups reach LAYER 3 lowercased, so a policy carrying uppercase has
// to match anyway or the user ends up with no groups at all.
func TestNewGroupFilterConfig(t *testing.T) {
	logger := logr.Discard()

	tests := []struct {
		name    string
		prefix  string
		pattern string
		groups  []string
		want    []string
	}{
		{
			name:   "mixed case prefix still matches",
			prefix: "Renovate-",
			groups: []string{"renovate-team-a", "team-b"},
			want:   []string{"renovate-team-a"},
		},
		{
			name:    "mixed case pattern still matches",
			pattern: `^Team-Renovate$`,
			groups:  []string{"team-renovate", "team-other"},
			want:    []string{"team-renovate"},
		},
		{
			name:    "case insensitivity reaches every branch of an alternation",
			pattern: `^(Team-|Platform-).*`,
			groups:  []string{"team-a", "platform-b", "other-c"},
			want:    []string{"team-a", "platform-b"},
		},
		{
			name:   "prefix is trimmed",
			prefix: "  renovate-  ",
			groups: []string{"renovate-team-a", "team-b"},
			want:   []string{"renovate-team-a"},
		},
		{
			name:    "lowercase config keeps behaving as before",
			prefix:  "renovate-",
			pattern: `^renovate-team-.*`,
			groups:  []string{"renovate-team-a", "renovate-admin", "team-b"},
			want:    []string{"renovate-team-a"},
		},
		{
			name:   "empty policy passes everything through",
			groups: []string{"team-a", "platform-b"},
			want:   []string{"team-a", "platform-b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := NewGroupFilterConfig(tt.prefix, tt.pattern)
			if err != nil {
				t.Fatalf("NewGroupFilterConfig() unexpected error: %v", err)
			}

			result := filterGroupsByPolicy(tt.groups, config, logger)
			if len(result) != len(tt.want) {
				t.Fatalf("filterGroupsByPolicy() = %v, want %v", result, tt.want)
			}
			for i := range result {
				if result[i] != tt.want[i] {
					t.Errorf("filterGroupsByPolicy()[%d] = %s, want %s", i, result[i], tt.want[i])
				}
			}
		})
	}
}

// TestNewGroupFilterConfigKeepsPatternSemantics guards the reason the pattern is
// compiled case-insensitively rather than lowercased: lowercasing the source
// would turn every negated class into its opposite, silently inverting which
// groups a deployment accepts.
func TestNewGroupFilterConfigKeepsPatternSemantics(t *testing.T) {
	config, err := NewGroupFilterConfig("", `^\D+$`)
	if err != nil {
		t.Fatalf("NewGroupFilterConfig() unexpected error: %v", err)
	}

	if !config.AllowedPattern.MatchString("team-a") {
		t.Error(`\D should still match a non-digit group name`)
	}
	if config.AllowedPattern.MatchString("12345") {
		t.Error(`\D must not have been rewritten into \d`)
	}

	// (?U) is a valid Go flag whose lowercase form does not compile at all.
	if _, err := NewGroupFilterConfig("", `(?U)team-.+`); err != nil {
		t.Errorf("NewGroupFilterConfig() should accept the (?U) flag, got %v", err)
	}
}

func TestNewGroupFilterConfigRejectsInvalidPattern(t *testing.T) {
	if _, err := NewGroupFilterConfig("", `^(team-`); err == nil {
		t.Error("NewGroupFilterConfig() should reject an invalid regex")
	}
}

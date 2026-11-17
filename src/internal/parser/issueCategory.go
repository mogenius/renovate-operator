package parser

import (
	"regexp"

	api "renovate-operator/api/v1alpha1"
)

// issueCategoryRule maps the messages a pattern matches to a category.
type issueCategoryRule struct {
	category api.LogIssueCategory
	pattern  *regexp.Regexp
}

// issueCategoryRules are tried in order and the first match wins. The order resolves
// messages that fit several categories:
//   - deprecation before config, so "config option X is deprecated" is a deprecation
//   - platform before lookup, so "registry rate limit" is a platform limit, not a lookup failure
//
// The patterns match Renovate's own wording (see issueCategory_test.go for the messages they
// were written against). A message none of them matches is LogIssueCategoryOther.
var issueCategoryRules = []issueCategoryRule{
	{api.LogIssueCategoryDeprecation, regexp.MustCompile(`(?i)deprecat|no longer supported|will be removed`)},
	{api.LogIssueCategoryConfig, regexp.MustCompile(`(?i)\bconfig\b|preset|validation`)},
	{api.LogIssueCategoryPlatform, regexp.MustCompile(`(?i)rate.?limit|authenticat|unauthori[sz]ed|forbidden|\b40[13]\b|permission|not accessible|bad credentials|token`)},
	{api.LogIssueCategoryLookup, regexp.MustCompile(`(?i)look ?up|registry|datasource|package file`)},
	{api.LogIssueCategoryBranches, regexp.MustCompile(`(?i)branch|artifact|lock ?file|rebase|conflict|command failed|npm ERR`)},
}

// categorizeIssue returns the category of a warning or error message.
func categorizeIssue(message string) api.LogIssueCategory {
	if message == "" {
		return api.LogIssueCategoryOther
	}
	for _, rule := range issueCategoryRules {
		if rule.pattern.MatchString(message) {
			return rule.category
		}
	}
	return api.LogIssueCategoryOther
}

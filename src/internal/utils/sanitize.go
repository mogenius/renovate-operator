package utils

import (
	"regexp"
	"strings"
)

// collapseSeparators replaces any run of characters matched by invalidChars with "-",
// collapses runs of separatorChars down to a single "-", and trims separatorChars from
// both ends - shared by every k8s-name/label-value sanitizer so an empty templated
// segment (e.g. a missing placeholder) disappears cleanly instead of leaving a
// dangling "--" or trailing separator.
func collapseSeparators(s string, invalidChars, repeatedSeparators *regexp.Regexp, separatorChars string) string {
	s = invalidChars.ReplaceAllString(s, "-")
	s = repeatedSeparators.ReplaceAllString(s, "-")
	return strings.Trim(s, separatorChars)
}

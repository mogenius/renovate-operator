package parser

import (
	"fmt"
	"strings"
	"testing"

	api "renovate-operator/api/v1alpha1"
)

// The messages are Renovate's own wording where we have seen it in real runs; the rest are
// shaped like them. A new rule should add the message that motivated it here.
func TestCategorizeIssue(t *testing.T) {
	tests := []struct {
		message string
		want    api.LogIssueCategory
	}{
		// config
		{"Found renovate config warnings", api.LogIssueCategoryConfig},
		{"Config validation errors found", api.LogIssueCategoryConfig},
		{"Repository has invalid config", api.LogIssueCategoryConfig},
		{"Cannot find preset's package (github>acme/renovate-presets)", api.LogIssueCategoryConfig},
		// deprecation, including deprecations that name the config
		{"Updating multiple npm lock files is deprecated and support will be removed in future versions.", api.LogIssueCategoryDeprecation},
		{"Config option fileMatch is deprecated", api.LogIssueCategoryDeprecation},
		{"This datasource is no longer supported", api.LogIssueCategoryDeprecation},
		// platform and access, including registry rate limits
		{"Authentication failure", api.LogIssueCategoryPlatform},
		{"Repository is not accessible: 403 Forbidden", api.LogIssueCategoryPlatform},
		{"Response code 401 (Unauthorized)", api.LogIssueCategoryPlatform},
		{"Registry rate limit reached, retrying", api.LogIssueCategoryPlatform},
		{"GitHub rate-limit exceeded", api.LogIssueCategoryPlatform},
		{"Bad credentials", api.LogIssueCategoryPlatform},
		// lookup
		{"Package lookup failures", api.LogIssueCategoryLookup},
		{"Failed to look up npm package @acme/ui", api.LogIssueCategoryLookup},
		{"Dependency lookup failure for internal/tooling", api.LogIssueCategoryLookup},
		{"Package file has unsupported registry", api.LogIssueCategoryLookup},
		{"Datasource error", api.LogIssueCategoryLookup},
		// branches and artifacts
		{"Error updating branch: update failure", api.LogIssueCategoryBranches},
		{"Branch has been modified externally", api.LogIssueCategoryBranches},
		{"Artifact update error in package-lock.json", api.LogIssueCategoryBranches},
		{"Command failed: npm install --package-lock-only", api.LogIssueCategoryBranches},
		{"Cannot rebase: merge conflict", api.LogIssueCategoryBranches},
		// other
		{"Ignoring pinned dependency internal/base", api.LogIssueCategoryOther},
		{"Disk space error", api.LogIssueCategoryOther},
		{"", api.LogIssueCategoryOther},
	}
	for _, tt := range tests {
		t.Run(tt.message, func(t *testing.T) {
			if got := categorizeIssue(tt.message); got != tt.want {
				t.Errorf("categorizeIssue(%q) = %q, want %q", tt.message, got, tt.want)
			}
		})
	}
}

func TestCategorizeIssueIgnoresCase(t *testing.T) {
	if got := categorizeIssue("AUTHENTICATION FAILURE"); got != api.LogIssueCategoryPlatform {
		t.Errorf("got %q, want %q", got, api.LogIssueCategoryPlatform)
	}
}

func logLine(level int, msg string) string {
	return fmt.Sprintf(`{"level":%d,"msg":%q}`, level, msg)
}

func sumCategoryCounts(counts map[api.LogIssueCategory]int) int {
	total := 0
	for _, count := range counts {
		total += count
	}
	return total
}

func TestParseRenovateLogsCategorisesIssues(t *testing.T) {
	logs := strings.Join([]string{
		logLine(30, "Repository started"),
		logLine(40, "Package lookup failures"),
		logLine(40, "Package lookup failures"), // a duplicate is stored once but counted twice
		logLine(40, "Found renovate config warnings"),
		logLine(50, "Error updating branch: update failure"),
		logLine(60, "Authentication failure"), // fatal counts as an error
		logLine(40, ""),                       // counted, never stored
	}, "\n")

	logIssues := ParseRenovateLogs(logs).LogIssues
	if logIssues == nil {
		t.Fatal("LogIssues is nil")
	}

	wantCounts := map[api.LogIssueCategory]int{
		api.LogIssueCategoryLookup:   2,
		api.LogIssueCategoryConfig:   1,
		api.LogIssueCategoryBranches: 1,
		api.LogIssueCategoryPlatform: 1,
		api.LogIssueCategoryOther:    1,
	}
	if len(logIssues.CategoryCounts) != len(wantCounts) {
		t.Errorf("CategoryCounts = %v, want %v", logIssues.CategoryCounts, wantCounts)
	}
	for category, want := range wantCounts {
		if got := logIssues.CategoryCounts[category]; got != want {
			t.Errorf("CategoryCounts[%s] = %d, want %d", category, got, want)
		}
	}
	if sum, total := sumCategoryCounts(logIssues.CategoryCounts), logIssues.WarnCount+logIssues.ErrorCount; sum != total {
		t.Errorf("category counts add up to %d, want WarnCount + ErrorCount = %d", sum, total)
	}

	wantStored := map[string]api.LogIssueCategory{
		"Package lookup failures":               api.LogIssueCategoryLookup,
		"Found renovate config warnings":        api.LogIssueCategoryConfig,
		"Error updating branch: update failure": api.LogIssueCategoryBranches,
		"Authentication failure":                api.LogIssueCategoryPlatform,
	}
	if len(logIssues.Issues) != len(wantStored) {
		t.Fatalf("stored %d issues, want %d: %+v", len(logIssues.Issues), len(wantStored), logIssues.Issues)
	}
	for _, issue := range logIssues.Issues {
		if want, ok := wantStored[issue.Message]; !ok || issue.Category != want {
			t.Errorf("issue %q has category %q, want %q", issue.Message, issue.Category, want)
		}
	}
}

func TestParseRenovateLogsCategoryCountsCoverIssuesBeyondTheStoredOnes(t *testing.T) {
	var lines []string
	for i := 0; i < MaxLogIssues+5; i++ {
		lines = append(lines, logLine(40, fmt.Sprintf("Dependency lookup failure for vendor/module-%d", i)))
	}
	lines = append(lines, logLine(50, "Authentication failure"))

	logIssues := ParseRenovateLogs(strings.Join(lines, "\n")).LogIssues
	if !logIssues.Truncated || len(logIssues.Issues) != MaxLogIssues {
		t.Fatalf("expected the stored list to be cut at %d, got %d (truncated=%v)", MaxLogIssues, len(logIssues.Issues), logIssues.Truncated)
	}
	if got := logIssues.CategoryCounts[api.LogIssueCategoryLookup]; got != MaxLogIssues+5 {
		t.Errorf("lookup count = %d, want %d", got, MaxLogIssues+5)
	}
	// the platform error is past the stored list, and only the counts know about it
	if got := logIssues.CategoryCounts[api.LogIssueCategoryPlatform]; got != 1 {
		t.Errorf("platform count = %d, want 1", got)
	}
}

func TestParseRenovateLogsCategorisesTheWholeMessageBeforeCuttingIt(t *testing.T) {
	// the only word that gives the category away sits past the 256-character cut
	message := strings.Repeat("x", MaxIssueMessageLen+10) + " Authentication failure"

	logIssues := ParseRenovateLogs(logLine(50, message)).LogIssues
	if len(logIssues.Issues) != 1 {
		t.Fatalf("stored %d issues, want 1", len(logIssues.Issues))
	}
	if !strings.HasSuffix(logIssues.Issues[0].Message, "…") {
		t.Errorf("message was not cut: %q", logIssues.Issues[0].Message)
	}
	if got := logIssues.Issues[0].Category; got != api.LogIssueCategoryPlatform {
		t.Errorf("category = %q, want %q", got, api.LogIssueCategoryPlatform)
	}
}

func TestParseRenovateLogsLeavesCategoryCountsOutWithoutIssues(t *testing.T) {
	logIssues := ParseRenovateLogs(logLine(30, "Repository finished")).LogIssues
	if logIssues == nil {
		t.Fatal("LogIssues is nil")
	}
	if logIssues.CategoryCounts != nil {
		t.Errorf("CategoryCounts = %v, want nil so the status omits it", logIssues.CategoryCounts)
	}
}

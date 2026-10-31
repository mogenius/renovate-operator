package parser

import (
	"encoding/json"
	"sort"
	"strings"
)

// baseBranchEntry picks the fields that name the base branch a log line belongs to:
// Renovate tags every line of a branch's processing with "baseBranch", and at debug
// level logs the list it is about to process as {"msg":"baseBranches","baseBranches":[…]}.
type baseBranchEntry struct {
	Msg          string   `json:"msg"`
	BaseBranch   string   `json:"baseBranch"`
	BaseBranches []string `json:"baseBranches"`
}

// ProcessedBaseBranches returns the base branches a Renovate run worked on, sorted. It
// is empty when the log names none, for example when Renovate stopped before
// processing branches (onboarding, disabled repository) — callers must treat that as
// unknown rather than as "the requested branch was skipped".
func ProcessedBaseBranches(logs string) []string {
	seen := map[string]bool{}
	// strings.Lines has no line length limit; debug lines carrying whole configs can be
	// longer than bufio.Scanner's buffer, which would end the scan early
	for line := range strings.Lines(logs) {
		if !strings.Contains(line, "baseBranch") {
			continue
		}
		var entry baseBranchEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry.BaseBranch != "" {
			seen[entry.BaseBranch] = true
		}
		if entry.Msg == "baseBranches" {
			for _, branch := range entry.BaseBranches {
				seen[branch] = true
			}
		}
	}

	branches := make([]string, 0, len(seen))
	for branch := range seen {
		branches = append(branches, branch)
	}
	sort.Strings(branches)
	return branches
}

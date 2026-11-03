package renovate

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"renovate-operator/internal/parser"
)

// customBranchMismatchEntry is a log entry the operator appends to a custom branch run's
// log, in Renovate's JSON log format, so the logs page shows it next to Renovate's own.
type customBranchMismatchEntry struct {
	Name                  string   `json:"name"`
	Level                 int      `json:"level"`
	Msg                   string   `json:"msg"`
	RequestedBaseBranch   string   `json:"requestedBaseBranch"`
	ProcessedBaseBranches []string `json:"processedBaseBranches"`
	Time                  string   `json:"time"`
}

// checkCustomBaseBranch compares the base branch a custom branch run asked for with the
// branches Renovate processed. A repository whose own config sets baseBranchPatterns (or
// baseBranches) overrides the operator's value, so Renovate silently runs against the
// repository's usual branches instead. When that happened, it returns the log with a
// warning appended and the branches that were processed; otherwise it returns the log
// unchanged and nil. A log that names no base branch at all (e.g. onboarding) is unknown
// and not flagged.
func checkCustomBaseBranch(logs, requested string, now time.Time) (string, []string) {
	processed := parser.ProcessedBaseBranches(logs)
	if len(processed) == 0 || slices.Contains(processed, requested) {
		return logs, nil
	}

	entry, err := json.Marshal(customBranchMismatchEntry{
		Name:  "renovate-operator",
		Level: 40,
		Msg: fmt.Sprintf("Custom branch %q was not processed: Renovate ran against %s instead. "+
			"The repository's own config (baseBranchPatterns or baseBranches on its default branch) "+
			"overrides the branch this run asked for, so this result does not describe %q.",
			requested, strings.Join(processed, ", "), requested),
		RequestedBaseBranch:   requested,
		ProcessedBaseBranches: processed,
		Time:                  now.UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		// cannot happen for this struct; keep Renovate's log as it is
		return logs, processed
	}
	if logs != "" && !strings.HasSuffix(logs, "\n") {
		logs += "\n"
	}
	return logs + string(entry) + "\n", processed
}

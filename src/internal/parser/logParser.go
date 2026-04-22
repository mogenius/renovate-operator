package parser

import (
	"bufio"
	"encoding/json"
	"regexp"
	"sort"
	"strconv"
	"strings"

	api "renovate-operator/api/v1alpha1"

	"k8s.io/utils/ptr"
)

// MaxPRDetails is the maximum number of individual PR details stored to prevent CRD bloat.
const MaxPRDetails = 100

// MaxLogIssues is the maximum number of individual log issue messages stored.
const MaxLogIssues = 20

// MaxIssueMessageLen is the maximum length of a single issue message.
const MaxIssueMessageLen = 256

// LogParseResult contains the result of parsing Renovate logs.
type LogParseResult struct {
	HasIssues            bool            // true if any WARN (level 40) or ERROR (level 50) found
	RenovateResultStatus *string         // nil = unknown; "Disabled", "No Config", "Onboarding Closed", or raw result string
	PRActivity           *api.PRActivity // nil when logs are empty/unparseable, non-nil (possibly zero counts) when logs were parsed successfully
	LogIssues            *api.LogIssues  // nil when logs are empty/unparseable, non-nil when logs were parsed successfully
}

// renovateLogEntry represents a single line in Renovate's JSON log output.
type renovateLogEntry struct {
	Level int    `json:"level"`
	Msg   string `json:"msg"`
}

type repositoryFinishedEntry struct {
	Msg    string `json:"msg"`
	Result string `json:"result,omitempty"`
}

// prCreateUpdateEntry is a targeted partial-unmarshal struct for "Creating PR" / "Updating PR" messages.
type prCreateUpdateEntry struct {
	Msg    string `json:"msg"`
	Branch string `json:"branch"`
	Title  string `json:"title"`
}

// prUnchangedEntry is a targeted partial-unmarshal struct for "does not need updating" messages.
type prUnchangedEntry struct {
	Msg    string `json:"msg"`
	Branch string `json:"branch"`
}

// gitPushEntry is a targeted partial-unmarshal struct for "git push" messages containing PR URLs.
type gitPushEntry struct {
	Msg    string `json:"msg"`
	Branch string `json:"branch"`
	Result struct {
		RemoteMessages struct {
			All []string `json:"all"`
		} `json:"remoteMessages"`
	} `json:"result"`
}

// prCreatedEntry is a targeted partial-unmarshal struct for "PR created" / "PR automerged" messages (level 30)
// which contain the PR number and title after the PR is created on the forge.
type prCreatedEntry struct {
	Msg     string `json:"msg"`
	Branch  string `json:"branch"`
	PR      int    `json:"pr"`
	PRTitle string `json:"prTitle"`
}

// branchInfoItem represents a single branch in the "branches info extended" summary.
type branchInfoItem struct {
	BranchName string `json:"branchName"`
	PRNo       *int   `json:"prNo"` // pointer because null means no PR
	PRTitle    string `json:"prTitle"`
	Result     string `json:"result"`
}

// branchesInfoEntry is a targeted partial-unmarshal struct for "branches info extended" (level 20)
// which contains a complete list of all branches Renovate knows about, including skipped ones.
type branchesInfoEntry struct {
	Msg          string           `json:"msg"`
	BranchesInfo []branchInfoItem `json:"branchesInformation"`
}

var (
	// prURLRegex matches PR/MR URLs from GitHub (/pull/N), Forgejo (/pulls/N), and GitLab (/merge_requests/N).
	prURLRegex = regexp.MustCompile(`https?://[^\s"]+/(?:pulls|pull|merge_requests)/(\d+)`)
	// prNumberRegex extracts the PR number from "Pull Request #N does not need updating" messages.
	prNumberRegex = regexp.MustCompile(`Pull Request #(\d+)`)
	// actionOrder defines the sort priority for PR actions (lower = sorted first).
	actionOrder = map[api.PRAction]int{api.PRActionAutomerged: 0, api.PRActionCreated: 1, api.PRActionUpdated: 2, api.PRActionUnchanged: 3}
)

// ParseRenovateLogs parses Renovate JSON logs (NDJSON format) and detects warnings/errors,
// repository config status, and PR activity.
// Returns HasIssues=true if any log entry has level >= 40 (WARN or ERROR).
// Returns RenovateResultStatus based on the "Repository finished" result field.
// Returns PRActivity with counts and per-PR details extracted from log messages.
func ParseRenovateLogs(logs string) *LogParseResult {
	result := &LogParseResult{
		HasIssues: false,
	}

	if logs == "" {
		return result
	}

	// Per-branch map for accumulating PR details (last-write-wins for action)
	branchMap := make(map[string]*api.PRDetail)
	parsedAnyLine := false

	// Issue accumulation
	var warnCount, errorCount int
	var issues []api.LogIssue
	seenMessages := make(map[string]bool)
	issuesTruncated := false

	scanner := bufio.NewScanner(strings.NewReader(logs))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024) // 64KB initial, 1MB max
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var entry renovateLogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			// Line is not valid JSON, skip it
			continue
		}

		parsedAnyLine = true

		// Renovate log levels: 10=trace, 20=debug, 30=info, 40=warn, 50=error, 60=fatal
		if entry.Level >= 40 {
			result.HasIssues = true
			if entry.Level >= 50 {
				errorCount++
			} else {
				warnCount++
			}
			msg := entry.Msg
			if len(msg) > MaxIssueMessageLen {
				msg = msg[:MaxIssueMessageLen] + "…"
			}
			if msg != "" && !seenMessages[msg] {
				seenMessages[msg] = true
				if len(issues) < MaxLogIssues {
					issues = append(issues, api.LogIssue{Level: entry.Level, Message: msg})
				} else {
					issuesTruncated = true
				}
			}
		}

		// Dispatch by message type (mutually exclusive)
		switch {
		case entry.Msg == "Repository finished":
			var finished repositoryFinishedEntry
			if err := json.Unmarshal([]byte(line), &finished); err == nil {
				switch finished.Result {
				case "disabled-by-config":
					result.RenovateResultStatus = ptr.To("Disabled")
				case "disabled-closed-onboarding":
					result.RenovateResultStatus = ptr.To("Onboarding Closed")
				case "disabled-no-config":
					result.RenovateResultStatus = ptr.To("No Config")
				default:
					if finished.Result == "" {
						result.RenovateResultStatus = ptr.To("Unknown")
					} else {
						result.RenovateResultStatus = ptr.To(finished.Result)
					}
				}
			}

		case entry.Msg == "Creating PR":
			var pr prCreateUpdateEntry
			if err := json.Unmarshal([]byte(line), &pr); err == nil && pr.Branch != "" {
				detail := getOrCreateDetail(branchMap, pr.Branch)
				detail.Action = api.PRActionCreated
				detail.Title = pr.Title
			}

		case entry.Msg == "Updating PR":
			var pr prCreateUpdateEntry
			if err := json.Unmarshal([]byte(line), &pr); err == nil && pr.Branch != "" {
				detail := getOrCreateDetail(branchMap, pr.Branch)
				detail.Action = api.PRActionUpdated
				detail.Title = pr.Title
			}

		case strings.Contains(entry.Msg, "does not need updating"):
			if matches := prNumberRegex.FindStringSubmatch(entry.Msg); len(matches) == 2 {
				if num, err := strconv.Atoi(matches[1]); err == nil {
					var unch prUnchangedEntry
					if err := json.Unmarshal([]byte(line), &unch); err == nil && unch.Branch != "" {
						detail := getOrCreateDetail(branchMap, unch.Branch)
						detail.Action = api.PRActionUnchanged
						detail.Number = num
					}
				}
			}

		case entry.Msg == "git push":
			var gp gitPushEntry
			if err := json.Unmarshal([]byte(line), &gp); err == nil && gp.Branch != "" {
				for _, msg := range gp.Result.RemoteMessages.All {
					if matches := prURLRegex.FindStringSubmatch(msg); len(matches) == 2 {
						detail := getOrCreateDetail(branchMap, gp.Branch)
						if num, err := strconv.Atoi(matches[1]); err == nil {
							detail.Number = num
						}
						break
					}
				}
			}

		case entry.Msg == "PR created":
			var pc prCreatedEntry
			if err := json.Unmarshal([]byte(line), &pc); err == nil && pc.Branch != "" && pc.PR > 0 {
				detail := getOrCreateDetail(branchMap, pc.Branch)
				detail.Number = pc.PR
				if detail.Title == "" && pc.PRTitle != "" {
					detail.Title = pc.PRTitle
				}
			}

		case entry.Msg == "PR automerged":
			var pc prCreatedEntry
			if err := json.Unmarshal([]byte(line), &pc); err == nil && pc.Branch != "" {
				detail := getOrCreateDetail(branchMap, pc.Branch)
				detail.Action = api.PRActionAutomerged
				if pc.PR > 0 {
					detail.Number = pc.PR
				}
				if detail.Title == "" && pc.PRTitle != "" {
					detail.Title = pc.PRTitle
				}
			}

		case entry.Msg == "branches info extended":
			// Complete list of all branches, including those skipped in this run.
			// Backfills branches not seen in per-message parsing.
			// Skip branches with result="already-existed" (stale branches with closed/merged PRs).
			var bi branchesInfoEntry
			if err := json.Unmarshal([]byte(line), &bi); err == nil {
				for _, b := range bi.BranchesInfo {
					if b.BranchName == "" {
						continue
					}
					// Always backfill title for branches already captured by per-message parsing,
					// regardless of result value (the branch is already in our map, we just want the title).
					if existing, exists := branchMap[b.BranchName]; exists {
						if existing.Title == "" && b.PRTitle != "" {
							existing.Title = b.PRTitle
						}
						continue
					}
					// For new branches not yet in the map, only include those actively processed.
					// Skip stale (already-existed), not-scheduled, and other non-active results.
					if b.Result != "done" && b.Result != "automerged" && b.Result != "" {
						continue
					}
					detail := getOrCreateDetail(branchMap, b.BranchName)
					detail.Action = api.PRActionUnchanged
					detail.Title = b.PRTitle
					if b.PRNo != nil {
						detail.Number = *b.PRNo
					}
				}
			}
		}
	}

	// Build PRActivity and LogIssues if we parsed any log lines
	if parsedAnyLine {
		result.PRActivity = buildPRActivity(branchMap)
		result.LogIssues = &api.LogIssues{
			WarnCount:  warnCount,
			ErrorCount: errorCount,
			Issues:     issues,
			Truncated:  issuesTruncated,
		}
	}

	return result
}

// getOrCreateDetail returns the PRDetail for a branch, creating it if needed.
func getOrCreateDetail(m map[string]*api.PRDetail, branch string) *api.PRDetail {
	if d, ok := m[branch]; ok {
		return d
	}
	d := &api.PRDetail{Branch: branch}
	m[branch] = d
	return d
}

// buildPRActivity collapses the branch map into counts and a capped PRDetail slice.
func buildPRActivity(branchMap map[string]*api.PRDetail) *api.PRActivity {
	activity := &api.PRActivity{}

	if len(branchMap) == 0 {
		return activity
	}

	// Default action for branches that only appeared in "git push" or "PR created" messages
	for _, detail := range branchMap {
		if detail.Action == "" && detail.Number > 0 {
			detail.Action = api.PRActionUpdated
		}
	}

	// Count actions across all branches (before truncation)
	for _, detail := range branchMap {
		switch detail.Action {
		case api.PRActionAutomerged:
			activity.Automerged++
		case api.PRActionCreated:
			activity.Created++
		case api.PRActionUpdated:
			activity.Updated++
		case api.PRActionUnchanged:
			activity.Unchanged++
		}
	}

	// Collect all PRDetails, sorted by action priority (automerged > created > updated > unchanged), then branch name
	prs := make([]api.PRDetail, 0, len(branchMap))
	for _, detail := range branchMap {
		prs = append(prs, *detail)
	}
	sort.Slice(prs, func(i, j int) bool {
		oi, oj := actionOrder[prs[i].Action], actionOrder[prs[j].Action]
		if oi != oj {
			return oi < oj
		}
		return prs[i].Branch < prs[j].Branch
	})

	// Cap at MaxPRDetails
	if len(prs) > MaxPRDetails {
		prs = prs[:MaxPRDetails]
		activity.Truncated = true
	}

	activity.PRs = prs
	return activity
}

package parser

import (
	"bufio"
	"encoding/json"
	"strings"

	"k8s.io/utils/ptr"
)

// LogParseResult contains the result of parsing Renovate logs
type LogParseResult struct {
	HasIssues            bool    // true if any WARN (level 40) or ERROR (level 50) found
	RenovateResultStatus *string // nil = unknown, true = config found, false = no config (onboarding detected)
}

// renovateLogEntry represents a single line in Renovate's JSON log output
type renovateLogEntry struct {
	Level int    `json:"level"`
	Msg   string `json:"msg"`
}

type repositoryFinishedEntry struct {
	Msg    string `json:"msg"`
	Result string `json:"result,omitempty"`
}

// ParseRenovateLogs parses Renovate JSON logs (NDJSON format) and detects warnings/errors
// and whether the repository has a Renovate config file.
// Returns HasIssues=true if any log entry has level >= 40 (WARN or ERROR).
// Returns RenovateResultStatus based on onboarding detection in log messages:
//   - "Disabled" if Renovate is disabled for the repository
//   - "No Config" if onboarding-related messages are found (repo has no config)
//   - nil if logs were parsed successfully without onboarding signals or logs are empty/not parseable
func ParseRenovateLogs(logs string) *LogParseResult {
	result := &LogParseResult{
		HasIssues: false,
	}

	if logs == "" {
		return result
	}

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

		// Renovate log levels: 10=trace, 20=debug, 30=info, 40=warn, 50=error, 60=fatal
		if entry.Level >= 40 {
			result.HasIssues = true
		}

		// Parse the "Repository finished" line which has the definitive status
		if entry.Msg == "Repository finished" {
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
		}
	}

	return result
}

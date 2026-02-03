package parser

import (
	"bufio"
	"encoding/json"
	"strings"
)

// LogParseResult contains the result of parsing Renovate logs
type LogParseResult struct {
	HasIssues bool // true if any WARN (level 40) or ERROR (level 50) found
}

// renovateLogEntry represents a single line in Renovate's JSON log output
type renovateLogEntry struct {
	Level int `json:"level"`
}

// ParseRenovateLogs parses Renovate JSON logs (NDJSON format) and detects warnings/errors.
// Returns HasIssues=true if any log entry has level >= 40 (WARN or ERROR).
// If logs are not in JSON format or empty, returns HasIssues=false.
func ParseRenovateLogs(logs string) *LogParseResult {
	result := &LogParseResult{
		HasIssues: false,
	}

	if logs == "" {
		return result
	}

	scanner := bufio.NewScanner(strings.NewReader(logs))
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
			return result // Early return, we found at least one issue
		}
	}

	return result
}

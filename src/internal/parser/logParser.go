package parser

import (
	"bufio"
	"encoding/json"
	"strings"
)

// LogParseResult contains the result of parsing Renovate logs
type LogParseResult struct {
	HasIssues         bool  // true if any WARN (level 40) or ERROR (level 50) found
	HasRenovateConfig *bool // nil = unknown, true = config found, false = no config (onboarding detected)
}

// renovateLogEntry represents a single line in Renovate's JSON log output
type renovateLogEntry struct {
	Level int    `json:"level"`
	Msg   string `json:"msg"`
}

// ParseRenovateLogs parses Renovate JSON logs (NDJSON format) and detects warnings/errors
// and whether the repository has a Renovate config file.
// Returns HasIssues=true if any log entry has level >= 40 (WARN or ERROR).
// Returns HasRenovateConfig based on onboarding detection in log messages:
//   - false if onboarding-related messages are found (repo has no config)
//   - true if logs were parsed successfully without onboarding signals
//   - nil if logs are empty or not parseable
func ParseRenovateLogs(logs string) *LogParseResult {
	result := &LogParseResult{
		HasIssues: false,
	}

	if logs == "" {
		return result
	}

	hasValidEntries := false
	onboardingDetected := false

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

		hasValidEntries = true

		// Renovate log levels: 10=trace, 20=debug, 30=info, 40=warn, 50=error, 60=fatal
		if entry.Level >= 40 {
			result.HasIssues = true
		}

		// Detect onboarding signals - Renovate logs these when no config file is found
		if !onboardingDetected && entry.Msg != "" {
			msgLower := strings.ToLower(entry.Msg)
			if strings.Contains(msgLower, "onboarding") {
				onboardingDetected = true
			}
		}
	}

	// Fallback: raw string search for onboarding signals.
	// The line-by-line scanner may miss signals when log lines exceed the
	// default buffer size (e.g. the "Repository finished" line with large stats).
	// Renovate's "Repository finished" entry contains "onboarded":false when no config exists.
	if !onboardingDetected {
		if strings.Contains(logs, `"onboarded":false`) {
			onboardingDetected = true
			hasValidEntries = true
		}
	}

	// Determine config status based on parsed logs
	if hasValidEntries {
		hasConfig := !onboardingDetected
		result.HasRenovateConfig = &hasConfig
	}

	return result
}

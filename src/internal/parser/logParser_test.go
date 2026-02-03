package parser

import (
	"testing"
)

func TestParseRenovateLogs(t *testing.T) {
	tests := []struct {
		name      string
		logs      string
		wantIssue bool
	}{
		{
			name:      "empty logs",
			logs:      "",
			wantIssue: false,
		},
		{
			name:      "only info level logs",
			logs:      `{"level":30,"msg":"Repository started"}` + "\n" + `{"level":30,"msg":"Dependency extraction complete"}`,
			wantIssue: false,
		},
		{
			name:      "debug level logs",
			logs:      `{"level":20,"msg":"Some debug message"}` + "\n" + `{"level":10,"msg":"Trace message"}`,
			wantIssue: false,
		},
		{
			name:      "warning level log",
			logs:      `{"level":30,"msg":"Info message"}` + "\n" + `{"level":40,"msg":"Warning: config validation issue"}`,
			wantIssue: true,
		},
		{
			name:      "error level log",
			logs:      `{"level":30,"msg":"Info message"}` + "\n" + `{"level":50,"msg":"Error: failed to lookup dependency"}`,
			wantIssue: true,
		},
		{
			name:      "fatal level log",
			logs:      `{"level":60,"msg":"Fatal error occurred"}`,
			wantIssue: true,
		},
		{
			name:      "mixed valid and invalid JSON lines",
			logs:      "some non-json output\n" + `{"level":30,"msg":"Info"}` + "\nnot json either",
			wantIssue: false,
		},
		{
			name:      "non-JSON logs only",
			logs:      "This is plain text output\nAnother line of text",
			wantIssue: false,
		},
		{
			name:      "JSON without level field",
			logs:      `{"msg":"No level field"}` + "\n" + `{"other":"field"}`,
			wantIssue: false,
		},
		{
			name:      "real world example with warning",
			logs:      `{"level":30,"time":1706011234567,"msg":"Repository started","repository":"owner/repo"}` + "\n" + `{"level":40,"time":1706011234568,"msg":"Configuration validation warning","repository":"owner/repo"}` + "\n" + `{"level":30,"time":1706011234569,"msg":"Repository finished","repository":"owner/repo"}`,
			wantIssue: true,
		},
		{
			name:      "level exactly 40 (boundary)",
			logs:      `{"level":40,"msg":"Warning level"}`,
			wantIssue: true,
		},
		{
			name:      "level exactly 39 (below boundary)",
			logs:      `{"level":39,"msg":"Just below warning"}`,
			wantIssue: false,
		},
		{
			name:      "empty lines in logs",
			logs:      "\n\n" + `{"level":30,"msg":"Info"}` + "\n\n",
			wantIssue: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseRenovateLogs(tt.logs)
			if result.HasIssues != tt.wantIssue {
				t.Errorf("ParseRenovateLogs() HasIssues = %v, want %v", result.HasIssues, tt.wantIssue)
			}
		})
	}
}

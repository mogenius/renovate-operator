package parser

import (
	"strings"
	"testing"

	"k8s.io/utils/ptr"
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
		{
			name:      "onboarded but disabled",
			logs:      `{"level":30, "repository":"k8s/adguard-home", "result":"disabled-by-config", "status":"disabled", "enabled":false, "msg":"Repository finished"}`,
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

func TestParseRenovateLogsConfigDetection(t *testing.T) {
	tests := []struct {
		name         string
		logs         string
		configStatus *string
	}{
		{
			name:         "empty logs - unknown config status",
			logs:         "",
			configStatus: nil,
		},
		{
			name:         "non-JSON logs only - unknown config status",
			logs:         "This is plain text output\nAnother line of text",
			configStatus: nil,
		},
		{
			name:         "normal run without onboarding - has config",
			logs:         `{"level":30,"msg":"Repository started"}` + "\n" + `{"level":30,"msg":"Dependency extraction complete"}` + "\n" + `{"level":30,"result":"done","onboarded":true,"msg":"Repository finished"}`,
			configStatus: ptr.To("done"),
		},
		{
			name:         "onboarding detected - no config",
			logs:         `{"level":30,"msg":"Repository started"}` + "\n" + `{"level":30,"msg":"Onboarding PR is needed"}` + "\n" + `{"level":30,"result":"disabled-no-config","onboarded":false,"msg":"Repository finished"}`,
			configStatus: ptr.To("No Config"),
		},
		{
			name:         "onboarding case insensitive",
			logs:         `{"level":30,"msg":"Repository started"}` + "\n" + `{"level":30,"msg":"ONBOARDING branch created"}` + "\n" + `{"level":30,"result":"disabled-no-config","onboarded":false,"msg":"Repository finished"}`,
			configStatus: ptr.To("No Config"),
		},
		{
			name:         "onboarding in mixed case message",
			logs:         `{"level":30,"msg":"Ensuring onboarding PR"}` + "\n" + `{"level":30,"result":"disabled-closed-onboarding","onboarded":false,"msg":"Repository finished"}`,
			configStatus: ptr.To("Onboarding Closed"),
		},
		{
			name:         "onboarding with warning - no config and has issues",
			logs:         `{"level":30,"msg":"Repository started"}` + "\n" + `{"level":40,"msg":"Onboarding PR needs update"}` + "\n" + `{"level":30,"result":"disabled-no-config","onboarded":false,"msg":"Repository finished"}`,
			configStatus: ptr.To("No Config"),
		},
		{
			name:         "run with warnings but no onboarding - has config",
			logs:         `{"level":30,"msg":"Repository started"}` + "\n" + `{"level":40,"msg":"Dependency lookup failed"}` + "\n" + `{"level":30,"result":"done","onboarded":true,"msg":"Repository finished"}`,
			configStatus: ptr.To("done"),
		},
		{
			name:         "onboarded false in Repository finished line - no config",
			logs:         `{"level":30,"msg":"Repository started"}` + "\n" + `{"level":30,"msg":"Repository finished","result":"disabled-no-config","onboarded":false,"status":"onboarding"}`,
			configStatus: ptr.To("No Config"),
		},
		{
			name:         "onboarded false detected via raw fallback when line exceeds scanner buffer",
			logs:         `{"level":30,"msg":"Repository started"}` + "\n" + `{"level":30,"msg":"Onboarding PR updated"}` + "\n" + `{"level":30,"cloned":true,"result":"disabled-no-config","onboarded":false,"msg":"Repository finished"}`,
			configStatus: ptr.To("No Config"),
		},
		{
			name:         "real world: onboarded false with scanner-breaking stats line",
			logs:         `{"level":30,"msg":"Repository started"}` + "\n" + `{"level":30,"msg":"stats","stats":{"data":"` + strings.Repeat("x", 70000) + `"}}` + "\n" + `{"level":30,"cloned":true,"result":"disabled-no-config","onboarded":false,"msg":"Repository finished"}`,
			configStatus: ptr.To("No Config"),
		},
		{
			name:         "onboarded but disabled",
			logs:         `{"level":30, "repository":"k8s/adguard-home", "result":"disabled-by-config", "status":"disabled", "enabled":false, "msg":"Repository finished"}`,
			configStatus: ptr.To("Disabled"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseRenovateLogs(tt.logs)
			if tt.configStatus == nil {
				if result.RenovateResultStatus != nil {
					t.Errorf("ParseRenovateLogs() RenovateResultStatus = %v, want nil", *result.RenovateResultStatus)
				}
			} else {
				if result.RenovateResultStatus == nil {
					t.Errorf("ParseRenovateLogs() RenovateResultStatus = nil, want %v", *tt.configStatus)
				} else if *result.RenovateResultStatus != *tt.configStatus {
					t.Errorf("ParseRenovateLogs() RenovateResultStatus = %v, want %v", *result.RenovateResultStatus, *tt.configStatus)
				}
			}
		})
	}
}

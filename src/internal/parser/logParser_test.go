package parser

import (
	"strings"
	"testing"
)

func boolPtr(b bool) *bool {
	return &b
}

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

func TestParseRenovateLogsConfigDetection(t *testing.T) {
	tests := []struct {
		name          string
		logs          string
		wantHasConfig *bool
	}{
		{
			name:          "empty logs - unknown config status",
			logs:          "",
			wantHasConfig: nil,
		},
		{
			name:          "non-JSON logs only - unknown config status",
			logs:          "This is plain text output\nAnother line of text",
			wantHasConfig: nil,
		},
		{
			name:          "normal run without onboarding - has config",
			logs:          `{"level":30,"msg":"Repository started"}` + "\n" + `{"level":30,"msg":"Dependency extraction complete"}` + "\n" + `{"level":30,"result":"done","onboarded":true,"msg":"Repository finished"}`,
			wantHasConfig: boolPtr(true),
		},
		{
			name:          "onboarding detected - no config",
			logs:          `{"level":30,"msg":"Repository started"}` + "\n" + `{"level":30,"msg":"Onboarding PR is needed"}` + "\n" + `{"level":30,"result":"done","onboarded":false,"msg":"Repository finished"}`,
			wantHasConfig: boolPtr(false),
		},
		{
			name:          "onboarding case insensitive",
			logs:          `{"level":30,"msg":"Repository started"}` + "\n" + `{"level":30,"msg":"ONBOARDING branch created"}` + "\n" + `{"level":30,"result":"done","onboarded":false,"msg":"Repository finished"}`,
			wantHasConfig: boolPtr(false),
		},
		{
			name:          "onboarding in mixed case message",
			logs:          `{"level":30,"msg":"Ensuring onboarding PR"}` + "\n" + `{"level":30,"result":"done","onboarded":false,"msg":"Repository finished"}`,
			wantHasConfig: boolPtr(false),
		},
		{
			name:          "onboarding with warning - no config and has issues",
			logs:          `{"level":30,"msg":"Repository started"}` + "\n" + `{"level":40,"msg":"Onboarding PR needs update"}` + "\n" + `{"level":30,"result":"done","onboarded":false,"msg":"Repository finished"}`,
			wantHasConfig: boolPtr(false),
		},
		{
			name:          "run with warnings but no onboarding - has config",
			logs:          `{"level":30,"msg":"Repository started"}` + "\n" + `{"level":40,"msg":"Dependency lookup failed"}` + "\n" + `{"level":30,"result":"done","onboarded":true,"msg":"Repository finished"}`,
			wantHasConfig: boolPtr(true),
		},
		{
			name:          "onboarded false in Repository finished line - no config",
			logs:          `{"level":30,"msg":"Repository started"}` + "\n" + `{"level":30,"msg":"Repository finished","onboarded":false,"status":"onboarding"}`,
			wantHasConfig: boolPtr(false),
		},
		{
			name:          "onboarded false detected via raw fallback when line exceeds scanner buffer",
			logs:          `{"level":30,"msg":"Repository started"}` + "\n" + `{"level":30,"msg":"Onboarding PR updated"}` + "\n" + `{"level":30,"cloned":true,"onboarded":false,"msg":"Repository finished"}`,
			wantHasConfig: boolPtr(false),
		},
		{
			name:          "real world: onboarded false with scanner-breaking stats line",
			logs:          `{"level":30,"msg":"Repository started"}` + "\n" + `{"level":30,"msg":"stats","stats":{"data":"` + strings.Repeat("x", 70000) + `"}}` + "\n" + `{"level":30,"cloned":true,"onboarded":false,"msg":"Repository finished"}`,
			wantHasConfig: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseRenovateLogs(tt.logs)
			if tt.wantHasConfig == nil {
				if result.HasRenovateConfig != nil {
					t.Errorf("ParseRenovateLogs() HasRenovateConfig = %v, want nil", *result.HasRenovateConfig)
				}
			} else {
				if result.HasRenovateConfig == nil {
					t.Errorf("ParseRenovateLogs() HasRenovateConfig = nil, want %v", *tt.wantHasConfig)
				} else if *result.HasRenovateConfig != *tt.wantHasConfig {
					t.Errorf("ParseRenovateLogs() HasRenovateConfig = %v, want %v", *result.HasRenovateConfig, *tt.wantHasConfig)
				}
			}
		})
	}
}

package parser

import (
	"fmt"
	"strings"
	"testing"

	api "renovate-operator/api/v1alpha1"

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

func TestParseRenovateLogsPRActivity(t *testing.T) {
	tests := []struct {
		name          string
		logs          string
		wantNil        bool // expect PRActivity == nil
		wantAutomerged int
		wantCreated    int
		wantUpdated    int
		wantUnchanged  int
		wantPRCount   int  // number of PRDetail entries
		wantTruncated bool
		checkDetails  func(t *testing.T, prs []api.PRDetail) // optional detailed assertions
	}{
		{
			name:    "empty logs - nil PRActivity",
			logs:    "",
			wantNil: true,
		},
		{
			name: "zero-PR run (clean scan with no PR messages)",
			logs: `{"level":30,"msg":"Repository started"}` + "\n" +
				`{"level":30,"msg":"Dependency extraction complete"}` + "\n" +
				`{"level":30,"msg":"Repository finished","result":"done"}`,
			wantNil:       false,
			wantCreated:   0,
			wantUpdated:   0,
			wantUnchanged: 0,
			wantPRCount:   0,
		},
		{
			name: "single PR created",
			logs: `{"level":30,"msg":"Creating PR","branch":"renovate/golang-1.x","title":"Update golang to v1.22"}`,
			wantCreated: 1,
			wantPRCount: 1,
			checkDetails: func(t *testing.T, prs []api.PRDetail) {
				if prs[0].Branch != "renovate/golang-1.x" {
					t.Errorf("branch = %q, want %q", prs[0].Branch, "renovate/golang-1.x")
				}
				if prs[0].Title != "Update golang to v1.22" {
					t.Errorf("title = %q, want %q", prs[0].Title, "Update golang to v1.22")
				}
				if prs[0].Action != api.PRActionCreated {
					t.Errorf("action = %q, want %q", prs[0].Action, api.PRActionCreated)
				}
			},
		},
		{
			name: "single PR updated",
			logs: `{"level":30,"msg":"Updating PR","branch":"renovate/react-18.x","title":"Update react to v18.3"}`,
			wantUpdated: 1,
			wantPRCount: 1,
			checkDetails: func(t *testing.T, prs []api.PRDetail) {
				if prs[0].Action != api.PRActionUpdated {
					t.Errorf("action = %q, want %q", prs[0].Action, api.PRActionUpdated)
				}
				if prs[0].Title != "Update react to v18.3" {
					t.Errorf("title = %q, want %q", prs[0].Title, "Update react to v18.3")
				}
			},
		},
		{
			name: "single PR unchanged",
			logs: `{"level":20,"msg":"Pull Request #101 does not need updating","branch":"renovate/rook-packages"}`,
			wantUnchanged: 1,
			wantPRCount:   1,
			checkDetails: func(t *testing.T, prs []api.PRDetail) {
				if prs[0].Action != api.PRActionUnchanged {
					t.Errorf("action = %q, want %q", prs[0].Action, api.PRActionUnchanged)
				}
				if prs[0].Number != 101 {
					t.Errorf("number = %d, want %d", prs[0].Number, 101)
				}
				if prs[0].Branch != "renovate/rook-packages" {
					t.Errorf("branch = %q, want %q", prs[0].Branch, "renovate/rook-packages")
				}
			},
		},
		{
			name: "mixed activity - multiple branches with different actions",
			logs: `{"level":30,"msg":"Creating PR","branch":"renovate/foo","title":"Update foo"}` + "\n" +
				`{"level":30,"msg":"Updating PR","branch":"renovate/bar","title":"Update bar"}` + "\n" +
				`{"level":20,"msg":"Pull Request #50 does not need updating","branch":"renovate/baz"}` + "\n" +
				`{"level":30,"msg":"Creating PR","branch":"renovate/qux","title":"Update qux"}`,
			wantCreated:   2,
			wantUpdated:   1,
			wantUnchanged: 1,
			wantPRCount:   4,
		},
		{
			name: "duplicate branch - last-write-wins",
			logs: `{"level":30,"msg":"Creating PR","branch":"renovate/dep-1.x","title":"Create dep v1"}` + "\n" +
				`{"level":30,"msg":"Updating PR","branch":"renovate/dep-1.x","title":"Update dep v1"}`,
			wantCreated: 0,
			wantUpdated: 1,
			wantPRCount: 1,
			checkDetails: func(t *testing.T, prs []api.PRDetail) {
				if prs[0].Action != api.PRActionUpdated {
					t.Errorf("action = %q, want %q (last-write-wins)", prs[0].Action, api.PRActionUpdated)
				}
				if prs[0].Title != "Update dep v1" {
					t.Errorf("title = %q, want %q", prs[0].Title, "Update dep v1")
				}
			},
		},
		{
			name: "git push extracts PR number - Forgejo format (/pulls/N)",
			logs: `{"level":30,"msg":"Creating PR","branch":"renovate/helm-3.x","title":"Update helm to v3.15"}` + "\n" +
				`{"level":20,"msg":"git push","branch":"renovate/helm-3.x","result":{"remoteMessages":{"all":["Visit the existing pull request:","https://git.example.com/org/repo/pulls/101 merges into main"]}}}`,
			wantCreated: 1,
			wantPRCount: 1,
			checkDetails: func(t *testing.T, prs []api.PRDetail) {
				if prs[0].Number != 101 {
					t.Errorf("number = %d, want %d", prs[0].Number, 101)
				}
				if prs[0].Title != "Update helm to v3.15" {
					t.Errorf("title = %q, want %q", prs[0].Title, "Update helm to v3.15")
				}
			},
		},
		{
			name: "git push extracts PR number - GitHub format (/pull/N)",
			logs: `{"level":20,"msg":"git push","branch":"renovate/lodash-4.x","result":{"remoteMessages":{"all":["Create a pull request:","https://github.com/org/repo/pull/42"]}}}`,
			wantUpdated: 1,
			wantPRCount: 1,
			checkDetails: func(t *testing.T, prs []api.PRDetail) {
				if prs[0].Number != 42 {
					t.Errorf("number = %d, want %d", prs[0].Number, 42)
				}
			},
		},
		{
			name: "git push extracts PR number - GitLab format (/merge_requests/N)",
			logs: `{"level":20,"msg":"git push","branch":"renovate/axios-1.x","result":{"remoteMessages":{"all":["To create a merge request:","https://gitlab.com/org/repo/-/merge_requests/99"]}}}`,
			wantUpdated: 1,
			wantPRCount: 1,
			checkDetails: func(t *testing.T, prs []api.PRDetail) {
				if prs[0].Number != 99 {
					t.Errorf("number = %d, want %d", prs[0].Number, 99)
				}
			},
		},
		{
			name: "partial data - PR with title but no number",
			logs: `{"level":30,"msg":"Creating PR","branch":"renovate/dep-a","title":"Update dep-a"}`,
			wantCreated: 1,
			wantPRCount: 1,
			checkDetails: func(t *testing.T, prs []api.PRDetail) {
				if prs[0].Title != "Update dep-a" {
					t.Errorf("title = %q, want %q", prs[0].Title, "Update dep-a")
				}
			},
		},
		{
			name: "partial data - PR with number but no title",
			logs: `{"level":20,"msg":"Pull Request #77 does not need updating","branch":"renovate/dep-b"}`,
			wantUnchanged: 1,
			wantPRCount:   1,
			checkDetails: func(t *testing.T, prs []api.PRDetail) {
				if prs[0].Number != 77 {
					t.Errorf("number = %d, want %d", prs[0].Number, 77)
				}
				if prs[0].Title != "" {
					t.Errorf("title = %q, want empty", prs[0].Title)
				}
			},
		},
		{
			name: "truncated logs (OOMKilled) - returns partial data",
			logs: `{"level":30,"msg":"Repository started"}` + "\n" +
				`{"level":30,"msg":"Creating PR","branch":"renovate/dep-1","title":"Update dep-1"}` + "\n" +
				`{"level":30,"msg":"Updating PR","branch":"renovate/dep-2","title":"Update dep-2"}`,
			// No "Repository finished" line - simulates truncated logs
			wantCreated: 1,
			wantUpdated: 1,
			wantPRCount: 2,
		},
		{
			name: "git push enriches existing branch entry with PR number",
			logs: `{"level":30,"msg":"Creating PR","branch":"renovate/dep-x","title":"Update dep-x"}` + "\n" +
				`{"level":20,"msg":"git push","branch":"renovate/dep-x","result":{"remoteMessages":{"all":["Visit:","https://git.example.com/org/repo/pulls/200 merges into main"]}}}`,
			wantCreated: 1,
			wantPRCount: 1,
			checkDetails: func(t *testing.T, prs []api.PRDetail) {
				if prs[0].Title != "Update dep-x" {
					t.Errorf("title = %q, want %q", prs[0].Title, "Update dep-x")
				}
				if prs[0].Number != 200 {
					t.Errorf("number = %d, want %d", prs[0].Number, 200)
				}
				if prs[0].Action != api.PRActionCreated {
					t.Errorf("action = %q, want %q", prs[0].Action, api.PRActionCreated)
				}
			},
		},
		{
			name:    "non-JSON logs only - nil PRActivity",
			logs:    "plain text output\nnot json",
			wantNil: true,
		},
		{
			name: "created and unchanged PRs all have numbers",
			logs: `{"level":30,"msg":"Creating PR","branch":"renovate/dep-a","title":"Update dep-a"}` + "\n" +
				`{"level":20,"msg":"git push","branch":"renovate/dep-a","result":{"remoteMessages":{"all":["Visit:","https://git.example.com/org/repo/pulls/100 merges into main"]}}}` + "\n" +
				`{"level":20,"msg":"Pull Request #200 does not need updating","branch":"renovate/dep-b"}` + "\n" +
				`{"level":20,"msg":"Pull Request #300 does not need updating","branch":"renovate/dep-c"}`,
			wantCreated:   1,
			wantUnchanged: 2,
			wantPRCount:   3,
			checkDetails: func(t *testing.T, prs []api.PRDetail) {
				prsByBranch := make(map[string]api.PRDetail)
				for _, pr := range prs {
					prsByBranch[pr.Branch] = pr
				}
				if prsByBranch["renovate/dep-a"].Number != 100 {
					t.Errorf("dep-a number = %d, want 100", prsByBranch["renovate/dep-a"].Number)
				}
				if prsByBranch["renovate/dep-b"].Number != 200 {
					t.Errorf("dep-b number = %d, want 200", prsByBranch["renovate/dep-b"].Number)
				}
				if prsByBranch["renovate/dep-c"].Number != 300 {
					t.Errorf("dep-c number = %d, want 300", prsByBranch["renovate/dep-c"].Number)
				}
			},
		},
		{
			name: "git push only (no Creating/Updating message) defaults to updated",
			logs: `{"level":20,"msg":"git push","branch":"renovate/dep-z","result":{"remoteMessages":{"all":["Visit the existing pull request:","https://forge.example.com/org/repo/pulls/555 merges into main"]}}}`,
			wantUpdated: 1,
			wantPRCount: 1,
			checkDetails: func(t *testing.T, prs []api.PRDetail) {
				if prs[0].Action != api.PRActionUpdated {
					t.Errorf("action = %q, want %q (default for git-push-only)", prs[0].Action, api.PRActionUpdated)
				}
				if prs[0].Number != 555 {
					t.Errorf("number = %d, want 555", prs[0].Number)
				}
			},
		},
		{
			name: "PR created message captures PR number for new PRs",
			logs: `{"level":30,"msg":"Creating PR","branch":"renovate/new-dep","title":"Update new-dep to v2.0"}` + "\n" +
				`{"level":20,"msg":"git push","branch":"renovate/new-dep","result":{"remoteMessages":{"all":["Create a new pull request:","https://forge.example.com/org/repo/compare/main...renovate/new-dep"]}}}` + "\n" +
				`{"level":30,"msg":"PR created","branch":"renovate/new-dep","pr":42,"prTitle":"Update new-dep to v2.0"}`,
			wantCreated: 1,
			wantPRCount: 1,
			checkDetails: func(t *testing.T, prs []api.PRDetail) {
				if prs[0].Action != api.PRActionCreated {
					t.Errorf("action = %q, want %q", prs[0].Action, api.PRActionCreated)
				}
				if prs[0].Number != 42 {
					t.Errorf("number = %d, want 42 (from PR created message)", prs[0].Number)
				}
				if prs[0].Title != "Update new-dep to v2.0" {
					t.Errorf("title = %q, want %q", prs[0].Title, "Update new-dep to v2.0")
				}
			},
		},
		{
			name: "PR automerged - captures action and PR number",
			logs: `{"level":30,"msg":"Creating PR","branch":"renovate/dep-auto","title":"Update dep-auto"}` + "\n" +
				`{"level":20,"msg":"git push","branch":"renovate/dep-auto","result":{"remoteMessages":{"all":["Create a new pull request:","https://forge.example.com/org/repo/compare/main...renovate/dep-auto"]}}}` + "\n" +
				`{"level":30,"msg":"PR created","branch":"renovate/dep-auto","pr":99}` + "\n" +
				`{"level":30,"msg":"PR automerged","branch":"renovate/dep-auto","pr":99}`,
			wantAutomerged: 1,
			wantPRCount:    1,
			checkDetails: func(t *testing.T, prs []api.PRDetail) {
				if prs[0].Action != api.PRActionAutomerged {
					t.Errorf("action = %q, want %q", prs[0].Action, api.PRActionAutomerged)
				}
				if prs[0].Number != 99 {
					t.Errorf("number = %d, want 99", prs[0].Number)
				}
			},
		},
		{
			name: "automerged sorted first, then created, then unchanged",
			logs: `{"level":20,"msg":"Pull Request #10 does not need updating","branch":"renovate/unchanged-dep"}` + "\n" +
				`{"level":30,"msg":"Creating PR","branch":"renovate/new-dep","title":"New dep"}` + "\n" +
				`{"level":30,"msg":"PR created","branch":"renovate/new-dep","pr":20}` + "\n" +
				`{"level":30,"msg":"Creating PR","branch":"renovate/auto-dep","title":"Auto dep"}` + "\n" +
				`{"level":30,"msg":"PR created","branch":"renovate/auto-dep","pr":30}` + "\n" +
				`{"level":30,"msg":"PR automerged","branch":"renovate/auto-dep","pr":30}`,
			wantAutomerged: 1,
			wantCreated:    1,
			wantUnchanged:  1,
			wantPRCount:    3,
			checkDetails: func(t *testing.T, prs []api.PRDetail) {
				if prs[0].Action != api.PRActionAutomerged {
					t.Errorf("first PR action = %q, want automerged", prs[0].Action)
				}
				if prs[1].Action != api.PRActionCreated {
					t.Errorf("second PR action = %q, want created", prs[1].Action)
				}
				if prs[2].Action != api.PRActionUnchanged {
					t.Errorf("third PR action = %q, want unchanged", prs[2].Action)
				}
			},
		},
		{
			name: "branches info extended backfills skipped branches, excludes stale",
			logs: `{"level":30,"msg":"Creating PR","branch":"renovate/active-dep","title":"Update active-dep"}` + "\n" +
				`{"level":30,"msg":"PR created","branch":"renovate/active-dep","pr":10}` + "\n" +
				`{"level":20,"msg":"branches info extended","branchesInformation":[` +
				`{"branchName":"renovate/active-dep","prNo":10,"prTitle":"Update active-dep","result":"done"},` +
				`{"branchName":"renovate/skipped-dep-a","prNo":20,"prTitle":"Update skipped-dep-a","result":"done"},` +
				`{"branchName":"renovate/skipped-dep-b","prNo":30,"prTitle":"Update skipped-dep-b"},` +
				`{"branchName":"renovate/stale-branch","prNo":18,"prTitle":"Old closed PR","result":"already-existed"},` +
				`{"branchName":"renovate/not-scheduled","prNo":null,"prTitle":"lock file maintenance","result":"not-scheduled"},` +
				`{"branchName":"renovate/no-pr-branch","prNo":null,"prTitle":"Update no-pr-branch"}]}`,
			wantCreated:   1,
			wantUnchanged: 3,
			wantPRCount:   4,
			checkDetails: func(t *testing.T, prs []api.PRDetail) {
				prsByBranch := make(map[string]api.PRDetail)
				for _, pr := range prs {
					prsByBranch[pr.Branch] = pr
				}
				// Active dep was captured by per-message parsing
				if prsByBranch["renovate/active-dep"].Action != api.PRActionCreated {
					t.Errorf("active-dep action = %q, want created", prsByBranch["renovate/active-dep"].Action)
				}
				// Skipped deps were backfilled from branches info
				skA := prsByBranch["renovate/skipped-dep-a"]
				if skA.Action != api.PRActionUnchanged {
					t.Errorf("skipped-dep-a action = %q, want unchanged", skA.Action)
				}
				if skA.Number != 20 {
					t.Errorf("skipped-dep-a number = %d, want 20", skA.Number)
				}
				if skA.Title != "Update skipped-dep-a" {
					t.Errorf("skipped-dep-a title = %q, want %q", skA.Title, "Update skipped-dep-a")
				}
				// Stale branch (already-existed) should be excluded
				if _, exists := prsByBranch["renovate/stale-branch"]; exists {
					t.Error("stale-branch should be excluded (result=already-existed)")
				}
				// Not-scheduled branch should be excluded
				if _, exists := prsByBranch["renovate/not-scheduled"]; exists {
					t.Error("not-scheduled branch should be excluded")
				}
				// Branch with null prNo and empty result should still be included
				noPR := prsByBranch["renovate/no-pr-branch"]
				if noPR.Number != 0 {
					t.Errorf("no-pr-branch number = %d, want 0", noPR.Number)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseRenovateLogs(tt.logs)

			if tt.wantNil {
				if result.PRActivity != nil {
					t.Errorf("PRActivity = %+v, want nil", result.PRActivity)
				}
				return
			}

			if result.PRActivity == nil {
				t.Fatal("PRActivity = nil, want non-nil")
			}

			pa := result.PRActivity
			if pa.Automerged != tt.wantAutomerged {
				t.Errorf("Automerged = %d, want %d", pa.Automerged, tt.wantAutomerged)
			}
			if pa.Created != tt.wantCreated {
				t.Errorf("Created = %d, want %d", pa.Created, tt.wantCreated)
			}
			if pa.Updated != tt.wantUpdated {
				t.Errorf("Updated = %d, want %d", pa.Updated, tt.wantUpdated)
			}
			if pa.Unchanged != tt.wantUnchanged {
				t.Errorf("Unchanged = %d, want %d", pa.Unchanged, tt.wantUnchanged)
			}
			if len(pa.PRs) != tt.wantPRCount {
				t.Errorf("len(PRs) = %d, want %d", len(pa.PRs), tt.wantPRCount)
			}
			if pa.Truncated != tt.wantTruncated {
				t.Errorf("Truncated = %v, want %v", pa.Truncated, tt.wantTruncated)
			}

			if tt.checkDetails != nil && len(pa.PRs) > 0 {
				tt.checkDetails(t, pa.PRs)
			}
		})
	}
}

func TestParseRenovateLogsPRActivityCap(t *testing.T) {
	// Generate 150 branches to test capping at MaxPRDetails (100)
	var lines []string
	lines = append(lines, `{"level":30,"msg":"Repository started"}`)
	for i := 0; i < 150; i++ {
		branch := fmt.Sprintf("renovate/dep-%03d", i)
		title := fmt.Sprintf("Update dep-%03d", i)
		lines = append(lines, fmt.Sprintf(`{"level":30,"msg":"Creating PR","branch":%q,"title":%q}`, branch, title))
	}
	lines = append(lines, `{"level":30,"msg":"Repository finished","result":"done"}`)
	logs := strings.Join(lines, "\n")

	result := ParseRenovateLogs(logs)

	if result.PRActivity == nil {
		t.Fatal("PRActivity = nil, want non-nil")
	}

	pa := result.PRActivity

	// Counts should reflect all 150 branches (computed before truncation)
	if pa.Created != 150 {
		t.Errorf("Created = %d, want 150", pa.Created)
	}

	// PRDetails should be capped at MaxPRDetails
	if len(pa.PRs) != MaxPRDetails {
		t.Errorf("len(PRs) = %d, want %d", len(pa.PRs), MaxPRDetails)
	}

	if !pa.Truncated {
		t.Error("Truncated = false, want true")
	}

	// PRs should be sorted by branch name, so first should be dep-000
	if len(pa.PRs) > 0 && pa.PRs[0].Branch != "renovate/dep-000" {
		t.Errorf("first PR branch = %q, want %q (sorted order)", pa.PRs[0].Branch, "renovate/dep-000")
	}

	// Last should be dep-099 (first 100 of 150 sorted branches)
	if len(pa.PRs) == MaxPRDetails && pa.PRs[99].Branch != "renovate/dep-099" {
		t.Errorf("last PR branch = %q, want %q", pa.PRs[99].Branch, "renovate/dep-099")
	}
}

func TestParseRenovateLogsPRActivityGoldenFile(t *testing.T) {
	// Simulates a realistic Renovate run with mixed PR activity
	logs := strings.Join([]string{
		`{"level":30,"msg":"Repository started","repository":"k8s/flux"}`,
		`{"level":30,"msg":"Dependency extraction complete"}`,
		// Two new PRs created
		`{"level":30,"msg":"Creating PR","branch":"renovate/golang-1.x","title":"Update golang Docker tag to v1.22"}`,
		`{"level":20,"msg":"git push","branch":"renovate/golang-1.x","result":{"remoteMessages":{"all":["Visit the existing pull request:","https://git.example.com/org/repo/pulls/900 merges into main"]}}}`,
		`{"level":30,"msg":"Creating PR","branch":"renovate/helm-renovate-46.x","title":"Update registry.example.com/org/helm/renovate Docker tag to v46.99.0"}`,
		`{"level":20,"msg":"git push","branch":"renovate/helm-renovate-46.x","result":{"remoteMessages":{"all":["Visit the existing pull request:","https://git.example.com/org/repo/pulls/901 merges into main"]}}}`,
		// One PR updated
		`{"level":30,"msg":"Updating PR","branch":"renovate/rook-ceph-1.x","title":"Update rook-ceph to v1.15.0"}`,
		`{"level":20,"msg":"git push","branch":"renovate/rook-ceph-1.x","result":{"remoteMessages":{"all":["Visit the existing pull request:","https://git.example.com/org/repo/pulls/850 merges into main"]}}}`,
		// Three unchanged PRs
		`{"level":20,"msg":"Pull Request #800 does not need updating","branch":"renovate/traefik-30.x"}`,
		`{"level":20,"msg":"Pull Request #810 does not need updating","branch":"renovate/cert-manager-1.x"}`,
		`{"level":20,"msg":"Pull Request #820 does not need updating","branch":"renovate/harbor-1.x"}`,
		// Finish
		`{"level":30,"msg":"Repository finished","result":"done"}`,
	}, "\n")

	result := ParseRenovateLogs(logs)

	if result.PRActivity == nil {
		t.Fatal("PRActivity = nil, want non-nil")
	}

	pa := result.PRActivity

	if pa.Created != 2 {
		t.Errorf("Created = %d, want 2", pa.Created)
	}
	if pa.Updated != 1 {
		t.Errorf("Updated = %d, want 1", pa.Updated)
	}
	if pa.Unchanged != 3 {
		t.Errorf("Unchanged = %d, want 3", pa.Unchanged)
	}
	if len(pa.PRs) != 6 {
		t.Errorf("len(PRs) = %d, want 6", len(pa.PRs))
	}
	if pa.Truncated {
		t.Error("Truncated = true, want false")
	}

	// PRs are sorted by branch name; verify a few
	prsByBranch := make(map[string]api.PRDetail)
	for _, pr := range pa.PRs {
		prsByBranch[pr.Branch] = pr
	}

	// Check golang PR has all fields populated
	golang, ok := prsByBranch["renovate/golang-1.x"]
	if !ok {
		t.Fatal("missing PR for renovate/golang-1.x")
	}
	if golang.Action != api.PRActionCreated {
		t.Errorf("golang action = %q, want %q", golang.Action, api.PRActionCreated)
	}
	if golang.Title != "Update golang Docker tag to v1.22" {
		t.Errorf("golang title = %q, want %q", golang.Title, "Update golang Docker tag to v1.22")
	}
	if golang.Number != 900 {
		t.Errorf("golang number = %d, want 900", golang.Number)
	}

	// Check unchanged PR has number
	traefik, ok := prsByBranch["renovate/traefik-30.x"]
	if !ok {
		t.Fatal("missing PR for renovate/traefik-30.x")
	}
	if traefik.Action != api.PRActionUnchanged {
		t.Errorf("traefik action = %q, want %q", traefik.Action, api.PRActionUnchanged)
	}
	if traefik.Number != 800 {
		t.Errorf("traefik number = %d, want 800", traefik.Number)
	}

	// Check updated PR
	rook, ok := prsByBranch["renovate/rook-ceph-1.x"]
	if !ok {
		t.Fatal("missing PR for renovate/rook-ceph-1.x")
	}
	if rook.Action != api.PRActionUpdated {
		t.Errorf("rook action = %q, want %q", rook.Action, api.PRActionUpdated)
	}
	if rook.Number != 850 {
		t.Errorf("rook number = %d, want 850", rook.Number)
	}

	// Verify existing parsing still works
	if result.RenovateResultStatus == nil || *result.RenovateResultStatus != "done" {
		t.Errorf("RenovateResultStatus = %v, want 'done'", result.RenovateResultStatus)
	}
	if result.HasIssues {
		t.Error("HasIssues should be false")
	}
}

func TestParseRenovateLogsLogIssues(t *testing.T) {
	tests := []struct {
		name           string
		logs           string
		wantNil        bool
		wantWarnCount  int
		wantErrorCount int
		wantIssueCount int
		wantTruncated  bool
		checkIssues    func(t *testing.T, issues []api.LogIssue)
	}{
		{
			name:    "empty logs - nil LogIssues",
			logs:    "",
			wantNil: true,
		},
		{
			name:           "no warnings or errors - zero counts",
			logs:           `{"level":30,"msg":"Info message"}`,
			wantWarnCount:  0,
			wantErrorCount: 0,
			wantIssueCount: 0,
		},
		{
			name:           "single warning",
			logs:           `{"level":40,"msg":"Config validation issue"}`,
			wantWarnCount:  1,
			wantErrorCount: 0,
			wantIssueCount: 1,
			checkIssues: func(t *testing.T, issues []api.LogIssue) {
				if issues[0].Level != 40 {
					t.Errorf("issue level = %d, want 40", issues[0].Level)
				}
				if issues[0].Message != "Config validation issue" {
					t.Errorf("issue message = %q, want %q", issues[0].Message, "Config validation issue")
				}
			},
		},
		{
			name:           "single error",
			logs:           `{"level":50,"msg":"Failed to look up dependency"}`,
			wantWarnCount:  0,
			wantErrorCount: 1,
			wantIssueCount: 1,
			checkIssues: func(t *testing.T, issues []api.LogIssue) {
				if issues[0].Level != 50 {
					t.Errorf("issue level = %d, want 50", issues[0].Level)
				}
			},
		},
		{
			name: "mixed warnings and errors",
			logs: `{"level":40,"msg":"Warn 1"}` + "\n" +
				`{"level":50,"msg":"Error 1"}` + "\n" +
				`{"level":40,"msg":"Warn 2"}` + "\n" +
				`{"level":50,"msg":"Error 2"}`,
			wantWarnCount:  2,
			wantErrorCount: 2,
			wantIssueCount: 4,
		},
		{
			name: "fatal counts as error",
			logs: `{"level":60,"msg":"Fatal error"}`,
			wantWarnCount:  0,
			wantErrorCount: 1,
			wantIssueCount: 1,
			checkIssues: func(t *testing.T, issues []api.LogIssue) {
				if issues[0].Level != 60 {
					t.Errorf("issue level = %d, want 60", issues[0].Level)
				}
			},
		},
		{
			name: "deduplication - same message counted but stored once",
			logs: `{"level":40,"msg":"Duplicate warning"}` + "\n" +
				`{"level":40,"msg":"Duplicate warning"}` + "\n" +
				`{"level":40,"msg":"Duplicate warning"}`,
			wantWarnCount:  3,
			wantErrorCount: 0,
			wantIssueCount: 1,
		},
		{
			name: "message truncation with ellipsis suffix",
			logs: fmt.Sprintf(`{"level":40,"msg":"%s"}`, strings.Repeat("x", MaxIssueMessageLen+50)),
			wantWarnCount:  1,
			wantErrorCount: 0,
			wantIssueCount: 1,
			checkIssues: func(t *testing.T, issues []api.LogIssue) {
				wantLen := MaxIssueMessageLen + len("…")
				if len(issues[0].Message) != wantLen {
					t.Errorf("message length = %d, want %d", len(issues[0].Message), wantLen)
				}
				if !strings.HasSuffix(issues[0].Message, "…") {
					t.Errorf("message does not end with ellipsis")
				}
			},
		},
		{
			name: "message exactly at MaxIssueMessageLen - no ellipsis",
			logs: fmt.Sprintf(`{"level":40,"msg":"%s"}`, strings.Repeat("x", MaxIssueMessageLen)),
			wantWarnCount:  1,
			wantErrorCount: 0,
			wantIssueCount: 1,
			checkIssues: func(t *testing.T, issues []api.LogIssue) {
				if len(issues[0].Message) != MaxIssueMessageLen {
					t.Errorf("message length = %d, want %d (no ellipsis expected)", len(issues[0].Message), MaxIssueMessageLen)
				}
			},
		},
		{
			name: "cap at MaxLogIssues with truncated flag",
			logs: func() string {
				var lines []string
				for i := 0; i < MaxLogIssues+5; i++ {
					lines = append(lines, fmt.Sprintf(`{"level":40,"msg":"Unique warning %d"}`, i))
				}
				return strings.Join(lines, "\n")
			}(),
			wantWarnCount:  MaxLogIssues + 5,
			wantErrorCount: 0,
			wantIssueCount: MaxLogIssues,
			wantTruncated:  true,
		},
		{
			name: "empty message is not stored",
			logs: `{"level":40,"msg":""}` + "\n" +
				`{"level":40,"msg":"Real warning"}`,
			wantWarnCount:  2,
			wantErrorCount: 0,
			wantIssueCount: 1,
			checkIssues: func(t *testing.T, issues []api.LogIssue) {
				if issues[0].Message != "Real warning" {
					t.Errorf("issue message = %q, want %q", issues[0].Message, "Real warning")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseRenovateLogs(tt.logs)

			if tt.wantNil {
				if result.LogIssues != nil {
					t.Errorf("LogIssues = %+v, want nil", result.LogIssues)
				}
				return
			}

			if result.LogIssues == nil {
				t.Fatal("LogIssues is nil, want non-nil")
			}

			if result.LogIssues.WarnCount != tt.wantWarnCount {
				t.Errorf("WarnCount = %d, want %d", result.LogIssues.WarnCount, tt.wantWarnCount)
			}
			if result.LogIssues.ErrorCount != tt.wantErrorCount {
				t.Errorf("ErrorCount = %d, want %d", result.LogIssues.ErrorCount, tt.wantErrorCount)
			}
			if len(result.LogIssues.Issues) != tt.wantIssueCount {
				t.Errorf("len(Issues) = %d, want %d", len(result.LogIssues.Issues), tt.wantIssueCount)
			}
			if result.LogIssues.Truncated != tt.wantTruncated {
				t.Errorf("Truncated = %v, want %v", result.LogIssues.Truncated, tt.wantTruncated)
			}

			if tt.checkIssues != nil && len(result.LogIssues.Issues) > 0 {
				tt.checkIssues(t, result.LogIssues.Issues)
			}
		})
	}
}

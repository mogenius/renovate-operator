package parser

import (
	"reflect"
	"strings"
	"testing"
)

func TestProcessedBaseBranches(t *testing.T) {
	tests := []struct {
		name string
		logs []string
		want []string
	}{
		{
			// shape of the live run on atomic/portal: the repository config overrode the
			// requested branch, so only master is listed and tagged
			name: "debug run whose repository config overrides the requested branch",
			logs: []string{
				`{"level":20,"msg":"Env config","config":{"baseBranchPatterns":["test/renovate-685"]}}`,
				`{"level":20,"msg":"Repository config","fileName":"renovate.json","config":{"baseBranchPatterns":["master"]}}`,
				`{"level":20,"msg":"baseBranches","baseBranches":["master"]}`,
				`{"level":20,"msg":"baseBranch: master","baseBranch":"master"}`,
				`{"level":30,"msg":"DRY-RUN: Would ensure Dependency Dashboard"}`,
			},
			want: []string{"master"},
		},
		{
			name: "info-level run only tags lines with the branch",
			logs: []string{
				`{"level":40,"msg":"Found renovate config warnings","baseBranch":"develop"}`,
				`{"level":30,"msg":"DRY-RUN: Would update PR #2","baseBranch":"develop","branch":"renovate/x"}`,
			},
			want: []string{"develop"},
		},
		{
			name: "several base branches are listed once each, sorted",
			logs: []string{
				`{"level":20,"msg":"baseBranches","baseBranches":["release/1","main"]}`,
				`{"level":30,"msg":"x","baseBranch":"main"}`,
				`{"level":30,"msg":"y","baseBranch":"release/1"}`,
			},
			want: []string{"main", "release/1"},
		},
		{
			name: "a run that never reached the branches names none",
			logs: []string{
				`{"level":30,"msg":"Repository started"}`,
				`{"level":30,"msg":"Repository finished","result":"onboarding"}`,
			},
			want: []string{},
		},
		{
			name: "lines that are not JSON are skipped",
			logs: []string{
				`not json mentioning baseBranch`,
				`{"level":30,"msg":"x","baseBranch":"main"}`,
			},
			want: []string{"main"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ProcessedBaseBranches(strings.Join(tt.logs, "\n")); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ProcessedBaseBranches() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Debug lines carrying whole configs can exceed bufio.Scanner's 1 MB limit; a long line
// must not hide the branch tags that follow it.
func TestProcessedBaseBranchesReadsPastVeryLongLines(t *testing.T) {
	longLine := `{"level":20,"msg":"Repository config","config":{"description":"` + strings.Repeat("x", 2*1024*1024) + `","baseBranch":"padding"}}`
	logs := longLine + "\n" + `{"level":30,"msg":"x","baseBranch":"master"}`

	got := ProcessedBaseBranches(logs)
	if !reflect.DeepEqual(got, []string{"master"}) {
		t.Errorf("ProcessedBaseBranches() = %v, want [master] (the long line has no top-level baseBranch)", got)
	}
}

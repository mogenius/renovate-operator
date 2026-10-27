package renovate

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

var checkTime = time.Date(2026, 10, 27, 11, 45, 43, 0, time.UTC)

func TestCheckCustomBaseBranch_RepositoryConfigOverridesTheBranch(t *testing.T) {
	logs := strings.Join([]string{
		`{"level":20,"msg":"baseBranches","baseBranches":["master"]}`,
		`{"level":20,"msg":"baseBranch: master","baseBranch":"master"}`,
	}, "\n") // no trailing newline, like a log cut at the last line

	checked, processed := checkCustomBaseBranch(logs, "test/renovate-685", checkTime)

	if !reflect.DeepEqual(processed, []string{"master"}) {
		t.Fatalf("processed = %v, want [master]", processed)
	}
	if !strings.HasPrefix(checked, logs+"\n") {
		t.Fatal("expected Renovate's log to stay unchanged in front of the warning")
	}
	lines := strings.Split(strings.TrimSpace(checked), "\n")
	var warning customBranchMismatchEntry
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &warning); err != nil {
		t.Fatalf("the appended warning is not a JSON log entry: %v", err)
	}
	if warning.Level != 40 || warning.Name != "renovate-operator" {
		t.Errorf("expected a warn-level entry from renovate-operator, got level %d name %q", warning.Level, warning.Name)
	}
	if warning.RequestedBaseBranch != "test/renovate-685" || !reflect.DeepEqual(warning.ProcessedBaseBranches, []string{"master"}) {
		t.Errorf("unexpected branches in warning: %+v", warning)
	}
	if !strings.Contains(warning.Msg, `"test/renovate-685" was not processed`) || !strings.Contains(warning.Msg, "master") {
		t.Errorf("message does not name the branches: %q", warning.Msg)
	}
	if warning.Time != "2026-10-27T11:45:43Z" {
		t.Errorf("time = %q, want the check time", warning.Time)
	}
}

func TestCheckCustomBaseBranch_LeavesTheLogAloneOtherwise(t *testing.T) {
	tests := []struct {
		name string
		logs string
	}{
		{name: "the requested branch was processed", logs: `{"level":30,"msg":"x","baseBranch":"test/renovate-685"}` + "\n"},
		{name: "the requested branch was processed among others", logs: `{"level":20,"msg":"baseBranches","baseBranches":["master","test/renovate-685"]}` + "\n"},
		{name: "the log names no base branch (onboarding)", logs: `{"level":30,"msg":"Repository finished","result":"onboarding"}` + "\n"},
		{name: "empty log", logs: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checked, processed := checkCustomBaseBranch(tt.logs, "test/renovate-685", checkTime)
			if checked != tt.logs || processed != nil {
				t.Errorf("expected the log unchanged and no mismatch, got processed=%v and log %q", processed, checked)
			}
		})
	}
}

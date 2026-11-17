package v1alpha1

import (
	"encoding/json"
	"testing"
)

// DeepCopy is hand-written for RenovateProject, so a map added to the status has to be
// copied there explicitly or the copy shares it with the original.
func TestRenovateProjectDeepCopyDoesNotAliasCategoryCounts(t *testing.T) {
	in := &RenovateProject{Status: RenovateProjectState{LogIssues: &LogIssues{
		WarnCount:      1,
		CategoryCounts: map[LogIssueCategory]int{LogIssueCategoryLookup: 1},
		Issues:         []LogIssue{{Level: 40, Message: "Package lookup failures", Category: LogIssueCategoryLookup}},
	}}}

	out := &RenovateProject{}
	in.DeepCopyInto(out)
	out.Status.LogIssues.CategoryCounts[LogIssueCategoryLookup] = 99
	out.Status.LogIssues.Issues[0].Category = LogIssueCategoryOther

	if got := in.Status.LogIssues.CategoryCounts[LogIssueCategoryLookup]; got != 1 {
		t.Errorf("original CategoryCounts changed through the copy: %d", got)
	}
	if got := in.Status.LogIssues.Issues[0].Category; got != LogIssueCategoryLookup {
		t.Errorf("original issue category changed through the copy: %q", got)
	}
}

// A status written before issues were categorised has neither field, and must still decode.
func TestLogIssuesWithoutCategoriesStillDecode(t *testing.T) {
	var logIssues LogIssues
	stored := `{"warnCount":1,"errorCount":0,"issues":[{"level":40,"message":"Package lookup failures"}]}`
	if err := json.Unmarshal([]byte(stored), &logIssues); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if logIssues.CategoryCounts != nil || logIssues.Issues[0].Category != "" {
		t.Errorf("expected no categories, got %+v", logIssues)
	}

	encoded, err := json.Marshal(logIssues)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(encoded) != stored {
		t.Errorf("round trip added fields:\n got %s\nwant %s", encoded, stored)
	}
}

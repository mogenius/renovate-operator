package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// PRAction represents what happened to a PR in a Renovate run.
type PRAction string

const (
	PRActionAutomerged    PRAction = "automerged"
	PRActionCreated       PRAction = "created"
	PRActionUpdated       PRAction = "updated"
	PRActionNeedsApproval PRAction = "needs-approval"
	PRActionUnchanged     PRAction = "unchanged"
)

// PRDetail represents a single PR found in Renovate logs.
type PRDetail struct {
	Branch string   `json:"branch"`
	Number int      `json:"number,omitempty"`
	Title  string   `json:"title,omitempty"`
	Action PRAction `json:"action"`
}

// PRActivity contains aggregate counts and individual details of PR activity from a run.
type PRActivity struct {
	Automerged    int        `json:"automerged"`
	Created       int        `json:"created"`
	Updated       int        `json:"updated"`
	NeedsApproval int        `json:"needsApproval"`
	Unchanged     int        `json:"unchanged"`
	PRs           []PRDetail `json:"prs,omitempty"`
	Truncated     bool       `json:"truncated,omitempty"`
}

// LogIssue represents a single warning or error from Renovate logs.
type LogIssue struct {
	Level   int    `json:"level"`
	Message string `json:"message"`
}

// LogIssues contains aggregate counts and individual issue messages from a Renovate run.
type LogIssues struct {
	WarnCount  int        `json:"warnCount"`
	ErrorCount int        `json:"errorCount"`
	Issues     []LogIssue `json:"issues,omitempty"`
	Truncated  bool       `json:"truncated,omitempty"`
}

type RenovateProjectStatus string

const (
	JobStatusScheduled RenovateProjectStatus = "scheduled"
	JobStatusRunning   RenovateProjectStatus = "running"
	JobStatusCompleted RenovateProjectStatus = "completed"
	JobStatusFailed    RenovateProjectStatus = "failed"
	JobStatusCancelled RenovateProjectStatus = "cancelled"
)

type RenovateExecutionOptions struct {
	// If true, the renovate job will be executed with RENOVATE_LOG_LEVEL=debug
	Debug bool `json:"debug,omitempty"`
}

// RenovateProjectSpec contains the immutable identity of the tracked repository.
type RenovateProjectSpec struct {
	// Project is the full repository name as reported by the git platform (e.g. "org/repo").
	Project string `json:"project"`
}

// RenovateProjectState captures the runtime status of one Renovate run for this repository.
type RenovateProjectState struct {
	// LastTransition records when the project most recently changed state.
	LastTransition       metav1.Time               `json:"lastTransition,omitempty"`
	Duration             *string                   `json:"duration,omitempty"`
	Status               RenovateProjectStatus     `json:"status"`
	Priority             int32                     `json:"priority,omitempty"`
	RenovateResultStatus *string                   `json:"renovateResultStatus,omitempty"`
	PRActivity           *PRActivity               `json:"prActivity,omitempty"`
	LogIssues            *LogIssues                `json:"logIssues,omitempty"`
	ExecutionOptions     *RenovateExecutionOptions `json:"executionOptions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=rp
// +kubebuilder:printcolumn:name="Job",type=string,JSONPath=`.metadata.labels.renovate-operator\.mogenius\.com/renovatejob`
// +kubebuilder:printcolumn:name="Project",type=string,JSONPath=`.spec.project`
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.status`
// +kubebuilder:printcolumn:name="Last Transition",type=date,JSONPath=`.status.lastTransition`
type RenovateProject struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RenovateProjectSpec  `json:"spec,omitempty"`
	Status RenovateProjectState `json:"status,omitempty"`
}

type RenovateProjectList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RenovateProject `json:"items"`
}

func (in *RenovateProject) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(RenovateProject)
	in.DeepCopyInto(out)
	return out
}

func (in *RenovateProject) DeepCopyInto(out *RenovateProject) {
	*out = *in
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	if in.Status.Duration != nil {
		out.Status.Duration = new(string)
		*out.Status.Duration = *in.Status.Duration
	}
	if in.Status.RenovateResultStatus != nil {
		out.Status.RenovateResultStatus = new(string)
		*out.Status.RenovateResultStatus = *in.Status.RenovateResultStatus
	}
	if in.Status.PRActivity != nil {
		out.Status.PRActivity = new(PRActivity)
		*out.Status.PRActivity = *in.Status.PRActivity
		if in.Status.PRActivity.PRs != nil {
			out.Status.PRActivity.PRs = make([]PRDetail, len(in.Status.PRActivity.PRs))
			copy(out.Status.PRActivity.PRs, in.Status.PRActivity.PRs)
		}
	}
	if in.Status.LogIssues != nil {
		out.Status.LogIssues = new(LogIssues)
		*out.Status.LogIssues = *in.Status.LogIssues
		if in.Status.LogIssues.Issues != nil {
			out.Status.LogIssues.Issues = make([]LogIssue, len(in.Status.LogIssues.Issues))
			copy(out.Status.LogIssues.Issues, in.Status.LogIssues.Issues)
		}
	}
	if in.Status.ExecutionOptions != nil {
		out.Status.ExecutionOptions = new(RenovateExecutionOptions)
		*out.Status.ExecutionOptions = *in.Status.ExecutionOptions
	}
}

func (in *RenovateProjectList) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(RenovateProjectList)
	*out = *in
	if in.Items != nil {
		out.Items = make([]RenovateProject, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
	return out
}

package ui

import (
	"testing"

	api "renovate-operator/api/v1alpha1"
	"renovate-operator/internal/policy"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func jobWithCondition(conditions ...metav1.Condition) *api.RenovateJob {
	job := &api.RenovateJob{}
	job.ObjectMeta = metav1.ObjectMeta{Name: "job1", Namespace: "default"}
	job.Status.Conditions = conditions
	return job
}

func TestAcceptedState(t *testing.T) {
	tests := []struct {
		name        string
		job         *api.RenovateJob
		wantAccept  bool
		wantMessage string
	}{
		{
			// A job the operator has not reconciled since upgrading has no condition.
			// Reporting it as halted would black out the whole dashboard on rollout.
			name:       "no conditions yet is treated as accepted",
			job:        jobWithCondition(),
			wantAccept: true,
		},
		{
			name: "unrelated condition only",
			job: jobWithCondition(metav1.Condition{
				Type:   "SomethingElse",
				Status: metav1.ConditionFalse,
				Reason: "Whatever",
			}),
			wantAccept: true,
		},
		{
			name: "refused",
			job: jobWithCondition(metav1.Condition{
				Type:    api.ConditionAccepted,
				Status:  metav1.ConditionFalse,
				Reason:  policy.ReasonDestinationNotAllowed,
				Message: `spec.provider.endpoint: host "attacker.example.net" is not allowed`,
			}),
			wantAccept:  false,
			wantMessage: `spec.provider.endpoint: host "attacker.example.net" is not allowed`,
		},
		{
			name: "accepted",
			job: jobWithCondition(metav1.Condition{
				Type:    api.ConditionAccepted,
				Status:  metav1.ConditionTrue,
				Reason:  policy.ReasonPolicySatisfied,
				Message: "RenovateJob satisfies the operator's policy",
			}),
			wantAccept:  true,
			wantMessage: "RenovateJob satisfies the operator's policy",
		},
		{
			// Unknown is not a refusal: the operator has not decided yet.
			name: "unknown is not a refusal",
			job: jobWithCondition(metav1.Condition{
				Type:   api.ConditionAccepted,
				Status: metav1.ConditionUnknown,
				Reason: "Pending",
			}),
			wantAccept: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			accepted, message := acceptedState(tc.job)
			if accepted != tc.wantAccept {
				t.Errorf("expected accepted=%v, got %v", tc.wantAccept, accepted)
			}
			if message != tc.wantMessage {
				t.Errorf("expected message %q, got %q", tc.wantMessage, message)
			}
		})
	}
}

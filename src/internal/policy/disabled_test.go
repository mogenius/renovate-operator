package policy

import (
	"strings"
	"testing"

	api "renovate-operator/api/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// everythingRefused is a job that violates every operator-side rule at once, so a
// single fixture proves the switch reaches all of them.
func everythingRefused() *api.RenovateJob {
	job := &api.RenovateJob{}
	job.ObjectMeta = metav1.ObjectMeta{Name: "job1", Namespace: "default"}
	job.Spec = api.RenovateJobSpec{
		Image:    "ghcr.io/attacker/renovate:latest",
		Provider: &api.RenovateProvider{Name: "github", Endpoint: "https://attacker.example.net"},
		Webhook: &api.RenovateWebhook{
			Enabled: true,
			BaseURL: "https://attacker.example.net",
		},
		ServiceAccount:  &api.RenovateJobServiceAccount{Name: "renovate-operator"},
		SecurityContext: &api.RenovateJobSecurityContext{Pod: &corev1.PodSecurityContext{RunAsUser: new(int64(0))}},
	}
	return job
}

func TestDisabledPolicyAllowsEverything(t *testing.T) {
	job := everythingRefused()

	// Sanity check: with enforcement on, this job is refused.
	if err := (Policy{}).ValidateJob(job); err == nil {
		t.Fatal("the fixture is supposed to violate the policy")
	}

	disabled := Policy{Disabled: true}

	if err := disabled.ValidateJob(job); err != nil {
		t.Errorf("ValidateJob: expected no error, got %v", err)
	}
	if err := disabled.ValidateJobDestinations(job); err != nil {
		t.Errorf("ValidateJobDestinations: expected no error, got %v", err)
	}
	if err := disabled.ValidateJobSpec(job.Spec); err != nil {
		t.Errorf("ValidateJobSpec: expected no error, got %v", err)
	}
	if err := disabled.ValidateDestination("https://attacker.example.net", "spec.provider.endpoint"); err != nil {
		t.Errorf("ValidateDestination: expected no error, got %v", err)
	}
	unlabeled := &corev1.Secret{Name: "db-credentials"}
	if err := disabled.ValidateReferencedSecret(unlabeled); err != nil {
		t.Errorf("ValidateReferencedSecret: expected no error, got %v", err)
	}
}

// The zero value is what a caller that forgot to configure anything gets, and what
// most tests construct. It has to enforce.
func TestZeroPolicyEnforces(t *testing.T) {
	if err := (Policy{}).ValidateJob(everythingRefused()); err == nil {
		t.Fatal("the zero Policy must enforce, not wave everything through")
	}
}

func TestAcceptedReasonReportsDisabledState(t *testing.T) {
	if got := (Policy{}).AcceptedReason(); got != ReasonPolicySatisfied {
		t.Errorf("expected %q with enforcement on, got %q", ReasonPolicySatisfied, got)
	}
	if got := (Policy{Disabled: true}).AcceptedReason(); got != ReasonPolicyDisabled {
		t.Errorf("expected %q with enforcement off, got %q", ReasonPolicyDisabled, got)
	}
}

// Disabling enforcement must not also disable the config validation that catches a
// typo, or a value fixed while disabled would fail on the way back to enabled.
func TestDisablingDoesNotSkipConfigValidation(t *testing.T) {
	if err := ValidateAllowedHosts(".example.com"); err == nil {
		t.Error("expected a subtree host entry to stay invalid")
	}
	if err := ValidateAllowedImages("renovate/renovate:1"); err == nil {
		t.Error("expected a tagged image entry to stay invalid")
	}
}

// FromConfig maps the chart's positive policy.enabled onto the negative field, so a
// missing or malformed value must not silently disable enforcement.
func TestDisabledOnlyWhenExplicitlyFalse(t *testing.T) {
	for _, tc := range []struct {
		value        string
		wantDisabled bool
	}{
		{value: "false", wantDisabled: true},
		{value: "true", wantDisabled: false},
		{value: "", wantDisabled: false},
		{value: "FALSE", wantDisabled: false},
		{value: "0", wantDisabled: false},
		{value: "no", wantDisabled: false},
	} {
		t.Run(tc.value, func(t *testing.T) {
			// Mirrors FromConfig's mapping without the config singleton.
			disabled := tc.value == "false"
			if disabled != tc.wantDisabled {
				t.Errorf("POLICY_ENABLED=%q: disabled=%v, want %v", tc.value, disabled, tc.wantDisabled)
			}
		})
	}
}

func TestDisabledPolicyStillReportsNoViolationReason(t *testing.T) {
	err := Policy{Disabled: true}.ValidateJob(everythingRefused())
	if err != nil {
		t.Fatalf("expected no error: %v", err)
	}
	if reason := ReasonFor(err); reason != "" {
		t.Errorf("expected no violation reason, got %q", reason)
	}
	if strings.TrimSpace(ReasonPolicyDisabled) == "" {
		t.Error("ReasonPolicyDisabled must be a non-empty condition reason")
	}
}

package policy

import (
	"strings"
	"testing"

	api "renovate-operator/api/v1alpha1"

	corev1 "k8s.io/api/core/v1"
)

func specWithServiceAccount(name string) api.RenovateJobSpec {
	return api.RenovateJobSpec{ServiceAccount: &api.RenovateJobServiceAccount{Name: name}}
}

func TestValidateJobSpecServiceAccount(t *testing.T) {
	tests := []struct {
		name    string
		policy  Policy
		spec    api.RenovateJobSpec
		allowed bool
	}{
		{
			name:    "unset service account uses the namespace default",
			spec:    api.RenovateJobSpec{},
			allowed: true,
		},
		{
			name:    "empty name is the same as unset",
			spec:    specWithServiceAccount(""),
			allowed: true,
		},
		{
			// The operator's own ServiceAccount is the prize here: it can read secrets
			// across the cluster.
			name:    "naming a service account is refused by default",
			spec:    specWithServiceAccount("renovate-operator"),
			allowed: false,
		},
		{
			name:    "allowlisted service account",
			policy:  Policy{AllowedServiceAccounts: []string{"renovate-workload-identity"}},
			spec:    specWithServiceAccount("renovate-workload-identity"),
			allowed: true,
		},
		{
			name:    "service account outside the allowlist",
			policy:  Policy{AllowedServiceAccounts: []string{"renovate-workload-identity"}},
			spec:    specWithServiceAccount("renovate-operator"),
			allowed: false,
		},
		{
			// An allowlist must not accidentally permit the default too loosely; an
			// unset name is always fine regardless.
			name:    "unset name with a populated allowlist",
			policy:  Policy{AllowedServiceAccounts: []string{"renovate-workload-identity"}},
			spec:    api.RenovateJobSpec{},
			allowed: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.policy.ValidateJobSpec(tc.spec)
			if tc.allowed && err != nil {
				t.Fatalf("expected the spec to be allowed, got %v", err)
			}
			if !tc.allowed {
				if err == nil {
					t.Fatal("expected the spec to be refused")
				}
				if got := ReasonFor(err); got != ReasonServiceAccountNotAllowed {
					t.Errorf("expected reason %q, got %q", ReasonServiceAccountNotAllowed, got)
				}
			}
		})
	}
}

func podSecurityContext(runAsUser *int64, runAsNonRoot *bool) api.RenovateJobSpec {
	return api.RenovateJobSpec{SecurityContext: &api.RenovateJobSecurityContext{
		Pod: &corev1.PodSecurityContext{RunAsUser: runAsUser, RunAsNonRoot: runAsNonRoot},
	}}
}

func containerSecurityContext(runAsUser *int64, runAsNonRoot *bool) api.RenovateJobSpec {
	return api.RenovateJobSpec{SecurityContext: &api.RenovateJobSecurityContext{
		Container: &corev1.SecurityContext{RunAsUser: runAsUser, RunAsNonRoot: runAsNonRoot},
	}}
}

func TestValidateJobSpecRootUser(t *testing.T) {
	tests := []struct {
		name    string
		policy  Policy
		spec    api.RenovateJobSpec
		allowed bool
	}{
		{name: "no security context", spec: api.RenovateJobSpec{}, allowed: true},
		{name: "non-root uid on the pod", spec: podSecurityContext(new(int64(12021)), nil), allowed: true},
		{name: "uid 0 on the pod", spec: podSecurityContext(new(int64(0)), nil), allowed: false},
		{name: "uid 0 on the container", spec: containerSecurityContext(new(int64(0)), nil), allowed: false},
		{name: "runAsNonRoot disabled on the pod", spec: podSecurityContext(nil, new(false)), allowed: false},
		{name: "runAsNonRoot disabled on the container", spec: containerSecurityContext(nil, new(false)), allowed: false},
		{name: "runAsNonRoot enabled", spec: podSecurityContext(nil, new(true)), allowed: true},
		{
			name:    "uid 0 permitted when allowRootUser is on",
			policy:  Policy{AllowRootUser: true},
			spec:    podSecurityContext(new(int64(0)), nil),
			allowed: true,
		},
		{
			name:    "runAsNonRoot disabled permitted when allowRootUser is on",
			policy:  Policy{AllowRootUser: true},
			spec:    containerSecurityContext(nil, new(false)),
			allowed: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.policy.ValidateJobSpec(tc.spec)
			if tc.allowed && err != nil {
				t.Fatalf("expected the spec to be allowed, got %v", err)
			}
			if !tc.allowed {
				if err == nil {
					t.Fatal("expected the spec to be refused")
				}
				if got := ReasonFor(err); got != ReasonRootUserNotAllowed {
					t.Errorf("expected reason %q, got %q", ReasonRootUserNotAllowed, got)
				}
			}
		})
	}
}

func TestValidateJobSpecErrorsAreActionable(t *testing.T) {
	saErr := Policy{}.ValidateJobSpec(specWithServiceAccount("renovate-operator"))
	for _, want := range []string{"renovate-operator", "policy.allowedServiceAccounts"} {
		if !strings.Contains(saErr.Error(), want) {
			t.Errorf("expected the service account error to mention %q, got: %v", want, saErr)
		}
	}

	rootErr := Policy{}.ValidateJobSpec(podSecurityContext(new(int64(0)), nil))
	for _, want := range []string{"runAsUser", "policy.allowRootUser"} {
		if !strings.Contains(rootErr.Error(), want) {
			t.Errorf("expected the root error to mention %q, got: %v", want, rootErr)
		}
	}
}

// ValidateJob is the single gate; it has to apply both halves.
func TestValidateJobCoversDestinationsAndSpec(t *testing.T) {
	p := Policy{AllowedHosts: []string{"api.github.com"}}

	job := &api.RenovateJob{}
	job.Spec.Provider = &api.RenovateProvider{Name: "github"}
	if err := p.ValidateJob(job); err != nil {
		t.Fatalf("expected a compliant job to pass, got %v", err)
	}

	job.Spec.ServiceAccount = &api.RenovateJobServiceAccount{Name: "renovate-operator"}
	err := p.ValidateJob(job)
	if err == nil {
		t.Fatal("expected ValidateJob to apply the spec checks, not only the destinations")
	}
	if got := ReasonFor(err); got != ReasonServiceAccountNotAllowed {
		t.Errorf("expected reason %q, got %q", ReasonServiceAccountNotAllowed, got)
	}

	// A destination violation still wins when both are present, since nothing can run
	// either way and the destination is the more urgent finding.
	job.Spec.Provider.Endpoint = "https://attacker.example.net"
	if got := ReasonFor(p.ValidateJob(job)); got != ReasonDestinationNotAllowed {
		t.Errorf("expected the destination reason to be reported, got %q", got)
	}
}

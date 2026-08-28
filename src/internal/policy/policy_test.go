package policy

import (
	"strings"
	"testing"

	api "renovate-operator/api/v1alpha1"

	corev1 "k8s.io/api/core/v1"
)

func TestValidateDestination(t *testing.T) {
	tests := []struct {
		name         string
		allowedHosts []string
		url          string
		allowed      bool
	}{
		{name: "exact host matches", allowedHosts: []string{"gitlab.example.com"}, url: "https://gitlab.example.com/api/v4", allowed: true},
		{name: "different host denied", allowedHosts: []string{"gitlab.example.com"}, url: "https://attacker.example.net", allowed: false},
		{name: "empty allowlist denies everything", allowedHosts: nil, url: "https://api.github.com", allowed: false},
		// Matching is exact. A sibling name under an allowed domain is not
		// implicitly trusted, because standing one up is often within reach of an
		// internal attacker.
		{name: "sibling subdomain denied", allowedHosts: []string{"git.example.com"}, url: "https://evil.example.com", allowed: false},
		{name: "parent domain denied", allowedHosts: []string{"git.example.com"}, url: "https://example.com", allowed: false},
		{name: "deeper subdomain denied", allowedHosts: []string{"example.com"}, url: "https://git.example.com", allowed: false},
		{name: "suffix collision denied", allowedHosts: []string{"example.com"}, url: "https://notexample.com", allowed: false},
		{name: "port is ignored", allowedHosts: []string{"gitea.internal"}, url: "https://gitea.internal:3000/api/v1", allowed: true},
		{name: "host comparison is case insensitive", allowedHosts: []string{"gitlab.example.com"}, url: "https://GitLab.Example.COM", allowed: true},
		{name: "allowlist entry case is normalised", allowedHosts: []string{"GitLab.Example.com"}, url: "https://gitlab.example.com", allowed: true},
		{name: "http is allowed when the host is", allowedHosts: []string{"gitea.internal"}, url: "http://gitea.internal", allowed: true},
		{name: "url without host denied", allowedHosts: []string{"gitea.internal"}, url: "not-a-url", allowed: false},
		{name: "empty url denied", allowedHosts: []string{"gitea.internal"}, url: "", allowed: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Mirrors FromConfig's normalisation without needing the config singleton.
			p := Policy{AllowedHosts: parseList(strings.Join(tc.allowedHosts, ","))}

			err := p.ValidateDestination(tc.url, "spec.provider.endpoint")
			if tc.allowed && err != nil {
				t.Fatalf("expected %q to be allowed, got %v", tc.url, err)
			}
			if !tc.allowed && err == nil {
				t.Fatalf("expected %q to be denied", tc.url)
			}
		})
	}
}

func TestValidateDestinationErrorNamesHostAndField(t *testing.T) {
	p := Policy{AllowedHosts: []string{"api.github.com"}}

	err := p.ValidateDestination("https://attacker.example.net/x", "spec.webhook.baseUrl")
	if err == nil {
		t.Fatal("expected denial")
	}
	// The message is surfaced verbatim to users (and, from the next step, on the
	// RenovateJob status), so it has to name both the field and the host to add.
	for _, want := range []string{"spec.webhook.baseUrl", "attacker.example.net", "policy.allowedHosts"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("expected error to mention %q, got: %v", want, err)
		}
	}
}

func TestValidateDestinationEmptyAllowlistExplainsItself(t *testing.T) {
	err := Policy{}.ValidateDestination("https://api.github.com", "spec.provider.endpoint")
	if err == nil {
		t.Fatal("expected denial with an empty allowlist")
	}
	if !strings.Contains(err.Error(), "no destinations are configured") {
		t.Errorf("expected the empty-allowlist hint, got: %v", err)
	}
}

func githubJob() *api.RenovateJob {
	job := &api.RenovateJob{}
	job.Spec.Provider = &api.RenovateProvider{Name: "github"}
	return job
}

func TestValidateJobDestinations(t *testing.T) {
	tests := []struct {
		name    string
		policy  Policy
		mutate  func(*api.RenovateJob)
		allowed bool
	}{
		{
			name:    "default github endpoint resolves to api.github.com",
			policy:  Policy{AllowedHosts: []string{"api.github.com"}},
			mutate:  func(*api.RenovateJob) {},
			allowed: true,
		},
		{
			name:    "default github endpoint denied when api host missing",
			policy:  Policy{AllowedHosts: []string{"github.com"}},
			mutate:  func(*api.RenovateJob) {},
			allowed: false,
		},
		{
			name:    "explicit endpoint on an allowed host",
			policy:  Policy{AllowedHosts: []string{"gitlab.example.com"}},
			mutate:  func(j *api.RenovateJob) { j.Spec.Provider.Endpoint = "https://gitlab.example.com/api/v4" },
			allowed: true,
		},
		{
			name:    "explicit endpoint on a foreign host",
			policy:  Policy{AllowedHosts: []string{"api.github.com"}},
			mutate:  func(j *api.RenovateJob) { j.Spec.Provider.Endpoint = "https://attacker.example.net" },
			allowed: false,
		},
		{
			name:    "publicEndpoint is checked",
			policy:  Policy{AllowedHosts: []string{"api.github.com"}},
			mutate:  func(j *api.RenovateJob) { j.Spec.Provider.PublicEndpoint = "https://attacker.example.net" },
			allowed: false,
		},
		{
			name:   "webhook baseUrl is checked",
			policy: Policy{AllowedHosts: []string{"api.github.com"}},
			mutate: func(j *api.RenovateJob) {
				j.Spec.Webhook = &api.RenovateWebhook{Enabled: true, BaseURL: "https://attacker.example.net"}
			},
			allowed: false,
		},
		{
			name:   "webhook baseUrl on an allowed host",
			policy: Policy{AllowedHosts: []string{"api.github.com", "renovate.example.com"}},
			mutate: func(j *api.RenovateJob) {
				j.Spec.Webhook = &api.RenovateWebhook{Enabled: true, BaseURL: "https://renovate.example.com"}
			},
			allowed: true,
		},
		{
			name:    "provider without a default endpoint has nothing to check",
			policy:  Policy{},
			mutate:  func(j *api.RenovateJob) { j.Spec.Provider = &api.RenovateProvider{Name: "gitea"} },
			allowed: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			job := githubJob()
			tc.mutate(job)

			err := tc.policy.ValidateJobDestinations(job)
			if tc.allowed && err != nil {
				t.Fatalf("expected job to be allowed, got %v", err)
			}
			if !tc.allowed && err == nil {
				t.Fatal("expected job to be denied")
			}
		})
	}
}

func TestValidateJobDestinationsNilJob(t *testing.T) {
	if err := (Policy{}).ValidateJobDestinations(nil); err != nil {
		t.Fatalf("expected nil job to be a no-op, got %v", err)
	}
}

// chartDefaultAllowedHosts mirrors policy.allowedHosts in
// charts/renovate-operator/values.yaml. If the chart ships a host the matcher
// rejects, a stock install breaks on upgrade, so the two are pinned together.
var chartDefaultAllowedHosts = []string{
	"api.github.com",
	"github.com",
	"gitlab.com",
	"api.bitbucket.org",
	"bitbucket.org",
	"gitea.com",
	"codeberg.org",
}

func TestChartDefaultsAreAccepted(t *testing.T) {
	p := Policy{AllowedHosts: chartDefaultAllowedHosts}

	for _, host := range chartDefaultAllowedHosts {
		if err := p.ValidateDestination("https://"+host, "chart default"); err != nil {
			t.Errorf("chart default host %q is rejected by the matcher: %v", host, err)
		}
	}

	// The stock provider defaults have to resolve inside that list.
	for _, provider := range []string{"github", "gitlab"} {
		job := &api.RenovateJob{}
		job.Spec.Provider = &api.RenovateProvider{Name: provider}
		if err := p.ValidateJobDestinations(job); err != nil {
			t.Errorf("stock %s job is rejected under the chart defaults: %v", provider, err)
		}
	}
}

func labeledSecret(name string, labels map[string]string) *corev1.Secret {
	return &corev1.Secret{
		Name: name, Namespace: "default", Labels: labels,
		Data: map[string][]byte{"password": []byte("s3cret")},
	}
}

func TestValidateReferencedSecret(t *testing.T) {
	tests := []struct {
		name    string
		policy  Policy
		secret  *corev1.Secret
		allowed bool
	}{
		{
			name:    "opted in",
			secret:  labeledSecret("webhook-token", map[string]string{api.LabelAllowRef: "true"}),
			allowed: true,
		},
		{
			name:    "no labels at all",
			secret:  labeledSecret("db-credentials", nil),
			allowed: false,
		},
		{
			name:    "other labels but not the opt-in",
			secret:  labeledSecret("db-credentials", map[string]string{"app": "postgres"}),
			allowed: false,
		},
		{
			// Only the exact string "true" opts in: "yes"/"1"/"" must not, or the
			// label becomes a guess about truthiness.
			name:    "label present but not true",
			secret:  labeledSecret("db-credentials", map[string]string{api.LabelAllowRef: "yes"}),
			allowed: false,
		},
		{
			name:    "label explicitly false",
			secret:  labeledSecret("db-credentials", map[string]string{api.LabelAllowRef: "false"}),
			allowed: false,
		},
		{
			name:    "opt-in requirement disabled",
			policy:  Policy{AllowUnlabeledSecretRefs: true},
			secret:  labeledSecret("db-credentials", nil),
			allowed: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.policy.ValidateReferencedSecret(tc.secret)
			if tc.allowed && err != nil {
				t.Fatalf("expected the reference to be allowed, got %v", err)
			}
			if !tc.allowed && err == nil {
				t.Fatal("expected the reference to be refused")
			}
			if err != nil && strings.Contains(err.Error(), "s3cret") {
				t.Error("the refusal must not echo the secret value it protected")
			}
		})
	}
}

// The zero Policy is what a caller that forgot to configure anything gets, so it
// has to be the strict one.
func TestZeroPolicyRequiresOptIn(t *testing.T) {
	if err := (Policy{}).ValidateReferencedSecret(labeledSecret("db-credentials", nil)); err == nil {
		t.Fatal("the zero Policy must require the opt-in label")
	}
}

func TestValidateReferencedSecretNamesLabelAndSecret(t *testing.T) {
	err := Policy{}.ValidateReferencedSecret(labeledSecret("db-credentials", nil))
	if err == nil {
		t.Fatal("expected refusal")
	}
	for _, want := range []string{"db-credentials", api.LabelAllowRef, "requireSecretRefOptIn"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("expected the error to mention %q, got: %v", want, err)
		}
	}
}

func TestValidateAllowedHosts(t *testing.T) {
	tests := []struct {
		name  string
		raw   string
		valid bool
	}{
		{name: "empty is valid and means deny-all", raw: "", valid: true},
		{name: "single hostname", raw: "gitlab.example.com", valid: true},
		{name: "several hostnames", raw: "api.github.com, github.com", valid: true},
		{name: "blank entries are tolerated", raw: "a.example.com,,", valid: true},
		{name: "mixed case is fine", raw: "GitLab.Example.com", valid: true},
		{name: "single label host", raw: "gitea", valid: true},
		// A leading dot used to mean "subtree". Now it matches nothing, so it must
		// be rejected loudly rather than quietly refusing every job under it.
		{name: "leading dot rejected", raw: ".example.com", valid: false},
		{name: "wildcard rejected", raw: "*.example.com", valid: false},
		{name: "scheme rejected", raw: "https://gitlab.example.com", valid: false},
		{name: "port rejected", raw: "gitea.internal:3000", valid: false},
		{name: "path rejected", raw: "gitlab.example.com/api/v4", valid: false},
		{name: "trailing dot rejected", raw: "example.com.", valid: false},
		{name: "one bad entry fails the whole list", raw: "api.github.com,.example.com", valid: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateAllowedHosts(tc.raw)
			if tc.valid && err != nil {
				t.Fatalf("expected %q to validate, got %v", tc.raw, err)
			}
			if !tc.valid && err == nil {
				t.Fatalf("expected %q to be rejected", tc.raw)
			}
		})
	}
}

func TestValidateAllowedHostsExplainsSubtreeRemoval(t *testing.T) {
	err := ValidateAllowedHosts(".example.com")
	if err == nil {
		t.Fatal("expected a leading-dot entry to be rejected")
	}
	if !strings.Contains(err.Error(), "list each hostname explicitly") {
		t.Errorf("expected the error to say what to do instead, got: %v", err)
	}
}

func TestChartDefaultsValidate(t *testing.T) {
	if err := ValidateAllowedHosts(strings.Join(chartDefaultAllowedHosts, ",")); err != nil {
		t.Fatalf("the hosts shipped in values.yaml must pass validation: %v", err)
	}
}

func TestParseList(t *testing.T) {
	tests := []struct {
		raw  string
		want []string
	}{
		{raw: "", want: nil},
		{raw: "   ", want: nil},
		{raw: ",,", want: nil},
		{raw: "a.example.com", want: []string{"a.example.com"}},
		{raw: " a.example.com , b.example.com ", want: []string{"a.example.com", "b.example.com"}},
		{raw: "A.Example.COM", want: []string{"a.example.com"}},
		{raw: "a.example.com,,b.example.com", want: []string{"a.example.com", "b.example.com"}},
	}

	for _, tc := range tests {
		got := parseList(tc.raw)
		if len(got) != len(tc.want) {
			t.Fatalf("parseList(%q) = %v, want %v", tc.raw, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("parseList(%q) = %v, want %v", tc.raw, got, tc.want)
			}
		}
	}
}

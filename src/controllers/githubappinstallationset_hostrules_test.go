package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"testing"
	"time"

	api "renovate-operator/api/v1alpha1"
	"renovate-operator/github"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type fakeTokenMinter struct {
	lifetime time.Duration
	err      error
	calls    int
}

func (f *fakeTokenMinter) MintInstallationToken(appID, installationID, pem, githubApi string) (string, time.Time, error) {
	f.calls++
	if f.err != nil {
		return "", time.Time{}, f.err
	}
	return "ghs_" + installationID + "_" + strconv.Itoa(f.calls), time.Now().Add(f.lifetime), nil
}

type renderedHostRule struct {
	MatchHost string `json:"matchHost"`
	HostType  string `json:"hostType"`
	Token     string `json:"token"`
}

func hostRulesSet(name string, filter *api.GitHubAppInstallationFilter) *api.GitHubAppInstallationSet {
	set := newInstallationSet(name)
	set.Spec.HostRules = &api.GitHubAppInstallationSetHostRules{Enabled: true, Installations: filter}
	return set
}

func hostRulesReconciler(c crclient.Client, lister *fakeInstallationLister, minter *fakeTokenMinter) *GitHubAppInstallationSetReconciler {
	r := installationSetReconciler(c, lister)
	r.Tokens = minter
	return r
}

func readHostRules(t *testing.T, c crclient.Client, setName string) []renderedHostRule {
	t.Helper()
	secret := getSecret(t, c, github.GetNameForGithubAppHostRulesSecret(setName))
	var cfg struct {
		HostRules []renderedHostRule `json:"hostRules"`
	}
	if err := json.Unmarshal(secret.Data[github.HostRulesConfigFileKey], &cfg); err != nil {
		t.Fatalf("hostRules config is not valid JSON: %v", err)
	}
	return cfg.HostRules
}

func matchHosts(rules []renderedHostRule) []string {
	hosts := make([]string, 0, len(rules))
	for _, r := range rules {
		hosts = append(hosts, r.MatchHost)
	}
	return hosts
}

func updateSetSpec(t *testing.T, c crclient.Client, name string, mutate func(*api.GitHubAppInstallationSet)) {
	t.Helper()
	set := getSet(t, c, name)
	mutate(set)
	set.Generation++
	if err := c.Update(context.Background(), set); err != nil {
		t.Fatalf("failed to update set: %v", err)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestHostRulesShareSelectedInstallationsIncludingOnesWithoutRenovateJob(t *testing.T) {
	set := hostRulesSet("club", &api.GitHubAppInstallationFilter{Exclude: []string{"Sandbox"}})
	set.Spec.Installations = &api.GitHubAppInstallationFilter{Include: []string{"org-a"}}
	c := installationSetClient(t, set, appCredentialsSecret(true))
	lister := &fakeInstallationLister{installations: []github.AppInstallation{
		{ID: 1, Account: "org-a", AccountType: api.GitHubAccountTypeOrganization},
		{ID: 2, Account: "presets", AccountType: api.GitHubAccountTypeOrganization},
		{ID: 3, Account: "sandbox", AccountType: api.GitHubAccountTypeOrganization},
		{ID: 4, Account: "frozen", AccountType: api.GitHubAccountTypeOrganization, Suspended: true},
	}}
	minter := &fakeTokenMinter{lifetime: time.Hour}

	reconcileSet(t, hostRulesReconciler(c, lister, minter), "club")

	rules := readHostRules(t, c, "club")
	want := []string{"https://api.github.com/repos/org-a/", "https://api.github.com/repos/presets/"}
	if got := matchHosts(rules); !equalStrings(got, want) {
		t.Fatalf("expected matchHosts %v, got %v", want, got)
	}
	for _, rule := range rules {
		if rule.HostType != "github" || rule.Token == "" {
			t.Errorf("expected a github rule with a token, got %#v", rule)
		}
	}

	secret := getSecret(t, c, github.GetNameForGithubAppHostRulesSecret("club"))
	if !metav1.IsControlledBy(secret, set) {
		t.Error("the hostRules secret must be controlled by the set")
	}
	if got := secret.Annotations[api.SharedInstallationsAnnotationKey]; got != "1=org-a,2=presets" {
		t.Errorf("unexpected shared installations annotation %q", got)
	}
	if secret.Labels[api.LabelAppComponent] != api.LabelValueComponentGitHubAppHostRules {
		t.Errorf("unexpected component label %q", secret.Labels[api.LabelAppComponent])
	}

	job := getJob(t, c, "club-org-a")
	if got := job.Spec.GithubAppReference.HostRulesSecretName; got != secret.Name {
		t.Errorf("generated RenovateJob must reference the hostRules secret, got %q", got)
	}
	if err := c.Get(context.Background(), k8stypes.NamespacedName{Name: "club-presets", Namespace: testSetNamespace}, &api.RenovateJob{}); err == nil {
		t.Error("an installation outside spec.installations must not get a RenovateJob")
	}
}

func TestHostRulesShareEveryInstallationWithoutFilter(t *testing.T) {
	c := installationSetClient(t, hostRulesSet("club", nil), appCredentialsSecret(true))
	lister := &fakeInstallationLister{installations: []github.AppInstallation{
		{ID: 2, Account: "org-b", AccountType: api.GitHubAccountTypeOrganization},
		{ID: 1, Account: "alice", AccountType: api.GitHubAccountTypeUser},
	}}

	reconcileSet(t, hostRulesReconciler(c, lister, &fakeTokenMinter{lifetime: time.Hour}), "club")

	want := []string{"https://api.github.com/repos/alice/", "https://api.github.com/repos/org-b/"}
	if got := matchHosts(readHostRules(t, c, "club")); !equalStrings(got, want) {
		t.Errorf("expected matchHosts %v, got %v", want, got)
	}
}

func TestHostRulesUseTheSetEndpoint(t *testing.T) {
	set := hostRulesSet("club", nil)
	set.Spec.Provider = &api.GitHubAppProvider{Endpoint: "https://ghe.example.com/api/v3/"}
	c := installationSetClient(t, set, appCredentialsSecret(true))
	lister := &fakeInstallationLister{installations: []github.AppInstallation{
		{ID: 1, Account: "org-a", AccountType: api.GitHubAccountTypeOrganization},
	}}

	reconcileSet(t, hostRulesReconciler(c, lister, &fakeTokenMinter{lifetime: time.Hour}), "club")

	want := []string{"https://ghe.example.com/api/v3/repos/org-a/"}
	if got := matchHosts(readHostRules(t, c, "club")); !equalStrings(got, want) {
		t.Errorf("expected matchHosts %v, got %v", want, got)
	}
}

func TestHostRulesAreNotMintedAgainWhileFresh(t *testing.T) {
	c := installationSetClient(t, hostRulesSet("club", nil), appCredentialsSecret(true))
	lister := &fakeInstallationLister{installations: []github.AppInstallation{
		{ID: 1, Account: "org-a", AccountType: api.GitHubAccountTypeOrganization},
	}}
	minter := &fakeTokenMinter{lifetime: time.Hour}
	r := hostRulesReconciler(c, lister, minter)

	reconcileSet(t, r, "club")
	reconcileSet(t, r, "club")

	if minter.calls != 1 {
		t.Errorf("fresh tokens must not be minted again, got %d mints", minter.calls)
	}
	if lister.calls != 1 {
		t.Errorf("fresh tokens must not list installations again, got %d listings", lister.calls)
	}
}

func TestHostRulesAreRenewedBeforeExpiryWithoutSync(t *testing.T) {
	c := installationSetClient(t, hostRulesSet("club", nil), appCredentialsSecret(true))
	lister := &fakeInstallationLister{installations: []github.AppInstallation{
		{ID: 1, Account: "org-a", AccountType: api.GitHubAccountTypeOrganization},
	}}
	minter := &fakeTokenMinter{lifetime: 20 * time.Minute}
	r := hostRulesReconciler(c, lister, minter)

	reconcileSet(t, r, "club")
	first := readHostRules(t, c, "club")[0].Token
	reconcileSet(t, r, "club")

	if minter.calls != 2 {
		t.Fatalf("a token expiring within %s must be renewed, got %d mints", hostRulesRenewBefore, minter.calls)
	}
	if lister.calls != 2 {
		t.Errorf("renewal outside a sync must list the installations, got %d listings", lister.calls)
	}
	if readHostRules(t, c, "club")[0].Token == first {
		t.Error("the renewed token must replace the expiring one")
	}
}

func TestHostRulesAreRebuiltWhenSelectionChanges(t *testing.T) {
	c := installationSetClient(t, hostRulesSet("club", nil), appCredentialsSecret(true))
	lister := &fakeInstallationLister{installations: []github.AppInstallation{
		{ID: 1, Account: "org-a", AccountType: api.GitHubAccountTypeOrganization},
	}}
	r := hostRulesReconciler(c, lister, &fakeTokenMinter{lifetime: time.Hour})
	reconcileSet(t, r, "club")

	lister.installations = append(lister.installations, github.AppInstallation{ID: 2, Account: "org-b", AccountType: api.GitHubAccountTypeOrganization})
	setAnnotationForTest(t, c, "club", api.TriggerResyncAnnotationKey, "true")
	reconcileSet(t, r, "club")

	want := []string{"https://api.github.com/repos/org-a/", "https://api.github.com/repos/org-b/"}
	if got := matchHosts(readHostRules(t, c, "club")); !equalStrings(got, want) {
		t.Fatalf("a new installation must be shared on the next sync, got %v", got)
	}

	updateSetSpec(t, c, "club", func(set *api.GitHubAppInstallationSet) {
		set.Spec.HostRules.Installations = &api.GitHubAppInstallationFilter{Exclude: []string{"org-a"}}
	})
	reconcileSet(t, r, "club")

	want = []string{"https://api.github.com/repos/org-b/"}
	if got := matchHosts(readHostRules(t, c, "club")); !equalStrings(got, want) {
		t.Errorf("an excluded installation must leave the hostRules, got %v", got)
	}
}

func TestHostRulesDisabledEmptiesSecretAndUnmountsIt(t *testing.T) {
	c := installationSetClient(t, hostRulesSet("club", nil), appCredentialsSecret(true))
	lister := &fakeInstallationLister{installations: []github.AppInstallation{
		{ID: 1, Account: "org-a", AccountType: api.GitHubAccountTypeOrganization},
	}}
	minter := &fakeTokenMinter{lifetime: time.Hour}
	r := hostRulesReconciler(c, lister, minter)
	reconcileSet(t, r, "club")

	updateSetSpec(t, c, "club", func(set *api.GitHubAppInstallationSet) {
		set.Spec.HostRules.Enabled = false
	})
	reconcileSet(t, r, "club")

	secret := getSecret(t, c, github.GetNameForGithubAppHostRulesSecret("club"))
	if len(secret.Data) != 0 {
		t.Errorf("disabled hostRules must drop the shared tokens, got keys %v", secret.Data)
	}
	if _, ok := secret.Annotations[api.TokenExpiresAtAnnotationKey]; ok {
		t.Error("an emptied hostRules secret must lose its expiry")
	}
	if got := getJob(t, c, "club-org-a").Spec.GithubAppReference.HostRulesSecretName; got != "" {
		t.Errorf("generated RenovateJob must stop mounting hostRules, got %q", got)
	}

	updateSetSpec(t, c, "club", func(set *api.GitHubAppInstallationSet) {
		set.Spec.HostRules.Enabled = true
	})
	reconcileSet(t, r, "club")
	if len(readHostRules(t, c, "club")) != 1 || minter.calls != 2 {
		t.Errorf("re-enabled hostRules must be minted anew, got %d mints", minter.calls)
	}
}

func TestHostRulesMintFailureKeepsSecretAndReportsNotReady(t *testing.T) {
	c := installationSetClient(t, hostRulesSet("club", nil), appCredentialsSecret(true))
	lister := &fakeInstallationLister{installations: []github.AppInstallation{
		{ID: 1, Account: "org-a", AccountType: api.GitHubAccountTypeOrganization},
	}}
	minter := &fakeTokenMinter{lifetime: 20 * time.Minute}
	r := hostRulesReconciler(c, lister, minter)
	reconcileSet(t, r, "club")
	before := readHostRules(t, c, "club")

	minter.err = errors.New("github is down")
	res := reconcileSet(t, r, "club")

	after := readHostRules(t, c, "club")
	if len(after) != 1 || after[0].Token != before[0].Token {
		t.Errorf("a failed renewal must keep the previous tokens, got %#v", after)
	}
	cond := meta.FindStatusCondition(getSet(t, c, "club").Status.Conditions, api.ConditionReady)
	if cond == nil || cond.Status != metav1.ConditionFalse || cond.Reason != ReasonPartiallySynced {
		t.Errorf("expected Ready=False/%s, got %#v", ReasonPartiallySynced, cond)
	}
	if res.RequeueAfter != installationRetryInterval {
		t.Errorf("expected retry after %s, got %s", installationRetryInterval, res.RequeueAfter)
	}
	getJob(t, c, "club-org-a")
}

func TestHostRulesRevokedWithCredentials(t *testing.T) {
	set := hostRulesSet("club", nil)
	source := appCredentialsSecret(true)
	c := installationSetClient(t, set, source)
	lister := &fakeInstallationLister{installations: []github.AppInstallation{
		{ID: 1, Account: "org-a", AccountType: api.GitHubAccountTypeOrganization},
	}}
	r := hostRulesReconciler(c, lister, &fakeTokenMinter{lifetime: time.Hour})
	r.Policy.Disabled = false
	r.Policy.AllowedHosts = []string{"api.github.com"}
	reconcileSet(t, r, "club")

	unlabelSecret(t, c, source)
	reconcileSet(t, r, "club")

	if secret := getSecret(t, c, github.GetNameForGithubAppHostRulesSecret("club")); len(secret.Data) != 0 {
		t.Errorf("shared tokens must be revoked with the credentials, got keys %v", secret.Data)
	}
}

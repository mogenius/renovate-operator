package renovate

import (
	"context"
	"errors"
	"strings"
	"testing"

	api "renovate-operator/api/v1alpha1"
	"renovate-operator/gitProviderClients"
	"renovate-operator/internal/policy"
)

type fakePropertyClient struct {
	byProperty map[string][]string
	err        error
}

func (f *fakePropertyClient) GetRepositoryInfo(context.Context, string) (gitProviderClients.RepositoryInfo, error) {
	return gitProviderClients.RepositoryInfo{}, nil
}
func (f *fakePropertyClient) ListRepositoriesByProperty(_ context.Context, name string, values []string) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	var out []string
	for _, v := range values {
		out = append(out, f.byProperty[name+"="+v]...)
	}
	return out, nil
}
func (f *fakePropertyClient) ListRepoWebhooks(context.Context, string) ([]gitProviderClients.Webhook, error) {
	return nil, nil
}
func (f *fakePropertyClient) CreateRepoWebhook(context.Context, string, gitProviderClients.CreateWebhookOptions) (*gitProviderClients.Webhook, error) {
	return nil, nil
}
func (f *fakePropertyClient) UpdateRepoWebhook(context.Context, string, string, gitProviderClients.CreateWebhookOptions) (*gitProviderClients.Webhook, error) {
	return nil, nil
}
func (f *fakePropertyClient) DeleteRepoWebhook(context.Context, string, string) error { return nil }

type fakeFactory struct {
	client gitProviderClients.GitProviderClient
}

func (f *fakeFactory) NewClient(context.Context, *api.RenovateJob) (gitProviderClients.GitProviderClient, error) {
	return f.client, nil
}
func (f *fakeFactory) NewClientWithTokenRef(context.Context, *api.RenovateJob, *api.RenovateSecretKeyReference) (gitProviderClients.GitProviderClient, error) {
	return f.client, nil
}

func propertyJob(props []api.DiscoveryProperty, filters []string) *api.RenovateJob {
	return &api.RenovateJob{Name: "rj", Namespace: "ns", Spec: api.RenovateJobSpec{
		Image:               "img",
		Provider:            &api.RenovateProvider{Name: "github"},
		DiscoveryProperties: props,
		DiscoveryFilters:    filters,
	}}
}

func TestResolveDiscoveryProperties_UnionAndDedup(t *testing.T) {
	client := &fakePropertyClient{byProperty: map[string][]string{
		"owning-team=software-platform": {"org/a", "org/b"},
		"owning-team=gnc":               {"org/b", "org/c"},
		"tier=gold":                     {"org/c", "org/d"},
	}}
	da := &discoveryAgent{policy: policy.Policy{}, providers: &fakeFactory{client: client}}
	got, err := da.resolveDiscoveryProperties(context.Background(), propertyJob([]api.DiscoveryProperty{
		{Name: "owning-team", Values: []string{"software-platform", "gnc"}},
		{Name: "tier", Values: []string{"gold"}},
	}, nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Join(got, ",") != "org/a,org/b,org/c,org/d" {
		t.Fatalf("expected a,b,c,d once each, got %v", got)
	}
}

func TestResolveDiscoveryProperties_RefusesUnfilteredDiscovery(t *testing.T) {
	da := &discoveryAgent{providers: &fakeFactory{client: &fakePropertyClient{}}}
	_, err := da.resolveDiscoveryProperties(context.Background(), propertyJob([]api.DiscoveryProperty{{Name: "owning-team", Values: []string{"nobody"}}}, nil))
	if err == nil || !strings.Contains(err.Error(), "refusing") {
		t.Fatalf("expected a refusal when nothing resolves and no filters exist, got %v", err)
	}
	// with explicit filters present an empty resolution is fine: the filters still bound the run
	got, err := da.resolveDiscoveryProperties(context.Background(), propertyJob([]api.DiscoveryProperty{{Name: "owning-team", Values: []string{"nobody"}}}, []string{"org/explicit"}))
	if err != nil || len(got) != 0 {
		t.Fatalf("expected empty resolution without error, got %v %v", got, err)
	}
}

func TestResolveDiscoveryProperties_UnsupportedPlatform(t *testing.T) {
	da := &discoveryAgent{providers: &fakeFactory{client: &fakePropertyClient{err: gitProviderClients.ErrCustomPropertiesUnsupported}}}
	_, err := da.resolveDiscoveryProperties(context.Background(), propertyJob([]api.DiscoveryProperty{{Name: "owning-team", Values: []string{"x"}}}, nil))
	if !errors.Is(err, gitProviderClients.ErrCustomPropertiesUnsupported) {
		t.Fatalf("expected ErrCustomPropertiesUnsupported, got %v", err)
	}
}

func TestResolveDiscoveryProperties_NoopWithoutProperties(t *testing.T) {
	da := &discoveryAgent{}
	got, err := da.resolveDiscoveryProperties(context.Background(), propertyJob(nil, []string{"org/*"}))
	if err != nil || got != nil {
		t.Fatalf("expected nil, nil; got %v %v", got, err)
	}
}

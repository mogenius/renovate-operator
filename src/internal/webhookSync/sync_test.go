package webhookSync

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"renovate-operator/gitProviderClients"
	"strconv"

	"github.com/go-logr/logr"
)

type fakeClient struct {
	mu sync.Mutex
	// hooks per project full name
	hooks  map[string][]gitProviderClients.Webhook
	nextID int

	listErr   map[string]error
	createErr map[string]error
	deleteErr map[string]error

	created []string
	updated []string
	deleted []string
}

func newFakeClient() *fakeClient {
	return &fakeClient{
		hooks:  make(map[string][]gitProviderClients.Webhook),
		nextID: 1,
	}
}

func (f *fakeClient) GetRepositoryInfo(ctx context.Context, project string) (gitProviderClients.RepositoryInfo, error) {
	return gitProviderClients.RepositoryInfo{}, nil
}

func (f *fakeClient) ListRepoWebhooks(ctx context.Context, project string) ([]gitProviderClients.Webhook, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.listErr[project]; err != nil {
		return nil, err
	}
	return append([]gitProviderClients.Webhook(nil), f.hooks[project]...), nil
}

func (f *fakeClient) CreateRepoWebhook(ctx context.Context, project string, opts gitProviderClients.CreateWebhookOptions) (*gitProviderClients.Webhook, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.createErr[project]; err != nil {
		return nil, err
	}
	hook := gitProviderClients.Webhook{ID: strconv.Itoa(f.nextID), URL: opts.URL, Active: opts.Active, EventsUpToDate: true}
	f.nextID++
	f.hooks[project] = append(f.hooks[project], hook)
	f.created = append(f.created, project)
	return &hook, nil
}

func (f *fakeClient) UpdateRepoWebhook(ctx context.Context, project string, hookID string, opts gitProviderClients.CreateWebhookOptions) (*gitProviderClients.Webhook, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	hooks := f.hooks[project]
	for i, hook := range hooks {
		if hook.ID == hookID {
			hooks[i] = gitProviderClients.Webhook{ID: hookID, URL: opts.URL, Active: opts.Active, EventsUpToDate: true}
			f.updated = append(f.updated, project)
			return &hooks[i], nil
		}
	}
	return nil, fmt.Errorf("hook %s not found", hookID)
}

func (f *fakeClient) DeleteRepoWebhook(ctx context.Context, project string, hookID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.deleteErr[project]; err != nil {
		return err
	}
	hooks := f.hooks[project]
	for i, hook := range hooks {
		if hook.ID == hookID {
			f.hooks[project] = append(hooks[:i], hooks[i+1:]...)
			f.deleted = append(f.deleted, project)
			return nil
		}
	}
	return fmt.Errorf("hook %s not found", hookID)
}

var testOpts = Options{WebhookURL: "https://operator.example.com/webhook/v1/forgejo?job=a&namespace=b"}

func TestSyncCreatesMissingWebhooks(t *testing.T) {
	client := newFakeClient()

	Sync(context.Background(), logr.Discard(), client, testOpts, []string{"org/a", "org/b"}, nil)

	for _, project := range []string{"org/a", "org/b"} {
		if len(client.hooks[project]) != 1 {
			t.Errorf("expected 1 hook on %s, got %d", project, len(client.hooks[project]))
		}
	}
}

func TestSyncReusesExistingWebhook(t *testing.T) {
	client := newFakeClient()
	client.hooks["org/a"] = []gitProviderClients.Webhook{
		{ID: "7", URL: testOpts.WebhookURL, Active: true, EventsUpToDate: true},
		{ID: "8", URL: "https://something-else.example.com"},
	}

	Sync(context.Background(), logr.Discard(), client, testOpts, []string{"org/a"}, nil)

	if len(client.created) != 0 {
		t.Errorf("expected no hook creation, got %v", client.created)
	}
	if len(client.updated) != 0 {
		t.Errorf("expected no hook update for matching config, got %v", client.updated)
	}
}

func TestSyncUpdatesDriftedWebhook(t *testing.T) {
	client := newFakeClient()
	// over-subscribed hook, e.g. created before event narrowing existed
	client.hooks["org/a"] = []gitProviderClients.Webhook{
		{ID: "7", URL: testOpts.WebhookURL, Active: true, EventsUpToDate: false},
	}

	Sync(context.Background(), logr.Discard(), client, testOpts, []string{"org/a"}, nil)

	if len(client.created) != 0 {
		t.Errorf("expected no hook creation, got %v", client.created)
	}
	if len(client.updated) != 1 {
		t.Fatalf("expected drifted hook to be updated, got %v", client.updated)
	}
	hook := client.hooks["org/a"][0]
	if !hook.EventsUpToDate || !hook.Active {
		t.Errorf("expected hook reset to the provider subscription and active, got %+v", hook)
	}
}

func TestSyncUpdatesInactiveWebhook(t *testing.T) {
	client := newFakeClient()
	client.hooks["org/a"] = []gitProviderClients.Webhook{
		{ID: "7", URL: testOpts.WebhookURL, Active: false, EventsUpToDate: true},
	}

	Sync(context.Background(), logr.Discard(), client, testOpts, []string{"org/a"}, nil)

	if len(client.updated) != 1 {
		t.Fatalf("expected inactive hook to be updated, got %v", client.updated)
	}
	if !client.hooks["org/a"][0].Active {
		t.Error("expected hook to be re-activated")
	}
}

func TestSyncRemovesHookFromRemovedRepos(t *testing.T) {
	client := newFakeClient()
	client.hooks["org/old"] = []gitProviderClients.Webhook{
		{ID: "3", URL: testOpts.WebhookURL},
		{ID: "4", URL: "https://something-else.example.com"},
	}

	Sync(context.Background(), logr.Discard(), client, testOpts, []string{"org/new"}, []string{"org/old"})

	if len(client.hooks["org/old"]) != 1 || client.hooks["org/old"][0].ID != "4" {
		t.Errorf("expected only the operator's hook removed from org/old, got %+v", client.hooks["org/old"])
	}
	if len(client.hooks["org/new"]) != 1 {
		t.Error("expected hook ensured on org/new")
	}
}

func TestSyncRemovalSkipsForeignHooks(t *testing.T) {
	client := newFakeClient()
	client.hooks["org/old"] = []gitProviderClients.Webhook{
		{ID: "4", URL: "https://something-else.example.com"},
	}

	Sync(context.Background(), logr.Discard(), client, testOpts, nil, []string{"org/old"})

	if len(client.deleted) != 0 {
		t.Errorf("expected no deletion when no hook matches the operator URL, got %v", client.deleted)
	}
}

func TestSyncRemovesAllWhenDisabled(t *testing.T) {
	client := newFakeClient()
	client.hooks["org/a"] = []gitProviderClients.Webhook{{ID: "1", URL: testOpts.WebhookURL}}
	client.hooks["org/b"] = []gitProviderClients.Webhook{{ID: "2", URL: testOpts.WebhookURL}}

	// the caller passes all current projects as removed when sync is disabled
	Sync(context.Background(), logr.Discard(), client, testOpts, nil, []string{"org/a", "org/b"})

	if len(client.hooks["org/a"]) != 0 || len(client.hooks["org/b"]) != 0 {
		t.Error("expected all operator hooks deleted")
	}
}

func TestSyncContinuesAfterEnsureFailure(t *testing.T) {
	client := newFakeClient()
	client.listErr = map[string]error{"org/a": fmt.Errorf("boom")}

	Sync(context.Background(), logr.Discard(), client, testOpts, []string{"org/a", "org/b"}, nil)

	if len(client.hooks["org/b"]) != 1 {
		t.Error("expected org/b to be ensured despite org/a failing")
	}
}

func TestSyncRemovalFailureDoesNotPanic(t *testing.T) {
	client := newFakeClient()
	client.hooks["org/old"] = []gitProviderClients.Webhook{{ID: "3", URL: testOpts.WebhookURL}}
	client.deleteErr = map[string]error{"org/old": fmt.Errorf("boom")}

	// the failure is logged; the hook stays orphaned (documented behavior)
	Sync(context.Background(), logr.Discard(), client, testOpts, nil, []string{"org/old"})

	if len(client.hooks["org/old"]) != 1 {
		t.Error("expected hook to remain when deletion fails")
	}
}

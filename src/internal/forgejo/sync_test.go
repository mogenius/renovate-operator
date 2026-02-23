package forgejo

import (
	"context"
	"fmt"
	"testing"

	"github.com/go-logr/logr"
)

type mockClient struct {
	repos    []Repository
	hooks    map[string][]Webhook // "owner/repo" -> hooks
	created  map[string][]CreateWebhookOptions
	deleted  map[string][]int64
	nextHookID int64
	searchErr error
	listErr   map[string]error
	createErr map[string]error
	deleteErr map[string]error
}

func newMockClient() *mockClient {
	return &mockClient{
		hooks:   make(map[string][]Webhook),
		created: make(map[string][]CreateWebhookOptions),
		deleted: make(map[string][]int64),
		listErr: make(map[string]error),
		createErr: make(map[string]error),
		deleteErr: make(map[string]error),
		nextHookID: 100,
	}
}

func (m *mockClient) SearchReposByTopic(_ context.Context, _ string) ([]Repository, error) {
	if m.searchErr != nil {
		return nil, m.searchErr
	}
	return m.repos, nil
}

func (m *mockClient) ListRepoWebhooks(_ context.Context, owner, repo string) ([]Webhook, error) {
	key := owner + "/" + repo
	if err, ok := m.listErr[key]; ok {
		return nil, err
	}
	return m.hooks[key], nil
}

func (m *mockClient) CreateRepoWebhook(_ context.Context, owner, repo string, opts CreateWebhookOptions) (*Webhook, error) {
	key := owner + "/" + repo
	if err, ok := m.createErr[key]; ok {
		return nil, err
	}
	m.created[key] = append(m.created[key], opts)
	m.nextHookID++
	hook := &Webhook{ID: m.nextHookID, Config: opts.Config, Events: opts.Events}
	m.hooks[key] = append(m.hooks[key], *hook)
	return hook, nil
}

func (m *mockClient) DeleteRepoWebhook(_ context.Context, owner, repo string, hookID int64) error {
	key := owner + "/" + repo
	if err, ok := m.deleteErr[key]; ok {
		return err
	}
	m.deleted[key] = append(m.deleted[key], hookID)
	return nil
}

func TestSyncCreatesWebhooksOnNewRepos(t *testing.T) {
	mc := newMockClient()
	mc.repos = []Repository{
		{ID: 1, FullName: "org/repo1", Name: "repo1", Owner: struct{ Login string `json:"login"` }{Login: "org"}, Permissions: &RepositoryPermissions{Admin: true}},
		{ID: 2, FullName: "org/repo2", Name: "repo2", Owner: struct{ Login string `json:"login"` }{Login: "org"}, Permissions: &RepositoryPermissions{Admin: true}},
	}

	syncer := NewWebhookSyncer(mc, "https://webhook.example.com/hook", "secret-token", "renovate", nil, logr.Discard())

	err := syncer.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mc.created["org/repo1"]) != 1 {
		t.Errorf("expected 1 webhook created for org/repo1, got %d", len(mc.created["org/repo1"]))
	}
	if len(mc.created["org/repo2"]) != 1 {
		t.Errorf("expected 1 webhook created for org/repo2, got %d", len(mc.created["org/repo2"]))
	}

	// Verify auth header
	if mc.created["org/repo1"][0].Config.AuthorizationHeader != "Bearer secret-token" {
		t.Errorf("expected Bearer auth header in config, got %q", mc.created["org/repo1"][0].Config.AuthorizationHeader)
	}
}

func TestSyncSkipsReposWithExistingWebhook(t *testing.T) {
	mc := newMockClient()
	mc.repos = []Repository{
		{ID: 1, FullName: "org/repo1", Name: "repo1", Owner: struct{ Login string `json:"login"` }{Login: "org"}, Permissions: &RepositoryPermissions{Admin: true}},
	}
	mc.hooks["org/repo1"] = []Webhook{
		{ID: 99, Config: WebhookConfig{URL: "https://webhook.example.com/hook"}},
	}

	syncer := NewWebhookSyncer(mc, "https://webhook.example.com/hook", "", "renovate", nil, logr.Discard())

	err := syncer.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mc.created["org/repo1"]) != 0 {
		t.Errorf("expected no webhooks created, got %d", len(mc.created["org/repo1"]))
	}

	// Verify the existing hook was tracked
	if syncer.managedRepos["org/repo1"] != 99 {
		t.Errorf("expected managed hook ID 99, got %d", syncer.managedRepos["org/repo1"])
	}
}

func TestSyncRemovesWebhookWhenRepoLosesTopic(t *testing.T) {
	mc := newMockClient()
	mc.repos = []Repository{
		{ID: 1, FullName: "org/repo1", Name: "repo1", Owner: struct{ Login string `json:"login"` }{Login: "org"}, Permissions: &RepositoryPermissions{Admin: true}},
		{ID: 2, FullName: "org/repo2", Name: "repo2", Owner: struct{ Login string `json:"login"` }{Login: "org"}, Permissions: &RepositoryPermissions{Admin: true}},
	}

	syncer := NewWebhookSyncer(mc, "https://webhook.example.com/hook", "", "renovate", nil, logr.Discard())

	// First run: create webhooks on both repos
	err := syncer.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("first run error: %v", err)
	}

	repo2HookID := syncer.managedRepos["org/repo2"]
	if repo2HookID == 0 {
		t.Fatal("expected repo2 to have a managed hook")
	}

	// Second run: repo2 loses the topic
	mc.repos = []Repository{
		{ID: 1, FullName: "org/repo1", Name: "repo1", Owner: struct{ Login string `json:"login"` }{Login: "org"}, Permissions: &RepositoryPermissions{Admin: true}},
	}
	mc.hooks["org/repo1"] = []Webhook{
		{ID: syncer.managedRepos["org/repo1"], Config: WebhookConfig{URL: "https://webhook.example.com/hook"}},
	}

	err = syncer.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("second run error: %v", err)
	}

	if len(mc.deleted["org/repo2"]) != 1 {
		t.Errorf("expected 1 deletion for org/repo2, got %d", len(mc.deleted["org/repo2"]))
	}
	if mc.deleted["org/repo2"][0] != repo2HookID {
		t.Errorf("expected deletion of hook %d, got %d", repo2HookID, mc.deleted["org/repo2"][0])
	}
	if _, exists := syncer.managedRepos["org/repo2"]; exists {
		t.Error("expected org/repo2 to be removed from managedRepos")
	}
}

func TestSyncLogsErrorWhenAdminLostButTopicRemains(t *testing.T) {
	mc := newMockClient()
	mc.repos = []Repository{
		{ID: 1, FullName: "org/repo1", Name: "repo1", Owner: struct{ Login string `json:"login"` }{Login: "org"}, Permissions: &RepositoryPermissions{Admin: true}},
	}

	syncer := NewWebhookSyncer(mc, "https://webhook.example.com/hook", "", "renovate", nil, logr.Discard())

	// First run: create webhook
	err := syncer.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("first run error: %v", err)
	}
	if len(mc.created["org/repo1"]) != 1 {
		t.Fatalf("expected webhook created on first run")
	}

	// Second run: repo still has topic but we lost admin
	mc.repos = []Repository{
		{ID: 1, FullName: "org/repo1", Name: "repo1", Owner: struct{ Login string `json:"login"` }{Login: "org"}, Permissions: &RepositoryPermissions{Admin: false}},
	}

	err = syncer.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("second run error: %v", err)
	}

	// Should NOT attempt to delete (no admin access to do so)
	if len(mc.deleted["org/repo1"]) != 0 {
		t.Errorf("expected no deletion attempts (no admin access), got %d", len(mc.deleted["org/repo1"]))
	}

	// Should remove from managedRepos (we can't manage it anymore)
	if _, exists := syncer.managedRepos["org/repo1"]; exists {
		t.Error("expected org/repo1 to be removed from managedRepos after losing admin")
	}
}

func TestSyncSkipsReposWithoutAdminPermission(t *testing.T) {
	mc := newMockClient()
	mc.repos = []Repository{
		{ID: 1, FullName: "org/admin-repo", Name: "admin-repo", Owner: struct{ Login string `json:"login"` }{Login: "org"}, Permissions: &RepositoryPermissions{Admin: true}},
		{ID: 2, FullName: "org/no-admin", Name: "no-admin", Owner: struct{ Login string `json:"login"` }{Login: "org"}, Permissions: &RepositoryPermissions{Admin: false}},
		{ID: 3, FullName: "org/nil-perms", Name: "nil-perms", Owner: struct{ Login string `json:"login"` }{Login: "org"}, Permissions: nil},
	}

	syncer := NewWebhookSyncer(mc, "https://webhook.example.com/hook", "", "renovate", nil, logr.Discard())

	err := syncer.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mc.created["org/admin-repo"]) != 1 {
		t.Errorf("expected webhook on admin-repo, got %d", len(mc.created["org/admin-repo"]))
	}
	if len(mc.created["org/no-admin"]) != 0 {
		t.Error("expected no webhook on no-admin repo")
	}
	if len(mc.created["org/nil-perms"]) != 0 {
		t.Error("expected no webhook on nil-perms repo")
	}
}

func TestSyncHandlesAPIErrorsWithoutAborting(t *testing.T) {
	mc := newMockClient()
	mc.repos = []Repository{
		{ID: 1, FullName: "org/repo1", Name: "repo1", Owner: struct{ Login string `json:"login"` }{Login: "org"}, Permissions: &RepositoryPermissions{Admin: true}},
		{ID: 2, FullName: "org/repo2", Name: "repo2", Owner: struct{ Login string `json:"login"` }{Login: "org"}, Permissions: &RepositoryPermissions{Admin: true}},
	}
	mc.listErr["org/repo1"] = fmt.Errorf("connection refused")

	syncer := NewWebhookSyncer(mc, "https://webhook.example.com/hook", "", "renovate", nil, logr.Discard())

	err := syncer.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// repo1 should have failed, but repo2 should succeed
	if len(mc.created["org/repo2"]) != 1 {
		t.Errorf("expected webhook on repo2 despite repo1 failure, got %d", len(mc.created["org/repo2"]))
	}
}

func TestSyncStateRebuiltOnFirstRun(t *testing.T) {
	mc := newMockClient()
	mc.repos = []Repository{
		{ID: 1, FullName: "org/repo1", Name: "repo1", Owner: struct{ Login string `json:"login"` }{Login: "org"}, Permissions: &RepositoryPermissions{Admin: true}},
	}
	// Simulate existing webhook (created before syncer existed)
	mc.hooks["org/repo1"] = []Webhook{
		{ID: 55, Config: WebhookConfig{URL: "https://webhook.example.com/hook"}},
	}

	syncer := NewWebhookSyncer(mc, "https://webhook.example.com/hook", "", "renovate", nil, logr.Discard())

	err := syncer.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should detect existing webhook and track it without creating a new one
	if len(mc.created["org/repo1"]) != 0 {
		t.Errorf("expected no new webhooks, got %d", len(mc.created["org/repo1"]))
	}
	if syncer.managedRepos["org/repo1"] != 55 {
		t.Errorf("expected tracked hook ID 55, got %d", syncer.managedRepos["org/repo1"])
	}

	// Now remove the topic
	mc.repos = nil
	err = syncer.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should remove the webhook
	if len(mc.deleted["org/repo1"]) != 1 || mc.deleted["org/repo1"][0] != 55 {
		t.Errorf("expected deletion of hook 55, got %v", mc.deleted["org/repo1"])
	}
}

func TestSyncDefaultEvents(t *testing.T) {
	mc := newMockClient()
	mc.repos = []Repository{
		{ID: 1, FullName: "org/repo1", Name: "repo1", Owner: struct{ Login string `json:"login"` }{Login: "org"}, Permissions: &RepositoryPermissions{Admin: true}},
	}

	syncer := NewWebhookSyncer(mc, "https://webhook.example.com/hook", "", "renovate", nil, logr.Discard())

	err := syncer.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mc.created["org/repo1"]) != 1 {
		t.Fatalf("expected 1 webhook, got %d", len(mc.created["org/repo1"]))
	}
	events := mc.created["org/repo1"][0].Events
	if len(events) != 2 || events[0] != "issues" || events[1] != "pull_request" {
		t.Errorf("expected default events [issues, pull_request], got %v", events)
	}
}

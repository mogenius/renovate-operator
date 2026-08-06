package crdmanager

import (
	"context"
	"strings"
	"sync"
	"testing"

	api "renovate-operator/api/v1alpha1"
	"renovate-operator/gitProviderClients"
	"renovate-operator/internal/policy"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// recordingProvider notes which platform calls a sync cycle actually made, so a
// test can distinguish "refused before touching the platform" from "ran anyway".
type recordingProvider struct {
	mu      sync.Mutex
	listed  []string
	created []string
	deleted []string
	hooks   map[string][]gitProviderClients.Webhook
}

func (p *recordingProvider) GetRepositoryInfo(context.Context, string) (gitProviderClients.RepositoryInfo, error) {
	return gitProviderClients.RepositoryInfo{}, nil
}

func (p *recordingProvider) ListRepoWebhooks(_ context.Context, project string) ([]gitProviderClients.Webhook, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.listed = append(p.listed, project)
	return p.hooks[project], nil
}

func (p *recordingProvider) CreateRepoWebhook(_ context.Context, project string, opts gitProviderClients.CreateWebhookOptions) (*gitProviderClients.Webhook, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.created = append(p.created, project)
	return &gitProviderClients.Webhook{ID: "1", URL: opts.URL, Active: true, EventsUpToDate: true}, nil
}

func (p *recordingProvider) UpdateRepoWebhook(_ context.Context, _ string, hookID string, opts gitProviderClients.CreateWebhookOptions) (*gitProviderClients.Webhook, error) {
	return &gitProviderClients.Webhook{ID: hookID, URL: opts.URL, Active: true, EventsUpToDate: true}, nil
}

func (p *recordingProvider) DeleteRepoWebhook(_ context.Context, project string, _ string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.deleted = append(p.deleted, project)
	return nil
}

type stubFactory struct {
	provider gitProviderClients.GitProviderClient
}

func (f stubFactory) NewClient(context.Context, *api.RenovateJob) (gitProviderClients.GitProviderClient, error) {
	return f.provider, nil
}

func (f stubFactory) NewClientWithTokenRef(context.Context, *api.RenovateJob, *api.RenovateSecretKeyReference) (gitProviderClients.GitProviderClient, error) {
	return f.provider, nil
}

func syncManager(t *testing.T, provider gitProviderClients.GitProviderClient, allowedHosts ...string) *renovateJobManager {
	t.Helper()
	return &renovateJobManager{
		gitProviderClientFactory: stubFactory{provider: provider},
		logger:                   logr.Discard(),
		lock:                     &sync.RWMutex{},
		policy:                   policy.Policy{AllowedHosts: allowedHosts},
	}
}

func TestRunWebhookSyncRefusesWritesToForeignBaseURL(t *testing.T) {
	setBaseURL(t, "")
	provider := &recordingProvider{hooks: map[string][]gitProviderClients.Webhook{}}
	mgr := syncManager(t, provider, "renovate.example.com")

	job := syncJob("forgejo")
	job.Spec.Webhook.BaseURL = "https://attacker.example.net"

	err := mgr.runWebhookSync(context.Background(), job, RenovateJobIdentifier{Name: "j", Namespace: "n"}, []string{"org/a"}, nil)
	if err == nil {
		t.Fatal("expected the sync to be refused for a non-allowlisted delivery host")
	}
	if !strings.Contains(err.Error(), "attacker.example.net") {
		t.Errorf("expected the denied host in the error, got: %v", err)
	}
	if len(provider.listed) != 0 || len(provider.created) != 0 {
		t.Errorf("expected no platform calls, listed=%v created=%v", provider.listed, provider.created)
	}
}

func TestRunWebhookSyncAllowsWritesToAllowlistedBaseURL(t *testing.T) {
	setBaseURL(t, "")
	provider := &recordingProvider{hooks: map[string][]gitProviderClients.Webhook{}}
	mgr := syncManager(t, provider, "renovate.example.com")

	job := syncJob("forgejo")
	job.Spec.Webhook.BaseURL = "https://renovate.example.com"

	if err := mgr.runWebhookSync(context.Background(), job, RenovateJobIdentifier{Name: "j", Namespace: "n"}, []string{"org/a"}, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(provider.created) != 1 {
		t.Errorf("expected the hook to be created, created=%v", provider.created)
	}
}

// Removal must not be gated on the delivery host. A hook written under a hostile
// baseUrl is exactly the one that needs deleting, and its URL is matching input
// here rather than somewhere the operator sends anything.
func TestRunWebhookSyncStillRemovesHooksUnderForeignBaseURL(t *testing.T) {
	setBaseURL(t, "")
	deliveryURL := "https://attacker.example.net/webhook/v1/forgejo?job=j&namespace=n"
	provider := &recordingProvider{hooks: map[string][]gitProviderClients.Webhook{
		"org/a": {{ID: "7", URL: deliveryURL, Active: true, EventsUpToDate: true}},
	}}
	mgr := syncManager(t, provider, "renovate.example.com")

	job := syncJob("forgejo")
	job.Spec.Webhook.BaseURL = "https://attacker.example.net"

	if err := mgr.runWebhookSync(context.Background(), job, RenovateJobIdentifier{Name: "j", Namespace: "n"}, nil, []string{"org/a"}); err != nil {
		t.Fatalf("removal must not be blocked by the destination policy, got %v", err)
	}
	if len(provider.deleted) != 1 {
		t.Errorf("expected the orphaned hook to be deleted, deleted=%v", provider.deleted)
	}
}

// authJob is a RenovateJob whose webhook authentication token is read from an
// arbitrary secret key: the reference that needs the opt-in label.
func authJob(secretName, key string) *api.RenovateJob {
	job := syncJob("forgejo")
	job.Namespace = "default"
	job.Spec.Webhook.Authentication = &api.RenovateWebhookAuth{
		Enabled:   true,
		SecretRef: &api.RenovateSecretKeyReference{Name: secretName, Key: key},
	}
	return job
}

func tokenManager(t *testing.T, secret *corev1.Secret, p policy.Policy) *renovateJobManager {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := api.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add api scheme: %v", err)
	}
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add core scheme: %v", err)
	}
	return &renovateJobManager{
		client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build(),
		logger: logr.Discard(),
		lock:   &sync.RWMutex{},
		policy: p,
	}
}

func TestGetRenovateJobTokensRefusesUnlabeledSecret(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "db-credentials", Namespace: "default"},
		Data:       map[string][]byte{"password": []byte("s3cret")},
	}
	mgr := tokenManager(t, secret, policy.Policy{})

	_, err := mgr.getRenovateJobTokens(context.Background(), authJob("db-credentials", "password"))
	if err == nil {
		t.Fatal("expected an arbitrary secret key to be refused without the opt-in label")
	}
	if !strings.Contains(err.Error(), api.LabelAllowRef) {
		t.Errorf("expected the error to name the label to add, got: %v", err)
	}
	if strings.Contains(err.Error(), "s3cret") {
		t.Error("the error must not contain the secret value")
	}
}

func TestGetRenovateJobTokensReadsLabeledSecret(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "webhook-auth",
			Namespace: "default",
			Labels:    map[string]string{api.LabelAllowRef: "true"},
		},
		Data: map[string][]byte{"token": []byte("first,second")},
	}
	mgr := tokenManager(t, secret, policy.Policy{})

	tokens, err := mgr.getRenovateJobTokens(context.Background(), authJob("webhook-auth", "token"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tokens) != 2 || tokens[0] != "first" || tokens[1] != "second" {
		t.Errorf("expected both comma-separated tokens, got %v", tokens)
	}
}

func TestRunWebhookSyncRefusesWritesToForeignEnvBaseURL(t *testing.T) {
	setBaseURL(t, "https://attacker.example.net")
	provider := &recordingProvider{hooks: map[string][]gitProviderClients.Webhook{}}
	mgr := syncManager(t, provider, "renovate.example.com")

	job := syncJob("forgejo")
	job.Spec.Webhook.BaseURL = ""

	if err := mgr.runWebhookSync(context.Background(), job, RenovateJobIdentifier{Name: "j", Namespace: "n"}, []string{"org/a"}, nil); err == nil {
		t.Fatal("expected a misconfigured WEBHOOK_BASE_URL to be refused too")
	}
	if len(provider.created) != 0 {
		t.Errorf("expected no hook creation, created=%v", provider.created)
	}
}

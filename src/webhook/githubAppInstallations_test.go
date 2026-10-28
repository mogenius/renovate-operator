package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	api "renovate-operator/api/v1alpha1"
	"renovate-operator/internal/policy"
	"renovate-operator/internal/renovate"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

const installationWebhookSecret = "app-webhook-secret"

func installationWebhookServer(t *testing.T, objs ...crclient.Object) (*Server, crclient.Client) {
	t.Helper()
	server, c, _ := countingInstallationWebhookServer(t, objs...)
	return server, c
}

// countingInstallationWebhookServer also returns the number of Secret reads,
// which go to the API server uncached in the operator.
func countingInstallationWebhookServer(t *testing.T, objs ...crclient.Object) (*Server, crclient.Client, *int) {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := api.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add scheme: %v", err)
	}
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add core scheme: %v", err)
	}
	secretReads := 0
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, c crclient.WithWatch, key crclient.ObjectKey, obj crclient.Object, opts ...crclient.GetOption) error {
				if _, ok := obj.(*corev1.Secret); ok {
					secretReads++
				}
				return c.Get(ctx, key, obj, opts...)
			},
		}).Build()
	server := (&Server{manager: &mockWebhookManager{}, logger: logr.Discard()}).WithInstallationSets(c, policy.Policy{Disabled: true})
	return server, c, &secretReads
}

func installationSetWithWebhook(name string) *api.GitHubAppInstallationSet {
	set := &api.GitHubAppInstallationSet{Spec: api.GitHubAppInstallationSetSpec{
		Webhook: &api.GitHubAppInstallationSetWebhook{
			Enabled:   true,
			SecretRef: &api.RenovateSecretKeyReference{Name: "app-webhook", Key: "secret"},
		},
	}}
	set.Name = name
	set.Namespace = "renovate"
	return set
}

func installationWebhookSecretObject() *corev1.Secret {
	secret := &corev1.Secret{Data: map[string][]byte{"secret": []byte(installationWebhookSecret)}}
	secret.Name = "app-webhook"
	secret.Namespace = "renovate"
	return secret
}

func signedInstallationRequest(event, body, secret string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/webhook/v1/github", bytes.NewBufferString(body))
	req.Header.Set("X-GitHub-Event", event)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	req.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	return req
}

func TestInstallationEventTriggersResync(t *testing.T) {
	server, c := installationWebhookServer(t, installationSetWithWebhook("club"), installationWebhookSecretObject())

	rec := httptest.NewRecorder()
	server.githubWebhook(rec, signedInstallationRequest("installation", `{"action":"created","installation":{"id":7}}`, installationWebhookSecret))

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}
	set := &api.GitHubAppInstallationSet{}
	if err := c.Get(context.Background(), k8stypes.NamespacedName{Name: "club", Namespace: "renovate"}, set); err != nil {
		t.Fatalf("failed to get set: %v", err)
	}
	if set.Annotations[api.TriggerResyncAnnotationKey] == "" {
		t.Error("a verified installation event must mark the set for resync")
	}
}

// The event may come from another App sharing the URL, so it is not rejected.
func TestInstallationEventWithWrongSignatureIsIgnored(t *testing.T) {
	server, c := installationWebhookServer(t, installationSetWithWebhook("club"), installationWebhookSecretObject())

	rec := httptest.NewRecorder()
	server.githubWebhook(rec, signedInstallationRequest("installation", `{"action":"created","installation":{"id":7}}`, "wrong"))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	set := &api.GitHubAppInstallationSet{}
	if err := c.Get(context.Background(), k8stypes.NamespacedName{Name: "club", Namespace: "renovate"}, set); err != nil {
		t.Fatalf("failed to get set: %v", err)
	}
	if _, ok := set.Annotations[api.TriggerResyncAnnotationKey]; ok {
		t.Error("an unverified event must not trigger a resync")
	}
}

func TestInstallationEventIgnoredForSetWithoutWebhook(t *testing.T) {
	set := installationSetWithWebhook("club")
	set.Spec.Webhook.Enabled = false
	server, _ := installationWebhookServer(t, set, installationWebhookSecretObject())

	rec := httptest.NewRecorder()
	server.githubWebhook(rec, signedInstallationRequest("installation", `{"action":"created","installation":{"id":7}}`, installationWebhookSecret))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 when no set accepts installation events, got %d", rec.Code)
	}
}

func TestInstallationRepositoriesEventIgnoredWithoutVerifyingSet(t *testing.T) {
	server, _ := installationWebhookServer(t)

	rec := httptest.NewRecorder()
	server.githubWebhook(rec, signedInstallationRequest("installation_repositories", `{"action":"added","installation":{"id":7}}`, installationWebhookSecret))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

// RenovateJobs outside any set may autodiscover the new installation's
// repositories, so a set verifying the event must not swallow their discovery.
func TestVerifiedInstallationEventAlsoStartsDiscovery(t *testing.T) {
	server, c := installationWebhookServer(t, installationSetWithWebhook("club"), installationWebhookSecretObject())
	var discovered []string
	server.manager = &mockWebhookManager{
		listRenovateJobsFullFunc: func(ctx context.Context) ([]api.RenovateJob, error) {
			return []api.RenovateJob{makeGitHubRenovateJob("renovate", "org-wide", "alice/*")}, nil
		},
	}
	server.discovery = &fakeDiscoveryAgent{
		createFunc: func(ctx context.Context, job api.RenovateJob, options renovate.DiscoveryJobOptions) (string, error) {
			discovered = append(discovered, job.Name)
			return "discovery", nil
		},
	}

	rec := httptest.NewRecorder()
	server.githubWebhook(rec, signedInstallationRequest("installation", `{"action":"created","installation":{"id":7,"account":{"login":"alice"}}}`, installationWebhookSecret))

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(discovered) != 1 || discovered[0] != "org-wide" {
		t.Errorf("expected discovery for org-wide, got %v", discovered)
	}
	set := &api.GitHubAppInstallationSet{}
	if err := c.Get(context.Background(), k8stypes.NamespacedName{Name: "club", Namespace: "renovate"}, set); err != nil {
		t.Fatalf("failed to get set: %v", err)
	}
	if set.Annotations[api.TriggerResyncAnnotationKey] == "" {
		t.Error("the verifying set must still be marked for resync")
	}
}

func TestInstallationEventReadsSharedWebhookSecretOnce(t *testing.T) {
	server, _, secretReads := countingInstallationWebhookServer(t,
		installationSetWithWebhook("club"), installationSetWithWebhook("guild"), installationWebhookSecretObject())

	rec := httptest.NewRecorder()
	server.githubWebhook(rec, signedInstallationRequest("installation", `{"action":"created","installation":{"id":7}}`, installationWebhookSecret))

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}
	if *secretReads != 1 {
		t.Errorf("expected one read of the shared webhook secret, got %d", *secretReads)
	}
}

func TestInstallationEventWithMalformedSignatureReadsNoSecret(t *testing.T) {
	server, _, secretReads := countingInstallationWebhookServer(t, installationSetWithWebhook("club"), installationWebhookSecretObject())

	req := signedInstallationRequest("installation", `{"action":"created","installation":{"id":7}}`, installationWebhookSecret)
	req.Header.Set("X-Hub-Signature-256", "sha256=not-hex")
	rec := httptest.NewRecorder()
	server.githubWebhook(rec, req)

	if *secretReads != 0 {
		t.Errorf("a malformed signature must not cost a secret read, got %d", *secretReads)
	}
}

func TestInstallationRepositoriesEventTriggersDiscovery(t *testing.T) {
	job := &api.RenovateJob{}
	job.Name = "club-alice"
	job.Namespace = "renovate"
	job.Labels = map[string]string{api.LabelInstallationSet: "club", api.LabelInstallationID: "7"}
	other := &api.RenovateJob{}
	other.Name = "club-bob"
	other.Namespace = "renovate"
	other.Labels = map[string]string{api.LabelInstallationSet: "club", api.LabelInstallationID: "8"}
	server, c := installationWebhookServer(t, installationSetWithWebhook("club"), installationWebhookSecretObject(), job, other)

	rec := httptest.NewRecorder()
	server.githubWebhook(rec, signedInstallationRequest("installation_repositories", `{"action":"added","installation":{"id":7}}`, installationWebhookSecret))

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}
	got := &api.RenovateJob{}
	if err := c.Get(context.Background(), k8stypes.NamespacedName{Name: "club-alice", Namespace: "renovate"}, got); err != nil {
		t.Fatalf("failed to get job: %v", err)
	}
	if got.Annotations[api.TriggerDiscoveryAnnotationKey] != "true" {
		t.Error("the installation's RenovateJob must be marked for discovery")
	}
	if err := c.Get(context.Background(), k8stypes.NamespacedName{Name: "club-bob", Namespace: "renovate"}, got); err != nil {
		t.Fatalf("failed to get job: %v", err)
	}
	if _, ok := got.Annotations[api.TriggerDiscoveryAnnotationKey]; ok {
		t.Error("other installations' RenovateJobs must not be marked for discovery")
	}
}

func TestInstallationEventIgnoredWithoutInstallationSets(t *testing.T) {
	server := &Server{manager: &mockWebhookManager{}, logger: logr.Discard()}

	rec := httptest.NewRecorder()
	server.githubWebhook(rec, signedInstallationRequest("installation", `{"action":"created","installation":{"id":7}}`, installationWebhookSecret))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

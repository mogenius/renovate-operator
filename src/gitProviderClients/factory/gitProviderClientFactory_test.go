package gitProviderClientFactory

import (
	"context"
	"strings"
	"testing"

	api "renovate-operator/api/v1alpha1"
	"renovate-operator/gitProviderClients/githubProvider"
	"renovate-operator/internal/policy"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// testPolicy allows the default github endpoint the fixtures resolve to, so the
// cases below stay about token resolution rather than the destination policy.
func testPolicy() policy.Policy {
	return policy.Policy{AllowedHosts: []string{"api.github.com"}}
}

func newTestJob() *api.RenovateJob {
	job := &api.RenovateJob{}
	job.ObjectMeta = metav1.ObjectMeta{Name: "my-job", Namespace: "default"}
	job.Spec.Provider = &api.RenovateProvider{Name: "github"}
	job.Spec.SecretRef = "renovate-secret"
	return job
}

func newTestScheme(t *testing.T) *runtime.Scheme {
	scheme := runtime.NewScheme()
	if err := api.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add api scheme: %v", err)
	}
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add core scheme: %v", err)
	}
	return scheme
}

// newSecret builds a secret that has opted in to being referenced. Use
// newUnlabeledSecret to exercise a secret that has not.
func newSecret(name string, data map[string][]byte) *corev1.Secret {
	secret := newUnlabeledSecret(name, data)
	secret.Labels = map[string]string{policy.AllowRefLabel: "true"}
	return secret
}

func newUnlabeledSecret(name string, data map[string][]byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Data:       data,
	}
}

func TestNewClientWithTokenRef_ExplicitKey(t *testing.T) {
	secret := newSecret("webhook-token", map[string][]byte{"token": []byte("dedicated-token")})
	cl := fake.NewClientBuilder().WithScheme(newTestScheme(t)).WithObjects(secret).Build()
	factory := NewGitProviderClientFactory(cl, testPolicy())

	client, err := factory.NewClientWithTokenRef(context.Background(), newTestJob(), &api.RenovateSecretKeyReference{Name: "webhook-token", Key: "token"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	gh, ok := client.(*githubProvider.GitHubClient)
	if !ok {
		t.Fatalf("expected a GitHub client, got %T", client)
	}
	if gh.Token != "dedicated-token" {
		t.Fatalf("expected token from referenced secret key, got %q", gh.Token)
	}
}

func TestNewClientWithTokenRef_FallsBackToCommonKeys(t *testing.T) {
	secret := newSecret("webhook-token", map[string][]byte{"RENOVATE_TOKEN": []byte("common-key-token")})
	cl := fake.NewClientBuilder().WithScheme(newTestScheme(t)).WithObjects(secret).Build()
	factory := NewGitProviderClientFactory(cl, testPolicy())

	client, err := factory.NewClientWithTokenRef(context.Background(), newTestJob(), &api.RenovateSecretKeyReference{Name: "webhook-token"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gh := client.(*githubProvider.GitHubClient); gh.Token != "common-key-token" {
		t.Fatalf("expected token from common key names, got %q", gh.Token)
	}
}

func TestNewClientWithTokenRef_MissingKey(t *testing.T) {
	secret := newSecret("webhook-token", map[string][]byte{"other": []byte("x")})
	cl := fake.NewClientBuilder().WithScheme(newTestScheme(t)).WithObjects(secret).Build()
	factory := NewGitProviderClientFactory(cl, testPolicy())

	if _, err := factory.NewClientWithTokenRef(context.Background(), newTestJob(), &api.RenovateSecretKeyReference{Name: "webhook-token", Key: "token"}); err == nil {
		t.Fatal("expected error for missing secret key")
	}
}

func TestNewClientWithTokenRef_NilRef(t *testing.T) {
	cl := fake.NewClientBuilder().WithScheme(newTestScheme(t)).Build()
	factory := NewGitProviderClientFactory(cl, testPolicy())

	if _, err := factory.NewClientWithTokenRef(context.Background(), newTestJob(), nil); err == nil {
		t.Fatal("expected error for nil secret reference")
	}
}

func TestNewClientWithTokenRef_DeniesUnlabeledSecret(t *testing.T) {
	secret := newUnlabeledSecret("db-credentials", map[string][]byte{"password": []byte("s3cret")})
	cl := fake.NewClientBuilder().WithScheme(newTestScheme(t)).WithObjects(secret).Build()
	factory := NewGitProviderClientFactory(cl, testPolicy())

	_, err := factory.NewClientWithTokenRef(context.Background(), newTestJob(), &api.RenovateSecretKeyReference{Name: "db-credentials", Key: "password"})
	if err == nil {
		t.Fatal("expected an arbitrary secret key to be refused without the opt-in label")
	}
	if !strings.Contains(err.Error(), policy.AllowRefLabel) {
		t.Errorf("expected the error to name the label to add, got: %v", err)
	}
	// The refusal must not echo what it protected.
	if strings.Contains(err.Error(), "s3cret") {
		t.Error("the error must not contain the secret value")
	}
}

func TestNewClientWithTokenRef_AllowsUnlabeledSecretWhenOptInDisabled(t *testing.T) {
	secret := newUnlabeledSecret("webhook-token", map[string][]byte{"token": []byte("dedicated-token")})
	cl := fake.NewClientBuilder().WithScheme(newTestScheme(t)).WithObjects(secret).Build()
	p := testPolicy()
	p.AllowUnlabeledSecretRefs = true
	factory := NewGitProviderClientFactory(cl, p)

	client, err := factory.NewClientWithTokenRef(context.Background(), newTestJob(), &api.RenovateSecretKeyReference{Name: "webhook-token", Key: "token"})
	if err != nil {
		t.Fatalf("unexpected error with the opt-in requirement disabled: %v", err)
	}
	if gh := client.(*githubProvider.GitHubClient); gh.Token != "dedicated-token" {
		t.Fatalf("expected the referenced token, got %q", gh.Token)
	}
}

func TestNewClient_JobSecretRefNeedsNoLabel(t *testing.T) {
	// spec.secretRef is read only at Renovate's own well-known token key names, so
	// it is not an arbitrary-key read and deliberately does not require the label.
	secret := newUnlabeledSecret("renovate-secret", map[string][]byte{"RENOVATE_TOKEN": []byte("platform-token")})
	cl := fake.NewClientBuilder().WithScheme(newTestScheme(t)).WithObjects(secret).Build()
	factory := NewGitProviderClientFactory(cl, testPolicy())

	client, err := factory.NewClient(context.Background(), newTestJob())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gh := client.(*githubProvider.GitHubClient); gh.Token != "platform-token" {
		t.Fatalf("expected the platform token, got %q", gh.Token)
	}
}

func TestNewClientWithTokenRef_DeniesForeignEndpoint(t *testing.T) {
	secret := newSecret("webhook-token", map[string][]byte{"token": []byte("dedicated-token")})
	cl := fake.NewClientBuilder().WithScheme(newTestScheme(t)).WithObjects(secret).Build()
	factory := NewGitProviderClientFactory(cl, testPolicy())

	job := newTestJob()
	job.Spec.Provider.Endpoint = "https://attacker.example.net"

	_, err := factory.NewClientWithTokenRef(context.Background(), job, &api.RenovateSecretKeyReference{Name: "webhook-token", Key: "token"})
	if err == nil {
		t.Fatal("expected a client for a non-allowlisted endpoint to be refused")
	}
	if !strings.Contains(err.Error(), "attacker.example.net") {
		t.Errorf("expected the denied host in the error, got: %v", err)
	}
}

func TestNewClient_DeniesForeignEndpoint(t *testing.T) {
	secret := newSecret("renovate-secret", map[string][]byte{"RENOVATE_TOKEN": []byte("platform-token")})
	cl := fake.NewClientBuilder().WithScheme(newTestScheme(t)).WithObjects(secret).Build()
	factory := NewGitProviderClientFactory(cl, testPolicy())

	job := newTestJob()
	job.Spec.Provider.Endpoint = "https://attacker.example.net"

	if _, err := factory.NewClient(context.Background(), job); err == nil {
		t.Fatal("expected the job's platform token not to be sent to a non-allowlisted endpoint")
	}
}

func TestNewClient_AllowsAllowlistedEndpoint(t *testing.T) {
	secret := newSecret("renovate-secret", map[string][]byte{"RENOVATE_TOKEN": []byte("platform-token")})
	cl := fake.NewClientBuilder().WithScheme(newTestScheme(t)).WithObjects(secret).Build()
	factory := NewGitProviderClientFactory(cl, testPolicy())

	// newTestJob leaves the endpoint empty, which resolves to api.github.com.
	client, err := factory.NewClient(context.Background(), newTestJob())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gh := client.(*githubProvider.GitHubClient); gh.Token != "platform-token" {
		t.Fatalf("expected the platform token, got %q", gh.Token)
	}
}

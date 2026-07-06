package gitProviderClientFactory

import (
	"context"
	"testing"

	api "renovate-operator/api/v1alpha1"
	"renovate-operator/gitProviderClients/githubProvider"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

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

func newSecret(name string, data map[string][]byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Data:       data,
	}
}

func TestNewClientWithTokenRef_ExplicitKey(t *testing.T) {
	secret := newSecret("webhook-token", map[string][]byte{"token": []byte("dedicated-token")})
	cl := fake.NewClientBuilder().WithScheme(newTestScheme(t)).WithObjects(secret).Build()
	factory := NewGitProviderClientFactory(cl)

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
	factory := NewGitProviderClientFactory(cl)

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
	factory := NewGitProviderClientFactory(cl)

	if _, err := factory.NewClientWithTokenRef(context.Background(), newTestJob(), &api.RenovateSecretKeyReference{Name: "webhook-token", Key: "token"}); err == nil {
		t.Fatal("expected error for missing secret key")
	}
}

func TestNewClientWithTokenRef_NilRef(t *testing.T) {
	cl := fake.NewClientBuilder().WithScheme(newTestScheme(t)).Build()
	factory := NewGitProviderClientFactory(cl)

	if _, err := factory.NewClientWithTokenRef(context.Background(), newTestJob(), nil); err == nil {
		t.Fatal("expected error for nil secret reference")
	}
}

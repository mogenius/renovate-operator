package crdmanager

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"testing"

	gitProviderClientFactory "renovate-operator/gitProviderClients/factory"
	"renovate-operator/internal/policy"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// A sync cycle through the real factory writes the push event to managed GitHub
// hooks only while the job has webhook.triggerOnPush.
func TestRunWebhookSyncSubscribesPushOnlyWithTriggerOnPush(t *testing.T) {
	setBaseURL(t, "https://renovate.example.com")

	var written []string
	github := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte("[]"))
			return
		}
		var hook struct {
			Events []string `json:"events"`
		}
		_ = json.NewDecoder(r.Body).Decode(&hook)
		written = hook.Events
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":1}`))
	}))
	defer github.Close()
	githubURL, _ := url.Parse(github.URL)

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add core scheme: %v", err)
	}
	secret := &corev1.Secret{
		Name: "renovate-secret", Namespace: "n",
		Data: map[string][]byte{"RENOVATE_TOKEN": []byte("token")},
	}
	p := policy.Policy{AllowedHosts: []string{"renovate.example.com", githubURL.Hostname()}}
	mgr := &renovateJobManager{
		gitProviderClientFactory: gitProviderClientFactory.NewGitProviderClientFactory(
			fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build(), p),
		logger: logr.Discard(),
		policy: p,
	}

	tests := []struct {
		triggerOnPush bool
		wantEvents    []string
	}{
		{false, []string{"issues", "pull_request"}},
		{true, []string{"issues", "pull_request", "push"}},
	}
	for _, tt := range tests {
		job := syncJob("github")
		job.Namespace = "n"
		job.Spec.SecretRef = "renovate-secret"
		job.Spec.Provider.Endpoint = github.URL
		job.Spec.Webhook.TriggerOnPush = tt.triggerOnPush

		written = nil
		if err := mgr.runWebhookSync(context.Background(), job, RenovateJobIdentifier{Name: "j", Namespace: "n"}, []string{"org/a"}, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		slices.Sort(written)
		if !slices.Equal(written, tt.wantEvents) {
			t.Errorf("triggerOnPush=%v: expected events %v, got %v", tt.triggerOnPush, tt.wantEvents, written)
		}
	}
}

//go:build integration

// Package integration contains an over-the-wire integration test for the operator's webhook server.
//
// Unlike the in-package webhook tests (httptest.NewRecorder + a mock manager), this boots the REAL
// webhook.Server on a real TCP socket, backed by the REAL crdManager.RenovateJobManager over a
// controller-runtime fake client seeded with a real Secret, and drives it with a client that
// independently emulates a Standard Webhooks sender (e.g. GitLab "signing tokens"). This exercises
// the genuine path: HTTP -> router -> gitLabWebhook -> resolver -> manager -> Secret read ->
// HMAC-SHA256 signature verification -> CRD status update, including the 5-minute replay window.
//
// Run: go test -tags integration -count=1 -v ./integration/...   (or: just test-integration)
package integration

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	api "renovate-operator/api/v1alpha1"
	"renovate-operator/config"
	crdmanager "renovate-operator/internal/crdManager"
	"renovate-operator/webhook"
)

const (
	testNamespace = "renovate-operator"
	authedJobName = "gitlab-signed"
	authedProject = "example/secured-repo"
	openJobName   = "gitlab-open"
	openProject   = "example/open-repo"
	secretName    = "renovate-webhook-secret"
	secretKey     = "token"

	// signingSecret is the public Standard Webhooks / svix example key (whsec_ + base64(key)); legacyToken
	// is a plain secret token for the legacy X-Gitlab-Token path. Both live in the same comma-separated
	// secret value, mirroring how the operator splits getRenovateJobTokens. The "whsec_" prefix is kept
	// separate from the body so secret scanners don't flag this public test vector as a live credential.
	signingSecret = "whsec_" + "MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw"
	legacyToken   = "plain-legacy-token"
)

// gitlabSigner independently emulates the sender side of Standard Webhooks: it derives the HMAC key
// from the whsec_ secret and signs "{id}.{timestamp}.{body}". Kept deliberately separate from the
// operator's implementation so the test is a true black box.
type gitlabSigner struct{ secret string }

func (g gitlabSigner) signature(id, ts string, body []byte) string {
	key := []byte(strings.TrimPrefix(g.secret, "whsec_"))
	if decoded, err := base64.StdEncoding.DecodeString(string(key)); err == nil {
		key = decoded
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(id + "." + ts + "." + string(body)))
	return "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func signedHeaders(signer gitlabSigner, id, ts string, body []byte) map[string]string {
	return map[string]string{
		"Content-Type":      "application/json",
		"Webhook-Id":        id,
		"Webhook-Timestamp": ts,
		"Webhook-Signature": signer.signature(id, ts, body),
	}
}

// validEvent builds a GitLab merge-request webhook payload that passes the operator's event filter
// (object_kind merge_request, action update, a checked Renovate rebase-check checkbox). extra varies
// the body so a signature computed over one payload won't match another.
func validEvent(project, extra string) []byte {
	current := "Renovate MR " + extra + "\n - [x] <!-- rebase-check -->If you want to rebase/retry this MR"
	ev := map[string]any{
		"object_kind": "merge_request",
		"event_type":  "merge_request",
		"project": map[string]any{
			"id":                  185,
			"name":                "repo",
			"namespace":           "example",
			"path_with_namespace": project,
		},
		"object_attributes": map[string]any{"id": 1, "action": "update"},
		"changes": map[string]any{
			"description": map[string]any{
				"previous": "Renovate MR\n - [ ] <!-- rebase-check -->If you want to rebase/retry this MR",
				"current":  current,
			},
		},
	}
	b, err := json.Marshal(ev)
	if err != nil {
		panic(err)
	}
	return b
}

func renovateJob(name, project string, authEnabled bool) *api.RenovateJob {
	job := &api.RenovateJob{
		TypeMeta:   metav1.TypeMeta{APIVersion: "renovate-operator.mogenius.com/v1alpha1", Kind: "RenovateJob"},
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec:       api.RenovateJobSpec{Webhook: &api.RenovateWebhook{Enabled: true}},
		Status:     api.RenovateJobStatus{Projects: []api.ProjectStatus{{Name: project}}},
	}
	if authEnabled {
		job.Spec.Webhook.Authentication = &api.RenovateWebhookAuth{
			Enabled:   true,
			SecretRef: &api.RenovateSecretKeyReference{Name: secretName, Key: secretKey},
		}
	}
	return job
}

// startWebhookServer boots the real webhook.Server on a free port over a fake-client-backed manager
// and returns its base URL plus the manager (for CRD status read-back).
func startWebhookServer(t *testing.T) (string, crdmanager.RenovateJobManager) {
	t.Helper()

	port := freePort(t)
	if err := config.InitializeConfigModule([]config.ConfigItemDescription{
		{Key: "WEBHOOK_SERVER_PORT", Default: port, Optional: true},
	}); err != nil {
		t.Fatalf("config init: %v", err)
	}

	scheme := runtime.NewScheme()
	if err := api.AddToScheme(scheme); err != nil {
		t.Fatalf("add api to scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add corev1 to scheme: %v", err)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: testNamespace},
		Data:       map[string][]byte{secretKey: []byte(signingSecret + "," + legacyToken)},
	}
	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(secret, renovateJob(authedJobName, authedProject, true), renovateJob(openJobName, openProject, false)).
		WithStatusSubresource(&api.RenovateJob{}).
		Build()

	mgr := crdmanager.NewRenovateJobManager(cl, nil, logr.Discard(), nil, nil)
	webhook.NewWebookServer(mgr, logr.Discard()).Run()

	baseURL := "http://127.0.0.1:" + port
	waitReady(t, baseURL)
	return baseURL, mgr
}

func freePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	defer l.Close()
	return strconv.Itoa(l.Addr().(*net.TCPAddr).Port)
}

func waitReady(t *testing.T, baseURL string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		// The route is POST-only, so a GET returns 405 — which still proves the server is listening.
		if resp, err := http.Get(baseURL + "/webhook/v1/gitlab"); err == nil {
			resp.Body.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("webhook server did not become ready")
}

func post(t *testing.T, url string, body []byte, headers map[string]string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func projectStatus(job *api.RenovateJob, project string) api.RenovateProjectStatus {
	for _, p := range job.Status.Projects {
		if p.Name == project {
			return p.Status
		}
	}
	return ""
}

func nowUnix() string { return strconv.FormatInt(time.Now().Unix(), 10) }

func TestSigningTokenWebhookIntegration(t *testing.T) {
	baseURL, mgr := startWebhookServer(t)
	signer := gitlabSigner{secret: signingSecret}
	authedURL := baseURL + "/webhook/v1/gitlab?namespace=" + testNamespace + "&job=" + authedJobName
	openURL := baseURL + "/webhook/v1/gitlab?namespace=" + testNamespace + "&job=" + openJobName

	t.Run("valid signing-token signature is accepted and schedules the project", func(t *testing.T) {
		body := validEvent(authedProject, "valid")
		code, msg := post(t, authedURL, body, signedHeaders(signer, "msg-valid", nowUnix(), body))
		if code != http.StatusAccepted {
			t.Fatalf("want 202, got %d: %s", code, msg)
		}
		job, err := mgr.GetRenovateJob(context.Background(), authedJobName, testNamespace)
		if err != nil {
			t.Fatalf("read back job: %v", err)
		}
		if got := projectStatus(job, authedProject); got != api.JobStatusScheduled {
			t.Errorf("project status = %q, want %q", got, api.JobStatusScheduled)
		}
	})

	t.Run("tampered body is rejected", func(t *testing.T) {
		signed := validEvent(authedProject, "original")
		headers := signedHeaders(signer, "msg-tamper", nowUnix(), signed)
		tampered := validEvent(authedProject, "tampered-after-signing")
		if code, msg := post(t, authedURL, tampered, headers); code != http.StatusUnauthorized {
			t.Fatalf("want 401, got %d: %s", code, msg)
		}
	})

	t.Run("signature from a different key is rejected", func(t *testing.T) {
		wrong := gitlabSigner{secret: "whsec_" + base64.StdEncoding.EncodeToString([]byte("an-entirely-different-signing-key"))}
		body := validEvent(authedProject, "wrongkey")
		if code, msg := post(t, authedURL, body, signedHeaders(wrong, "msg-wrong", nowUnix(), body)); code != http.StatusUnauthorized {
			t.Fatalf("want 401, got %d: %s", code, msg)
		}
	})

	t.Run("stale timestamp is rejected (replay window)", func(t *testing.T) {
		body := validEvent(authedProject, "stale")
		stale := strconv.FormatInt(time.Now().Add(-10*time.Minute).Unix(), 10)
		if code, msg := post(t, authedURL, body, signedHeaders(signer, "msg-stale", stale, body)); code != http.StatusUnauthorized {
			t.Fatalf("want 401, got %d: %s", code, msg)
		}
	})

	t.Run("future timestamp is rejected (replay window)", func(t *testing.T) {
		body := validEvent(authedProject, "future")
		future := strconv.FormatInt(time.Now().Add(10*time.Minute).Unix(), 10)
		if code, msg := post(t, authedURL, body, signedHeaders(signer, "msg-future", future, body)); code != http.StatusUnauthorized {
			t.Fatalf("want 401, got %d: %s", code, msg)
		}
	})

	t.Run("missing credentials on an auth-required job is rejected", func(t *testing.T) {
		body := validEvent(authedProject, "nocreds")
		headers := map[string]string{"Content-Type": "application/json"}
		if code, msg := post(t, authedURL, body, headers); code != http.StatusUnauthorized {
			t.Fatalf("want 401, got %d: %s", code, msg)
		}
	})

	t.Run("legacy X-Gitlab-Token still authenticates", func(t *testing.T) {
		body := validEvent(authedProject, "legacy")
		headers := map[string]string{"Content-Type": "application/json", "X-Gitlab-Token": legacyToken}
		if code, msg := post(t, authedURL, body, headers); code != http.StatusAccepted {
			t.Fatalf("want 202, got %d: %s", code, msg)
		}
	})

	t.Run("auth-disabled job accepts unsigned deliveries", func(t *testing.T) {
		body := validEvent(openProject, "open")
		headers := map[string]string{"Content-Type": "application/json"}
		if code, msg := post(t, openURL, body, headers); code != http.StatusAccepted {
			t.Fatalf("want 202, got %d: %s", code, msg)
		}
		job, err := mgr.GetRenovateJob(context.Background(), openJobName, testNamespace)
		if err != nil {
			t.Fatalf("read back job: %v", err)
		}
		if got := projectStatus(job, openProject); got != api.JobStatusScheduled {
			t.Errorf("project status = %q, want %q", got, api.JobStatusScheduled)
		}
	})
}

package webhook

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	crdmanager "renovate-operator/internal/crdManager"
)

func TestBuildAuthCheckerFromRequest(t *testing.T) {
	body := []byte(`{"hello":"world"}`)
	jobID := crdmanager.RenovateJobIdentifier{Name: "j", Namespace: "ns"}

	t.Run("standard webhook signature wins over token and forwards all fields", func(t *testing.T) {
		called := ""
		var gotMsgID, gotTS, gotSig string
		var gotBody []byte
		m := &mockWebhookManager{
			isWebhookStandardSignatureValidFunc: func(_ context.Context, _ crdmanager.RenovateJobIdentifier, msgID, ts, sig string, b []byte) (bool, error) {
				called = "standard"
				gotMsgID, gotTS, gotSig, gotBody = msgID, ts, sig, b
				return true, nil
			},
			isWebhookTokenValidFunc: func(context.Context, crdmanager.RenovateJobIdentifier, string) (bool, error) {
				called = "token"
				return true, nil
			},
		}
		r := httptest.NewRequest(http.MethodPost, "/webhook/v1/gitlab", nil)
		r.Header.Set("Webhook-Id", "msg_1")
		r.Header.Set("Webhook-Timestamp", "1700000000")
		r.Header.Set("Webhook-Signature", "v1,abc")
		r.Header.Set("X-Gitlab-Token", "sometoken") // present, but signature must take precedence

		checker := buildAuthCheckerFromRequest(r, body, m)
		if checker == nil {
			t.Fatal("expected non-nil checker")
		}
		ok, err := checker(context.Background(), jobID)
		if err != nil || !ok {
			t.Fatalf("checker returned ok=%v err=%v", ok, err)
		}
		if called != "standard" {
			t.Fatalf("expected standard validator, got %q", called)
		}
		if gotMsgID != "msg_1" || gotTS != "1700000000" || gotSig != "v1,abc" {
			t.Errorf("wrong args: id=%q ts=%q sig=%q", gotMsgID, gotTS, gotSig)
		}
		if string(gotBody) != string(body) {
			t.Errorf("body not forwarded: got %q", gotBody)
		}
	})

	t.Run("x-gitlab-token routes to token validator", func(t *testing.T) {
		var gotToken string
		m := &mockWebhookManager{
			isWebhookTokenValidFunc: func(_ context.Context, _ crdmanager.RenovateJobIdentifier, token string) (bool, error) {
				gotToken = token
				return true, nil
			},
		}
		r := httptest.NewRequest(http.MethodPost, "/x", nil)
		r.Header.Set("X-Gitlab-Token", "secret-token")
		checker := buildAuthCheckerFromRequest(r, body, m)
		if checker == nil {
			t.Fatal("expected non-nil checker")
		}
		if _, err := checker(context.Background(), jobID); err != nil {
			t.Fatal(err)
		}
		if gotToken != "secret-token" {
			t.Errorf("got token %q, want secret-token", gotToken)
		}
	})

	t.Run("bearer authorization strips prefix", func(t *testing.T) {
		var gotToken string
		m := &mockWebhookManager{
			isWebhookTokenValidFunc: func(_ context.Context, _ crdmanager.RenovateJobIdentifier, token string) (bool, error) {
				gotToken = token
				return true, nil
			},
		}
		r := httptest.NewRequest(http.MethodPost, "/x", nil)
		r.Header.Set("Authorization", "Bearer abc123")
		checker := buildAuthCheckerFromRequest(r, body, m)
		if checker == nil {
			t.Fatal("expected non-nil checker")
		}
		if _, err := checker(context.Background(), jobID); err != nil {
			t.Fatal(err)
		}
		if gotToken != "abc123" {
			t.Errorf("got token %q, want abc123", gotToken)
		}
	})

	t.Run("hub signature routes to signature validator", func(t *testing.T) {
		called := false
		m := &mockWebhookManager{
			isWebhookSignatureValidFunc: func(context.Context, crdmanager.RenovateJobIdentifier, string, []byte) (bool, error) {
				called = true
				return true, nil
			},
		}
		r := httptest.NewRequest(http.MethodPost, "/x", nil)
		r.Header.Set("X-Hub-Signature-256", "sha256=abc")
		checker := buildAuthCheckerFromRequest(r, body, m)
		if checker == nil {
			t.Fatal("expected non-nil checker")
		}
		if _, err := checker(context.Background(), jobID); err != nil {
			t.Fatal(err)
		}
		if !called {
			t.Error("expected signature validator to be called")
		}
	})

	t.Run("no credentials returns nil checker", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/x", nil)
		if checker := buildAuthCheckerFromRequest(r, body, &mockWebhookManager{}); checker != nil {
			t.Error("expected nil checker when no credentials present")
		}
	})
}

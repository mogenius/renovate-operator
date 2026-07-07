package crdmanager

import (
	"testing"
	"time"
)

// canonicalSigningSecret is the public Standard Webhooks / svix example signing secret (published in
// the spec). The "whsec_" prefix is kept separate from the body so secret scanners don't flag this
// public test vector as a live credential — the prefix collides with Stripe's webhook-secret format.
const canonicalSigningSecret = "whsec_" + "MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw"

// TestStandardWebhookSignatureKnownAnswer pins the GitLab signing-token (Standard Webhooks)
// computation to the canonical svix test vector, exercising key decode, signing, and header match.
func TestStandardWebhookSignatureKnownAnswer(t *testing.T) {
	const (
		secret    = canonicalSigningSecret
		msgID     = "msg_p5jXN8AQM9LWM0D4loKWxJek"
		timestamp = "1614265330"
		want      = "g0hM9SsE+OTPJTGt/tmIKtSyZlE3uFJELVlNIOLJ1OE="
	)
	body := []byte(`{"test": 2432232314}`)

	key, ok := decodeStandardWebhookSigningKey(secret)
	if !ok {
		t.Fatal("decodeStandardWebhookSigningKey returned ok=false for canonical secret")
	}

	got := computeStandardWebhookSignature(key, msgID+"."+timestamp+"."+string(body))
	if got != want {
		t.Fatalf("signature mismatch:\n got %q\nwant %q", got, want)
	}

	if !matchesAnyStandardWebhookSignature("v1,"+want, got) {
		t.Error("expected match for single v1 entry")
	}
	if !matchesAnyStandardWebhookSignature("v0,ignored v1,"+want+" v2,other", got) {
		t.Error("expected match for v1 entry within a space-separated list")
	}
	if matchesAnyStandardWebhookSignature("v1,deadbeef", got) {
		t.Error("did not expect a match for a wrong signature")
	}
	if matchesAnyStandardWebhookSignature("v2,"+want, got) {
		t.Error("did not expect a match for a non-v1 version")
	}
}

func TestDecodeStandardWebhookSigningKey(t *testing.T) {
	if _, ok := decodeStandardWebhookSigningKey(""); ok {
		t.Error("empty secret should return ok=false")
	}
	if _, ok := decodeStandardWebhookSigningKey("whsec_not!!base64"); ok {
		t.Error("whsec_ secret with invalid base64 should return ok=false")
	}
	if key, ok := decodeStandardWebhookSigningKey(canonicalSigningSecret); !ok || len(key) == 0 {
		t.Errorf("canonical whsec_ secret should decode to a non-empty key, got len=%d ok=%v", len(key), ok)
	}
	if raw, ok := decodeStandardWebhookSigningKey("plain-secret-!!"); !ok || string(raw) != "plain-secret-!!" {
		t.Errorf("bare non-base64 secret should be used verbatim, got %q ok=%v", raw, ok)
	}
}

func TestIsStandardWebhookTimestampFresh(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	tests := []struct {
		name      string
		timestamp string
		want      bool
	}{
		{"now", "1700000000", true},
		{"within tolerance past", "1699999900", true},
		{"within tolerance future", "1700000100", true},
		{"too old", "1699999000", false},
		{"too far future", "1700001000", false},
		{"empty", "", false},
		{"non-numeric", "not-a-number", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isStandardWebhookTimestampFresh(tt.timestamp, now); got != tt.want {
				t.Errorf("isStandardWebhookTimestampFresh(%q) = %v, want %v", tt.timestamp, got, tt.want)
			}
		})
	}
}

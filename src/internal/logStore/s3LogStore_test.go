package logStore

import "testing"

func TestEncodeTagging(t *testing.T) {
	if got := encodeTagging(nil); got != nil {
		t.Fatalf("expected nil tagging for no tags, got %q", *got)
	}
	if got := encodeTagging(map[string]string{}); got != nil {
		t.Fatalf("expected nil tagging for empty tags, got %q", *got)
	}

	got := encodeTagging(map[string]string{"retention": "renovate-logs", "team": "a&b"})
	if got == nil {
		t.Fatal("expected tagging, got nil")
	}
	if want := "retention=renovate-logs&team=a%26b"; *got != want {
		t.Fatalf("expected %q, got %q", want, *got)
	}
}

package ui

import (
	"testing"

	"renovate-operator/config"
)

func setBasePath(t *testing.T, value string) {
	t.Helper()
	t.Setenv("BASE_PATH", value)
	t.Setenv("WEBHOOK_SERVER_UNIFIED_HOST", "false")
	if err := config.InitializeConfigModule([]config.ConfigItemDescription{
		{Key: "BASE_PATH", Optional: true, Default: ""},
		{Key: "WEBHOOK_SERVER_UNIFIED_HOST", Optional: true, Default: "false"},
	}); err != nil {
		t.Fatalf("failed to initialize config module: %v", err)
	}
}

func TestNormalizeBasePath(t *testing.T) {
	cases := map[string]string{
		"":               "",
		"/":              "",
		"renovate":       "/renovate",
		"/renovate":      "/renovate",
		"/renovate/":     "/renovate",
		"renovate/":      "/renovate",
		"  /renovate/  ": "/renovate",
		"/foo/bar":       "/foo/bar",
		"/foo/bar/":      "/foo/bar",
	}
	for in, want := range cases {
		if got := normalizeBasePath(in); got != want {
			t.Errorf("normalizeBasePath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWithBaseAndCookiePath(t *testing.T) {
	setBasePath(t, "/renovate")
	if got := withBase("/auth/login"); got != "/renovate/auth/login" {
		t.Errorf("withBase(/auth/login) = %q", got)
	}
	if got := withBase("/"); got != "/renovate/" {
		t.Errorf("withBase(/) = %q", got)
	}
	if got := cookiePath(); got != "/renovate" {
		t.Errorf("cookiePath() = %q, want /renovate", got)
	}

	setBasePath(t, "")
	if got := withBase("/auth/login"); got != "/auth/login" {
		t.Errorf("withBase(/auth/login) with empty base = %q", got)
	}
	if got := cookiePath(); got != "/" {
		t.Errorf("cookiePath() with empty base = %q, want /", got)
	}
}

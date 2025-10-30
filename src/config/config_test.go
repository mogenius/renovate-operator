package config

import (
	"os"
	"testing"
)

func TestInitializeAndGetValue(t *testing.T) {
	// ensure env is clean
	if err := os.Unsetenv("SOME_KEY"); err != nil {
		t.Fatalf("failed to unset env: %v", err)
	}
	defs := []ConfigItemDescription{
		{Key: "SOME_KEY", Optional: true, Default: "default-value"},
		{Key: "REQUIRED_KEY", Optional: false, Default: "req-default"},
	}

	// missing required should error
	if err := InitializeConfigModule(defs); err == nil {
		t.Fatalf("expected error when required env not set")
	}

	// set required env and re-init
	if err := os.Setenv("REQUIRED_KEY", "set-value"); err != nil {
		t.Fatalf("failed to set env: %v", err)
	}
	if err := InitializeConfigModule(defs); err != nil {
		t.Fatalf("unexpected error initializing config: %v", err)
	}

	// optional key should return default
	if v := GetValue("SOME_KEY"); v != "default-value" {
		t.Fatalf("expected default value, got %s", v)
	}

	// required key should return provided value
	if v := GetValue("REQUIRED_KEY"); v != "set-value" {
		t.Fatalf("expected set-value, got %s", v)
	}
}

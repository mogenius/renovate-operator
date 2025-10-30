package config

import (
	"os"
	"testing"
)

func TestInitializeAndGetValue(t *testing.T) {
	// ensure env is clean
	os.Unsetenv("SOME_KEY")

	defs := []ConfigItemDescription{
		{Key: "SOME_KEY", Optional: true, Default: "default-value"},
		{Key: "REQUIRED_KEY", Optional: false, Default: "req-default"},
	}

	// missing required should error
	if err := InitializeConfigModule(defs); err == nil {
		t.Fatalf("expected error when required env not set")
	}

	// set required env and re-init
	os.Setenv("REQUIRED_KEY", "set-value")
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

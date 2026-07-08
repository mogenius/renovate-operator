package ui

import (
	"os"
	"testing"

	"renovate-operator/config"
)

// TestMain ensures the singleton config module is initialized before any test
// runs. Several helpers (cookie scoping, base-path resolution) read config
// values, and individual tests may re-initialize the module with their own
// keys as needed.
func TestMain(m *testing.M) {
	if err := config.InitializeConfigModule([]config.ConfigItemDescription{
		{Key: "BASE_PATH", Optional: true, Default: ""},
		{Key: "WEBHOOK_SERVER_UNIFIED_HOST", Optional: true, Default: "false"},
	}); err != nil {
		os.Exit(1)
	}
	os.Exit(m.Run())
}

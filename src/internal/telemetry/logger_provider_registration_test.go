package telemetry

import (
	"context"
	"renovate-operator/config"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	sdklog "go.opentelemetry.io/otel/sdk/log"
)

func initializeOTelConfigForTest(t *testing.T, otlpEndpoint string) {
	t.Helper()
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", otlpEndpoint)
	t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "")
	err := config.InitializeConfigModule([]config.ConfigItemDescription{
		{Key: "OTEL_EXPORTER_OTLP_ENDPOINT", Optional: true},
		{Key: "OTEL_EXPORTER_OTLP_PROTOCOL", Optional: true},
	})
	if err != nil {
		t.Fatalf("failed to initialize config: %v", err)
	}
}

// TestSetupOTelSDK_RegistersGlobalLoggerProvider checks that the global
// LoggerProvider stays a no-op while OTel is disabled and becomes the SDK
// provider once SetupOTelSDK enables it. The OTel global can only be delegated
// once per process, so both phases run in order inside a single test.
func TestSetupOTelSDK_RegistersGlobalLoggerProvider(t *testing.T) {
	t.Run("disabled keeps a no-op global provider", func(t *testing.T) {
		initializeOTelConfigForTest(t, "")

		_, enabled, err := SetupOTelSDK(context.Background(), "test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if enabled {
			t.Fatal("expected OTel to be disabled without an OTLP endpoint")
		}
		if _, isSDKProvider := otel.GetLoggerProvider().(*sdklog.LoggerProvider); isSDKProvider {
			t.Fatal("expected a no-op global LoggerProvider while OTel is disabled")
		}
	})

	t.Run("enabled registers the SDK provider globally", func(t *testing.T) {
		// The gRPC exporters connect lazily, so no collector needs to listen here.
		initializeOTelConfigForTest(t, "http://127.0.0.1:4317")

		shutdown, enabled, err := SetupOTelSDK(context.Background(), "test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		t.Cleanup(func() {
			shutdownContext, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_ = shutdown(shutdownContext) // no collector listens, so flush errors are expected
			otelEnabled.Store(false)
		})
		if !enabled {
			t.Fatal("expected OTel to be enabled with an OTLP endpoint")
		}
		if _, isSDKProvider := otel.GetLoggerProvider().(*sdklog.LoggerProvider); !isSDKProvider {
			t.Fatalf("expected the SDK LoggerProvider to be registered globally, got %T", otel.GetLoggerProvider())
		}
	})
}

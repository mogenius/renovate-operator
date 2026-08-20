package telemetry

import (
	"context"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"
)

// TestResourceMerge_SchemaURLConsistency exercises the exact resource.Merge call
// inside SetupOTelSDK. Mixed semconv versions produce a "conflicting Schema URL"
// error at runtime; this catches that before deployment.
func TestResourceMerge_SchemaURLConsistency(t *testing.T) {
	_, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("renovate-operator"),
			semconv.ServiceVersion("test"),
		),
	)
	if err != nil {
		t.Fatalf("resource.Merge failed — semconv version likely mismatches the OTel SDK: %v", err)
	}
}

func TestContextWithTraceLogger_NoSpan(t *testing.T) {
	var output string
	base := funcr.New(func(prefix, args string) { output = args }, funcr.Options{})
	ctx := ContextWithTraceLogger(context.Background(), base)

	got := logr.FromContextOrDiscard(ctx)
	got.Info("test")

	// Without a span, trace_id and span_id should not appear.
	if strings.Contains(output, "trace_id") || strings.Contains(output, "span_id") {
		t.Errorf("expected no trace fields, got: %s", output)
	}
}

func TestContextWithTraceLogger_WithSpan(t *testing.T) {
	traceID := trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	spanID := trace.SpanID{1, 2, 3, 4, 5, 6, 7, 8}
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})
	ctx := trace.ContextWithSpanContext(context.Background(), sc)

	var output string
	base := funcr.New(func(prefix, args string) { output = args }, funcr.Options{})
	ctx = ContextWithTraceLogger(ctx, base)

	got := logr.FromContextOrDiscard(ctx)
	got.Info("test")

	if !strings.Contains(output, traceID.String()) {
		t.Errorf("expected trace_id %s in output, got: %s", traceID.String(), output)
	}
	if !strings.Contains(output, spanID.String()) {
		t.Errorf("expected span_id %s in output, got: %s", spanID.String(), output)
	}
}

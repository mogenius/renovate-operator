package telemetry

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/contrib/bridges/otellogr"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// SetupOTelSDK initializes OpenTelemetry trace, metric, and log providers when
// OTEL_EXPORTER_OTLP_ENDPOINT is set. Returns a shutdown function that flushes
// and closes all providers, and an enabled flag indicating whether providers were
// actually configured. When the endpoint is not set, enabled is false and the
// shutdown function is a no-op (zero overhead).
//
// Only gRPC exporters are supported. If OTEL_EXPORTER_OTLP_PROTOCOL is set to a
// non-gRPC value, an error is returned.
func SetupOTelSDK(ctx context.Context, version string) (shutdown func(context.Context) error, enabled bool, err error) {
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" {
		return func(context.Context) error { return nil }, false, nil
	}

	if proto := os.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL"); proto != "" && proto != "grpc" {
		return func(context.Context) error { return nil }, false,
			fmt.Errorf("unsupported OTLP protocol %q: only \"grpc\" is supported", proto)
	}

	var shutdownFuncs []func(context.Context) error

	shutdown = func(ctx context.Context) error {
		var errs []error
		for _, fn := range shutdownFuncs {
			errs = append(errs, fn(ctx))
		}
		return errors.Join(errs...)
	}

	handleErr := func(inErr error) {
		err = errors.Join(inErr, shutdown(ctx))
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("renovate-operator"),
			semconv.ServiceVersion(version),
		),
	)
	if err != nil {
		handleErr(err)
		return
	}

	prop := propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
	otel.SetTextMapPropagator(prop)

	traceExporter, err := otlptracegrpc.New(ctx)
	if err != nil {
		handleErr(err)
		return
	}
	tracerProvider := trace.NewTracerProvider(
		trace.WithBatcher(traceExporter),
		trace.WithResource(res),
	)
	shutdownFuncs = append(shutdownFuncs, tracerProvider.Shutdown)
	otel.SetTracerProvider(tracerProvider)

	metricExporter, err := otlpmetricgrpc.New(ctx)
	if err != nil {
		handleErr(err)
		return
	}
	meterProvider := metric.NewMeterProvider(
		metric.WithReader(metric.NewPeriodicReader(metricExporter)),
		metric.WithResource(res),
	)
	shutdownFuncs = append(shutdownFuncs, meterProvider.Shutdown)
	otel.SetMeterProvider(meterProvider)

	logExporter, err := otlploggrpc.New(ctx)
	if err != nil {
		handleErr(err)
		return
	}
	loggerProvider := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(logExporter)),
		sdklog.WithResource(res),
	)
	shutdownFuncs = append(shutdownFuncs, loggerProvider.Shutdown)
	global.SetLoggerProvider(loggerProvider)

	return shutdown, true, nil
}

// NewOTelLogSink returns an otellogr.LogSink that sends log records as OTel log
// signals. When OTel is not enabled the global LoggerProvider is a no-op, so the
// returned sink silently discards records with minimal overhead. The caller should
// only tee this with the primary zap-based sink when OTel is actually enabled
// (see SetupOTelSDK's enabled return value).
func NewOTelLogSink(name string) *otellogr.LogSink {
	return otellogr.NewLogSink(name,
		otellogr.WithLoggerProvider(global.GetLoggerProvider()),
		otellogr.WithLevelSeverity(func(level int) otellog.Severity {
			switch {
			case level == 0:
				return otellog.SeverityInfo
			case level == 1:
				return otellog.SeverityDebug
			default:
				return otellog.SeverityTrace
			}
		}),
	)
}

// TeeLogSink fans out log calls to a primary and secondary logr.LogSink.
// The primary sink controls Enabled(); both sinks receive all log calls.
type TeeLogSink struct {
	primary   logr.LogSink
	secondary logr.LogSink
}

func NewTeeLogSink(primary, secondary logr.LogSink) logr.LogSink {
	return &TeeLogSink{primary: primary, secondary: secondary}
}

func (t *TeeLogSink) Init(info logr.RuntimeInfo) {
	t.primary.Init(info)
	t.secondary.Init(info)
}

func (t *TeeLogSink) Enabled(level int) bool {
	return t.primary.Enabled(level)
}

func (t *TeeLogSink) Info(level int, msg string, keysAndValues ...any) {
	t.primary.Info(level, msg, keysAndValues...)
	t.secondary.Info(level, msg, keysAndValues...)
}

func (t *TeeLogSink) Error(err error, msg string, keysAndValues ...any) {
	t.primary.Error(err, msg, keysAndValues...)
	t.secondary.Error(err, msg, keysAndValues...)
}

func (t *TeeLogSink) WithValues(keysAndValues ...any) logr.LogSink {
	return &TeeLogSink{
		primary:   t.primary.WithValues(keysAndValues...),
		secondary: t.secondary.WithValues(keysAndValues...),
	}
}

func (t *TeeLogSink) WithName(name string) logr.LogSink {
	return &TeeLogSink{
		primary:   t.primary.WithName(name),
		secondary: t.secondary.WithName(name),
	}
}

// ContextWithTraceLogger stores a trace-enriched logger in the returned context.
// Downstream code retrieves it with log.FromContext(ctx) and automatically gets
// trace_id and span_id fields. When there is no valid span the logger is stored
// unchanged, so this is always safe to call.
func ContextWithTraceLogger(ctx context.Context, logger logr.Logger) context.Context {
	sc := oteltrace.SpanFromContext(ctx).SpanContext()
	if sc.IsValid() {
		logger = logger.WithValues(
			"trace_id", sc.TraceID().String(),
			"span_id", sc.SpanID().String(),
		)
	}
	return logr.NewContext(ctx, logger)
}

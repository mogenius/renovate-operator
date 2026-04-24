package telemetry

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
	"go.opentelemetry.io/contrib/bridges/otellogr"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gorilla/mux/otelmux"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
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

// otelEnabled tracks whether OTel providers were successfully initialized.
var otelEnabled bool

// Enabled reports whether OTel was successfully initialized by SetupOTelSDK.
// Use this to conditionally register HTTP instrumentation middlewares/transports.
func Enabled() bool {
	return otelEnabled
}

// SetupOTelSDK initializes OpenTelemetry trace, metric, and log providers when
// OTEL_EXPORTER_OTLP_ENDPOINT is set. Returns a shutdown function that flushes
// and closes all providers, and an enabled flag indicating whether providers were
// actually configured. When the endpoint is not set, enabled is false and the
// shutdown function is a no-op (zero overhead).
//
// Only gRPC exporters are supported. If OTEL_EXPORTER_OTLP_PROTOCOL is set to a
// non-gRPC value, an error is returned.
func SetupOTelSDK(ctx context.Context, version string) (shutdown func(context.Context) error, enabled bool, err error) {
	otelEnabled = false

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
		// Use a fresh context for shutdown so cleanup is best-effort even when
		// the init context has been cancelled or timed out.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err = errors.Join(inErr, shutdown(shutdownCtx))
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

	// Register all providers only after all are successfully created, so a
	// partial failure doesn't leave globals pointing at shut-down providers.
	otel.SetTracerProvider(tracerProvider)
	otel.SetMeterProvider(meterProvider)
	global.SetLoggerProvider(loggerProvider)
	otelEnabled = true

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
			if level == 0 {
				return otellog.SeverityInfo
			}
			return otellog.SeverityDebug
		}),
	)
}

// TeeLogSink fans out log calls to a primary and secondary logr.LogSink.
// The primary sink controls Enabled(); the secondary only receives entries that
// pass the primary's Enabled check (logr short-circuits before calling Info).
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

// WithCallDepth delegates call-depth adjustment to both sinks so that
// caller/file attribution remains correct when the logger is wrapped.
func (t *TeeLogSink) WithCallDepth(depth int) logr.LogSink {
	primary := t.primary
	if s, ok := t.primary.(interface{ WithCallDepth(int) logr.LogSink }); ok {
		primary = s.WithCallDepth(depth)
	}
	secondary := t.secondary
	if s, ok := t.secondary.(interface{ WithCallDepth(int) logr.LogSink }); ok {
		secondary = s.WithCallDepth(depth)
	}
	return &TeeLogSink{primary: primary, secondary: secondary}
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

// WrapTransport wraps an http.RoundTripper with otelhttp instrumentation when
// OTel is enabled. Returns the transport unchanged when OTel is disabled (zero overhead).
func WrapTransport(base http.RoundTripper) http.RoundTripper {
	if !otelEnabled {
		return base
	}
	return otelhttp.NewTransport(base)
}

// MuxMiddleware returns an otelmux.Middleware for the given server name when
// OTel is enabled. Returns a no-op middleware when OTel is disabled.
func MuxMiddleware(serverName string) mux.MiddlewareFunc {
	if !otelEnabled {
		return func(next http.Handler) http.Handler { return next }
	}
	return otelmux.Middleware(serverName)
}

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	serviceName     = "gofipe"
	fipePeerService = "fipe-api"
)

// telemetryEnabled reports whether an OTLP endpoint was configured through the
// standard OpenTelemetry environment variables and the SDK is not disabled.
func telemetryEnabled() bool {
	if strings.EqualFold(os.Getenv("OTEL_SDK_DISABLED"), "true") {
		return false
	}
	for _, v := range []string{
		"OTEL_EXPORTER_OTLP_ENDPOINT",
		"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT",
		"OTEL_EXPORTER_OTLP_LOGS_ENDPOINT",
	} {
		if os.Getenv(v) != "" {
			return true
		}
	}
	return false
}

// newJSONLogHandler returns the JSON log format used everywhere (stdout and
// OTLP): one JSON object per line with time, level, msg, the record attributes
// and, inside a span, trace_id and span_id.
func newJSONLogHandler(w io.Writer) slog.Handler {
	return newTraceContextHandler(slog.NewJSONHandler(w, nil))
}

// setupTelemetry configures the default JSON logger and, when an OTLP endpoint
// is configured, exports traces and logs with OpenTelemetry (OTLP over HTTP).
// Exporters read the standard OTEL_* environment variables. The returned
// function flushes and stops the exporters.
func setupTelemetry(ctx context.Context, stdout io.Writer) (func(context.Context) error, error) {
	console := newJSONLogHandler(stdout)
	noop := func(context.Context) error { return nil }

	if !telemetryEnabled() {
		slog.SetDefault(slog.New(console))
		return noop, nil
	}

	res, err := resource.New(ctx,
		// Defaults first: OTEL_SERVICE_NAME / OTEL_RESOURCE_ATTRIBUTES override them.
		resource.WithAttributes(semconv.ServiceName(serviceName), semconv.ServiceVersion(appVersion)),
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
		resource.WithHost(),
	)
	if err != nil {
		return noop, fmt.Errorf("otel resource: %w", err)
	}

	traceExporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return noop, fmt.Errorf("otlp trace exporter: %w", err)
	}
	tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(traceExporter), sdktrace.WithResource(res))

	logExporter, err := otlploghttp.New(ctx)
	if err != nil {
		return noop, errors.Join(fmt.Errorf("otlp log exporter: %w", err), tp.Shutdown(ctx))
	}
	lp := sdklog.NewLoggerProvider(sdklog.WithProcessor(sdklog.NewBatchProcessor(logExporter)), sdklog.WithResource(res))

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	// The same JSON line goes to stdout and, as the OTLP log body, to the collector.
	slog.SetDefault(slog.New(slog.NewMultiHandler(
		console,
		newOTLPJSONHandler(lp.Logger(serviceName)),
	)))

	return func(ctx context.Context) error {
		return errors.Join(tp.Shutdown(ctx), lp.Shutdown(ctx))
	}, nil
}

// otlpJSONHandler formats records as JSON (the same format as stdout) and
// emits each line as the body of an OpenTelemetry log record. The OTel SDK
// attaches the trace context from ctx, so Loki stores trace_id/span_id as
// structured metadata and the line itself can be parsed with LogQL "| json".
type otlpJSONHandler struct {
	json   slog.Handler
	line   *lineBuffer
	logger otellog.Logger
}

// lineBuffer receives the output of the JSON handler; mu serializes
// format-then-emit across handlers derived with WithAttrs/WithGroup.
type lineBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lineBuffer) Write(p []byte) (int, error) { return b.buf.Write(p) }

func newOTLPJSONHandler(logger otellog.Logger) *otlpJSONHandler {
	line := &lineBuffer{}
	return &otlpJSONHandler{json: newJSONLogHandler(line), line: line, logger: logger}
}

func (h *otlpJSONHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.json.Enabled(ctx, level)
}

func (h *otlpJSONHandler) Handle(ctx context.Context, r slog.Record) error {
	h.line.mu.Lock()
	defer h.line.mu.Unlock()

	h.line.buf.Reset()
	if err := h.json.Handle(ctx, r); err != nil {
		return err
	}
	var rec otellog.Record
	rec.SetTimestamp(r.Time)
	rec.SetObservedTimestamp(time.Now())
	rec.SetSeverity(otelSeverity(r.Level))
	rec.SetSeverityText(r.Level.String())
	rec.SetBody(attribute.StringValue(strings.TrimSuffix(h.line.buf.String(), "\n")))
	h.logger.Emit(ctx, rec)
	return nil
}

func (h *otlpJSONHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &otlpJSONHandler{json: h.json.WithAttrs(attrs), line: h.line, logger: h.logger}
}

func (h *otlpJSONHandler) WithGroup(name string) slog.Handler {
	return &otlpJSONHandler{json: h.json.WithGroup(name), line: h.line, logger: h.logger}
}

// otelSeverity maps slog levels to OpenTelemetry severities
// (DEBUG=-4 -> 5, INFO=0 -> 9, WARN=4 -> 13, ERROR=8 -> 17).
func otelSeverity(level slog.Level) otellog.Severity {
	return otellog.Severity(min(max(int(level)+9, int(otellog.SeverityTrace1)), int(otellog.SeverityFatal4)))
}

// traceContextHandler adds trace_id and span_id to records logged with a
// context that carries a valid span, so logs can be correlated with traces.
// The IDs are always top-level keys (never inside a WithGroup group), which is
// where Grafana/Loki look for them: groups and attributes are replayed on top
// of the base handler after the trace attributes.
type traceContextHandler struct {
	base slog.Handler
	ops  []func(slog.Handler) slog.Handler // WithAttrs/WithGroup calls, in order
	h    slog.Handler                      // base with ops applied (no trace context)
}

func newTraceContextHandler(base slog.Handler) *traceContextHandler {
	return &traceContextHandler{base: base, h: base}
}

func (t *traceContextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return t.h.Enabled(ctx, level)
}

func (t *traceContextHandler) Handle(ctx context.Context, r slog.Record) error {
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return t.h.Handle(ctx, r)
	}
	h := t.base.WithAttrs([]slog.Attr{
		slog.String("trace_id", sc.TraceID().String()),
		slog.String("span_id", sc.SpanID().String()),
	})
	for _, op := range t.ops {
		h = op(h)
	}
	return h.Handle(ctx, r)
}

func (t *traceContextHandler) with(op func(slog.Handler) slog.Handler) slog.Handler {
	return &traceContextHandler{
		base: t.base,
		ops:  append(slices.Clip(t.ops), op),
		h:    op(t.h),
	}
}

func (t *traceContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return t.with(func(h slog.Handler) slog.Handler { return h.WithAttrs(attrs) })
}

func (t *traceContextHandler) WithGroup(name string) slog.Handler {
	return t.with(func(h slog.Handler) slog.Handler { return h.WithGroup(name) })
}

// untraced reports whether path is a probe or scrape endpoint that would only
// add noise to traces and logs.
func untraced(path string) bool {
	return path == "/metrics" || path == "/health" || strings.HasPrefix(path, "/health/")
}

// traceHandler creates a server span per request, named after the matched route.
// With no tracer provider configured, OpenTelemetry uses a no-op implementation.
func traceHandler(next http.Handler) http.Handler {
	return otelhttp.NewHandler(next, serviceName,
		otelhttp.WithFilter(func(r *http.Request) bool { return !untraced(r.URL.Path) }),
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
			if r.Pattern == "" {
				return r.Method
			}
			return r.Method + " " + routeLabel(r.Pattern)
		}),
	)
}

// tracedTransport creates a client span for each request sent to FIPE and
// propagates the trace context.
func tracedTransport(base http.RoundTripper) http.RoundTripper {
	return otelhttp.NewTransport(base,
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
			return "FIPE " + r.Method
		}),
		// peer.service names the remote side, so Tempo's service graph shows
		// the FIPE API as a node even though it does not send traces itself.
		otelhttp.WithSpanOptions(trace.WithAttributes(attribute.String("peer.service", fipePeerService))),
	)
}

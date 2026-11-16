package main

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/noop"
	"go.opentelemetry.io/otel/trace"
)

func TestParseFipePrice(t *testing.T) {
	tests := []struct {
		in   string
		want float64
	}{
		{"R$ 71.229,00", 71229},
		{"R$ 1.234.567,89", 1234567.89},
		{"R$ 999,50", 999.5},
		{"1,234.56", 1234.56},
		{"71.229", 71229},
		{"12.5", 12.5},
		{"  R$ 10 ", 10},
	}
	for _, tt := range tests {
		got, err := parseFipePrice(tt.in)
		if err != nil {
			t.Errorf("parseFipePrice(%q) error: %v", tt.in, err)
			continue
		}
		if math.Abs(got-tt.want) > 1e-9 {
			t.Errorf("parseFipePrice(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
	if _, err := parseFipePrice("N/A"); err == nil {
		t.Error("parseFipePrice(\"N/A\") expected error")
	}
}

func TestRouteLabel(t *testing.T) {
	cases := map[string]string{
		"":                "unmatched",
		"GET /{$}":        "/",
		"GET /api/brands": "/api/brands",
		"GET /static/":    "/static/",
		"/plain":          "/plain",
	}
	for in, want := range cases {
		if got := routeLabel(in); got != want {
			t.Errorf("routeLabel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHistoryMonths(t *testing.T) {
	cases := map[string]int{"": 12, "abc": 12, "3": 3, "0": 1, "-5": 1, "999": maxHistoryMonths}
	for in, want := range cases {
		q := map[string][]string{"months": {in}}
		if got := historyMonths(q); got != want {
			t.Errorf("historyMonths(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestTTLCache(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c := newTTLCache()
	c.now = func() time.Time { return now }

	c.set("k", []byte("v"), time.Minute)
	if got, ok := c.get("k"); !ok || string(got) != "v" {
		t.Fatalf("get after set = %q, %v", got, ok)
	}
	now = now.Add(2 * time.Minute)
	if _, ok := c.get("k"); ok {
		t.Fatal("expected entry to be expired")
	}
	c.purgeExpired()
	if len(c.items) != 0 {
		t.Fatalf("purgeExpired left %d items", len(c.items))
	}
}

func TestPriceTrackerKeepsMinAndMax(t *testing.T) {
	p := newPriceTracker()
	for _, v := range []float64{100, 50, 200, 150} {
		p.observe("b", "m", "y", v)
	}
	if r := p.seen[[3]string{"b", "m", "y"}]; r.low != 50 || r.high != 200 {
		t.Fatalf("range = %+v, want low=50 high=200", r)
	}
}

// fakeFipe emulates the subset of the FIPE v2 API used by the app.
func fakeFipe(t *testing.T, calls *atomic.Int32) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /cars/brands", func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = io.WriteString(w, `[{"code":"59","name":"VW - VolksWagen"}]`)
	})
	mux.HandleFunc("GET /cars/brands/59/models/5940/years/2011-3", func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = io.WriteString(w, `{"vehicleType":1,"price":"R$ 71.229,00","brand":"VW - VolksWagen","model":"AMAROK","modelYear":2011,"fuel":"Diesel","codeFipe":"005340-6","referenceMonth":"novembro de 2026","fuelAcronym":"D"}`)
	})
	mux.HandleFunc("GET /cars/005340-6/years/2011-3/history", func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = io.WriteString(w, `{"vehicleType":1,"brand":"VW - VolksWagen","model":"AMAROK","modelYear":2011,"fuel":"Diesel","codeFipe":"005340-6","fuelAcronym":"D","priceHistory":[
			{"price":"R$ 71.229,00","month":"novembro de 2026","reference":"339"},
			{"price":"R$ 68.821,00","month":"outubro de 2026","reference":"338"},
			{"price":"R$ 68.126,00","month":"setembro de 2026","reference":"337"}]}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func newTestApp(t *testing.T) (http.Handler, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	upstream := fakeFipe(t, &calls)
	app, err := newServer(config{FipeBaseURL: upstream.URL})
	if err != nil {
		t.Fatal(err)
	}
	return app.routes(), &calls
}

func get(t *testing.T, h http.Handler, target string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, target, nil))
	return rec
}

func TestHandlers(t *testing.T) {
	h, calls := newTestApp(t)

	tests := []struct {
		name, target string
		wantStatus   int
		wantBody     string
	}{
		{"health", "/health", http.StatusOK, `{"status": "ok"}`},
		{"liveness", "/health/live", http.StatusOK, `{"status": "ok"}`},
		{"not ready before start", "/health/ready", http.StatusServiceUnavailable, `{"status": "unavailable"}`},
		{"index", "/", http.StatusOK, "<!DOCTYPE html>"},
		{"unknown path is 404", "/does-not-exist", http.StatusNotFound, ""},
		{"static asset", "/static/js/app.js", http.StatusOK, "DOMContentLoaded"},
		{"no static dir listing", "/static/js/", http.StatusNotFound, ""},
		{"brands default type", "/api/brands", http.StatusOK, "VolksWagen"},
		{"invalid type", "/api/brands?type=../../etc", http.StatusBadRequest, "invalid type"},
		{"missing brandId", "/api/models?type=cars", http.StatusBadRequest, "brandId"},
		{"injection in id", "/api/models?type=cars&brandId=59%2F..%2Fx", http.StatusBadRequest, "brandId"},
		{"upstream 404", "/api/models?type=cars&brandId=1", http.StatusNotFound, ""},
		{"price", "/api/price?type=cars&brandId=59&modelId=5940&yearId=2011-3", http.StatusOK, `"fuelAcronym":"D"`},
		{"metrics", "/metrics", http.StatusOK, "fipe_search_stats"},
		{"latency histogram", "/metrics", http.StatusOK, `fipe_http_request_duration_seconds_bucket{code="200",method="GET",path="/api/brands"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := get(t, h, tt.target)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %q)", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Fatalf("body %q does not contain %q", rec.Body.String(), tt.wantBody)
			}
		})
	}

	// Brands must be served from cache on the second call.
	before := calls.Load()
	get(t, h, "/api/brands?type=cars")
	if calls.Load() != before {
		t.Error("expected /api/brands to be served from cache")
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/brands", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /api/brands status = %d, want 405", rec.Code)
	}
}

func TestPriceHistory(t *testing.T) {
	h, _ := newTestApp(t)

	rec := get(t, h, "/api/priceHistory?type=cars&brandId=59&modelId=5940&yearId=2011-3&months=2")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %q", rec.Code, rec.Body.String())
	}
	var got historyResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.History) != 2 {
		t.Fatalf("got %d entries, want 2 (months=2)", len(got.History))
	}
	first := got.History[0]
	if first.ReferenceMonth != "novembro de 2026" || first.Price != "R$ 71.229,00" || first.CodeFipe != "005340-6" || first.Model != "AMAROK" {
		t.Errorf("unexpected first entry: %+v", first)
	}
	if got.History[1].ReferenceMonth != "outubro de 2026" {
		t.Errorf("second entry month = %q", got.History[1].ReferenceMonth)
	}
}

func TestReadiness(t *testing.T) {
	app, err := newServer(config{FipeBaseURL: "http://127.0.0.1:1"})
	if err != nil {
		t.Fatal(err)
	}
	h := app.routes()

	if rec := get(t, h, "/health/ready"); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("before start: status = %d, want 503", rec.Code)
	}
	app.ready.Store(true)
	if rec := get(t, h, "/health/ready"); rec.Code != http.StatusOK || rec.Body.String() != `{"status": "ok"}` {
		t.Fatalf("ready: status = %d body = %q", rec.Code, rec.Body.String())
	}
	app.ready.Store(false)
	if rec := get(t, h, "/health/ready"); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("shutting down: status = %d, want 503", rec.Code)
	}
	// Liveness stays OK while draining, so Kubernetes does not restart the pod.
	if rec := get(t, h, "/health/live"); rec.Code != http.StatusOK {
		t.Fatalf("liveness while draining: status = %d, want 200", rec.Code)
	}
}

func TestLoadConfigShutdownDelay(t *testing.T) {
	t.Setenv("SHUTDOWN_DELAY", "5s")
	cfg, err := loadConfig()
	if err != nil || cfg.ShutdownDelay != 5*time.Second {
		t.Fatalf("got %v, %v; want 5s", cfg.ShutdownDelay, err)
	}
	for _, bad := range []string{"five", "-1s"} {
		t.Setenv("SHUTDOWN_DELAY", bad)
		if _, err := loadConfig(); err == nil {
			t.Errorf("SHUTDOWN_DELAY=%q: expected error", bad)
		}
	}
}

// sampledContext returns a request context carrying a fixed, sampled span.
func sampledContext(t *testing.T) (trace.SpanContext, *http.Request) {
	t.Helper()
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{0x4b, 0xf9, 0x2f, 0x35, 0x77, 0xb3, 0x4d, 0xa6, 0xa3, 0xce, 0x92, 0x9d, 0x0e, 0x0e, 0x47, 0x36},
		SpanID:     trace.SpanID{0x00, 0xf0, 0x67, 0xaa, 0x0b, 0xa9, 0x02, 0xb7},
		TraceFlags: trace.FlagsSampled,
	})
	ctx := trace.ContextWithSpanContext(t.Context(), sc)
	return sc, httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/brands", nil)
}

func TestJSONLogsCarryTraceContext(t *testing.T) {
	sc, r := sampledContext(t)
	var buf bytes.Buffer
	logger := slog.New(newJSONLogHandler(&buf)).With("component", "test")

	logger.InfoContext(r.Context(), "hello", "status", "200")
	var got map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &got); err != nil {
		t.Fatalf("log line is not JSON: %q (%v)", buf.String(), err)
	}
	want := map[string]any{
		"level": "INFO", "msg": "hello", "component": "test", "status": "200",
		"trace_id": sc.TraceID().String(), "span_id": sc.SpanID().String(),
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %v, want %v", k, got[k], v)
		}
	}

	buf.Reset()
	logger.Info("no span")
	if strings.Contains(buf.String(), "trace_id") {
		t.Errorf("log without span should not have trace_id: %q", buf.String())
	}
}

// recordingLogger is an OpenTelemetry logger that keeps emitted records.
type recordingLogger struct {
	noop.Logger
	records []otellog.Record
}

func (l *recordingLogger) Emit(_ context.Context, r otellog.Record) { l.records = append(l.records, r) }

func TestOTLPLogBodyIsJSON(t *testing.T) {
	sc, r := sampledContext(t)
	rl := &recordingLogger{}
	logger := slog.New(newOTLPJSONHandler(rl)).WithGroup("req").With("route", "/api/brands")

	logger.WarnContext(r.Context(), "slow request")
	if len(rl.records) != 1 {
		t.Fatalf("emitted %d records, want 1", len(rl.records))
	}
	rec := rl.records[0]
	if rec.Severity() != otellog.SeverityWarn || rec.SeverityText() != "WARN" {
		t.Errorf("severity = %v %q, want WARN", rec.Severity(), rec.SeverityText())
	}
	var got struct {
		Msg     string            `json:"msg"`
		TraceID string            `json:"trace_id"`
		Req     map[string]string `json:"req"`
	}
	body := rec.Body().AsString()
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("body is not JSON: %q (%v)", body, err)
	}
	if got.Msg != "slow request" || got.Req["route"] != "/api/brands" || got.TraceID != sc.TraceID().String() {
		t.Errorf("unexpected body %q", body)
	}
}

func TestLatencyExemplarHasTraceID(t *testing.T) {
	sc, r := sampledContext(t)
	observeDuration(r, "/exemplar-test", http.MethodGet, "200", 42*time.Millisecond)

	h, _ := newTestApp(t)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/metrics", nil)
	req.Header.Set("Accept", "application/openmetrics-text; version=1.0.0")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	want := `trace_id="` + sc.TraceID().String() + `"`
	if !strings.Contains(rec.Body.String(), want) {
		t.Fatalf("OpenMetrics output does not contain exemplar %s", want)
	}
}

func TestAccessLogIsJSON(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(newJSONLogHandler(&buf)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	h, _ := newTestApp(t)
	get(t, h, "/api/brands?type=cars")
	get(t, h, "/health/live") // probes are not logged

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("got %d log lines, want 1: %q", len(lines), buf.String())
	}
	var got struct {
		Msg        string `json:"msg"`
		Method     string `json:"method"`
		Route      string `json:"route"`
		Status     int    `json:"status"`
		DurationMS *int64 `json:"duration_ms"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &got); err != nil {
		t.Fatalf("access log is not JSON: %q (%v)", lines[0], err)
	}
	if got.Msg != "http request" || got.Method != http.MethodGet || got.Route != "/api/brands" || got.Status != http.StatusOK || got.DurationMS == nil {
		t.Errorf("unexpected access log %q", lines[0])
	}
}

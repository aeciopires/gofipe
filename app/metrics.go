package main

import (
	"cmp"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/trace"
)

var (
	// httpRequestsCounter counts incoming HTTP requests by route and method.
	httpRequestsCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "fipe_http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"path", "method"},
	)

	// httpRequestDuration measures request latency by route, method and status code.
	// Observations carry the trace ID as an exemplar, linking metrics to traces.
	httpRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "fipe_http_request_duration_seconds",
			Help:    "Duration of HTTP requests in seconds",
			Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		},
		[]string{"path", "method", "code"},
	)

	// vehicleSearchCounter counts vehicle searches labeled by brand, model and year.
	vehicleSearchCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "fipe_search_stats",
			Help: "Counter for specific vehicle searches by brand, model, and year",
		},
		[]string{"brand_name", "model_name", "year_id"},
	)

	// minPriceGauge stores the minimum observed price per vehicle label.
	minPriceGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "fipe_price_min",
			Help: "Minimum observed price for searches",
		},
		[]string{"brand_name", "model_name", "year_id"},
	)

	// maxPriceGauge stores the maximum observed price per vehicle label.
	maxPriceGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "fipe_price_max",
			Help: "Maximum observed price for searches",
		},
		[]string{"brand_name", "model_name", "year_id"},
	)

	// fuelTypeCounter counts searches grouped by fuel type.
	fuelTypeCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "fipe_fuel_count",
			Help: "Count of searches by fuel type",
		},
		[]string{"fuel"},
	)

	// brandSearchCounter counts searches by brand name.
	brandSearchCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "fipe_brand_search_count",
			Help: "Count of searches by brand",
		},
		[]string{"brand_name"},
	)
)

func init() {
	prometheus.MustRegister(
		httpRequestsCounter,
		httpRequestDuration,
		vehicleSearchCounter,
		minPriceGauge,
		maxPriceGauge,
		fuelTypeCounter,
		brandSearchCounter,
	)
}

// recordHTTPRequest increments the HTTP requests counter for a route and method.
func recordHTTPRequest(path, method string) {
	httpRequestsCounter.WithLabelValues(path, method).Inc()
}

// knownMethods bounds the "method" label so arbitrary client input cannot
// create unlimited time series.
var knownMethods = map[string]bool{
	http.MethodGet: true, http.MethodHead: true, http.MethodPost: true,
	http.MethodPut: true, http.MethodPatch: true, http.MethodDelete: true,
	http.MethodOptions: true,
}

// statusRecorder captures the status code written by a handler.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	if r.status == 0 {
		r.status = code
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// instrument records fipe_http_requests_total and fipe_http_request_duration_seconds
// for every request using the matched ServeMux route (not the raw URL), keeping
// label cardinality bounded, and writes an access log line correlated with the trace.
func instrument(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		elapsed := time.Since(start)

		method := r.Method
		if !knownMethods[method] {
			method = "OTHER"
		}
		route := routeLabel(r.Pattern)
		status := cmp.Or(rec.status, http.StatusOK) // 0 = never written: net/http sends 200
		code := strconv.Itoa(status)
		recordHTTPRequest(route, method)
		observeDuration(r, route, method, code, elapsed)

		if !untraced(r.URL.Path) {
			slog.InfoContext(r.Context(), "http request",
				"method", method, "route", route, "status", status,
				"duration_ms", elapsed.Milliseconds())
		}
	})
}

// observeDuration records the request latency, attaching the trace ID as an
// exemplar when the request is part of a sampled trace.
func observeDuration(r *http.Request, route, method, code string, elapsed time.Duration) {
	obs := httpRequestDuration.WithLabelValues(route, method, code)
	sc := trace.SpanContextFromContext(r.Context())
	if eo, ok := obs.(prometheus.ExemplarObserver); ok && sc.IsSampled() {
		eo.ObserveWithExemplar(elapsed.Seconds(), prometheus.Labels{"trace_id": sc.TraceID().String()})
		return
	}
	obs.Observe(elapsed.Seconds())
}

// routeLabel converts a ServeMux pattern such as "GET /api/brands" into the
// path label "/api/brands". Unmatched requests are grouped as "unmatched".
func routeLabel(pattern string) string {
	if pattern == "" {
		return "unmatched"
	}
	_, path, found := strings.Cut(pattern, " ")
	if !found {
		path = pattern
	}
	return strings.TrimSuffix(path, "{$}")
}

// priceRange holds the lowest and highest price seen for a vehicle.
type priceRange struct {
	low, high float64
}

// priceTracker keeps the real minimum and maximum observed price per vehicle,
// because a Prometheus gauge cannot be read back to compare values.
type priceTracker struct {
	mu   sync.Mutex
	seen map[[3]string]priceRange
}

func newPriceTracker() *priceTracker {
	return &priceTracker{seen: make(map[[3]string]priceRange)}
}

// observe updates the min/max gauges for the given vehicle with price.
func (t *priceTracker) observe(brand, model, year string, price float64) {
	key := [3]string{brand, model, year}

	t.mu.Lock()
	defer t.mu.Unlock()

	r, ok := t.seen[key]
	if !ok {
		r = priceRange{low: price, high: price}
	} else {
		r.low = min(r.low, price)
		r.high = max(r.high, price)
	}
	t.seen[key] = r

	minPriceGauge.WithLabelValues(brand, model, year).Set(r.low)
	maxPriceGauge.WithLabelValues(brand, model, year).Set(r.high)
}

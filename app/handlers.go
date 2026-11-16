package main

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Cache TTLs per kind of data. FIPE publishes a new reference table once a month.
const (
	brandsTTL  = 12 * time.Hour
	modelsTTL  = 12 * time.Hour
	yearsTTL   = 24 * time.Hour
	priceTTL   = 1 * time.Hour
	historyTTL = 6 * time.Hour

	defaultHistoryMonths = 12
	maxHistoryMonths     = 24
)

var (
	// vehicleTypes are the only vehicle types the FIPE v2 API understands.
	vehicleTypes = []string{"cars", "motorcycles", "trucks"}
	// idPattern restricts IDs (brand, model, year, FIPE code) to URL-safe tokens,
	// preventing path traversal or query injection towards the upstream API.
	idPattern = regexp.MustCompile(`^[0-9A-Za-z-]{1,32}$`)
)

// server bundles the dependencies shared by HTTP handlers.
type server struct {
	fipe   *fipeClient
	cache  *ttlCache
	prices *priceTracker
	tmpl   *template.Template
	static fs.FS
	// ready is false while starting up and after shutdown begins, so
	// Kubernetes stops sending traffic before the server closes.
	ready atomic.Bool
}

// routes registers every endpoint and wraps the mux with instrumentation.
func (s *server) routes() http.Handler {
	mux := http.NewServeMux()

	// Frontend
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.Handle("GET /static/", http.StripPrefix("/static/", noDirListing(http.FileServerFS(s.static))))

	// System
	mux.HandleFunc("GET /health", handleHealth)      // liveness (kept for compatibility)
	mux.HandleFunc("GET /health/live", handleHealth) // liveness
	mux.HandleFunc("GET /health/ready", s.handleReady)
	// OpenMetrics format is required to expose exemplars (trace IDs) to Prometheus.
	mux.Handle("GET /metrics", promhttp.HandlerFor(prometheus.DefaultGatherer, promhttp.HandlerOpts{EnableOpenMetrics: true}))

	// API Proxy Routes (BFF)
	mux.HandleFunc("GET /api/brands", s.handleBrands)
	mux.HandleFunc("GET /api/models", s.handleModels)
	mux.HandleFunc("GET /api/years", s.handleYears)
	mux.HandleFunc("GET /api/price", s.handlePrice)
	mux.HandleFunc("GET /api/priceHistory", s.handlePriceHistory)

	return traceHandler(instrument(mux))
}

// --- Frontend & system handlers ---

func (s *server) handleIndex(w http.ResponseWriter, r *http.Request) {
	var buf bytes.Buffer
	if err := s.tmpl.Execute(&buf, nil); err != nil {
		slog.ErrorContext(r.Context(), "render index", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = buf.WriteTo(w)
}

// handleHealth is the liveness check used by Docker and Kubernetes: it answers
// 200 while the process can serve HTTP. Keep the response shape stable.
func handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status": "ok"}`))
}

// handleReady is the readiness check: 200 when the server accepts traffic,
// 503 while starting or shutting down. It does not check FIPE, because an
// upstream outage must not remove every replica from the load balancer.
func (s *server) handleReady(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if !s.ready.Load() {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"status": "unavailable"}`))
		return
	}
	_, _ = w.Write([]byte(`{"status": "ok"}`))
}

// noDirListing returns 404 for directory requests instead of an index page.
func noDirListing(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "" || strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// --- Request parsing helpers ---

// badRequestError is returned when query parameters are missing or invalid.
type badRequestError struct{ msg string }

func (e *badRequestError) Error() string { return e.msg }

// vehicleType returns the validated "type" parameter (default "cars").
func vehicleType(q url.Values) (string, error) {
	t := cmp.Or(q.Get("type"), "cars")
	if !slices.Contains(vehicleTypes, t) {
		return "", &badRequestError{fmt.Sprintf("invalid type %q: use one of %s", t, strings.Join(vehicleTypes, ", "))}
	}
	return t, nil
}

// requireIDs returns the validated values of the named ID parameters, in order.
func requireIDs(q url.Values, names ...string) ([]string, error) {
	ids := make([]string, 0, len(names))
	for _, name := range names {
		v := q.Get(name)
		if !idPattern.MatchString(v) {
			return nil, &badRequestError{fmt.Sprintf("missing or invalid parameter %q", name)}
		}
		ids = append(ids, v)
	}
	return ids, nil
}

// parseQuery validates "type" plus the given ID parameters.
func parseQuery(r *http.Request, idNames ...string) (vType string, ids []string, err error) {
	q := r.URL.Query()
	if vType, err = vehicleType(q); err != nil {
		return "", nil, err
	}
	if ids, err = requireIDs(q, idNames...); err != nil {
		return "", nil, err
	}
	return vType, ids, nil
}

// --- Response helpers ---

func writeJSON(w http.ResponseWriter, data []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(data) //nolint:gosec // G705: JSON with nosniff, never rendered as HTML
}

// writeError logs the detailed error and returns a safe message to the client.
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	if br, ok := errors.AsType[*badRequestError](err); ok {
		http.Error(w, br.msg, http.StatusBadRequest)
		return
	}
	status := upstreamStatus(err)
	slog.WarnContext(r.Context(), "upstream request failed", "path", r.URL.Path, "status", status, "error", err)
	http.Error(w, http.StatusText(status), status)
}

// cachedGet returns the cached value for key or fetches path from FIPE and caches it.
func (s *server) cachedGet(ctx context.Context, key, path string, ttl time.Duration) ([]byte, error) {
	if data, ok := s.cache.get(key); ok {
		return data, nil
	}
	data, err := s.fipe.get(ctx, path)
	if err != nil {
		return nil, err
	}
	s.cache.set(key, data, ttl)
	return data, nil
}

// proxy serves a cached FIPE resource as JSON.
func (s *server) proxy(w http.ResponseWriter, r *http.Request, key, path string, ttl time.Duration) {
	data, err := s.cachedGet(r.Context(), key, path, ttl)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, data)
}

// --- API handlers ---

// handleBrands: GET /api/brands?type=cars
func (s *server) handleBrands(w http.ResponseWriter, r *http.Request) {
	vType, _, err := parseQuery(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	s.proxy(w, r, "brands:"+vType, "/"+vType+"/brands", brandsTTL)
}

// handleModels: GET /api/models?type=cars&brandId=59
func (s *server) handleModels(w http.ResponseWriter, r *http.Request) {
	vType, ids, err := parseQuery(r, "brandId")
	if err != nil {
		writeError(w, r, err)
		return
	}
	brandID := ids[0]
	s.proxy(w, r,
		fmt.Sprintf("models:%s:%s", vType, brandID),
		fmt.Sprintf("/%s/brands/%s/models", vType, brandID),
		modelsTTL)
}

// handleYears: GET /api/years?type=cars&brandId=59&modelId=5940
func (s *server) handleYears(w http.ResponseWriter, r *http.Request) {
	vType, ids, err := parseQuery(r, "brandId", "modelId")
	if err != nil {
		writeError(w, r, err)
		return
	}
	brandID, modelID := ids[0], ids[1]
	s.proxy(w, r,
		fmt.Sprintf("years:%s:%s:%s", vType, brandID, modelID),
		fmt.Sprintf("/%s/brands/%s/models/%s/years", vType, brandID, modelID),
		yearsTTL)
}

// fetchPrice returns the raw and decoded current price of a vehicle.
func (s *server) fetchPrice(ctx context.Context, vType, brandID, modelID, yearID string) ([]byte, PriceResponse, error) {
	data, err := s.cachedGet(ctx,
		fmt.Sprintf("price:%s:%s:%s:%s", vType, brandID, modelID, yearID),
		fmt.Sprintf("/%s/brands/%s/models/%s/years/%s", vType, brandID, modelID, yearID),
		priceTTL)
	if err != nil {
		return nil, PriceResponse{}, err
	}
	var pr PriceResponse
	if err := json.Unmarshal(data, &pr); err != nil {
		return data, PriceResponse{}, fmt.Errorf("decode price: %w", err)
	}
	return data, pr, nil
}

// handlePrice: GET /api/price?type=cars&brandId=59&modelId=5940&yearId=2014-3
//
// It is the only handler that records business metrics. Brand and model labels
// come from the FIPE response (not from client input) so that metric
// cardinality is bounded by the real FIPE catalog.
func (s *server) handlePrice(w http.ResponseWriter, r *http.Request) {
	vType, ids, err := parseQuery(r, "brandId", "modelId", "yearId")
	if err != nil {
		writeError(w, r, err)
		return
	}
	yearID := ids[2]

	data, pr, err := s.fetchPrice(r.Context(), vType, ids[0], ids[1], yearID)
	if err != nil && data == nil {
		writeError(w, r, err)
		return
	}
	if err == nil {
		s.recordPriceMetrics(pr, yearID)
	} else {
		slog.WarnContext(r.Context(), "price metrics skipped", "error", err)
	}
	writeJSON(w, data)
}

// recordPriceMetrics updates the business metrics for a successful price lookup.
func (s *server) recordPriceMetrics(pr PriceResponse, yearID string) {
	if pr.Brand == "" || pr.Model == "" {
		return
	}
	vehicleSearchCounter.WithLabelValues(pr.Brand, pr.Model, yearID).Inc()
	brandSearchCounter.WithLabelValues(pr.Brand).Inc()
	if f, err := parseFipePrice(pr.Price); err == nil {
		s.prices.observe(pr.Brand, pr.Model, yearID, f)
	}
	if pr.Fuel != "" {
		fuelTypeCounter.WithLabelValues(pr.Fuel).Inc()
	}
}

// historyResponse is the payload returned by /api/priceHistory.
type historyResponse struct {
	History []PriceResponse `json:"history"`
}

// historyMonths parses the "months" parameter, clamped to [1, maxHistoryMonths].
func historyMonths(q url.Values) int {
	n, err := strconv.Atoi(q.Get("months"))
	if err != nil {
		return defaultHistoryMonths
	}
	return min(max(n, 1), maxHistoryMonths)
}

// handlePriceHistory: GET /api/priceHistory?type=cars&brandId=59&modelId=5940&yearId=2014-3&months=12
//
// It looks up the vehicle's FIPE code from the current price and then calls
// the FIPE v2 history endpoint (/{type}/{codeFipe}/years/{yearId}/history).
// The free FIPE tier only returns the last few months; set FIPE_API_TOKEN to
// get more. If the history endpoint fails, the current price is returned as a
// single-point history so the chart still renders.
func (s *server) handlePriceHistory(w http.ResponseWriter, r *http.Request) {
	vType, ids, err := parseQuery(r, "brandId", "modelId", "yearId")
	if err != nil {
		writeError(w, r, err)
		return
	}
	yearID := ids[2]
	months := historyMonths(r.URL.Query())

	_, current, err := s.fetchPrice(r.Context(), vType, ids[0], ids[1], yearID)
	if err != nil {
		writeError(w, r, err)
		return
	}

	history, err := s.fetchHistory(r.Context(), vType, current, yearID)
	if err != nil || len(history) == 0 {
		if err != nil {
			slog.WarnContext(r.Context(), "price history unavailable, using current price", "error", err)
		}
		history = []PriceResponse{current}
	}
	if len(history) > months {
		history = history[:months]
	}

	body, err := json.Marshal(historyResponse{History: history})
	if err != nil {
		slog.ErrorContext(r.Context(), "encode history", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, body)
}

// fetchHistory returns the monthly price history (newest first) for a vehicle.
func (s *server) fetchHistory(ctx context.Context, vType string, current PriceResponse, yearID string) ([]PriceResponse, error) {
	if !idPattern.MatchString(current.CodeFipe) {
		return nil, fmt.Errorf("invalid FIPE code %q in price response", current.CodeFipe)
	}
	data, err := s.cachedGet(ctx,
		fmt.Sprintf("history:%s:%s:%s", vType, current.CodeFipe, yearID),
		fmt.Sprintf("/%s/%s/years/%s/history", vType, current.CodeFipe, yearID),
		historyTTL)
	if err != nil {
		return nil, err
	}

	var h fipeHistory
	if err := json.Unmarshal(data, &h); err != nil {
		return nil, fmt.Errorf("decode history: %w", err)
	}

	out := make([]PriceResponse, 0, len(h.PriceHistory))
	for _, e := range h.PriceHistory {
		if e.Price == "" {
			continue
		}
		item := h.PriceResponse
		item.Price = e.Price
		item.ReferenceMonth = e.Month
		out = append(out, item)
	}
	return out, nil
}

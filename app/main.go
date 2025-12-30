package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// --- Prometheus Metrics ---

var (
	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "fipe_http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"path", "method"},
	)

	vehicleSearchStats = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "fipe_search_stats",
			Help: "Counter for specific vehicle searches by brand, model, and year",
		},
		[]string{"brand_name", "model_name", "year_id"},
	)
	// New metrics
	priceMinGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "fipe_price_min",
			Help: "Minimum observed price for searches",
		},
		[]string{"brand_name", "model_name", "year_id"},
	)

	priceMaxGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "fipe_price_max",
			Help: "Maximum observed price for searches",
		},
		[]string{"brand_name", "model_name", "year_id"},
	)

	fuelCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "fipe_fuel_count",
			Help: "Count of searches by fuel type",
		},
		[]string{"fuel"},
	)

	brandCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "fipe_brand_search_count",
			Help: "Count of searches by brand",
		},
		[]string{"brand_name"},
	)
)

func init() {
	prometheus.MustRegister(httpRequestsTotal)
	prometheus.MustRegister(vehicleSearchStats)
	prometheus.MustRegister(priceMinGauge)
	prometheus.MustRegister(priceMaxGauge)
	prometheus.MustRegister(fuelCounter)
	prometheus.MustRegister(brandCounter)
}

// --- Data Structs (Updated for API v2) ---

// v2 uses "code" and "name" instead of "codigo" and "nome"
type ReferenceItem struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// v2 Price response uses English keys
type PriceResponse struct {
	Price          string `json:"price"`          // was "Valor"
	Brand          string `json:"brand"`          // was "Marca"
	Model          string `json:"model"`          // was "Modelo"
	ModelYear      int    `json:"modelYear"`      // was "AnoModelo"
	Fuel           string `json:"fuel"`           // was "Combustivel"
	CodeFipe       string `json:"codeFipe"`       // was "CodigoFipe"
	ReferenceMonth string `json:"referenceMonth"` // was "MesReferencia"
	VehicleType    int    `json:"vehicleType"`    // was "TipoVeiculo"
	AcronymFuel    string `json:"acronymFuel"`    // was "SiglaCombustivel"
}

// --- Simple in-memory cache ---
type cacheItem struct {
	data      []byte
	expiresAt time.Time
}

var (
	cacheMu sync.RWMutex
	cache   = map[string]cacheItem{}
)

func getCached(key string) ([]byte, bool) {
	cacheMu.RLock()
	defer cacheMu.RUnlock()
	it, ok := cache[key]
	if !ok || time.Now().After(it.expiresAt) {
		return nil, false
	}
	return it.data, true
}

func setCached(key string, data []byte, ttl time.Duration) {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	cache[key] = cacheItem{data: data, expiresAt: time.Now().Add(ttl)}
}

// --- Main Application ---

func main() {
	tmpl := template.Must(template.ParseFiles("templates/index.html"))

	mux := http.NewServeMux()

	// Frontend
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		recordMetrics(r.URL.Path, r.Method)
		tmpl.Execute(w, nil)
	})

	// Serve static assets under /static/
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	// Health Check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		recordMetrics(r.URL.Path, r.Method)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok"}`))
	})

	// Metrics
	mux.Handle("/metrics", promhttp.Handler())

	// API Proxy Routes (BFF)
	mux.HandleFunc("/api/brands", handleGetBrands)
	mux.HandleFunc("/api/models", handleGetModels)
	mux.HandleFunc("/api/years", handleGetYears)
	mux.HandleFunc("/api/price", handleGetPrice)
	mux.HandleFunc("/api/priceHistory", handleGetPriceHistory)

	port := ":8080"
	fmt.Printf("Server starting on port %s...\n", port)
	if err := http.ListenAndServe(port, mux); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

// --- Helper Functions ---

func recordMetrics(path, method string) {
	httpRequestsTotal.WithLabelValues(path, method).Inc()
}

func fetchExternal(url string) ([]byte, error) {
	// Added a User-Agent just in case v2 enforces it
	client := http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Go-Fipe-App/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("external API returned status: %d for url: %s", resp.StatusCode, url)
	}

	return io.ReadAll(resp.Body)
}

// --- API Handlers (Updated for v2 Endpoints) ---

// Base URL for v2
const BaseURL = "https://fipe.parallelum.com.br/api/v2"

// Get Brands: /api/brands?type=cars
func handleGetBrands(w http.ResponseWriter, r *http.Request) {
	recordMetrics("/api/brands", r.Method)
	vType := r.URL.Query().Get("type") // cars, motorcycles, trucks
	if vType == "" {
		vType = "cars"
	}

	key := "brands:" + vType
	if d, ok := getCached(key); ok {
		w.Header().Set("Content-Type", "application/json")
		w.Write(d)
		return
	}

	// v2 Endpoint: /{type}/brands
	url := fmt.Sprintf("%s/%s/brands", BaseURL, vType)

	data, err := fetchExternal(url)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	// cache for 12 hours
	setCached(key, data, 12*time.Hour)

	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

// Get Models: /api/models?type=cars&brandId=23
func handleGetModels(w http.ResponseWriter, r *http.Request) {
	recordMetrics("/api/models", r.Method)
	vType := r.URL.Query().Get("type")
	brandId := r.URL.Query().Get("brandId")

	key := fmt.Sprintf("models:%s:%s", vType, brandId)
	if d, ok := getCached(key); ok {
		w.Header().Set("Content-Type", "application/json")
		w.Write(d)
		return
	}

	// v2 Endpoint: /{type}/brands/{brandId}/models
	url := fmt.Sprintf("%s/%s/brands/%s/models", BaseURL, vType, brandId)

	data, err := fetchExternal(url)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	setCached(key, data, 12*time.Hour)

	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

// Get Years: /api/years?type=cars&brandId=23&modelId=5585
func handleGetYears(w http.ResponseWriter, r *http.Request) {
	recordMetrics("/api/years", r.Method)
	vType := r.URL.Query().Get("type")
	brandId := r.URL.Query().Get("brandId")
	modelId := r.URL.Query().Get("modelId")

	key := fmt.Sprintf("years:%s:%s:%s", vType, brandId, modelId)
	if d, ok := getCached(key); ok {
		w.Header().Set("Content-Type", "application/json")
		w.Write(d)
		return
	}

	// v2 Endpoint: /{type}/brands/{brandId}/models/{modelId}/years
	url := fmt.Sprintf("%s/%s/brands/%s/models/%s/years", BaseURL, vType, brandId, modelId)

	data, err := fetchExternal(url)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	setCached(key, data, 24*time.Hour)

	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

// Get Price: /api/price?type=cars&brandId=23&modelId=5585&yearId=2022-3
func handleGetPrice(w http.ResponseWriter, r *http.Request) {
	recordMetrics("/api/price", r.Method)
	vType := r.URL.Query().Get("type")
	brandId := r.URL.Query().Get("brandId")
	modelId := r.URL.Query().Get("modelId")
	yearId := r.URL.Query().Get("yearId")

	brandName := r.URL.Query().Get("brandName")
	modelName := r.URL.Query().Get("modelName")

	vehicleSearchStats.WithLabelValues(brandName, modelName, yearId).Inc()

	// increment brand count
	brandCounter.WithLabelValues(brandName).Inc()

	// v2 Endpoint: /{type}/brands/{brandId}/models/{modelId}/years/{yearId}
	url := fmt.Sprintf("%s/%s/brands/%s/models/%s/years/%s", BaseURL, vType, brandId, modelId, yearId)

	data, err := fetchExternal(url)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	// Try to parse price to update min/max metrics and fuel counts
	var pr PriceResponse
	if err := json.Unmarshal(data, &pr); err == nil {
		if f, err := parsePrice(pr.Price); err == nil {
			// set min and max to current observed value
			priceMinGauge.WithLabelValues(pr.Brand, pr.Model, yearId).Set(f)
			priceMaxGauge.WithLabelValues(pr.Brand, pr.Model, yearId).Set(f)
		}
		if pr.Fuel != "" {
			fuelCounter.WithLabelValues(pr.Fuel).Inc()
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

// fetchExternalConcurrent fetches multiple URLs concurrently and returns results in order.
func fetchExternalConcurrent(urls []string) ([][]byte, []error) {
	var wg sync.WaitGroup
	results := make([][]byte, len(urls))
	errs := make([]error, len(urls))

	for i, u := range urls {
		wg.Add(1)
		go func(idx int, url string) {
			defer wg.Done()
			b, err := fetchExternal(url)
			results[idx] = b
			errs[idx] = err
		}(i, u)
	}
	wg.Wait()
	return results, errs
}

// handleGetPriceHistory attempts to return a price history for the vehicle.
func handleGetPriceHistory(w http.ResponseWriter, r *http.Request) {
	recordMetrics("/api/priceHistory", r.Method)
	vType := r.URL.Query().Get("type")
	brandId := r.URL.Query().Get("brandId")
	modelId := r.URL.Query().Get("modelId")
	yearId := r.URL.Query().Get("yearId")
	monthsStr := r.URL.Query().Get("months")
	if monthsStr == "" {
		monthsStr = "12"
	}
	months, err := strconv.Atoi(monthsStr)
	if err != nil || months <= 0 {
		months = 12
	}

	// Try a common history path. If it fails, fallback to single-point history.
	histURL := fmt.Sprintf("%s/%s/brands/%s/models/%s/years/%s/history?months=%d", BaseURL, vType, brandId, modelId, yearId, months)
	data, err := fetchExternal(histURL)
	if err == nil {
		// Normalize the returned history payload so each item has a distinct reference label
		var raw interface{}
		if err := json.Unmarshal(data, &raw); err == nil {
			if m, ok := raw.(map[string]interface{}); ok {
				if arr, ok2 := m["history"].([]interface{}); ok2 {
					log.Printf("normalizing %d history entries (direct)\n", len(arr))
					for i := range arr {
						if item, ok3 := arr[i].(map[string]interface{}); ok3 {
							// set normalized reference label
							ref := time.Now().AddDate(0, -i, 0)
							item["referenceMonth"] = fmt.Sprintf("%02d/%d", int(ref.Month()), ref.Year())
							arr[i] = item
						}
					}
					m["history"] = arr
					if b, err := json.Marshal(m); err == nil {
						w.Header().Set("Content-Type", "application/json")
						w.Write(b)
						return
					}
				}
			}
		}
		// if normalization failed, return raw data
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
		return
	}

	// Fallback: try to query multiple past months concurrently using common query params
	results := make([]json.RawMessage, months)
	var wg sync.WaitGroup
	for i := 0; i < months; i++ {
		wg.Add(1)
		go func(offset int) {
			defer wg.Done()
			ref := time.Now().AddDate(0, -offset, 0).Format("2006-01")
			// try several candidate endpoints that some FIPE providers use for historic data
			candidates := []string{
				// query param variants
				fmt.Sprintf("%s/%s/brands/%s/models/%s/years/%s?referenceMonth=%s", BaseURL, vType, brandId, modelId, yearId, ref),
				fmt.Sprintf("%s/%s/brands/%s/models/%s/years/%s?reference=%s", BaseURL, vType, brandId, modelId, yearId, ref),
				fmt.Sprintf("%s/%s/brands/%s/models/%s/years/%s?month=%s", BaseURL, vType, brandId, modelId, yearId, ref),
				// path variant
				fmt.Sprintf("%s/%s/brands/%s/models/%s/years/%s/history/%s", BaseURL, vType, brandId, modelId, yearId, ref),
				fmt.Sprintf("%s/%s/brands/%s/models/%s/years/%s/historico/%s", BaseURL, vType, brandId, modelId, yearId, ref),
			}

			for _, u := range candidates {
				b, e := fetchExternal(u)
				if e == nil {
					// decode into PriceResponse when possible and set ReferenceMonth explicitly
					var pr PriceResponse
					if err := json.Unmarshal(b, &pr); err == nil {
						if pr.ReferenceMonth == "" {
							parts := strings.Split(ref, "-")
							if len(parts) == 2 {
								pr.ReferenceMonth = parts[1] + "/" + parts[0]
							} else {
								pr.ReferenceMonth = ref
							}
						}
						// ensure price string exists; if empty, skip this candidate
						if pr.Price == "" {
							// try next candidate
							continue
						}
						if nb, err := json.Marshal(pr); err == nil {
							results[offset] = json.RawMessage(nb)
							return
						}
					}
					// if unmarshalling failed, but we have raw bytes, try to set a minimal wrapper
					// attempt to extract numeric price and set a reference
					var raw map[string]interface{}
					if err := json.Unmarshal(b, &raw); err == nil {
						if raw["referenceMonth"] == nil {
							raw["referenceMonth"] = ref
						}
						if _, ok := raw["price"]; !ok {
							// attempt to look for value-like fields
							if v, ok2 := raw["Valor"]; ok2 {
								raw["price"] = v
							}
						}
						if nb, err := json.Marshal(raw); err == nil {
							results[offset] = json.RawMessage(nb)
							return
						}
					}
					// last resort: store raw bytes
					results[offset] = json.RawMessage(b)
					return
				}
			}
		}(i)
	}
	wg.Wait()

	// collect non-empty results preserving month order (current -> past)
	history := make([]json.RawMessage, 0, months)
	for i := 0; i < months; i++ {
		if len(results[i]) > 0 {
			history = append(history, results[i])
		}
	}

	if len(history) == 0 {
		// final fallback: fetch the single current price
		singleURL := fmt.Sprintf("%s/%s/brands/%s/models/%s/years/%s", BaseURL, vType, brandId, modelId, yearId)
		single, err2 := fetchExternal(singleURL)
		if err2 != nil {
			http.Error(w, fmt.Sprintf("history fetch failed: %v, fallback failed: %v", err, err2), http.StatusBadGateway)
			return
		}
		history = append(history, json.RawMessage(single))
	}

	// Normalize entries: ensure each history item has a distinct ReferenceMonth label
	for i := range history {
		var pr PriceResponse
		if err := json.Unmarshal(history[i], &pr); err == nil {
			// compute label for this offset: current month -> offset 0
			ref := time.Now().AddDate(0, -i, 0)
			label := fmt.Sprintf("%02d/%d", int(ref.Month()), ref.Year())
			pr.ReferenceMonth = label
			if nb, err := json.Marshal(pr); err == nil {
				history[i] = json.RawMessage(nb)
			}
		} else {
			// try to add a simple wrapper if raw data doesn't match structure
			var raw map[string]interface{}
			if err := json.Unmarshal(history[i], &raw); err == nil {
				ref := time.Now().AddDate(0, -i, 0)
				raw["referenceMonth"] = fmt.Sprintf("%02d/%d", int(ref.Month()), ref.Year())
				if nb, err := json.Marshal(raw); err == nil {
					history[i] = json.RawMessage(nb)
				}
			}
		}
	}

	resp := map[string]interface{}{"history": history}
	b, _ := json.Marshal(resp)
	w.Header().Set("Content-Type", "application/json")
	w.Write(b)
}

// parsePrice attempts to convert FIPE price strings to float64.
func parsePrice(s string) (float64, error) {
	s = strings.TrimSpace(s)
	re := regexp.MustCompile(`[0-9,.]+`)
	m := re.FindString(s)
	if m == "" {
		return math.NaN(), fmt.Errorf("no numeric part")
	}
	if strings.Contains(m, ".") && strings.Contains(m, ",") {
		m = strings.ReplaceAll(m, ".", "")
		m = strings.ReplaceAll(m, ",", ".")
	} else if strings.Contains(m, ",") && !strings.Contains(m, ".") {
		m = strings.ReplaceAll(m, ",", ".")
	}
	return strconv.ParseFloat(m, 64)
}

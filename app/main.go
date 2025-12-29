package main

import (
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
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
)

func init() {
	prometheus.MustRegister(httpRequestsTotal)
	prometheus.MustRegister(vehicleSearchStats)
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

// --- Main Application ---

func main() {
	tmpl := template.Must(template.ParseFiles("templates/index.html"))

	mux := http.NewServeMux()

	// Frontend
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		recordMetrics(r.URL.Path, r.Method)
		tmpl.Execute(w, nil)
	})

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

	// v2 Endpoint: /{type}/brands
	url := fmt.Sprintf("%s/%s/brands", BaseURL, vType)
	
	data, err := fetchExternal(url)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

// Get Models: /api/models?type=cars&brandId=23
func handleGetModels(w http.ResponseWriter, r *http.Request) {
	recordMetrics("/api/models", r.Method)
	vType := r.URL.Query().Get("type")
	brandId := r.URL.Query().Get("brandId")

	// v2 Endpoint: /{type}/brands/{brandId}/models
	url := fmt.Sprintf("%s/%s/brands/%s/models", BaseURL, vType, brandId)
	
	data, err := fetchExternal(url)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

// Get Years: /api/years?type=cars&brandId=23&modelId=5585
func handleGetYears(w http.ResponseWriter, r *http.Request) {
	recordMetrics("/api/years", r.Method)
	vType := r.URL.Query().Get("type")
	brandId := r.URL.Query().Get("brandId")
	modelId := r.URL.Query().Get("modelId")

	// v2 Endpoint: /{type}/brands/{brandId}/models/{modelId}/years
	url := fmt.Sprintf("%s/%s/brands/%s/models/%s/years", BaseURL, vType, brandId, modelId)
	
	data, err := fetchExternal(url)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

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

	// v2 Endpoint: /{type}/brands/{brandId}/models/{modelId}/years/{yearId}
	url := fmt.Sprintf("%s/%s/brands/%s/models/%s/years/%s", BaseURL, vType, brandId, modelId, yearId)
	
	data, err := fetchExternal(url)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}
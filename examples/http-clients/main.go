// Package main demonstrates the HTTP client collector for go-profiler.
// It creates a server with handlers that make outbound HTTP calls to mock
// downstream services, showcasing how the collector captures timing, headers,
// bodies, and analysis (slow/failed/duplicate detection).
//
// Run with embedded UI:
//
//	go build -tags profiler_ui -o http-clients . && ./http-clients
//
// Run in dev mode:
//
//	go run .
package main

import (
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"time"

	profiler "github.com/piotrkardasz/go-profiler"
	"github.com/piotrkardasz/go-profiler/collector"
	httpcollector "github.com/piotrkardasz/go-profiler/collector/http"
	"github.com/piotrkardasz/go-profiler/handler"
	"github.com/piotrkardasz/go-profiler/storage"
)

// Mock downstream services (started as httptest servers)
var (
	productAPI       *httptest.Server
	inventoryService *httptest.Server
	pricingService   *httptest.Server
	authService      *httptest.Server
)

// Instrumented HTTP clients
var (
	productClient   *http.Client
	inventoryClient *http.Client
	pricingClient   *http.Client
	authClient      *http.Client
)

func main() {
	// Load .env
	loadDotenv(".env")

	// Start mock downstream services
	startMockServices()
	defer productAPI.Close()
	defer inventoryService.Close()
	defer pricingService.Close()
	defer authService.Close()

	// Create storage
	store, err := storage.NewFilesystemStorage("./var/profiler")
	if err != nil {
		log.Fatalf("Failed to create storage: %v", err)
	}

	// Create profiler
	cfg := profiler.DefaultConfig()
	p := profiler.New(cfg, store)

	// Register standard collectors
	p.AddCollector(collector.NewRequestCollector())
	p.AddCollector(collector.NewTimingCollector())
	p.AddCollector(collector.NewMemoryCollector())

	// Register the HTTP client collector
	httpCollector := httpcollector.New(
		httpcollector.WithSlowThreshold(100*time.Millisecond),
		httpcollector.WithBodyCapture(true),
		httpcollector.WithMaxBodySize(4096),
		httpcollector.WithRedactHeaders("Authorization", "X-Api-Key"),
		httpcollector.WithBacktrace(true),
	)
	p.AddCollector(httpCollector)

	// Create instrumented HTTP clients for each downstream service
	productClient = &http.Client{
		Timeout: 5 * time.Second,
		Transport: httpcollector.NewTransport("product-api", http.DefaultTransport,
			httpcollector.WithBodyCapture(true),
		),
	}
	inventoryClient = &http.Client{
		Timeout: 5 * time.Second,
		Transport: httpcollector.NewTransport("inventory-service", http.DefaultTransport,
			httpcollector.WithBodyCapture(true),
		),
	}
	pricingClient = &http.Client{
		Timeout: 5 * time.Second,
		Transport: httpcollector.NewTransport("pricing-service", http.DefaultTransport,
			httpcollector.WithBodyCapture(true),
		),
	}
	authClient = &http.Client{
		Timeout: 5 * time.Second,
		Transport: httpcollector.NewTransport("auth-service", http.DefaultTransport,
			httpcollector.WithBodyCapture(true),
		),
	}

	// Set up HTTP mux
	mux := http.NewServeMux()

	// Register profiler API and UI routes
	apiHandler := handler.NewAPIHandler(p)
	apiHandler.RegisterRoutes(mux, cfg.RoutePrefix)

	uiHandler := handler.NewUIHandler(handler.UIConfig{
		RoutePrefix:  cfg.RoutePrefix,
		DevMode:      cfg.UIDevMode,
		DevServerURL: cfg.UIDevServerURL,
		Assets:       handler.UIDistFS(),
	})
	uiHandler.RegisterRoutes(mux, cfg.RoutePrefix)

	// Application routes
	mux.HandleFunc("/", handleHome)
	mux.HandleFunc("/orders/create", handleCreateOrder)
	mux.HandleFunc("/products/search", handleProductSearch)
	mux.HandleFunc("/health", handleHealth)

	// Wrap with profiler middleware
	srv := &http.Server{
		Addr:    ":8080",
		Handler: p.Middleware(mux),
	}

	fmt.Println("=== Go Profiler - HTTP Client Collector Example ===")
	fmt.Println()
	fmt.Println("Server running at:  http://localhost:8080")
	fmt.Println("Profiler UI at:     http://localhost:8080/_profiler/")
	fmt.Println()
	fmt.Println("Try these endpoints:")
	fmt.Println("  GET  http://localhost:8080/orders/create    (5 outbound calls: auth, product, pricing, inventory, duplicate)")
	fmt.Println("  GET  http://localhost:8080/products/search  (3 outbound calls: fan-out to product API)")
	fmt.Println("  GET  http://localhost:8080/health           (1 outbound call: auth check)")
	fmt.Println()
	fmt.Println("The HTTP Clients panel shows:")
	fmt.Println("  - Per-call timing, status codes, headers, bodies")
	fmt.Println("  - Slow call detection (>100ms threshold)")
	fmt.Println("  - Failed call detection (non-2xx)")
	fmt.Println("  - Duplicate call detection")
	fmt.Println("  - cURL commands for reproduction")
	fmt.Println("  - Backtraces showing call origins")
	fmt.Println()

	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

// handleHome renders a simple HTML page with links to test endpoints.
func handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head><title>Go Profiler - HTTP Client Collector</title></head>
<body>
<h1>Go Profiler - HTTP Client Collector Example</h1>
<p>Check the <a href="/_profiler/">Profiler UI</a> to see collected profiles with HTTP client data.</p>
<h2>Test Endpoints:</h2>
<ul>
<li><a href="/orders/create">/orders/create</a> - Creates an order (calls auth, product, pricing, inventory services)</li>
<li><a href="/products/search">/products/search</a> - Searches products (fan-out to product API, shows duplicates)</li>
<li><a href="/health">/health</a> - Health check (single auth service call)</li>
</ul>
<h2>What to Look For:</h2>
<ul>
<li><strong>HTTP Clients panel</strong> - shows all outbound calls with timing bars</li>
<li><strong>Slow calls</strong> - pricing-service exceeds 100ms threshold (yellow badge)</li>
<li><strong>Failed calls</strong> - auth-service returns 401 (red badge)</li>
<li><strong>Duplicates</strong> - /products/search calls the same URL twice (orange badge)</li>
</ul>
</body>
</html>`)
}

// handleCreateOrder simulates creating an order that calls multiple downstream services.
func handleCreateOrder(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// 1. Check auth (will fail with 401 — simulates token expiry)
	authReq, _ := http.NewRequestWithContext(ctx, "GET", authService.URL+"/verify", nil)
	authReq.Header.Set("Authorization", "Bearer expired-token-123")
	authResp, err := authClient.Do(authReq)
	if err == nil {
		io.ReadAll(authResp.Body)
		authResp.Body.Close()
	}

	// 2. Get product details
	productReq, _ := http.NewRequestWithContext(ctx, "GET", productAPI.URL+"/products/prod-123", nil)
	productReq.Header.Set("Accept", "application/json")
	productResp, err := productClient.Do(productReq)
	if err == nil {
		io.ReadAll(productResp.Body)
		productResp.Body.Close()
	}

	// 3. Get pricing (slow call — exceeds 100ms threshold)
	pricingReq, _ := http.NewRequestWithContext(ctx, "GET", pricingService.URL+"/price?sku=prod-123", nil)
	pricingResp, err := pricingClient.Do(pricingReq)
	if err == nil {
		io.ReadAll(pricingResp.Body)
		pricingResp.Body.Close()
	}

	// 4. Reserve inventory
	reserveBody := `{"sku":"prod-123","quantity":1,"source":"warehouse-eu"}`
	inventoryReq, _ := http.NewRequestWithContext(ctx, "POST", inventoryService.URL+"/reservations", strings.NewReader(reserveBody))
	inventoryReq.Header.Set("Content-Type", "application/json")
	inventoryResp, err := inventoryClient.Do(inventoryReq)
	if err == nil {
		io.ReadAll(inventoryResp.Body)
		inventoryResp.Body.Close()
	}

	// 5. Duplicate: fetch product again (unnecessary repeated call)
	productReq2, _ := http.NewRequestWithContext(ctx, "GET", productAPI.URL+"/products/prod-123", nil)
	productReq2.Header.Set("Accept", "application/json")
	productResp2, err := productClient.Do(productReq2)
	if err == nil {
		io.ReadAll(productResp2.Body)
		productResp2.Body.Close()
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"order_id":"ord-456","status":"created"}`))
}

// handleProductSearch simulates a product search that fans out to multiple
// product API calls, demonstrating duplicate detection.
func handleProductSearch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	skus := []string{"sku-001", "sku-002", "sku-003"}

	for _, sku := range skus {
		req, _ := http.NewRequestWithContext(ctx, "GET", productAPI.URL+"/products/"+sku, nil)
		req.Header.Set("Accept", "application/json")
		resp, err := productClient.Do(req)
		if err == nil {
			io.ReadAll(resp.Body)
			resp.Body.Close()
		}
	}

	// Duplicate: fetch sku-001 again (bug — already fetched above)
	req, _ := http.NewRequestWithContext(ctx, "GET", productAPI.URL+"/products/sku-001", nil)
	req.Header.Set("Accept", "application/json")
	resp, err := productClient.Do(req)
	if err == nil {
		io.ReadAll(resp.Body)
		resp.Body.Close()
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"results":3,"skus":["sku-001","sku-002","sku-003"]}`))
}

// handleHealth makes a single auth service call to verify connectivity.
func handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	req, _ := http.NewRequestWithContext(ctx, "GET", authService.URL+"/health", nil)
	resp, err := authClient.Do(req)

	status := "ok"
	if err != nil || (resp != nil && resp.StatusCode != http.StatusOK) {
		status = "degraded"
	}
	if resp != nil {
		io.ReadAll(resp.Body)
		resp.Body.Close()
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"%s"}`, status)
}

// startMockServices initializes httptest servers simulating downstream services.
func startMockServices() {
	// Product API: returns product data with a slight delay
	productAPI = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(time.Duration(10+rand.Intn(20)) * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"prod-123","name":"Premium Widget","price":29.99,"stock":42}`))
	}))

	// Inventory Service: reserves stock
	inventoryService = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(time.Duration(20+rand.Intn(15)) * time.Millisecond)
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(fmt.Sprintf(`{"reserved":true,"request":%s}`, string(body))))
	}))

	// Pricing Service: intentionally slow (simulates downstream bottleneck)
	pricingService = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(time.Duration(110+rand.Intn(50)) * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"discount":0.15,"final_price":25.49}`))
	}))

	// Auth Service: returns 401 for /verify, 200 for /health
	authService = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"ok"}`))
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"token expired"}`))
	}))
}

// loadDotenv reads a .env file and sets entries as environment variables.
func loadDotenv(path string) {
	reader := collector.NewDotenvReader(path)
	entries, err := reader.Read()
	if err != nil || len(entries) == 0 {
		return
	}
	for _, entry := range entries {
		if os.Getenv(entry.Key) == "" {
			os.Setenv(entry.Key, entry.Value)
		}
	}
}

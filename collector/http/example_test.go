package http_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	httpcollector "github.com/piotrkardasz/go-profiler/collector/http"
)

func Example() {
	// Set up mock downstream services
	productAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"prod-123","name":"Widget","price":9.99}`))
	}))
	defer productAPI.Close()

	inventoryService := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Millisecond) // slight delay
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"reserved":true}`))
	}))
	defer inventoryService.Close()

	// Create the HTTP client collector
	collector := httpcollector.New(
		httpcollector.WithSlowThreshold(100 * time.Millisecond),
		httpcollector.WithBodyCapture(true),
		httpcollector.WithMaxBodySize(4096),
	)

	// Initialize per-request context (normally done by profiler middleware)
	ctx := collector.SetupContext(context.Background())

	// Create instrumented transports for each downstream service
	productTransport := httpcollector.NewTransport("product-api", http.DefaultTransport,
		httpcollector.WithBodyCapture(true),
	)
	inventoryTransport := httpcollector.NewTransport("inventory-service", http.DefaultTransport,
		httpcollector.WithBodyCapture(true),
	)

	// Simulate handler making outbound calls
	productClient := &http.Client{Transport: productTransport}
	inventoryClient := &http.Client{Transport: inventoryTransport}

	// Call product API
	req1, _ := http.NewRequestWithContext(ctx, "GET", productAPI.URL+"/products/123", nil)
	req1.Header.Set("Accept", "application/json")
	resp1, _ := productClient.Do(req1)
	io.ReadAll(resp1.Body)
	resp1.Body.Close()

	// Call inventory service
	req2, _ := http.NewRequestWithContext(ctx, "POST", inventoryService.URL+"/reserve",
		strings.NewReader(`{"sku":"prod-123","qty":1}`))
	req2.Header.Set("Content-Type", "application/json")
	resp2, _ := inventoryClient.Do(req2)
	io.ReadAll(resp2.Body)
	resp2.Body.Close()

	// Retrieve captured calls
	calls := httpcollector.CallsFromContext(ctx)
	fmt.Printf("Captured %d HTTP calls\n", len(calls))
	for _, c := range calls {
		fmt.Printf("[%d] %s %s -> %d\n",
			c.Index, c.Service, c.Method, c.StatusCode)
	}

	// Output:
	// Captured 2 HTTP calls
	// [0] product-api GET -> 200
	// [1] inventory-service POST -> 201
}

func ExampleNewTransport() {
	// Create an instrumented transport for a downstream service
	transport := httpcollector.NewTransport("payment-gateway", http.DefaultTransport,
		httpcollector.WithSlowThreshold(200*time.Millisecond),
		httpcollector.WithHeaderCapture(true),
		httpcollector.WithRedactHeaders("Authorization", "X-Api-Key"),
	)

	// Use it in an http.Client
	client := &http.Client{
		Timeout:   10 * time.Second,
		Transport: transport,
	}

	_ = client // use client in your handler
}

func ExampleNew() {
	// Create the collector with custom options
	collector := httpcollector.New(
		httpcollector.WithSlowThreshold(300*time.Millisecond),
		httpcollector.WithBodyCapture(true),
		httpcollector.WithMaxBodySize(32768),
		httpcollector.WithRedactHeaders("Authorization", "X-Api-Key"),
		httpcollector.WithDuplicateDetection(true),
	)

	_ = collector // register with profiler: p.AddCollector(collector)
}

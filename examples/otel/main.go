// Package main demonstrates the go-profiler with the OpenTelemetry collector.
// It creates spans and metrics during request handling and shows them in the
// profiler UI's OTel panel.
package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"time"

	profiler "github.com/piotrkardasz/go-profiler"
	"github.com/piotrkardasz/go-profiler/collector"
	otelcollector "github.com/piotrkardasz/go-profiler/collector/otel"
	"github.com/piotrkardasz/go-profiler/handler"
	"github.com/piotrkardasz/go-profiler/storage"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func main() {
	// Create storage
	store, err := storage.NewFilesystemStorage("./var/profiler")
	if err != nil {
		log.Fatalf("Failed to create storage: %v", err)
	}

	// Set up OpenTelemetry span capturer
	spanCapturer := otelcollector.NewSpanCapturer()

	// Create TracerProvider with our span capturer
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(spanCapturer),
	)
	defer tp.Shutdown(context.Background())
	otel.SetTracerProvider(tp)

	// Create profiler
	cfg := profiler.DefaultConfig()
	p := profiler.New(cfg, store)

	// Register collectors (including OTel)
	p.AddCollector(collector.NewRequestCollector())
	p.AddCollector(collector.NewTimingCollector())
	p.AddCollector(collector.NewMemoryCollector())
	p.AddCollector(otelcollector.NewCollector(spanCapturer, nil))

	// Set up HTTP mux
	mux := http.NewServeMux()

	// Register profiler routes
	apiHandler := handler.NewAPIHandler(p)
	apiHandler.RegisterRoutes(mux, cfg.RoutePrefix)

	uiHandler := handler.NewUIHandler(handler.UIConfig{
		RoutePrefix:  cfg.RoutePrefix,
		DevMode:      cfg.UIDevMode,
		DevServerURL: cfg.UIDevServerURL,
		Assets:       handler.UIDistFS(),
	})
	uiHandler.RegisterRoutes(mux, cfg.RoutePrefix)

	// Register application routes with tracing
	mux.HandleFunc("/", handleHome)
	mux.HandleFunc("/api/orders", handleOrders)
	mux.HandleFunc("/api/checkout", handleCheckout)

	// Wrap with profiler middleware
	srv := &http.Server{
		Addr:    ":8080",
		Handler: p.Middleware(mux),
	}

	fmt.Println("=== Go Profiler - OpenTelemetry Example ===")
	fmt.Println()
	fmt.Println("Server running at:  http://localhost:8080")
	fmt.Println("Profiler UI at:     http://localhost:8080/_profiler/")
	fmt.Println()
	fmt.Println("Try these endpoints (they create OTel spans):")
	fmt.Println("  GET  http://localhost:8080/api/orders")
	fmt.Println("  POST http://localhost:8080/api/checkout")
	fmt.Println()
	fmt.Println("View the 'OpenTelemetry' tab in the profiler to see spans.")
	fmt.Println()

	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head><title>Go Profiler - OTel Example</title></head>
<body>
<h1>Go Profiler - OpenTelemetry Example</h1>
<p>Check the <a href="/_profiler/">Profiler UI</a> to see collected profiles with OTel data.</p>
<h2>Test Endpoints:</h2>
<ul>
<li><a href="/api/orders">/api/orders</a> - Lists orders (creates DB query spans)</li>
<li><form method="POST" action="/api/checkout" style="display:inline">
    <button type="submit">/api/checkout</button></form> - Simulates checkout (creates multiple spans)</li>
</ul>
</body>
</html>`)
}

func handleOrders(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tracer := otel.Tracer("example-app")

	// Simulate database query
	ctx, dbSpan := tracer.Start(ctx, "db.query",
		oteltrace.WithAttributes(
			attribute.String("db.system", "postgresql"),
			attribute.String("db.statement", "SELECT * FROM orders WHERE user_id = ?"),
		),
	)
	time.Sleep(time.Duration(15+rand.Intn(30)) * time.Millisecond)
	dbSpan.End()

	// Simulate serialization
	_, serSpan := tracer.Start(ctx, "json.marshal",
		oteltrace.WithAttributes(
			attribute.Int("orders.count", 5),
		),
	)
	time.Sleep(time.Duration(2+rand.Intn(5)) * time.Millisecond)
	serSpan.End()

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"orders": [{"id": 1, "total": 99.99}, {"id": 2, "total": 149.50}]}`)
}

func handleCheckout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tracer := otel.Tracer("example-app")

	// Validate cart
	ctx, validateSpan := tracer.Start(ctx, "checkout.validate",
		oteltrace.WithAttributes(
			attribute.Int("cart.items", 3),
		),
	)
	time.Sleep(time.Duration(5+rand.Intn(10)) * time.Millisecond)
	validateSpan.End()

	// Process payment
	ctx, paymentSpan := tracer.Start(ctx, "checkout.payment",
		oteltrace.WithAttributes(
			attribute.String("payment.provider", "stripe"),
			attribute.Float64("payment.amount", 249.49),
		),
	)
	time.Sleep(time.Duration(50+rand.Intn(100)) * time.Millisecond)
	paymentSpan.End()

	// Update inventory
	_, inventorySpan := tracer.Start(ctx, "checkout.inventory",
		oteltrace.WithAttributes(
			attribute.String("db.system", "postgresql"),
			attribute.String("db.operation", "UPDATE"),
		),
	)
	time.Sleep(time.Duration(10+rand.Intn(20)) * time.Millisecond)
	inventorySpan.End()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	fmt.Fprint(w, `{"order_id": "ord_abc123", "status": "confirmed"}`)
}

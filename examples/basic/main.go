// Package main demonstrates basic usage of the go-profiler package.
// It sets up an HTTP server with the profiler middleware, built-in collectors,
// and the profiler UI/API routes.
//
// Run with embedded UI:
//
//	go build -tags profiler_ui -o basic . && ./basic
//
// Run in dev mode (proxies to Vite dev server, no UI build needed):
//
//	GO_PROFILER_UI_DEV=true go run .
package main

import (
	"fmt"
	"log"
	"log/slog"
	"math/rand"
	"net/http"
	"time"

	profiler "github.com/piotrkardasz/go-profiler"
	"github.com/piotrkardasz/go-profiler/collector"
	"github.com/piotrkardasz/go-profiler/handler"
	"github.com/piotrkardasz/go-profiler/storage"
)

func main() {
	// Create storage (file-based, default directory)
	store, err := storage.NewFilesystemStorage("./var/profiler")
	if err != nil {
		log.Fatalf("Failed to create storage: %v", err)
	}

	// Create profiler with default config
	cfg := profiler.DefaultConfig()
	p := profiler.New(cfg, store)

	// Register built-in collectors
	p.AddCollector(collector.NewRequestCollector())
	p.AddCollector(collector.NewTimingCollector())
	p.AddCollector(collector.NewMemoryCollector())
	p.AddCollector(collector.NewConfigCollector(
		// Only show env vars with APP_ or DB_ prefix (optional, remove for all vars)
		// collector.WithEnvPrefix("APP_", "DB_"),
	))

	// Register logger collector — captures slog and log output per request
	loggerCollector := collector.NewLoggerCollector(
		// collector.WithMinLevel(collector.LevelInfo),  // Only capture INFO and above
		// collector.WithMaxEntries(500),                // Limit entries per request
	)
	defer loggerCollector.Close() // Flush pending logs and restore original loggers on shutdown
	p.AddCollector(loggerCollector)

	// Set up HTTP mux
	mux := http.NewServeMux()

	// Register profiler API and UI routes
	apiHandler := handler.NewAPIHandler(p)
	apiHandler.RegisterRoutes(mux, cfg.RoutePrefix)

	uiHandler := handler.NewUIHandler(handler.UIConfig{
		RoutePrefix: cfg.RoutePrefix,
		DevMode:     cfg.UIDevMode,
		DevServerURL: cfg.UIDevServerURL,
		Assets:      handler.UIDistFS(),
	})
	uiHandler.RegisterRoutes(mux, cfg.RoutePrefix)

	// Register application routes
	mux.HandleFunc("/", handleHome)
	mux.HandleFunc("/api/users", handleUsers)
	mux.HandleFunc("/api/slow", handleSlow)
	mux.HandleFunc("/api/error", handleError)

	// Wrap with profiler middleware
	srv := &http.Server{
		Addr:    ":8080",
		Handler: p.Middleware(mux),
	}

	fmt.Println("=== Go Profiler - Basic Example ===")
	fmt.Println()
	fmt.Println("Server running at:  http://localhost:8080")
	fmt.Println("Profiler UI at:     http://localhost:8080/_profiler/")
	fmt.Println("Profiler API at:    http://localhost:8080/_profiler/api/profiles")
	fmt.Println()
	fmt.Println("Try these endpoints:")
	fmt.Println("  GET  http://localhost:8080/")
	fmt.Println("  GET  http://localhost:8080/api/users")
	fmt.Println("  GET  http://localhost:8080/api/slow")
	fmt.Println("  GET  http://localhost:8080/api/error")
	fmt.Println()
	fmt.Println("Each response includes an X-Profiler-Id header.")
	fmt.Println("Use it to view the profile at: http://localhost:8080/_profiler/profile/{id}")
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
<head><title>Go Profiler Example</title></head>
<body>
<h1>Go Profiler - Basic Example</h1>
<p>Check the <a href="/_profiler/">Profiler UI</a> to see collected profiles.</p>
<h2>Test Endpoints:</h2>
<ul>
<li><a href="/api/users">/api/users</a> - Returns mock user data</li>
<li><a href="/api/slow">/api/slow</a> - Simulates a slow request</li>
<li><a href="/api/error">/api/error</a> - Returns a 500 error</li>
</ul>
</body>
</html>`)
}

func handleUsers(w http.ResponseWriter, r *http.Request) {
	slog.InfoContext(r.Context(), "handling users request", "method", r.Method)

	// Simulate some work
	time.Sleep(time.Duration(10+rand.Intn(40)) * time.Millisecond)

	slog.InfoContext(r.Context(), "returning user list", "count", 2)
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"users": [{"id": 1, "name": "Alice"}, {"id": 2, "name": "Bob"}]}`)
}

func handleSlow(w http.ResponseWriter, r *http.Request) {
	slog.WarnContext(r.Context(), "starting slow operation", "threshold_ms", 200)

	// Simulate a slow operation
	time.Sleep(time.Duration(200+rand.Intn(300)) * time.Millisecond)

	slog.InfoContext(r.Context(), "slow operation completed")
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"message": "This was a slow request", "delay_ms": 200}`)
}

func handleError(w http.ResponseWriter, r *http.Request) {
	slog.ErrorContext(r.Context(), "something went wrong", "endpoint", "/api/error")
	time.Sleep(5 * time.Millisecond)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	fmt.Fprint(w, `{"error": "Something went wrong"}`)
}

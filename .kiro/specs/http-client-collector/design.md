# Design: HTTP Client Collector

## Overview

Add a new collector module (`collector/http`) to `go-profiler` that captures outbound HTTP calls made during a profiled request. This provides visibility into service-to-service communication — timing, status codes, request/response details, and analysis (slow calls, failures, duplicates) — displayed in the profiler UI alongside existing panels.

## Motivation

The profiler currently captures inbound request details, database queries (GORM), OpenTelemetry spans, logs, timing, and memory. However, outbound HTTP calls to downstream services are invisible unless you correlate OTel spans manually.

For applications that orchestrate multiple HTTP service calls per request (product resolvers, inventory services, source resolvers), a dedicated HTTP client collector provides:

- Per-call timing breakdown showing where latency comes from
- Request/response inspection for debugging integration issues
- Slow call detection highlighting downstream bottlenecks
- Failed call tracking surfacing transient errors
- Duplicate call detection finding unnecessary repeated requests
- cURL command generation for easy reproduction

## Design Decisions

### Integration Mechanism: `http.RoundTripper`

The collector uses a wrapping `http.RoundTripper` to intercept outbound calls transparently.

**Why RoundTripper:**
- Idiomatic Go — the standard instrumentation point for HTTP clients (used by OTel, retry libraries, circuit breakers)
- Transparent to application code — no changes to existing service clients needed
- Context flows naturally — `http.NewRequestWithContext` passes the profiler's per-request context into the transport
- Composable — stacks with existing transports (retries, tracing)

**Why not middleware or wrapper functions:**
- A handler-level middleware can't see outbound calls, only inbound
- Wrapping individual client methods requires invasive changes to every service client
- RoundTripper sits at the lowest common point where all `http.Client.Do()` calls pass through

### Per-Request Tracking: Context-Based (same as GORM collector)

Follows the established pattern from `collector/gorm`:
1. Collector implements `collector.ContextSetup` to initialize a per-request tracker in context
2. The `RoundTripper` reads the tracker from `req.Context()` and appends call entries
3. `Collect()` reads accumulated entries from context at profile time

This ensures no cross-request contamination when multiple requests are in flight.

### Separate Module

Published as `github.com/piotrkardasz/go-profiler/collector/http` — a separate Go module (like `collector/gorm`) with its own `go.mod`. This avoids pulling HTTP-specific dependencies into the core profiler package.

## Requirements

### Functional

1. **Capture outbound HTTP calls** — method, URL, status code, duration, request/response size
2. **Per-service naming** — each transport instance is tagged with a service name (e.g., "product-api", "inventory-service") for grouping in the UI
3. **Optional header capture** — configurable, with header redaction for sensitive values (Authorization, Cookie, etc.)
4. **Optional body capture** — request and response bodies with configurable max size limit
5. **Error capture** — transport errors (timeouts, DNS failures, connection refused) with error message
6. **cURL command generation** — for each captured call, produce a copy-paste cURL command
7. **Backtrace capture** — optional call stack showing where the HTTP call originated
8. **Analysis: slow calls** — flag calls exceeding a configurable threshold
9. **Analysis: failed calls** — group calls with non-2xx status or transport errors
10. **Analysis: duplicate calls** — detect repeated identical requests (same method + URL + body hash)
11. **Summary statistics** — total calls, total duration, calls per service, failure count, slowest call
12. **No-op when not profiling** — if context has no tracker (profiler disabled or non-profiled request), the transport passes through with zero overhead beyond a nil check
13. **Thread-safe** — concurrent calls from the same request (e.g., fan-out) must safely append to the shared tracker

### Non-Functional

1. **Overhead when profiling** — less than 50µs per call (excluding body capture I/O)
2. **Overhead when not profiling** — single context value lookup (~10ns)
3. **No response modification** — the transport must not alter the response body or close it prematurely; body capture should use `io.TeeReader` to buffer without consuming
4. **Composable** — must work when stacked with other transports (otelhttp, retryable, etc.)

## Module Structure

```
collector/http/
├── go.mod              # module github.com/piotrkardasz/go-profiler/collector/http
├── go.sum
├── collector.go        # Collector: Name, Collect, Reset, SetupContext, PanelMeta
├── transport.go        # ProfilingRoundTripper implementation
├── entry.go            # HTTPCallEntry, HTTPData, Summary data types
├── context.go          # Context helpers: WithContext, CallsFromContext, callsFromContext
├── options.go          # Option funcs: WithSlowThreshold, WithBodyCapture, WithRedactHeaders, etc.
├── analysis.go         # Slow/failed/duplicate detection
├── curl.go             # cURL command generation
├── collector_test.go   # Unit tests for collector
├── transport_test.go   # Unit tests for round tripper
├── analysis_test.go    # Unit tests for analysis
└── example_test.go     # Runnable examples
```

## Data Types

```go
// HTTPCallEntry represents a single captured outbound HTTP call.
type HTTPCallEntry struct {
    Index           int                 `json:"index"`
    Service         string              `json:"service"`
    Method          string              `json:"method"`
    URL             string              `json:"url"`
    RequestHeaders  map[string][]string `json:"request_headers,omitempty"`
    RequestBody     string              `json:"request_body,omitempty"`
    RequestSize     int64               `json:"request_size"`
    StatusCode      int                 `json:"status_code"`
    ResponseHeaders map[string][]string `json:"response_headers,omitempty"`
    ResponseBody    string              `json:"response_body,omitempty"`
    ResponseSize    int64               `json:"response_size"`
    DurationMs      float64             `json:"duration_ms"`
    Error           string              `json:"error,omitempty"`
    Timestamp       time.Time           `json:"timestamp"`
    Backtrace       []string            `json:"backtrace,omitempty"`
    CurlCommand     string              `json:"curl_command,omitempty"`
}

// AnalysisResult holds analysis output.
type AnalysisResult struct {
    SlowCalls      []HTTPCallEntry  `json:"slow_calls,omitempty"`
    FailedCalls    []HTTPCallEntry  `json:"failed_calls,omitempty"`
    DuplicateCalls []DuplicateGroup `json:"duplicate_calls,omitempty"`
}

// DuplicateGroup represents repeated identical calls.
type DuplicateGroup struct {
    Method  string `json:"method"`
    URL     string `json:"url"`
    Count   int    `json:"count"`
    Indices []int  `json:"indices"`
}

// Summary holds aggregate statistics.
type Summary struct {
    TotalCalls      int            `json:"total_calls"`
    TotalDurationMs float64        `json:"total_duration_ms"`
    CallsPerService map[string]int `json:"calls_per_service"`
    FailedCount     int            `json:"failed_count"`
    SlowCount       int            `json:"slow_count"`
    DuplicateCount  int            `json:"duplicate_count"`
    SlowestCall     *HTTPCallEntry `json:"slowest_call,omitempty"`
}

// HTTPData is the top-level structure stored in Profile.CollectorData["http"].
type HTTPData struct {
    Calls    []HTTPCallEntry `json:"calls"`
    Analysis AnalysisResult  `json:"analysis"`
    Summary  Summary         `json:"summary"`
}
```

## Public API

### Collector

```go
// New creates a new HTTP client collector with the given options.
func New(options ...Option) *Collector

// Collector implements collector.Collector and collector.ContextSetup.
type Collector struct{ ... }

func (c *Collector) Name() string                                                              // returns "http"
func (c *Collector) Collect(ctx context.Context, req *http.Request, res collector.ResponseData) (any, error)
func (c *Collector) Reset()
func (c *Collector) SetupContext(ctx context.Context) context.Context
func (c *Collector) PanelMeta() collector.PanelMeta
```

### Transport

```go
// NewTransport creates a profiling round tripper wrapping the given base transport.
// serviceName identifies the downstream service for grouping in the profiler UI.
func NewTransport(serviceName string, base http.RoundTripper, options ...Option) http.RoundTripper
```

### Options

```go
func WithSlowThreshold(d time.Duration) Option       // default: 500ms
func WithBodyCapture(enabled bool) Option             // default: false
func WithMaxBodySize(n int) Option                    // default: 64KB
func WithHeaderCapture(enabled bool) Option           // default: true
func WithRedactHeaders(headers ...string) Option      // default: Authorization, Cookie, Set-Cookie
func WithBacktrace(enabled bool) Option               // env: HTTP_PROFILER_BACKTRACE
func WithDuplicateDetection(enabled bool) Option      // default: true
func WithCurlGeneration(enabled bool) Option          // default: true
```

### Context Helpers

```go
// WithContext initializes HTTP call tracking in the context.
// Called automatically by SetupContext; exposed for manual use in tests.
func WithContext(ctx context.Context) context.Context

// CallsFromContext retrieves all captured HTTP calls from the context.
// Returns nil if no tracking is active.
func CallsFromContext(ctx context.Context) []HTTPCallEntry
```

## Integration Example

```go
package main

import (
    "net/http"
    "time"

    profiler "github.com/piotrkardasz/go-profiler"
    httpcollector "github.com/piotrkardasz/go-profiler/collector/http"
    "github.com/piotrkardasz/go-profiler/storage"
)

func main() {
    store, _ := storage.NewFilesystemStorage("./var/profiler")
    p := profiler.New(profiler.DefaultConfig(), store)

    // Register the HTTP client collector
    httpCollector := httpcollector.New(
        httpcollector.WithSlowThreshold(300 * time.Millisecond),
        httpcollector.WithBodyCapture(true),
        httpcollector.WithRedactHeaders("Authorization", "X-Api-Key"),
    )
    p.AddCollector(httpCollector)

    // Create instrumented HTTP clients for downstream services
    productClient := &http.Client{
        Timeout:   10 * time.Second,
        Transport: httpcollector.NewTransport("product-api", http.DefaultTransport),
    }
    inventoryClient := &http.Client{
        Timeout:   5 * time.Second,
        Transport: httpcollector.NewTransport("inventory-service", http.DefaultTransport),
    }

    // Use clients in your handlers — calls are captured automatically
    // as long as the request context is passed through.
    mux := http.NewServeMux()
    mux.HandleFunc("/orders", func(w http.ResponseWriter, r *http.Request) {
        // Context from the profiler middleware flows into these calls
        req, _ := http.NewRequestWithContext(r.Context(), "GET", "http://product-api/items/123", nil)
        productClient.Do(req)

        req2, _ := http.NewRequestWithContext(r.Context(), "POST", "http://inventory/reserve", nil)
        inventoryClient.Do(req2)
    })

    http.ListenAndServe(":8080", p.Middleware(mux))
}
```

## Profiler Output Example

```json
{
  "http": {
    "calls": [
      {
        "index": 0,
        "service": "product-api",
        "method": "GET",
        "url": "http://product-api:8080/shop/product-variants/sku-12345",
        "request_headers": {
          "Content-Type": ["application/json"],
          "Accept": ["application/json"]
        },
        "status_code": 200,
        "response_headers": {
          "Content-Type": ["application/json"]
        },
        "response_size": 342,
        "duration_ms": 12.45,
        "timestamp": "2026-08-07T11:57:15.845Z",
        "curl_command": "curl -X GET 'http://product-api:8080/shop/product-variants/sku-12345' -H 'Content-Type: application/json' -H 'Accept: application/json'"
      },
      {
        "index": 1,
        "service": "inventory-service",
        "method": "POST",
        "url": "http://inventory:8080/pre-reservations",
        "request_headers": {
          "Content-Type": ["application/json"]
        },
        "request_body": "{\"sku\":\"sku-12345\",\"source\":\"warehouse-eu\"}",
        "request_size": 42,
        "status_code": 201,
        "response_size": 128,
        "duration_ms": 45.32,
        "timestamp": "2026-08-07T11:57:15.858Z",
        "curl_command": "curl -X POST 'http://inventory:8080/pre-reservations' -H 'Content-Type: application/json' -d '{\"sku\":\"sku-12345\",\"source\":\"warehouse-eu\"}'"
      }
    ],
    "analysis": {
      "slow_calls": [],
      "failed_calls": [],
      "duplicate_calls": []
    },
    "summary": {
      "total_calls": 2,
      "total_duration_ms": 57.77,
      "calls_per_service": {
        "product-api": 1,
        "inventory-service": 1
      },
      "failed_count": 0,
      "slow_count": 0,
      "duplicate_count": 0,
      "slowest_call": {
        "index": 1,
        "service": "inventory-service",
        "method": "POST",
        "url": "http://inventory:8080/pre-reservations",
        "duration_ms": 45.32
      }
    }
  }
}
```

## UI Panel

The collector registers with panel metadata:

```go
PanelMeta{
    Name:      "http",
    Label:     "HTTP Clients",
    Icon:      "world",
    Component: "HttpPanel",
}
```

### Panel Features (Vue component)

1. **Call list** — table with service, method, URL, status badge, duration bar
2. **Timeline visualization** — waterfall showing call start/end relative to request start
3. **Call detail view** — expandable row showing headers, bodies, cURL command, backtrace
4. **Service grouping** — toggle to group calls by service name
5. **Analysis badges** — warning indicators for slow, failed, or duplicate calls
6. **Filter/search** — filter by service, method, status range

Without a custom Vue component, the generic JSON panel renders the data as an interactive tree (zero friction, works immediately).

## Implementation Tasks

1. **Create module skeleton** — `collector/http/go.mod`, import `go-profiler/collector`
2. **Implement context tracking** — `context.go` with `WithContext`, `callsFromContext`, `CallsFromContext`
3. **Implement data types** — `entry.go` with all JSON-serializable structs
4. **Implement options** — `options.go` with functional option pattern
5. **Implement ProfilingRoundTripper** — `transport.go`, core interception logic
6. **Implement body capture** — `io.TeeReader` approach, respecting `MaxBodySize`
7. **Implement cURL generation** — `curl.go`, reconstruct cURL from captured request data
8. **Implement analysis** — `analysis.go` with slow/failed/duplicate detection
9. **Implement collector** — `collector.go`, wire together context + analysis + panel meta
10. **Write tests** — unit tests for transport, collector, analysis, context, options
11. **Write example** — `example_test.go` with runnable integration example
12. **Add to README** — document in main go-profiler README under a new section
13. **(Optional) Build HttpPanel Vue component** — custom UI with waterfall timeline

## Edge Cases

- **No context tracker** (profiler disabled) — RoundTripper passes through, zero overhead
- **Concurrent calls** (fan-out/fan-in) — `sync.Mutex` on the tracker struct (same as GORM collector)
- **Response body already consumed** — body capture uses `io.TeeReader` to buffer without consuming the original; caller still reads the full body
- **Large response bodies** — truncated at `MaxBodySize` with `[truncated]` indicator
- **Transport errors** (no response) — entry records the error string, status code stays 0
- **Redirect chains** — each hop is captured as a separate entry (standard `http.Transport` behavior with `CheckRedirect`)
- **Retry middleware stacked above** — each retry attempt is a separate `RoundTrip` call, each gets captured individually (correct behavior — shows retry count)

## Compatibility

- Go 1.21+ (same as core profiler)
- No external dependencies beyond `go-profiler/collector` interface package
- Works with any `http.RoundTripper`-compatible stack (otelhttp, retryablehttp, etc.)
- Stacks transparently: `httpcollector.NewTransport("svc", otelhttp.NewTransport(http.DefaultTransport))`

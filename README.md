# Go Profiler

A framework-agnostic HTTP profiling middleware for Go, inspired by [Symfony's Profiler](https://symfony.com/doc/current/profiler.html). It collects per-request profiling data via pluggable collectors, stores profiles as JSON files, and exposes them through a JSON API and embedded Vue.js web UI.

## Features

- **Framework-agnostic** — Works with any `http.Handler`-compatible router or framework
- **Zero-overhead profiling** — All collection runs asynchronously off the request hot path (~5µs overhead per request)
- **Pluggable collectors** — Ships with Request, Timing, Memory, Config, and Logger collectors; easily extensible
- **OpenTelemetry integration** — Captures traces and metrics per request
- **GORM integration** — Captures SQL queries with N+1 detection, duplicate detection, and slow query analysis
- **HTTP client integration** — Captures outbound HTTP calls with timing, slow/failed/duplicate detection, and cURL generation
- **Pluggable storage** — File-based JSON storage by default, with an in-memory option for testing
- **`X-Profiler-Id` header** — Every profiled response includes a unique profile token
- **JSON API** — Query, filter, and retrieve profiles programmatically
- **Embedded Vue.js UI** — Browse profiles at `/_profiler/` with zero external dependencies
- **Extensible UI panels** — Register custom Vue components for custom collector visualization
- **Dual UI mode** — Embedded assets for production; proxies to Vite dev server for UI development
- **Probabilistic sampling** — Configure `SampleRate` to profile a fraction of requests in production
- **Graceful shutdown** — `Shutdown(ctx)` waits for in-flight profiles to be persisted
- **Enable/disable** — Controlled via `GO_PROFILER_ENABLED` env var or config

## Installation

```bash
go get github.com/piotrkardasz/go-profiler
```

Requires Go 1.21+.

### Building with Embedded UI

The profiler UI assets are pre-built and committed to the repository. To embed
them into your binary, use the `profiler_ui` build tag:

```bash
go build -tags profiler_ui ./cmd/myapp
```

Without the tag, `UIDistFS()` returns nil and the UI handler responds with 404.
This is useful for development mode where you proxy to the Vite dev server instead.

### Development Mode (no build tag needed)

```bash
GO_PROFILER_UI_DEV=true go run ./cmd/myapp
```

## Quick Start

```go
package main

import (
    "fmt"
    "net/http"

    profiler "github.com/piotrkardasz/go-profiler"
    "github.com/piotrkardasz/go-profiler/collector"
    "github.com/piotrkardasz/go-profiler/handler"
    "github.com/piotrkardasz/go-profiler/storage"
)

func main() {
    // Create file-based storage
    store, _ := storage.NewFilesystemStorage("./var/profiler")

    // Create profiler with default config
    cfg := profiler.DefaultConfig()
    p := profiler.New(cfg, store)

    // Register built-in collectors
    p.AddCollector(collector.NewRequestCollector())
    p.AddCollector(collector.NewTimingCollector())
    p.AddCollector(collector.NewMemoryCollector())
    p.AddCollector(collector.NewLoggerCollector())

    // Set up routes
    mux := http.NewServeMux()

    // Register profiler API and UI
    handler.NewAPIHandler(p).RegisterRoutes(mux, cfg.RoutePrefix)
    handler.NewUIHandler(handler.UIConfig{
        RoutePrefix: cfg.RoutePrefix,
        Assets:      handler.UIDistFS(),
    }).RegisterRoutes(mux, cfg.RoutePrefix)

    // Your application routes
    mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        fmt.Fprint(w, "Hello, World!")
    })

    // Wrap with profiler middleware and serve
    http.ListenAndServe(":8080", p.Middleware(mux))
}
```

After running, make requests to your server and browse profiles at `http://localhost:8080/_profiler/`.

## Configuration

```go
cfg := profiler.DefaultConfig()
```

| Field | Default | Env Var | Description |
|-------|---------|---------|-------------|
| `Enabled` | `true` | `GO_PROFILER_ENABLED` | Enable/disable profiling |
| `StoragePath` | `./var/profiler` | — | Directory for file-based storage |
| `RoutePrefix` | `/_profiler` | — | URL prefix for profiler routes |
| `SampleRate` | `1.0` | — | Fraction of requests to profile (0.0–1.0) |
| `UIDevMode` | `false` | `GO_PROFILER_UI_DEV` | Proxy UI to Vite dev server |
| `UIDevServerURL` | `http://localhost:5173` | — | Vite dev server URL |

### Sampling

For production or high-traffic environments, reduce profiling overhead by sampling:

```go
cfg := profiler.DefaultConfig()
cfg.SampleRate = 0.1 // Profile ~10% of requests
p := profiler.New(cfg, store)
```

Skipped requests have near-zero overhead (a single float comparison). No `X-Profiler-Id` header is set on skipped requests.

### Graceful Shutdown

The profiler runs collection asynchronously. Call `Shutdown` before your application exits to ensure all in-flight profiles are persisted:

```go
// On application shutdown:
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

if err := p.Shutdown(ctx); err != nil {
    log.Printf("profiler shutdown: %v", err)
}
```

After `Shutdown` is called, new requests still execute normally but profiling is skipped.

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│  HTTP Request                                            │
├─────────────────────────────────────────────────────────┤
│  Profiler Middleware (synchronous — ~5µs overhead)       │
│  ┌─────────────────┐  ┌──────────────────────────────┐ │
│  │ Sampling check   │  │ Set X-Profiler-Id header     │ │
│  │ Generate ID      │  │ Wrap ResponseWriter          │ │
│  │ Memory snapshot  │  │ Capture timing + response    │ │
│  └─────────────────┘  └──────────────────────────────┘ │
├─────────────────────────────────────────────────────────┤
│  Your Handler                                            │
├─────────────────────────────────────────────────────────┤
│  Response returned to client                             │
├─────────────────────────────────────────────────────────┤
│  Async goroutine (does not block response)              │
│  ┌─────────┐ ┌────────┐ ┌────────┐ ┌──────┐          │
│  │ Request │ │ Timing │ │ Memory │ │ OTel │ ...       │
│  └─────────┘ └────────┘ └────────┘ └──────┘          │
│  ┌──────────────┐    ┌───────────────┐  ┌───────────┐ │
│  │ Storage      │    │ JSON API      │  │ Vue UI    │ │
│  │ (Filesystem) │    │ /api/profiles │  │ /_profiler/│ │
│  └──────────────┘    └───────────────┘  └───────────┘ │
└─────────────────────────────────────────────────────────┘
```

### Performance

All collector work (memory stats, request data, config, GORM analysis) runs **asynchronously** in a goroutine after the response is sent. The synchronous overhead added to each request is minimal:

| Scenario | Overhead per request |
|----------|---------------------|
| Profiler enabled | ~5µs |
| Profiler disabled | ~300ns |
| SampleRate < 1.0 (skipped) | ~300ns |

The memory collector uses `runtime/metrics` (no stop-the-world pauses), and the config collector caches its data at startup (no per-request file I/O).

## Creating Custom Collectors

Implement the `collector.Collector` interface:

```go
package mycollector

import (
    "context"
    "net/http"

    "github.com/piotrkardasz/go-profiler/collector"
)

type DBCollector struct {
    // your fields
}

func (c *DBCollector) Name() string { return "database" }

func (c *DBCollector) Collect(ctx context.Context, req *http.Request, res collector.ResponseData) (any, error) {
    // Return any JSON-serializable data
    return map[string]any{
        "queries":    getQueriesFromContext(ctx),
        "total_time": getTotalDBTime(ctx),
    }, nil
}

func (c *DBCollector) Reset() {
    // Clear state between requests
}

// Optional: implement collector.PanelProvider for custom UI metadata
func (c *DBCollector) PanelMeta() collector.PanelMeta {
    return collector.PanelMeta{
        Name:      "database",
        Label:     "Database",
        Icon:      "database",
        Component: "DatabasePanel", // Custom Vue component name
    }
}
```

For collectors that need post-response data (e.g., async operations), implement `collector.LateCollector`:

```go
func (c *DBCollector) LateCollect(ctx context.Context) (any, error) {
    // Called after the response is sent
    return additionalData, nil
}
```

## Creating Custom UI Panels

The profiler UI uses a plugin registry with a `GenericPanel` fallback. Custom
collectors that return JSON data automatically render as an interactive JSON tree
— no UI rebuild required.

For richer visualization (charts, tables, etc.), you can write a custom Vue
component and rebuild the UI with the `profiler-setup` tool:

### Step 1: Copy the UI source

```bash
cp -r $(go list -m -json github.com/piotrkardasz/go-profiler | jq -r .Dir)/ui ./profiler-ui
```

### Step 2: Create your panel component

```vue
<!-- profiler-ui/src/components/panels/DatabasePanel.vue -->
<script setup lang="ts">
defineProps<{
  data: unknown
  collectorName: string
}>()
</script>

<template>
  <div class="database-panel">
    <!-- Your custom rendering -->
  </div>
</template>
```

### Step 3: Register the panel

```typescript
// In profiler-ui/src/plugin/builtin.ts, add:
import DatabasePanel from '@/components/panels/DatabasePanel.vue'

registerPanel('database', DatabasePanel)
```

### Step 4: Build and embed

```bash
# Using Go 1.24+ tool directive:
go get -tool github.com/piotrkardasz/go-profiler/cmd/profiler-setup
go tool profiler-setup --ui-source=./profiler-ui --output=./vendor/github.com/piotrkardasz/go-profiler/handler/ui_dist

# Or using go run:
go run github.com/piotrkardasz/go-profiler/cmd/profiler-setup --ui-source=./profiler-ui --output=./handler/ui_dist

# Then build with the tag:
go build -tags profiler_ui ./cmd/myapp
```

Collectors without a registered panel automatically use the generic JSON tree view.

## OpenTelemetry Integration

```go
import (
    otelcollector "github.com/piotrkardasz/go-profiler/collector/otel"
    sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Create span capturer and register with TracerProvider
spanCapturer := otelcollector.NewSpanCapturer()
tp := sdktrace.NewTracerProvider(
    sdktrace.WithSpanProcessor(spanCapturer),
)

// Optionally capture metrics too
metricCapturer := otelcollector.NewMetricCapturer(nil)

// Add to profiler
p.AddCollector(otelcollector.NewCollector(spanCapturer, metricCapturer))
```

The OTel panel shows a waterfall trace timeline and metrics table.

## Storage

### File-based (default)

```go
store, err := storage.NewFilesystemStorage("./var/profiler")
```

Stores one JSON file per profile. Supports filtering, pagination, and time-based purging.

### In-memory (for testing)

```go
store := storage.NewMemoryStorage(200) // max 200 entries with LRU eviction
```

### Custom storage

Implement the `profiler.Storage` interface:

```go
type Storage interface {
    Store(profile *Profile) error
    Load(id string) (*Profile, error)
    List(criteria SearchCriteria) ([]*ProfileSummary, error)
    Purge(maxAge time.Duration) (int, error)
}
```

## JSON API

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/_profiler/api/profiles` | GET | List profiles (supports query filters) |
| `/_profiler/api/profiles/{id}` | GET | Get full profile by ID |
| `/_profiler/api/profiles` | DELETE | Purge old profiles |
| `/_profiler/api/collectors` | GET | List registered collectors |

### Query Parameters for listing

- `method` — Filter by HTTP method
- `url` — Filter by URL substring
- `status` — Filter by exact status code
- `min_status`, `max_status` — Filter by status range
- `since`, `until` — Filter by time range (RFC3339)
- `limit`, `offset` — Pagination

## UI Development

To develop the Vue UI with hot reload:

```bash
# Terminal 1: Start Go server with dev mode
GO_PROFILER_UI_DEV=true go run ./examples/basic/

# Terminal 2: Start Vite dev server
make ui-dev
```

## Logger Collector

The logger collector captures log entries produced during each HTTP request. It ships with built-in adapters for Go's `log/slog` and standard `log` packages and supports custom adapters for third-party libraries.

```go
import "github.com/piotrkardasz/go-profiler/collector"

p.AddCollector(collector.NewLoggerCollector())
```

Options:

```go
p.AddCollector(collector.NewLoggerCollector(
    collector.WithMinLevel(collector.LevelInfo),  // Capture INFO and above
    collector.WithMaxEntries(500),                // Cap entries per request
    collector.WithBacktrace(true),                // Capture call stacks
))
```

### Logger Backtrace

When backtrace is enabled, each log entry includes the call stack showing where the log call originated. This helps you trace which handler or function produced a specific log message.

Enable via environment variable (no code change):

```bash
PROFILER_LOGGER_BACKTRACE=1 go run .
```

Or programmatically:

```go
p.AddCollector(collector.NewLoggerCollector(
    collector.WithBacktrace(true),
))
```

**Precedence:**
1. Explicit `WithBacktrace(bool)` — highest priority
2. `PROFILER_LOGGER_BACKTRACE` env variable (set to `"true"` or `"1"` to enable)
3. Default: disabled

When enabled, the Logs panel in the UI shows a collapsible stack trace for each entry:

```
/app/handlers/users.go:45 main.handleUsers
/app/routes.go:32 main.setupRoutes.func1
```

Internal frames (runtime, slog internals, profiler machinery) are automatically filtered out.

For custom `LogAdapter` implementations, call the exported `collector.CaptureLogBacktrace()` helper to populate the `LogEntry.Stack` field.

## GORM Collector

The GORM collector captures per-request SQL queries, detects N+1 patterns, slow queries, and duplicates.

```go
import (
    gormcollector "github.com/piotrkardasz/go-profiler/collector/gorm"
)

// Create the GORM collector with options
gc := gormcollector.NewCollector(
    gormcollector.WithConnection("primary", db),
    gormcollector.WithSlowThreshold(200 * time.Millisecond),
    gormcollector.WithN1Threshold(3),
)

p.AddCollector(gc)
```

### Backtrace Collection

The GORM collector can capture Go call stack traces for each query. This is opt-in because `runtime.Callers()` has measurable overhead.

Enable it via the `GORM_PROFILER_BACKTRACE` environment variable:

```bash
GORM_PROFILER_BACKTRACE=1 go run .
```

Or programmatically:

```go
gc := gormcollector.NewCollector(
    gormcollector.WithConnection("primary", db),
    gormcollector.WithBacktrace(true),
)
```

**Precedence:**
1. Explicit `WithBacktrace(bool)` — highest priority
2. `GORM_PROFILER_BACKTRACE` env variable (set to `"true"` or `"1"` to enable)
3. Default: disabled

The Make targets `example-gorm-mysql` and `example-gorm-postgres` enable backtrace automatically.

## HTTP Client Collector

The HTTP client collector captures outbound HTTP calls made during a profiled request via an `http.RoundTripper` wrapper. It provides visibility into service-to-service communication — timing, status codes, request/response details, and analysis.

```bash
go get github.com/piotrkardasz/go-profiler/collector/http
```

### Basic Usage

```go
import (
    "net/http"
    "time"

    profiler "github.com/piotrkardasz/go-profiler"
    httpcollector "github.com/piotrkardasz/go-profiler/collector/http"
)

// Register the HTTP client collector with the profiler
httpCollector := httpcollector.New(
    httpcollector.WithSlowThreshold(300 * time.Millisecond),
    httpcollector.WithBodyCapture(true),
    httpcollector.WithRedactHeaders("Authorization", "X-Api-Key"),
)
p.AddCollector(httpCollector)

// Create instrumented HTTP clients for downstream services
productClient := &http.Client{
    Timeout: 10 * time.Second,
    Transport: httpcollector.NewTransport("product-api", http.DefaultTransport,
        httpcollector.WithBodyCapture(true),
    ),
}

inventoryClient := &http.Client{
    Timeout: 5 * time.Second,
    Transport: httpcollector.NewTransport("inventory-service", http.DefaultTransport),
}
```

Calls are captured automatically as long as the request context flows through:

```go
mux.HandleFunc("/orders", func(w http.ResponseWriter, r *http.Request) {
    // Context from the profiler middleware flows into outbound calls
    req, _ := http.NewRequestWithContext(r.Context(), "GET", "http://product-api/items/123", nil)
    productClient.Do(req)

    req2, _ := http.NewRequestWithContext(r.Context(), "POST", "http://inventory/reserve", body)
    inventoryClient.Do(req2)
})

http.ListenAndServe(":8080", p.Middleware(mux))
```

### Options

| Option | Default | Description |
|--------|---------|-------------|
| `WithSlowThreshold(d)` | `500ms` | Duration threshold for flagging slow calls |
| `WithBodyCapture(bool)` | `false` | Enable request/response body capture |
| `WithMaxBodySize(n)` | `64KB` | Max body size before truncation |
| `WithHeaderCapture(bool)` | `true` | Capture request/response headers |
| `WithRedactHeaders(h...)` | `Authorization, Cookie, Set-Cookie` | Headers to redact |
| `WithBacktrace(bool)` | `false` | Capture call stacks for each HTTP call |
| `WithDuplicateDetection(bool)` | `true` | Detect repeated identical requests |
| `WithCurlGeneration(bool)` | `true` | Generate cURL commands for each call |

### Backtrace

Enable backtrace capture to see where each outbound HTTP call originated:

```bash
HTTP_PROFILER_BACKTRACE=true go run .
```

Or programmatically:

```go
httpcollector.NewTransport("svc", base, httpcollector.WithBacktrace(true))
```

**Precedence:**
1. Explicit `WithBacktrace(bool)` — highest priority
2. `HTTP_PROFILER_BACKTRACE` env variable (set to `"true"` or `"1"` to enable)
3. Default: disabled

### Analysis Features

The collector automatically detects:

- **Slow calls** — Calls exceeding the configured threshold (default 500ms)
- **Failed calls** — Non-2xx status codes or transport errors (timeouts, DNS failures)
- **Duplicate calls** — Repeated identical requests (same method + URL + body) within a single request

### Composability

The profiling transport stacks transparently with other `http.RoundTripper` implementations:

```go
// Stack with OpenTelemetry tracing
transport := httpcollector.NewTransport("svc",
    otelhttp.NewTransport(http.DefaultTransport),
)

// Stack with retry middleware
transport := httpcollector.NewTransport("svc",
    retryablehttp.NewTransport(http.DefaultTransport),
)
```

Each retry attempt or redirect hop is captured as a separate entry.

### Zero Overhead When Disabled

When the profiler is disabled or a request is not being profiled, the transport adds only a single context value lookup (~10ns) before delegating to the base transport.

### Example Output

The HTTP Clients panel in the profiler UI shows:
- Call list with method badges, status codes, and duration bars
- Service grouping when multiple services are called
- Expandable details with headers, bodies, cURL commands, and backtraces
- Analysis badges highlighting slow (yellow), failed (red), and duplicate (orange) calls

See the full example at `examples/http-clients/`.

## Make Targets

```
make build              # Build UI + Go package (with -tags profiler_ui)
make build-dev          # Build Go package without embedded UI (no Node.js required)
make test               # Run all Go tests (including collector/gorm and collector/http)
make test-http          # Run HTTP client collector tests with -race -v
make test-ui            # Run all Go tests with embedded UI
make vet                # Run go vet on all packages
make ui-build           # Build Vue UI for production
make ui-dist            # Build UI and copy to handler/ui_dist
make ui-dev             # Start Vue UI dev server
make clean              # Clean build artifacts
make example-basic      # Run basic example
make example-otel       # Run OTel example
make example-gorm-mysql    # Run GORM MySQL example (backtrace enabled)
make example-gorm-postgres # Run GORM PostgreSQL example (backtrace enabled)
make example-http-clients  # Run HTTP client collector example (backtrace enabled)
```

## Project Structure

```
github.com/piotrkardasz/go-profiler/
├── profile.go           # Profile struct, Storage interface, ID generation
├── profiler.go          # Core Profiler, Config, Shutdown, collector management
├── middleware.go        # HTTP middleware (async collection, sampling, panic recovery)
├── cmd/
│   └── profiler-setup/  # CLI tool for power users (custom Vue panels)
├── collector/
│   ├── collector.go     # Collector interfaces
│   ├── request.go       # Request/Response collector
│   ├── timing.go        # Timing collector
│   ├── memory.go        # Memory collector (runtime/metrics, no STW)
│   ├── memory_metrics.go # runtime/metrics helpers
│   ├── logger.go        # Logger collector (slog + stdlog adapters)
│   ├── logger_backtrace.go # Log entry backtrace capture
│   ├── config.go        # Config collector (cached, Refresh())
│   ├── config_reader.go # ConfigReader interface
│   ├── config_dotenv.go # Built-in .env file parser
│   ├── config_env.go    # Environment variable reader
│   ├── gorm/            # GORM database query collector (separate module)
│   ├── http/            # HTTP client collector (separate module)
│   └── otel/            # OpenTelemetry collector
├── storage/
│   ├── filesystem.go    # File-based JSON storage
│   └── memory.go        # In-memory LRU storage
├── handler/
│   ├── api.go           # JSON API handlers
│   ├── ui.go            # UI handler (embed + dev proxy)
│   ├── embed_ui.go      # //go:build profiler_ui — embeds ui_dist/
│   ├── embed_stub.go    # //go:build !profiler_ui — returns nil
│   └── ui_dist/         # Pre-built Vue UI assets (committed, auto-updated by CI)
├── ui/                  # Vue 3 + Vite + TypeScript source
│   └── src/
│       ├── plugin/      # Panel plugin system (registry + GenericPanel fallback)
│       └── components/panels/  # Built-in panels
├── .github/workflows/
│   ├── build-ui.yml     # Auto-rebuild ui_dist on push to main
│   └── check-ui-freshness.yml  # PR check for stale assets
├── examples/
│   ├── basic/           # Basic usage example
│   ├── gorm-mysql/      # GORM MySQL example
│   ├── gorm-postgres/   # GORM PostgreSQL example
│   ├── http-clients/    # HTTP client collector example
│   └── otel/            # OpenTelemetry example
└── Makefile
```

## License

MIT

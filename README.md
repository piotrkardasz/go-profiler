# Go Profiler

A framework-agnostic HTTP profiling middleware for Go, inspired by [Symfony's Profiler](https://symfony.com/doc/current/profiler.html). It collects per-request profiling data via pluggable collectors, stores profiles as JSON files, and exposes them through a JSON API and embedded Vue.js web UI.

## Features

- **Framework-agnostic** — Works with any `http.Handler`-compatible router or framework
- **Pluggable collectors** — Ships with Request, Timing, and Memory collectors; easily extensible
- **OpenTelemetry integration** — Captures traces and metrics per request
- **Pluggable storage** — File-based JSON storage by default, with an in-memory option for testing
- **`X-Profiler-Id` header** — Every profiled response includes a unique profile token
- **JSON API** — Query, filter, and retrieve profiles programmatically
- **Embedded Vue.js UI** — Browse profiles at `/_profiler/` with zero external dependencies
- **Extensible UI panels** — Register custom Vue components for custom collector visualization
- **Dual UI mode** — Embedded assets for production; proxies to Vite dev server for UI development
- **Enable/disable** — Controlled via `GO_PROFILER_ENABLED` env var or config

## Installation

```bash
go get github.com/piotrkardasz/go-profiler
```

Requires Go 1.21+.

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
| `UIDevMode` | `false` | `GO_PROFILER_UI_DEV` | Proxy UI to Vite dev server |
| `UIDevServerURL` | `http://localhost:5173` | — | Vite dev server URL |

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│  HTTP Request                                            │
├─────────────────────────────────────────────────────────┤
│  Profiler Middleware                                     │
│  ┌─────────────────┐  ┌──────────────────────────────┐ │
│  │ Generate ID      │  │ Set X-Profiler-Id header     │ │
│  │ Capture start    │  │ Wrap ResponseWriter          │ │
│  └─────────────────┘  └──────────────────────────────┘ │
├─────────────────────────────────────────────────────────┤
│  Your Handler                                            │
├─────────────────────────────────────────────────────────┤
│  Collectors (run after handler)                          │
│  ┌─────────┐ ┌────────┐ ┌────────┐ ┌──────┐          │
│  │ Request │ │ Timing │ │ Memory │ │ OTel │ ...       │
│  └─────────┘ └────────┘ └────────┘ └──────┘          │
├─────────────────────────────────────────────────────────┤
│  Storage (async)           JSON API         Vue UI      │
│  ┌──────────────┐    ┌───────────────┐  ┌───────────┐ │
│  │ Filesystem   │    │ /api/profiles │  │ /_profiler/│ │
│  │ (or Memory)  │    │ /api/collect. │  │ (embedded) │ │
│  └──────────────┘    └───────────────┘  └───────────┘ │
└─────────────────────────────────────────────────────────┘
```

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

Register a Vue component for your custom collector:

```typescript
// In your Vue app setup
import { registerPanel } from '@/plugin'
import DatabasePanel from './DatabasePanel.vue'

registerPanel('database', DatabasePanel)
```

Your panel component receives these props:

```vue
<script setup lang="ts">
defineProps<{
  data: unknown        // The collector's JSON data
  collectorName: string // The collector's name
}>()
</script>
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

## Make Targets

```
make build              # Build UI + Go package
make test               # Run all Go tests
make vet                # Run go vet
make ui-build           # Build Vue UI for production
make ui-dev             # Start Vue UI dev server
make clean              # Clean build artifacts
make example-basic      # Run basic example
make example-otel       # Run OTel example
make example-gorm-mysql    # Run GORM MySQL example (backtrace enabled)
make example-gorm-postgres # Run GORM PostgreSQL example (backtrace enabled)
```

## Project Structure

```
github.com/piotrkardasz/go-profiler/
├── profile.go           # Profile struct, Storage interface, ID generation
├── profiler.go          # Core Profiler, Config, collector management
├── middleware.go        # HTTP middleware
├── collector/
│   ├── collector.go     # Collector interfaces
│   ├── request.go       # Request/Response collector
│   ├── timing.go        # Timing collector
│   ├── memory.go        # Memory collector
│   └── otel/            # OpenTelemetry collector
├── storage/
│   ├── filesystem.go    # File-based JSON storage
│   └── memory.go        # In-memory LRU storage
├── handler/
│   ├── api.go           # JSON API handlers
│   ├── ui.go            # UI handler (embed + dev proxy)
│   └── embed.go         # Embedded UI assets
├── ui/                  # Vue 3 + Vite + TypeScript
│   └── src/
│       ├── plugin/      # Panel plugin system
│       └── components/panels/  # Built-in panels
├── examples/
│   ├── basic/           # Basic usage example
│   └── otel/            # OpenTelemetry example
└── Makefile
```

## License

MIT

# Design: Go Profiler

## Technical Design Document

### 1. System Architecture

The Go Profiler follows a layered architecture inspired by Symfony's profiler design:

```
┌─────────────────────────────────────────────────────────┐
│  HTTP Request                                            │
├─────────────────────────────────────────────────────────┤
│  Profiler Middleware (middleware.go)                      │
│  - Generates profile ID                                  │
│  - Injects context values (start time, memory snapshot)  │
│  - Wraps ResponseWriter to capture status/size           │
│  - Sets X-Profiler-Id header                            │
│  - After handler: runs collectors, stores async          │
├─────────────────────────────────────────────────────────┤
│  Application Handler                                     │
├─────────────────────────────────────────────────────────┤
│  Collectors Layer                                        │
│  ┌─────────┐ ┌────────┐ ┌────────┐ ┌──────┐          │
│  │ Request │ │ Timing │ │ Memory │ │ OTel │ custom... │
│  └─────────┘ └────────┘ └────────┘ └──────┘          │
├─────────────────────────────────────────────────────────┤
│  Storage Layer           API Layer         UI Layer      │
│  ┌──────────────┐    ┌───────────────┐  ┌───────────┐ │
│  │ Filesystem   │    │ APIHandler    │  │ UIHandler  │ │
│  │ Memory       │    │ (JSON REST)   │  │ (embed.FS) │ │
│  │ Custom...    │    └───────────────┘  └───────────┘ │
│  └──────────────┘                                      │
└─────────────────────────────────────────────────────────┘
```

### 2. Package Structure

```
github.com/piotrkardasz/go-profiler/
├── profile.go          # Profile, ProfileSummary, Storage interface, SearchCriteria, GenerateProfileID
├── profiler.go         # Profiler struct, Config, DefaultConfig, collector management
├── middleware.go       # HTTP middleware, responseWriter wrapper
├── collector/
│   ├── collector.go    # Collector, LateCollector, PanelMeta, PanelProvider, ResponseData interfaces
│   ├── request.go     # RequestCollector
│   ├── timing.go      # TimingCollector + context helpers
│   ├── memory.go      # MemoryCollector + context helpers
│   └── otel/
│       ├── collector.go  # Combined OTel Collector (LateCollector)
│       ├── traces.go     # SpanCapturer (sdktrace.SpanProcessor), TracesCollector
│       └── metrics.go    # MetricCapturer (sdkmetric.Exporter)
├── storage/
│   ├── storage.go      # Package declaration
│   ├── filesystem.go   # FilesystemStorage
│   └── memory.go       # MemoryStorage (LRU)
├── handler/
│   ├── api.go         # APIHandler (REST endpoints)
│   ├── ui.go          # UIHandler (embed or dev proxy)
│   ├── embed.go       # //go:embed directive, UIDistFS()
│   └── ui_dist/       # Embedded Vue build output
├── ui/                # Vue 3 + Vite + TypeScript source
│   ├── src/
│   │   ├── main.ts
│   │   ├── router.ts
│   │   ├── api.ts
│   │   ├── App.vue
│   │   ├── views/
│   │   │   ├── ProfileList.vue
│   │   │   └── ProfileDetail.vue
│   │   ├── plugin/
│   │   │   ├── index.ts
│   │   │   ├── registry.ts
│   │   │   └── builtin.ts
│   │   └── components/panels/
│   │       ├── GenericPanel.vue
│   │       ├── JsonNode.vue
│   │       ├── RequestPanel.vue
│   │       ├── TimingPanel.vue
│   │       ├── MemoryPanel.vue
│   │       └── OtelPanel.vue
│   ├── package.json
│   ├── vite.config.ts
│   └── tsconfig.json
└── examples/
    ├── basic/main.go
    └── otel/main.go
```

### 3. Core Interfaces

#### 3.1 Collector Interface

```go
// collector/collector.go
type Collector interface {
    Name() string
    Collect(ctx context.Context, req *http.Request, res ResponseData) (any, error)
    Reset()
}

type LateCollector interface {
    Collector
    LateCollect(ctx context.Context) (any, error)
}

type PanelProvider interface {
    PanelMeta() PanelMeta
}
```

Design decisions:
- `Collect()` returns `any` to allow collectors maximum flexibility in data shape.
- `Reset()` is called at the start of each request to clear per-request state.
- `LateCollector` allows post-response data collection (OTel spans, async operations).
- `PanelProvider` is optional — collectors without it get a default generic panel.

#### 3.2 Storage Interface

```go
// profile.go (in root package to avoid import cycles)
type Storage interface {
    Store(profile *Profile) error
    Load(id string) (*Profile, error)
    List(criteria SearchCriteria) ([]*ProfileSummary, error)
    Purge(maxAge time.Duration) (int, error)
}
```

Design decisions:
- Interface lives in root package because `Profile` is defined there; prevents import cycles.
- `List()` returns `ProfileSummary` (lightweight) instead of full profiles for efficiency.
- `Purge()` returns count of removed profiles for reporting.

#### 3.3 Profile

```go
type Profile struct {
    ID            string         `json:"id"`
    Method        string         `json:"method"`
    URL           string         `json:"url"`
    StatusCode    int            `json:"status_code"`
    Timestamp     time.Time      `json:"timestamp"`
    Duration      time.Duration  `json:"duration"`
    CollectorData map[string]any `json:"collector_data"`
}
```

Design decisions:
- `Duration` serializes as milliseconds (float64) in JSON for cross-language compatibility.
- `CollectorData` keyed by collector `Name()` — simple, no collisions if names are unique.
- Profile ID is 16-char hex (8 bytes from `crypto/rand`) — same approach as Symfony's tokens.

### 4. Middleware Design

The middleware wraps `http.ResponseWriter` to capture status/size:

```go
type responseWriter struct {
    http.ResponseWriter
    statusCode int
    size       int64
    written    bool
}
```

Flow:
1. Check if profiler is enabled; pass through if not.
2. Skip profiler's own routes (`/_profiler/` prefix).
3. Generate profile ID, set `X-Profiler-Id` header.
4. Inject start time and memory snapshot into context.
5. Call `next.ServeHTTP()` with wrapped writer.
6. After handler: compute duration, run collectors, assemble profile.
7. Store profile + run late collectors in a goroutine (async).

Design decisions:
- Header is set BEFORE the handler runs so it appears even if the handler panics.
- Async storage ensures zero response latency impact.
- `Unwrap()` method for `http.ResponseController` compatibility (Go 1.20+).
- `http.Flusher` support for streaming responses.

### 5. Storage Implementations

#### 5.1 FilesystemStorage

- One JSON file per profile: `{dir}/{id}.json`
- Atomic writes: create temp file, write, `os.Rename()`
- Thread-safe: `sync.RWMutex`
- Path traversal prevention: reject IDs containing `/`, `\`, `..`
- List: reads all files, sorts in memory (suitable for dev workloads of ~1000 profiles)

#### 5.2 MemoryStorage

- `container/list` doubly-linked list for LRU ordering
- `map[string]*list.Element` for O(1) lookups
- Configurable max entries (default 200)
- `Load()` moves to front (most recently used)
- `Store()` evicts from back when over capacity

### 6. OpenTelemetry Integration

The OTel collector uses two interception points:

1. **SpanCapturer** (`sdktrace.SpanProcessor`): registered with `TracerProvider`, captures all completed spans in a buffer. The profiler's `LateCollect()` drains this buffer.

2. **MetricCapturer** (`sdkmetric.Exporter`): intercepts metric exports, captures data points, optionally forwards to downstream exporter.

Design decisions:
- Uses SDK interfaces (not API-level) because we need access to completed span data.
- `LateCollect` is necessary because child spans may end after the response is sent.
- `CapturedSpans()`/`CapturedMetrics()` drain the buffer (reset after read).
- Combined `Collector` merges spans + metrics into single `OtelData` struct for one panel.

### 7. UI Architecture

#### 7.1 Embedding Strategy

- Vue app built to `ui/dist/`
- Copied to `handler/ui_dist/` before Go build
- Embedded via `//go:embed all:ui_dist`
- `UIDistFS()` accessor provides `fs.FS` rooted at dist contents

#### 7.2 Panel Plugin System

```typescript
// Registry pattern
const panelRegistry = new Map<string, Component>()

registerPanel(collectorName, component)  // custom panels
getPanel(collectorName) → Component      // returns custom or GenericPanel
```

- Built-in panels registered in `initBuiltinPanels()` on app start
- `GenericPanel` + `JsonNode` provide recursive JSON tree for any data shape
- Custom panels receive `{ data, collectorName }` props

#### 7.3 Dual Mode

- **Production**: `UIHandler` serves from `embed.FS`, with SPA fallback (non-file paths → index.html)
- **Dev mode**: `UIHandler` is a `httputil.ReverseProxy` to `http://localhost:5173`
- Controlled by `GO_PROFILER_UI_DEV` env var

### 8. API Design

Standard REST with `net/http.ServeMux`:

| Route | Handler | Notes |
|-------|---------|-------|
| `{prefix}/api/profiles` | `handleProfiles` | GET=list, DELETE=purge |
| `{prefix}/api/profiles/` | `handleProfileByID` | GET by trailing path segment |
| `{prefix}/api/collectors` | `handleCollectors` | GET metadata for UI |

CORS headers added on all responses for Vite dev server cross-origin requests.

### 9. Dependency Management

External dependencies (minimal):
- `go.opentelemetry.io/otel` + `/sdk` + `/sdk/metric` — OTel collector only
- All other Go code uses stdlib only

Frontend:
- `vue@^3.4`, `vue-router@^4.3`
- `vite@^5.4`, `@vitejs/plugin-vue`, `typescript`, `vue-tsc`

### 10. Import Cycle Resolution

Key design constraint: `Storage` interface references `Profile`, and `Profiler` uses `Storage`. To avoid cycles:
- `Profile`, `ProfileSummary`, `Storage`, `SearchCriteria` all live in the **root** `profiler` package.
- The `storage/` package imports the root package for these types.
- The root package does NOT import `storage/` — users wire them together at init time.
- `collector/` package is standalone (no root import); root imports `collector/`.

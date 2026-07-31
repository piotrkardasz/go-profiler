# Tasks: Go Profiler

## Implementation Tasks

### Task 1: Project scaffolding and core interfaces [DONE]

**Objective:** Initialize the Go module, define core interfaces (`Collector`, `LateCollector`, `Storage`), and create the `Profile` struct with ID generation.

**Implementation:**
- Run `go mod init github.com/piotrkardasz/go-profiler`
- Define `Profile` struct with fields: ID, URL, Method, StatusCode, Timestamp, Duration, CollectorData (map[string]any)
- Define `ProfileSummary` for lightweight list results
- Custom JSON marshaling: Duration as milliseconds (float64)
- Define `Collector` and `LateCollector` interfaces in `collector/collector.go`
- Define `Storage` interface and `SearchCriteria` in root `profile.go` (avoids import cycles)
- Use `crypto/rand` for profile ID generation (16-char hex string)
- Define `PanelMeta` and `PanelProvider` for UI panel metadata

**Files created:**
- `go.mod`
- `profile.go`
- `profile_test.go`
- `collector/collector.go`
- `storage/storage.go`

---

### Task 2: Profiler core and configuration [DONE]

**Objective:** Build the central `Profiler` struct that manages collectors, storage, and enable/disable logic.

**Implementation:**
- `Config` struct: Enabled, StoragePath, RoutePrefix, Logger, UIDevMode, UIDevServerURL
- `DefaultConfig()` reads `GO_PROFILER_ENABLED` and `GO_PROFILER_UI_DEV` env vars
- `Profiler` struct with `sync.RWMutex` for thread-safe collector management
- Methods: `AddCollector()`, `Collectors()`, `IsEnabled()`, `SetEnabled()`
- `CollectProfile()`: iterates collectors, calls `Collect()`, assembles profile (skips errored collectors)
- `CollectLate()`: runs `LateCollect()` on LateCollector implementations
- `ResetCollectors()`: clears state between requests
- `PanelMetas()`: returns panel metadata for UI (uses PanelProvider or defaults)

**Files created:**
- `profiler.go`
- `profiler_test.go`

---

### Task 3: Built-in collectors (Request, Timing, Memory) [DONE]

**Objective:** Implement three foundational collectors that ship with the package.

**Implementation:**
- `RequestCollector`: method, URL, host, remote_addr, proto, headers (with sensitive redaction), query params, content_type, response status/headers/size
- `TimingCollector`: uses context key for start time, computes duration in microsecond precision
- `MemoryCollector`: uses context key for pre-handler `runtime.MemStats` snapshot, computes delta
- Context helpers: `WithStartTime()`, `StartTimeFromContext()`, `WithMemoryStats()`, `MemoryStatsFromContext()`
- All implement `Collector` + `PanelProvider` interfaces
- Sensitive headers redacted: Authorization, Cookie, Set-Cookie

**Files created:**
- `collector/request.go`, `collector/request_test.go`
- `collector/timing.go`, `collector/timing_test.go`
- `collector/memory.go`, `collector/memory_test.go`

---

### Task 4: File-based storage implementation [DONE]

**Objective:** Implement the default file-based storage that persists profiles as JSON files.

**Implementation:**
- `FilesystemStorage` with configurable directory (auto-created)
- `Store()`: atomic writes (temp file → rename), JSON indent formatting
- `Load()`: read + unmarshal, path traversal prevention (rejects `/`, `\`, `..` in IDs)
- `List()`: reads directory, unmarshals all profiles, sorts by timestamp desc, applies filters, paginates
- `Purge()`: iterates files, removes those with timestamp before cutoff
- Thread-safe: `sync.RWMutex` (write lock for Store/Purge, read lock for Load/List)
- File permissions: 0644 for files, 0755 for directory

**Files created:**
- `storage/filesystem.go`
- `storage/filesystem_test.go`

---

### Task 5: In-memory storage implementation [DONE]

**Objective:** Implement a simple in-memory storage for use in tests and ephemeral environments.

**Implementation:**
- `MemoryStorage` using `container/list` + `map[string]*list.Element`
- LRU eviction: configurable max entries (default 200)
- `Store()`: add to front, evict from back if over capacity
- `Load()`: moves element to front (most recently accessed)
- Same filter/pagination logic as filesystem
- `Purge()`: iterates list, removes by timestamp

**Files created:**
- `storage/memory.go`
- `storage/memory_test.go`

---

### Task 6: HTTP middleware [DONE]

**Objective:** Create the framework-agnostic HTTP middleware that ties everything together.

**Implementation:**
- `Profiler.Middleware(next http.Handler) http.Handler`
- `responseWriter` wrapper: captures statusCode, size, implements Flusher + Unwrap
- Flow: check enabled → skip profiler routes → generate ID → reset collectors → inject context → set header → serve → collect → async store
- Async goroutine: runs LateCollect, then Store
- `X-Profiler-Id` header set before handler runs

**Files created:**
- `middleware.go`
- `middleware_test.go`

---

### Task 7: JSON API handlers [DONE]

**Objective:** Implement REST API endpoints for profile access.

**Implementation:**
- `APIHandler` with `RegisterRoutes(mux, prefix)` on `http.ServeMux`
- `GET /api/profiles`: list with query filters (method, url, status, min_status, max_status, since, until, limit, offset)
- `GET /api/profiles/{id}`: load by ID, 404 if not found
- `DELETE /api/profiles`: purge with `max_age` param (default 24h)
- `GET /api/collectors`: returns panel metadata array
- CORS headers on all responses, OPTIONS preflight support
- JSON error responses with `{"error": "message"}` format

**Files created:**
- `handler/api.go`
- `handler/api_test.go`

---

### Task 8: Vue 3 + Vite UI scaffolding [DONE]

**Objective:** Set up the Vue 3 project with Vite, basic layout, and routing.

**Implementation:**
- Vue 3 + Vite + TypeScript in `ui/` directory
- `App.vue`: header with brand/nav, main content area
- Vue Router: `/_profiler/` (list), `/_profiler/profile/:id` (detail)
- `api.ts`: typed fetch client (listProfiles, getProfile, getCollectors, purgeProfiles)
- `ProfileList.vue`: table with method/URL/status badges, filters, pagination, purge
- `ProfileDetail.vue`: summary header + tabbed panels using dynamic components
- `plugin/registry.ts`: panel registration system (registerPanel, getPanel)
- `GenericPanel.vue` + `JsonNode.vue`: recursive JSON tree renderer fallback
- Vite config: base `/_profiler/`, proxy API to Go backend in dev

**Files created:**
- `ui/package.json`, `ui/tsconfig.json`, `ui/vite.config.ts`, `ui/env.d.ts`, `ui/index.html`
- `ui/src/main.ts`, `ui/src/router.ts`, `ui/src/api.ts`, `ui/src/App.vue`
- `ui/src/views/ProfileList.vue`, `ui/src/views/ProfileDetail.vue`
- `ui/src/plugin/registry.ts`
- `ui/src/components/panels/GenericPanel.vue`, `ui/src/components/panels/JsonNode.vue`

---

### Task 9: Vue panel plugin system [DONE]

**Objective:** Implement the extensible panel registration system so custom collectors can provide rich UI.

**Implementation:**
- `plugin/index.ts`: public API exports
- `plugin/builtin.ts`: `initBuiltinPanels()` registers built-in panels on app init
- `RequestPanel.vue`: method/URL/headers/query params/response in sections with tables
- `TimingPanel.vue`: hero duration display + timing bar + details table
- `MemoryPanel.vue`: stat cards (heap/delta/goroutines/GC) + full memory details table
- `PanelRegistry.vue`: dynamic component rendering via getPanel()
- Plugin API: `registerPanel(collectorName, component)` for external custom panels

**Files created:**
- `ui/src/plugin/index.ts`, `ui/src/plugin/builtin.ts`
- `ui/src/components/PanelRegistry.vue`
- `ui/src/components/panels/RequestPanel.vue`
- `ui/src/components/panels/TimingPanel.vue`
- `ui/src/components/panels/MemoryPanel.vue`

---

### Task 10: OpenTelemetry collector [DONE]

**Objective:** Implement a collector that captures OTel metrics and traces/spans associated with a request.

**Implementation:**
- `collector/otel/` sub-package
- `SpanCapturer`: implements `sdktrace.SpanProcessor`, buffers completed spans
- `MetricCapturer`: implements `sdkmetric.Exporter`, intercepts metric exports with optional downstream forwarding
- `Collector`: combined OTel collector implementing `LateCollector`
  - `Collect()`: captures current span context
  - `LateCollect()`: drains SpanCapturer + MetricCapturer buffers
- `TracesCollector`: traces-only variant
- Data types: `OtelData{Spans, Metrics}`, `SpanInfo`, `SpanEvent`, `MetricInfo`
- Supports: Gauge[float64/int64], Sum[float64/int64], Histogram[float64/int64]
- `attrSetToMap()` for OTel attribute.Set → map[string]string conversion

**Files created:**
- `collector/otel/collector.go`
- `collector/otel/traces.go`
- `collector/otel/metrics.go`
- `collector/otel/otel_test.go`

---

### Task 11: Vue OTel panel [DONE]

**Objective:** Create a rich Vue panel for the OTel collector showing traces and metrics.

**Implementation:**
- `OtelPanel.vue`: dual-tab view (Traces/Metrics) with count badges
- Traces tab:
  - Waterfall timeline with grid layout (span name, duration, bar)
  - Spans sorted by start_time
  - Nesting depth via parent_id lookup
  - Relative bar positioning using time window calculation
  - Status badges (ok/error)
  - Click-to-detail: trace ID, span ID, parent, duration, attributes, events
- Metrics tab:
  - Table with name/description, type badges (gauge/sum/histogram), value, unit, attribute badges
- Registered in `builtin.ts`

**Files created:**
- `ui/src/components/panels/OtelPanel.vue`

---

### Task 12: Embed Vue UI and dual-mode handler [DONE]

**Objective:** Build the Vue app, embed it in Go via `embed.FS`, and support both dev proxy and embedded modes.

**Implementation:**
- `handler/embed.go`: `//go:embed all:ui_dist` + `UIDistFS()` accessor
- `handler/ui_dist/`: copy of built `ui/dist/`
- `UIHandler` with `UIConfig{RoutePrefix, DevMode, DevServerURL, Assets}`
- Production mode: serves from `fs.FS` directly
  - SPA routing: non-file-extension paths → index.html
  - `inferContentType()` for proper MIME types
  - `http.ServeContent()` for proper caching headers
- Dev mode: `httputil.NewSingleHostReverseProxy` to Vite dev server
- `RegisterRoutes()` catches all under prefix

**Files created:**
- `handler/embed.go`
- `handler/ui.go`
- `handler/ui_test.go`
- `handler/ui_dist/` (built assets)

---

### Task 13: Integration examples and documentation [DONE]

**Objective:** Create complete working examples and README documentation.

**Implementation:**
- `examples/basic/main.go`: HTTP server with all built-in collectors, 4 test endpoints (home, users, slow, error), profiler UI/API setup
- `examples/otel/main.go`: OTel integration with TracerProvider + SpanCapturer, traced handlers creating DB/payment/inventory spans with attributes
- `Makefile` with targets: help, build, test, vet, lint, ui-build, ui-dev, clean, example-basic, example-otel
- `README.md`: installation, quick start code, configuration table, architecture diagram, custom collectors guide, custom UI panels guide, OTel integration guide, storage options, JSON API reference, UI development workflow, project structure, make targets

**Files created:**
- `examples/basic/main.go`
- `examples/otel/main.go`
- `Makefile`
- `README.md`

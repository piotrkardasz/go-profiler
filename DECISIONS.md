# Decision Log: Go Profiler

This document records the decisions made during the initial conversation that shaped the requirements and design of the Go Profiler package.

---

## Decision 1: Primary Use Case

**Question:** What is the primary use case for this profiler?

**Options presented:**
- a. Web applications — HTTP request/response profiling
- b. General-purpose — Profile any Go code
- c. Both — Web-focused but extensible

**Decision:** **Web applications (a)**

**Rationale:** Focus on HTTP request/response profiling, similar to how Symfony profiles web requests. This scopes the project to middleware-based profiling rather than trying to be a general-purpose profiling tool.

---

## Decision 2: Feature Scope

**Question:** Which Symfony profiler features are most important?

**Options presented:**
- a. Data collection — Timing, memory, DB queries, custom collectors
- b. Storage & retrieval — Storing profiles and searching/viewing them later
- c. Web toolbar/UI — A visual debug bar
- d. All of the above

**Decision:** **Web toolbar/UI (c), with a specific approach**

**Clarification:** Rather than injecting a toolbar into HTML responses (Symfony's approach), the profiler attaches an `X-Profiler-Id` HTTP header to responses. This ID can then be used to browse a dedicated endpoint which provides a UI for viewing profiled requests.

**Rationale:** The header-based approach works for all response types (JSON APIs, HTML, binary), not just HTML pages. It's cleaner for modern API-first applications and doesn't modify response bodies.

---

## Decision 3: Framework Compatibility

**Question:** What web framework(s) or router(s) do you plan to use this with?

**Options presented:**
- a. Standard library (`net/http`)
- b. Popular frameworks (Gin, Echo, Chi, Fiber)
- c. Framework-agnostic — works with any `http.Handler`-compatible stack
- d. Not sure yet

**Decision:** **Framework-agnostic (c)**

**Rationale:** By implementing as standard `http.Handler` middleware, the profiler works with any Go HTTP framework without requiring framework-specific adapters. Maximum compatibility with minimal coupling.

---

## Decision 4: Initial Collectors

**Question:** What data collectors do you want in the initial version?

**Options presented:**
- a. Minimal — Request/response details, timing, memory
- b. Moderate — Above + database queries, template rendering, logs
- c. Extensible — Start minimal but design a collector interface for custom extensions

**Decision:** **Extensible (c)**

**Rationale:** Ship with the essential collectors (request, timing, memory) but design the `Collector` interface so users can easily add their own without modifying the core package. This follows the open/closed principle.

---

## Decision 5: Storage Backend

**Question:** How should profiles be stored?

**Options presented:**
- a. In-memory — Simple, lost on restart
- b. File-based — Persist to disk
- c. Pluggable — Storage interface with default, users can implement custom backends

**Decision:** **Pluggable (c) with file-based as default**

**Rationale:** A `Storage` interface provides maximum flexibility (Redis, database, etc.), while file-based JSON is the sensible default for development — it persists across restarts, is human-readable, and requires no additional infrastructure. The in-memory implementation is provided for testing.

---

## Decision 6: Web UI Approach

**Question:** What level of sophistication for the web UI?

**Options presented:**
- a. Embedded HTML/JS — Self-contained in the Go binary
- b. JSON API only — Build frontend separately later
- c. Both — JSON API + basic embedded UI

**Decision:** **Both (c), using Vue.js**

**Additional decision:** The UI should be built with Vue 3 because it should be easy to extend — since the collector system is pluggable, custom collectors need to be able to register their own UI panels (e.g., an OpenTelemetry collector showing what metrics were sent).

**Rationale:** A JSON API alone is useful for programmatic access, but a bundled UI provides immediate value without requiring users to build their own frontend. Vue's component system maps naturally to the panel-per-collector architecture.

---

## Decision 7: UI Panel Extensibility

**Question:** How should custom collector panels be added to the Vue UI?

**Options presented:**
- a. Build-time plugin system — Rebuild UI when adding collectors
- b. Runtime registration — Dynamic rendering from metadata
- c. Hybrid — Generic dynamic rendering by default, custom Vue components for richer views

**Decision:** **Hybrid (c)**

**Rationale:** The generic JSON tree renderer handles any collector with zero effort. But collectors that want a richer experience (waterfall traces, charts, formatted tables) can register a custom Vue component. This means:
- Zero friction to add a new collector (generic panel works immediately)
- Optional investment for a polished experience (register custom component)

---

## Decision 8: Enable/Disable Strategy

**Question:** How should the profiler be enabled/disabled?

**Options presented:**
- a. Always on in dev, disabled in prod — Controlled by env var or config flag
- b. Conditional — Per-request based on header, IP, or cookie
- c. Both — Global + per-request conditional

**Decision:** **Environment-based (a)**

**Rationale:** Keep it simple. An environment variable (`GO_PROFILER_ENABLED`) or config flag controls whether profiling is active. This matches typical deployment patterns where dev/staging have it on and production has it off. Per-request conditional profiling adds complexity that can be added later if needed.

---

## Decision 9: Go Version and Module Path

**Question:** Preferred Go version and module naming?

**Options presented:**
- a. Go 1.21+ (structured logging with `slog`, latest features)
- b. Go 1.18+ (generics, wider compatibility)

**Decision:** **Go 1.21+ (a)**

**Module path:** `github.com/piotrkardasz/go-profiler`

**Rationale:** Go 1.21 provides `slog` for structured logging (used in the profiler's error reporting), and allows use of modern stdlib features without worrying about backward compatibility with older Go versions.

---

## Decision 10: Storage File Format

**Question:** For file-based storage, what format and retention?

**Options presented:**
- a. JSON files — One file per profile, easy to inspect manually
- b. SQLite — Single file, efficient querying
- c. JSON files + index — JSON per profile with lightweight index for fast searching

**Decision:** **JSON files (a)**

**Rationale:** One JSON file per profile is the simplest approach. Files are human-readable (useful for debugging), require no additional dependencies, and are straightforward to implement. For the expected workload in development (hundreds to low thousands of profiles), reading the directory is fast enough without needing a separate index.

---

## Decision 11: OpenTelemetry Collector Scope

**Question:** What scope for the OTel collector?

**Options presented:**
- a. Capture outgoing metrics only
- b. Capture traces/spans only
- c. Both metrics and traces

**Decision:** **Both metrics and traces (c)**

**Rationale:** Providing full observability per request makes the profiler a comprehensive debugging tool. Seeing which spans were created (DB queries, HTTP calls, custom operations) alongside which metrics were recorded gives a complete picture of what happened during the request.

---

## Decision 12: Vue UI Bundling

**Question:** How should the Vue UI be bundled into the Go binary?

**Options presented:**
- a. `embed.FS` only — Pre-built Vue app embedded in Go binary
- b. Separate process — Vue dev server during development, then build for production
- c. Both modes — Dev mode proxies to Vite dev server, production uses embed.FS

**Decision:** **Both modes (c)**

**Rationale:** During UI development, hot reload via Vite dev server is essential for productivity. In production (or for users who just consume the library), the embedded assets require zero external dependencies. A `GO_PROFILER_UI_DEV=true` flag switches between modes.

---

## Decision 13: Design Philosophy

**Question:** Are there existing Go profiling packages to reference?

**Decision:** **Start fresh, use Symfony as the conceptual model**

**Clarification:** Focus mainly on Symfony's architecture (DataCollectorInterface, token-based profiles, web profiler UI) but apply Go best practices. Do not reuse whole implementations from existing Go profiling libraries. The package must be extendable in a clean, idiomatic Go way.

**Rationale:** Symfony's profiler is a proven, battle-tested architecture for web request profiling. Translating its concepts to Go idioms (interfaces, middleware pattern, embed.FS) produces a package that feels natural to Go developers while providing Symfony-level functionality.

---

## Decision Summary Table

| # | Topic | Decision | Key Reason |
|---|-------|----------|------------|
| 1 | Use case | Web applications | Scoped to HTTP profiling |
| 2 | Feature scope | X-Profiler-Id header + dedicated UI | Works for all response types |
| 3 | Framework | Framework-agnostic (http.Handler) | Maximum compatibility |
| 4 | Collectors | Extensible with interface | Open/closed principle |
| 5 | Storage | Pluggable, file-based default | Flexible + zero-infra default |
| 6 | UI | Vue 3 + JSON API | Extensible panel system |
| 7 | Panel extensibility | Hybrid (generic + custom components) | Zero friction + optional polish |
| 8 | Enable/disable | Environment variable | Simple, matches deployment patterns |
| 9 | Go version | 1.21+ | slog, modern stdlib |
| 10 | File format | JSON (one per profile) | Human-readable, no deps |
| 11 | OTel scope | Both metrics + traces | Complete observability |
| 12 | UI bundling | Dual mode (embed + dev proxy) | Best of both worlds |
| 13 | Design philosophy | Symfony concepts, Go idioms | Proven architecture, native feel |


---

## Decision 14: Async Collection (Performance)

**Question:** How should the profiler collect data without affecting application request latency?

**Options evaluated:**
- a. Goroutine-per-request — Move `CollectProfile()` into the existing async goroutine
- b. Bounded worker pool — Fixed N workers consuming from a buffered channel
- c. Lock-free ring buffer — MPSC queue for minimum enqueue latency
- d. Sampling only — Skip a fraction of requests entirely

**Decision:** **Goroutine-per-request (a) + runtime/metrics + config caching + optional sampling**

**Rationale:**
- The previous synchronous implementation added 200µs–10ms latency per request due to two `runtime.ReadMemStats` stop-the-world calls, file I/O (.env reading), and GORM query analysis.
- Analysis proved all `Collector.Collect()` calls can be safely deferred: `context.Context` values are immutable, `*http.Request` is not recycled after `ServeHTTP`, and GORM queries are no longer written to after the handler completes.
- Goroutine-per-request is the simplest change (5 lines in middleware.go) with ~95% overhead reduction. It's already the pattern used for `LateCollect` and `Storage.Store`.
- Worker pool was evaluated but deferred — it adds complexity unnecessary for a development profiler. Can be added later if needed at >10K req/s.
- Ring buffer was rejected as overkill — it optimizes nanoseconds when we're removing milliseconds.
- Sampling was added as an optional complement (configurable `SampleRate`).

**Additional optimizations applied:**
1. `runtime.ReadMemStats` replaced with `runtime/metrics` package (no STW)
2. ConfigCollector caches sources at construction (no per-request file I/O)
3. Panic recovery in goroutine prevents collector bugs from crashing the app
4. `sync.WaitGroup`-based inflight tracking for graceful shutdown

**Result:** Synchronous overhead reduced from 200µs–10ms to ~5µs per request (benchmarked). Zero stop-the-world pauses.

---

## Decision 15: Memory Statistics via runtime/metrics

**Question:** How to collect memory statistics without stop-the-world pauses?

**Options evaluated:**
- a. Keep `runtime.ReadMemStats` — Simple, but causes STW pauses (10µs–1ms+ per call)
- b. Use `runtime/metrics` package — No STW, available since Go 1.16
- c. Drop memory collection entirely — No overhead, but loses useful data

**Decision:** **runtime/metrics (b)**

**Rationale:**
- `runtime.ReadMemStats` is documented to stop all goroutines while reading consistent memory statistics. On large heaps this takes milliseconds and affects the entire process — not just the profiled request.
- `runtime/metrics` reads atomically-maintained counters with zero STW impact. DataDog (dd-trace-go) and Prometheus (client_golang) both migrated to it for the same reason.
- The metrics available cover all fields needed by `MemoryData`: heap allocation, total allocation, heap in-use, heap objects, GC cycles, goroutine count, system memory.
- Available since Go 1.16, well within our Go 1.21+ requirement.

**Trade-off:** Some metric values are slightly different (aggregated differently by the runtime), but provide equivalent insight for profiling purposes. The JSON output field names remain identical for UI compatibility.

---

## Decision 16: ConfigCollector Caching

**Question:** Should the ConfigCollector read .env files and environment variables on every request?

**Options evaluated:**
- a. Read per request — Current behavior, simple but adds 50–500µs of file I/O
- b. Cache at construction, never refresh — Fastest, but can't detect runtime changes
- c. Cache at construction with manual Refresh() — Fast + escape hatch for changes

**Decision:** **Cache with Refresh() (c)**

**Rationale:**
- Environment variables and `.env` files virtually never change during a running process.
- Reading them on every request was pure waste: file `open()`/`read()`/`close()` syscalls and `os.Environ()` allocating a copy of all env vars.
- `Refresh()` provides an explicit mechanism for the rare case where configuration changes at runtime (e.g., hot-reload scenarios).
- Runtime info and build info were already cached (immutable). Extending caching to reader sources is consistent.

---

## Decision 17: Graceful Shutdown

**Question:** How to ensure no profile data is lost when the application shuts down?

**Decision:** Add `Shutdown(ctx context.Context) error` method using `sync.WaitGroup`.

**Rationale:**
- With async collection, in-flight goroutines may still be processing when `main()` returns.
- `sync.WaitGroup` tracks active goroutines; `Shutdown` waits for all to complete.
- Context-based timeout prevents hanging forever if a collector gets stuck.
- After shutdown, new requests skip profiling (handlers still execute normally).
- This pattern is consistent with `http.Server.Shutdown()` in the standard library.

---

## Updated Decision Summary Table

| # | Topic | Decision | Key Reason |
|---|-------|----------|------------|
| 1 | Use case | Web applications | Scoped to HTTP profiling |
| 2 | Feature scope | X-Profiler-Id header + dedicated UI | Works for all response types |
| 3 | Framework | Framework-agnostic (http.Handler) | Maximum compatibility |
| 4 | Collectors | Extensible with interface | Open/closed principle |
| 5 | Storage | Pluggable, file-based default | Flexible + zero-infra default |
| 6 | UI | Vue 3 + JSON API | Extensible panel system |
| 7 | Panel extensibility | Hybrid (generic + custom components) | Zero friction + optional polish |
| 8 | Enable/disable | Environment variable | Simple, matches deployment patterns |
| 9 | Go version | 1.21+ | slog, modern stdlib |
| 10 | File format | JSON (one per profile) | Human-readable, no deps |
| 11 | OTel scope | Both metrics + traces | Complete observability |
| 12 | UI bundling | Dual mode (embed + dev proxy) | Best of both worlds |
| 13 | Design philosophy | Symfony concepts, Go idioms | Proven architecture, native feel |
| 14 | Collection execution | Async goroutine-per-request | ~5µs overhead vs previous ms |
| 15 | Memory stats | runtime/metrics (no STW) | Eliminates global pauses |
| 16 | Config caching | Cache at construction + Refresh() | Zero per-request I/O |
| 17 | Graceful shutdown | sync.WaitGroup + context timeout | No data loss on exit |



---

## Decision 18: Per-Request OTel Span and Metric Isolation

**Question:** How to prevent cross-request contamination of OpenTelemetry spans and metrics when multiple requests are in flight?

**Problem:**
The original OTel collector used a single global buffer for spans (`SpanCapturer`) and metrics (`MetricCapturer`). `LateCollect` drained the entire buffer for whichever request called it first. With async collection in a goroutine, concurrent requests would race each other, resulting in:
- Request A's profile containing spans from Request B (and vice versa)
- Metrics being attributed to whichever request's `LateCollect` fired first after a periodic export tick
- Duplicated or missing spans across profiles

**Options evaluated:**
- a. Per-request SpanProcessor instance — Create a new capturer per request, inject via context
- b. Trace-ID-correlated shared capturer — Keep one global capturer but index spans by trace ID, drain per-trace
- c. Keep global drain, document as limitation — Accept the behavior and label it "process-wide"

**Decision:** **Trace-ID-correlated shared capturer (b) + per-request metric windowing**

**Rationale:**
- Option (a) requires users to wire a new SpanProcessor per request into the TracerProvider, which is complex, non-standard, and can't work with a fixed provider setup.
- Option (b) keeps the existing wiring pattern (one capturer registered once as a SpanProcessor) while adding per-request isolation internally. The change is transparent to users.
- Option (c) loses valuable per-request observability data.

**Implementation:**

1. **SpanCapturer redesigned:** Internal storage changed from `[]ReadOnlySpan` to `map[trace.TraceID][]ReadOnlySpan`. `OnEnd` indexes each span by its trace ID. New method `CapturedSpansForTrace(traceID)` drains only one trace's spans; `CapturedSpans()` retained for backward compatibility.

2. **Collector implements `ContextSetup`:** `SetupContext(ctx)` captures the request's active trace ID from `trace.SpanContextFromContext(ctx)` and stores it in context. The profiler middleware already calls `SetupContext` on collectors that implement this interface — zero changes needed upstream.

3. **LateCollect uses trace ID:** Reads the stored trace ID from context and calls `CapturedSpansForTrace(traceID)` instead of the global drain. Falls back to `CapturedSpans()` if no trace ID is present (backward compat for users not using instrumented middleware).

4. **Per-request metric windowing:** `MetricCapturer` gained `StartRequestMetrics(requestID)` / `EndRequestMetrics(requestID)` methods. `SetupContext` registers the request; `LateCollect` retrieves only metrics exported during that request's lifetime. This doesn't give "request-caused metrics" (impossible with global aggregates) but does give a time-correlated window.

**Trade-offs:**
- Spans: Fully correct per-request attribution. A span belongs to exactly one trace ID, so there's no ambiguity.
- Metrics: Still process-global by nature (OTel periodic reader exports all instruments). The windowing approach attributes metrics by time overlap, not causation. For true per-request metrics, use the profiler's built-in timing/memory collectors.
- Memory: The `byTraceID` map grows with active traces. Cleaned up as each request's `LateCollect` runs. Long-lived uncollected traces could accumulate — acceptable for a development profiler.

**Result:** Concurrent requests no longer contaminate each other's span data. Verified with race-detector-clean concurrent tests (20 simultaneous goroutines).

---

## Updated Decision Summary Table

| # | Topic | Decision | Key Reason |
|---|-------|----------|------------|
| 1 | Use case | Web applications | Scoped to HTTP profiling |
| 2 | Feature scope | X-Profiler-Id header + dedicated UI | Works for all response types |
| 3 | Framework | Framework-agnostic (http.Handler) | Maximum compatibility |
| 4 | Collectors | Extensible with interface | Open/closed principle |
| 5 | Storage | Pluggable, file-based default | Flexible + zero-infra default |
| 6 | UI | Vue 3 + JSON API | Extensible panel system |
| 7 | Panel extensibility | Hybrid (generic + custom components) | Zero friction + optional polish |
| 8 | Enable/disable | Environment variable | Simple, matches deployment patterns |
| 9 | Go version | 1.21+ | slog, modern stdlib |
| 10 | File format | JSON (one per profile) | Human-readable, no deps |
| 11 | OTel scope | Both metrics + traces | Complete observability |
| 12 | UI bundling | Dual mode (embed + dev proxy) | Best of both worlds |
| 13 | Design philosophy | Symfony concepts, Go idioms | Proven architecture, native feel |
| 14 | Collection execution | Async goroutine-per-request | ~5µs overhead vs previous ms |
| 15 | Memory stats | runtime/metrics (no STW) | Eliminates global pauses |
| 16 | Config caching | Cache at construction + Refresh() | Zero per-request I/O |
| 17 | Graceful shutdown | sync.WaitGroup + context timeout | No data loss on exit |
| 18 | OTel isolation | Trace-ID-correlated capture | No cross-request span bleed |

# Requirements: HTTP Client Collector

## Overview

A new collector module (`collector/http`) for `go-profiler` that captures outbound HTTP calls made during a profiled request, providing visibility into service-to-service communication with timing, status codes, request/response details, and analysis.

## Functional Requirements

### FR-1: Outbound HTTP Call Capture

The collector MUST capture outbound HTTP calls via an `http.RoundTripper` wrapper that records:
- HTTP method
- Full URL
- Status code
- Duration (milliseconds, microsecond precision)
- Request size (bytes)
- Response size (bytes)
- Timestamp of the call
- Transport errors (timeouts, DNS failures, connection refused)

**Acceptance Criteria:**
- Each call through a `ProfilingRoundTripper` is recorded as an `HTTPCallEntry`
- Calls are indexed sequentially per request (starting at 0)
- Transport errors are captured with the error message; status code remains 0

### FR-2: Per-Service Naming

Each `ProfilingRoundTripper` instance MUST be tagged with a service name (e.g., "product-api", "inventory-service") for grouping calls in the UI.

**Acceptance Criteria:**
- `NewTransport(serviceName, base, options...)` requires a non-empty service name
- The service name is stored in every `HTTPCallEntry.Service` field
- Summary statistics include per-service call counts

### FR-3: Header Capture with Redaction

The collector MUST optionally capture request and response headers, with configurable redaction of sensitive headers.

**Acceptance Criteria:**
- Header capture is enabled by default (`WithHeaderCapture(true)`)
- Default redacted headers: `Authorization`, `Cookie`, `Set-Cookie`
- Redacted headers show `[REDACTED]` as the value
- Additional headers can be redacted via `WithRedactHeaders(headers...)`
- When disabled, no headers are stored

### FR-4: Body Capture

The collector MUST optionally capture request and response bodies with a configurable max size limit.

**Acceptance Criteria:**
- Body capture is disabled by default (`WithBodyCapture(false)`)
- Max body size defaults to 64KB (`WithMaxBodySize(n)`)
- Bodies exceeding max size are truncated with a `[truncated]` indicator
- Response body capture uses `io.TeeReader` — the caller can still read the full response body
- Request body is captured without consuming it (re-readable by downstream)

### FR-5: Error Capture

The collector MUST capture transport-level errors that prevent a response from being received.

**Acceptance Criteria:**
- Errors from `RoundTrip()` (timeouts, DNS, connection refused) are stored in `HTTPCallEntry.Error`
- Status code is 0 when no response was received
- The original error is returned to the caller unmodified

### FR-6: cURL Command Generation

The collector MUST generate a copy-paste cURL command for each captured call.

**Acceptance Criteria:**
- Enabled by default (`WithCurlGeneration(true)`)
- Generates valid shell-safe cURL with proper quoting
- Includes method, URL, headers (respecting redaction), and body
- Can be disabled to reduce profile size

### FR-7: Backtrace Capture

The collector MUST optionally capture the call stack showing where each HTTP call originated.

**Acceptance Criteria:**
- Disabled by default; enabled via `WithBacktrace(true)` or `HTTP_PROFILER_BACKTRACE=true` env var
- Filters out runtime internals, net/http internals, and the collector's own frames
- Limited to 10 meaningful frames
- Each frame formatted as `"file:line function"`

### FR-8: Slow Call Detection

The collector MUST flag calls exceeding a configurable duration threshold.

**Acceptance Criteria:**
- Default threshold: 500ms (`WithSlowThreshold(d)`)
- Calls with `DurationMs >= threshold` appear in `AnalysisResult.SlowCalls`
- Summary includes `SlowCount`

### FR-9: Failed Call Detection

The collector MUST identify calls with non-2xx status codes or transport errors.

**Acceptance Criteria:**
- Calls with status code outside 200-299 range (when status > 0) are "failed"
- Calls with a non-empty `Error` field are "failed"
- Failed calls appear in `AnalysisResult.FailedCalls`
- Summary includes `FailedCount`

### FR-10: Duplicate Call Detection

The collector MUST detect repeated identical requests within the same profiled request.

**Acceptance Criteria:**
- Duplicate = same method + same URL + same body hash (SHA-256)
- Groups with count > 1 appear in `AnalysisResult.DuplicateCalls`
- Each `DuplicateGroup` includes method, URL, count, and indices of the duplicate calls
- Enabled by default (`WithDuplicateDetection(true)`)
- Summary includes `DuplicateCount`

### FR-11: Summary Statistics

The collector MUST produce aggregate statistics for all captured calls.

**Acceptance Criteria:**
- `Summary` includes: `TotalCalls`, `TotalDurationMs`, `CallsPerService`, `FailedCount`, `SlowCount`, `DuplicateCount`, `SlowestCall`
- `SlowestCall` is the entry with the highest `DurationMs` (nil if no calls)

### FR-12: No-op When Not Profiling

When the profiler is disabled or the request is not being profiled, the transport MUST pass through with negligible overhead.

**Acceptance Criteria:**
- If context has no tracker (nil value from context lookup), `RoundTrip` delegates directly to the base transport
- No allocations or mutex operations on the non-profiling path
- Overhead is a single context value lookup (~10ns)

### FR-13: Thread Safety

The collector MUST safely handle concurrent outbound calls from the same request (e.g., fan-out patterns).

**Acceptance Criteria:**
- The per-request tracker uses `sync.Mutex` to protect the calls slice
- Concurrent `RoundTrip` calls from goroutines sharing the same context do not race
- Passes `go test -race` with concurrent access patterns

## Non-Functional Requirements

### NFR-1: Performance Overhead When Profiling

The collector MUST add less than 50µs per call when profiling is active (excluding body capture I/O).

**Acceptance Criteria:**
- Benchmark test demonstrates per-call overhead < 50µs for a no-body, no-backtrace call
- Body capture overhead is proportional to body size only

### NFR-2: Performance Overhead When Not Profiling

The collector MUST add less than 50ns overhead when profiling is disabled.

**Acceptance Criteria:**
- Benchmark test demonstrates ~10ns overhead (single context value lookup)
- No allocations on the non-profiling path

### NFR-3: Response Integrity

The transport MUST NOT alter the response body or close it prematurely.

**Acceptance Criteria:**
- Response body is fully readable by the caller after capture
- `io.TeeReader` buffers without consuming
- Content-Length and transfer encoding are unmodified
- The caller is responsible for closing the response body (as usual)

### NFR-4: Composability

The transport MUST work when stacked with other `http.RoundTripper` implementations.

**Acceptance Criteria:**
- Works with `otelhttp.NewTransport()` as the base
- Works when wrapped by retry libraries (each retry is a separate captured call)
- Works with any custom `RoundTripper` implementation
- Stacking example: `httpcollector.NewTransport("svc", otelhttp.NewTransport(http.DefaultTransport))`

### NFR-5: No External Dependencies

The module MUST have no dependencies beyond the Go standard library and `go-profiler/collector`.

**Acceptance Criteria:**
- `go.mod` requires only `github.com/piotrkardasz/go-profiler`
- All functionality uses stdlib packages (`net/http`, `io`, `sync`, `context`, `crypto/sha256`, `runtime`, `time`)

### NFR-6: Go Version Compatibility

The module MUST require Go 1.26.1 (matching the parent project).

**Acceptance Criteria:**
- `go.mod` declares `go 1.26.1`

## Interface Requirements

### IR-1: Collector Interface Compliance

The collector MUST implement `collector.Collector`, `collector.ContextSetup`, and `collector.PanelProvider`.

**Acceptance Criteria:**
- `Name()` returns `"http"`
- `Collect(ctx, req, res)` returns `HTTPData` (JSON-serializable)
- `Reset()` is a no-op (all state is in context)
- `SetupContext(ctx)` injects per-request tracker into context
- `PanelMeta()` returns metadata with name "http", label "HTTP Clients", icon "world", component "HttpPanel"

### IR-2: Transport Interface Compliance

The profiling transport MUST implement `http.RoundTripper`.

**Acceptance Criteria:**
- `NewTransport()` returns an `http.RoundTripper`
- Satisfies the `RoundTrip(*http.Request) (*http.Response, error)` contract
- Does not modify the request (per `RoundTripper` contract)
- Callers must not mutate the request after calling `RoundTrip`

### IR-3: Module Structure

The collector MUST be a separate Go module at `collector/http/`.

**Acceptance Criteria:**
- Module path: `github.com/piotrkardasz/go-profiler/collector/http`
- Contains `go.mod` with `replace github.com/piotrkardasz/go-profiler => ../..`
- Importable independently without pulling in GORM or OTel dependencies

## UI Requirements

### UIR-1: Panel Registration

The HTTP panel MUST be registered in the UI plugin system.

**Acceptance Criteria:**
- `registerPanel('http', HttpPanel)` added to `ui/src/plugin/builtin.ts`
- Falls back to `GenericPanel` (JSON tree) if `HttpPanel` component doesn't exist yet

### UIR-2: Panel Data Contract

The collector output MUST conform to the `HTTPData` JSON structure for the UI to consume.

**Acceptance Criteria:**
- Top-level structure: `{ calls: [], analysis: {}, summary: {} }`
- All fields use `json:"snake_case"` tags
- Optional fields use `omitempty` to reduce profile size

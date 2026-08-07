# Tasks: HTTP Client Collector

## Implementation Plan

Ordered implementation tasks for the `collector/http` module. Each task builds on the previous ones. Tasks are grouped by phase.

---

## Phase 1: Module Skeleton & Data Types

### Task 1: Create module skeleton

Create the `collector/http/` directory with `go.mod` and `go.sum`.

**Files:**
- `collector/http/go.mod`

**Details:**
- Module path: `github.com/piotrkardasz/go-profiler/collector/http`
- Go version: `go 1.26.1`
- Require: `github.com/piotrkardasz/go-profiler v0.0.0`
- Replace: `github.com/piotrkardasz/go-profiler => ../..`
- Run `go mod tidy` to generate `go.sum`

**Requirements:** NFR-5, NFR-6, IR-3

**Acceptance Criteria:**
- [ ] `collector/http/go.mod` exists with correct module path and replace directive
- [ ] `go mod tidy` succeeds in the `collector/http/` directory
- [ ] No external dependencies beyond the parent profiler module

---

### Task 2: Implement data types

Define all JSON-serializable structs for the collector output.

**Files:**
- `collector/http/entry.go`

**Details:**
- `HTTPCallEntry` — single captured call (index, service, method, URL, headers, body, size, status, duration, error, timestamp, backtrace, curl)
- `AnalysisResult` — slow/failed/duplicate analysis output
- `DuplicateGroup` — method, URL, count, indices
- `Summary` — aggregate stats (total calls, duration, per-service, failures, slow, duplicates, slowest)
- `HTTPData` — top-level struct: calls + analysis + summary
- All fields with `json:"snake_case"` tags, optional fields with `omitempty`

**Requirements:** UIR-2

**Acceptance Criteria:**
- [ ] All structs defined with proper JSON tags
- [ ] `HTTPData` is the top-level type returned by `Collect()`
- [ ] File compiles cleanly (`go build ./...`)

---

### Task 3: Implement context tracking

Context key and per-request call accumulation with thread safety.

**Files:**
- `collector/http/context.go`

**Details:**
- Unexported context key type (`contextKeyType struct{}`)
- `requestCalls` struct: `sync.Mutex`, `[]HTTPCallEntry`, `index int`
- `WithContext(ctx) context.Context` — injects fresh `*requestCalls`, idempotent (returns unchanged if already set)
- `callsFromContext(ctx) *requestCalls` — unexported, returns nil if not set (used by transport)
- `CallsFromContext(ctx) []HTTPCallEntry` — exported, returns thread-safe copy of calls slice
- `appendCall(ctx, entry)` — unexported helper that acquires mutex, sets index, appends

**Requirements:** FR-12, FR-13, IR-1

**Acceptance Criteria:**
- [ ] `WithContext` is idempotent — calling twice returns same context
- [ ] `callsFromContext` returns nil when no tracker is set
- [ ] `appendCall` is safe for concurrent use (mutex-protected)
- [ ] `CallsFromContext` returns a copy, not a reference to the internal slice

---

### Task 4: Implement options

Functional options pattern with environment variable fallbacks.

**Files:**
- `collector/http/options.go`

**Details:**
- `options` struct with all configurable fields
- `Option` type: `func(*options)`
- Option constructors:
  - `WithSlowThreshold(d time.Duration)` — default 500ms
  - `WithBodyCapture(enabled bool)` — default false
  - `WithMaxBodySize(n int)` — default 64KB (65536)
  - `WithHeaderCapture(enabled bool)` — default true
  - `WithRedactHeaders(headers ...string)` — default: Authorization, Cookie, Set-Cookie
  - `WithBacktrace(enabled bool)` — env fallback: `HTTP_PROFILER_BACKTRACE`
  - `WithDuplicateDetection(enabled bool)` — default true
  - `WithCurlGeneration(enabled bool)` — default true
- `defaultOptions()` function with safe defaults then env var overrides
- Programmatic options take highest precedence (same pattern as `collector/request_options.go`)

**Requirements:** FR-3, FR-4, FR-6, FR-7, FR-8, FR-10

**Acceptance Criteria:**
- [ ] All option constructors defined and functional
- [ ] `defaultOptions()` returns sensible defaults
- [ ] Env var `HTTP_PROFILER_BACKTRACE=true` enables backtrace when no explicit option is set
- [ ] Options are applied in order (last write wins for each field)

---

## Phase 2: Core Transport

### Task 5: Implement ProfilingRoundTripper

The core interception logic that captures outbound HTTP calls.

**Files:**
- `collector/http/transport.go`

**Details:**
- `profilingTransport` struct: `serviceName string`, `base http.RoundTripper`, `opts *options`
- `NewTransport(serviceName string, base http.RoundTripper, options ...Option) http.RoundTripper`
  - If `base` is nil, use `http.DefaultTransport`
  - Panics or returns error if `serviceName` is empty
- `RoundTrip(req *http.Request) (*http.Response, error)`:
  1. Get `*requestCalls` from `req.Context()` — if nil, delegate directly to `base.RoundTrip(req)` (no-op path)
  2. Record start time
  3. Capture request details (method, URL, headers if enabled, body if enabled)
  4. Call `base.RoundTrip(req)`
  5. Record duration
  6. Capture response details (status, headers, body via TeeReader if enabled, size)
  7. Capture error if `RoundTrip` failed
  8. Generate cURL command if enabled
  9. Capture backtrace if enabled
  10. Append entry to tracker via `appendCall`
  11. Return original response and error to caller

**Requirements:** FR-1, FR-2, FR-5, FR-12, FR-13, NFR-1, NFR-2, NFR-3, NFR-4, IR-2

**Acceptance Criteria:**
- [ ] Implements `http.RoundTripper` interface
- [ ] No-op path when context has no tracker (single nil check)
- [ ] Request is not modified (per RoundTripper contract)
- [ ] Original response and error are returned unmodified to caller
- [ ] Duration is calculated correctly (end - start)
- [ ] Concurrent calls from same context don't race (verified with `-race`)
- [ ] Works when stacked with other transports

---

### Task 6: Implement body capture in transport

Safe request and response body capture without consuming bodies.

**Files:**
- `collector/http/transport.go` (extend Task 5 implementation)

**Details:**
- **Request body capture:**
  - If `req.Body != nil` and `req.Body != http.NoBody`, read up to `MaxBodySize` bytes
  - Replace `req.Body` with a new reader that replays the buffered bytes + remainder (use `io.MultiReader`)
  - If body exceeds max size, store truncated content with `[truncated]` suffix
  - Store captured body string and request size in entry
- **Response body capture:**
  - If `resp.Body != nil`, wrap with a capturing reader:
    - Use a `bytes.Buffer` (capped at `MaxBodySize`) as the tee target
    - Replace `resp.Body` with a wrapper that tees to the buffer while the caller reads
    - On close, store the buffered content in the entry
  - Alternatively: read up to MaxBodySize, then reconstruct body with `io.NopCloser(io.MultiReader(buffered, originalRemainder))`
  - If body exceeds max size, note `[truncated]`
- **Size tracking:** `RequestSize` from Content-Length or captured bytes; `ResponseSize` from Content-Length header or bytes read

**Requirements:** FR-4, NFR-3

**Acceptance Criteria:**
- [ ] Request body is fully readable by the base transport after capture
- [ ] Response body is fully readable by the caller after capture
- [ ] Bodies exceeding `MaxBodySize` are truncated with indicator
- [ ] No body capture occurs when `WithBodyCapture(false)` (default)
- [ ] `resp.Body.Close()` still works correctly after wrapping

---

## Phase 3: Analysis & Utilities

### Task 7: Implement cURL command generation

Generate shell-safe cURL commands for captured outbound calls.

**Files:**
- `collector/http/curl.go`

**Details:**
- `buildCurlCommand(entry *HTTPCallEntry, opts *options) string`
- Start with `curl` (or `curl -X METHOD` for non-GET/HEAD)
- Append URL single-quoted with proper escaping
- Append headers alphabetically, excluding transport-level headers (Content-Length, Host, Accept-Encoding, Connection, Transfer-Encoding)
- Respect header redaction (don't include redacted headers in curl, or include them as `[REDACTED]`)
- Append body as `-d 'body'` if present
- Use `escapeSingleQuotes` helper (replace `'` with `'\''`)
- Follow the same patterns as `collector/request_curl.go`

**Requirements:** FR-6

**Acceptance Criteria:**
- [ ] Generates valid, copy-paste-able cURL commands
- [ ] Special characters in URLs and bodies are properly shell-escaped
- [ ] Redacted headers are excluded or shown as redacted
- [ ] Non-GET methods include `-X METHOD`
- [ ] Body included with `-d` flag when present

---

### Task 8: Implement backtrace capture

Capture call stacks for HTTP calls, filtering internal frames.

**Files:**
- `collector/http/backtrace.go`

**Details:**
- `captureBacktrace() []string`
- Use `runtime.Callers(skip, pcs)` with appropriate skip count
- Iterate with `runtime.CallersFrames`
- Filter out:
  - `runtime.*` frames
  - `net/http.*` frames (transport internals)
  - `go-profiler/collector/http` frames (the collector itself)
- Keep up to 10 meaningful frames
- Format: `"file:line function"` (strip module prefix for readability)
- Follow the pattern from `collector/logger_backtrace.go`

**Requirements:** FR-7

**Acceptance Criteria:**
- [ ] Returns up to 10 frames of user application code
- [ ] Filters runtime, net/http, and collector internals
- [ ] Returns empty slice when backtrace is disabled
- [ ] Each frame is formatted as `"file:line function"`

---

### Task 9: Implement analysis (slow, failed, duplicates)

Analysis engine that processes captured calls and classifies them.

**Files:**
- `collector/http/analysis.go`

**Details:**
- `analyze(calls []HTTPCallEntry, opts *options) AnalysisResult`
- **`detectSlow(calls, threshold)`**: Return entries where `DurationMs >= threshold.Milliseconds()`
- **`detectFailed(calls)`**: Return entries where `StatusCode < 200 || StatusCode >= 300` (when StatusCode > 0) or `Error != ""`
- **`detectDuplicates(calls)`**: Hash `method + URL + bodyHash(SHA-256)` → group by hash → return groups with count > 1
- **`buildSummary(calls, analysis, opts)`**: Compute TotalCalls, TotalDurationMs, CallsPerService map, FailedCount, SlowCount, DuplicateCount, SlowestCall
- Follow the pattern from `collector/gorm/analysis.go`

**Requirements:** FR-8, FR-9, FR-10, FR-11

**Acceptance Criteria:**
- [ ] Slow calls correctly identified against threshold
- [ ] Failed calls include both non-2xx and transport errors
- [ ] Duplicates detected by method + URL + body hash
- [ ] Summary statistics are accurate
- [ ] Empty calls slice produces zero-value summary (no panics)
- [ ] Duplicate detection skippable via `WithDuplicateDetection(false)`

---

## Phase 4: Collector Integration

### Task 10: Implement the Collector struct

Wire everything together into the main collector that integrates with the profiler.

**Files:**
- `collector/http/collector.go`

**Details:**
- `Collector` struct with `opts *options`
- `New(options ...Option) *Collector` — applies options, returns collector
- `Name() string` — returns `"http"`
- `Reset()` — no-op (state is in context)
- `SetupContext(ctx context.Context) context.Context` — delegates to `WithContext(ctx)`
- `Collect(ctx context.Context, req *http.Request, res collector.ResponseData) (any, error)`:
  1. Get calls from context via `CallsFromContext(ctx)`
  2. If no calls, return empty `HTTPData` with zero summary
  3. Run analysis: `analyze(calls, c.opts)`
  4. Build summary: `buildSummary(calls, analysis, c.opts)`
  5. Return `HTTPData{Calls: calls, Analysis: analysis, Summary: summary}`
- `PanelMeta() collector.PanelMeta` — returns name "http", label "HTTP Clients", icon "world", component "HttpPanel"

**Requirements:** IR-1, FR-11

**Acceptance Criteria:**
- [ ] Implements `collector.Collector` interface
- [ ] Implements `collector.ContextSetup` interface
- [ ] Implements `collector.PanelProvider` interface
- [ ] `Collect` returns valid `HTTPData` JSON structure
- [ ] Returns empty data gracefully when no calls were captured
- [ ] Integrates with profiler via `p.AddCollector(httpCollector)`

---

## Phase 5: Testing

### Task 11: Write unit tests for context tracking

**Files:**
- `collector/http/context_test.go`

**Details:**
- Test `WithContext` initializes tracker
- Test `WithContext` idempotency
- Test `callsFromContext` returns nil for bare context
- Test `CallsFromContext` returns copy (mutating copy doesn't affect internal state)
- Test `appendCall` concurrent safety with goroutines + `sync.WaitGroup`
- Run with `-race` flag

**Requirements:** FR-12, FR-13

**Acceptance Criteria:**
- [ ] All context operations tested
- [ ] Concurrent append test passes with `-race`
- [ ] 100% coverage of context.go

---

### Task 12: Write unit tests for transport

**Files:**
- `collector/http/transport_test.go`

**Details:**
- Test no-op path (no context tracker) — verifies base transport is called, no recording
- Test basic call capture (method, URL, status, duration)
- Test header capture and redaction
- Test body capture (request + response)
- Test body truncation at MaxBodySize
- Test transport error capture (use a failing RoundTripper mock)
- Test concurrent calls from same context
- Test nil base transport defaults to `http.DefaultTransport`
- Test service name is recorded
- Use `httptest.NewServer` for integration-style tests
- Benchmark: `BenchmarkRoundTripNoProfile` and `BenchmarkRoundTripWithProfile`

**Requirements:** FR-1, FR-2, FR-3, FR-4, FR-5, NFR-1, NFR-2, NFR-3

**Acceptance Criteria:**
- [ ] All transport paths tested (profiling on/off, success/error, with/without body)
- [ ] Benchmark proves < 50µs overhead when profiling
- [ ] Benchmark proves < 50ns overhead when not profiling
- [ ] Passes `go test -race`

---

### Task 13: Write unit tests for analysis

**Files:**
- `collector/http/analysis_test.go`

**Details:**
- Test slow call detection with various thresholds
- Test failed call detection (non-2xx status, transport errors)
- Test duplicate detection (same method+URL+body, different calls)
- Test duplicate detection with different bodies (no false positives)
- Test summary computation
- Test empty calls list (no panics, zero-value output)
- Test disabled duplicate detection

**Requirements:** FR-8, FR-9, FR-10, FR-11

**Acceptance Criteria:**
- [ ] Each analysis function tested independently
- [ ] Edge cases covered (empty input, single call, all duplicates)
- [ ] Summary math is verified

---

### Task 14: Write unit tests for curl and options

**Files:**
- `collector/http/curl_test.go`
- `collector/http/options_test.go`

**Details:**
- **curl tests:** GET/POST/PUT requests, special characters in URL/body, header inclusion, redacted headers, empty body
- **options tests:** default values, each option constructor, env var fallback for backtrace, option override precedence

**Requirements:** FR-6, FR-3, FR-7

**Acceptance Criteria:**
- [ ] cURL output validated against expected strings
- [ ] Shell-unsafe characters properly escaped
- [ ] Default options match spec (500ms threshold, 64KB body, headers on, body off)
- [ ] Env var override works for backtrace

---

### Task 15: Write integration example test

**Files:**
- `collector/http/example_test.go`

**Details:**
- Runnable `Example` function demonstrating full setup:
  - Create profiler with HTTP collector
  - Create instrumented HTTP client
  - Make a request through the profiled middleware
  - Show captured data in output
- Uses `httptest.NewServer` for both the "downstream service" and the profiled app
- Demonstrates multiple services, concurrent calls, and analysis output

**Requirements:** All

**Acceptance Criteria:**
- [ ] Example compiles and runs (`go test -run Example`)
- [ ] Demonstrates realistic usage pattern
- [ ] Shows integration with the profiler middleware

---

## Phase 6: Documentation & UI

### Task 16: Register HttpPanel in UI

Add the panel registration to the UI plugin system.

**Files:**
- `ui/src/plugin/builtin.ts` (modify)
- `ui/src/components/panels/HttpPanel.vue` (create — basic version)

**Details:**
- Add `import HttpPanel from '../components/panels/HttpPanel.vue'` to builtin.ts
- Add `registerPanel('http', HttpPanel)` in `initBuiltinPanels()`
- Create a basic `HttpPanel.vue` that renders the calls table:
  - Columns: index, service, method, URL (truncated), status badge (color-coded), duration bar
  - Expandable row detail: headers, body, cURL command, backtrace
  - Summary section at top with key stats
  - Analysis badges (slow=yellow, failed=red, duplicate=orange)
- Falls back to GenericPanel if component is removed

**Requirements:** UIR-1, UIR-2

**Acceptance Criteria:**
- [ ] Panel registered in builtin.ts
- [ ] HttpPanel.vue renders call list with status badges
- [ ] Expandable detail shows headers, body, cURL
- [ ] Summary stats displayed
- [ ] Build succeeds (`npm run build` in ui/)

---

### Task 17: Add example application

Create a working example demonstrating the HTTP client collector.

**Files:**
- `examples/http-clients/main.go`
- `examples/http-clients/.env`

**Details:**
- Simple HTTP server with a handler that calls 2-3 mock downstream services
- Uses `httptest.NewServer` for mock downstream endpoints
- Shows: profiler setup, collector registration, transport wrapping, making calls
- Demonstrates: slow call (add artificial delay), failed call (return 500), duplicate call

**Requirements:** All FR

**Acceptance Criteria:**
- [ ] Example compiles and runs (`go run .`)
- [ ] Profiles are stored in `./var/profiler/`
- [ ] Profile JSON contains HTTP collector data with calls, analysis, and summary

---

### Task 18: Update README

Add documentation for the HTTP client collector to the project README.

**Files:**
- `README.md` (modify)

**Details:**
- New section "HTTP Client Collector" under collectors
- Quick start: install, configure, instrument HTTP clients
- Options reference table
- Example output snippet
- Link to `examples/http-clients/`

**Requirements:** Documentation

**Acceptance Criteria:**
- [ ] README documents the HTTP client collector
- [ ] Installation and usage instructions are clear
- [ ] Options are listed with defaults

---

## Verification Checklist

After all tasks are complete:

- [ ] `cd collector/http && go build ./...` succeeds
- [ ] `cd collector/http && go test ./... -race` passes
- [ ] `cd collector/http && go vet ./...` clean
- [ ] `cd . && go build ./...` (root module) still builds
- [ ] `cd examples/http-clients && go run .` works
- [ ] `cd ui && npm run build` succeeds
- [ ] Profile JSON matches the `HTTPData` schema from design.md
- [ ] No-profiling benchmark < 50ns/op
- [ ] Profiling benchmark < 50µs/op

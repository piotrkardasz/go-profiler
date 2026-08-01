# Tasks: Async Profiler Collection

## Implementation Tasks

### Task 1: Restructure middleware to defer CollectProfile to goroutine

**Objective:** Move all `CollectProfile()` execution off the HTTP request hot path into the existing async goroutine.

**Implementation:**
- Capture `r.Method`, `r.URL.String()`, `wrapped.statusCode` as local variables immediately after handler returns
- Clone response headers with `wrapped.Header().Clone()` before spawning goroutine (connection reuse safety)
- Build `ResponseData` struct synchronously from captured values
- Move `p.CollectProfile(ctx, r, resData)` and profile field assignment into the goroutine
- Keep `time.Since(startTime)` synchronous (accurate duration measurement)
- Ensure the goroutine receives only safe references: `ctx`, `r`, value-type locals, cloned headers

**Files to modify:**
- `middleware.go`

**Acceptance criteria:**
- `go build ./...` succeeds
- `X-Profiler-Id` header is still set synchronously before response
- Profile data is still stored correctly (verified by existing tests with timing tolerance)
- No data races under `go test -race ./...`

---

### Task 2: Add inflight tracking and Shutdown method

**Objective:** Track in-flight async collection goroutines and support graceful shutdown.

**Implementation:**
- Add `inflight sync.WaitGroup` field to `Profiler` struct
- Add `shutdown atomic.Bool` field to `Profiler` struct
- Call `p.inflight.Add(1)` before spawning collection goroutine in middleware
- Call `defer p.inflight.Done()` at start of collection goroutine
- Add `defer` + `recover` in goroutine to prevent panics from crashing the process
- Implement `Shutdown(ctx context.Context) error`:
  - Set `p.shutdown.Store(true)`
  - Spawn goroutine that calls `p.inflight.Wait()` and signals done channel
  - Select on done channel vs `ctx.Done()` for timeout
  - Return `nil` on clean shutdown, `ctx.Err()` on timeout
- In middleware: if `p.shutdown.Load()` is true, skip profiling (handler still executes normally)

**Files to modify:**
- `profiler.go` — add fields, `Shutdown()` method, helper methods `inflightAdd()`, `inflightDone()`
- `middleware.go` — add inflight tracking calls, shutdown check

**Acceptance criteria:**
- `Shutdown(ctx)` blocks until all in-flight profiles are stored
- `Shutdown` with expired context returns `context.DeadlineExceeded`
- After shutdown, new requests are served without profiling
- No goroutine leaks in tests

---

### Task 3: Replace runtime.ReadMemStats with runtime/metrics in MemoryCollector

**Objective:** Eliminate stop-the-world pauses by using the `runtime/metrics` package for memory statistics.

**Implementation:**
- Create `collector/memory_metrics.go` with helper types and functions:
  - `memorySnapshot` struct with fields for all needed metrics
  - Pre-allocated `[]metrics.Sample` slice with metric names
  - `captureMemorySnapshot() memorySnapshot` function that calls `metrics.Read()`
  - Helper to extract uint64 values from `metrics.Value`
- Modify `WithMemoryStats(ctx)` in `collector/memory.go`:
  - Replace `runtime.ReadMemStats(&stats)` with `captureMemorySnapshot()`
  - Store `memorySnapshot` in context instead of `*runtime.MemStats`
- Modify `MemoryCollector.Collect()`:
  - Replace `runtime.ReadMemStats(&afterStats)` with `captureMemorySnapshot()`
  - Replace `runtime.NumGoroutine()` with goroutine count from metrics
  - Map new snapshot fields to `MemoryData` struct fields
  - Maintain same JSON output field names for UI compatibility
- Update `MemoryStatsFromContext()` to return `memorySnapshot` instead of `*runtime.MemStats`

**Metric mapping:**
| MemoryData field | runtime/metrics key |
|-----------------|-------------------|
| Alloc (before/after) | `/memory/classes/heap/objects:bytes` |
| TotalAlloc | `/gc/heap/allocs:bytes` |
| HeapAlloc | `/memory/classes/heap/objects:bytes` |
| HeapInuse | `/memory/classes/heap/objects:bytes` + `/memory/classes/heap/unused:bytes` |
| HeapObjects | `/gc/heap/objects:objects` |
| NumGC | `/gc/cycles/total:gc-cycles` |
| Sys | `/memory/classes/total:bytes` |
| GoroutineCount | `/sched/goroutines:goroutines` |

**Files to create:**
- `collector/memory_metrics.go`

**Files to modify:**
- `collector/memory.go`

**Acceptance criteria:**
- No `runtime.ReadMemStats` calls remain in the codebase
- `go test ./collector/...` passes
- `MemoryData` JSON output has same field names (UI compatibility)
- No stop-the-world pauses from memory collection (verified by absence of ReadMemStats)
- `go test -race ./...` passes

---

### Task 4: Cache ConfigCollector reader results

**Objective:** Eliminate per-request file I/O and `os.Environ()` calls by caching config data at construction time.

**Implementation:**
- Add `mu sync.RWMutex` and `cachedSources []ConfigSource` fields to `ConfigCollector`
- Add internal `readAllSources() []ConfigSource` method:
  - Iterates all readers, calls `Read()`, populates Source field, applies masking
  - Returns assembled `[]ConfigSource`
- Call `readAllSources()` in `NewConfigCollector()` to populate initial cache
- Modify `Collect()` to read cached sources under `RLock` instead of calling readers
- Add public `Refresh()` method:
  - Acquires write lock
  - Re-runs `readAllSources()`
  - Updates `cachedSources`
- Ensure `Reset()` remains a no-op (cache is process-lifetime data, not per-request)

**Files to modify:**
- `collector/config.go`

**Acceptance criteria:**
- `ConfigCollector.Collect()` performs zero file I/O and zero `os.Environ()` calls
- `Refresh()` re-reads all sources and updates the cache
- Existing `config_test.go` tests pass
- Thread-safe under concurrent access (`go test -race`)

---

### Task 5: Add SampleRate configuration and sampling logic

**Objective:** Allow probabilistic request sampling to skip profiling entirely for a configurable fraction of requests.

**Implementation:**
- Add `SampleRate float64` field to `Config` struct (default: 1.0)
- Update `DefaultConfig()` to set `SampleRate: 1.0`
- In `Middleware()`, add sampling check immediately after enabled/route-prefix checks:
  ```go
  if cfg.SampleRate < 1.0 && rand.Float64() >= cfg.SampleRate {
      next.ServeHTTP(w, r)
      return
  }
  ```
- Import `math/rand/v2` (or `math/rand` for Go 1.21 compat) for `Float64()`
- Ensure sampling decision happens BEFORE:
  - `GenerateProfileID()`
  - `ResetCollectors()`
  - `WithMemoryStats()`
  - `ContextSetup`

**Files to modify:**
- `profiler.go` — add `SampleRate` to `Config`, update `DefaultConfig()`
- `middleware.go` — add sampling check early in middleware

**Acceptance criteria:**
- `SampleRate: 1.0` profiles all requests (no behavior change from current)
- `SampleRate: 0.0` profiles no requests
- `SampleRate: 0.5` profiles approximately 50% of requests (statistical test)
- Skipped requests do NOT have `X-Profiler-Id` header
- Skipped requests have zero profiler overhead beyond the sampling check
- `go test -race ./...` passes

---

### Task 6: Update existing tests for async behavior

**Objective:** Ensure all existing middleware and profiler tests pass with the new async collection model.

**Implementation:**
- Review `middleware_test.go` — tests that check stored profiles need timing tolerance:
  - Increase `time.Sleep` in `TestMiddlewareStoresProfile` to allow async goroutine completion
  - Or use polling with timeout instead of fixed sleep
- Add helper function `waitForProfile(store, id, timeout) (*Profile, error)` for tests
- Verify `TestMiddlewareSetsProfilerIDHeader` still passes (header set synchronously)
- Verify `TestMiddlewareDisabled` still passes (no profiling = no goroutine)
- Verify `TestMiddlewareSkipsProfilerRoutes` still passes
- Run `go test -race ./...` to verify no data races
- Update `TestMiddlewareCapturesStatusCode` and `TestMiddlewareDefaultStatusCode` with polling

**Files to modify:**
- `middleware_test.go`

**Acceptance criteria:**
- All existing tests pass with `go test ./...`
- No data races with `go test -race ./...`
- Tests are not flaky (use polling/retry rather than fixed sleeps where possible)

---

### Task 7: Write new tests for async-specific behavior

**Objective:** Add tests that verify the new async, shutdown, and sampling functionality.

**Implementation:**
- `TestMiddlewareAsyncDoesNotBlockResponse`: Verify that a slow collector does not delay the HTTP response
  - Register a collector with artificial `time.Sleep(100ms)` in `Collect()`
  - Measure response time — should be << 100ms
- `TestProfilerShutdown`: Verify graceful shutdown waits for in-flight profiles
  - Start profiling a request
  - Call `Shutdown(ctx)` with long timeout
  - Verify profile was stored after Shutdown returns
- `TestProfilerShutdownTimeout`: Verify shutdown respects context deadline
  - Register a collector with long sleep
  - Call `Shutdown` with very short timeout
  - Verify error is `context.DeadlineExceeded`
- `TestProfilerShutdownSkipsNewRequests`: After shutdown, new requests skip profiling
- `TestMiddlewareSampling100Percent`: SampleRate=1.0 profiles all requests
- `TestMiddlewareSampling0Percent`: SampleRate=0.0 profiles no requests
- `TestMiddlewareSamplingPartial`: SampleRate=0.5 profiles approximately half (statistical)
- `TestMiddlewareNoDataRaceUnderLoad`: Launch 100 concurrent requests, verify no races
- `TestMemoryCollectorNoSTW`: Verify `runtime.ReadMemStats` is not called (no STW markers in trace)
- `TestConfigCollectorCacheHit`: Verify Collect() does not call readers after construction
- `TestConfigCollectorRefresh`: Verify Refresh() re-reads sources

**Files to create:**
- `middleware_async_test.go`

**Files to modify:**
- `collector/memory_test.go` — update for new snapshot types
- `collector/config_test.go` — add cache/refresh tests

**Acceptance criteria:**
- All new tests pass
- `go test -race ./...` passes across entire project
- No flaky tests

---

### Task 8: Add panic recovery in collection goroutine

**Objective:** Ensure that a panicking collector does not crash the entire application.

**Implementation:**
- Add `defer` block at the top of the async goroutine in middleware:
  ```go
  go func() {
      defer p.inflightDone()
      defer func() {
          if r := recover(); r != nil {
              p.logger.Error("panic in profile collection",
                  "profile_id", profileID,
                  "panic", fmt.Sprintf("%v", r),
              )
          }
      }()
      // ... collection work ...
  }()
  ```
- Ensure the recover logs enough context to diagnose the issue (profile ID, panic value)
- Do NOT re-panic — the profile is simply lost

**Files to modify:**
- `middleware.go`

**Acceptance criteria:**
- A panicking collector does not crash the process
- The panic is logged with profile ID
- Other requests continue to be profiled normally
- `inflightDone()` is still called (WaitGroup doesn't leak)

---

### Task 9: Update memory collector tests

**Objective:** Update the memory collector test suite for the new `runtime/metrics` implementation.

**Implementation:**
- Update `TestMemoryCollectorCollect` to verify output fields are populated (values > 0)
- Update `TestMemoryCollectorDelta` to verify AllocBefore/AllocAfter/AllocDelta make sense
- Remove any tests that directly assert on `runtime.MemStats` struct fields
- Add `TestCaptureMemorySnapshot` to verify `captureMemorySnapshot()` returns valid data
- Verify `HeapAlloc > 0`, `Sys > 0`, `GoroutineCount > 0` in snapshot
- Test that `WithMemorySnapshot` / `MemorySnapshotFromContext` round-trips correctly

**Files to modify:**
- `collector/memory_test.go`

**Acceptance criteria:**
- All memory collector tests pass
- Tests verify behavior (metrics populated, delta computed) not implementation details

---

### Task 10: Final verification and benchmarks

**Objective:** Verify the complete implementation builds, passes tests, and demonstrates measurable performance improvement.

**Verification steps:**
- `go build ./...` — root module builds without errors
- `go test ./...` — all tests pass
- `go test -race ./...` — no data races
- `go vet ./...` — no warnings
- GORM collector module: `cd collector/gorm && go build ./...`
- Verify no `runtime.ReadMemStats` calls remain: `grep -r "ReadMemStats" collector/`
- Verify `runtime/metrics` is imported: `grep -r "runtime/metrics" collector/`
- Run basic example to verify end-to-end functionality

**Performance validation (optional benchmark):**
- Create `middleware_bench_test.go` with:
  ```go
  func BenchmarkMiddlewareOverhead(b *testing.B) {
      // Measure time added by middleware excluding handler
  }
  ```
- Compare before/after: expect <5µs per request vs current 200µs+

**Files to verify (no changes):**
- `collector/gorm/collector.go` — still builds
- `collector/otel/collector.go` — still builds
- `handler/api.go` — API still works
- `profile.go` — Profile struct unchanged
- `go.mod` — no new external dependencies

**Acceptance criteria:**
- Zero build errors
- Zero test failures
- Zero data races
- No new external dependencies in `go.mod`
- `grep -r "ReadMemStats" .` returns zero results (excluding git/vendor/test comments)
- Middleware benchmark shows <5µs overhead per request

---

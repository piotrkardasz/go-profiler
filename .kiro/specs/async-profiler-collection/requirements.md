# Requirements: Async Profiler Collection

## Overview

Refactor the profiler middleware to execute collectors asynchronously (off the HTTP request hot path) so that the profiling instrumentation does not add observable latency to the application's responses. Currently, all `Collector.Collect()` calls — including two stop-the-world `runtime.ReadMemStats` calls, file I/O, environment scanning, and GORM query analysis — run synchronously between the handler returning and the response being finalized. This adds 200µs–10ms of latency to every profiled request and causes global stop-the-world pauses affecting all goroutines in the process.

## Functional Requirements

### FR-1: Async Collection Execution

- FR-1.1: The middleware MUST execute all `Collector.Collect()` calls asynchronously in a goroutine, off the HTTP request's hot path.
- FR-1.2: The middleware MUST capture all data needed by collectors (context, request metadata, response data, timing) synchronously BEFORE spawning the async collection goroutine.
- FR-1.3: The existing `LateCollector.LateCollect()` and `Storage.Store()` calls MUST continue to run asynchronously (no regression).
- FR-1.4: The profiler MUST still set the `X-Profiler-Id` response header synchronously before the handler writes the response.
- FR-1.5: The profiler MUST still capture accurate request duration (`time.Since(startTime)`) synchronously immediately after the handler returns.
- FR-1.6: The `CollectProfile()` method MUST be moved into the same goroutine that currently handles `CollectLate()` and `Storage.Store()`.

### FR-2: Replace `runtime.ReadMemStats` with `runtime/metrics`

- FR-2.1: The `MemoryCollector` MUST replace `runtime.ReadMemStats()` with the `runtime/metrics` package to eliminate stop-the-world pauses.
- FR-2.2: The pre-handler memory snapshot (`WithMemoryStats`) MUST use `runtime/metrics` instead of `runtime.ReadMemStats`.
- FR-2.3: The post-handler memory collection (`MemoryCollector.Collect()`) MUST use `runtime/metrics` instead of `runtime.ReadMemStats`.
- FR-2.4: The `MemoryData` output structure MUST retain the same JSON field names and semantics for backward compatibility with the UI panel.
- FR-2.5: The following memory metrics MUST still be collected: heap allocation (before/after/delta), total allocation, heap in-use, heap objects, GC cycles, goroutine count, system memory.
- FR-2.6: If a metric is not available via `runtime/metrics` (e.g., exact field-for-field match), the collector MUST provide the closest equivalent and document the difference.

### FR-3: ConfigCollector Caching

- FR-3.1: The `ConfigCollector` MUST cache `.env` file contents and environment variables at construction time rather than re-reading on every request.
- FR-3.2: The `ConfigCollector` MUST provide a `Refresh()` method to manually invalidate the cache and re-read config sources.
- FR-3.3: The cached runtime info and build info behavior MUST remain unchanged (already cached).
- FR-3.4: The `ConfigCollector.Collect()` method MUST return cached data without performing file I/O or calling `os.Environ()` on each request.

### FR-4: Graceful Shutdown

- FR-4.1: The profiler MUST provide a `Shutdown(ctx context.Context) error` method that waits for all in-flight async collection goroutines to complete before returning.
- FR-4.2: The `Shutdown` method MUST respect the context deadline/cancellation for timeout behavior.
- FR-4.3: After `Shutdown` is called, new requests MUST still be served (handler executes normally) but profiling MAY be skipped for those requests.

### FR-5: Optional Sampling

- FR-5.1: The profiler config MUST support an optional `SampleRate float64` field (0.0 to 1.0, default 1.0 meaning profile all requests).
- FR-5.2: When `SampleRate` is less than 1.0, the middleware MUST skip profiling for a proportional fraction of requests (probabilistic sampling).
- FR-5.3: Skipped requests MUST NOT have the `X-Profiler-Id` header set.
- FR-5.4: Skipped requests MUST have zero profiler overhead beyond a single float comparison.
- FR-5.5: The sampling decision MUST be made before any expensive operations (ID generation, memory snapshots, context setup).

### FR-6: Backward Compatibility

- FR-6.1: The `Collector` interface MUST NOT change (no new methods required on existing collector implementations).
- FR-6.2: The `Profile` struct and its JSON serialization MUST remain unchanged.
- FR-6.3: The profiler API endpoints and UI MUST continue to work without modification.
- FR-6.4: The `CollectProfile()` method signature MUST remain unchanged (it's called from the goroutine now instead of inline, but the method itself doesn't change).
- FR-6.5: Existing tests MUST continue to pass (with minor timing adjustments for async behavior).

## Non-Functional Requirements

### NFR-1: Performance

- NFR-1.1: The synchronous overhead added to each request by the profiler middleware MUST be less than 5µs (excluding the handler execution itself).
- NFR-1.2: The profiler MUST NOT cause stop-the-world pauses in the Go runtime (no `runtime.ReadMemStats` in the hot path).
- NFR-1.3: The profiler MUST NOT perform file I/O in the synchronous request path.
- NFR-1.4: Memory allocations in the hot path SHOULD be minimized (use pre-allocated IDs or `sync.Pool` where practical).

### NFR-2: Resource Usage

- NFR-2.1: The async collection goroutines MUST complete promptly (within milliseconds, not accumulate indefinitely).
- NFR-2.2: Under normal load (≤1000 req/s), the goroutine count increase from profiling MUST be bounded and proportional to active requests.
- NFR-2.3: The profiler SHOULD NOT introduce unbounded memory growth from queued collection work.

### NFR-3: Correctness

- NFR-3.1: Profile data accuracy MUST be maintained — timing duration MUST reflect actual handler execution time, not include collection overhead.
- NFR-3.2: Memory deltas MUST still reflect the handler's memory impact (before/after relative to handler execution), with acceptable approximation from the `runtime/metrics` package.
- NFR-3.3: Request/response metadata (method, URL, status code, headers, body size) MUST be captured accurately.
- NFR-3.4: GORM query data MUST be complete — all queries executed during the handler MUST appear in the profile.

### NFR-4: Compatibility

- NFR-4.1: MUST work with Go 1.21+ (matching existing module requirement; `runtime/metrics` available since Go 1.16).
- NFR-4.2: MUST NOT break the GORM collector module or OTel collector.
- NFR-4.3: MUST NOT introduce new external dependencies (only standard library packages).

### NFR-5: Observability

- NFR-5.1: The profiler SHOULD log a warning if async collection goroutines are backing up (e.g., more than 100 in-flight).
- NFR-5.2: The profiler SHOULD log errors from failed collectors (existing behavior, maintained in async path).

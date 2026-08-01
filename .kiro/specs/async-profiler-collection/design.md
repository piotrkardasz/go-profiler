# Design: Async Profiler Collection

## Technical Design Document

### 1. System Architecture

The refactoring moves all expensive collector work off the HTTP request hot path into an async goroutine. The middleware's synchronous path is reduced to lightweight data capture (timestamps, response metadata, context references), while the actual `Collect()` calls, analysis, and storage happen asynchronously.

```
┌─────────────────────────────────────────────────────────────────────────┐
│ BEFORE (current — synchronous hot path)                                  │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  Request ──► [ReadMemStats STW] ──► [Handler] ──► [CollectProfile:      │
│                                                     ReadMemStats STW    │
│                                                     .env file I/O       │
│                                                     os.Environ()        │
│                                                     GORM analysis]      │
│              ──► Response returned to client                            │
│                                                                          │
│              ──► go { LateCollect + Store }                              │
│                                                                          │
│  Overhead: 200µs – 10ms per request (blocks response)                   │
└─────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────┐
│ AFTER (proposed — minimal synchronous path)                              │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  Request ──► [runtime/metrics snapshot: ~0.1µs] ──► [Handler]           │
│          ──► [capture endTime + ResponseData: ~0.01µs]                  │
│          ──► Response returned to client                                │
│                                                                          │
│          ──► go { CollectProfile + CollectLate + Store }                 │
│                  (all expensive work happens here, off hot path)         │
│                                                                          │
│  Overhead: ~1-2µs per request (does not block response)                 │
└─────────────────────────────────────────────────────────────────────────┘
```

### 2. File Changes

```
Modified files:
├── middleware.go              # Restructure to defer CollectProfile to goroutine
├── profiler.go                # Add Shutdown(), inflight tracking, SampleRate
├── collector/memory.go        # Replace ReadMemStats with runtime/metrics
├── collector/config.go        # Cache reader results, add Refresh()

New files:
├── collector/memory_metrics.go  # runtime/metrics helper for memory stats
```

### 3. Core Design Decisions

#### 3.1 Move CollectProfile into Existing Goroutine

**Decision:** Move the `CollectProfile()` call into the goroutine that already runs `CollectLate()` + `Storage.Store()`.

**Current middleware flow (lines 115-147):**
```go
// [SYNC] Execute handler
next.ServeHTTP(wrapped, r)

// [SYNC] Compute duration
duration := time.Since(startTime)

// [SYNC] Build response data
resData := collector.ResponseData{...}

// [SYNC] ⚠️ EXPENSIVE — runs all collectors synchronously
profile := p.CollectProfile(ctx, r, resData)
profile.ID = profileID
profile.Method = r.Method
profile.URL = r.URL.String()
profile.StatusCode = wrapped.statusCode
profile.Timestamp = startTime
profile.Duration = duration

// [ASYNC] Only late collect + store
go func() {
    lateData := p.CollectLate(ctx)
    // ...merge + store...
}()
```

**Proposed middleware flow:**
```go
// [SYNC] Execute handler
next.ServeHTTP(wrapped, r)

// [SYNC] Capture lightweight data immediately (before anything can be recycled)
duration := time.Since(startTime)
method := r.Method
url := r.URL.String()
statusCode := wrapped.statusCode
resData := collector.ResponseData{
    StatusCode: wrapped.statusCode,
    Headers:    wrapped.Header().Clone(),  // Clone headers before connection reuse
    Size:       wrapped.size,
}

// [ASYNC] ALL collection work deferred
p.inflightAdd()
go func() {
    defer p.inflightDone()

    // Run all collectors
    profile := p.CollectProfile(ctx, r, resData)
    profile.ID = profileID
    profile.Method = method
    profile.URL = url
    profile.StatusCode = statusCode
    profile.Timestamp = startTime
    profile.Duration = duration

    // Run late collectors
    lateData := p.CollectLate(ctx)
    for name, data := range lateData {
        profile.CollectorData[name] = data
    }

    // Persist
    if storage := p.Storage(); storage != nil {
        if err := storage.Store(profile); err != nil {
            p.logger.Error("failed to store profile", ...)
        }
    }
}()
```

**Rationale:**
- Simplest possible change — no new concurrency primitives, no channels, no worker pools.
- Already proven safe in this codebase (the existing goroutine does the same kind of work).
- Go runtime efficiently schedules millions of short-lived goroutines.
- For a development profiler, unbounded goroutine-per-request is perfectly acceptable.
- Can upgrade to worker pool later if needed (Phase 4 in REPORT.md).

#### 3.2 Header Cloning

**Decision:** Clone response headers before spawning the goroutine.

```go
resData := collector.ResponseData{
    StatusCode: wrapped.statusCode,
    Headers:    wrapped.Header().Clone(),  // IMPORTANT: clone before connection reuse
    Size:       wrapped.size,
}
```

**Rationale:**
- After `ServeHTTP` returns, the underlying `http.ResponseWriter` and its header map may be reused by the connection for the next request (HTTP/1.1 keep-alive).
- `http.Header.Clone()` creates a deep copy (~200ns for typical headers) that's safe to read from any goroutine.
- `wrapped.statusCode` and `wrapped.size` are value types on our custom struct — already safe.

#### 3.3 Replace `runtime.ReadMemStats` with `runtime/metrics`

**Decision:** Use the `runtime/metrics` package (Go 1.16+) which does NOT require stopping all goroutines.

**Metrics mapping:**

| MemoryData field | runtime.ReadMemStats field | runtime/metrics equivalent |
|-----------------|---------------------------|---------------------------|
| `AllocBefore/After` | `MemStats.Alloc` | `/memory/classes/heap/objects:bytes` |
| `TotalAlloc` | `MemStats.TotalAlloc` | `/gc/heap/allocs:bytes` (cumulative) |
| `HeapAlloc` | `MemStats.HeapAlloc` | `/memory/classes/heap/objects:bytes` |
| `HeapInuse` | `MemStats.HeapInuse` | `/memory/classes/heap/unused:bytes` + objects |
| `HeapObjects` | `MemStats.HeapObjects` | `/gc/heap/objects:objects` |
| `NumGC` | `MemStats.NumGC` | `/gc/cycles/total:gc-cycles` |
| `Sys` | `MemStats.Sys` | `/memory/classes/total:bytes` |
| `GoroutineCount` | N/A (uses `runtime.NumGoroutine()`) | `/sched/goroutines:goroutines` |

**Implementation approach:**
```go
package collector

import "runtime/metrics"

// memSamples are pre-allocated metric descriptors read once per collection.
var memSampleDescs = []metrics.Sample{
    {Name: "/memory/classes/heap/objects:bytes"},
    {Name: "/memory/classes/total:bytes"},
    {Name: "/gc/heap/allocs:bytes"},
    {Name: "/gc/heap/objects:objects"},
    {Name: "/gc/cycles/total:gc-cycles"},
    {Name: "/sched/goroutines:goroutines"},
    {Name: "/memory/classes/heap/unused:bytes"},
}

type memorySnapshot struct {
    heapObjects uint64
    totalAlloc  uint64
    heapInuse   uint64
    heapObjs    uint64
    numGC       uint64
    sys         uint64
    goroutines  uint64
}

func captureMemorySnapshot() memorySnapshot {
    samples := make([]metrics.Sample, len(memSampleDescs))
    copy(samples, memSampleDescs)
    metrics.Read(samples)
    // Extract values from samples...
    return memorySnapshot{...}
}
```

**Rationale:**
- `runtime.ReadMemStats` is documented to stop the world. On large heaps (GB+), this can take milliseconds and affects ALL goroutines.
- `runtime/metrics.Read()` does NOT stop the world — it reads atomically-maintained counters.
- DataDog (dd-trace-go) and Prometheus (client_golang) both migrated away from ReadMemStats for this reason.
- The metrics are slightly different (aggregated differently), but provide equivalent insight for profiling purposes.
- Available since Go 1.16, well within our Go 1.21+ requirement.

#### 3.4 ConfigCollector Caching

**Decision:** Cache `.env` file contents and environment variables at construction time. Provide `Refresh()` for manual invalidation.

**Current behavior:** Reads `.env` files and calls `os.Environ()` on every request.

**Proposed behavior:**
```go
type ConfigCollector struct {
    // Cached at construction (immutable during process lifetime)
    runtimeInfo  RuntimeInfo
    buildInfo    BuildInfo
    dependencies []DependencyInfo

    // Cached at construction, refreshable
    mu            sync.RWMutex
    cachedSources []ConfigSource

    // Configuration
    readers           []ConfigReader
    maskEnabled       bool
    sensitivePatterns []string
    buildInfoDisabled bool
}

func NewConfigCollector(opts ...ConfigOption) *ConfigCollector {
    c := &ConfigCollector{...}
    c.refreshCache() // Initial read
    return c
}

func (c *ConfigCollector) Collect(...) (any, error) {
    c.mu.RLock()
    sources := c.cachedSources
    c.mu.RUnlock()

    return &ConfigData{
        Runtime:      c.runtimeInfo,
        Build:        c.buildInfo,
        Dependencies: c.dependencies,
        Sources:      sources,
        MaskEnabled:  c.maskEnabled,
    }, nil
}

func (c *ConfigCollector) Refresh() {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.cachedSources = c.readAllSources()
}
```

**Rationale:**
- Environment variables and `.env` files virtually never change during a running process.
- Reading them on every request adds 50-500µs of unnecessary file I/O and allocation.
- `Refresh()` provides an escape hatch for the rare case where env vars change at runtime.
- Even after moving to async, caching eliminates unnecessary work in the goroutine.

#### 3.5 Inflight Tracking and Graceful Shutdown

**Decision:** Use `sync.WaitGroup` to track in-flight collection goroutines and support graceful shutdown.

```go
type Profiler struct {
    // ...existing fields...
    inflight sync.WaitGroup
    shutdown atomic.Bool
}

func (p *Profiler) inflightAdd() {
    p.inflight.Add(1)
}

func (p *Profiler) inflightDone() {
    p.inflight.Done()
}

func (p *Profiler) Shutdown(ctx context.Context) error {
    p.shutdown.Store(true)
    done := make(chan struct{})
    go func() {
        p.inflight.Wait()
        close(done)
    }()
    select {
    case <-done:
        return nil
    case <-ctx.Done():
        return ctx.Err()
    }
}
```

**Rationale:**
- Prevents data loss on application shutdown — all in-flight profiles are persisted.
- `sync.WaitGroup` is the idiomatic Go pattern for tracking concurrent work.
- `atomic.Bool` for shutdown flag is lock-free and safe for concurrent reads.
- Context-based timeout prevents hanging forever if a goroutine gets stuck.

#### 3.6 Sampling

**Decision:** Add probabilistic sampling with a `SampleRate` config field.

```go
type Config struct {
    // ...existing fields...

    // SampleRate controls what fraction of requests are profiled.
    // 1.0 (default) means profile all requests. 0.1 means profile 10%.
    // Set to < 1.0 for production use to reduce overhead.
    SampleRate float64
}
```

**Sampling in middleware (very early, before any expensive work):**
```go
func (p *Profiler) Middleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if !p.IsEnabled() {
            next.ServeHTTP(w, r)
            return
        }

        // Sampling decision (before any allocations)
        cfg := p.Config()
        if cfg.SampleRate < 1.0 {
            if fastrand() > cfg.SampleRate {
                next.ServeHTTP(w, r)
                return
            }
        }
        // ... rest of profiling logic ...
    })
}

// fastrand returns a float64 in [0, 1) using cheap PRNG.
// Uses runtime fastrand or math/rand/v2 for low overhead.
func fastrand() float64 {
    return rand.Float64()
}
```

**Rationale:**
- For development (default SampleRate=1.0), all requests are profiled — no behavior change.
- For production/staging, users can set SampleRate=0.1 to only profile 10% of requests.
- The sampling check is placed before ALL profiler work (ID generation, memory snapshot, context setup).
- Skipped requests have effectively zero overhead (single float comparison + branch).

### 4. Data Flow — Before vs After

#### Before (synchronous):
```
time─────────────────────────────────────────────────►
     │ ReadMemStats │ Handler │ CollectProfile │ Response │
     │    (STW)     │         │  (STW + I/O)  │ to client│
     ▲──────────────────────────────────────────▲
     │           adds latency here              │
```

#### After (async):
```
time─────────────────────────────────────────────────►
     │ metrics │ Handler │ capture │ Response │
     │ (~0.1µs)│         │ (~0.01µs)│ to client│
     ▲────────────────────────────────▲
     │    minimal latency added       │

                              └── go { CollectProfile + LateCollect + Store }
                                       (runs after response sent, no client impact)
```

### 5. Memory Safety Analysis

All data accessed by the async goroutine must remain valid after `ServeHTTP` returns:

| Data | Lifetime | Access Pattern | Safe? |
|------|----------|---------------|-------|
| `ctx` (context.Context) | Immutable linked list, GC'd when no refs | Read-only in goroutine | ✅ |
| `r` (*http.Request) | Lives until connection close | Read-only (Method, URL, Headers) | ✅ |
| `resData` (ResponseData) | Value struct, copied before goroutine | Owned by goroutine | ✅ |
| `resData.Headers` | `Clone()`d map | Owned by goroutine | ✅ |
| `startTime`, `duration` | Value types (time.Time, time.Duration) | Copied at capture | ✅ |
| `method`, `url`, `statusCode` | string/int values captured locally | Copied at capture | ✅ |
| `profileID` | string allocated before goroutine | Immutable string | ✅ |
| GORM `*requestQueries` | Pointer in ctx, handler done = no writers | Read-only in goroutine | ✅ |
| `wrapped` (responseWriter) | NOT passed to goroutine | N/A — we extract data before | ✅ |

**Key safety rule:** The goroutine must NOT access `wrapped` (the responseWriter) or any data derived from the underlying `http.ResponseWriter` after the middleware returns. All needed data is extracted into value types or cloned maps before the goroutine spawns.

### 6. `runtime/metrics` Integration Detail

**Pre-handler snapshot (replaces `WithMemoryStats`):**
```go
func WithMemorySnapshot(ctx context.Context) context.Context {
    snap := captureMemorySnapshot() // Uses runtime/metrics — no STW
    return context.WithValue(ctx, memorySnapshotKey, snap)
}
```

**Post-handler collection (replaces `MemoryCollector.Collect`):**
```go
func (c *MemoryCollector) Collect(ctx context.Context, _ *http.Request, _ ResponseData) (any, error) {
    afterSnap := captureMemorySnapshot() // No STW

    data := &MemoryData{
        AllocAfter:     afterSnap.heapObjects,
        TotalAlloc:     afterSnap.totalAlloc,
        HeapAlloc:      afterSnap.heapObjects,
        HeapInuse:      afterSnap.heapObjects + afterSnap.heapUnused,
        HeapObjects:    afterSnap.heapObjs,
        NumGC:          uint32(afterSnap.numGC),
        GoroutineCount: int(afterSnap.goroutines),
        Sys:            afterSnap.sys,
    }

    if beforeSnap, ok := MemorySnapshotFromContext(ctx); ok {
        data.AllocBefore = beforeSnap.heapObjects
        data.AllocDelta = int64(afterSnap.heapObjects) - int64(beforeSnap.heapObjects)
    }

    return data, nil
}
```

**Fallback consideration:** If any metric returns `KindBad` (unsupported on platform), fall back to `runtime.NumGoroutine()` for goroutine count. All other metrics have been stable since Go 1.16.

### 7. Error Handling

- If a collector's `Collect()` fails in the goroutine, the error is logged (existing behavior) and that collector is skipped. Other collectors continue.
- If `Storage.Store()` fails, the error is logged (existing behavior). The profile is lost.
- If the goroutine panics, it's recovered with a deferred recover+log to prevent crashing the process.
- If `Shutdown()` context expires, pending goroutines continue running (they'll complete eventually) but `Shutdown()` returns the context error.

### 8. Configuration Defaults

```go
func DefaultConfig() Config {
    return Config{
        Enabled:     true,           // existing
        StoragePath: "./var/profiler", // existing
        RoutePrefix: "/_profiler",    // existing
        SampleRate:  1.0,            // NEW: profile all requests by default
    }
}
```

### 9. Future Upgrade Path: Worker Pool

If goroutine-per-request becomes a concern at scale (>10K req/s), the design supports a clean upgrade:

```go
type Profiler struct {
    // Replace goroutine-per-request with:
    workCh chan profileWork
}

// In middleware, replace `go func() { ... }()` with:
select {
case p.workCh <- work:
    // Queued successfully
default:
    p.logger.Warn("profiler queue full, dropping profile", "id", profileID)
}
```

This change is isolated to `middleware.go` and `profiler.go` — no collector changes needed. The worker pool pattern was evaluated but deferred because goroutine-per-request is simpler and sufficient for the development profiler use case.

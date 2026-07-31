# Design: GORM Collector

## Technical Design Document

### 1. System Architecture

The GORM collector integrates with the profiler as a separate Go module, hooking into GORM's callback system to intercept queries and associating them with HTTP requests via context:

```
┌──────────────────────────────────────────────────────────────────┐
│  HTTP Request                                                     │
├──────────────────────────────────────────────────────────────────┤
│  Profiler Middleware (root module)                                 │
│  └─► GormCollector.Middleware (initializes context tracking)      │
│       └─► Application Handler                                    │
│            └─► db.WithContext(r.Context()).Find(...)              │
│                 ├─► GORM Plugin "before" callback → record start │
│                 ├─► GORM executes query                          │
│                 └─► GORM Plugin "after" callback → capture data  │
│                      └─► append to context query slice           │
├──────────────────────────────────────────────────────────────────┤
│  After Response: Profiler calls Collector.Collect()                │
│  └─► Reads queries from context                                  │
│  └─► Groups by connection                                        │
│  └─► Builds transaction groups                                   │
│  └─► Runs analysis (duplicates, N+1, slow)                      │
│  └─► Returns GormData (JSON-serializable)                        │
└──────────────────────────────────────────────────────────────────┘
```

### 2. Module Structure

```
collector/gorm/
├── go.mod              # Separate module: github.com/piotrkardasz/go-profiler/collector/gorm
├── go.sum
├── query.go            # Data types: QueryEntry, GormData, Summary, analysis types
├── options.go          # Configuration: Options, Option funcs, ConnectionConfig
├── plugin.go           # GORM plugin (callbacks), context management, backtrace capture
├── collector.go        # collector.Collector implementation, middleware, transaction grouping
├── analysis.go         # Analysis engine: duplicates, similar, N+1, slow detection
├── collector_test.go   # Collector interface tests
├── analysis_test.go    # Analysis algorithm tests
└── options_test.go     # Configuration tests
```

### 3. Core Design Decisions

#### 3.1 Separate Module Strategy

**Decision:** Use Go's multi-module repository pattern.

**Rationale:**
- Users who don't use GORM never pull `gorm.io/gorm` into their dependency tree.
- Follows established patterns (OpenTelemetry, AWS SDK v2).
- Independent versioning possible via `collector/gorm/v0.x.x` tags.
- Development uses `replace` directive pointing to root module.

**Module dependency graph:**
```
github.com/piotrkardasz/go-profiler/collector/gorm
  ├── github.com/piotrkardasz/go-profiler  (for collector.Collector interface)
  └── gorm.io/gorm v1.25.12               (GORM ORM)
```

#### 3.2 Context-Based Request Scoping

**Decision:** Store per-request query data in `context.Context` using a typed key.

**Rationale:**
- Matches existing collector patterns (TimingCollector uses context for start time).
- Thread-safe with mutex-protected append.
- Natural Go HTTP pattern — users already pass context via `db.WithContext(r.Context())`.
- Queries outside tracked context are silently ignored (safe for background jobs, tests).

**Implementation:**
```go
type contextKeyType struct{}
var contextKey = contextKeyType{}

type requestQueries struct {
    mu      sync.Mutex
    queries []QueryEntry
    index   int
}
```

#### 3.3 GORM Plugin Integration

**Decision:** Implement `gorm.Plugin` interface and register "before"/"after" callbacks.

**Rationale:**
- Official extension point — no hacks or monkey-patching.
- Hooks into all GORM operations: Create, Query, Update, Delete, Raw, Row.
- Each connection gets its own plugin instance (named uniquely).
- `InstanceSet`/`InstanceGet` for per-statement start time (thread-safe).

**Callback registration:**
```
Before: profiler:before_{create|query|update|delete|raw|row}
After:  profiler:after_{create|query|update|delete|raw|row}
```

#### 3.4 Runnable Query Generation

**Decision:** Use GORM's built-in `Dialector.Explain()` to generate the runnable query.

**Rationale:**
- Database-specific parameter formatting (different quoting for Postgres vs MySQL).
- Already tested and maintained by the GORM team.
- Available in the "after" callback via `db.Dialector.Explain(sql, vars...)`.

#### 3.5 Transaction Tracking via Context

**Decision:** Use a separate context key (`txKey`) to store the current transaction ID.

**Rationale:**
- GORM's `Transaction()` function creates a new `*gorm.DB` but preserves context.
- Users call `gormcollector.WithTransaction(ctx)` before `db.Transaction()`.
- All queries executed within that context automatically inherit the transaction ID.
- Simple, explicit, no reflection or internal GORM state inspection needed.

**Limitation:** Requires user to explicitly call `WithTransaction()`. Auto-detection would require hooking into GORM's internal transaction lifecycle which is fragile.

#### 3.6 Analysis as Pure Functions

**Decision:** Analysis functions operate on `[]QueryEntry` slices, not on GORM instances.

**Rationale:**
- Testable in isolation (no database needed for analysis tests).
- Can be reused for offline analysis of stored profile data.
- Clear separation of concerns: capture vs. analysis.

### 4. Data Flow

```
1. Request arrives
   └─► GormCollector.Middleware → WithContext(ctx) → stores *requestQueries in context

2. Handler executes GORM queries
   └─► db.WithContext(ctx).Find(...)
       ├─► Plugin.before() → InstanceSet("profiler:start_time", time.Now())
       ├─► GORM executes SQL
       └─► Plugin.after("SELECT")
           ├─► Gets start time from InstanceGet
           ├─► Builds QueryEntry (sql, params, duration, rows, error, tx_id, backtrace)
           ├─► Gets *requestQueries from context
           └─► Appends QueryEntry (mutex-protected)

3. Profiler calls Collect()
   └─► QueriesFromContext(ctx) → []QueryEntry
   └─► Group by connection → []ConnectionData
   └─► Build transaction groups per connection
   └─► Run analysis (duplicates, similar, N+1, slow)
   └─► Collect failed queries
   └─► Build summary statistics
   └─► Return &GormData{...}
```

### 5. Analysis Algorithms

#### 5.1 Duplicate Detection

- Hash each query by SHA256(SQL + "|" + param1 + "|" + param2 + ...)
- Group by hash, return groups with count > 1
- O(n) time complexity

#### 5.2 Similar Query Detection

- Normalize SQL: lowercase, collapse whitespace
- Group by normalized SQL
- Return groups with count > 1
- Note: Same SQL with different params = similar (potential batch opportunity)

#### 5.3 N+1 Detection

- Group by (normalized SQL, connection) pair
- Return groups where count >= threshold (default 5)
- Higher threshold than "similar" to reduce false positives
- Typically indicates: one list query followed by N detail queries

#### 5.4 Slow Query Detection

- Compare each query's duration against its connection's threshold
- Per-connection thresholds checked first, fall back to global default
- Returns all queries exceeding the threshold

### 6. Configuration Design

Options pattern with functional options:

```go
gormcollector.New(
    gormcollector.WithConnection("postgres-main", postgresDB),
    gormcollector.WithConnection("mysql-analytics", mysqlDB),
    gormcollector.WithSlowThreshold(200 * time.Millisecond),
    gormcollector.WithN1Threshold(3),
    gormcollector.WithBacktrace(true),  // or use GORM_PROFILER_BACKTRACE env
)
```

**Precedence for backtrace:**
1. Explicit `WithBacktrace(bool)` — highest priority
2. `GORM_PROFILER_BACKTRACE` env variable
3. Default: disabled

**Precedence for slow threshold:**
1. Per-connection `ConnectionConfig.SlowThreshold` — highest priority
2. Global `Options.SlowThreshold`
3. Default: 100ms

### 7. Parameter Serialization

Special handling for JSON compatibility:
- `time.Time` → RFC3339Nano string
- `*time.Time` → RFC3339Nano string or nil
- `[]byte` (>64 bytes) → `"[N bytes]"` (prevents large blobs in JSON)
- `[]byte` (≤64 bytes) → hex string
- All other types → passed through as-is (Go's JSON encoder handles them)

### 8. Backtrace Capture

When enabled:
- `runtime.Callers(4, pcs)` — skip internal frames
- `runtime.CallersFrames()` for human-readable output
- Filter out frames containing: `gorm.io/gorm`, `gormcollector`, `runtime.`, `database/sql`
- Limit to 10 user frames
- Format: `file:line function_name`

Performance note: `runtime.Callers` has ~1-5µs overhead per call. Acceptable for dev profiling, hence opt-in.

### 9. UI Panel Design

**GormPanel.vue** structure:

```
┌─────────────────────────────────────────────────────┐
│ Summary Bar                                          │
│ [12 Queries] [45.2ms Total] [2 TX] [1 Dup] [1 N+1] │
├─────────────────────────────────────────────────────┤
│ [Queries] [Transactions] [Analysis] [Failed]         │
├─────────────────────────────────────────────────────┤
│ Queries Tab:                                         │
│ ┌─ postgres-main (8 queries · 30.1ms) ────────────┐│
│ │ #1 [SELECT] 2.1ms  1 row                        ││
│ │   SELECT * FROM users WHERE id = ?               ││
│ │   Params: [42]                                   ││
│ │ #2 [INSERT] 5.3ms  1 row  [TX]                  ││
│ │   INSERT INTO orders ...                         ││
│ └──────────────────────────────────────────────────┘│
│ ┌─ mysql-analytics (4 queries · 15.1ms) ──────────┐│
│ │ ...                                              ││
│ └──────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────┘
```

**Color coding:**
- SELECT → green badge
- INSERT → blue badge
- UPDATE → yellow badge
- DELETE → red badge
- RAW → gray badge
- Slow → yellow background
- Error → red background
- Transaction → purple "TX" badge

### 10. Docker Example Architecture

```
examples/gorm-postgres/
├── docker-compose.yml    # postgres:16 + app service
├── Dockerfile            # Multi-stage Go build
├── go.mod                # Example module with replace directives
└── main.go               # Demo server with test endpoints

examples/gorm-mysql/
├── docker-compose.yml    # mysql:8 + app service
├── Dockerfile            # Multi-stage Go build
├── go.mod                # Example module with replace directives
└── main.go               # Demo server with test endpoints
```

Each example provides endpoints that demonstrate:
- Basic CRUD queries
- Eager loading (Preload) vs N+1 pattern
- Explicit transactions with grouped queries
- Error-producing queries (constraint violations, missing tables)

### 11. Integration Points

The GORM collector integrates with the profiler at three levels:

1. **Collector interface** — `Collect()` returns `*GormData` stored in `Profile.CollectorData["gorm"]`
2. **Middleware chain** — `gormCollector.Middleware(handler)` wraps inside `profiler.Middleware()`
3. **UI panel** — `GormPanel` registered via `registerPanel('gorm', GormPanel)` in `builtin.ts`

**Middleware ordering:**
```go
// Profiler middleware MUST wrap the GORM middleware
handler := profiler.Middleware(gormCollector.Middleware(appHandler))
```

This ensures the profiler's context (start time, memory) is set before GORM queries execute, and `Collect()` is called after all queries have completed.

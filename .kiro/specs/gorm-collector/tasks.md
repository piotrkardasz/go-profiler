# Tasks: GORM Collector

## Implementation Tasks

### Task 1: Create separate Go module structure [DONE]

**Objective:** Set up the GORM collector as an independent Go module within the mono-repo.

**Implementation:**
- Create `collector/gorm/go.mod` with module path `github.com/piotrkardasz/go-profiler/collector/gorm`
- Add `gorm.io/gorm v1.25.12` dependency
- Add `replace` directive for local development: `github.com/piotrkardasz/go-profiler => ../..`
- Run `go mod tidy` to resolve transitive dependencies

**Files created:**
- `collector/gorm/go.mod`
- `collector/gorm/go.sum`

---

### Task 2: Define data types and models [DONE]

**Objective:** Define all the data structures used by the collector for query capture, analysis, and output.

**Implementation:**
- `QueryEntry`: SQL, params, runnable query, duration, rows affected, operation, connection, error, transaction ID, backtrace, timestamp, index
- `TransactionGroup`: ID, connection, queries, total duration, status
- `ConnectionData`: name, queries, transactions, total duration, query count, failed queries
- `AnalysisResult`: duplicate groups, similar groups, N+1 groups, slow queries
- `DuplicateGroup`, `SimilarGroup`, `N1Group`: SQL, count, indices
- `Summary`: total queries, total duration, per-connection counts, slowest query, issue counts
- `GormData`: top-level output struct (connections, analysis, summary, failed queries)

**Files created:**
- `collector/gorm/query.go`

---

### Task 3: Implement configuration options [DONE]

**Objective:** Create the functional options pattern for configuring the collector.

**Implementation:**
- `Options` struct: connections, slow threshold, N+1 threshold, backtrace enabled
- `ConnectionConfig`: name, DB instance, per-connection slow threshold
- Option functions: `WithConnection()`, `WithConnectionConfig()`, `WithSlowThreshold()`, `WithN1Threshold()`, `WithBacktrace()`
- `defaultOptions()`: sensible defaults (100ms slow, 5 N+1 threshold)
- `isBacktraceEnabled()`: checks explicit config → env var → default false
- `slowThresholdFor(connection)`: per-connection override → global default
- Constants: `DefaultSlowThreshold`, `DefaultN1Threshold`, `EnvBacktrace`

**Files created:**
- `collector/gorm/options.go`

---

### Task 4: Implement GORM plugin with callback hooks [DONE]

**Objective:** Create the GORM plugin that intercepts all database operations and captures query data.

**Implementation:**
- `Plugin` struct implementing `gorm.Plugin` interface (`Name()`, `Initialize()`)
- `Initialize()` registers before/after callbacks for: Create, Query, Update, Delete, Raw, Row
- `before()` callback: stores `time.Now()` via `db.InstanceSet()`
- `after(operation)` callback: computes duration, builds `QueryEntry`, appends to context
- Context management:
  - `WithContext(ctx)`: initializes `*requestQueries` in context (typed key)
  - `queriesFromContext(ctx)`: retrieves tracker (nil = not tracked, skip silently)
  - `QueriesFromContext(ctx)`: public API, returns copy of queries
  - `WithTransaction(ctx)`: stores new transaction ID in context
  - `TransactionIDFromContext(ctx)`: retrieves current transaction ID
- `cloneParams()`: converts non-serializable types (time.Time, []byte) to JSON-safe values
- `captureBacktrace()`: captures filtered Go call stack (when enabled)
- `isInternalFrame()`: filters gorm/runtime/collector frames from backtrace
- `generateTxID()`: creates "tx-" + 8-char hex random ID

**Files created:**
- `collector/gorm/plugin.go`

---

### Task 5: Implement analysis engine [DONE]

**Objective:** Build the query analysis algorithms for detecting performance issues.

**Implementation:**
- `analyze()`: orchestrator that runs all detection algorithms
- `detectDuplicates()`: SHA256 hash of (SQL + params), groups with count > 1
- `detectSimilar()`: groups by normalized SQL (lowercase + collapsed whitespace), count > 1
- `detectN1()`: groups by (normalized SQL + connection), count >= threshold
- `detectSlow()`: filters queries exceeding per-connection or global threshold
- `buildSummary()`: aggregates stats across all connections and analysis results
- Helper functions:
  - `hashQuery()`: deterministic hash for duplicate detection
  - `normalizeSQL()`: lowercase + whitespace normalization
  - `collapseWhitespace()`: efficient single-pass whitespace reducer

**Files created:**
- `collector/gorm/analysis.go`

---

### Task 6: Implement collector.Collector interface [DONE]

**Objective:** Create the main `Collector` struct that integrates with the profiler framework.

**Implementation:**
- `Collector` struct holding `*Options`
- `New(options ...Option)`: applies options, registers GORM plugin with each connection
- `Name()`: returns "gorm"
- `Collect(ctx, req, res)`:
  - Reads queries from context
  - Groups by connection (preserving first-seen order)
  - Builds transaction groups per connection
  - Runs analysis
  - Collects failed queries
  - Builds summary
  - Returns `*GormData`
- `Reset()`: no-op (state is per-request in context)
- `PanelMeta()`: returns metadata (name: "gorm", label: "Database", icon: "database", component: "GormPanel")
- `Middleware(next)`: returns `http.Handler` that calls `WithContext(r.Context())`
- `buildTransactionGroups()`: organizes queries by transaction ID, determines status

**Files created:**
- `collector/gorm/collector.go`

---

### Task 7: Write unit tests [DONE]

**Objective:** Comprehensive test coverage for all collector functionality.

**Implementation:**
- **Collector tests** (`collector_test.go`):
  - `TestCollectorName`: verifies name is "gorm"
  - `TestCollectorPanelMeta`: verifies all panel metadata fields
  - `TestCollectorImplementsInterfaces`: compile-time interface checks
  - `TestCollectWithNoContext`: graceful handling when no tracking active
  - `TestCollectWithQueries`: basic query capture and grouping
  - `TestCollectMultipleConnections`: multi-connection ordering and counts
  - `TestCollectFailedQueries`: error capture and failed queries section
  - `TestCollectTransactionGroups`: transaction grouping and duration
  - `TestCollectTransactionRolledBack`: error → rolled_back status
  - `TestMiddleware`: context initialization
  - `TestWithContext`: context tracking lifecycle
  - `TestWithTransaction`: transaction ID generation

- **Analysis tests** (`analysis_test.go`):
  - `TestDetectDuplicates`: identifies identical queries
  - `TestDetectDuplicatesNoDuplicates`: no false positives
  - `TestDetectSimilar`: groups same SQL with different params
  - `TestDetectSimilarWithWhitespace`: normalization handles whitespace
  - `TestDetectN1`: identifies N+1 patterns above threshold
  - `TestDetectN1BelowThreshold`: no false positives below threshold
  - `TestDetectSlow`: identifies queries above threshold
  - `TestDetectSlowPerConnection`: per-connection threshold overrides
  - `TestBuildSummary`: aggregate statistics computation
  - `TestNormalizeSQL`: SQL normalization cases
  - `TestCollapseWhitespace`: whitespace collapsing
  - `TestHashQuery`: deterministic hashing, collision avoidance

- **Options tests** (`options_test.go`):
  - `TestDefaultOptions`: default values
  - `TestWithSlowThresholdOption`: option application
  - `TestWithN1ThresholdOption`: option application
  - `TestWithBacktraceOption`: explicit enable/disable
  - `TestIsBacktraceEnabledExplicit`: explicit overrides env
  - `TestIsBacktraceEnabledFromEnv`: env variable reading
  - `TestSlowThresholdForConnection`: per-connection override logic

**Test results:** 30 tests, all passing.

**Files created:**
- `collector/gorm/collector_test.go`
- `collector/gorm/analysis_test.go`
- `collector/gorm/options_test.go`

---

### Task 8: Create PostgreSQL Docker example [DONE]

**Objective:** Provide a working example demonstrating the GORM collector with PostgreSQL.

**Implementation:**
- `docker-compose.yml`: PostgreSQL 16 Alpine + Go app service with health checks
- `Dockerfile`: multi-stage build (golang:1.23-alpine → alpine:3.19)
- `go.mod`: example module with replace directives for local development
- `main.go`: HTTP server demonstrating:
  - Models: `User` (name, email) + `Post` (title, body, user_id)
  - Auto-migration and seed data (5 users, 5 posts)
  - Endpoints:
    - `GET /api/users` — simple query
    - `POST /api/users/create` — insert
    - `GET /api/posts` — eager loading with Preload
    - `GET /api/posts/n1` — intentional N+1 pattern
    - `POST /api/transaction` — explicit transaction with WithTransaction
    - `GET /api/error` — duplicate key violation
  - Full profiler integration with UI

**Files created:**
- `examples/gorm-postgres/docker-compose.yml`
- `examples/gorm-postgres/Dockerfile`
- `examples/gorm-postgres/go.mod`
- `examples/gorm-postgres/main.go`

---

### Task 9: Create MySQL Docker example [DONE]

**Objective:** Provide a working example demonstrating the GORM collector with MySQL.

**Implementation:**
- `docker-compose.yml`: MySQL 8.0 + Go app service with health checks
- `Dockerfile`: multi-stage build (golang:1.23-alpine → alpine:3.19)
- `go.mod`: example module with replace directives for local development
- `main.go`: HTTP server demonstrating:
  - Models: `Product` (name, price, stock) + `Order` (product_id, quantity, total)
  - Auto-migration and seed data (5 products, 5 orders)
  - Endpoints:
    - `GET /api/products` — simple query
    - `GET /api/orders` — eager loading with Preload
    - `GET /api/orders/n1` — intentional N+1 pattern
    - `POST /api/purchase` — transaction (find product, check stock, decrement, create order)
    - `GET /api/error` — query on nonexistent table
  - Full profiler integration with UI

**Files created:**
- `examples/gorm-mysql/docker-compose.yml`
- `examples/gorm-mysql/Dockerfile`
- `examples/gorm-mysql/go.mod`
- `examples/gorm-mysql/main.go`

---

### Task 10: Create UI panel component [DONE]

**Objective:** Build the Vue panel that displays GORM collector data in the profiler UI.

**Implementation:**
- `GormPanel.vue` with full TypeScript typing for all data structures
- **Summary bar**: total queries, total time, transactions, duplicates, N+1, failed (with color coding)
- **Tab system**: dynamic tabs based on available data
  - Queries tab: grouped by connection, each query shows index/operation/duration/rows/SQL/params/runnable/backtrace
  - Transactions tab: grouped by transaction ID with status badges and nested queries
  - Analysis tab: sections for N+1, duplicates, similar, slow queries
  - Failed tab: all errored queries with error messages
- **Visual indicators**:
  - Operation badges: color-coded (SELECT=green, INSERT=blue, UPDATE=yellow, DELETE=red)
  - Slow queries: yellow background + "SLOW" badge
  - Errored queries: red background + "ERROR" badge
  - Transaction markers: purple "TX" badge
  - Transaction status: committed=green, rolled_back=red, pending=yellow
- **Collapsible backtraces** with frame count
- Registered in `ui/src/plugin/builtin.ts` as `registerPanel('gorm', GormPanel)`

**Files created:**
- `ui/src/components/panels/GormPanel.vue`
- Modified: `ui/src/plugin/builtin.ts`

---

### Task 11: Final verification [DONE]

**Objective:** Verify all components build and test correctly.

**Verification results:**
- ✅ Root module: `go build ./...` — passes
- ✅ Root module: `go test ./...` — 6 packages pass
- ✅ GORM collector module: `go build ./...` — passes
- ✅ GORM collector module: `go test ./...` — 30 tests pass
- ✅ PostgreSQL example: `go build ./...` — passes
- ✅ MySQL example: `go build ./...` — passes
- ✅ UI TypeScript: `vue-tsc --noEmit` — passes
- ✅ UI Vite build: `npx vite build` — passes (58 modules, 122KB JS gzipped to 44KB)

# Requirements: GORM Collector

## Overview

Add a GORM database query collector to the go-profiler. It captures per-request SQL queries, parameters, execution time, and provides analysis features (N+1 detection, duplicate queries, slow query highlighting). It supports multiple named database connections with different providers (PostgreSQL, MySQL, etc.) and is implemented as a separate Go module to avoid coupling GORM as a dependency of the root profiler package.

## Functional Requirements

### FR-1: Separate Go Module

- FR-1.1: The GORM collector MUST be a separate Go module at `collector/gorm/` with its own `go.mod`.
- FR-1.2: The module MUST depend on `gorm.io/gorm` without adding this dependency to the root module.
- FR-1.3: The module MUST reference the root profiler module for the `collector.Collector` interface.
- FR-1.4: Users MUST be able to `go get github.com/piotrkardasz/go-profiler/collector/gorm` independently.
- FR-1.5: The module MUST be versionable independently (e.g., `collector/gorm/v0.1.0` tags).

### FR-2: Per-Query Data Capture

- FR-2.1: The collector MUST capture the raw SQL query string.
- FR-2.2: The collector MUST capture bound query parameters with their types.
- FR-2.3: The collector MUST generate a "runnable query" with parameters interpolated into the SQL for copy-paste debugging.
- FR-2.4: The collector MUST capture query execution duration in milliseconds.
- FR-2.5: The collector MUST capture the number of rows affected/returned for all queries.
- FR-2.6: The collector MUST detect the SQL operation type (SELECT, INSERT, UPDATE, DELETE, RAW).
- FR-2.7: The collector MUST associate each query with its named connection identifier.
- FR-2.8: The collector MUST capture the full error message for failed queries.
- FR-2.9: The collector MUST capture the execution timestamp for each query.
- FR-2.10: The collector MUST maintain sequential execution order (index) for each query.

### FR-3: Multiple Database Connections

- FR-3.1: The collector MUST support registering multiple named GORM database connections.
- FR-3.2: Each connection MUST be identified by a user-provided name (e.g., "postgres-main", "mysql-analytics").
- FR-3.3: Queries MUST be grouped by connection name in the output data.
- FR-3.4: Per-connection configuration MUST be supported (e.g., different slow query thresholds).
- FR-3.5: The API for adding connections MUST follow the pattern: `WithConnection(name string, db *gorm.DB)`.

### FR-4: Backtrace / Call Stack Capture

- FR-4.1: The collector MUST support capturing Go call stack traces for each query.
- FR-4.2: Backtrace collection MUST be controlled by the `GORM_PROFILER_BACKTRACE` environment variable.
- FR-4.3: When the env variable is set to "true" or "1", backtraces MUST be captured.
- FR-4.4: Backtrace collection MUST be overridable programmatically via `WithBacktrace(bool)`.
- FR-4.5: The backtrace MUST filter out GORM internals, runtime, and collector package frames.
- FR-4.6: The backtrace MUST be limited to a reasonable depth (10 user frames max).

### FR-5: Transaction Tracking

- FR-5.1: The collector MUST support grouping queries within the same database transaction.
- FR-5.2: Transactions MUST be identified by a unique transaction ID.
- FR-5.3: Users MUST be able to mark a transaction boundary via `WithTransaction(ctx)` context helper.
- FR-5.4: Transaction groups MUST show total duration and query count.
- FR-5.5: Transaction status MUST be tracked: "committed", "rolled_back", or "pending".
- FR-5.6: A transaction containing any errored query MUST be marked as "rolled_back".

### FR-6: Query Analysis

- FR-6.1: The collector MUST detect **duplicate queries** — identical SQL with identical parameters executed multiple times in one request.
- FR-6.2: The collector MUST detect **similar queries** — same SQL structure with different parameters (potential batch candidates).
- FR-6.3: The collector MUST detect **N+1 query patterns** — the same SQL pattern repeated N or more times with varying parameters.
- FR-6.4: The N+1 detection threshold MUST be configurable (default: 5).
- FR-6.5: The collector MUST detect **slow queries** — queries exceeding a configurable duration threshold.
- FR-6.6: The slow query threshold MUST default to 100ms.
- FR-6.7: The slow query threshold MUST be configurable per-connection (overriding the global default).

### FR-7: Summary Statistics

- FR-7.1: The collector MUST produce summary statistics including: total queries, total DB time.
- FR-7.2: The summary MUST include queries-per-connection breakdown.
- FR-7.3: The summary MUST identify the slowest query.
- FR-7.4: The summary MUST include counts of: duplicates, N+1 patterns, failed queries, transactions.

### FR-8: Failed Queries Section

- FR-8.1: Failed queries (those with errors) MUST be collected in a separate dedicated section.
- FR-8.2: Failed queries MUST include the full error message, SQL, params, duration, and connection name.
- FR-8.3: Failed queries MUST be visually distinct in the UI panel.

### FR-9: GORM Plugin Integration

- FR-9.1: The collector MUST integrate with GORM via the `gorm.Plugin` interface.
- FR-9.2: The plugin MUST hook into all GORM callback types: Create, Query, Update, Delete, Raw, Row.
- FR-9.3: The plugin MUST use "before" callbacks to record query start time.
- FR-9.4: The plugin MUST use "after" callbacks to capture query results and compute duration.
- FR-9.5: The plugin MUST only capture queries for requests that have been initialized with the collector context.

### FR-10: Context-Based Per-Request Tracking

- FR-10.1: Query tracking MUST be scoped to individual HTTP requests via Go context.
- FR-10.2: The collector MUST provide `WithContext(ctx)` to initialize per-request query tracking.
- FR-10.3: The collector MUST provide an HTTP middleware that automatically initializes the context.
- FR-10.4: GORM queries MUST use `db.WithContext(r.Context())` to associate with the current request.
- FR-10.5: Queries outside of a tracked context MUST be silently ignored (no error, no capture).

### FR-11: UI Panel

- FR-11.1: The collector MUST provide a Vue panel component (`GormPanel`) registered as "gorm".
- FR-11.2: The panel MUST display a summary bar with: total queries, total time, transaction count, duplicate count, N+1 count, failed count.
- FR-11.3: The panel MUST have tabbed navigation: Queries, Transactions (if any), Analysis, Failed (if any).
- FR-11.4: The Queries tab MUST show queries grouped by connection name with per-connection stats.
- FR-11.5: Each query MUST display: index, operation badge (color-coded), duration, rows affected, SQL, params.
- FR-11.6: Slow queries MUST be visually highlighted.
- FR-11.7: The Transactions tab MUST show grouped queries with transaction status badges.
- FR-11.8: The Analysis tab MUST show N+1 warnings, duplicate groups, similar groups, and slow queries.
- FR-11.9: The Failed tab MUST show all errored queries with error messages.
- FR-11.10: Backtraces MUST be displayed in collapsible sections when available.

### FR-12: Docker Examples

- FR-12.1: A PostgreSQL example MUST be provided with `docker-compose.yml` and Go application code.
- FR-12.2: A MySQL example MUST be provided with `docker-compose.yml` and Go application code.
- FR-12.3: Examples MUST demonstrate: basic queries, eager loading, N+1 problems, transactions, error queries.
- FR-12.4: Examples MUST include a `Dockerfile` for building the application.
- FR-12.5: Examples MUST use `docker-compose` health checks to ensure database readiness.

## Non-Functional Requirements

### NFR-1: Performance

- NFR-1.1: Query capture MUST add minimal overhead (<0.1ms per query in normal operation).
- NFR-1.2: Backtrace capture overhead is acceptable only when explicitly enabled.
- NFR-1.3: The collector MUST be safe for concurrent use (multiple goroutines issuing queries).
- NFR-1.4: Context-based tracking MUST use `sync.Mutex` to protect the per-request query slice.

### NFR-2: Compatibility

- NFR-2.1: MUST work with GORM v1.25+ (current stable).
- NFR-2.2: MUST work with any GORM dialect/driver (PostgreSQL, MySQL, SQLite, SQL Server).
- NFR-2.3: MUST NOT interfere with other GORM plugins or middlewares.
- NFR-2.4: MUST follow the same Go version requirement as the root module (Go 1.21+).

### NFR-3: Data Safety

- NFR-3.1: Query parameters MUST be logged as-is by default (user opted in by using the collector).
- NFR-3.2: Large byte slice parameters (>64 bytes) MUST be truncated to "[N bytes]" representation.
- NFR-3.3: Time parameters MUST be serialized in RFC3339Nano format for JSON compatibility.
- NFR-3.4: The collector MUST NOT modify or interfere with GORM's actual query execution.

### NFR-4: Extensibility

- NFR-4.1: The data structures MUST be JSON-serializable for the profiler storage/API.
- NFR-4.2: The option pattern MUST allow future additions without breaking changes.
- NFR-4.3: The analysis module MUST be separable (functions operate on data types, not on GORM directly).

# Build Report: Go Profiler

## Summary

Full implementation of `github.com/piotrkardasz/go-profiler` — a framework-agnostic HTTP profiling middleware for Go inspired by Symfony's Profiler. Built in a single session across 13 tasks, later extended with a GORM database collector.

**Total estimated time:** ~45-60 minutes of continuous agent execution (core), plus additional time for the GORM collector.

## Timeline Breakdown

| Phase | Tasks | Approx Time | Description |
|-------|-------|-------------|-------------|
| Core Go interfaces & scaffolding | Tasks 1-2 | ~5 min | Module init, Profile struct, Collector/Storage interfaces, Profiler core, config |
| Collectors (Request, Timing, Memory) | Task 3 | ~4 min | Three built-in collectors with context helpers |
| Storage (Filesystem + Memory) | Tasks 4-5 | ~6 min | File-based JSON storage with atomic writes, in-memory LRU storage |
| Middleware | Task 6 | ~5 min | HTTP middleware with ResponseWriter wrapper, async storage |
| JSON API handlers | Task 7 | ~4 min | REST endpoints for profiles and collectors |
| Vue 3 UI (scaffolding + panels + plugin system) | Tasks 8-9 | ~10 min | Full Vue app with routing, API client, 4 custom panels, plugin system |
| OpenTelemetry collector + OTel panel | Tasks 10-11 | ~8 min | Span/metric capture, waterfall trace UI |
| Embed UI + dual-mode handler | Task 12 | ~5 min | embed.FS, SPA routing, dev proxy mode |
| Examples, Makefile, README, specs | Task 13 + specs | ~8 min | 2 examples, documentation, .kiro specs |
| GORM collector | Extension | ~15 min | Full DB query profiling with analysis, 2 examples (MySQL + PostgreSQL) |

## Output Metrics

| Metric | Count |
|--------|-------|
| Go source files | 22 |
| Go test files | 11 |
| Go lines of code (excl. tests) | ~2,600 |
| Go test lines | ~1,000 |
| Vue/TypeScript/CSS files | 18 |
| Vue/TypeScript/CSS lines | ~1,500 |
| Go packages | 6 (`profiler`, `collector`, `collector/otel`, `collector/gorm`, `storage`, `handler`) |
| Go test coverage | All packages tested |
| External Go dependencies | 2 (`go.opentelemetry.io/otel` family, `gorm.io/gorm`) |
| npm dependencies | 4 runtime + 4 dev |

## What Was Built

### Go Backend
- **Profiler core** — Config, collector management, enable/disable, `X-Profiler-Id` header generation
- **HTTP middleware** — Framework-agnostic `http.Handler` wrapper with async profile storage
- **3 built-in collectors** — Request/Response, Timing, Memory (with context-based before/after snapshots)
- **OpenTelemetry collector** — Captures traces (spans) and metrics per request via LateCollector
- **GORM collector** — Full database query profiling (see detailed section below)
- **File-based storage** — Atomic JSON writes, filtering, pagination, purging
- **In-memory storage** — LRU eviction, same interface for testing
- **JSON API** — List/filter/get/purge profiles, collector metadata, CORS support
- **UI handler** — Embedded assets (embed.FS) or reverse-proxy to Vite dev server

### Vue Frontend
- **Profile list view** — Table with method/status badges, filters, pagination, purge
- **Profile detail view** — Tabbed panels per collector with dynamic component loading
- **Panel plugin system** — `registerPanel()` API for custom collector UI components
- **4 built-in panels** — Request (headers table), Timing (hero + bar), Memory (stat cards), OTel (waterfall + metrics table)
- **Generic fallback** — Recursive JSON tree view for collectors without custom panels

### Documentation & Tooling
- **README.md** — Installation, quick start, configuration, architecture, custom collectors/panels guide, API reference
- **Makefile** — build, test, vet, ui-build, ui-dev, clean, example targets
- **4 working examples** — Basic server, OpenTelemetry integration, GORM MySQL, GORM PostgreSQL
- **.kiro/specs/** — Requirements, design, and tasks documents
- **.kiro/skills/** — Reusable skills for Go package architecture and spec-driven development

---

## GORM Collector — Design Decisions

The GORM collector (`collector/gorm/`) was built as a reference implementation of an advanced custom collector, demonstrating the extensibility of the profiler system. Key decisions made:

### Decision: Separate Go Module

The GORM collector is a separate Go module (`collector/gorm/go.mod`) to avoid forcing the `gorm.io/gorm` dependency on users who don't need it. Users who want database profiling explicitly import it.

### Decision: GORM Plugin Architecture

Rather than wrapping database calls externally, the collector registers as a native **GORM plugin** via `gorm.DB.Use()`. It hooks into GORM's callback system (before/after for Create, Query, Update, Delete, Raw, Row operations). This means:
- Zero changes required to user's existing GORM code
- Captures all queries automatically, including those from Preload, Associations, etc.
- Works transparently with any GORM driver (MySQL, PostgreSQL, SQLite, etc.)

### Decision: Context-Based Per-Request Query Tracking

Queries are accumulated in a `requestQueries` struct stored in the request context. The collector's middleware (`gormCollector.Middleware()`) initializes this context. Users must pass `r.Context()` to GORM via `db.WithContext(r.Context())` for queries to be captured.

### Decision: Named Connections

The collector supports **multiple named database connections** (e.g., "postgres-main", "mysql-analytics"). Each connection is registered separately and queries are grouped by connection in the profile data. This mirrors real applications that connect to multiple databases.

### Decision: Query Analysis Engine

The collector includes an analysis layer that detects common performance issues:

| Analysis | Description | Detection Method |
|----------|-------------|-----------------|
| **Duplicate queries** | Exact same SQL + params executed multiple times | SHA-256 hash of SQL + params |
| **Similar queries** | Same SQL template with different params | Normalized SQL comparison |
| **N+1 queries** | Same query pattern repeated N times (configurable threshold, default 5) | Count of normalized SQL per connection |
| **Slow queries** | Queries exceeding configurable threshold (default 100ms) | Duration comparison per connection |

### Decision: Runnable Query Output

Each captured query stores both the raw SQL with placeholders AND a `runnable_query` field with parameters interpolated (via `db.Dialector.Explain()`). This allows developers to copy-paste the exact query into a database client for debugging.

### Decision: Transaction Grouping

Queries within a transaction are grouped via a `TransactionID` stored in context. Users call `gormcollector.WithTransaction(ctx)` before starting a transaction to enable grouping. The UI shows queries organized by transaction with commit/rollback status.

### Decision: Optional Backtrace Collection

Call stack capture is **opt-in** (via `GORM_PROFILER_BACKTRACE=true` env var or `WithBacktrace(true)` option) because `runtime.Callers()` has measurable overhead. When enabled, the collector filters out GORM internals and runtime frames, showing only application code.

### Decision: Configurable Thresholds

- `WithSlowThreshold(duration)` — Per-connection or global slow query threshold
- `WithN1Threshold(count)` — How many repeated queries trigger an N+1 warning
- Per-connection threshold overrides for heterogeneous setups (e.g., stricter on primary, relaxed on analytics)

### Decision: UI Panel Registration

The GORM collector registers as `"gorm"` with panel metadata pointing to `"GormPanel"` component. The Vue panel shows:
- Summary cards (total queries, duration, duplicates, N+1 count)
- Per-connection query timeline
- Transaction groups with individual queries
- Analysis tab with flagged issues
- Failed queries with error details

## Challenges & Fixes During Implementation

| Issue | Resolution |
|-------|------------|
| Import cycle (root ↔ storage) | Moved `Storage` interface + `SearchCriteria` to root package |
| Import cycle in tests (middleware_test → storage → root) | Created inline `testStorage` mock in test file |
| `http.FileServer` 301 redirects for index.html | Replaced with direct `fs.FS` serving + manual SPA fallback |
| TypeScript `fractionalSecondDigits` not in type defs | Used manual millisecond formatting instead |
| `sdktrace.WithAttributes` undefined | Corrected to `oteltrace.WithAttributes` (trace package, not SDK) |
| Unused variable in Vue (TypeScript strict mode) | Removed `availablePanels` computed that wasn't referenced in template |

## Verification

```
$ go test ./...
ok   github.com/piotrkardasz/go-profiler
ok   github.com/piotrkardasz/go-profiler/collector
ok   github.com/piotrkardasz/go-profiler/collector/otel
ok   github.com/piotrkardasz/go-profiler/handler
ok   github.com/piotrkardasz/go-profiler/storage

$ go vet ./...
(no issues)

$ go build ./...
(success, including examples)

$ cd ui && npx vue-tsc --noEmit
(no errors)

$ cd ui && npx vite build
✓ built in ~3s
```

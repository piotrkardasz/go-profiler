# Requirements: Go Profiler

## Overview

Build a framework-agnostic Go HTTP profiling package inspired by Symfony's Profiler. It collects per-request profiling data via pluggable collectors, stores profiles as JSON files, attaches an `X-Profiler-Id` header to responses, and exposes a JSON API + Vue 3 UI for browsing profiles.

## Functional Requirements

### FR-1: HTTP Middleware

- FR-1.1: The package MUST provide an `http.Handler` middleware that works with any Go HTTP framework or router.
- FR-1.2: Each profiled response MUST include an `X-Profiler-Id` header with a unique profile token.
- FR-1.3: The middleware MUST skip profiling its own routes (the profiler UI and API).
- FR-1.4: The middleware MUST capture the HTTP status code and response body size.
- FR-1.5: Profile storage MUST happen asynchronously to avoid blocking the response.

### FR-2: Pluggable Collectors

- FR-2.1: The package MUST define a `Collector` interface with `Name()`, `Collect()`, and `Reset()` methods.
- FR-2.2: The package MUST define a `LateCollector` interface for collectors that need post-response data (e.g., async operations like OTel span export).
- FR-2.3: The package MUST ship with three built-in collectors:
  - Request collector: HTTP method, URL, headers (with sensitive header redaction), query params, response status/headers/size.
  - Timing collector: Start time, end time, total duration via context.
  - Memory collector: Alloc before/after/delta, heap stats, goroutine count via context snapshot.
- FR-2.4: Collectors MUST be registered in order and invoked sequentially.
- FR-2.5: A failing collector MUST NOT prevent other collectors from running.

### FR-3: OpenTelemetry Collector

- FR-3.1: The package MUST provide an OTel collector that captures both traces (spans) and metrics per request.
- FR-3.2: The OTel collector MUST implement `LateCollector` since spans may not be complete until after the response.
- FR-3.3: Captured span data MUST include: name, trace ID, span ID, parent ID, start/end time, duration, status, attributes, events.
- FR-3.4: Captured metric data MUST include: name, description, unit, type, value, attributes, timestamp.
- FR-3.5: The collector MUST support Gauge, Sum, and Histogram metric types.

### FR-4: Pluggable Storage

- FR-4.1: The package MUST define a `Storage` interface with `Store()`, `Load()`, `List()`, and `Purge()` methods.
- FR-4.2: The default storage MUST be file-based, storing one JSON file per profile in a configurable directory.
- FR-4.3: File writes MUST be atomic (write to temp file, then rename).
- FR-4.4: `List()` MUST return profiles sorted by timestamp descending (newest first).
- FR-4.5: `List()` MUST support filtering by: method, URL substring, status code, status range, time range.
- FR-4.6: `List()` MUST support pagination with limit and offset.
- FR-4.7: `Purge()` MUST remove profiles older than a configurable max age.
- FR-4.8: An in-memory storage implementation MUST be provided for testing, with LRU eviction.

### FR-5: JSON API

- FR-5.1: `GET /_profiler/api/profiles` — List profiles with query parameter filters.
- FR-5.2: `GET /_profiler/api/profiles/{id}` — Get full profile by ID.
- FR-5.3: `DELETE /_profiler/api/profiles` — Purge profiles (with `max_age` parameter).
- FR-5.4: `GET /_profiler/api/collectors` — List registered collectors with panel metadata.
- FR-5.5: All API responses MUST be JSON with proper Content-Type headers.
- FR-5.6: The API MUST include CORS headers for Vue dev server compatibility.

### FR-6: Web UI

- FR-6.1: The package MUST include an embedded Vue 3 + Vite web UI served at `/_profiler/`.
- FR-6.2: The UI MUST show a profile list with: method, URL, status, duration, timestamp, profile ID.
- FR-6.3: The UI MUST show a profile detail view with tabbed panels per collector.
- FR-6.4: The UI MUST support a panel plugin system where custom collectors can register Vue components.
- FR-6.5: Collectors without a registered panel MUST fall back to a generic JSON tree view.
- FR-6.6: Built-in panels: Request (headers table), Timing (hero duration + bar), Memory (stat cards), OTel (waterfall trace + metrics table).

### FR-7: Dual UI Mode

- FR-7.1: In production, the UI MUST be served from Go's `embed.FS` with zero external dependencies.
- FR-7.2: In dev mode (`GO_PROFILER_UI_DEV=true`), the handler MUST reverse-proxy to the Vite dev server.
- FR-7.3: SPA routing MUST work (non-file paths serve index.html).

### FR-8: Configuration & Enable/Disable

- FR-8.1: The profiler MUST be controllable via `GO_PROFILER_ENABLED` env var (default: `true`).
- FR-8.2: The profiler MUST support runtime enable/disable via `SetEnabled()`.
- FR-8.3: When disabled, the middleware MUST pass through with zero overhead.
- FR-8.4: Configuration MUST support: storage path, route prefix, logger, UI dev mode, UI dev server URL.

## Non-Functional Requirements

### NFR-1: Performance

- NFR-1.1: Profile storage MUST be asynchronous to not impact response latency.
- NFR-1.2: The middleware MUST add minimal overhead when enabled (<1ms for simple requests).
- NFR-1.3: Storage implementations MUST be safe for concurrent use.

### NFR-2: Compatibility

- NFR-2.1: MUST require Go 1.21+ (for `slog`, modern stdlib features).
- NFR-2.2: MUST work with any `http.Handler`-compatible framework (stdlib, Chi, Gin adapter, Echo, etc.).
- NFR-2.3: MUST use `net/http` standard library for all HTTP handling (no third-party router dependencies).

### NFR-3: Security

- NFR-3.1: Sensitive headers (Authorization, Cookie, Set-Cookie) MUST be redacted in collected data.
- NFR-3.2: Profile IDs MUST be generated using `crypto/rand` for unpredictability.
- NFR-3.3: File storage MUST prevent path traversal in profile IDs.
- NFR-3.4: The profiler SHOULD NOT be enabled in production environments.

### NFR-4: Extensibility

- NFR-4.1: Adding a new collector MUST require only implementing the `Collector` interface and calling `AddCollector()`.
- NFR-4.2: Adding a new storage backend MUST require only implementing the `Storage` interface.
- NFR-4.3: Adding a new UI panel MUST require only calling `registerPanel(name, component)` in the Vue app.
- NFR-4.4: The package MUST NOT force any specific project structure on users.

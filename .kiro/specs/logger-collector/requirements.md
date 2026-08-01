# Requirements: Logger Collector

## Overview

Add a logger collector to the go-profiler that captures log entries produced during an HTTP request lifecycle. The collector uses a `LogAdapter` interface to abstract logging libraries, ships with built-in adapters for Go's standard `log/slog` package and the `log` package, and allows users to plug in adapters for popular third-party libraries (zerolog, zap, logrus). Log entries are captured per-request via context, providing developers visibility into what was logged during each profiled request.

## Functional Requirements

### FR-1: Core Module (No External Dependencies)

- FR-1.1: The logger collector MUST live in the root module under `collector/logger.go` (alongside timing, memory, request, config collectors).
- FR-1.2: The collector MUST NOT introduce any external Go dependencies — only standard library.
- FR-1.3: The built-in `log/slog` adapter MUST use only the standard library `log/slog` package.
- FR-1.4: The collector MUST implement the `collector.Collector` interface.
- FR-1.5: The collector MUST implement the `collector.PanelProvider` interface.
- FR-1.6: The collector MUST implement the `collector.ContextSetup` interface to inject log capture state into the request context.

### FR-2: LogAdapter Interface

- FR-2.1: The collector MUST define a `LogAdapter` interface that any logging library can implement.
- FR-2.2: The interface MUST expose:
  ```go
  type LogAdapter interface {
      Name() string
      Install(capture CaptureFunc) RemoveFunc
  }
  ```
- FR-2.3: `CaptureFunc` MUST be defined as `func(entry LogEntry)` — called by adapters when a log entry is produced.
- FR-2.4: `RemoveFunc` MUST be defined as `func()` — called to uninstall/detach the adapter (cleanup).
- FR-2.5: The collector MUST support multiple `LogAdapter` implementations registered simultaneously.
- FR-2.6: Users MUST be able to create adapters for external libraries (zerolog, zap, logrus) by implementing the `LogAdapter` interface.

### FR-3: Per-Request Log Capture

- FR-3.1: The collector MUST capture log entries on a per-request basis using request context.
- FR-3.2: Log entries produced outside a request context MUST NOT be captured (no global log pollution).
- FR-3.3: The collector MUST use `ContextSetup` to initialize a per-request log buffer in the context.
- FR-3.4: Adapters MUST receive a `CaptureFunc` that is context-aware — it only captures entries when an active request context exists.
- FR-3.5: The log buffer MUST be safe for concurrent writes (goroutines spawned during request handling may log concurrently).

### FR-4: LogEntry Data Structure

- FR-4.1: Each captured log entry MUST contain:
  - `Timestamp` — when the log entry was produced
  - `Level` — log severity level (DEBUG, INFO, WARN, ERROR, FATAL)
  - `Message` — the log message text
  - `Source` — adapter name that produced the entry (e.g., "slog", "zap")
- FR-4.2: Each log entry MAY contain:
  - `Attributes` — structured key-value pairs (from structured logging)
  - `Caller` — file:line where the log call originated (if available)
  - `Stack` — stack trace (for ERROR/FATAL level entries, if available)

### FR-5: Built-in slog Adapter

- FR-5.1: The collector MUST include a built-in adapter for Go's `log/slog` package.
- FR-5.2: The slog adapter MUST implement `slog.Handler` to intercept log records.
- FR-5.3: The adapter MUST wrap an existing `slog.Handler` (chain pattern) so logs still reach their original destination.
- FR-5.4: The adapter MUST capture: timestamp, level, message, attributes (including group prefixes), and source (caller) if `AddSource` is enabled.
- FR-5.5: The adapter MUST be installable by wrapping the default logger:
  ```go
  slog.SetDefault(slog.New(adapter.Handler(slog.Default().Handler())))
  ```
- FR-5.6: The adapter `Name()` MUST return "slog".

### FR-6: Built-in log Package Adapter

- FR-6.1: The collector MUST include a built-in adapter for Go's standard `log` package.
- FR-6.2: The adapter MUST capture output written via `log.Print`, `log.Printf`, `log.Println`, `log.Fatal`, `log.Panic` and their variants.
- FR-6.3: The adapter MUST use `log.SetOutput()` with an `io.Writer` that intercepts log output while still forwarding to the original writer.
- FR-6.4: All entries from the standard `log` package MUST use level INFO (the standard log package has no levels).
- FR-6.5: The adapter `Name()` MUST return "log".

### FR-7: Log Level Normalization

- FR-7.1: The collector MUST define a canonical set of log levels: `DEBUG`, `INFO`, `WARN`, `ERROR`, `FATAL`.
- FR-7.2: Adapters MUST map their library-specific levels to the canonical levels:
  - slog: Debug→DEBUG, Info→INFO, Warn→WARN, Error→ERROR
  - zap: Debug→DEBUG, Info→INFO, Warn→WARN, Error→ERROR, DPanic→ERROR, Panic→FATAL, Fatal→FATAL
  - zerolog: Trace→DEBUG, Debug→DEBUG, Info→INFO, Warn→WARN, Error→ERROR, Fatal→FATAL, Panic→FATAL
  - logrus: Trace→DEBUG, Debug→DEBUG, Info→INFO, Warn→WARN, Error→ERROR, Fatal→FATAL, Panic→FATAL
- FR-7.3: The canonical level type MUST be exported so adapter authors can use it.

### FR-8: Configuration Options

- FR-8.1: The collector MUST use functional options pattern consistent with other collectors.
- FR-8.2: Options MUST include:
  - `WithAdapter(adapter LogAdapter)` — register a custom log adapter
  - `WithMinLevel(level LogLevel)` — minimum level to capture (default: DEBUG, captures all)
  - `WithMaxEntries(n int)` — maximum log entries per request (default: 1000, prevents memory issues)
  - `WithCallerInfo(enabled bool)` — enable/disable caller info capture (default: true)
  - `WithStackTrace(enabled bool)` — enable/disable stack trace for ERROR+ entries (default: false)
  - `WithoutSlog()` — disable the built-in slog adapter
  - `WithoutStdLog()` — disable the built-in standard log adapter
  - `WithAttributeMaxSize(bytes int)` — maximum byte size for attribute values (default: 1024, truncates large values)
  - `WithForwardBufferSize(size int)` — buffered channel capacity for async log forwarding (default: 4096)

### FR-9: Collected Data Structure

- FR-9.1: The collector output MUST be JSON-serializable.
- FR-9.2: The output MUST be structured as:
  ```go
  type LoggerData struct {
      Entries    []LogEntry        `json:"entries"`
      Summary    LogSummary        `json:"summary"`
      Truncated  bool              `json:"truncated"`
      MaxEntries int               `json:"max_entries"`
  }
  ```
- FR-9.3: `LogSummary` MUST contain counts per level and total count:
  ```go
  type LogSummary struct {
      Total int `json:"total"`
      Debug int `json:"debug"`
      Info  int `json:"info"`
      Warn  int `json:"warn"`
      Error int `json:"error"`
      Fatal int `json:"fatal"`
  }
  ```
- FR-9.4: If entries exceed `MaxEntries`, the `Truncated` field MUST be true and oldest entries beyond the limit MUST be dropped.

### FR-10: UI Panel

- FR-10.1: The collector MUST have a custom Vue panel component (`LoggerPanel`) registered as "logger".
- FR-10.2: The panel MUST display a summary bar with: total log count, counts per level (with color coding), and a truncation warning if applicable.
- FR-10.3: The panel MUST display a scrollable list of log entries in chronological order.
- FR-10.4: Each log entry MUST show: timestamp (relative to request start), level (color-coded badge), message, and source adapter name.
- FR-10.5: Log entries with attributes MUST be expandable to show key-value pairs.
- FR-10.6: The panel MUST support filtering by log level (toggle buttons for each level).
- FR-10.7: The panel MUST support text search across message content and attribute values.
- FR-10.8: ERROR and FATAL entries MUST be visually distinct (red background/border).
- FR-10.9: If caller info is available, it MUST be displayed as a clickable/copyable file:line reference.
- FR-10.10: If stack trace is available, it MUST be displayed in a collapsible code block.

## Non-Functional Requirements

### NFR-1: Performance

- NFR-1.1: The collector MUST add negligible overhead (<0.5ms) per log entry capture.
- NFR-1.2: The per-request log buffer MUST use a pre-allocated or pooled slice to minimize allocations.
- NFR-1.3: The `MaxEntries` cap MUST prevent unbounded memory growth for requests that produce excessive logging.
- NFR-1.4: Attribute serialization MUST be lazy (only done during `Collect()`, not during capture).
- NFR-1.5: Forwarding log records to the inner/original handler (file I/O, network, stdout) MUST happen asynchronously via a background goroutine, so the request hot path is not blocked on log output I/O.
- NFR-1.6: The async forwarding channel MUST use a buffered channel (default: 4096) to absorb bursts without blocking the caller.
- NFR-1.7: When the forwarding channel is full (backpressure), the adapter MUST fall back to synchronous forwarding rather than dropping log records.

### NFR-2: Concurrency Safety

- NFR-2.1: The per-request log buffer MUST be safe for concurrent writes from multiple goroutines.
- NFR-2.2: Adapter installation/removal MUST be safe for concurrent use.
- NFR-2.3: The collector MUST NOT introduce global locks that could contend across requests.

### NFR-3: Compatibility

- NFR-3.1: MUST work with Go 1.21+ (matching root module requirement, `log/slog` available since 1.21).
- NFR-3.2: MUST NOT break existing collectors or middleware chain.
- NFR-3.3: The `LogAdapter` interface MUST be stable for third-party adapter implementation.
- NFR-3.4: The slog adapter MUST be compatible with any existing `slog.Handler` chain.

### NFR-4: Extensibility

- NFR-4.1: Third-party adapters (zap, zerolog, logrus) MUST be implementable without modifying the collector package.
- NFR-4.2: The interface design MUST accommodate both structured loggers (slog, zap, zerolog) and unstructured loggers (standard log).
- NFR-4.3: Future adapters MAY be published as separate modules (e.g., `collector/logger/zap`) if they introduce dependencies.

### NFR-5: Reliability

- NFR-5.1: If an adapter panics during log capture, the panic MUST be recovered and NOT crash the application.
- NFR-5.2: The collector MUST function correctly even if no adapters are registered (returns empty log data).
- NFR-5.3: The collector MUST handle the case where `Collect()` is called without prior `SetupContext()` (returns empty data, no panic).
- NFR-5.4: On graceful shutdown (`Close()`), the collector MUST flush all pending log records in the async forwarding channel before returning.
- NFR-5.5: The async forwarder goroutine MUST exit cleanly on `Close()` with no goroutine leaks.

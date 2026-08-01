# Tasks: Logger Collector

## Implementation Tasks

### Task 1: Define LogAdapter interface, LogEntry, and LogLevel types

**Objective:** Create the public types and interface that all log adapters implement and the shared data structures.

**Implementation:**
- `LogLevel` type (int) with constants: `LevelDebug`, `LevelInfo`, `LevelWarn`, `LevelError`, `LevelFatal`
- `LogLevel.String()` method returning canonical names (DEBUG, INFO, WARN, ERROR, FATAL)
- `LogLevel.MarshalJSON()` for JSON serialization as string
- `LogEntry` struct: Timestamp, Level, Message, Source, Attributes (map[string]any), Caller, Stack
- `CaptureFunc` type: `func(ctx context.Context, entry LogEntry)`
- `RemoveFunc` type: `func()`
- `LogAdapter` interface: `Name() string`, `Install(capture CaptureFunc) RemoveFunc`
- `LoggerData` struct: Entries, Summary, Truncated, MaxEntries
- `LogSummary` struct: Total, Debug, Info, Warn, Error, Fatal

**Files to create:**
- `collector/logger_adapter.go`

---

### Task 2: Implement per-request LogBuffer with context storage

**Objective:** Create the thread-safe per-request log buffer and context helper functions.

**Implementation:**
- `logBufferKey` unexported context key type
- `WithLogBuffer(ctx, *LogBuffer) context.Context` — stores buffer in context
- `GetLogBuffer(ctx) *LogBuffer` — retrieves buffer from context (nil-safe)
- `LogBuffer` struct: `mu sync.Mutex`, `entries []LogEntry`, `maxEntries int`, `truncated bool`
- `NewLogBuffer(maxEntries int) *LogBuffer` — pre-allocates with `min(maxEntries, 64)` capacity
- `(*LogBuffer).Append(entry LogEntry)` — thread-safe append, sets truncated flag if at cap
- `(*LogBuffer).Drain() ([]LogEntry, bool)` — returns entries and truncated flag, resets buffer

**Files to create:**
- `collector/logger_buffer.go`

---

### Task 3: Implement LogForwarder (async background goroutine)

**Objective:** Create the shared background worker that forwards log records to inner handlers without blocking the caller.

**Implementation:**
- `forwardRecord` struct: ctx `context.Context`, record `slog.Record`, inner `slog.Handler`
- `LogForwarder` struct: ch `chan forwardRecord`, done `chan struct{}`
- `NewLogForwarder(bufferSize int) *LogForwarder` — creates channel, starts goroutine
- `(*LogForwarder).run()` — drains channel, calls `inner.Handle()` for each record (best-effort, ignores errors)
- `(*LogForwarder).Forward(ctx, record, inner)` — non-blocking send; falls back to synchronous `inner.Handle()` if channel full
- `(*LogForwarder).Close()` — closes channel, waits for goroutine to drain all pending records

**Design properties:**
- Buffered channel (default 4096) absorbs bursts
- Single consumer goroutine preserves log ordering
- `select`/`default` fallback ensures logs are never dropped (degrades to sync under backpressure)
- Graceful shutdown flushes all pending records

**Files to create:**
- `collector/logger_forwarder.go`

---

### Task 4: Implement built-in slog adapter (with async forwarding)

**Objective:** Create a `slog.Handler` wrapper that captures log records into the request's LogBuffer synchronously and forwards to the inner handler asynchronously via LogForwarder.

**Implementation:**
- `SlogAdapter` struct: inner `slog.Handler`, capture `CaptureFunc`, forwarder `*LogForwarder`, addSource bool, groups []string, attrs []slog.Attr
- `NewSlogAdapter(inner slog.Handler, capture CaptureFunc, forwarder *LogForwarder, addSource bool) *SlogAdapter`
- `(*SlogAdapter).Handle(ctx, slog.Record) error`:
  - Convert `slog.Level` to `LogLevel` via `slogLevelToLogLevel()`
  - Extract attributes from record + pre-existing attrs with group prefixes
  - Extract caller from `r.PC` if addSource is true
  - Call `capture(ctx, entry)` — synchronous, ~50ns
  - Call `forwarder.Forward(ctx, r, a.inner)` — async, non-blocking
  - Return nil immediately (caller not blocked on inner handler I/O)
- `(*SlogAdapter).Enabled(ctx, level)` — delegates to inner
- `(*SlogAdapter).WithAttrs(attrs)` — returns new SlogAdapter with accumulated attrs + same forwarder
- `(*SlogAdapter).WithGroup(name)` — returns new SlogAdapter with accumulated group + same forwarder
- Helper: `slogLevelToLogLevel(slog.Level) LogLevel`
- Helper: `extractSlogAttributes(r slog.Record, groups []string, preAttrs []slog.Attr) map[string]any`
- Helper: `formatCaller(pc uintptr) string` — returns "file.go:line" format
- Helper: `buildGroupKey(groups []string, key string) string`
- The adapter implements `LogAdapter` interface:
  - `Name()` returns "slog"
  - `Install(capture)` creates LogForwarder, wraps current `slog.Default().Handler()`, sets as new default, returns RemoveFunc that restores original and closes forwarder

**Files to create:**
- `collector/logger_slog.go`

---

### Task 5: Implement built-in standard log adapter (with async forwarding)

**Objective:** Create an `io.Writer` adapter that intercepts standard `log` package output, captures synchronously, and forwards to the original writer asynchronously.

**Implementation:**
- `stdLogForwardRecord` struct: data []byte, original io.Writer
- `StdLogAdapter` struct: original `io.Writer`, capture `CaptureFunc`, activeCtx `atomic.Pointer[context.Context]`, forwardCh `chan stdLogForwardRecord`, done `chan struct{}`
- `NewStdLogAdapter(capture CaptureFunc, bufferSize int) *StdLogAdapter` — creates channel, starts forwarder goroutine
- `(*StdLogAdapter).runForwarder()` — drains channel, writes to original writer
- `(*StdLogAdapter).Write(p []byte) (int, error)`:
  - Parse log message from bytes (trim prefix timestamps if standard flags used)
  - Create `LogEntry` with LevelInfo, Source "log", current timestamp
  - Load active context from atomic pointer
  - Call `capture(ctx, entry)` — synchronous
  - Copy byte slice (caller may reuse), send to forwardCh — async with sync fallback
  - Return `len(p), nil`
- `(*StdLogAdapter).SetActiveContext(ctx context.Context)` — atomically stores context
- `(*StdLogAdapter).Close()` — closes channel, waits for drain
- The adapter implements `LogAdapter` interface:
  - `Name()` returns "log"
  - `Install(capture)` captures `log.Writer()` as original, creates StdLogAdapter, calls `log.SetOutput(adapter)`, returns RemoveFunc that calls `log.SetOutput(original)` and closes forwarder

**Files to create:**
- `collector/logger_stdlog.go`

---

### Task 6: Implement LoggerCollector with options and lifecycle

**Objective:** Create the main collector struct that orchestrates adapters, context setup, and data collection.

**Implementation:**
- `loggerOptions` struct with all option fields and defaults
- All `LoggerOption` functions: `WithAdapter`, `WithMinLevel`, `WithMaxEntries`, `WithCallerInfo`, `WithStackTrace`, `WithoutSlog`, `WithoutStdLog`, `WithAttributeMaxSize`
- Constants: `defaultMaxEntries = 1000`, `defaultAttrMaxSize = 1024`, `defaultMinLevel = LevelDebug`, `defaultForwardBufSize = 4096`
- `LoggerCollector` struct: opts, removeFuncs []RemoveFunc, captureFunc CaptureFunc, stdLogAdapter *StdLogAdapter, forwarder *LogForwarder
- `NewLoggerCollector(opts ...LoggerOption) *LoggerCollector`:
  - Apply options with defaults
  - Create LogForwarder with configured buffer size
  - Build CaptureFunc (min level filter, nil-ctx guard, panic recovery)
  - Install slog adapter with forwarder (unless disabled)
  - Install stdlog adapter with its own forwarder (unless disabled)
  - Install user-provided adapters
  - Store all RemoveFuncs
- `Name()` returns "logger"
- `SetupContext(ctx) context.Context`:
  - Create new LogBuffer
  - Store in context
  - Update StdLogAdapter active context (if enabled)
  - Return enriched context
- `Collect(ctx, req, res) (any, error)`:
  - Get LogBuffer from context (return empty data if nil)
  - Drain entries
  - Truncate oversized attributes
  - Build summary (count per level)
  - Return `*LoggerData`
- `Reset()` — no-op
- `PanelMeta()` — name "logger", label "Logs", icon "file-text", component "LoggerPanel"
- `Close()` — call all RemoveFuncs, then close forwarder and stdlog forwarder (drains pending records)
- Helper: `buildSummary(entries []LogEntry) LogSummary`
- Helper: `truncateAttributes(attrs map[string]any, maxSize int) map[string]any`

**Files to create:**
- `collector/logger.go`

---

### Task 7: Write unit tests for LogBuffer

**Objective:** Test the per-request buffer's thread safety and behavior.

**Tests:**
- `TestLogBufferAppend`: basic append and drain
- `TestLogBufferMaxEntries`: entries beyond max are dropped, truncated flag set
- `TestLogBufferDrainResetsState`: drain returns entries and resets buffer
- `TestLogBufferConcurrentAppend`: multiple goroutines appending concurrently (race detector)
- `TestLogBufferEmptyDrain`: drain on empty buffer returns nil/empty slice
- `TestGetLogBufferNilContext`: GetLogBuffer with no buffer in context returns nil
- `TestWithLogBufferRoundTrip`: store and retrieve buffer from context

**Files to create:**
- `collector/logger_buffer_test.go`

---

### Task 8: Write unit tests for LogForwarder

**Objective:** Test the async forwarding goroutine behavior, ordering, backpressure, and shutdown.

**Tests:**
- `TestLogForwarderOrderPreserved`: records forwarded in FIFO order
- `TestLogForwarderAsyncDoesNotBlock`: Forward() returns immediately even with slow inner handler
- `TestLogForwarderBackpressureFallback`: when channel full, falls back to synchronous call
- `TestLogForwarderCloseFlushes`: Close() waits until all pending records are processed
- `TestLogForwarderCloseIdempotent`: multiple Close() calls don't panic
- `TestLogForwarderInnerHandlerError`: errors from inner handler don't crash forwarder
- `TestLogForwarderConcurrentForward`: multiple goroutines calling Forward() concurrently (race detector)

**Files to create:**
- `collector/logger_forwarder_test.go`

---

### Task 9: Write unit tests for slog adapter

**Objective:** Test slog record capture, attribute extraction, and handler chaining.

**Tests:**
- `TestSlogAdapterName`: returns "slog"
- `TestSlogAdapterCapturesInfoRecord`: captures info-level message
- `TestSlogAdapterCapturesAllLevels`: debug, info, warn, error all captured correctly
- `TestSlogAdapterLevelMapping`: slog levels map to correct LogLevel values
- `TestSlogAdapterAttributes`: structured attributes captured in entry
- `TestSlogAdapterGroupedAttributes`: WithGroup prefixes attribute keys
- `TestSlogAdapterPreAttrs`: WithAttrs attrs included in every entry
- `TestSlogAdapterCallerInfo`: caller file:line captured when addSource=true
- `TestSlogAdapterNoCallerWhenDisabled`: caller empty when addSource=false
- `TestSlogAdapterAsyncForwarding`: inner handler called asynchronously (not on caller goroutine)
- `TestSlogAdapterForwardsToInner`: inner handler still receives the record (via forwarder)
- `TestSlogAdapterEnabled`: delegates Enabled() to inner handler
- `TestSlogAdapterInstallAndRemove`: Install wraps default, Remove restores original
- `TestSlogAdapterContextRequired`: no panic when context has no LogBuffer

**Files to create:**
- `collector/logger_slog_test.go`

---

### Task 10: Write unit tests for standard log adapter

**Objective:** Test standard log package interception and forwarding.

**Tests:**
- `TestStdLogAdapterName`: returns "log"
- `TestStdLogAdapterCapturesOutput`: log.Println output captured
- `TestStdLogAdapterLevelIsInfo`: all entries have LevelInfo
- `TestStdLogAdapterAsyncForwarding`: original writer called asynchronously
- `TestStdLogAdapterForwardsToOriginal`: original writer receives output (via forwarder)
- `TestStdLogAdapterSetActiveContext`: context updates atomically
- `TestStdLogAdapterNoContextNoPanic`: Write() with nil active context doesn't panic
- `TestStdLogAdapterInstallAndRemove`: Install replaces log output, Remove restores
- `TestStdLogAdapterMessageParsing`: log prefix/timestamp stripped from message

**Files to create:**
- `collector/logger_stdlog_test.go`

---

### Task 11: Write unit tests for LoggerCollector

**Objective:** Test the main collector integration, options, and lifecycle.

**Tests:**
- `TestLoggerCollectorName`: returns "logger"
- `TestLoggerCollectorPanelMeta`: verifies all metadata fields
- `TestLoggerCollectorImplementsInterfaces`: compile-time checks for Collector + PanelProvider + ContextSetup
- `TestLoggerCollectorSetupContextCreatesBuffer`: context contains LogBuffer after SetupContext
- `TestLoggerCollectorCollectReturnsEntries`: entries captured during request are returned
- `TestLoggerCollectorCollectEmptyWithoutSetup`: Collect without SetupContext returns empty data
- `TestLoggerCollectorMinLevelFilter`: entries below min level are not captured
- `TestLoggerCollectorMaxEntries`: truncation works correctly
- `TestLoggerCollectorWithoutSlog`: slog adapter not installed
- `TestLoggerCollectorWithoutStdLog`: stdlog adapter not installed
- `TestLoggerCollectorCustomAdapter`: user adapter integrated and captures entries
- `TestLoggerCollectorAttributeTruncation`: oversized attribute values truncated
- `TestLoggerCollectorSummaryBuilt`: summary counts match entry levels
- `TestLoggerCollectorClose`: all adapters removed on close
- `TestLoggerCollectorPanicRecovery`: adapter panic doesn't crash collector

**Files to create:**
- `collector/logger_test.go`

---

### Task 12: Create UI panel component (LoggerPanel.vue)

**Objective:** Build the Vue panel that displays captured log entries with filtering.

**Implementation:**
- **Summary bar**: total count, per-level count badges (color-coded), truncation warning
- **Filter controls**: level toggle buttons (each level on/off), text search input
- **Log entry list** (scrollable, chronological):
  - Each entry: relative timestamp, level badge (colored), source tag, message, inline attributes preview
  - Expandable: full attributes table, caller info (copyable), stack trace (collapsible code block)
  - ERROR/FATAL entries: red left border for visual distinction
- **Empty state**: message when no logs captured
- **TypeScript interfaces**: LogEntry, LoggerData, LogSummary, LogLevel
- **Level colors**: DEBUG=gray, INFO=blue, WARN=amber, ERROR=red, FATAL=dark-red
- **Search**: case-insensitive substring match on message + attribute values
- **Combined filtering**: level toggles AND text search (both must match)

**Files to create:**
- `ui/src/components/panels/LoggerPanel.vue`

**Files to modify:**
- `ui/src/plugin/builtin.ts` — add `registerPanel('logger', LoggerPanel)` import and call

---

### Task 13: Update basic example to include logger collector

**Objective:** Show logger collector usage in the basic example.

**Implementation:**
- Add `collector.NewLoggerCollector()` to the basic example's profiler setup
- Add sample `slog.Info()` and `slog.Warn()` calls in the handler to demonstrate capture
- Add a comment showing how to use `WithMinLevel` and `WithMaxEntries` options
- Call `loggerCollector.Close()` in a deferred shutdown to flush pending logs and restore loggers

**Files to modify:**
- `examples/basic/main.go`

---

### Task 14: Final verification

**Objective:** Verify all components build and test correctly.

**Verification steps:**
- `go build ./...` — root module builds without errors
- `go test ./...` — all tests pass (existing + new logger collector tests)
- `go vet ./...` — no warnings
- `go test -race ./...` — no race conditions in concurrent tests
- GORM collector module still builds: `cd collector/gorm && go build ./...`
- Verify no external dependencies added to root `go.mod`
- Verify `LogAdapter` interface is exported and usable from external packages
- Verify `LogLevel` constants are exported for adapter authors
- Verify `GetLogBuffer` and `WithLogBuffer` are exported for advanced use cases

---

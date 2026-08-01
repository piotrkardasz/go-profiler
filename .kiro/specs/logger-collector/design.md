# Design: Logger Collector

## Technical Design Document

### 1. System Architecture

The logger collector lives in the root module alongside existing collectors. It captures log entries produced during HTTP request handling by injecting a per-request log buffer into the context via `ContextSetup`. Adapters hook into logging libraries and forward entries to the buffer. The design mirrors the GORM collector's context-based capture pattern.

```
┌──────────────────────────────────────────────────────────────────┐
│  HTTP Request                                                     │
├──────────────────────────────────────────────────────────────────┤
│  Profiler Middleware                                              │
│  ├─► SetupContext(): initialize per-request LogBuffer in ctx     │
│  └─► Application Handler                                         │
│      ├─► slog.Info("handling request", "path", r.URL.Path)       │
│      │   └─► SlogAdapter.Handle():                               │
│      │       ├─ capture to LogBuffer (sync, ~50ns)               │
│      │       └─ forward to inner handler (async, via channel)    │
│      ├─► log.Printf("processing item %d", id)                   │
│      │   └─► StdLogAdapter.Write():                              │
│      │       ├─ capture to LogBuffer (sync, ~50ns)               │
│      │       └─ forward to original writer (async, via channel)  │
│      └─► [user code with zap/zerolog/logrus]                     │
│          └─► [UserAdapter] → captures to LogBuffer               │
├──────────────────────────────────────────────────────────────────┤
│  Background Goroutine (LogForwarder):                             │
│  └─► Drains channel → calls inner.Handle() / original.Write()   │
│      (I/O happens here, off the request hot path)                │
├──────────────────────────────────────────────────────────────────┤
│  After Response: Profiler calls LoggerCollector.Collect()          │
│  ├─► Retrieve LogBuffer from context                             │
│  ├─► Build summary (counts per level)                            │
│  ├─► Serialize attributes (lazy)                                 │
│  └─► Return LoggerData (JSON-serializable)                       │
└──────────────────────────────────────────────────────────────────┘
```


### 2. File Structure

```
collector/
├── collector.go              # Existing: Collector interface, PanelProvider, ContextSetup
├── logger.go                 # LoggerCollector struct, Collect(), options, SetupContext()
├── logger_adapter.go         # LogAdapter interface, LogEntry, LogLevel, CaptureFunc types
├── logger_buffer.go          # Per-request LogBuffer (context-stored, thread-safe)
├── logger_forwarder.go       # LogForwarder — async background goroutine for inner handler I/O
├── logger_slog.go            # Built-in slog.Handler adapter (with async forwarding)
├── logger_stdlog.go          # Built-in standard log package adapter (with async forwarding)
├── logger_test.go            # Unit tests for LoggerCollector
├── logger_slog_test.go       # Unit tests for slog adapter
├── logger_stdlog_test.go     # Unit tests for standard log adapter
├── logger_buffer_test.go     # Unit tests for LogBuffer concurrency
├── logger_forwarder_test.go  # Unit tests for LogForwarder (ordering, backpressure, shutdown)
├── config.go                 # Existing
├── memory.go                 # Existing
├── request.go                # Existing
└── timing.go                 # Existing
```

### 3. Core Design Decisions

#### 3.1 Context-Based Per-Request Capture (Like GORM Collector)

**Decision:** Use `ContextSetup` to inject a `*LogBuffer` into the request context. Adapters retrieve the buffer from context to append entries.

**Rationale:**
- Follows the exact same pattern as the GORM collector's query tracking.
- Guarantees log isolation between concurrent requests.
- No global state — entries are only captured when an active request context exists.
- Goroutines spawned during request handling that propagate the context will have their logs captured too.

**Context key pattern:**
```go
type logBufferKey struct{}

func WithLogBuffer(ctx context.Context, buf *LogBuffer) context.Context {
    return context.WithValue(ctx, logBufferKey{}, buf)
}

func GetLogBuffer(ctx context.Context) *LogBuffer {
    buf, _ := ctx.Value(logBufferKey{}).(*LogBuffer)
    return buf
}
```


#### 3.2 LogAdapter Interface

**Decision:** Define a minimal two-method interface for adapter integration.

```go
// CaptureFunc is called by adapters to record a log entry into the active request buffer.
// Adapters receive this function during Install() and call it for each log entry.
// The function is context-aware — it no-ops if no active request buffer exists.
type CaptureFunc func(ctx context.Context, entry LogEntry)

// RemoveFunc uninstalls an adapter, restoring original logging behavior.
type RemoveFunc func()

// LogAdapter abstracts a logging library integration.
// Implement this interface to capture logs from any logging library.
type LogAdapter interface {
    // Name returns a human-readable identifier for this adapter (e.g., "slog", "zap").
    Name() string

    // Install hooks the adapter into the logging library.
    // The provided CaptureFunc should be called for each log entry produced.
    // Returns a RemoveFunc that uninstalls the adapter when called.
    Install(capture CaptureFunc) RemoveFunc
}
```

**Rationale:**
- `Install/Remove` pattern allows adapters to hook into global loggers (slog, standard log) and undo the hook on shutdown.
- `CaptureFunc` takes context so it can locate the per-request buffer — adapters don't need to know about `LogBuffer` internals.
- Minimal surface area — only two methods to implement for third-party adapters.
- The context-aware `CaptureFunc` handles the "no active request" case gracefully (no-op).

#### 3.3 LogBuffer Thread Safety

**Decision:** Use `sync.Mutex` on the per-request buffer with a capped slice.

```go
// LogBuffer stores log entries for a single request. Thread-safe.
type LogBuffer struct {
    mu         sync.Mutex
    entries    []LogEntry
    maxEntries int
    truncated  bool
}

func NewLogBuffer(maxEntries int) *LogBuffer {
    return &LogBuffer{
        entries:    make([]LogEntry, 0, min(maxEntries, 64)), // pre-allocate reasonable capacity
        maxEntries: maxEntries,
    }
}

func (b *LogBuffer) Append(entry LogEntry) {
    b.mu.Lock()
    defer b.mu.Unlock()
    if len(b.entries) >= b.maxEntries {
        b.truncated = true
        return
    }
    b.entries = append(b.entries, entry)
}

func (b *LogBuffer) Drain() ([]LogEntry, bool) {
    b.mu.Lock()
    defer b.mu.Unlock()
    entries := b.entries
    truncated := b.truncated
    b.entries = nil
    b.truncated = false
    return entries, truncated
}
```

**Rationale:**
- `sync.Mutex` is simpler and lower overhead than channels for append-only workloads.
- Pre-allocated slice with `min(maxEntries, 64)` avoids over-allocation for most requests.
- `maxEntries` cap prevents unbounded growth from runaway logging.
- `Drain()` transfers ownership — after `Collect()` the buffer is empty (ready for pool reuse if added later).


#### 3.4 Log Level Normalization

**Decision:** Define a `LogLevel` type with canonical values and a string representation.

```go
type LogLevel int

const (
    LevelDebug LogLevel = iota
    LevelInfo
    LevelWarn
    LevelError
    LevelFatal
)

func (l LogLevel) String() string {
    switch l {
    case LevelDebug:
        return "DEBUG"
    case LevelInfo:
        return "INFO"
    case LevelWarn:
        return "WARN"
    case LevelError:
        return "ERROR"
    case LevelFatal:
        return "FATAL"
    default:
        return "UNKNOWN"
    }
}

func (l LogLevel) MarshalJSON() ([]byte, error) {
    return []byte(`"` + l.String() + `"`), nil
}
```

**Rationale:**
- Integer type enables efficient comparison for min-level filtering.
- String serialization matches common log level conventions.
- Exported constants allow adapter authors to map their library's levels.

#### 3.5 Async Forwarding Strategy (Performance)

**Decision:** Decouple the profiler's log capture from the inner handler's I/O by forwarding log records to the original handler asynchronously via a dedicated background goroutine with a buffered channel.

**Problem:** The real latency risk is not the buffer append (~50ns with mutex) — it's waiting for the inner `slog.Handler.Handle()` to complete. Inner handlers often perform I/O (write to file, send to network, flush to stdout with a lock). This blocks the application goroutine on every log call, adding potentially microseconds to milliseconds per entry.

**Solution:** The `SlogAdapter.Handle()` method:
1. Captures the entry into the per-request `LogBuffer` (fast, in-process, ~50ns)
2. Sends the `slog.Record` + context to a buffered channel for async forwarding to the inner handler
3. Returns immediately — the application goroutine is not blocked on I/O

A single background goroutine (the "log forwarder") drains the channel and calls `inner.Handle()` sequentially. This preserves log ordering while removing I/O from the hot path.

```go
// forwardRecord holds everything needed to replay a log record on the inner handler.
type forwardRecord struct {
    ctx    context.Context
    record slog.Record
    inner  slog.Handler
}

// LogForwarder is a shared background worker that forwards log records
// to inner handlers without blocking the caller.
type LogForwarder struct {
    ch   chan forwardRecord
    done chan struct{}
}

func NewLogForwarder(bufferSize int) *LogForwarder {
    f := &LogForwarder{
        ch:   make(chan forwardRecord, bufferSize),
        done: make(chan struct{}),
    }
    go f.run()
    return f
}

func (f *LogForwarder) run() {
    defer close(f.done)
    for rec := range f.ch {
        // Best-effort: if inner handler fails, we log and continue
        _ = rec.inner.Handle(rec.ctx, rec.record)
    }
}

func (f *LogForwarder) Forward(ctx context.Context, r slog.Record, inner slog.Handler) {
    select {
    case f.ch <- forwardRecord{ctx: ctx, record: r, inner: inner}:
        // Sent successfully
    default:
        // Channel full — fall back to synchronous to avoid dropping logs
        _ = inner.Handle(ctx, r)
    }
}

func (f *LogForwarder) Close() {
    close(f.ch)
    <-f.done // Wait for all pending records to be flushed
}
```

**Key design properties:**
- **Buffered channel** (default: 4096 entries) absorbs bursts without blocking
- **Fallback to sync** when channel is full — never drops logs, degrades gracefully under extreme load
- **Single goroutine** consumer preserves log ordering (no interleaved writes)
- **Graceful shutdown** via `Close()` — drains all pending records before returning
- **Shared across requests** — one forwarder per collector instance, not per request

**Benchmarks (expected):**
| Scenario | Without async | With async |
|----------|--------------|------------|
| slog.Info() with file handler | ~2-5µs (file I/O) | ~100ns (channel send) |
| slog.Info() with JSON/stderr | ~1-3µs (stdout lock) | ~100ns (channel send) |
| Channel full (backpressure) | N/A | ~2-5µs (sync fallback) |

#### 3.6 Slog Adapter as Handler Wrapper (with Async Forwarding)

**Decision:** Implement `slog.Handler` that wraps an existing handler, capturing entries into the buffer synchronously and forwarding to the inner handler asynchronously.

```go
// SlogAdapter captures slog records into the request's LogBuffer.
// It wraps an existing slog.Handler — capture is synchronous (fast),
// forwarding to the inner handler is asynchronous (avoids I/O blocking).
type SlogAdapter struct {
    inner      slog.Handler
    capture    CaptureFunc
    forwarder  *LogForwarder
    addSource  bool
    groups     []string
    attrs      []slog.Attr
}

func (a *SlogAdapter) Handle(ctx context.Context, r slog.Record) error {
    // 1. Capture the record into LogBuffer via CaptureFunc (FAST — ~50ns)
    entry := LogEntry{
        Timestamp:  r.Time,
        Level:      slogLevelToLogLevel(r.Level),
        Message:    r.Message,
        Source:     "slog",
        Attributes: extractAttributes(r, a.groups, a.attrs),
    }
    if a.addSource && r.PC != 0 {
        entry.Caller = formatCaller(r.PC)
    }
    a.capture(ctx, entry)

    // 2. Forward to inner handler ASYNCHRONOUSLY (avoids I/O blocking)
    a.forwarder.Forward(ctx, r, a.inner)

    // 3. Return immediately — caller is not blocked on inner handler I/O
    return nil
}

// Enabled, WithAttrs, WithGroup delegate to inner handler
func (a *SlogAdapter) Enabled(ctx context.Context, level slog.Level) bool {
    return a.inner.Enabled(ctx, level)
}

func (a *SlogAdapter) WithAttrs(attrs []slog.Attr) slog.Handler {
    return &SlogAdapter{
        inner:     a.inner.WithAttrs(attrs),
        capture:   a.capture,
        forwarder: a.forwarder,
        addSource: a.addSource,
        groups:    a.groups,
        attrs:     append(a.attrs, attrs...),
    }
}

func (a *SlogAdapter) WithGroup(name string) slog.Handler {
    return &SlogAdapter{
        inner:     a.inner.WithGroup(name),
        capture:   a.capture,
        forwarder: a.forwarder,
        addSource: a.addSource,
        groups:    append(a.groups, name),
        attrs:     a.attrs,
    }
}
```

**Rationale:**
- Wrapping (not replacing) ensures logs still flow to stderr/file/etc — just asynchronously.
- The capture path (buffer append) is synchronous and fast — it's the data the profiler needs.
- The forwarding path (inner handler I/O) is asynchronous — this is what would block the request.
- Implementing the full `slog.Handler` interface makes it composable with other handlers.
- `WithAttrs`/`WithGroup` are tracked so captured entries include full attribute context.
- Source/caller extraction uses `r.PC` which is already captured by slog when `AddSource: true`.
- `Handle()` returns `nil` since errors from the inner handler are not actionable by the caller (they're handled best-effort in the forwarder goroutine).

**Trade-offs:**
- Log write errors from the inner handler are silently swallowed (acceptable for a profiler — the log is still captured in the profiler buffer).
- Logs may appear in the output file/stream slightly after the code that produced them returns (typically <1ms delay). This is invisible to users in practice.
- On application crash, buffered records in the channel may be lost. `Close()` must be called during graceful shutdown.


#### 3.7 Standard Log Adapter as io.Writer (with Async Forwarding)

**Decision:** Implement `io.Writer` that intercepts `log.SetOutput()` writes, captures synchronously, and forwards to the original writer asynchronously.

```go
// stdLogForwardRecord holds data for async forwarding to the original writer.
type stdLogForwardRecord struct {
    data     []byte
    original io.Writer
}

// StdLogAdapter captures standard log package output by wrapping the log output writer.
// Capture is synchronous; forwarding to the original writer is asynchronous.
type StdLogAdapter struct {
    original   io.Writer
    capture    CaptureFunc
    activeCtx  atomic.Pointer[context.Context]
    forwardCh  chan stdLogForwardRecord
    done       chan struct{}
}

func NewStdLogAdapter(capture CaptureFunc, bufferSize int) *StdLogAdapter {
    a := &StdLogAdapter{
        capture:   capture,
        forwardCh: make(chan stdLogForwardRecord, bufferSize),
        done:      make(chan struct{}),
    }
    go a.runForwarder()
    return a
}

func (a *StdLogAdapter) runForwarder() {
    defer close(a.done)
    for rec := range a.forwardCh {
        _, _ = rec.original.Write(rec.data)
    }
}

func (a *StdLogAdapter) Write(p []byte) (n int, err error) {
    // 1. Parse the log line (timestamp + message)
    msg := strings.TrimSpace(string(p))
    entry := LogEntry{
        Timestamp: time.Now(),
        Level:     LevelInfo, // standard log has no levels
        Message:   msg,
        Source:    "log",
    }

    // 2. Capture into LogBuffer (FAST — synchronous)
    a.capture(a.getActiveContext(), entry)

    // 3. Forward to original writer ASYNCHRONOUSLY
    // Copy the slice since the caller may reuse it
    data := make([]byte, len(p))
    copy(data, p)
    select {
    case a.forwardCh <- stdLogForwardRecord{data: data, original: a.original}:
        // Sent
    default:
        // Channel full — sync fallback
        _, _ = a.original.Write(p)
    }

    return len(p), nil
}

func (a *StdLogAdapter) Close() {
    close(a.forwardCh)
    <-a.done
}
```

**Challenge:** The standard `log` package's `Writer` interface doesn't accept a context. We solve this with an atomic context pointer (see section 3.8).

#### 3.8 Context Propagation for Standard Log

**Decision:** Use a goroutine-aware context store backed by the request middleware.

**Approach:** The collector's `SetupContext()` stores the context in a goroutine-local-safe map keyed by goroutine ID. However, goroutine IDs are not reliably accessible in Go. Instead, we use a simpler approach:

**Alternative approach (chosen):** The standard log adapter requires users to use the context-aware logging pattern. We provide a helper:

```go
// ContextLogger returns a *log.Logger that captures to the request's LogBuffer.
func ContextLogger(ctx context.Context) *log.Logger {
    buf := GetLogBuffer(ctx)
    if buf == nil {
        return log.Default()
    }
    return log.New(&contextLogWriter{ctx: ctx, capture: buf.capture}, "", log.LstdFlags)
}
```

**However**, for zero-friction capture of `log.Print()` (global logger), we use a different strategy:

The `StdLogAdapter` maintains a single active context reference (the most recent request's context). This works correctly for single-threaded servers and provides best-effort capture for concurrent servers. The trade-off is acceptable because:
1. The standard `log` package is mostly used in simple/legacy code.
2. Users who need precise per-request attribution should use `slog` or structured loggers.
3. For concurrent servers, entries may occasionally be attributed to the wrong request — this is documented as a known limitation.

```go
type StdLogAdapter struct {
    original   io.Writer
    capture    CaptureFunc
    activeCtx  atomic.Pointer[context.Context]
}

func (a *StdLogAdapter) SetActiveContext(ctx context.Context) {
    a.activeCtx.Store(&ctx)
}
```

The `LoggerCollector.SetupContext()` updates the active context on the standard log adapter each time a new request begins.


### 4. Data Structures

```go
// LogEntry represents a single captured log entry.
type LogEntry struct {
    Timestamp  time.Time         `json:"timestamp"`
    Level      LogLevel          `json:"level"`
    Message    string            `json:"message"`
    Source     string            `json:"source"`
    Attributes map[string]any    `json:"attributes,omitempty"`
    Caller     string            `json:"caller,omitempty"`
    Stack      string            `json:"stack,omitempty"`
}

// LoggerData is the top-level output returned by Collect().
type LoggerData struct {
    Entries    []LogEntry `json:"entries"`
    Summary    LogSummary `json:"summary"`
    Truncated  bool       `json:"truncated"`
    MaxEntries int        `json:"max_entries"`
}

// LogSummary holds per-level counts.
type LogSummary struct {
    Total int `json:"total"`
    Debug int `json:"debug"`
    Info  int `json:"info"`
    Warn  int `json:"warn"`
    Error int `json:"error"`
    Fatal int `json:"fatal"`
}
```

### 5. Options Design

```go
type loggerOptions struct {
    adapters         []LogAdapter
    minLevel         LogLevel
    maxEntries       int
    callerInfo       bool
    stackTrace       bool
    slogDisabled     bool
    stdLogDisabled   bool
    attrMaxSize      int
    forwardBufSize   int  // buffered channel size for async forwarding
}

// Defaults
const (
    defaultMaxEntries    = 1000
    defaultAttrMaxSize   = 1024 // bytes
    defaultMinLevel      = LevelDebug
    defaultForwardBufSize = 4096 // channel capacity for async log forwarding
)

type LoggerOption func(*loggerOptions)

func WithAdapter(adapter LogAdapter) LoggerOption {
    return func(o *loggerOptions) {
        o.adapters = append(o.adapters, adapter)
    }
}

func WithMinLevel(level LogLevel) LoggerOption {
    return func(o *loggerOptions) {
        o.minLevel = level
    }
}

func WithMaxEntries(n int) LoggerOption {
    return func(o *loggerOptions) {
        o.maxEntries = n
    }
}

func WithCallerInfo(enabled bool) LoggerOption {
    return func(o *loggerOptions) {
        o.callerInfo = enabled
    }
}

func WithStackTrace(enabled bool) LoggerOption {
    return func(o *loggerOptions) {
        o.stackTrace = enabled
    }
}

func WithoutSlog() LoggerOption {
    return func(o *loggerOptions) {
        o.slogDisabled = true
    }
}

func WithoutStdLog() LoggerOption {
    return func(o *loggerOptions) {
        o.stdLogDisabled = true
    }
}

func WithAttributeMaxSize(bytes int) LoggerOption {
    return func(o *loggerOptions) {
        o.attrMaxSize = bytes
    }
}

// WithForwardBufferSize sets the buffered channel capacity for async log forwarding.
// Higher values absorb larger bursts but use more memory. Default: 4096.
func WithForwardBufferSize(size int) LoggerOption {
    return func(o *loggerOptions) {
        o.forwardBufSize = size
    }
}
```


### 6. Collector Lifecycle

```go
func NewLoggerCollector(opts ...LoggerOption) *LoggerCollector {
    // 1. Apply options with defaults
    // 2. Create LogForwarder (background goroutine with buffered channel)
    // 3. Build CaptureFunc (context-aware, respects minLevel, maxEntries)
    // 4. Install built-in adapters (slog, stdlog) unless disabled — pass forwarder
    // 5. Install user-provided adapters
    // 6. Store RemoveFuncs for cleanup
    return collector
}

func (c *LoggerCollector) Name() string { return "logger" }

func (c *LoggerCollector) SetupContext(ctx context.Context) context.Context {
    // 1. Create new LogBuffer with configured maxEntries
    // 2. Store in context via WithLogBuffer()
    // 3. Update StdLogAdapter's active context (if enabled)
    buf := NewLogBuffer(c.opts.maxEntries)
    ctx = WithLogBuffer(ctx, buf)
    if c.stdLogAdapter != nil {
        c.stdLogAdapter.SetActiveContext(ctx)
    }
    return ctx
}

func (c *LoggerCollector) Collect(ctx context.Context, req *http.Request, res ResponseData) (any, error) {
    // 1. Retrieve LogBuffer from context
    buf := GetLogBuffer(ctx)
    if buf == nil {
        return &LoggerData{MaxEntries: c.opts.maxEntries}, nil
    }

    // 2. Drain entries from buffer
    entries, truncated := buf.Drain()

    // 3. Truncate oversized attributes (lazy serialization)
    for i := range entries {
        entries[i].Attributes = truncateAttributes(entries[i].Attributes, c.opts.attrMaxSize)
    }

    // 4. Build summary
    summary := buildSummary(entries)

    // 5. Return LoggerData
    return &LoggerData{
        Entries:    entries,
        Summary:    summary,
        Truncated:  truncated,
        MaxEntries: c.opts.maxEntries,
    }, nil
}

func (c *LoggerCollector) Reset() {
    // No-op — state is per-request in context, not in the collector struct
}

func (c *LoggerCollector) PanelMeta() PanelMeta {
    return PanelMeta{
        Name:      "logger",
        Label:     "Logs",
        Icon:      "file-text",
        Component: "LoggerPanel",
    }
}

// Close removes all installed adapters, stops the forwarder goroutine,
// and drains any pending log records. Must be called on application shutdown.
func (c *LoggerCollector) Close() {
    // 1. Remove adapters (restores original loggers)
    for _, remove := range c.removeFuncs {
        remove()
    }
    // 2. Close forwarder — drains pending records, then stops goroutine
    c.forwarder.Close()
    // 3. Close stdlog forwarder if enabled
    if c.stdLogAdapter != nil {
        c.stdLogAdapter.Close()
    }
}
```

### 7. CaptureFunc Implementation

The `CaptureFunc` is the bridge between adapters and the per-request buffer:

```go
func (c *LoggerCollector) buildCaptureFunc() CaptureFunc {
    return func(ctx context.Context, entry LogEntry) {
        // 1. Check min level filter
        if entry.Level < c.opts.minLevel {
            return
        }

        // 2. Retrieve buffer from context
        buf := GetLogBuffer(ctx)
        if buf == nil {
            return // No active request — discard
        }

        // 3. Append entry (buffer handles max cap internally)
        buf.Append(entry)
    }
}
```

### 8. Attribute Handling

Structured loggers produce key-value attributes. These are captured as `map[string]any` during log emission and truncated during `Collect()`:

```go
func truncateAttributes(attrs map[string]any, maxSize int) map[string]any {
    if attrs == nil {
        return nil
    }
    for k, v := range attrs {
        if s, ok := v.(string); ok && len(s) > maxSize {
            attrs[k] = s[:maxSize] + "...(truncated)"
        }
    }
    return attrs
}
```

**Slog attribute extraction:**
```go
func extractAttributes(r slog.Record, groups []string, preAttrs []slog.Attr) map[string]any {
    attrs := make(map[string]any)

    // Add pre-existing attrs from WithAttrs()
    for _, a := range preAttrs {
        key := buildKey(groups, a.Key)
        attrs[key] = a.Value.Any()
    }

    // Add record-level attrs
    r.Attrs(func(a slog.Attr) bool {
        key := buildKey(groups, a.Key)
        attrs[key] = a.Value.Any()
        return true
    })

    if len(attrs) == 0 {
        return nil
    }
    return attrs
}

func buildKey(groups []string, key string) string {
    if len(groups) == 0 {
        return key
    }
    return strings.Join(groups, ".") + "." + key
}
```


### 9. Adapter Examples (Documentation, Not Code in Repo)

Users implement the `LogAdapter` interface in their application code for third-party loggers:

**Zap adapter:**
```go
import "go.uber.org/zap"

type ZapAdapter struct {
    original *zap.Logger
}

func (a *ZapAdapter) Name() string { return "zap" }

func (a *ZapAdapter) Install(capture CaptureFunc) RemoveFunc {
    // Replace global zap logger with a wrapper that captures entries
    core := &zapCaptureCore{inner: a.original.Core(), capture: capture}
    wrapped := zap.New(core)
    zap.ReplaceGlobals(wrapped)
    return func() {
        zap.ReplaceGlobals(a.original)
    }
}
```

**Zerolog adapter:**
```go
import "github.com/rs/zerolog"

type ZerologAdapter struct{}

func (a *ZerologAdapter) Name() string { return "zerolog" }

func (a *ZerologAdapter) Install(capture CaptureFunc) RemoveFunc {
    // Use zerolog's Hook interface to intercept log events
    hook := &zerologCaptureHook{capture: capture}
    // Users configure their logger with: logger = logger.Hook(hook)
    // This adapter provides the hook; installation is user-managed
    return func() {} // no-op — user removes hook from their logger
}
```

**Logrus adapter:**
```go
import "github.com/sirupsen/logrus"

type LogrusAdapter struct{}

func (a *LogrusAdapter) Name() string { return "logrus" }

func (a *LogrusAdapter) Install(capture CaptureFunc) RemoveFunc {
    hook := &logrusCaptureHook{capture: capture}
    logrus.AddHook(hook)
    return func() {
        // logrus doesn't support hook removal — documented limitation
    }
}
```

### 10. UI Panel Design

**LoggerPanel.vue** structure:

```
┌─────────────────────────────────────────────────────────────────┐
│ Summary Bar                                                      │
│ [42 total] [DEBUG: 5] [INFO: 28] [WARN: 6] [ERROR: 3] [FATAL:0]│
│ [⚠ Truncated: 1000 max]                                         │
├─────────────────────────────────────────────────────────────────┤
│ Filters: [DEBUG ✓] [INFO ✓] [WARN ✓] [ERROR ✓] [FATAL ✓]      │
│ Search: [________________________]                               │
├─────────────────────────────────────────────────────────────────┤
│ Log Entries (scrollable)                                         │
│ ┌──────────────────────────────────────────────────────────────┐│
│ │ +0.2ms  INFO  [slog] handling request    path=/api/users     ││
│ │ +1.5ms  INFO  [slog] query executed      rows=15 dur=1.2ms  ││
│ │ +2.1ms  WARN  [slog] slow operation      dur=500ms           ││
│ │ +3.4ms  ERROR [slog] failed to send email                    ││
│ │         ├─ error: "connection refused"                        ││
│ │         ├─ to: "user@example.com"                            ││
│ │         └─ caller: mail/sender.go:45                         ││
│ │ +4.0ms  INFO  [log]  processing complete                     ││
│ └──────────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────────┘
```

**Level color coding:**
- DEBUG: gray/muted
- INFO: blue
- WARN: yellow/amber
- ERROR: red
- FATAL: dark red / bold red

**Entry layout:**
- Compact single-line by default (timestamp offset, level badge, source tag, message, inline attrs)
- Expandable on click to show full attributes, caller, and stack trace
- ERROR/FATAL entries have a subtle red left border

**Search/filter:**
- Level toggles: click to include/exclude each level
- Text search: client-side filter on message + attribute values (case-insensitive)
- Both filters combine (AND logic)

### 11. Integration Points

1. **Collector registration** — users add it like any other collector:
   ```go
   p.AddCollector(collector.NewLoggerCollector(
       collector.WithMinLevel(collector.LevelInfo),
       collector.WithMaxEntries(500),
   ))
   ```

2. **Slog setup** — users wrap their slog handler:
   ```go
   // The collector handles this internally during Install()
   // Original: slog.SetDefault(slog.New(jsonHandler))
   // After:    slog.SetDefault(slog.New(slogAdapter.Wrap(jsonHandler)))
   ```

3. **Profile data** — stored in `Profile.CollectorData["logger"]` as `*LoggerData`.

4. **API** — served via existing `GET /api/profiles/{id}` endpoint, no changes needed.

5. **UI panel** — registered in `builtin.ts`:
   ```ts
   import LoggerPanel from '../components/panels/LoggerPanel.vue'
   registerPanel('logger', LoggerPanel)
   ```

6. **Shutdown** — users should call `collector.Close()` on application shutdown to restore original loggers.

### 12. Error Handling

- If `GetLogBuffer(ctx)` returns nil (no active request), entries are silently discarded.
- If an adapter panics during capture, the panic is recovered in the `CaptureFunc` wrapper — the entry is lost but the application continues.
- If `LogBuffer.Append()` exceeds max entries, the entry is dropped and `truncated` flag is set.
- Attribute extraction errors (e.g., non-serializable values) result in the attribute being stored as its `fmt.Sprint()` string representation.
- `Collect()` never returns an error — it always produces valid `LoggerData` (possibly empty).

### 13. Lifecycle and Cleanup

```
Application Start:
  1. NewLoggerCollector(opts...) — creates LogForwarder goroutine, installs adapters
  2. p.AddCollector(loggerCollector) — registers with profiler

Per Request:
  1. SetupContext(ctx) — creates LogBuffer, stores in ctx
  2. Handler executes — log calls:
     a. Capture to LogBuffer (sync, ~50ns, on request goroutine)
     b. Forward record to channel (async, ~100ns, non-blocking)
  3. LogForwarder goroutine drains channel → calls inner handlers (I/O off hot path)
  4. Collect(ctx, req, res) — drains buffer, builds LoggerData

Application Shutdown:
  1. loggerCollector.Close():
     a. Remove adapters (restores original loggers — no more forwarding)
     b. Close forwarder channel (signals goroutine to drain & stop)
     c. Wait for forwarder goroutine to flush all pending records
     d. Close stdlog forwarder similarly
```

This ensures:
- Request hot path never blocks on log I/O (file writes, network sends, stdout locks).
- Logs still reach their original destination (just slightly delayed, typically <1ms).
- Graceful shutdown flushes all pending records before the process exits.
- No goroutine leaks — forwarder goroutine exits cleanly when channel is closed.


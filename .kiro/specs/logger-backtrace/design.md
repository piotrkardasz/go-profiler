# Design: Logger Backtrace

## Technical Design Document

### 1. Overview

This feature adds call stack backtrace capture to the logger collector. When enabled, each log entry's `Stack` field is populated with a formatted call stack trace showing the application code path that led to the log call. The design mirrors the existing GORM collector's backtrace implementation.

### 2. Architecture

```
┌────────────────────────────────────────────────────────────────┐
│  Application Code                                               │
│  └─► slog.Info("handling users request", "method", "GET")      │
│      └─► SlogAdapter.Handle(ctx, record)                       │
│          ├─ Build LogEntry (message, level, attrs, caller)     │
│          ├─ IF backtrace enabled:                              │
│          │   └─ entry.Stack = CaptureLogBacktrace()            │
│          │       └─ runtime.Callers(skip, pcs)                 │
│          │       └─ runtime.CallersFrames(pcs)                 │
│          │       └─ Filter internal frames                     │
│          │       └─ Format "file:line function" (max 10)       │
│          ├─ capture(ctx, entry) → LogBuffer.Append()           │
│          └─ forwarder.Forward(ctx, r, inner)                   │
└────────────────────────────────────────────────────────────────┘
```

### 3. Design Decisions

#### 3.1 Reuse Existing `Stack` Field

**Decision:** Store the backtrace in the existing `LogEntry.Stack` field rather than adding a new field.

**Rationale:**
- The `Stack` field already exists in `LogEntry` and is rendered by the UI in a collapsible `<pre>` block.
- The requirements spec (FR-4.2 of the original logger-collector spec) already specifies that the Stack field should be displayed as a collapsible code block.
- No schema changes needed — existing JSON serialization and UI rendering work as-is.
- The `Stack` field stores a single string with newline-separated frames (not a slice), which is appropriate for display in a `<pre>` block.

#### 3.2 Backtrace as String (Not Slice)

**Decision:** Format the backtrace as a newline-separated string rather than a `[]string` slice.

**Rationale:**
- The `LogEntry.Stack` field is already typed as `string` — changing it would be a breaking change.
- The UI already renders it in a `<pre>` block which naturally displays newline-separated content.
- Each frame is formatted as `file:line function` (one per line), matching the GORM collector's format visually while adapting to the string type.

#### 3.3 Configuration via Option + Environment Variable

**Decision:** Add a `WithBacktrace(bool)` option and support the `PROFILER_LOGGER_BACKTRACE` environment variable.

```go
// In loggerOptions struct:
type loggerOptions struct {
    // ... existing fields ...
    backtraceEnabled *bool  // nil = check env, non-nil = explicit setting
}

// WithBacktrace enables or disables call stack backtrace capture for log entries.
func WithBacktrace(enabled bool) LoggerOption {
    return func(o *loggerOptions) {
        o.backtraceEnabled = &enabled
    }
}

// isBacktraceEnabled returns whether backtrace capture is enabled.
// Checks explicit option first, then falls back to environment variable.
func (o *loggerOptions) isBacktraceEnabled() bool {
    if o.backtraceEnabled != nil {
        return *o.backtraceEnabled
    }
    env := os.Getenv("PROFILER_LOGGER_BACKTRACE")
    return env == "true" || env == "1"
}
```

**Rationale:**
- Mirrors the GORM collector's pattern (`GORM_PROFILER_BACKTRACE` env var + programmatic option).
- `*bool` (nil/true/false) distinguishes "not set" from "explicitly disabled".
- Environment variable provides zero-code-change enablement for debugging.

#### 3.4 Backtrace Capture Function

**Decision:** Implement an exported `CaptureLogBacktrace()` function that can be called from any adapter.

```go
// CaptureLogBacktrace captures the current call stack, filtering out internal
// frames from the collector, runtime, and logging library packages.
// Returns a newline-separated string of "file:line function" entries (max 10 frames).
// Returns empty string if no meaningful frames are found.
func CaptureLogBacktrace() string {
    const maxDepth = 32
    pcs := make([]uintptr, maxDepth)
    n := runtime.Callers(3, pcs) // skip: Callers, CaptureLogBacktrace, adapter.Handle/Write
    if n == 0 {
        return ""
    }

    frames := runtime.CallersFrames(pcs[:n])
    var trace []string

    for {
        frame, more := frames.Next()

        if isLogInternalFrame(frame.Function) {
            if !more {
                break
            }
            continue
        }

        trace = append(trace, fmt.Sprintf("%s:%d %s", frame.File, frame.Line, frame.Function))

        if len(trace) >= 10 || !more {
            break
        }
    }

    return strings.Join(trace, "\n")
}
```

**Rationale:**
- Exported so custom `LogAdapter` implementations can call it to add backtraces to their entries.
- Uses the same pattern as `captureBacktrace()` in the GORM collector.
- Returns a formatted string (not slice) to match the `LogEntry.Stack` string type.
- Skip count of 3 accounts for: `runtime.Callers`, `CaptureLogBacktrace`, and the immediate caller (adapter code).

#### 3.5 Internal Frame Filtering

**Decision:** Filter frames matching collector internals, runtime, and logging packages.

```go
func isLogInternalFrame(function string) bool {
    return strings.Contains(function, "go-profiler/collector.") ||
        strings.Contains(function, "runtime.") ||
        strings.Contains(function, "log/slog.") ||
        strings.Contains(function, "testing.") ||
        strings.Contains(function, "net/http.") ||
        strings.HasPrefix(function, "log.")
}
```

**Rationale:**
- Filters the profiler's own code so users see only their application frames.
- Filters `runtime.*` (goroutine setup, scheduler) for cleaner traces.
- Filters `log/slog.*` internals (handler chains, record creation).
- Filters `log.*` (standard log package internals).
- Keeps `net/http` filtering to remove generic server frames but can be refined.
- Note: We do NOT filter `net/http` fully — the middleware and handler chain are useful context. We only filter deep server internals (`net/http.(*conn).serve`, `net/http.serverHandler.ServeHTTP`).

**Refined approach:**
```go
func isLogInternalFrame(function string) bool {
    return strings.Contains(function, "go-profiler/collector.") ||
        strings.Contains(function, "go-profiler/collector/") ||
        strings.Contains(function, "runtime.") ||
        strings.Contains(function, "log/slog.") ||
        strings.HasPrefix(function, "log.")
}
```

This keeps user handlers and HTTP handler code in the trace while filtering only the profiler machinery and Go internals.

#### 3.6 Integration with Adapters

**Slog adapter:**
```go
func (a *SlogAdapter) Handle(ctx context.Context, r slog.Record) error {
    entry := LogEntry{
        Timestamp:  r.Time,
        Level:      slogLevelToLogLevel(r.Level),
        Message:    r.Message,
        Source:     "slog",
        Attributes: extractSlogAttributes(r, a.groups, a.attrs),
    }

    if a.addSource && r.PC != 0 {
        entry.Caller = formatCaller(r.PC)
    }

    // NEW: Capture backtrace if enabled
    if a.backtrace {
        entry.Stack = CaptureLogBacktrace()
    }

    a.capture(ctx, entry)
    a.forwarder.Forward(ctx, r, a.inner)
    return nil
}
```

**StdLog adapter:**
```go
func (a *StdLogAdapter) Write(p []byte) (int, error) {
    msg := strings.TrimSpace(string(p))
    entry := LogEntry{
        Timestamp: time.Now(),
        Level:     LevelInfo,
        Message:   msg,
        Source:    "log",
    }

    // NEW: Capture backtrace if enabled
    if a.backtrace {
        entry.Stack = CaptureLogBacktrace()
    }

    // ... rest of capture and forwarding logic
}
```

#### 3.7 Passing Backtrace Setting to Adapters

**Decision:** Pass the resolved backtrace boolean to each adapter at construction time.

The `LoggerCollector` resolves the backtrace setting once during `NewLoggerCollector()` and passes it to each adapter. This avoids repeated env var lookups on every log call.

```go
func NewLoggerCollector(opts ...LoggerOption) *LoggerCollector {
    // ... apply options ...
    
    backtraceEnabled := o.isBacktraceEnabled()
    
    // Install slog adapter
    if !o.slogDisabled {
        adapter := &slogLogAdapter{
            addSource: o.callerInfo,
            backtrace: backtraceEnabled,
        }
        // ...
    }
    
    // Install stdlog adapter
    if !o.stdLogDisabled {
        adapter := &stdLogLogAdapter{
            bufferSize: o.forwardBufSize,
            backtrace:  backtraceEnabled,
        }
        // ...
    }
}
```

### 4. File Changes

```
collector/
├── logger.go             # Add WithBacktrace option, pass backtrace flag to adapters
├── logger_adapter.go     # No changes (uses existing Stack field)
├── logger_backtrace.go   # NEW: CaptureLogBacktrace(), isLogInternalFrame()
├── logger_slog.go        # Add backtrace field, call CaptureLogBacktrace() in Handle()
├── logger_stdlog.go      # Add backtrace field, call CaptureLogBacktrace() in Write()
├── logger_backtrace_test.go  # NEW: unit tests for backtrace capture

ui/
└── (no changes — existing LoggerPanel already renders entry.stack)
```

### 5. Performance Considerations

- When `backtrace` is `false` (default): zero overhead — the `if a.backtrace` check is a simple boolean comparison.
- When `backtrace` is `true`: `runtime.Callers()` + `runtime.CallersFrames()` adds ~2-5 microseconds per log entry. This is acceptable for debugging scenarios where backtrace is explicitly enabled.
- The `pcs` slice is allocated per call (32 entries × 8 bytes = 256 bytes). For high-frequency logging with backtrace enabled, this could be optimized with `sync.Pool` in the future, but is acceptable for the initial implementation.

### 6. Example Output

With backtrace enabled, a log entry's `Stack` field would contain:
```
/workspaces/go-profiler/examples/basic/main.go:45 main.handleUsers
/workspaces/go-profiler/examples/basic/main.go:32 main.main.func1
```

This tells the developer exactly which handler and function produced the log entry.

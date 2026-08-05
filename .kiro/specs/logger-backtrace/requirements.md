# Requirements: Logger Backtrace

## Overview

Add call stack backtrace capture to the logger collector so that each log entry can optionally include the full call stack showing where the log call originated. This gives developers visibility into the execution path that led to each log message, similar to the existing backtrace support in the GORM collector.

## Functional Requirements

### FR-1: Backtrace Capture

- FR-1.1: The logger collector MUST support capturing a call stack backtrace for each log entry.
- FR-1.2: The backtrace MUST be stored in the existing `LogEntry.Stack` field as a formatted string (one frame per line).
- FR-1.3: The backtrace MUST filter out internal frames from the collector package, the `runtime` package, `log/slog` internals, and the standard `log` package internals.
- FR-1.4: The backtrace MUST include at most 10 meaningful frames (matching the GORM collector's limit).
- FR-1.5: Each frame MUST be formatted as `<file>:<line> <function>` (matching the GORM collector's format).
- FR-1.6: The backtrace MUST be captured at the point where the log entry is produced (inside the adapter's capture path).

### FR-2: Configuration

- FR-2.1: Backtrace capture MUST be disabled by default (to avoid performance overhead when not needed).
- FR-2.2: The collector MUST provide a `WithBacktrace(enabled bool)` functional option to enable backtrace capture programmatically.
- FR-2.3: The collector MUST support enabling backtrace via the `PROFILER_LOGGER_BACKTRACE` environment variable (set to "true" or "1").
- FR-2.4: The programmatic option (`WithBacktrace`) MUST take precedence over the environment variable when explicitly set.
- FR-2.5: The existing `WithCallerInfo` option MUST continue to work independently — `Caller` (single file:line) and `Stack` (full backtrace) are orthogonal features.

### FR-3: Adapter Support

- FR-3.1: The slog adapter MUST capture backtrace when enabled, using the `slog.Record`'s PC as the starting point to build a meaningful stack.
- FR-3.2: The standard log adapter MUST capture backtrace when enabled, starting from the `Write()` call site.
- FR-3.3: Custom adapters (via `LogAdapter` interface) MUST be able to opt into backtrace capture by calling a helper function provided by the collector package.
- FR-3.4: The backtrace capture helper MUST be exported: `CaptureLogBacktrace() string`.

### FR-4: UI Display

- FR-4.1: The existing LoggerPanel MUST already support displaying the `Stack` field in a collapsible code block (per existing FR-10.10 in logger-collector spec).
- FR-4.2: The backtrace MUST be displayed when available, using the existing stack trace expandable UI.
- FR-4.3: No additional UI changes are required since the `LoggerPanel` already renders `entry.stack` in a collapsible `<pre>` block.

## Non-Functional Requirements

### NFR-1: Performance

- NFR-1.1: When backtrace is disabled (default), there MUST be zero additional overhead on log capture.
- NFR-1.2: When backtrace is enabled, the overhead per log entry MUST be < 5 microseconds (stack unwinding is inherently expensive).
- NFR-1.3: The frame skipping logic MUST minimize the number of frames collected by `runtime.Callers` (use appropriate skip count).

### NFR-2: Consistency

- NFR-2.1: The backtrace format MUST match the GORM collector's format (`file:line function`) for a consistent developer experience across panels.
- NFR-2.2: The internal frame filtering MUST be comprehensive enough to hide all profiler/adapter machinery and show only application code.

### NFR-3: Compatibility

- NFR-3.1: MUST NOT break any existing logger collector tests or behavior.
- NFR-3.2: MUST NOT change the `LogEntry` struct layout (uses existing `Stack` field).
- NFR-3.3: MUST NOT require changes to the `LogAdapter` interface.

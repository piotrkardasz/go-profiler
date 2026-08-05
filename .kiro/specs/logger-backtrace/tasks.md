# Tasks: Logger Backtrace

## Implementation Tasks

### Task 1: Add backtrace configuration option

**Objective:** Add the `WithBacktrace` option and environment variable support to the logger collector.

**Implementation:**
- Add `backtraceEnabled *bool` field to `loggerOptions` struct
- Add `WithBacktrace(enabled bool) LoggerOption` function
- Add `isBacktraceEnabled() bool` method to `loggerOptions` that checks the explicit option first, then falls back to `PROFILER_LOGGER_BACKTRACE` environment variable
- Resolve backtrace setting in `NewLoggerCollector()` and pass to adapters

**Files to modify:**
- `collector/logger.go`

---

### Task 2: Implement CaptureLogBacktrace helper

**Objective:** Create the exported backtrace capture function and internal frame filter.

**Implementation:**
- `CaptureLogBacktrace() string` — exported function that captures the call stack:
  - Uses `runtime.Callers(3, pcs)` to skip internal frames (Callers, CaptureLogBacktrace, adapter)
  - Iterates frames via `runtime.CallersFrames()`
  - Filters internal frames via `isLogInternalFrame()`
  - Formats each frame as `file:line function`
  - Returns newline-separated string (max 10 frames)
  - Returns empty string if no meaningful frames found
- `isLogInternalFrame(function string) bool` — unexported helper that returns true for:
  - `go-profiler/collector.` (this package)
  - `go-profiler/collector/` (sub-packages like gorm)
  - `runtime.` (Go runtime)
  - `log/slog.` (slog internals)
  - `log.` prefix (standard log internals)
  - `testing.` (test framework)

**Files to create:**
- `collector/logger_backtrace.go`

---

### Task 3: Integrate backtrace into slog adapter

**Objective:** Capture backtrace in the slog adapter when enabled.

**Implementation:**
- Add `backtrace bool` field to `SlogAdapter` struct
- Add `backtrace bool` field to `slogLogAdapter` struct
- Update `SlogAdapter.Handle()` to call `CaptureLogBacktrace()` when `a.backtrace` is true and store result in `entry.Stack`
- Update `NewSlogAdapter()` to accept backtrace parameter
- Update `WithAttrs()` and `WithGroup()` to propagate the backtrace field
- Update `slogLogAdapter.Install()` to pass backtrace to `NewSlogAdapter()`
- Update `NewLoggerCollector()` to set `adapter.backtrace = backtraceEnabled` for the slog adapter

**Files to modify:**
- `collector/logger_slog.go`
- `collector/logger.go`

---

### Task 4: Integrate backtrace into standard log adapter

**Objective:** Capture backtrace in the stdlog adapter when enabled.

**Implementation:**
- Add `backtrace bool` field to `StdLogAdapter` struct
- Add `backtrace bool` field to `stdLogLogAdapter` struct
- Update `StdLogAdapter.Write()` to call `CaptureLogBacktrace()` when `a.backtrace` is true and store result in `entry.Stack`
- Update `NewStdLogAdapter()` to accept backtrace parameter
- Update `stdLogLogAdapter.Install()` to pass backtrace to `NewStdLogAdapter()`
- Update `NewLoggerCollector()` to set `adapter.backtrace = backtraceEnabled` for the stdlog adapter

**Files to modify:**
- `collector/logger_stdlog.go`
- `collector/logger.go`

---

### Task 5: Write unit tests for backtrace capture

**Objective:** Test the backtrace capture function and its integration with adapters.

**Tests:**
- `TestCaptureLogBacktrace`: verify it returns a non-empty string with expected format
- `TestCaptureLogBacktraceFormat`: verify each line matches `file:line function` pattern
- `TestCaptureLogBacktraceFiltersInternals`: verify collector/runtime/slog frames are excluded
- `TestCaptureLogBacktraceMaxFrames`: verify output is capped at 10 frames
- `TestIsLogInternalFrame`: verify known internal function names are filtered
- `TestSlogAdapterBacktraceEnabled`: verify Stack field is populated when backtrace=true
- `TestSlogAdapterBacktraceDisabled`: verify Stack field is empty when backtrace=false
- `TestStdLogAdapterBacktraceEnabled`: verify Stack field is populated when backtrace=true
- `TestStdLogAdapterBacktraceDisabled`: verify Stack field is empty when backtrace=false
- `TestWithBacktraceOption`: verify the option sets the field correctly
- `TestBacktraceEnvVariable`: verify PROFILER_LOGGER_BACKTRACE env var enables backtrace
- `TestBacktraceOptionOverridesEnv`: verify explicit option takes precedence over env var

**Files to create:**
- `collector/logger_backtrace_test.go`

---

### Task 6: Verification

**Objective:** Verify all components build and test correctly.

**Verification steps:**
- `go build ./...` — all modules build without errors
- `go test ./...` — all tests pass (existing + new backtrace tests)
- `go vet ./...` — no warnings
- `go test -race ./collector/...` — no race conditions
- Verify existing logger tests still pass unchanged
- Verify the UI already renders `entry.stack` (no UI changes needed)
- Verify `CaptureLogBacktrace` is exported and usable by custom adapter authors

---

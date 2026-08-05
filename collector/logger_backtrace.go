package collector

import (
	"fmt"
	"runtime"
	"strings"
)

// CaptureLogBacktrace captures the current call stack, filtering out internal
// frames from the collector, runtime, and logging library packages.
// Returns a newline-separated string of "file:line function" entries (max 10 frames).
// Returns an empty string if no meaningful frames are found.
//
// This function is exported so that custom LogAdapter implementations can use it
// to populate the LogEntry.Stack field with a backtrace.
func CaptureLogBacktrace() string {
	const maxDepth = 32
	pcs := make([]uintptr, maxDepth)
	n := runtime.Callers(2, pcs) // skip: Callers, CaptureLogBacktrace
	if n == 0 {
		return ""
	}

	frames := runtime.CallersFrames(pcs[:n])
	var trace []string

	for {
		frame, more := frames.Next()

		if isLogInternalFrame(frame.Function, frame.File) {
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

// isLogInternalFrame returns true if the frame belongs to the profiler's
// collector package implementation, Go runtime, or logging library internals.
// Test files (_test.go) within the collector package are NOT considered internal.
func isLogInternalFrame(function string, file string) bool {
	// Filter Go runtime internals.
	if strings.Contains(function, "runtime.") {
		return true
	}
	// Filter slog internals.
	if strings.Contains(function, "log/slog.") {
		return true
	}
	// Filter standard log package internals.
	if strings.HasPrefix(function, "log.") {
		return true
	}
	// Filter the testing framework's internal machinery (not test functions themselves).
	if function == "testing.tRunner" || strings.HasPrefix(function, "testing.(*") {
		return true
	}
	// Filter the profiler's collector package internals, but not test files.
	if strings.Contains(function, "go-profiler/collector.") ||
		strings.Contains(function, "go-profiler/collector/") {
		// Allow test file frames through.
		if strings.HasSuffix(file, "_test.go") {
			return false
		}
		return true
	}
	return false
}

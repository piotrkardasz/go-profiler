package http

import (
	"fmt"
	"runtime"
	"strings"
)

const maxBacktraceFrames = 10

// captureBacktrace captures the call stack, filtering out runtime and
// collector-internal frames. Returns up to 10 meaningful frames.
func captureBacktrace() []string {
	pcs := make([]uintptr, 32)
	n := runtime.Callers(3, pcs) // skip Callers, captureBacktrace, caller (RoundTrip)
	if n == 0 {
		return nil
	}

	frames := runtime.CallersFrames(pcs[:n])
	result := make([]string, 0, maxBacktraceFrames)

	for {
		frame, more := frames.Next()
		if !isInternalFrame(frame.Function, frame.File) {
			result = append(result, fmt.Sprintf("%s:%d %s", frame.File, frame.Line, frame.Function))
			if len(result) >= maxBacktraceFrames {
				break
			}
		}
		if !more {
			break
		}
	}

	return result
}

// isInternalFrame returns true for frames that should be filtered from backtraces.
func isInternalFrame(function, file string) bool {
	// Runtime internals
	if strings.HasPrefix(function, "runtime.") {
		return true
	}

	// net/http internals (transport machinery)
	if strings.HasPrefix(function, "net/http.") {
		return true
	}

	// This collector's own frames
	if strings.Contains(function, "go-profiler/collector/http.") {
		// Allow test files through
		if strings.HasSuffix(file, "_test.go") {
			return false
		}
		return true
	}

	// Testing framework
	if strings.HasPrefix(function, "testing.") {
		return true
	}

	return false
}

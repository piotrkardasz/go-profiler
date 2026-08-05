package collector

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"
)

func TestCaptureLogBacktrace(t *testing.T) {
	trace := CaptureLogBacktrace()
	if trace == "" {
		t.Fatal("expected non-empty backtrace")
	}
	// Should contain at least one frame with this test file.
	if !strings.Contains(trace, "logger_backtrace_test.go") {
		t.Errorf("expected backtrace to contain test file, got:\n%s", trace)
	}
}

func TestCaptureLogBacktraceFormat(t *testing.T) {
	trace := CaptureLogBacktrace()
	lines := strings.Split(trace, "\n")
	if len(lines) == 0 {
		t.Fatal("expected at least one frame")
	}
	for i, line := range lines {
		// Each line should match "file:line function" pattern.
		// At minimum it must contain a colon (file:line) and a space (before function).
		if !strings.Contains(line, ":") {
			t.Errorf("frame %d missing colon (file:line): %q", i, line)
		}
		if !strings.Contains(line, " ") {
			t.Errorf("frame %d missing space (before function): %q", i, line)
		}
	}
}

func TestCaptureLogBacktraceFiltersInternals(t *testing.T) {
	trace := CaptureLogBacktrace()
	lines := strings.Split(trace, "\n")
	for _, line := range lines {
		if strings.Contains(line, "runtime.") && !strings.Contains(line, "_test.go") {
			t.Errorf("expected runtime frames to be filtered, found: %q", line)
		}
		if strings.Contains(line, "go-profiler/collector.") && !strings.Contains(line, "_test.go") {
			t.Errorf("expected collector frames to be filtered, found: %q", line)
		}
	}
}

func TestCaptureLogBacktraceMaxFrames(t *testing.T) {
	trace := CaptureLogBacktrace()
	if trace == "" {
		t.Skip("no frames captured")
	}
	lines := strings.Split(trace, "\n")
	if len(lines) > 10 {
		t.Errorf("expected at most 10 frames, got %d", len(lines))
	}
}

func TestIsLogInternalFrame(t *testing.T) {
	tests := []struct {
		function string
		file     string
		internal bool
	}{
		{"runtime.goexit", "/usr/local/go/src/runtime/asm_amd64.s", true},
		{"runtime.main", "/usr/local/go/src/runtime/proc.go", true},
		{"log/slog.(*Logger).Info", "/usr/local/go/src/log/slog/logger.go", true},
		{"log.Printf", "/usr/local/go/src/log/log.go", true},
		{"testing.tRunner", "/usr/local/go/src/testing/testing.go", true},
		{"testing.(*T).Run", "/usr/local/go/src/testing/testing.go", true},
		{"github.com/piotrkardasz/go-profiler/collector.(*SlogAdapter).Handle", "collector/logger_slog.go", true},
		{"github.com/piotrkardasz/go-profiler/collector/gorm.captureBacktrace", "collector/gorm/plugin.go", true},
		{"github.com/piotrkardasz/go-profiler/collector.TestCaptureLogBacktrace", "collector/logger_backtrace_test.go", false},
		{"main.handleUsers", "main.go", false},
		{"myapp/handlers.GetUsers", "handlers/users.go", false},
		{"net/http.HandlerFunc.ServeHTTP", "/usr/local/go/src/net/http/server.go", false},
	}

	for _, tt := range tests {
		t.Run(tt.function, func(t *testing.T) {
			got := isLogInternalFrame(tt.function, tt.file)
			if got != tt.internal {
				t.Errorf("isLogInternalFrame(%q, %q) = %v, want %v", tt.function, tt.file, got, tt.internal)
			}
		})
	}
}

func TestSlogAdapterBacktraceEnabled(t *testing.T) {
	inner := newTestHandler()
	forwarder := NewLogForwarder(64)
	captured := &capturedEntries{}

	adapter := NewSlogAdapter(inner, captured.capture, forwarder, false, true)

	buf := NewLogBuffer(100)
	ctx := WithLogBuffer(context.Background(), buf)

	r := slog.NewRecord(time.Now(), slog.LevelInfo, "with backtrace", 0)
	if err := adapter.Handle(ctx, r); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	forwarder.Close()

	entries := captured.get()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Stack == "" {
		t.Error("expected non-empty Stack when backtrace is enabled")
	}
	// Stack should contain meaningful frames.
	if !strings.Contains(entries[0].Stack, "logger_backtrace_test.go") {
		t.Errorf("expected Stack to reference test file, got:\n%s", entries[0].Stack)
	}
}

func TestSlogAdapterBacktraceDisabled(t *testing.T) {
	inner := newTestHandler()
	forwarder := NewLogForwarder(64)
	captured := &capturedEntries{}

	adapter := NewSlogAdapter(inner, captured.capture, forwarder, false, false)

	buf := NewLogBuffer(100)
	ctx := WithLogBuffer(context.Background(), buf)

	r := slog.NewRecord(time.Now(), slog.LevelInfo, "no backtrace", 0)
	if err := adapter.Handle(ctx, r); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	forwarder.Close()

	entries := captured.get()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Stack != "" {
		t.Errorf("expected empty Stack when backtrace is disabled, got: %q", entries[0].Stack)
	}
}

func TestStdLogAdapterBacktraceEnabled(t *testing.T) {
	cap := &stdlogCaptured{}
	tw := &testWriter{}

	a := NewStdLogAdapter(cap.capture, 64)
	a.original = tw
	a.backtrace = true

	buf := NewLogBuffer(100)
	ctx := WithLogBuffer(context.Background(), buf)
	a.SetActiveContext(ctx)

	a.Write([]byte("hello with trace\n"))
	a.Close()

	entries := cap.get()
	if len(entries) == 0 {
		t.Fatal("expected at least one captured entry")
	}
	if entries[0].Stack == "" {
		t.Error("expected non-empty Stack when backtrace is enabled")
	}
	if !strings.Contains(entries[0].Stack, "logger_backtrace_test.go") {
		t.Errorf("expected Stack to reference test file, got:\n%s", entries[0].Stack)
	}
}

func TestStdLogAdapterBacktraceDisabled(t *testing.T) {
	cap := &stdlogCaptured{}
	tw := &testWriter{}

	a := NewStdLogAdapter(cap.capture, 64)
	a.original = tw
	a.backtrace = false

	buf := NewLogBuffer(100)
	ctx := WithLogBuffer(context.Background(), buf)
	a.SetActiveContext(ctx)

	a.Write([]byte("hello no trace\n"))
	a.Close()

	entries := cap.get()
	if len(entries) == 0 {
		t.Fatal("expected at least one captured entry")
	}
	if entries[0].Stack != "" {
		t.Errorf("expected empty Stack when backtrace is disabled, got: %q", entries[0].Stack)
	}
}

func TestWithBacktraceOption(t *testing.T) {
	o := &loggerOptions{}

	if o.backtraceEnabled != nil {
		t.Error("expected backtraceEnabled to be nil by default")
	}

	WithBacktrace(true)(o)
	if o.backtraceEnabled == nil || !*o.backtraceEnabled {
		t.Error("expected backtraceEnabled to be true after WithBacktrace(true)")
	}

	WithBacktrace(false)(o)
	if o.backtraceEnabled == nil || *o.backtraceEnabled {
		t.Error("expected backtraceEnabled to be false after WithBacktrace(false)")
	}
}

func TestBacktraceEnvVariable(t *testing.T) {
	// Test with env var set to "true".
	os.Setenv("PROFILER_LOGGER_BACKTRACE", "true")
	defer os.Unsetenv("PROFILER_LOGGER_BACKTRACE")

	o := &loggerOptions{}
	if !o.isBacktraceEnabled() {
		t.Error("expected isBacktraceEnabled() to return true when env is 'true'")
	}

	// Test with env var set to "1".
	os.Setenv("PROFILER_LOGGER_BACKTRACE", "1")
	if !o.isBacktraceEnabled() {
		t.Error("expected isBacktraceEnabled() to return true when env is '1'")
	}

	// Test with env var unset.
	os.Unsetenv("PROFILER_LOGGER_BACKTRACE")
	if o.isBacktraceEnabled() {
		t.Error("expected isBacktraceEnabled() to return false when env is unset")
	}
}

func TestBacktraceOptionOverridesEnv(t *testing.T) {
	os.Setenv("PROFILER_LOGGER_BACKTRACE", "true")
	defer os.Unsetenv("PROFILER_LOGGER_BACKTRACE")

	// Explicit false should override env var.
	o := &loggerOptions{}
	WithBacktrace(false)(o)
	if o.isBacktraceEnabled() {
		t.Error("expected explicit WithBacktrace(false) to override env var")
	}

	// Explicit true should also work regardless of env.
	os.Unsetenv("PROFILER_LOGGER_BACKTRACE")
	o2 := &loggerOptions{}
	WithBacktrace(true)(o2)
	if !o2.isBacktraceEnabled() {
		t.Error("expected explicit WithBacktrace(true) to enable backtrace")
	}
}

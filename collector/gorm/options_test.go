package gormcollector

import (
	"os"
	"testing"
	"time"
)

func TestDefaultOptions(t *testing.T) {
	opts := defaultOptions()

	if opts.SlowThreshold != DefaultSlowThreshold {
		t.Errorf("expected slow threshold %v, got %v", DefaultSlowThreshold, opts.SlowThreshold)
	}
	if opts.N1Threshold != DefaultN1Threshold {
		t.Errorf("expected N+1 threshold %d, got %d", DefaultN1Threshold, opts.N1Threshold)
	}
	if len(opts.Connections) != 0 {
		t.Errorf("expected 0 connections, got %d", len(opts.Connections))
	}
	if opts.BacktraceEnabled != nil {
		t.Error("expected BacktraceEnabled to be nil (use env)")
	}
}

func TestWithSlowThresholdOption(t *testing.T) {
	opts := defaultOptions()
	WithSlowThreshold(200 * time.Millisecond)(opts)

	if opts.SlowThreshold != 200*time.Millisecond {
		t.Errorf("expected 200ms, got %v", opts.SlowThreshold)
	}
}

func TestWithN1ThresholdOption(t *testing.T) {
	opts := defaultOptions()
	WithN1Threshold(3)(opts)

	if opts.N1Threshold != 3 {
		t.Errorf("expected N+1 threshold 3, got %d", opts.N1Threshold)
	}
}

func TestWithBacktraceOption(t *testing.T) {
	opts := defaultOptions()
	WithBacktrace(true)(opts)

	if opts.BacktraceEnabled == nil || !*opts.BacktraceEnabled {
		t.Error("expected backtrace enabled")
	}

	WithBacktrace(false)(opts)
	if opts.BacktraceEnabled == nil || *opts.BacktraceEnabled {
		t.Error("expected backtrace disabled")
	}
}

func TestIsBacktraceEnabledExplicit(t *testing.T) {
	opts := defaultOptions()

	enabled := true
	opts.BacktraceEnabled = &enabled
	if !opts.isBacktraceEnabled() {
		t.Error("expected backtrace enabled when explicitly set to true")
	}

	disabled := false
	opts.BacktraceEnabled = &disabled
	if opts.isBacktraceEnabled() {
		t.Error("expected backtrace disabled when explicitly set to false")
	}
}

func TestIsBacktraceEnabledFromEnv(t *testing.T) {
	opts := defaultOptions()

	// Not set in env
	os.Unsetenv(EnvBacktrace)
	if opts.isBacktraceEnabled() {
		t.Error("expected backtrace disabled when env not set")
	}

	// Set to "true"
	os.Setenv(EnvBacktrace, "true")
	defer os.Unsetenv(EnvBacktrace)
	if !opts.isBacktraceEnabled() {
		t.Error("expected backtrace enabled when env is 'true'")
	}

	// Set to "1"
	os.Setenv(EnvBacktrace, "1")
	if !opts.isBacktraceEnabled() {
		t.Error("expected backtrace enabled when env is '1'")
	}

	// Set to something else
	os.Setenv(EnvBacktrace, "false")
	if opts.isBacktraceEnabled() {
		t.Error("expected backtrace disabled when env is 'false'")
	}
}

func TestSlowThresholdForConnection(t *testing.T) {
	opts := defaultOptions()
	opts.SlowThreshold = 100 * time.Millisecond
	opts.Connections = []ConnectionConfig{
		{Name: "fast", SlowThreshold: 50 * time.Millisecond},
		{Name: "normal"},
	}

	// Connection with override
	if d := opts.slowThresholdFor("fast"); d != 50*time.Millisecond {
		t.Errorf("expected 50ms for 'fast', got %v", d)
	}

	// Connection without override (uses global)
	if d := opts.slowThresholdFor("normal"); d != 100*time.Millisecond {
		t.Errorf("expected 100ms for 'normal', got %v", d)
	}

	// Unknown connection (uses global)
	if d := opts.slowThresholdFor("unknown"); d != 100*time.Millisecond {
		t.Errorf("expected 100ms for 'unknown', got %v", d)
	}
}

package http

import (
	"os"
	"testing"
	"time"
)

func TestDefaultOptions(t *testing.T) {
	opts := defaultOptions()

	if opts.slowThreshold != 500*time.Millisecond {
		t.Errorf("expected slow threshold 500ms, got %v", opts.slowThreshold)
	}
	if opts.bodyCapture {
		t.Error("expected body capture disabled by default")
	}
	if opts.maxBodySize != 65536 {
		t.Errorf("expected max body size 65536, got %d", opts.maxBodySize)
	}
	if !opts.headerCapture {
		t.Error("expected header capture enabled by default")
	}
	if !opts.duplicateDetection {
		t.Error("expected duplicate detection enabled by default")
	}
	if !opts.curlGeneration {
		t.Error("expected curl generation enabled by default")
	}
	if opts.backtraceEnabled {
		t.Error("expected backtrace disabled by default")
	}

	// Default redacted headers
	if !opts.redactHeaders["authorization"] {
		t.Error("expected 'authorization' in redacted headers")
	}
	if !opts.redactHeaders["cookie"] {
		t.Error("expected 'cookie' in redacted headers")
	}
	if !opts.redactHeaders["set-cookie"] {
		t.Error("expected 'set-cookie' in redacted headers")
	}
}

func TestWithSlowThreshold(t *testing.T) {
	opts := applyOptions([]Option{WithSlowThreshold(200 * time.Millisecond)})
	if opts.slowThreshold != 200*time.Millisecond {
		t.Errorf("expected 200ms, got %v", opts.slowThreshold)
	}
}

func TestWithBodyCapture(t *testing.T) {
	opts := applyOptions([]Option{WithBodyCapture(true)})
	if !opts.bodyCapture {
		t.Error("expected body capture enabled")
	}
}

func TestWithMaxBodySize(t *testing.T) {
	opts := applyOptions([]Option{WithMaxBodySize(1024)})
	if opts.maxBodySize != 1024 {
		t.Errorf("expected 1024, got %d", opts.maxBodySize)
	}
}

func TestWithMaxBodySize_ZeroIgnored(t *testing.T) {
	opts := applyOptions([]Option{WithMaxBodySize(0)})
	if opts.maxBodySize != 65536 {
		t.Errorf("expected default 65536 for zero value, got %d", opts.maxBodySize)
	}
}

func TestWithHeaderCapture(t *testing.T) {
	opts := applyOptions([]Option{WithHeaderCapture(false)})
	if opts.headerCapture {
		t.Error("expected header capture disabled")
	}
}

func TestWithRedactHeaders(t *testing.T) {
	opts := applyOptions([]Option{WithRedactHeaders("X-Api-Key", "X-Secret")})

	if !opts.redactHeaders["x-api-key"] {
		t.Error("expected 'x-api-key' in redacted headers")
	}
	if !opts.redactHeaders["x-secret"] {
		t.Error("expected 'x-secret' in redacted headers")
	}
	// Default redacted headers should be replaced, not merged
	if opts.redactHeaders["authorization"] {
		t.Error("expected default 'authorization' to be replaced when custom headers set")
	}
}

func TestWithBacktrace(t *testing.T) {
	opts := applyOptions([]Option{WithBacktrace(true)})
	if !opts.backtraceEnabled {
		t.Error("expected backtrace enabled")
	}
}

func TestWithBacktrace_EnvVar(t *testing.T) {
	os.Setenv("HTTP_PROFILER_BACKTRACE", "true")
	defer os.Unsetenv("HTTP_PROFILER_BACKTRACE")

	opts := defaultOptions()
	if !opts.backtraceEnabled {
		t.Error("expected backtrace enabled via env var")
	}
}

func TestWithBacktrace_ExplicitOverridesEnv(t *testing.T) {
	os.Setenv("HTTP_PROFILER_BACKTRACE", "true")
	defer os.Unsetenv("HTTP_PROFILER_BACKTRACE")

	opts := applyOptions([]Option{WithBacktrace(false)})
	if opts.backtraceEnabled {
		t.Error("expected explicit WithBacktrace(false) to override env var")
	}
}

func TestWithDuplicateDetection(t *testing.T) {
	opts := applyOptions([]Option{WithDuplicateDetection(false)})
	if opts.duplicateDetection {
		t.Error("expected duplicate detection disabled")
	}
}

func TestWithCurlGeneration(t *testing.T) {
	opts := applyOptions([]Option{WithCurlGeneration(false)})
	if opts.curlGeneration {
		t.Error("expected curl generation disabled")
	}
}

func TestOptionPrecedence_LastWins(t *testing.T) {
	opts := applyOptions([]Option{
		WithSlowThreshold(100 * time.Millisecond),
		WithSlowThreshold(300 * time.Millisecond),
	})
	if opts.slowThreshold != 300*time.Millisecond {
		t.Errorf("expected last option to win, got %v", opts.slowThreshold)
	}
}

func TestEnvBool(t *testing.T) {
	tests := []struct {
		value    string
		expected bool
	}{
		{"true", true},
		{"TRUE", true},
		{"True", true},
		{"1", true},
		{"false", false},
		{"0", false},
		{"", false},
		{"yes", false},
	}

	for _, tt := range tests {
		os.Setenv("TEST_BOOL_VAR", tt.value)
		result := envBool("TEST_BOOL_VAR")
		if result != tt.expected {
			t.Errorf("envBool(%q) = %v, want %v", tt.value, result, tt.expected)
		}
	}
	os.Unsetenv("TEST_BOOL_VAR")
}

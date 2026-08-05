package collector

import (
	"os"
	"strconv"
	"strings"
)

// Environment variable names for request collector configuration.
const (
	EnvCaptureBody   = "PROFILER_CAPTURE_BODY"
	EnvBodyMaxSize   = "PROFILER_BODY_MAX_SIZE"
	EnvRedactHeaders = "PROFILER_REDACT_HEADERS"
)

// Default values for request collector options.
const (
	DefaultBodyMaxSize = 1048576 // 1 MB
)

// requestOptions holds configuration for the RequestCollector.
type requestOptions struct {
	bodyCaptureEnabled bool
	bodyMaxSize        int
	bodyContentTypes   []string
	redactHeaders      bool
}

// RequestOption configures the RequestCollector.
type RequestOption func(*requestOptions)

// WithBodyCapture enables or disables request body capture.
// Overrides the PROFILER_CAPTURE_BODY environment variable.
func WithBodyCapture(enabled bool) RequestOption {
	return func(o *requestOptions) {
		o.bodyCaptureEnabled = enabled
	}
}

// WithBodyMaxSize sets the maximum number of bytes to capture from the request body.
// Overrides the PROFILER_BODY_MAX_SIZE environment variable.
// Default: 1048576 (1 MB).
func WithBodyMaxSize(bytes int) RequestOption {
	return func(o *requestOptions) {
		if bytes > 0 {
			o.bodyMaxSize = bytes
		}
	}
}

// WithBodyContentTypes restricts body capture to requests with matching
// Content-Type headers. If empty (default), all text content types are captured.
func WithBodyContentTypes(types ...string) RequestOption {
	return func(o *requestOptions) {
		o.bodyContentTypes = types
	}
}

// WithRedactHeaders enables or disables sensitive header redaction.
// Overrides the PROFILER_REDACT_HEADERS environment variable.
// Default: true (headers are redacted).
func WithRedactHeaders(enabled bool) RequestOption {
	return func(o *requestOptions) {
		o.redactHeaders = enabled
	}
}

// defaultRequestOptions returns options with safe defaults, then applies
// environment variable overrides.
func defaultRequestOptions() *requestOptions {
	opts := &requestOptions{
		bodyCaptureEnabled: false,
		bodyMaxSize:        DefaultBodyMaxSize,
		bodyContentTypes:   nil,
		redactHeaders:      true,
	}

	// Environment variable overrides (lower precedence than programmatic options)
	if envBool(EnvCaptureBody) {
		opts.bodyCaptureEnabled = true
	}
	if v := envInt(EnvBodyMaxSize); v > 0 {
		opts.bodyMaxSize = v
	}
	if envBoolExplicitlyFalse(EnvRedactHeaders) {
		opts.redactHeaders = false
	}

	return opts
}

// envBool returns true if the environment variable is set to "true" or "1".
func envBool(key string) bool {
	v := strings.TrimSpace(os.Getenv(key))
	return v == "true" || v == "1"
}

// envBoolExplicitlyFalse returns true if the environment variable is explicitly
// set to "false" or "0". Returns false if unset or any other value.
func envBoolExplicitlyFalse(key string) bool {
	v := strings.TrimSpace(os.Getenv(key))
	return v == "false" || v == "0"
}

// envInt returns the integer value of an environment variable, or 0 if unset/invalid.
func envInt(key string) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0
	}
	return n
}

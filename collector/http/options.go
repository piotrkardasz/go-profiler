package http

import (
	"os"
	"strings"
	"time"
)

// options holds all configurable settings for the HTTP client collector.
type options struct {
	slowThreshold      time.Duration
	bodyCapture        bool
	maxBodySize        int
	headerCapture      bool
	redactHeaders      map[string]bool
	backtraceEnabled   bool
	duplicateDetection bool
	curlGeneration     bool
}

// Option is a functional option for configuring the HTTP client collector.
type Option func(*options)

// WithSlowThreshold sets the duration threshold for flagging slow calls.
// Default: 500ms.
func WithSlowThreshold(d time.Duration) Option {
	return func(o *options) {
		o.slowThreshold = d
	}
}

// WithBodyCapture enables or disables request/response body capture.
// Default: false.
func WithBodyCapture(enabled bool) Option {
	return func(o *options) {
		o.bodyCapture = enabled
	}
}

// WithMaxBodySize sets the maximum body size to capture in bytes.
// Bodies exceeding this limit are truncated. Default: 64KB.
func WithMaxBodySize(n int) Option {
	return func(o *options) {
		if n > 0 {
			o.maxBodySize = n
		}
	}
}

// WithHeaderCapture enables or disables request/response header capture.
// Default: true.
func WithHeaderCapture(enabled bool) Option {
	return func(o *options) {
		o.headerCapture = enabled
	}
}

// WithRedactHeaders sets the headers to redact from captured data.
// Default: Authorization, Cookie, Set-Cookie.
func WithRedactHeaders(headers ...string) Option {
	return func(o *options) {
		o.redactHeaders = make(map[string]bool)
		for _, h := range headers {
			o.redactHeaders[strings.ToLower(h)] = true
		}
	}
}

// WithBacktrace enables or disables call stack capture for each HTTP call.
// Can also be enabled via HTTP_PROFILER_BACKTRACE=true environment variable.
// Default: false.
func WithBacktrace(enabled bool) Option {
	return func(o *options) {
		o.backtraceEnabled = enabled
	}
}

// WithDuplicateDetection enables or disables duplicate call detection.
// Default: true.
func WithDuplicateDetection(enabled bool) Option {
	return func(o *options) {
		o.duplicateDetection = enabled
	}
}

// WithCurlGeneration enables or disables cURL command generation.
// Default: true.
func WithCurlGeneration(enabled bool) Option {
	return func(o *options) {
		o.curlGeneration = enabled
	}
}

// defaultOptions returns the default configuration.
func defaultOptions() *options {
	opts := &options{
		slowThreshold:      500 * time.Millisecond,
		bodyCapture:        false,
		maxBodySize:        65536, // 64KB
		headerCapture:      true,
		duplicateDetection: true,
		curlGeneration:     true,
		backtraceEnabled:   false,
		redactHeaders: map[string]bool{
			"authorization": true,
			"cookie":        true,
			"set-cookie":    true,
		},
	}

	// Environment variable fallback for backtrace
	if envBool("HTTP_PROFILER_BACKTRACE") {
		opts.backtraceEnabled = true
	}

	return opts
}

// applyOptions applies functional options to the default configuration.
func applyOptions(opts []Option) *options {
	o := defaultOptions()
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// envBool reads a boolean environment variable.
func envBool(key string) bool {
	v := os.Getenv(key)
	return strings.EqualFold(v, "true") || v == "1"
}

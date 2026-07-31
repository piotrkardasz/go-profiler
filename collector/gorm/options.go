package gormcollector

import (
	"os"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	// DefaultSlowThreshold is the default duration above which a query is considered slow.
	DefaultSlowThreshold = 100 * time.Millisecond

	// DefaultN1Threshold is the default count at which repeated similar queries
	// trigger an N+1 warning.
	DefaultN1Threshold = 5

	// EnvBacktrace is the environment variable that enables backtrace collection.
	// Set to "true" or "1" to enable.
	EnvBacktrace = "GORM_PROFILER_BACKTRACE"
)

// ConnectionConfig holds configuration for a single named database connection.
type ConnectionConfig struct {
	// Name is the unique identifier for this connection.
	Name string

	// DB is the GORM database instance.
	DB *gorm.DB

	// SlowThreshold overrides the global slow query threshold for this connection.
	// Zero means use the global default.
	SlowThreshold time.Duration
}

// Options configures the GORM collector behavior.
type Options struct {
	// Connections are the named database connections to monitor.
	Connections []ConnectionConfig

	// SlowThreshold is the global slow query threshold.
	// Queries exceeding this duration are flagged as slow.
	// Defaults to DefaultSlowThreshold (100ms).
	SlowThreshold time.Duration

	// N1Threshold is the number of similar queries that triggers an N+1 warning.
	// Defaults to DefaultN1Threshold (5).
	N1Threshold int

	// BacktraceEnabled controls whether call stack capture is enabled.
	// If not explicitly set, it reads from the GORM_PROFILER_BACKTRACE env variable.
	BacktraceEnabled *bool
}

// Option is a function that configures the collector Options.
type Option func(*Options)

// WithConnection adds a named database connection to monitor.
func WithConnection(name string, db *gorm.DB) Option {
	return func(o *Options) {
		o.Connections = append(o.Connections, ConnectionConfig{
			Name: name,
			DB:   db,
		})
	}
}

// WithConnectionConfig adds a connection with full configuration.
func WithConnectionConfig(cfg ConnectionConfig) Option {
	return func(o *Options) {
		o.Connections = append(o.Connections, cfg)
	}
}

// WithSlowThreshold sets the global slow query threshold.
func WithSlowThreshold(d time.Duration) Option {
	return func(o *Options) {
		o.SlowThreshold = d
	}
}

// WithN1Threshold sets the N+1 detection threshold.
func WithN1Threshold(n int) Option {
	return func(o *Options) {
		o.N1Threshold = n
	}
}

// WithBacktrace explicitly enables or disables backtrace collection,
// overriding the environment variable.
func WithBacktrace(enabled bool) Option {
	return func(o *Options) {
		o.BacktraceEnabled = &enabled
	}
}

// defaultOptions returns Options with sensible defaults.
func defaultOptions() *Options {
	return &Options{
		Connections:   make([]ConnectionConfig, 0),
		SlowThreshold: DefaultSlowThreshold,
		N1Threshold:   DefaultN1Threshold,
	}
}

// isBacktraceEnabled checks whether backtrace collection is enabled,
// first from explicit config, then from environment variable.
func (o *Options) isBacktraceEnabled() bool {
	if o.BacktraceEnabled != nil {
		return *o.BacktraceEnabled
	}
	env := os.Getenv(EnvBacktrace)
	return strings.EqualFold(env, "true") || env == "1"
}

// slowThresholdFor returns the slow query threshold for a given connection.
// If the connection has a per-connection override, it uses that; otherwise the global default.
func (o *Options) slowThresholdFor(connectionName string) time.Duration {
	for _, c := range o.Connections {
		if c.Name == connectionName && c.SlowThreshold > 0 {
			return c.SlowThreshold
		}
	}
	return o.SlowThreshold
}

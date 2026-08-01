package collector

import (
	"context"
	"encoding/json"
	"time"
)

// LogLevel represents the severity of a log entry.
type LogLevel int

const (
	// LevelDebug indicates verbose diagnostic information.
	LevelDebug LogLevel = iota
	// LevelInfo indicates general operational information.
	LevelInfo
	// LevelWarn indicates a potential issue that is not immediately harmful.
	LevelWarn
	// LevelError indicates a failure in a specific operation.
	LevelError
	// LevelFatal indicates an unrecoverable error that typically stops the program.
	LevelFatal
)

// String returns the human-readable name for the log level.
func (l LogLevel) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	case LevelFatal:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// MarshalJSON encodes the log level as a JSON string.
func (l LogLevel) MarshalJSON() ([]byte, error) {
	return json.Marshal(l.String())
}

// LogEntry represents a single captured log message with metadata.
type LogEntry struct {
	// Timestamp is when the log entry was created.
	Timestamp time.Time `json:"timestamp"`
	// Level is the severity of the log entry.
	Level LogLevel `json:"level"`
	// Message is the log message text.
	Message string `json:"message"`
	// Source identifies the logger or subsystem that produced the entry.
	Source string `json:"source"`
	// Attributes holds optional structured key-value data attached to the entry.
	Attributes map[string]any `json:"attributes,omitempty"`
	// Caller is the source file and line that generated the log entry.
	Caller string `json:"caller,omitempty"`
	// Stack holds an optional stack trace, typically for error-level entries.
	Stack string `json:"stack,omitempty"`
}

// CaptureFunc is a callback that receives log entries as they are produced.
type CaptureFunc func(ctx context.Context, entry LogEntry)

// RemoveFunc is returned by LogAdapter.Install and removes the installed hook
// when called.
type RemoveFunc func()

// LogAdapter defines how a specific logging library integrates with the
// logger collector. Implementations install a hook that forwards log entries
// to the provided CaptureFunc.
type LogAdapter interface {
	// Name returns a unique identifier for this adapter (e.g. "zap", "slog").
	Name() string
	// Install registers a capture hook with the underlying logger and returns
	// a function that removes the hook when called.
	Install(capture CaptureFunc) RemoveFunc
}

// LoggerData holds the collected log entries and their summary statistics.
type LoggerData struct {
	// Entries contains the captured log entries.
	Entries []LogEntry `json:"entries"`
	// Summary provides counts grouped by log level.
	Summary LogSummary `json:"summary"`
	// Truncated indicates whether entries were dropped due to the max limit.
	Truncated bool `json:"truncated"`
	// MaxEntries is the configured upper bound on stored entries.
	MaxEntries int `json:"max_entries"`
}

// LogSummary holds per-level counts of captured log entries.
type LogSummary struct {
	// Total is the total number of log entries captured.
	Total int `json:"total"`
	// Debug is the count of entries at LevelDebug.
	Debug int `json:"debug"`
	// Info is the count of entries at LevelInfo.
	Info int `json:"info"`
	// Warn is the count of entries at LevelWarn.
	Warn int `json:"warn"`
	// Error is the count of entries at LevelError.
	Error int `json:"error"`
	// Fatal is the count of entries at LevelFatal.
	Fatal int `json:"fatal"`
}

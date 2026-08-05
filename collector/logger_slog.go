package collector

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// SlogAdapter is a slog.Handler that captures log entries and asynchronously
// forwards them to an inner handler via a LogForwarder.
type SlogAdapter struct {
	inner     slog.Handler
	capture   CaptureFunc
	forwarder *LogForwarder
	addSource bool
	groups    []string
	attrs     []slog.Attr
}

// NewSlogAdapter creates a new SlogAdapter wrapping the given inner handler.
// Log entries are captured via the provided CaptureFunc and forwarded
// asynchronously through the LogForwarder.
func NewSlogAdapter(inner slog.Handler, capture CaptureFunc, forwarder *LogForwarder, addSource bool) *SlogAdapter {
	return &SlogAdapter{
		inner:     inner,
		capture:   capture,
		forwarder: forwarder,
		addSource: addSource,
	}
}

// Enabled reports whether the inner handler is enabled for the given level.
func (a *SlogAdapter) Enabled(ctx context.Context, level slog.Level) bool {
	return a.inner.Enabled(ctx, level)
}

// Handle captures the log record as a LogEntry and forwards it asynchronously
// to the inner handler.
func (a *SlogAdapter) Handle(ctx context.Context, r slog.Record) error {
	entry := LogEntry{
		Timestamp:  r.Time,
		Level:      slogLevelToLogLevel(r.Level),
		Message:    r.Message,
		Source:     "slog",
		Attributes: extractSlogAttributes(r, a.groups, a.attrs),
	}

	if a.addSource && r.PC != 0 {
		entry.Caller = formatCaller(r.PC)
	}

	a.capture(ctx, entry)
	a.forwarder.Forward(ctx, r, a.inner)

	return nil
}

// WithAttrs returns a new SlogAdapter that includes the given attributes.
func (a *SlogAdapter) WithAttrs(attrs []slog.Attr) slog.Handler {
	newGroups := make([]string, len(a.groups))
	copy(newGroups, a.groups)

	newAttrs := make([]slog.Attr, len(a.attrs), len(a.attrs)+len(attrs))
	copy(newAttrs, a.attrs)
	newAttrs = append(newAttrs, attrs...)

	return &SlogAdapter{
		inner:     a.inner.WithAttrs(attrs),
		capture:   a.capture,
		forwarder: a.forwarder,
		addSource: a.addSource,
		groups:    newGroups,
		attrs:     newAttrs,
	}
}

// WithGroup returns a new SlogAdapter that qualifies subsequent attributes
// with the given group name.
func (a *SlogAdapter) WithGroup(name string) slog.Handler {
	newGroups := make([]string, len(a.groups), len(a.groups)+1)
	copy(newGroups, a.groups)
	newGroups = append(newGroups, name)

	return &SlogAdapter{
		inner:     a.inner.WithGroup(name),
		capture:   a.capture,
		forwarder: a.forwarder,
		addSource: a.addSource,
		groups:    newGroups,
		attrs:     a.attrs,
	}
}

// slogLogAdapter implements the LogAdapter interface for the standard library
// slog package.
type slogLogAdapter struct {
	addSource       bool
	forwarder       *LogForwarder
	originalHandler slog.Handler
	originalWriter  io.Writer
}

// NewSlogLogAdapter creates a new LogAdapter implementation for slog.
// When addSource is true, captured entries will include caller information.
func NewSlogLogAdapter(addSource bool) *slogLogAdapter {
	return &slogLogAdapter{
		addSource: addSource,
	}
}

// Name returns the unique identifier for this adapter.
func (a *slogLogAdapter) Name() string {
	return "slog"
}

// Install registers a capture hook with slog by replacing the default logger.
// It returns a RemoveFunc that restores the original default logger and closes
// the forwarder.
func (a *slogLogAdapter) Install(capture CaptureFunc) RemoveFunc {
	originalHandler := slog.Default().Handler()

	forwarder := a.forwarder
	if forwarder == nil {
		forwarder = NewLogForwarder(4096)
	}

	// When an originalWriter is provided (i.e., the stdlog adapter's pre-intercept
	// writer), create a fresh text handler writing directly to it. This prevents
	// forwarded slog records from passing back through the StdLogAdapter and
	// being captured a second time with an already-formatted message.
	innerHandler := originalHandler
	if a.originalWriter != nil {
		innerHandler = slog.NewTextHandler(a.originalWriter, &slog.HandlerOptions{
			AddSource: a.addSource,
		})
	}

	adapter := NewSlogAdapter(innerHandler, capture, forwarder, a.addSource)
	slog.SetDefault(slog.New(adapter))

	return func() {
		slog.SetDefault(slog.New(originalHandler))
		forwarder.Close()
	}
}

// slogLevelToLogLevel converts a slog.Level to the package LogLevel type.
func slogLevelToLogLevel(l slog.Level) LogLevel {
	switch {
	case l < slog.LevelInfo:
		return LevelDebug
	case l < slog.LevelWarn:
		return LevelInfo
	case l < slog.LevelError:
		return LevelWarn
	default:
		return LevelError
	}
}

// extractSlogAttributes builds a map of attributes from the pre-set attrs and
// the record's own attributes, applying group prefixes as needed.
func extractSlogAttributes(r slog.Record, groups []string, preAttrs []slog.Attr) map[string]any {
	m := make(map[string]any)

	for _, attr := range preAttrs {
		key := buildGroupKey(groups, attr.Key)
		m[key] = attr.Value.Any()
	}

	r.Attrs(func(attr slog.Attr) bool {
		key := buildGroupKey(groups, attr.Key)
		m[key] = attr.Value.Any()
		return true
	})

	if len(m) == 0 {
		return nil
	}
	return m
}

// buildGroupKey constructs an attribute key prefixed by the active groups.
func buildGroupKey(groups []string, key string) string {
	if len(groups) == 0 {
		return key
	}
	return strings.Join(groups, ".") + "." + key
}

// formatCaller formats the caller information from a program counter value.
func formatCaller(pc uintptr) string {
	frames := runtime.CallersFrames([]uintptr{pc})
	frame, _ := frames.Next()
	return filepath.Base(frame.File) + ":" + strconv.Itoa(frame.Line)
}

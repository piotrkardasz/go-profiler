package collector

import (
	"context"
	"log"
	"net/http"
)

const (
	defaultMaxEntries    = 1000
	defaultAttrMaxSize   = 1024
	defaultMinLevel      = LevelDebug
	defaultForwardBufSize = 4096
)

// loggerOptions holds configuration for the LoggerCollector.
type loggerOptions struct {
	adapters       []LogAdapter
	minLevel       LogLevel
	maxEntries     int
	callerInfo     bool
	stackTrace     bool
	slogDisabled   bool
	stdLogDisabled bool
	attrMaxSize    int
	forwardBufSize int
}

// LoggerOption is a functional option for configuring a LoggerCollector.
type LoggerOption func(*loggerOptions)

// WithAdapter adds a custom LogAdapter to the collector.
func WithAdapter(adapter LogAdapter) LoggerOption {
	return func(o *loggerOptions) {
		o.adapters = append(o.adapters, adapter)
	}
}

// WithMinLevel sets the minimum log level that will be captured.
func WithMinLevel(level LogLevel) LoggerOption {
	return func(o *loggerOptions) {
		o.minLevel = level
	}
}

// WithMaxEntries sets the maximum number of log entries stored per request.
func WithMaxEntries(n int) LoggerOption {
	return func(o *loggerOptions) {
		o.maxEntries = n
	}
}

// WithCallerInfo enables or disables caller information on log entries.
func WithCallerInfo(enabled bool) LoggerOption {
	return func(o *loggerOptions) {
		o.callerInfo = enabled
	}
}

// WithStackTrace enables or disables stack trace capture on log entries.
func WithStackTrace(enabled bool) LoggerOption {
	return func(o *loggerOptions) {
		o.stackTrace = enabled
	}
}

// WithoutSlog disables automatic slog adapter installation.
func WithoutSlog() LoggerOption {
	return func(o *loggerOptions) {
		o.slogDisabled = true
	}
}

// WithoutStdLog disables automatic standard log adapter installation.
func WithoutStdLog() LoggerOption {
	return func(o *loggerOptions) {
		o.stdLogDisabled = true
	}
}

// WithAttributeMaxSize sets the maximum byte size for individual attribute values.
// String values exceeding this size will be truncated.
func WithAttributeMaxSize(bytes int) LoggerOption {
	return func(o *loggerOptions) {
		o.attrMaxSize = bytes
	}
}

// WithForwardBufferSize sets the buffer size for the log forwarding channel.
func WithForwardBufferSize(size int) LoggerOption {
	return func(o *loggerOptions) {
		o.forwardBufSize = size
	}
}

// LoggerCollector captures log entries from multiple logging libraries during
// HTTP request processing. It implements the Collector, ContextSetup, and
// PanelProvider interfaces.
type LoggerCollector struct {
	opts           loggerOptions
	removeFuncs    []RemoveFunc
	captureFunc    CaptureFunc
	stdLogAdapter  *StdLogAdapter
	forwarder      *LogForwarder
}

// NewLoggerCollector creates a new LoggerCollector with the given options.
// It installs slog and standard log adapters by default (unless disabled),
// along with any user-provided adapters.
func NewLoggerCollector(opts ...LoggerOption) *LoggerCollector {
	o := loggerOptions{
		maxEntries:     defaultMaxEntries,
		attrMaxSize:    defaultAttrMaxSize,
		minLevel:       defaultMinLevel,
		forwardBufSize: defaultForwardBufSize,
		callerInfo:     true,
	}
	for _, opt := range opts {
		opt(&o)
	}

	c := &LoggerCollector{
		opts:      o,
		forwarder: NewLogForwarder(o.forwardBufSize),
	}

	c.captureFunc = c.buildCaptureFunc()

	// Install slog adapter unless disabled.
	if !o.slogDisabled {
		adapter := &slogLogAdapter{
			addSource: o.callerInfo,
		}
		adapter.forwarder = c.forwarder
		removeFunc := adapter.Install(c.captureFunc)
		c.removeFuncs = append(c.removeFuncs, removeFunc)
	}

	// Install stdlog adapter unless disabled.
	if !o.stdLogDisabled {
		adapter := &stdLogLogAdapter{
			bufferSize: o.forwardBufSize,
		}
		removeFunc := adapter.Install(c.captureFunc)
		c.removeFuncs = append(c.removeFuncs, removeFunc)
		// Retrieve the StdLogAdapter that was set as log output.
		if w, ok := log.Writer().(*StdLogAdapter); ok {
			c.stdLogAdapter = w
		}
	}

	// Install user-provided adapters.
	for _, a := range o.adapters {
		removeFunc := a.Install(c.captureFunc)
		c.removeFuncs = append(c.removeFuncs, removeFunc)
	}

	return c
}

// buildCaptureFunc returns a CaptureFunc that appends log entries to the
// request-scoped LogBuffer, filtering by minimum level.
func (c *LoggerCollector) buildCaptureFunc() CaptureFunc {
	return func(ctx context.Context, entry LogEntry) {
		defer func() {
			recover() //nolint:errcheck
		}()

		if entry.Level < c.opts.minLevel {
			return
		}

		buf := GetLogBuffer(ctx)
		if buf == nil {
			return
		}

		buf.Append(entry)
	}
}

// Name returns the unique identifier for this collector.
func (c *LoggerCollector) Name() string {
	return "logger"
}

// Collect gathers all captured log entries for the current request and returns
// them along with summary statistics.
func (c *LoggerCollector) Collect(ctx context.Context, req *http.Request, res ResponseData) (any, error) {
	buf := GetLogBuffer(ctx)
	if buf == nil {
		return &LoggerData{MaxEntries: c.opts.maxEntries}, nil
	}

	entries, truncated := buf.Drain()

	for i := range entries {
		entries[i].Attributes = truncateAttributes(entries[i].Attributes, c.opts.attrMaxSize)
	}

	summary := buildSummary(entries)

	return &LoggerData{
		Entries:    entries,
		Summary:    summary,
		Truncated:  truncated,
		MaxEntries: c.opts.maxEntries,
	}, nil
}

// Reset is a no-op for the logger collector since state is per-request.
func (c *LoggerCollector) Reset() {}

// SetupContext creates a new LogBuffer and stores it in the context for the
// current request. It also sets the active context on the standard log adapter
// if installed.
func (c *LoggerCollector) SetupContext(ctx context.Context) context.Context {
	buf := NewLogBuffer(c.opts.maxEntries)
	ctx = WithLogBuffer(ctx, buf)

	if c.stdLogAdapter != nil {
		c.stdLogAdapter.SetActiveContext(ctx)
	}

	return ctx
}

// PanelMeta returns metadata describing how the logger collector's data should
// be displayed in the profiler UI.
func (c *LoggerCollector) PanelMeta() PanelMeta {
	return PanelMeta{
		Name:      "logger",
		Label:     "Logs",
		Icon:      "file-text",
		Component: "LoggerPanel",
	}
}

// Close removes all installed adapter hooks, closes the log forwarder, and
// cleans up the standard log adapter if present.
func (c *LoggerCollector) Close() {
	for _, remove := range c.removeFuncs {
		remove()
	}

	c.forwarder.Close()

	if c.stdLogAdapter != nil {
		c.stdLogAdapter.Close()
	}
}

// buildSummary counts log entries per level and returns a LogSummary.
func buildSummary(entries []LogEntry) LogSummary {
	var s LogSummary
	s.Total = len(entries)
	for _, e := range entries {
		switch e.Level {
		case LevelDebug:
			s.Debug++
		case LevelInfo:
			s.Info++
		case LevelWarn:
			s.Warn++
		case LevelError:
			s.Error++
		case LevelFatal:
			s.Fatal++
		}
	}
	return s
}

// truncateAttributes truncates string attribute values that exceed maxSize bytes,
// appending "...(truncated)" to indicate the value was shortened.
func truncateAttributes(attrs map[string]any, maxSize int) map[string]any {
	if attrs == nil {
		return nil
	}
	for k, v := range attrs {
		if s, ok := v.(string); ok && len(s) > maxSize {
			attrs[k] = s[:maxSize] + "...(truncated)"
		}
	}
	return attrs
}

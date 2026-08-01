package collector

import (
	"context"
	"log/slog"
	"runtime"
	"sync"
	"testing"
	"time"
)

// capturedEntries collects entries via CaptureFunc for assertions.
type capturedEntries struct {
	mu      sync.Mutex
	entries []LogEntry
}

func (c *capturedEntries) capture(ctx context.Context, entry LogEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = append(c.entries, entry)
}

func (c *capturedEntries) get() []LogEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]LogEntry(nil), c.entries...)
}

// testHandler is a minimal slog.Handler for testing purposes.
type testHandler struct {
	mu      sync.Mutex
	records []slog.Record
	enabled bool
}

func newTestHandler() *testHandler { return &testHandler{enabled: true} }

func (h *testHandler) Enabled(_ context.Context, _ slog.Level) bool { return h.enabled }

func (h *testHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r)
	return nil
}

func (h *testHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }

func (h *testHandler) WithGroup(_ string) slog.Handler { return h }

func TestSlogAdapterName(t *testing.T) {
	adapter := NewSlogLogAdapter(false)
	if name := adapter.Name(); name != "slog" {
		t.Errorf("expected name %q, got %q", "slog", name)
	}
}

func TestSlogAdapterCapturesInfoRecord(t *testing.T) {
	inner := newTestHandler()
	forwarder := NewLogForwarder(64)
	captured := &capturedEntries{}

	adapter := NewSlogAdapter(inner, captured.capture, forwarder, false)

	buf := NewLogBuffer(100)
	ctx := WithLogBuffer(context.Background(), buf)

	r := slog.NewRecord(time.Now(), slog.LevelInfo, "hello world", 0)
	if err := adapter.Handle(ctx, r); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	forwarder.Close()

	entries := captured.get()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Level != LevelInfo {
		t.Errorf("expected level %v, got %v", LevelInfo, entries[0].Level)
	}
	if entries[0].Message != "hello world" {
		t.Errorf("expected message %q, got %q", "hello world", entries[0].Message)
	}
}

func TestSlogAdapterCapturesAllLevels(t *testing.T) {
	tests := []struct {
		slogLevel slog.Level
		expected  LogLevel
	}{
		{slog.LevelDebug, LevelDebug},
		{slog.LevelInfo, LevelInfo},
		{slog.LevelWarn, LevelWarn},
		{slog.LevelError, LevelError},
	}

	for _, tt := range tests {
		t.Run(tt.expected.String(), func(t *testing.T) {
			inner := newTestHandler()
			forwarder := NewLogForwarder(64)
			captured := &capturedEntries{}

			adapter := NewSlogAdapter(inner, captured.capture, forwarder, false)

			buf := NewLogBuffer(100)
			ctx := WithLogBuffer(context.Background(), buf)

			r := slog.NewRecord(time.Now(), tt.slogLevel, "msg", 0)
			if err := adapter.Handle(ctx, r); err != nil {
				t.Fatalf("Handle returned error: %v", err)
			}

			forwarder.Close()

			entries := captured.get()
			if len(entries) != 1 {
				t.Fatalf("expected 1 entry, got %d", len(entries))
			}
			if entries[0].Level != tt.expected {
				t.Errorf("expected level %v, got %v", tt.expected, entries[0].Level)
			}
		})
	}
}

func TestSlogAdapterLevelMapping(t *testing.T) {
	tests := []struct {
		input    slog.Level
		expected LogLevel
	}{
		{slog.LevelDebug, LevelDebug},
		{slog.LevelInfo, LevelInfo},
		{slog.LevelWarn, LevelWarn},
		{slog.LevelError, LevelError},
	}

	for _, tt := range tests {
		t.Run(tt.expected.String(), func(t *testing.T) {
			got := slogLevelToLogLevel(tt.input)
			if got != tt.expected {
				t.Errorf("slogLevelToLogLevel(%v) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestSlogAdapterAttributes(t *testing.T) {
	inner := newTestHandler()
	forwarder := NewLogForwarder(64)
	captured := &capturedEntries{}

	adapter := NewSlogAdapter(inner, captured.capture, forwarder, false)

	buf := NewLogBuffer(100)
	ctx := WithLogBuffer(context.Background(), buf)

	r := slog.NewRecord(time.Now(), slog.LevelInfo, "with attrs", 0)
	r.AddAttrs(slog.String("key", "value"), slog.Int("count", 42))

	if err := adapter.Handle(ctx, r); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	forwarder.Close()

	entries := captured.get()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	attrs := entries[0].Attributes
	if attrs == nil {
		t.Fatal("expected non-nil attributes")
	}
	if v, ok := attrs["key"]; !ok || v != "value" {
		t.Errorf("expected attrs[key]=%q, got %v", "value", v)
	}
	if v, ok := attrs["count"]; !ok {
		t.Error("expected attrs[count] to exist")
	} else if v != int64(42) {
		t.Errorf("expected attrs[count]=%v (int64), got %v (%T)", int64(42), v, v)
	}
}

func TestSlogAdapterGroupedAttributes(t *testing.T) {
	inner := newTestHandler()
	forwarder := NewLogForwarder(64)
	captured := &capturedEntries{}

	adapter := NewSlogAdapter(inner, captured.capture, forwarder, false)
	grouped := adapter.WithGroup("db").(*SlogAdapter)

	buf := NewLogBuffer(100)
	ctx := WithLogBuffer(context.Background(), buf)

	r := slog.NewRecord(time.Now(), slog.LevelInfo, "query", 0)
	r.AddAttrs(slog.String("key", "users"))

	if err := grouped.Handle(ctx, r); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	forwarder.Close()

	entries := captured.get()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	attrs := entries[0].Attributes
	if attrs == nil {
		t.Fatal("expected non-nil attributes")
	}
	if v, ok := attrs["db.key"]; !ok || v != "users" {
		t.Errorf("expected attrs[db.key]=%q, got %v", "users", v)
	}
}

func TestSlogAdapterPreAttrs(t *testing.T) {
	inner := newTestHandler()
	forwarder := NewLogForwarder(64)
	captured := &capturedEntries{}

	adapter := NewSlogAdapter(inner, captured.capture, forwarder, false)
	withAttrs := adapter.WithAttrs([]slog.Attr{slog.String("service", "api")}).(*SlogAdapter)

	buf := NewLogBuffer(100)
	ctx := WithLogBuffer(context.Background(), buf)

	r := slog.NewRecord(time.Now(), slog.LevelInfo, "request", 0)
	if err := withAttrs.Handle(ctx, r); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	forwarder.Close()

	entries := captured.get()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	attrs := entries[0].Attributes
	if attrs == nil {
		t.Fatal("expected non-nil attributes")
	}
	if v, ok := attrs["service"]; !ok || v != "api" {
		t.Errorf("expected attrs[service]=%q, got %v", "api", v)
	}
}

func TestSlogAdapterCallerInfo(t *testing.T) {
	inner := newTestHandler()
	forwarder := NewLogForwarder(64)
	captured := &capturedEntries{}

	adapter := NewSlogAdapter(inner, captured.capture, forwarder, true)

	buf := NewLogBuffer(100)
	ctx := WithLogBuffer(context.Background(), buf)

	// Get a real PC value.
	var pcs [1]uintptr
	runtime.Callers(1, pcs[:])

	r := slog.NewRecord(time.Now(), slog.LevelInfo, "with caller", pcs[0])
	if err := adapter.Handle(ctx, r); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	forwarder.Close()

	entries := captured.get()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Caller == "" {
		t.Error("expected non-empty Caller when addSource=true")
	}
}

func TestSlogAdapterNoCallerWhenDisabled(t *testing.T) {
	inner := newTestHandler()
	forwarder := NewLogForwarder(64)
	captured := &capturedEntries{}

	adapter := NewSlogAdapter(inner, captured.capture, forwarder, false)

	buf := NewLogBuffer(100)
	ctx := WithLogBuffer(context.Background(), buf)

	// Get a real PC value.
	var pcs [1]uintptr
	runtime.Callers(1, pcs[:])

	r := slog.NewRecord(time.Now(), slog.LevelInfo, "no caller", pcs[0])
	if err := adapter.Handle(ctx, r); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	forwarder.Close()

	entries := captured.get()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Caller != "" {
		t.Errorf("expected empty Caller when addSource=false, got %q", entries[0].Caller)
	}
}

func TestSlogAdapterForwardsToInner(t *testing.T) {
	inner := newTestHandler()
	forwarder := NewLogForwarder(64)
	captured := &capturedEntries{}

	adapter := NewSlogAdapter(inner, captured.capture, forwarder, false)

	buf := NewLogBuffer(100)
	ctx := WithLogBuffer(context.Background(), buf)

	r := slog.NewRecord(time.Now(), slog.LevelInfo, "forwarded", 0)
	if err := adapter.Handle(ctx, r); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	forwarder.Close()

	inner.mu.Lock()
	defer inner.mu.Unlock()
	if len(inner.records) != 1 {
		t.Fatalf("expected inner handler to receive 1 record, got %d", len(inner.records))
	}
	if inner.records[0].Message != "forwarded" {
		t.Errorf("expected inner record message %q, got %q", "forwarded", inner.records[0].Message)
	}
}

func TestSlogAdapterEnabled(t *testing.T) {
	inner := newTestHandler()
	forwarder := NewLogForwarder(64)
	captured := &capturedEntries{}

	adapter := NewSlogAdapter(inner, captured.capture, forwarder, false)

	ctx := context.Background()

	if !adapter.Enabled(ctx, slog.LevelInfo) {
		t.Error("expected Enabled to return true when inner handler is enabled")
	}

	inner.enabled = false
	if adapter.Enabled(ctx, slog.LevelInfo) {
		t.Error("expected Enabled to return false when inner handler is disabled")
	}

	forwarder.Close()
}

func TestSlogAdapterContextRequired(t *testing.T) {
	inner := newTestHandler()
	forwarder := NewLogForwarder(64)
	captured := &capturedEntries{}

	adapter := NewSlogAdapter(inner, captured.capture, forwarder, false)

	// Context without LogBuffer — should not panic.
	ctx := context.Background()

	r := slog.NewRecord(time.Now(), slog.LevelInfo, "no buffer", 0)
	if err := adapter.Handle(ctx, r); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	forwarder.Close()

	// Entry is still captured by the CaptureFunc (it just won't be stored in a buffer).
	entries := captured.get()
	if len(entries) != 1 {
		t.Fatalf("expected 1 captured entry, got %d", len(entries))
	}
}

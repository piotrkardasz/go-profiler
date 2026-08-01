package collector

import (
	"bytes"
	"context"
	"log"
	"strings"
	"sync"
	"testing"
	"time"
)

// testWriter is a concurrency-safe bytes.Buffer used to verify forwarding.
type testWriter struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (w *testWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func (w *testWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

// stdlogCaptured collects LogEntry values from a CaptureFunc.
type stdlogCaptured struct {
	mu      sync.Mutex
	entries []LogEntry
}

func (c *stdlogCaptured) capture(_ context.Context, entry LogEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = append(c.entries, entry)
}

func (c *stdlogCaptured) get() []LogEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]LogEntry(nil), c.entries...)
}

func TestStdLogAdapterName(t *testing.T) {
	adapter := NewStdLogLogAdapter(4096)
	if adapter.Name() != "log" {
		t.Errorf("Name(): got %q, want %q", adapter.Name(), "log")
	}
}

func TestStdLogAdapterCapturesOutput(t *testing.T) {
	cap := &stdlogCaptured{}
	tw := &testWriter{}

	a := NewStdLogAdapter(cap.capture, 64)
	a.original = tw

	buf := NewLogBuffer(100)
	ctx := WithLogBuffer(context.Background(), buf)
	a.SetActiveContext(ctx)

	msg := "hello from stdlog"
	a.Write([]byte(msg + "\n"))
	a.Close()

	entries := cap.get()
	if len(entries) == 0 {
		t.Fatal("expected at least one captured entry")
	}
	if entries[0].Message != msg {
		t.Errorf("Message: got %q, want %q", entries[0].Message, msg)
	}
	if entries[0].Level != LevelInfo {
		t.Errorf("Level: got %v, want %v", entries[0].Level, LevelInfo)
	}
}

func TestStdLogAdapterLevelIsInfo(t *testing.T) {
	cap := &stdlogCaptured{}
	tw := &testWriter{}

	a := NewStdLogAdapter(cap.capture, 64)
	a.original = tw

	ctx := context.Background()
	a.SetActiveContext(ctx)

	messages := []string{"msg one", "msg two", "msg three"}
	for _, m := range messages {
		a.Write([]byte(m + "\n"))
	}
	a.Close()

	entries := cap.get()
	if len(entries) != len(messages) {
		t.Fatalf("captured %d entries, want %d", len(entries), len(messages))
	}
	for i, e := range entries {
		if e.Level != LevelInfo {
			t.Errorf("entry[%d] Level: got %v, want %v", i, e.Level, LevelInfo)
		}
	}
}

func TestStdLogAdapterAsyncForwarding(t *testing.T) {
	cap := &stdlogCaptured{}
	tw := &slowWriter{w: &testWriter{}, delay: 50 * time.Millisecond}

	a := NewStdLogAdapter(cap.capture, 64)
	a.original = tw

	ctx := context.Background()
	a.SetActiveContext(ctx)

	const count = 5
	start := time.Now()
	for i := 0; i < count; i++ {
		a.Write([]byte("async message\n"))
	}
	elapsed := time.Since(start)

	// Writes should return quickly because forwarding is async.
	if elapsed >= 50*time.Millisecond {
		t.Errorf("writes took %v, expected < 50ms (async)", elapsed)
	}

	a.Close()

	// After close, all messages should have been forwarded.
	got := tw.w.String()
	forwarded := strings.Count(got, "async message")
	if forwarded != count {
		t.Errorf("forwarded %d messages, want %d", forwarded, count)
	}
}

func TestStdLogAdapterForwardsToOriginal(t *testing.T) {
	cap := &stdlogCaptured{}
	tw := &testWriter{}

	a := NewStdLogAdapter(cap.capture, 64)
	a.original = tw

	ctx := context.Background()
	a.SetActiveContext(ctx)

	msg := "forwarded message\n"
	a.Write([]byte(msg))
	a.Close()

	if !strings.Contains(tw.String(), "forwarded message") {
		t.Errorf("original writer did not receive message; got %q", tw.String())
	}
}

func TestStdLogAdapterSetActiveContext(t *testing.T) {
	cap := &stdlogCaptured{}
	tw := &testWriter{}

	a := NewStdLogAdapter(cap.capture, 64)
	a.original = tw

	buf1 := NewLogBuffer(100)
	ctx1 := WithLogBuffer(context.Background(), buf1)
	a.SetActiveContext(ctx1)

	a.Write([]byte("first\n"))

	buf2 := NewLogBuffer(100)
	ctx2 := WithLogBuffer(context.Background(), buf2)
	a.SetActiveContext(ctx2)

	a.Write([]byte("second\n"))
	a.Close()

	entries := cap.get()
	if len(entries) != 2 {
		t.Fatalf("captured %d entries, want 2", len(entries))
	}
	if entries[0].Message != "first" {
		t.Errorf("entry[0] Message: got %q, want %q", entries[0].Message, "first")
	}
	if entries[1].Message != "second" {
		t.Errorf("entry[1] Message: got %q, want %q", entries[1].Message, "second")
	}
}

func TestStdLogAdapterNoContextNoPanic(t *testing.T) {
	cap := &stdlogCaptured{}
	tw := &testWriter{}

	a := NewStdLogAdapter(cap.capture, 64)
	a.original = tw

	// Do NOT set active context — should not panic.
	a.Write([]byte("no context\n"))
	a.Close()

	entries := cap.get()
	if len(entries) != 1 {
		t.Fatalf("captured %d entries, want 1", len(entries))
	}
	if entries[0].Message != "no context" {
		t.Errorf("Message: got %q, want %q", entries[0].Message, "no context")
	}
}

func TestStdLogAdapterInstallAndRemove(t *testing.T) {
	originalWriter := log.Writer()
	defer log.SetOutput(originalWriter)

	adapter := NewStdLogLogAdapter(64)
	cap := &stdlogCaptured{}

	removeFunc := adapter.Install(cap.capture)

	if log.Writer() == originalWriter {
		t.Error("expected log.Writer() to change after Install")
	}

	removeFunc()

	if log.Writer() != originalWriter {
		t.Error("expected log.Writer() to be restored after RemoveFunc")
	}
}

func TestStdLogAdapterMessageParsing(t *testing.T) {
	cap := &stdlogCaptured{}
	tw := &testWriter{}

	a := NewStdLogAdapter(cap.capture, 64)
	a.original = tw

	ctx := context.Background()
	a.SetActiveContext(ctx)

	raw := "2024/01/15 10:30:00 hello world\n"
	a.Write([]byte(raw))
	a.Close()

	entries := cap.get()
	if len(entries) == 0 {
		t.Fatal("expected at least one captured entry")
	}

	want := strings.TrimSpace(raw)
	if entries[0].Message != want {
		t.Errorf("Message: got %q, want %q", entries[0].Message, want)
	}
}

// slowWriter wraps a testWriter with an artificial delay per write.
type slowWriter struct {
	w     *testWriter
	delay time.Duration
}

func (sw *slowWriter) Write(p []byte) (int, error) {
	time.Sleep(sw.delay)
	return sw.w.Write(p)
}

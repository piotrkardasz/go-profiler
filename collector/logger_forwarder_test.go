package collector

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"
)

type mockHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *mockHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *mockHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r)
	return nil
}
func (h *mockHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *mockHandler) WithGroup(_ string) slog.Handler      { return h }

type slowHandler struct {
	delay time.Duration
	mockHandler
}

func (h *slowHandler) Handle(ctx context.Context, r slog.Record) error {
	time.Sleep(h.delay)
	return h.mockHandler.Handle(ctx, r)
}

func TestLogForwarderOrderPreserved(t *testing.T) {
	f := NewLogForwarder(100)
	h := &mockHandler{}

	for i := 0; i < 10; i++ {
		r := slog.NewRecord(time.Now(), slog.LevelInfo, fmt.Sprintf("msg-%d", i), 0)
		f.Forward(context.Background(), r, h)
	}

	f.Close()

	h.mu.Lock()
	defer h.mu.Unlock()

	if len(h.records) != 10 {
		t.Fatalf("expected 10 records, got %d", len(h.records))
	}
	for i, rec := range h.records {
		expected := fmt.Sprintf("msg-%d", i)
		if rec.Message != expected {
			t.Errorf("record %d: expected message %q, got %q", i, expected, rec.Message)
		}
	}
}

func TestLogForwarderAsyncDoesNotBlock(t *testing.T) {
	h := &slowHandler{delay: 100 * time.Millisecond}
	f := NewLogForwarder(100)

	start := time.Now()
	for i := 0; i < 5; i++ {
		r := slog.NewRecord(time.Now(), slog.LevelInfo, fmt.Sprintf("msg-%d", i), 0)
		f.Forward(context.Background(), r, h)
	}
	elapsed := time.Since(start)

	if elapsed >= 50*time.Millisecond {
		t.Fatalf("Forward calls took %v, expected <50ms (should be async)", elapsed)
	}

	f.Close()

	h.mu.Lock()
	defer h.mu.Unlock()

	if len(h.records) != 5 {
		t.Fatalf("expected 5 records, got %d", len(h.records))
	}
}

func TestLogForwarderBackpressureFallback(t *testing.T) {
	h := &slowHandler{delay: 50 * time.Millisecond}
	f := NewLogForwarder(1)

	for i := 0; i < 10; i++ {
		r := slog.NewRecord(time.Now(), slog.LevelInfo, fmt.Sprintf("msg-%d", i), 0)
		f.Forward(context.Background(), r, h)
	}

	f.Close()

	h.mu.Lock()
	defer h.mu.Unlock()

	if len(h.records) != 10 {
		t.Fatalf("expected 10 records, got %d", len(h.records))
	}
}

func TestLogForwarderCloseFlushes(t *testing.T) {
	f := NewLogForwarder(100)
	h := &mockHandler{}

	for i := 0; i < 50; i++ {
		r := slog.NewRecord(time.Now(), slog.LevelInfo, fmt.Sprintf("msg-%d", i), 0)
		f.Forward(context.Background(), r, h)
	}

	f.Close()

	h.mu.Lock()
	defer h.mu.Unlock()

	if len(h.records) != 50 {
		t.Fatalf("expected 50 records, got %d", len(h.records))
	}
}

func TestLogForwarderInnerHandlerError(t *testing.T) {
	errHandler := &errorHandler{}
	f := NewLogForwarder(100)

	for i := 0; i < 5; i++ {
		r := slog.NewRecord(time.Now(), slog.LevelInfo, fmt.Sprintf("msg-%d", i), 0)
		f.Forward(context.Background(), r, errHandler)
	}

	// Should not panic
	f.Close()
}

type errorHandler struct{}

func (h *errorHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *errorHandler) Handle(_ context.Context, _ slog.Record) error {
	return fmt.Errorf("fail")
}
func (h *errorHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *errorHandler) WithGroup(_ string) slog.Handler      { return h }

func TestLogForwarderConcurrentForward(t *testing.T) {
	f := NewLogForwarder(1000)
	h := &mockHandler{}

	var wg sync.WaitGroup
	for g := 0; g < 10; g++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				r := slog.NewRecord(time.Now(), slog.LevelInfo, fmt.Sprintf("g%d-msg-%d", goroutineID, i), 0)
				f.Forward(context.Background(), r, h)
			}
		}(g)
	}

	wg.Wait()
	f.Close()

	h.mu.Lock()
	defer h.mu.Unlock()

	if len(h.records) != 500 {
		t.Fatalf("expected 500 records, got %d", len(h.records))
	}
}

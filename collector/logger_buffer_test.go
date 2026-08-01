package collector

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestLogBufferAppend(t *testing.T) {
	buf := NewLogBuffer(10)

	for i := 0; i < 3; i++ {
		buf.Append(LogEntry{
			Timestamp: time.Now(),
			Level:     LevelInfo,
			Message:   fmt.Sprintf("msg-%d", i),
			Source:    "test",
		})
	}

	entries, truncated := buf.Drain()
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	if truncated {
		t.Fatal("expected truncated=false")
	}
}

func TestLogBufferMaxEntries(t *testing.T) {
	buf := NewLogBuffer(5)

	for i := 0; i < 8; i++ {
		buf.Append(LogEntry{
			Timestamp: time.Now(),
			Level:     LevelDebug,
			Message:   fmt.Sprintf("msg-%d", i),
			Source:    "test",
		})
	}

	entries, truncated := buf.Drain()
	if len(entries) != 5 {
		t.Fatalf("expected 5 entries, got %d", len(entries))
	}
	if !truncated {
		t.Fatal("expected truncated=true")
	}
}

func TestLogBufferDrainResetsState(t *testing.T) {
	buf := NewLogBuffer(10)

	buf.Append(LogEntry{
		Timestamp: time.Now(),
		Level:     LevelInfo,
		Message:   "first",
		Source:    "test",
	})

	// First drain.
	entries, _ := buf.Drain()
	if len(entries) != 1 {
		t.Fatalf("first drain: expected 1 entry, got %d", len(entries))
	}

	// Append more after drain.
	buf.Append(LogEntry{
		Timestamp: time.Now(),
		Level:     LevelInfo,
		Message:   "second",
		Source:    "test",
	})
	buf.Append(LogEntry{
		Timestamp: time.Now(),
		Level:     LevelInfo,
		Message:   "third",
		Source:    "test",
	})

	// Second drain should only contain new entries.
	entries, truncated := buf.Drain()
	if len(entries) != 2 {
		t.Fatalf("second drain: expected 2 entries, got %d", len(entries))
	}
	if truncated {
		t.Fatal("expected truncated=false after reset")
	}
}

func TestLogBufferConcurrentAppend(t *testing.T) {
	buf := NewLogBuffer(1000)

	var wg sync.WaitGroup
	for g := 0; g < 10; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				buf.Append(LogEntry{
					Timestamp: time.Now(),
					Level:     LevelInfo,
					Message:   fmt.Sprintf("goroutine-%d-msg-%d", id, i),
					Source:    "test",
				})
			}
		}(g)
	}
	wg.Wait()

	entries, truncated := buf.Drain()
	if len(entries) != 500 {
		t.Fatalf("expected 500 entries, got %d", len(entries))
	}
	if truncated {
		t.Fatal("expected truncated=false")
	}
}

func TestLogBufferEmptyDrain(t *testing.T) {
	buf := NewLogBuffer(10)

	entries, truncated := buf.Drain()
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(entries))
	}
	if truncated {
		t.Fatal("expected truncated=false")
	}
}

func TestGetLogBufferNilContext(t *testing.T) {
	buf := GetLogBuffer(context.Background())
	if buf != nil {
		t.Fatal("expected nil LogBuffer from background context")
	}
}

func TestWithLogBufferRoundTrip(t *testing.T) {
	buf := NewLogBuffer(10)
	ctx := WithLogBuffer(context.Background(), buf)

	got := GetLogBuffer(ctx)
	if got != buf {
		t.Fatal("expected same *LogBuffer pointer from context round-trip")
	}
}

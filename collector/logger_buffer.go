package collector

import (
	"context"
	"sync"
)

// logBufferKey is the unexported context key used to store a LogBuffer.
type logBufferKey struct{}

// WithLogBuffer returns a new context with the given LogBuffer stored in it.
func WithLogBuffer(ctx context.Context, buf *LogBuffer) context.Context {
	return context.WithValue(ctx, logBufferKey{}, buf)
}

// GetLogBuffer retrieves the LogBuffer from the context.
// Returns nil if no buffer is found.
func GetLogBuffer(ctx context.Context) *LogBuffer {
	buf, ok := ctx.Value(logBufferKey{}).(*LogBuffer)
	if !ok {
		return nil
	}
	return buf
}

// LogBuffer is a concurrency-safe, bounded buffer for collecting log entries
// during a single request lifecycle.
type LogBuffer struct {
	mu         sync.Mutex
	entries    []LogEntry
	maxEntries int
	truncated  bool
}

// NewLogBuffer creates a new LogBuffer with the given maximum number of entries.
// The internal slice is pre-allocated with a capacity of min(maxEntries, 64).
func NewLogBuffer(maxEntries int) *LogBuffer {
	cap := maxEntries
	if cap > 64 {
		cap = 64
	}
	return &LogBuffer{
		entries:    make([]LogEntry, 0, cap),
		maxEntries: maxEntries,
	}
}

// Append adds a log entry to the buffer. If the buffer has reached its maximum
// capacity, the entry is discarded and the truncated flag is set.
func (lb *LogBuffer) Append(entry LogEntry) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	if len(lb.entries) >= lb.maxEntries {
		lb.truncated = true
		return
	}
	lb.entries = append(lb.entries, entry)
}

// Drain returns all buffered log entries and whether the buffer was truncated,
// then resets the buffer to an empty state.
func (lb *LogBuffer) Drain() ([]LogEntry, bool) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	entries := lb.entries
	truncated := lb.truncated

	lb.entries = nil
	lb.truncated = false

	return entries, truncated
}

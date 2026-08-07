package http

import (
	"context"
	"sync"
)

type contextKeyType struct{}

var contextKey = contextKeyType{}

// requestCalls holds the per-request accumulation of HTTP call entries.
type requestCalls struct {
	mu    sync.Mutex
	calls []HTTPCallEntry
	index int
}

// WithContext initializes HTTP call tracking in the context.
// If tracking is already active, the context is returned unchanged (idempotent).
func WithContext(ctx context.Context) context.Context {
	if ctx.Value(contextKey) != nil {
		return ctx
	}
	return context.WithValue(ctx, contextKey, &requestCalls{
		calls: make([]HTTPCallEntry, 0, 16),
	})
}

// callsFromContext retrieves the internal requestCalls tracker from context.
// Returns nil if no tracking is active.
func callsFromContext(ctx context.Context) *requestCalls {
	rc, _ := ctx.Value(contextKey).(*requestCalls)
	return rc
}

// CallsFromContext retrieves all captured HTTP calls from the context.
// Returns nil if no tracking is active. The returned slice is a copy.
func CallsFromContext(ctx context.Context) []HTTPCallEntry {
	rc := callsFromContext(ctx)
	if rc == nil {
		return nil
	}
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if len(rc.calls) == 0 {
		return nil
	}
	result := make([]HTTPCallEntry, len(rc.calls))
	copy(result, rc.calls)
	return result
}

// appendCall adds an HTTP call entry to the per-request tracker.
// Thread-safe for concurrent use from multiple goroutines.
func appendCall(ctx context.Context, entry HTTPCallEntry) {
	rc := callsFromContext(ctx)
	if rc == nil {
		return
	}
	rc.mu.Lock()
	defer rc.mu.Unlock()
	entry.Index = rc.index
	rc.index++
	rc.calls = append(rc.calls, entry)
}

package collector

import (
	"context"
	"log/slog"
)

// forwardRecord holds the data needed to forward a log record to an inner handler.
type forwardRecord struct {
	ctx    context.Context
	record slog.Record
	inner  slog.Handler
}

// LogForwarder asynchronously forwards log records to their inner handlers
// via a buffered channel. When the channel is full, records are handled
// synchronously as a fallback.
type LogForwarder struct {
	ch   chan forwardRecord
	done chan struct{}
}

// NewLogForwarder creates a new LogForwarder with the given buffer capacity
// and starts the background forwarding goroutine.
func NewLogForwarder(bufferSize int) *LogForwarder {
	f := &LogForwarder{
		ch:   make(chan forwardRecord, bufferSize),
		done: make(chan struct{}),
	}
	go f.run()
	return f
}

// run processes forwarded records until the channel is closed, then signals
// completion via the done channel.
func (f *LogForwarder) run() {
	defer close(f.done)
	for rec := range f.ch {
		_ = rec.inner.Handle(rec.ctx, rec.record)
	}
}

// Forward attempts to send the record to the background goroutine for async
// handling. If the channel is full, it falls back to synchronous handling.
func (f *LogForwarder) Forward(ctx context.Context, r slog.Record, inner slog.Handler) {
	select {
	case f.ch <- forwardRecord{ctx, r, inner}:
	default:
		_ = inner.Handle(ctx, r)
	}
}

// Close stops the forwarder by closing the channel and waits for the
// background goroutine to finish draining any remaining records.
func (f *LogForwarder) Close() {
	close(f.ch)
	<-f.done
}

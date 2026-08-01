package collector

import (
	"context"
	"io"
	"log"
	"strings"
	"sync/atomic"
	"time"
)

// stdLogForwardRecord holds a copy of the log data and the original writer
// to which it should be forwarded asynchronously.
type stdLogForwardRecord struct {
	data     []byte
	original io.Writer
}

// StdLogAdapter intercepts writes to the standard log package and captures
// them as LogEntry values via the provided CaptureFunc. Forwarding to the
// original writer happens asynchronously through a buffered channel.
type StdLogAdapter struct {
	original  io.Writer
	capture   CaptureFunc
	activeCtx atomic.Pointer[context.Context]
	forwardCh chan stdLogForwardRecord
	done      chan struct{}
}

// NewStdLogAdapter creates a new StdLogAdapter with the given capture function
// and channel buffer size, and starts the background forwarder goroutine.
func NewStdLogAdapter(capture CaptureFunc, bufferSize int) *StdLogAdapter {
	a := &StdLogAdapter{
		capture:   capture,
		forwardCh: make(chan stdLogForwardRecord, bufferSize),
		done:      make(chan struct{}),
	}
	go a.runForwarder()
	return a
}

// runForwarder drains the forward channel and writes each record to its
// original writer. It closes the done channel when the forward channel is
// closed.
func (a *StdLogAdapter) runForwarder() {
	defer close(a.done)
	for rec := range a.forwardCh {
		rec.original.Write(rec.data)
	}
}

// Write satisfies the io.Writer interface. It captures the log message,
// then asynchronously forwards the data to the original writer.
func (a *StdLogAdapter) Write(p []byte) (int, error) {
	msg := strings.TrimSpace(string(p))

	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     LevelInfo,
		Message:   msg,
		Source:    "log",
	}

	var ctx context.Context
	ctxPtr := a.activeCtx.Load()
	if ctxPtr != nil {
		ctx = *ctxPtr
	} else {
		ctx = context.Background()
	}

	a.capture(ctx, entry)

	data := make([]byte, len(p))
	copy(data, p)

	select {
	case a.forwardCh <- stdLogForwardRecord{data: data, original: a.original}:
	default:
		a.original.Write(p)
	}

	return len(p), nil
}

// SetActiveContext stores the given context so that subsequent Write calls
// use it when invoking the capture function.
func (a *StdLogAdapter) SetActiveContext(ctx context.Context) {
	a.activeCtx.Store(&ctx)
}

// Close shuts down the forwarder goroutine and waits for it to finish
// processing any remaining records.
func (a *StdLogAdapter) Close() {
	close(a.forwardCh)
	<-a.done
}

// stdLogLogAdapter implements the LogAdapter interface for the standard log
// package.
type stdLogLogAdapter struct {
	bufferSize int
}

// Name returns the identifier for this adapter.
func (a *stdLogLogAdapter) Name() string {
	return "log"
}

// Install sets up the standard log package to forward entries to the given
// capture function. It returns a RemoveFunc that restores the original writer
// and shuts down the adapter.
func (a *stdLogLogAdapter) Install(capture CaptureFunc) RemoveFunc {
	original := log.Writer()
	adapter := NewStdLogAdapter(capture, a.bufferSize)
	adapter.original = original
	log.SetOutput(adapter)
	return func() {
		log.SetOutput(original)
		adapter.Close()
	}
}

// NewStdLogLogAdapter creates a new stdLogLogAdapter with the given buffer
// size for the async forwarding channel.
func NewStdLogLogAdapter(bufferSize int) *stdLogLogAdapter {
	return &stdLogLogAdapter{bufferSize: bufferSize}
}

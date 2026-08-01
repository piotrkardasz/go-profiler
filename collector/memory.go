package collector

import (
	"context"
	"net/http"
)

// memoryStatsKey is the context key for storing pre-handler memory snapshot.
type memoryStatsKeyType struct{}

var memoryStatsKey = memoryStatsKeyType{}

// MemoryData holds memory usage information captured before and after request handling.
type MemoryData struct {
	// AllocBefore is the bytes allocated before the handler ran.
	AllocBefore uint64 `json:"alloc_before"`

	// AllocAfter is the bytes allocated after the handler ran.
	AllocAfter uint64 `json:"alloc_after"`

	// AllocDelta is the difference in allocated bytes (after - before).
	AllocDelta int64 `json:"alloc_delta"`

	// TotalAlloc is the cumulative bytes allocated (after handler).
	TotalAlloc uint64 `json:"total_alloc"`

	// HeapAlloc is the current heap allocation in bytes.
	HeapAlloc uint64 `json:"heap_alloc"`

	// HeapInuse is the heap bytes in use.
	HeapInuse uint64 `json:"heap_inuse"`

	// HeapObjects is the number of allocated heap objects.
	HeapObjects uint64 `json:"heap_objects"`

	// NumGC is the number of completed GC cycles.
	NumGC uint32 `json:"num_gc"`

	// GoroutineCount is the number of goroutines at the time of collection.
	GoroutineCount int `json:"goroutine_count"`

	// Sys is the total bytes of memory obtained from the OS.
	Sys uint64 `json:"sys"`
}

// MemoryCollector captures memory statistics before and after request handling.
// It uses the runtime/metrics package which does NOT require stop-the-world
// pauses, unlike runtime.ReadMemStats.
type MemoryCollector struct{}

// NewMemoryCollector creates a new MemoryCollector.
func NewMemoryCollector() *MemoryCollector {
	return &MemoryCollector{}
}

// Name returns the collector identifier.
func (c *MemoryCollector) Name() string {
	return "memory"
}

// Collect gathers memory statistics after the handler has run.
// It compares against the pre-handler snapshot stored in the context.
// Uses runtime/metrics (no stop-the-world) instead of runtime.ReadMemStats.
func (c *MemoryCollector) Collect(ctx context.Context, _ *http.Request, _ ResponseData) (any, error) {
	afterSnap := captureMemorySnapshot()

	data := &MemoryData{
		AllocAfter:     afterSnap.HeapObjects,
		TotalAlloc:     afterSnap.HeapAllocs,
		HeapAlloc:      afterSnap.HeapObjects,
		HeapInuse:      afterSnap.HeapObjects + afterSnap.HeapUnused,
		HeapObjects:    afterSnap.HeapObjCount,
		NumGC:          uint32(afterSnap.GCCycles),
		GoroutineCount: int(afterSnap.Goroutines),
		Sys:            afterSnap.TotalMemory,
	}

	// If we have a pre-handler snapshot, compute the delta
	if beforeSnap, ok := MemorySnapshotFromContext(ctx); ok {
		data.AllocBefore = beforeSnap.HeapObjects
		data.AllocDelta = int64(afterSnap.HeapObjects) - int64(beforeSnap.HeapObjects)
	}

	return data, nil
}

// Reset clears internal state (no-op for this collector).
func (c *MemoryCollector) Reset() {}

// PanelMeta returns UI panel metadata for this collector.
func (c *MemoryCollector) PanelMeta() PanelMeta {
	return PanelMeta{
		Name:      "memory",
		Label:     "Memory",
		Icon:      "cpu",
		Component: "MemoryPanel",
	}
}

// WithMemoryStats captures a memory snapshot and stores it in the context.
// This should be called at the beginning of the middleware chain.
// Uses runtime/metrics (no stop-the-world) instead of runtime.ReadMemStats.
func WithMemoryStats(ctx context.Context) context.Context {
	snap := captureMemorySnapshot()
	return context.WithValue(ctx, memoryStatsKey, &snap)
}

// MemorySnapshotFromContext retrieves the pre-handler memory snapshot from the context.
func MemorySnapshotFromContext(ctx context.Context) (*memorySnapshot, bool) {
	snap, ok := ctx.Value(memoryStatsKey).(*memorySnapshot)
	return snap, ok
}

// MemoryStatsFromContext is kept for backward compatibility.
// Deprecated: Use MemorySnapshotFromContext instead.
func MemoryStatsFromContext(ctx context.Context) (*memorySnapshot, bool) {
	return MemorySnapshotFromContext(ctx)
}

package collector

import (
	"context"
	"net/http"
	"runtime"
)

// memoryStatsKey is the context key for storing pre-handler memory stats.
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
func (c *MemoryCollector) Collect(ctx context.Context, _ *http.Request, _ ResponseData) (any, error) {
	var afterStats runtime.MemStats
	runtime.ReadMemStats(&afterStats)

	data := &MemoryData{
		AllocAfter:     afterStats.Alloc,
		TotalAlloc:     afterStats.TotalAlloc,
		HeapAlloc:      afterStats.HeapAlloc,
		HeapInuse:      afterStats.HeapInuse,
		HeapObjects:    afterStats.HeapObjects,
		NumGC:          afterStats.NumGC,
		GoroutineCount: runtime.NumGoroutine(),
		Sys:            afterStats.Sys,
	}

	// If we have a pre-handler snapshot, compute the delta
	if beforeStats, ok := MemoryStatsFromContext(ctx); ok {
		data.AllocBefore = beforeStats.Alloc
		data.AllocDelta = int64(afterStats.Alloc) - int64(beforeStats.Alloc)
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

// WithMemoryStats captures a memory stats snapshot and stores it in the context.
// This should be called at the beginning of the middleware chain.
func WithMemoryStats(ctx context.Context) context.Context {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return context.WithValue(ctx, memoryStatsKey, &stats)
}

// MemoryStatsFromContext retrieves the pre-handler memory stats from the context.
func MemoryStatsFromContext(ctx context.Context) (*runtime.MemStats, bool) {
	stats, ok := ctx.Value(memoryStatsKey).(*runtime.MemStats)
	return stats, ok
}

package collector

import (
	"runtime/metrics"
)

// Metric names used for memory statistics collection.
// These are read via runtime/metrics which does NOT stop the world,
// unlike runtime.ReadMemStats.
const (
	metricHeapObjects = "/memory/classes/heap/objects:bytes"
	metricHeapUnused  = "/memory/classes/heap/unused:bytes"
	metricTotalMemory = "/memory/classes/total:bytes"
	metricHeapAllocs  = "/gc/heap/allocs:bytes"
	metricHeapObjs    = "/gc/heap/objects:objects"
	metricGCCycles    = "/gc/cycles/total:gc-cycles"
	metricGoroutines  = "/sched/goroutines:goroutines"
)

// metricIndex maps metric names to their index in the samples slice.
var metricIndex = map[string]int{
	metricHeapObjects: 0,
	metricHeapUnused:  1,
	metricTotalMemory: 2,
	metricHeapAllocs:  3,
	metricHeapObjs:    4,
	metricGCCycles:    5,
	metricGoroutines:  6,
}

// memorySnapshot holds a point-in-time snapshot of memory metrics
// captured via the runtime/metrics package (no stop-the-world).
type memorySnapshot struct {
	// HeapObjects is the bytes in live heap objects.
	HeapObjects uint64
	// HeapUnused is the bytes in the heap that are not in use.
	HeapUnused uint64
	// TotalMemory is the total bytes of memory mapped by the Go runtime.
	TotalMemory uint64
	// HeapAllocs is the cumulative bytes allocated on the heap.
	HeapAllocs uint64
	// HeapObjCount is the number of live heap objects.
	HeapObjCount uint64
	// GCCycles is the total number of completed GC cycles.
	GCCycles uint64
	// Goroutines is the number of live goroutines.
	Goroutines uint64
}

// captureMemorySnapshot reads memory metrics using runtime/metrics.
// This does NOT stop the world, unlike runtime.ReadMemStats.
func captureMemorySnapshot() memorySnapshot {
	samples := []metrics.Sample{
		{Name: metricHeapObjects},
		{Name: metricHeapUnused},
		{Name: metricTotalMemory},
		{Name: metricHeapAllocs},
		{Name: metricHeapObjs},
		{Name: metricGCCycles},
		{Name: metricGoroutines},
	}

	metrics.Read(samples)

	return memorySnapshot{
		HeapObjects:  readUint64(samples[metricIndex[metricHeapObjects]]),
		HeapUnused:   readUint64(samples[metricIndex[metricHeapUnused]]),
		TotalMemory:  readUint64(samples[metricIndex[metricTotalMemory]]),
		HeapAllocs:   readUint64(samples[metricIndex[metricHeapAllocs]]),
		HeapObjCount: readUint64(samples[metricIndex[metricHeapObjs]]),
		GCCycles:     readUint64(samples[metricIndex[metricGCCycles]]),
		Goroutines:   readUint64(samples[metricIndex[metricGoroutines]]),
	}
}

// readUint64 extracts a uint64 value from a metrics.Sample.
// It handles both KindUint64 and KindFloat64 value kinds.
func readUint64(s metrics.Sample) uint64 {
	switch s.Value.Kind() {
	case metrics.KindUint64:
		return s.Value.Uint64()
	case metrics.KindFloat64:
		v := s.Value.Float64()
		if v < 0 {
			return 0
		}
		return uint64(v)
	default:
		return 0
	}
}

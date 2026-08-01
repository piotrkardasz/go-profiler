package collector

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMemoryCollectorName(t *testing.T) {
	c := NewMemoryCollector()
	if c.Name() != "memory" {
		t.Errorf("Name(): got %q, want %q", c.Name(), "memory")
	}
}

func TestMemoryCollectorCollect(t *testing.T) {
	c := NewMemoryCollector()

	// Store pre-handler memory snapshot in context
	ctx := WithMemoryStats(context.Background())

	// Simulate some allocations
	_ = make([]byte, 1024*1024) // 1MB allocation

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	res := ResponseData{StatusCode: 200}

	result, err := c.Collect(ctx, req, res)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, ok := result.(*MemoryData)
	if !ok {
		t.Fatalf("expected *MemoryData, got %T", result)
	}

	// Basic sanity checks — values should be populated
	if data.AllocAfter == 0 {
		t.Error("AllocAfter is 0")
	}
	if data.AllocBefore == 0 {
		t.Error("AllocBefore is 0 (pre-handler snapshot should have been captured)")
	}
	if data.HeapAlloc == 0 {
		t.Error("HeapAlloc is 0")
	}
	if data.HeapInuse == 0 {
		t.Error("HeapInuse is 0")
	}
	if data.GoroutineCount == 0 {
		t.Error("GoroutineCount is 0")
	}
	if data.Sys == 0 {
		t.Error("Sys is 0")
	}
	if data.TotalAlloc == 0 {
		t.Error("TotalAlloc is 0")
	}
}

func TestMemoryCollectorWithoutContextStats(t *testing.T) {
	c := NewMemoryCollector()

	// No pre-handler stats in context
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	res := ResponseData{StatusCode: 200}

	result, err := c.Collect(context.Background(), req, res)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := result.(*MemoryData)

	// Without pre-handler stats, AllocBefore should be 0 and delta should be 0
	if data.AllocBefore != 0 {
		t.Errorf("AllocBefore: got %d, want 0 (no pre-handler stats)", data.AllocBefore)
	}
	if data.AllocDelta != 0 {
		t.Errorf("AllocDelta: got %d, want 0 (no pre-handler stats)", data.AllocDelta)
	}

	// Post-handler stats should still be captured
	if data.AllocAfter == 0 {
		t.Error("AllocAfter is 0")
	}
	if data.GoroutineCount == 0 {
		t.Error("GoroutineCount is 0")
	}
}

func TestMemoryCollectorPanelMeta(t *testing.T) {
	c := NewMemoryCollector()
	meta := c.PanelMeta()

	if meta.Name != "memory" {
		t.Errorf("PanelMeta.Name: got %q, want %q", meta.Name, "memory")
	}
	if meta.Label != "Memory" {
		t.Errorf("PanelMeta.Label: got %q, want %q", meta.Label, "Memory")
	}
	if meta.Component != "MemoryPanel" {
		t.Errorf("PanelMeta.Component: got %q, want %q", meta.Component, "MemoryPanel")
	}
}

func TestWithMemoryStatsAndFromContext(t *testing.T) {
	ctx := WithMemoryStats(context.Background())

	snap, ok := MemorySnapshotFromContext(ctx)
	if !ok {
		t.Fatal("expected memory snapshot in context")
	}
	if snap == nil {
		t.Fatal("snapshot is nil")
	}
	if snap.HeapObjects == 0 {
		t.Error("snap.HeapObjects is 0")
	}
}

func TestMemorySnapshotFromContextMissing(t *testing.T) {
	_, ok := MemorySnapshotFromContext(context.Background())
	if ok {
		t.Error("expected ok=false for context without memory snapshot")
	}
}

func TestCaptureMemorySnapshot(t *testing.T) {
	snap := captureMemorySnapshot()

	if snap.HeapObjects == 0 {
		t.Error("HeapObjects is 0")
	}
	if snap.TotalMemory == 0 {
		t.Error("TotalMemory is 0")
	}
	if snap.Goroutines == 0 {
		t.Error("Goroutines is 0")
	}
	if snap.HeapAllocs == 0 {
		t.Error("HeapAllocs is 0")
	}
}

func TestMemoryStatsFromContextBackwardCompat(t *testing.T) {
	// MemoryStatsFromContext should still work (deprecated but functional)
	ctx := WithMemoryStats(context.Background())
	snap, ok := MemoryStatsFromContext(ctx)
	if !ok {
		t.Fatal("expected snapshot via deprecated MemoryStatsFromContext")
	}
	if snap == nil {
		t.Fatal("snapshot is nil")
	}
}

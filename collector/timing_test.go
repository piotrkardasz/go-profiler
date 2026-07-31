package collector

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTimingCollectorName(t *testing.T) {
	c := NewTimingCollector()
	if c.Name() != "timing" {
		t.Errorf("Name(): got %q, want %q", c.Name(), "timing")
	}
}

func TestTimingCollectorCollect(t *testing.T) {
	c := NewTimingCollector()

	// Simulate middleware storing start time
	startTime := time.Now()
	ctx := WithStartTime(context.Background(), startTime)

	// Simulate some work
	time.Sleep(10 * time.Millisecond)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	res := ResponseData{StatusCode: 200}

	result, err := c.Collect(ctx, req, res)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, ok := result.(*TimingData)
	if !ok {
		t.Fatalf("expected *TimingData, got %T", result)
	}

	if data.StartTime.IsZero() {
		t.Error("StartTime is zero")
	}
	if data.EndTime.IsZero() {
		t.Error("EndTime is zero")
	}
	if !data.StartTime.Equal(startTime) {
		t.Errorf("StartTime: got %v, want %v", data.StartTime, startTime)
	}
	if data.DurationMs < 10 {
		t.Errorf("DurationMs: got %f, want >= 10", data.DurationMs)
	}
	if data.EndTime.Before(data.StartTime) {
		t.Error("EndTime is before StartTime")
	}
}

func TestTimingCollectorWithoutContextStartTime(t *testing.T) {
	c := NewTimingCollector()

	// No start time in context — should fallback to zero duration
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	res := ResponseData{StatusCode: 200}

	result, err := c.Collect(context.Background(), req, res)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := result.(*TimingData)

	// When no start time in context, start = end, so duration should be ~0
	if data.DurationMs > 1 {
		t.Errorf("DurationMs without context start time should be ~0, got %f", data.DurationMs)
	}
}

func TestTimingCollectorPanelMeta(t *testing.T) {
	c := NewTimingCollector()
	meta := c.PanelMeta()

	if meta.Name != "timing" {
		t.Errorf("PanelMeta.Name: got %q, want %q", meta.Name, "timing")
	}
	if meta.Label != "Timing" {
		t.Errorf("PanelMeta.Label: got %q, want %q", meta.Label, "Timing")
	}
	if meta.Component != "TimingPanel" {
		t.Errorf("PanelMeta.Component: got %q, want %q", meta.Component, "TimingPanel")
	}
}

func TestWithStartTimeAndStartTimeFromContext(t *testing.T) {
	now := time.Now()
	ctx := WithStartTime(context.Background(), now)

	got, ok := StartTimeFromContext(ctx)
	if !ok {
		t.Fatal("expected start time in context")
	}
	if !got.Equal(now) {
		t.Errorf("got %v, want %v", got, now)
	}
}

func TestStartTimeFromContextMissing(t *testing.T) {
	_, ok := StartTimeFromContext(context.Background())
	if ok {
		t.Error("expected ok=false for context without start time")
	}
}

package collector

import (
	"context"
	"net/http"
	"time"
)

// timingStartKey is the context key for storing the request start time.
type timingStartKeyType struct{}

var timingStartKey = timingStartKeyType{}

// TimingData holds timing information for the request.
type TimingData struct {
	// StartTime is when the request handling began.
	StartTime time.Time `json:"start_time"`

	// EndTime is when the request handling completed.
	EndTime time.Time `json:"end_time"`

	// DurationMs is the total request handling time in milliseconds.
	DurationMs float64 `json:"duration_ms"`
}

// TimingCollector records the start and end time of request processing.
type TimingCollector struct{}

// NewTimingCollector creates a new TimingCollector.
func NewTimingCollector() *TimingCollector {
	return &TimingCollector{}
}

// Name returns the collector identifier.
func (c *TimingCollector) Name() string {
	return "timing"
}

// Collect computes the request duration using the start time stored in context.
func (c *TimingCollector) Collect(ctx context.Context, _ *http.Request, _ ResponseData) (any, error) {
	endTime := time.Now()

	startTime, ok := StartTimeFromContext(ctx)
	if !ok {
		// Fallback: if no start time in context, use end time (zero duration)
		startTime = endTime
	}

	duration := endTime.Sub(startTime)

	return &TimingData{
		StartTime:  startTime,
		EndTime:    endTime,
		DurationMs: float64(duration.Microseconds()) / 1000.0,
	}, nil
}

// Reset clears internal state (no-op for this collector).
func (c *TimingCollector) Reset() {}

// PanelMeta returns UI panel metadata for this collector.
func (c *TimingCollector) PanelMeta() PanelMeta {
	return PanelMeta{
		Name:      "timing",
		Label:     "Timing",
		Icon:      "clock",
		Component: "TimingPanel",
	}
}

// WithStartTime returns a new context with the request start time attached.
// This should be called at the beginning of the middleware chain.
func WithStartTime(ctx context.Context, t time.Time) context.Context {
	return context.WithValue(ctx, timingStartKey, t)
}

// StartTimeFromContext retrieves the request start time from the context.
func StartTimeFromContext(ctx context.Context) (time.Time, bool) {
	t, ok := ctx.Value(timingStartKey).(time.Time)
	return t, ok
}

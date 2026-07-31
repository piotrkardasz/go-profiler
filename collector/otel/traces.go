// Package otel provides an OpenTelemetry collector for the Go profiler.
// It captures traces (spans) and metrics associated with each HTTP request,
// demonstrating how to build a custom LateCollector that integrates with
// the OpenTelemetry SDK.
package otel

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/piotrkardasz/go-profiler/collector"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// SpanInfo holds profiler-friendly information about a captured span.
type SpanInfo struct {
	Name       string            `json:"name"`
	TraceID    string            `json:"trace_id"`
	SpanID     string            `json:"span_id"`
	ParentID   string            `json:"parent_id,omitempty"`
	StartTime  time.Time         `json:"start_time"`
	EndTime    time.Time         `json:"end_time"`
	DurationMs float64           `json:"duration_ms"`
	Status     string            `json:"status"`
	Attributes map[string]string `json:"attributes,omitempty"`
	Events     []SpanEvent       `json:"events,omitempty"`
}

// SpanEvent represents an event recorded on a span.
type SpanEvent struct {
	Name       string            `json:"name"`
	Timestamp  time.Time         `json:"timestamp"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

// OtelData is the combined data structure stored in the profile
// for the OpenTelemetry collector.
type OtelData struct {
	Spans   []SpanInfo   `json:"spans"`
	Metrics []MetricInfo `json:"metrics"`
}

// SpanCapturer is a SpanProcessor that captures completed spans
// associated with a specific trace. It implements sdktrace.SpanProcessor.
type SpanCapturer struct {
	mu    sync.Mutex
	spans []sdktrace.ReadOnlySpan
}

// NewSpanCapturer creates a new span capturer.
func NewSpanCapturer() *SpanCapturer {
	return &SpanCapturer{
		spans: make([]sdktrace.ReadOnlySpan, 0),
	}
}

// OnStart is called when a span starts (no-op for capture purposes).
func (sc *SpanCapturer) OnStart(_ context.Context, _ sdktrace.ReadWriteSpan) {}

// OnEnd captures the completed span.
func (sc *SpanCapturer) OnEnd(s sdktrace.ReadOnlySpan) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.spans = append(sc.spans, s)
}

// Shutdown is called when the processor is shut down.
func (sc *SpanCapturer) Shutdown(_ context.Context) error { return nil }

// ForceFlush is called to flush pending spans.
func (sc *SpanCapturer) ForceFlush(_ context.Context) error { return nil }

// CapturedSpans returns the captured spans and resets the internal buffer.
func (sc *SpanCapturer) CapturedSpans() []sdktrace.ReadOnlySpan {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	spans := sc.spans
	sc.spans = make([]sdktrace.ReadOnlySpan, 0)
	return spans
}

// tracesContextKey is used to store the SpanCapturer in the request context.
type tracesContextKeyType struct{}

var tracesContextKey = tracesContextKeyType{}

// WithSpanCapturer attaches a SpanCapturer to the context.
func WithSpanCapturer(ctx context.Context, sc *SpanCapturer) context.Context {
	return context.WithValue(ctx, tracesContextKey, sc)
}

// SpanCapturerFromContext retrieves the SpanCapturer from the context.
func SpanCapturerFromContext(ctx context.Context) (*SpanCapturer, bool) {
	sc, ok := ctx.Value(tracesContextKey).(*SpanCapturer)
	return sc, ok
}

// TracesCollector captures OpenTelemetry spans associated with a request.
// It implements collector.LateCollector because spans may not be complete
// until after the response is sent.
type TracesCollector struct {
	capturer *SpanCapturer
}

// NewTracesCollector creates a new TracesCollector.
// The provided SpanCapturer should be registered as a SpanProcessor with
// the OpenTelemetry TracerProvider.
func NewTracesCollector(capturer *SpanCapturer) *TracesCollector {
	return &TracesCollector{capturer: capturer}
}

// Name returns the collector identifier.
func (c *TracesCollector) Name() string {
	return "otel"
}

// Collect gathers initial trace data from the request context.
// The main span data is collected in LateCollect after spans complete.
func (c *TracesCollector) Collect(ctx context.Context, _ *http.Request, _ collector.ResponseData) (any, error) {
	// Capture the current span context for association
	spanCtx := trace.SpanContextFromContext(ctx)
	return &OtelData{
		Spans: []SpanInfo{
			{
				TraceID: spanCtx.TraceID().String(),
				SpanID:  spanCtx.SpanID().String(),
				Name:    "request (pending late collection)",
			},
		},
		Metrics: []MetricInfo{},
	}, nil
}

// LateCollect captures all spans that completed during the request.
func (c *TracesCollector) LateCollect(_ context.Context) (any, error) {
	rawSpans := c.capturer.CapturedSpans()

	spans := make([]SpanInfo, 0, len(rawSpans))
	for _, s := range rawSpans {
		info := SpanInfo{
			Name:       s.Name(),
			TraceID:    s.SpanContext().TraceID().String(),
			SpanID:     s.SpanContext().SpanID().String(),
			StartTime:  s.StartTime(),
			EndTime:    s.EndTime(),
			DurationMs: float64(s.EndTime().Sub(s.StartTime()).Microseconds()) / 1000.0,
			Status:     s.Status().Code.String(),
		}

		if s.Parent().HasSpanID() {
			info.ParentID = s.Parent().SpanID().String()
		}

		// Convert attributes
		if attrs := s.Attributes(); len(attrs) > 0 {
			info.Attributes = make(map[string]string, len(attrs))
			for _, kv := range attrs {
				info.Attributes[string(kv.Key)] = kv.Value.Emit()
			}
		}

		// Convert events
		if events := s.Events(); len(events) > 0 {
			info.Events = make([]SpanEvent, 0, len(events))
			for _, ev := range events {
				event := SpanEvent{
					Name:      ev.Name,
					Timestamp: ev.Time,
				}
				if len(ev.Attributes) > 0 {
					event.Attributes = make(map[string]string, len(ev.Attributes))
					for _, kv := range ev.Attributes {
						event.Attributes[string(kv.Key)] = kv.Value.Emit()
					}
				}
				info.Events = append(info.Events, event)
			}
		}

		spans = append(spans, info)
	}

	return &OtelData{
		Spans:   spans,
		Metrics: []MetricInfo{}, // Metrics handled by MetricsCollector if combined
	}, nil
}

// Reset clears internal state.
func (c *TracesCollector) Reset() {}

// PanelMeta returns UI panel metadata.
func (c *TracesCollector) PanelMeta() collector.PanelMeta {
	return collector.PanelMeta{
		Name:      "otel",
		Label:     "OpenTelemetry",
		Icon:      "telescope",
		Component: "OtelPanel",
	}
}

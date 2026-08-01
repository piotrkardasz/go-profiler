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

// SpanCapturer is a SpanProcessor that captures completed spans keyed by
// their trace ID, enabling per-request span retrieval. It implements
// sdktrace.SpanProcessor.
type SpanCapturer struct {
	mu sync.Mutex
	// spans keyed by trace ID for per-request isolation.
	// Each trace ID maps to the list of spans belonging to that trace.
	byTraceID map[trace.TraceID][]sdktrace.ReadOnlySpan
	// orphans holds spans that arrive without a registered trace ID.
	// This allows backward-compatible CapturedSpans() to still work.
	orphans []sdktrace.ReadOnlySpan
}

// NewSpanCapturer creates a new span capturer.
func NewSpanCapturer() *SpanCapturer {
	return &SpanCapturer{
		byTraceID: make(map[trace.TraceID][]sdktrace.ReadOnlySpan),
		orphans:   make([]sdktrace.ReadOnlySpan, 0),
	}
}

// OnStart is called when a span starts (no-op for capture purposes).
func (sc *SpanCapturer) OnStart(_ context.Context, _ sdktrace.ReadWriteSpan) {}

// OnEnd captures the completed span, indexed by its trace ID.
func (sc *SpanCapturer) OnEnd(s sdktrace.ReadOnlySpan) {
	traceID := s.SpanContext().TraceID()

	sc.mu.Lock()
	defer sc.mu.Unlock()

	// Always store by trace ID for correlation lookup
	sc.byTraceID[traceID] = append(sc.byTraceID[traceID], s)
}

// Shutdown is called when the processor is shut down.
func (sc *SpanCapturer) Shutdown(_ context.Context) error { return nil }

// ForceFlush is called to flush pending spans.
func (sc *SpanCapturer) ForceFlush(_ context.Context) error { return nil }

// CapturedSpans returns ALL captured spans (across all trace IDs) and resets
// the internal buffer. This preserves backward compatibility for users who
// don't use per-request isolation.
func (sc *SpanCapturer) CapturedSpans() []sdktrace.ReadOnlySpan {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	var all []sdktrace.ReadOnlySpan
	for _, spans := range sc.byTraceID {
		all = append(all, spans...)
	}
	all = append(all, sc.orphans...)

	sc.byTraceID = make(map[trace.TraceID][]sdktrace.ReadOnlySpan)
	sc.orphans = make([]sdktrace.ReadOnlySpan, 0)
	return all
}

// CapturedSpansForTrace returns only spans belonging to the given trace ID
// and removes them from the buffer. Other traces' spans remain untouched.
// This is the key method for per-request isolation.
func (sc *SpanCapturer) CapturedSpansForTrace(traceID trace.TraceID) []sdktrace.ReadOnlySpan {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	spans := sc.byTraceID[traceID]
	delete(sc.byTraceID, traceID)
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

// otelContextKeyType is used to store the request's trace ID in context
// for per-request correlation during LateCollect.
type otelContextKeyType struct{}

var otelContextKey = otelContextKeyType{}

// otelRequestContext holds per-request OTel state set during SetupContext.
type otelRequestContext struct {
	traceID trace.TraceID
}

// withOtelRequestContext stores OTel request context.
func withOtelRequestContext(ctx context.Context, rc *otelRequestContext) context.Context {
	return context.WithValue(ctx, otelContextKey, rc)
}

// otelRequestContextFromContext retrieves OTel request context.
func otelRequestContextFromContext(ctx context.Context) (*otelRequestContext, bool) {
	rc, ok := ctx.Value(otelContextKey).(*otelRequestContext)
	return rc, ok
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

// LateCollect captures only spans belonging to the current request's trace.
func (c *TracesCollector) LateCollect(ctx context.Context) (any, error) {
	var rawSpans []sdktrace.ReadOnlySpan

	// Try per-request isolation via context
	if rc, ok := otelRequestContextFromContext(ctx); ok && rc.traceID.IsValid() {
		rawSpans = c.capturer.CapturedSpansForTrace(rc.traceID)
	} else {
		// Fallback: drain all (backward compat for users not using SetupContext)
		rawSpans = c.capturer.CapturedSpans()
	}

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

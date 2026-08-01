package otel

import (
	"context"
	"net/http"

	"github.com/piotrkardasz/go-profiler/collector"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// Collector is the combined OpenTelemetry collector that captures both
// traces and metrics for each profiled request. It implements LateCollector
// and ContextSetup for per-request span/metric isolation.
//
// Per-request isolation works as follows:
//   - SetupContext captures the request's trace ID from the active span context
//     and registers it for metric tracking.
//   - LateCollect uses the trace ID to drain only spans belonging to this request,
//     preventing cross-request bleed when multiple requests are in flight.
//   - Metrics are windowed per-request (metrics exported between request start
//     and LateCollect are attributed to the request).
type Collector struct {
	spanCapturer   *SpanCapturer
	metricCapturer *MetricCapturer
}

// NewCollector creates a combined OTel collector using the provided capturers.
// The SpanCapturer should be registered as a SpanProcessor with the TracerProvider,
// and the MetricCapturer should be used as the metric Exporter.
func NewCollector(spanCapturer *SpanCapturer, metricCapturer *MetricCapturer) *Collector {
	return &Collector{
		spanCapturer:   spanCapturer,
		metricCapturer: metricCapturer,
	}
}

// Name returns the collector identifier.
func (c *Collector) Name() string {
	return "otel"
}

// SetupContext implements collector.ContextSetup. It captures the current
// request's trace ID from the span context and stores it for LateCollect
// to use for per-request span filtering. It also starts metric tracking
// for this request.
func (c *Collector) SetupContext(ctx context.Context) context.Context {
	spanCtx := trace.SpanContextFromContext(ctx)
	rc := &otelRequestContext{
		traceID: spanCtx.TraceID(),
	}

	// Start per-request metric tracking using trace ID as request key
	if c.metricCapturer != nil && spanCtx.TraceID().IsValid() {
		c.metricCapturer.StartRequestMetrics(spanCtx.TraceID().String())
	}

	return withOtelRequestContext(ctx, rc)
}

// Collect captures initial OTel context (trace/span IDs) from the request.
func (c *Collector) Collect(ctx context.Context, _ *http.Request, _ collector.ResponseData) (any, error) {
	spanCtx := trace.SpanContextFromContext(ctx)

	data := &OtelData{
		Spans:   []SpanInfo{},
		Metrics: []MetricInfo{},
	}

	// If there's an active span, capture its context
	if spanCtx.IsValid() {
		data.Spans = append(data.Spans, SpanInfo{
			Name:    "request",
			TraceID: spanCtx.TraceID().String(),
			SpanID:  spanCtx.SpanID().String(),
		})
	}

	return data, nil
}

// LateCollect captures spans and metrics for the current request only.
// It uses the trace ID stored in context by SetupContext to filter spans,
// and retrieves only the metrics that were exported during this request's
// lifetime.
func (c *Collector) LateCollect(ctx context.Context) (any, error) {
	data := &OtelData{
		Spans:   []SpanInfo{},
		Metrics: []MetricInfo{},
	}

	// Determine trace ID for per-request filtering
	var requestTraceID trace.TraceID
	if rc, ok := otelRequestContextFromContext(ctx); ok {
		requestTraceID = rc.traceID
	}

	// Collect spans — per-request if trace ID is available, global drain otherwise
	if c.spanCapturer != nil {
		var rawSpans []sdktrace.ReadOnlySpan
		if requestTraceID.IsValid() {
			rawSpans = c.spanCapturer.CapturedSpansForTrace(requestTraceID)
		} else {
			// Fallback: drain all (backward compat)
			rawSpans = c.spanCapturer.CapturedSpans()
		}

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

			if attrs := s.Attributes(); len(attrs) > 0 {
				info.Attributes = make(map[string]string, len(attrs))
				for _, kv := range attrs {
					info.Attributes[string(kv.Key)] = kv.Value.Emit()
				}
			}

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

			data.Spans = append(data.Spans, info)
		}
	}

	// Collect metrics — per-request window if tracking was started, global otherwise
	if c.metricCapturer != nil {
		if requestTraceID.IsValid() {
			data.Metrics = c.metricCapturer.EndRequestMetrics(requestTraceID.String())
			if data.Metrics == nil {
				data.Metrics = []MetricInfo{}
			}
		} else {
			data.Metrics = c.metricCapturer.CapturedMetrics()
		}
	}

	return data, nil
}

// Reset clears internal state.
func (c *Collector) Reset() {}

// PanelMeta returns UI panel metadata for the OTel collector.
func (c *Collector) PanelMeta() collector.PanelMeta {
	return collector.PanelMeta{
		Name:      "otel",
		Label:     "OpenTelemetry",
		Icon:      "telescope",
		Component: "OtelPanel",
	}
}

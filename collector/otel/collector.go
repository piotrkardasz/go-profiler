package otel

import (
	"context"
	"net/http"

	"github.com/piotrkardasz/go-profiler/collector"
	"go.opentelemetry.io/otel/trace"
)

// Collector is the combined OpenTelemetry collector that captures both
// traces and metrics for each profiled request. It implements LateCollector
// because span data may not be available until after the response is sent.
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

// LateCollect captures all spans and metrics that were recorded during the request.
func (c *Collector) LateCollect(_ context.Context) (any, error) {
	data := &OtelData{
		Spans:   []SpanInfo{},
		Metrics: []MetricInfo{},
	}

	// Collect spans
	if c.spanCapturer != nil {
		rawSpans := c.spanCapturer.CapturedSpans()
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

	// Collect metrics
	if c.metricCapturer != nil {
		data.Metrics = c.metricCapturer.CapturedMetrics()
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

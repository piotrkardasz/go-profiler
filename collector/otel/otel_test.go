package otel

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/piotrkardasz/go-profiler/collector"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestCollectorName(t *testing.T) {
	c := NewCollector(NewSpanCapturer(), nil)
	if c.Name() != "otel" {
		t.Errorf("Name(): got %q, want %q", c.Name(), "otel")
	}
}

func TestCollectorImplementsLateCollector(t *testing.T) {
	c := NewCollector(NewSpanCapturer(), nil)
	var _ collector.LateCollector = c // compile-time check
}

func TestCollectorPanelMeta(t *testing.T) {
	c := NewCollector(NewSpanCapturer(), nil)
	meta := c.PanelMeta()
	if meta.Name != "otel" {
		t.Errorf("PanelMeta.Name: got %q, want %q", meta.Name, "otel")
	}
	if meta.Component != "OtelPanel" {
		t.Errorf("PanelMeta.Component: got %q, want %q", meta.Component, "OtelPanel")
	}
}

func TestSpanCapturerCapturesSpans(t *testing.T) {
	capturer := NewSpanCapturer()

	// Create a TracerProvider with our capturer as a processor
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(capturer),
	)
	defer tp.Shutdown(context.Background())

	tracer := tp.Tracer("test")

	// Create some spans
	ctx, span1 := tracer.Start(context.Background(), "parent-span")
	_, span2 := tracer.Start(ctx, "child-span")
	span2.End()
	span1.End()

	// Verify captured spans
	spans := capturer.CapturedSpans()
	if len(spans) != 2 {
		t.Fatalf("expected 2 captured spans, got %d", len(spans))
	}

	// First captured should be child (ended first)
	if spans[0].Name() != "child-span" {
		t.Errorf("first span name: got %q, want %q", spans[0].Name(), "child-span")
	}
	if spans[1].Name() != "parent-span" {
		t.Errorf("second span name: got %q, want %q", spans[1].Name(), "parent-span")
	}

	// After CapturedSpans(), buffer should be empty
	spans2 := capturer.CapturedSpans()
	if len(spans2) != 0 {
		t.Errorf("expected 0 spans after reset, got %d", len(spans2))
	}
}

func TestSpanCapturerCapturesAttributes(t *testing.T) {
	capturer := NewSpanCapturer()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(capturer),
	)
	defer tp.Shutdown(context.Background())

	tracer := tp.Tracer("test")
	_, span := tracer.Start(context.Background(), "test-span")
	span.SetAttributes(
		attribute.String("http.method", "GET"),
		attribute.Int("http.status_code", 200),
	)
	span.End()

	spans := capturer.CapturedSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	attrs := spans[0].Attributes()
	if len(attrs) != 2 {
		t.Fatalf("expected 2 attributes, got %d", len(attrs))
	}
}

func TestCollectorLateCollectWithSpans(t *testing.T) {
	capturer := NewSpanCapturer()
	c := NewCollector(capturer, nil)

	// Create spans using a TracerProvider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(capturer),
	)
	defer tp.Shutdown(context.Background())

	tracer := tp.Tracer("test")
	_, span := tracer.Start(context.Background(), "handler-work")
	span.SetAttributes(attribute.String("db.system", "postgresql"))
	span.End()

	// Late collect should pick up the span
	result, err := c.LateCollect(context.Background())
	if err != nil {
		t.Fatalf("LateCollect error: %v", err)
	}

	data, ok := result.(*OtelData)
	if !ok {
		t.Fatalf("expected *OtelData, got %T", result)
	}

	if len(data.Spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(data.Spans))
	}

	span0 := data.Spans[0]
	if span0.Name != "handler-work" {
		t.Errorf("span name: got %q, want %q", span0.Name, "handler-work")
	}
	if span0.DurationMs <= 0 {
		t.Errorf("span duration should be > 0, got %f", span0.DurationMs)
	}
	if span0.Attributes["db.system"] != "postgresql" {
		t.Errorf("span attribute db.system: got %q, want %q", span0.Attributes["db.system"], "postgresql")
	}
	if span0.TraceID == "" {
		t.Error("span TraceID is empty")
	}
	if span0.SpanID == "" {
		t.Error("span SpanID is empty")
	}
}

func TestCollectorCollectWithActiveSpan(t *testing.T) {
	capturer := NewSpanCapturer()
	c := NewCollector(capturer, nil)

	// Create an active span in context
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(capturer),
	)
	defer tp.Shutdown(context.Background())

	tracer := tp.Tracer("test")
	ctx, span := tracer.Start(context.Background(), "request")
	defer span.End()

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req = req.WithContext(ctx)
	res := collector.ResponseData{StatusCode: 200}

	result, err := c.Collect(ctx, req, res)
	if err != nil {
		t.Fatalf("Collect error: %v", err)
	}

	data, ok := result.(*OtelData)
	if !ok {
		t.Fatalf("expected *OtelData, got %T", result)
	}

	if len(data.Spans) != 1 {
		t.Fatalf("expected 1 initial span info, got %d", len(data.Spans))
	}
	if data.Spans[0].TraceID == "" {
		t.Error("initial span TraceID is empty")
	}
}

func TestMetricCapturerCapturesMetrics(t *testing.T) {
	capturer := NewMetricCapturer(nil)

	// Create ResourceMetrics with test data
	now := time.Now()
	rm := &metricdata.ResourceMetrics{
		ScopeMetrics: []metricdata.ScopeMetrics{
			{
				Metrics: []metricdata.Metrics{
					{
						Name:        "http.request.count",
						Description: "Number of HTTP requests",
						Unit:        "1",
						Data: metricdata.Sum[int64]{
							DataPoints: []metricdata.DataPoint[int64]{
								{
									Value:      42,
									Time:       now,
									Attributes: attribute.NewSet(attribute.String("method", "GET")),
								},
							},
						},
					},
					{
						Name: "http.request.duration",
						Unit: "ms",
						Data: metricdata.Gauge[float64]{
							DataPoints: []metricdata.DataPoint[float64]{
								{
									Value:      123.45,
									Time:       now,
									Attributes: attribute.NewSet(),
								},
							},
						},
					},
				},
			},
		},
	}

	err := capturer.Export(context.Background(), rm)
	if err != nil {
		t.Fatalf("Export error: %v", err)
	}

	metrics := capturer.CapturedMetrics()
	if len(metrics) != 2 {
		t.Fatalf("expected 2 metrics, got %d", len(metrics))
	}

	// First metric: counter
	if metrics[0].Name != "http.request.count" {
		t.Errorf("first metric name: got %q, want %q", metrics[0].Name, "http.request.count")
	}
	if metrics[0].Type != "sum" {
		t.Errorf("first metric type: got %q, want %q", metrics[0].Type, "sum")
	}
	if metrics[0].Value != 42 {
		t.Errorf("first metric value: got %f, want 42", metrics[0].Value)
	}
	if metrics[0].Attributes["method"] != "GET" {
		t.Errorf("first metric attribute: got %q, want 'GET'", metrics[0].Attributes["method"])
	}

	// Second metric: gauge
	if metrics[1].Name != "http.request.duration" {
		t.Errorf("second metric name: got %q, want %q", metrics[1].Name, "http.request.duration")
	}
	if metrics[1].Type != "gauge" {
		t.Errorf("second metric type: got %q, want %q", metrics[1].Type, "gauge")
	}
	if metrics[1].Value != 123.45 {
		t.Errorf("second metric value: got %f, want 123.45", metrics[1].Value)
	}

	// After capture, buffer should be empty
	metrics2 := capturer.CapturedMetrics()
	if len(metrics2) != 0 {
		t.Errorf("expected 0 metrics after reset, got %d", len(metrics2))
	}
}

func TestCollectorLateCollectWithMetrics(t *testing.T) {
	metricCapturer := NewMetricCapturer(nil)
	c := NewCollector(NewSpanCapturer(), metricCapturer)

	// Simulate metric export
	now := time.Now()
	rm := &metricdata.ResourceMetrics{
		ScopeMetrics: []metricdata.ScopeMetrics{
			{
				Metrics: []metricdata.Metrics{
					{
						Name: "app.requests",
						Data: metricdata.Sum[int64]{
							DataPoints: []metricdata.DataPoint[int64]{
								{Value: 10, Time: now, Attributes: attribute.NewSet()},
							},
						},
					},
				},
			},
		},
	}
	metricCapturer.Export(context.Background(), rm)

	// Late collect should get metrics
	result, err := c.LateCollect(context.Background())
	if err != nil {
		t.Fatalf("LateCollect error: %v", err)
	}

	data := result.(*OtelData)
	if len(data.Metrics) != 1 {
		t.Fatalf("expected 1 metric, got %d", len(data.Metrics))
	}
	if data.Metrics[0].Name != "app.requests" {
		t.Errorf("metric name: got %q, want %q", data.Metrics[0].Name, "app.requests")
	}
}

func TestTracesCollectorImplementsLateCollector(t *testing.T) {
	capturer := NewSpanCapturer()
	c := NewTracesCollector(capturer)
	var _ collector.LateCollector = c
}

func TestSpanCapturerWithInMemoryExporter(t *testing.T) {
	// Verify compatibility with tracetest exporter
	exporter := tracetest.NewInMemoryExporter()
	capturer := NewSpanCapturer()

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(capturer),
		sdktrace.WithBatcher(exporter),
	)
	defer tp.Shutdown(context.Background())

	tracer := tp.Tracer("test")
	_, span := tracer.Start(context.Background(), "test")
	span.End()

	// Our capturer should have the span (synchronous)
	spans := capturer.CapturedSpans()
	if len(spans) != 1 {
		t.Errorf("expected 1 span from capturer, got %d", len(spans))
	}
}

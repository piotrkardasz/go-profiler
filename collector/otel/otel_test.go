package otel

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
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

func TestCollectorImplementsContextSetup(t *testing.T) {
	c := NewCollector(NewSpanCapturer(), nil)
	var _ collector.ContextSetup = c // compile-time check
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


func TestSpanCapturerPerTraceIsolation(t *testing.T) {
	capturer := NewSpanCapturer()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(capturer),
	)
	defer tp.Shutdown(context.Background())

	tracer := tp.Tracer("test")

	// Simulate two concurrent requests with different traces
	ctx1, span1 := tracer.Start(context.Background(), "request-A")
	ctx2, span2 := tracer.Start(context.Background(), "request-B")

	// Each request does work under its own trace
	_, childA := tracer.Start(ctx1, "work-A")
	_, childB := tracer.Start(ctx2, "work-B")
	childA.End()
	childB.End()
	span1.End()
	span2.End()

	traceIDA := span1.SpanContext().TraceID()
	traceIDB := span2.SpanContext().TraceID()

	// Drain only request A's spans
	spansA := capturer.CapturedSpansForTrace(traceIDA)
	if len(spansA) != 2 {
		t.Fatalf("expected 2 spans for trace A, got %d", len(spansA))
	}
	for _, s := range spansA {
		if s.SpanContext().TraceID() != traceIDA {
			t.Errorf("span %q has trace ID %s, want %s",
				s.Name(), s.SpanContext().TraceID(), traceIDA)
		}
	}

	// Request B's spans should still be available
	spansB := capturer.CapturedSpansForTrace(traceIDB)
	if len(spansB) != 2 {
		t.Fatalf("expected 2 spans for trace B, got %d", len(spansB))
	}
	for _, s := range spansB {
		if s.SpanContext().TraceID() != traceIDB {
			t.Errorf("span %q has trace ID %s, want %s",
				s.Name(), s.SpanContext().TraceID(), traceIDB)
		}
	}

	// No spans left
	all := capturer.CapturedSpans()
	if len(all) != 0 {
		t.Errorf("expected 0 remaining spans, got %d", len(all))
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

	// Start root span (simulates middleware creating the request span)
	ctx, rootSpan := tracer.Start(context.Background(), "request")

	// SetupContext captures the trace ID (called before handler runs)
	ctx = c.SetupContext(ctx)

	// Handler does work under the same trace
	_, workSpan := tracer.Start(ctx, "handler-work")
	workSpan.SetAttributes(attribute.String("db.system", "postgresql"))
	time.Sleep(time.Millisecond) // ensure non-zero duration
	workSpan.End()
	rootSpan.End()

	// Late collect should pick up only this trace's spans
	result, err := c.LateCollect(ctx)
	if err != nil {
		t.Fatalf("LateCollect error: %v", err)
	}

	data, ok := result.(*OtelData)
	if !ok {
		t.Fatalf("expected *OtelData, got %T", result)
	}

	if len(data.Spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(data.Spans))
	}

	// Find the handler-work span
	var workInfo *SpanInfo
	for i := range data.Spans {
		if data.Spans[i].Name == "handler-work" {
			workInfo = &data.Spans[i]
			break
		}
	}
	if workInfo == nil {
		t.Fatal("handler-work span not found")
	}
	if workInfo.DurationMs <= 0 {
		t.Errorf("span duration should be > 0, got %f", workInfo.DurationMs)
	}
	if workInfo.Attributes["db.system"] != "postgresql" {
		t.Errorf("span attribute db.system: got %q, want %q",
			workInfo.Attributes["db.system"], "postgresql")
	}
	if workInfo.TraceID == "" {
		t.Error("span TraceID is empty")
	}
	if workInfo.SpanID == "" {
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


func TestCollectorPerRequestIsolation(t *testing.T) {
	capturer := NewSpanCapturer()
	metricCapturer := NewMetricCapturer(nil)
	c := NewCollector(capturer, metricCapturer)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(capturer),
	)
	defer tp.Shutdown(context.Background())

	tracer := tp.Tracer("test")

	// Simulate two overlapping requests
	ctxA, spanA := tracer.Start(context.Background(), "request-A")
	ctxB, spanB := tracer.Start(context.Background(), "request-B")

	// Both requests set up their context (as middleware would)
	ctxA = c.SetupContext(ctxA)
	ctxB = c.SetupContext(ctxB)

	// Request A does work
	_, workA := tracer.Start(ctxA, "db-query-A")
	workA.End()

	// Request B does work
	_, workB := tracer.Start(ctxB, "db-query-B")
	workB.End()

	// End root spans
	spanA.End()
	spanB.End()

	// Export some metrics between the two requests
	now := time.Now()
	rm := &metricdata.ResourceMetrics{
		ScopeMetrics: []metricdata.ScopeMetrics{{
			Metrics: []metricdata.Metrics{{
				Name: "http.duration",
				Data: metricdata.Gauge[float64]{
					DataPoints: []metricdata.DataPoint[float64]{
						{Value: 42.0, Time: now, Attributes: attribute.NewSet()},
					},
				},
			}},
		}},
	}
	metricCapturer.Export(context.Background(), rm)


	// LateCollect for request A — should only get A's spans
	resultA, err := c.LateCollect(ctxA)
	if err != nil {
		t.Fatalf("LateCollect A error: %v", err)
	}
	dataA := resultA.(*OtelData)

	// Should have request-A + db-query-A
	if len(dataA.Spans) != 2 {
		t.Fatalf("request A: expected 2 spans, got %d", len(dataA.Spans))
	}
	for _, s := range dataA.Spans {
		if s.TraceID != spanA.SpanContext().TraceID().String() {
			t.Errorf("request A span %q has wrong trace ID: %s",
				s.Name, s.TraceID)
		}
	}

	// LateCollect for request B — should only get B's spans
	resultB, err := c.LateCollect(ctxB)
	if err != nil {
		t.Fatalf("LateCollect B error: %v", err)
	}
	dataB := resultB.(*OtelData)

	if len(dataB.Spans) != 2 {
		t.Fatalf("request B: expected 2 spans, got %d", len(dataB.Spans))
	}
	for _, s := range dataB.Spans {
		if s.TraceID != spanB.SpanContext().TraceID().String() {
			t.Errorf("request B span %q has wrong trace ID: %s",
				s.Name, s.TraceID)
		}
	}

	// Verify no cross-contamination — A didn't get B's spans
	spanNamesA := make(map[string]bool)
	for _, s := range dataA.Spans {
		spanNamesA[s.Name] = true
	}
	if spanNamesA["db-query-B"] || spanNamesA["request-B"] {
		t.Error("request A contains request B's spans!")
	}

	spanNamesB := make(map[string]bool)
	for _, s := range dataB.Spans {
		spanNamesB[s.Name] = true
	}
	if spanNamesB["db-query-A"] || spanNamesB["request-A"] {
		t.Error("request B contains request A's spans!")
	}
}


func TestCollectorConcurrentRequestIsolation(t *testing.T) {
	capturer := NewSpanCapturer()
	metricCapturer := NewMetricCapturer(nil)
	c := NewCollector(capturer, metricCapturer)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(capturer),
	)
	defer tp.Shutdown(context.Background())

	tracer := tp.Tracer("test")
	const numRequests = 20

	type result struct {
		traceID string
		spans   []SpanInfo
	}

	results := make([]result, numRequests)
	var wg sync.WaitGroup

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			// Start a root span (simulating middleware)
			ctx, rootSpan := tracer.Start(context.Background(), "request")
			traceID := rootSpan.SpanContext().TraceID().String()

			// SetupContext as middleware would
			ctx = c.SetupContext(ctx)

			// Simulate handler work with child spans
			_, child := tracer.Start(ctx, "handler-work")
			child.End()
			rootSpan.End()

			// LateCollect
			raw, err := c.LateCollect(ctx)
			if err != nil {
				t.Errorf("request %d LateCollect error: %v", idx, err)
				return
			}
			data := raw.(*OtelData)
			results[idx] = result{traceID: traceID, spans: data.Spans}
		}(i)
	}

	wg.Wait()


	// Verify each request got exactly its own spans
	for i, r := range results {
		if len(r.spans) != 2 {
			t.Errorf("request %d: expected 2 spans, got %d", i, len(r.spans))
			continue
		}
		for _, s := range r.spans {
			if s.TraceID != r.traceID {
				t.Errorf("request %d: span %q has trace ID %s, want %s",
					i, s.Name, s.TraceID, r.traceID)
			}
		}
	}

	// Nothing should remain in the capturer
	remaining := capturer.CapturedSpans()
	if len(remaining) != 0 {
		t.Errorf("expected 0 remaining spans, got %d", len(remaining))
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


func TestMetricCapturerPerRequestWindow(t *testing.T) {
	capturer := NewMetricCapturer(nil)

	now := time.Now()
	makeRM := func(name string, value float64) *metricdata.ResourceMetrics {
		return &metricdata.ResourceMetrics{
			ScopeMetrics: []metricdata.ScopeMetrics{{
				Metrics: []metricdata.Metrics{{
					Name: name,
					Data: metricdata.Gauge[float64]{
						DataPoints: []metricdata.DataPoint[float64]{
							{Value: value, Time: now, Attributes: attribute.NewSet()},
						},
					},
				}},
			}},
		}
	}

	// Metrics arrive before any request starts
	capturer.Export(context.Background(), makeRM("pre-request", 1.0))

	// Request A starts
	capturer.StartRequestMetrics("req-A")

	// Metrics arrive during A
	capturer.Export(context.Background(), makeRM("during-A", 2.0))

	// Request B starts (overlapping)
	capturer.StartRequestMetrics("req-B")

	// Metrics arrive during both A and B
	capturer.Export(context.Background(), makeRM("during-both", 3.0))

	// End request A — should get metrics from its start onwards
	metricsA := capturer.EndRequestMetrics("req-A")
	if len(metricsA) != 2 {
		t.Fatalf("request A: expected 2 metrics, got %d", len(metricsA))
	}
	if metricsA[0].Name != "during-A" {
		t.Errorf("request A metric[0]: got %q, want %q", metricsA[0].Name, "during-A")
	}
	if metricsA[1].Name != "during-both" {
		t.Errorf("request A metric[1]: got %q, want %q", metricsA[1].Name, "during-both")
	}

	// More metrics arrive after A ends but B is still active
	capturer.Export(context.Background(), makeRM("after-A", 4.0))

	// End request B — should get metrics from its start onwards
	metricsB := capturer.EndRequestMetrics("req-B")
	if len(metricsB) != 2 {
		t.Fatalf("request B: expected 2 metrics, got %d", len(metricsB))
	}
	if metricsB[0].Name != "during-both" {
		t.Errorf("request B metric[0]: got %q, want %q", metricsB[0].Name, "during-both")
	}
	if metricsB[1].Name != "after-A" {
		t.Errorf("request B metric[1]: got %q, want %q", metricsB[1].Name, "after-A")
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

	// Without SetupContext, falls back to global drain
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


func TestCollectorSetupContextStoresTraceID(t *testing.T) {
	capturer := NewSpanCapturer()
	c := NewCollector(capturer, nil)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(capturer),
	)
	defer tp.Shutdown(context.Background())

	tracer := tp.Tracer("test")
	ctx, span := tracer.Start(context.Background(), "request")
	defer span.End()

	// SetupContext should store the trace ID
	ctx = c.SetupContext(ctx)
	rc, ok := otelRequestContextFromContext(ctx)
	if !ok {
		t.Fatal("expected otelRequestContext in context")
	}
	if rc.traceID != span.SpanContext().TraceID() {
		t.Errorf("trace ID mismatch: got %s, want %s",
			rc.traceID, span.SpanContext().TraceID())
	}
}

func TestCollectorFallbackWithoutSetupContext(t *testing.T) {
	capturer := NewSpanCapturer()
	c := NewCollector(capturer, nil)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(capturer),
	)
	defer tp.Shutdown(context.Background())

	tracer := tp.Tracer("test")
	_, span := tracer.Start(context.Background(), "work")
	span.End()

	// LateCollect without SetupContext should fall back to global drain
	result, err := c.LateCollect(context.Background())
	if err != nil {
		t.Fatalf("LateCollect error: %v", err)
	}

	data := result.(*OtelData)
	if len(data.Spans) != 1 {
		t.Fatalf("expected 1 span (fallback), got %d", len(data.Spans))
	}
	if data.Spans[0].Name != "work" {
		t.Errorf("span name: got %q, want %q", data.Spans[0].Name, "work")
	}
}

package otel

import (
	"context"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// MetricInfo holds profiler-friendly information about a captured metric data point.
type MetricInfo struct {
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Unit        string            `json:"unit,omitempty"`
	Type        string            `json:"type"`
	Value       float64           `json:"value"`
	Attributes  map[string]string `json:"attributes,omitempty"`
	Timestamp   time.Time         `json:"timestamp"`
}

// MetricCapturer implements sdkmetric.Exporter to intercept metric exports.
// It captures metric data for the profiler before optionally forwarding to
// a downstream exporter.
//
// Metrics are process-global by nature (they aggregate across all requests in
// each export cycle). The capturer supports two modes:
//   - Per-request snapshot: each request gets the metrics exported between its
//     start and completion (registered via StartRequestMetrics/EndRequestMetrics).
//   - Global drain (backward compat): CapturedMetrics() returns everything.
type MetricCapturer struct {
	mu         sync.Mutex
	metrics    []MetricInfo
	downstream sdkmetric.Exporter

	// Per-request metric tracking.
	// requestMetrics stores a snapshot index for each active request.
	// When a request starts, we record the current length; when it ends,
	// we copy metrics that arrived between start and end.
	requestMu      sync.Mutex
	requestMetrics map[string]*requestMetricState
}

// requestMetricState tracks metrics for a single request.
type requestMetricState struct {
	// startIdx is the index into the capturer's metrics slice at request start.
	startIdx int
	// captured holds the metrics snapshot for this request after end.
	captured []MetricInfo
	done     bool
}

// NewMetricCapturer creates a metric capturer. The optional downstream exporter
// receives metrics after capture (pass nil to only capture without forwarding).
func NewMetricCapturer(downstream sdkmetric.Exporter) *MetricCapturer {
	return &MetricCapturer{
		metrics:        make([]MetricInfo, 0),
		downstream:     downstream,
		requestMetrics: make(map[string]*requestMetricState),
	}
}

// Export captures metrics and optionally forwards them downstream.
func (mc *MetricCapturer) Export(ctx context.Context, rm *metricdata.ResourceMetrics) error {
	mc.mu.Lock()
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			infos := extractMetricInfos(m)
			mc.metrics = append(mc.metrics, infos...)
		}
	}
	mc.mu.Unlock()

	if mc.downstream != nil {
		return mc.downstream.Export(ctx, rm)
	}
	return nil
}

// StartRequestMetrics registers a request (by ID) to track metrics that arrive
// during its lifetime. Call this at request start.
func (mc *MetricCapturer) StartRequestMetrics(requestID string) {
	mc.mu.Lock()
	startIdx := len(mc.metrics)
	mc.mu.Unlock()

	mc.requestMu.Lock()
	mc.requestMetrics[requestID] = &requestMetricState{startIdx: startIdx}
	mc.requestMu.Unlock()
}

// EndRequestMetrics captures metrics that arrived since StartRequestMetrics
// was called for this request ID, and returns them. The request state is
// cleaned up. If no metrics arrived, returns an empty slice.
func (mc *MetricCapturer) EndRequestMetrics(requestID string) []MetricInfo {
	mc.requestMu.Lock()
	state, ok := mc.requestMetrics[requestID]
	if !ok {
		mc.requestMu.Unlock()
		return nil
	}
	delete(mc.requestMetrics, requestID)
	mc.requestMu.Unlock()

	mc.mu.Lock()
	defer mc.mu.Unlock()

	if state.startIdx >= len(mc.metrics) {
		return []MetricInfo{}
	}

	// Copy the slice of metrics that arrived during this request's lifetime.
	window := mc.metrics[state.startIdx:]
	result := make([]MetricInfo, len(window))
	copy(result, window)
	return result
}

// Temporality returns the temporality for the given instrument kind.
func (mc *MetricCapturer) Temporality(kind sdkmetric.InstrumentKind) metricdata.Temporality {
	if mc.downstream != nil {
		return mc.downstream.Temporality(kind)
	}
	return metricdata.CumulativeTemporality
}

// Aggregation returns the aggregation for the given instrument kind.
func (mc *MetricCapturer) Aggregation(kind sdkmetric.InstrumentKind) sdkmetric.Aggregation {
	if mc.downstream != nil {
		return mc.downstream.Aggregation(kind)
	}
	return sdkmetric.DefaultAggregationSelector(kind)
}

// ForceFlush flushes the exporter.
func (mc *MetricCapturer) ForceFlush(ctx context.Context) error {
	if mc.downstream != nil {
		return mc.downstream.ForceFlush(ctx)
	}
	return nil
}

// Shutdown shuts down the exporter.
func (mc *MetricCapturer) Shutdown(ctx context.Context) error {
	if mc.downstream != nil {
		return mc.downstream.Shutdown(ctx)
	}
	return nil
}

// CapturedMetrics returns the captured metrics and resets the internal buffer.
// This is the backward-compatible global drain. Prefer EndRequestMetrics for
// per-request isolation.
func (mc *MetricCapturer) CapturedMetrics() []MetricInfo {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	metrics := mc.metrics
	mc.metrics = make([]MetricInfo, 0)
	return metrics
}

// extractMetricInfos converts OTel metric data into our MetricInfo format.
func extractMetricInfos(m metricdata.Metrics) []MetricInfo {
	var infos []MetricInfo

	switch data := m.Data.(type) {
	case metricdata.Gauge[float64]:
		for _, dp := range data.DataPoints {
			infos = append(infos, MetricInfo{
				Name:        m.Name,
				Description: m.Description,
				Unit:        m.Unit,
				Type:        "gauge",
				Value:       dp.Value,
				Attributes:  attrSetToMap(dp.Attributes),
				Timestamp:   dp.Time,
			})
		}
	case metricdata.Gauge[int64]:
		for _, dp := range data.DataPoints {
			infos = append(infos, MetricInfo{
				Name:        m.Name,
				Description: m.Description,
				Unit:        m.Unit,
				Type:        "gauge",
				Value:       float64(dp.Value),
				Attributes:  attrSetToMap(dp.Attributes),
				Timestamp:   dp.Time,
			})
		}
	case metricdata.Sum[float64]:
		for _, dp := range data.DataPoints {
			infos = append(infos, MetricInfo{
				Name:        m.Name,
				Description: m.Description,
				Unit:        m.Unit,
				Type:        "sum",
				Value:       dp.Value,
				Attributes:  attrSetToMap(dp.Attributes),
				Timestamp:   dp.Time,
			})
		}
	case metricdata.Sum[int64]:
		for _, dp := range data.DataPoints {
			infos = append(infos, MetricInfo{
				Name:        m.Name,
				Description: m.Description,
				Unit:        m.Unit,
				Type:        "sum",
				Value:       float64(dp.Value),
				Attributes:  attrSetToMap(dp.Attributes),
				Timestamp:   dp.Time,
			})
		}
	case metricdata.Histogram[float64]:
		for _, dp := range data.DataPoints {
			var value float64
			if dp.Count > 0 {
				value = dp.Sum / float64(dp.Count)
			}
			infos = append(infos, MetricInfo{
				Name:        m.Name,
				Description: m.Description,
				Unit:        m.Unit,
				Type:        "histogram",
				Value:       value,
				Attributes:  attrSetToMap(dp.Attributes),
				Timestamp:   dp.Time,
			})
		}
	case metricdata.Histogram[int64]:
		for _, dp := range data.DataPoints {
			var value float64
			if dp.Count > 0 {
				value = float64(dp.Sum) / float64(dp.Count)
			}
			infos = append(infos, MetricInfo{
				Name:        m.Name,
				Description: m.Description,
				Unit:        m.Unit,
				Type:        "histogram",
				Value:       value,
				Attributes:  attrSetToMap(dp.Attributes),
				Timestamp:   dp.Time,
			})
		}
	}

	return infos
}

// attrSetToMap converts an OTel attribute.Set to a string map.
func attrSetToMap(attrs attribute.Set) map[string]string {
	if attrs.Len() == 0 {
		return nil
	}
	result := make(map[string]string, attrs.Len())
	iter := attrs.Iter()
	for iter.Next() {
		kv := iter.Attribute()
		result[string(kv.Key)] = kv.Value.Emit()
	}
	return result
}

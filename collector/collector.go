// Package collector defines the interfaces and types for profiler data collectors.
// Collectors gather runtime information about HTTP requests and make it available
// for inspection through the profiler UI and API.
package collector

import (
	"context"
	"net/http"
)

// ResponseData holds captured response information that collectors can inspect.
type ResponseData struct {
	// StatusCode is the HTTP response status code.
	StatusCode int

	// Headers contains the response headers.
	Headers http.Header

	// Size is the response body size in bytes.
	Size int64
}

// Collector gathers profiling data for a single HTTP request.
// Implementations should be safe for concurrent use.
type Collector interface {
	// Name returns a unique identifier for this collector.
	// This is used as the key in Profile.CollectorData and to match UI panels.
	Name() string

	// Collect gathers data about the request/response cycle.
	// It is called after the handler has completed and the response has been written.
	// The returned value must be JSON-serializable.
	Collect(ctx context.Context, req *http.Request, res ResponseData) (any, error)

	// Reset clears any internal state between requests.
	// This is called at the beginning of each new request cycle.
	Reset()
}

// LateCollector extends Collector with the ability to collect data after the
// response has been fully sent to the client. This is useful for collectors
// that need to wait for asynchronous operations to complete (e.g., OpenTelemetry
// span export).
type LateCollector interface {
	Collector

	// LateCollect gathers additional data after the response is sent.
	// The returned value is merged with or replaces the data from Collect().
	// It must be JSON-serializable.
	LateCollect(ctx context.Context) (any, error)
}

// PanelMeta provides metadata for the UI panel associated with a collector.
// Collectors can optionally implement the PanelProvider interface to supply
// custom panel metadata.
type PanelMeta struct {
	// Name is the collector name (matches Collector.Name()).
	Name string `json:"name"`

	// Label is the human-readable display name for the panel tab.
	Label string `json:"label"`

	// Icon is an icon identifier (e.g., from Tabler Icons) for the panel.
	Icon string `json:"icon"`

	// Component is the Vue component name for custom panel rendering.
	// If empty, the generic JSON tree panel is used.
	Component string `json:"component,omitempty"`
}

// PanelProvider is an optional interface that collectors can implement to
// provide custom UI panel metadata.
type PanelProvider interface {
	// PanelMeta returns metadata describing how this collector's data
	// should be displayed in the profiler UI.
	PanelMeta() PanelMeta
}

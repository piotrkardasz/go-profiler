package http

import (
	"context"
	"net/http"

	"github.com/piotrkardasz/go-profiler/collector"
)

// Collector captures outbound HTTP calls made during a profiled request.
// It implements collector.Collector, collector.ContextSetup, and collector.PanelProvider.
type Collector struct {
	opts *options
}

// New creates a new HTTP client collector with the given options.
func New(opts ...Option) *Collector {
	return &Collector{
		opts: applyOptions(opts),
	}
}

// Name returns the collector identifier used as the key in Profile.CollectorData.
func (c *Collector) Name() string {
	return "http"
}

// Reset is a no-op because all state is stored per-request in context.
func (c *Collector) Reset() {}

// SetupContext initializes per-request HTTP call tracking in the context.
// Called by the profiler middleware before the handler runs.
func (c *Collector) SetupContext(ctx context.Context) context.Context {
	return WithContext(ctx)
}

// Collect gathers all captured HTTP calls from the context, runs analysis,
// and returns the structured data for storage in the profile.
func (c *Collector) Collect(ctx context.Context, req *http.Request, res collector.ResponseData) (any, error) {
	calls := CallsFromContext(ctx)

	if len(calls) == 0 {
		return HTTPData{
			Calls:   []HTTPCallEntry{},
			Summary: Summary{CallsPerService: make(map[string]int)},
		}, nil
	}

	analysis := analyze(calls, c.opts)
	summary := buildSummary(calls, analysis)

	return HTTPData{
		Calls:    calls,
		Analysis: analysis,
		Summary:  summary,
	}, nil
}

// PanelMeta returns UI panel metadata for the HTTP clients panel.
func (c *Collector) PanelMeta() collector.PanelMeta {
	return collector.PanelMeta{
		Name:      "http",
		Label:     "HTTP Clients",
		Icon:      "world",
		Component: "HttpPanel",
	}
}

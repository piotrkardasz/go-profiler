package collector

import (
	"context"
	"net/http"
)

// RequestData holds the collected request/response information.
type RequestData struct {
	// Request information
	Method      string              `json:"method"`
	URL         string              `json:"url"`
	Host        string              `json:"host"`
	RemoteAddr  string              `json:"remote_addr"`
	Proto       string              `json:"proto"`
	Headers     map[string][]string `json:"headers"`
	QueryParams map[string][]string `json:"query_params"`
	ContentType string              `json:"content_type"`

	// Response information
	StatusCode      int                 `json:"status_code"`
	ResponseHeaders map[string][]string `json:"response_headers"`
	ResponseSize    int64               `json:"response_size"`
}

// RequestCollector captures HTTP request and response details.
type RequestCollector struct{}

// NewRequestCollector creates a new RequestCollector.
func NewRequestCollector() *RequestCollector {
	return &RequestCollector{}
}

// Name returns the collector identifier.
func (c *RequestCollector) Name() string {
	return "request"
}

// Collect gathers request and response data.
func (c *RequestCollector) Collect(_ context.Context, req *http.Request, res ResponseData) (any, error) {
	data := &RequestData{
		Method:     req.Method,
		URL:        req.URL.String(),
		Host:       req.Host,
		RemoteAddr: req.RemoteAddr,
		Proto:      req.Proto,
		Headers:    sanitizeHeaders(req.Header),
		ContentType: req.Header.Get("Content-Type"),

		StatusCode:      res.StatusCode,
		ResponseHeaders: sanitizeHeaders(res.Headers),
		ResponseSize:    res.Size,
	}

	if req.URL.RawQuery != "" {
		data.QueryParams = make(map[string][]string)
		for k, v := range req.URL.Query() {
			data.QueryParams[k] = v
		}
	}

	return data, nil
}

// Reset clears internal state (no-op for this collector).
func (c *RequestCollector) Reset() {}

// PanelMeta returns UI panel metadata for this collector.
func (c *RequestCollector) PanelMeta() PanelMeta {
	return PanelMeta{
		Name:      "request",
		Label:     "Request / Response",
		Icon:      "world",
		Component: "RequestPanel",
	}
}

// sanitizeHeaders converts http.Header to a plain map, filtering out
// sensitive headers like Authorization and Cookie values.
func sanitizeHeaders(h http.Header) map[string][]string {
	if h == nil {
		return nil
	}

	sensitiveHeaders := map[string]bool{
		"Authorization": true,
		"Cookie":        true,
		"Set-Cookie":    true,
	}

	result := make(map[string][]string, len(h))
	for k, v := range h {
		if sensitiveHeaders[k] {
			result[k] = []string{"[REDACTED]"}
		} else {
			result[k] = v
		}
	}
	return result
}

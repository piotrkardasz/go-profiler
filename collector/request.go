package collector

import (
	"context"
	"fmt"
	"net/http"
	"strings"
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
	QueryParams map[string][]string `json:"query_params,omitempty"`
	ContentType string              `json:"content_type"`

	// Response information
	StatusCode      int                 `json:"status_code"`
	ResponseHeaders map[string][]string `json:"response_headers"`
	ResponseSize    int64               `json:"response_size"`

	// Body capture (opt-in via PROFILER_CAPTURE_BODY)
	Body          string `json:"body,omitempty"`
	BodySize      int64  `json:"body_size,omitempty"`
	BodyTruncated bool   `json:"body_truncated,omitempty"`

	// Curl command (always generated)
	CurlCommand string `json:"curl_command,omitempty"`
}

// RequestCollector captures HTTP request and response details.
type RequestCollector struct {
	options *requestOptions
}

// NewRequestCollector creates a new RequestCollector with the given options.
// If no options are provided, defaults are used (body capture off, headers redacted).
func NewRequestCollector(opts ...RequestOption) *RequestCollector {
	options := defaultRequestOptions()
	for _, opt := range opts {
		opt(options)
	}
	return &RequestCollector{options: options}
}

// Name returns the collector identifier.
func (c *RequestCollector) Name() string {
	return "request"
}

// Collect gathers request and response data.
func (c *RequestCollector) Collect(ctx context.Context, req *http.Request, res ResponseData) (any, error) {
	data := &RequestData{
		Method:      req.Method,
		URL:         req.URL.String(),
		Host:        req.Host,
		RemoteAddr:  req.RemoteAddr,
		Proto:       req.Proto,
		Headers:     c.sanitizeHeaders(req.Header),
		ContentType: req.Header.Get("Content-Type"),

		StatusCode:      res.StatusCode,
		ResponseHeaders: c.sanitizeHeaders(res.Headers),
		ResponseSize:    res.Size,
	}

	if req.URL.RawQuery != "" {
		data.QueryParams = make(map[string][]string)
		for k, v := range req.URL.Query() {
			data.QueryParams[k] = v
		}
	}

	// Retrieve captured body from context (if body capture was enabled)
	body := bodyFromContext(ctx)
	if body != nil {
		data.Body = body.content
		data.BodySize = body.size
		data.BodyTruncated = body.truncated
	}

	// Generate curl command
	data.CurlCommand = c.buildCurl(req, data, body)

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

// buildCurl constructs a curl command from the request data.
func (c *RequestCollector) buildCurl(req *http.Request, data *RequestData, body *capturedBody) string {
	fullURL := buildFullURL(req)
	if fullURL == "" {
		return ""
	}

	input := &CurlInput{
		Method:  data.Method,
		URL:     fullURL,
		Headers: data.Headers,
	}

	if body != nil {
		if body.binary {
			input.IsBinary = true
			input.BinarySize = body.size
		} else if body.content != "" {
			input.HasBody = true
			input.Body = body.content
		}
	}

	return BuildCurlCommand(input)
}

// buildFullURL reconstructs the full URL including scheme, host, path, and query.
func buildFullURL(req *http.Request) string {
	if req == nil || req.URL == nil {
		return ""
	}

	scheme := "http"
	if req.TLS != nil {
		scheme = "https"
	} else if proto := req.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = strings.ToLower(proto)
	}

	host := req.Host
	if host == "" {
		host = req.URL.Host
	}
	if host == "" {
		return ""
	}

	path := req.URL.RequestURI()
	return fmt.Sprintf("%s://%s%s", scheme, host, path)
}

// sanitizeHeaders converts http.Header to a plain map, optionally filtering
// sensitive headers like Authorization and Cookie values.
func (c *RequestCollector) sanitizeHeaders(h http.Header) map[string][]string {
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
		if c.options.redactHeaders && sensitiveHeaders[k] {
			result[k] = []string{"[REDACTED]"}
		} else {
			result[k] = v
		}
	}
	return result
}

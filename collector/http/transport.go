package http

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"time"
)

// profilingTransport is an http.RoundTripper that captures outbound HTTP calls
// for the profiler. When no profiling context is active, it passes through
// with negligible overhead (single context value lookup).
type profilingTransport struct {
	serviceName string
	base        http.RoundTripper
	opts        *options
}

// NewTransport creates a profiling round tripper wrapping the given base transport.
// serviceName identifies the downstream service for grouping in the profiler UI.
// If base is nil, http.DefaultTransport is used.
func NewTransport(serviceName string, base http.RoundTripper, opts ...Option) http.RoundTripper {
	if serviceName == "" {
		panic("httpcollector: serviceName must not be empty")
	}
	if base == nil {
		base = http.DefaultTransport
	}
	return &profilingTransport{
		serviceName: serviceName,
		base:        base,
		opts:        applyOptions(opts),
	}
}

// RoundTrip implements http.RoundTripper. If the request context has no
// profiling tracker, it delegates directly to the base transport with zero overhead.
func (t *profilingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Fast path: no profiling active
	rc := callsFromContext(req.Context())
	if rc == nil {
		return t.base.RoundTrip(req)
	}

	entry := HTTPCallEntry{
		Service:   t.serviceName,
		Method:    req.Method,
		URL:       req.URL.String(),
		Timestamp: time.Now(),
	}

	// Capture request headers
	if t.opts.headerCapture && req.Header != nil {
		entry.RequestHeaders = t.captureHeaders(req.Header)
	}

	// Capture request body
	if t.opts.bodyCapture && req.Body != nil && req.Body != http.NoBody {
		body, size, newBody := t.captureRequestBody(req.Body)
		entry.RequestBody = body
		entry.RequestSize = size
		req.Body = newBody
	} else if req.ContentLength > 0 {
		entry.RequestSize = req.ContentLength
	}

	// Execute the actual request
	start := time.Now()
	resp, err := t.base.RoundTrip(req)
	entry.DurationMs = float64(time.Since(start).Microseconds()) / 1000.0

	// Capture transport error
	if err != nil {
		entry.Error = err.Error()
	}

	// Capture response details
	if resp != nil {
		entry.StatusCode = resp.StatusCode

		if t.opts.headerCapture && resp.Header != nil {
			entry.ResponseHeaders = t.captureHeaders(resp.Header)
		}

		if t.opts.bodyCapture && resp.Body != nil {
			body, size, newBody := t.captureResponseBody(resp.Body, resp.ContentLength)
			entry.ResponseBody = body
			entry.ResponseSize = size
			resp.Body = newBody
		} else if resp.ContentLength >= 0 {
			entry.ResponseSize = resp.ContentLength
		}
	}

	// Generate cURL command
	if t.opts.curlGeneration {
		entry.CurlCommand = buildCurlCommand(&entry, t.opts)
	}

	// Capture backtrace
	if t.opts.backtraceEnabled {
		entry.Backtrace = captureBacktrace()
	}

	// Append to per-request tracker
	appendCall(req.Context(), entry)

	return resp, err
}

// captureHeaders copies headers with redaction applied.
func (t *profilingTransport) captureHeaders(h http.Header) map[string][]string {
	captured := make(map[string][]string, len(h))
	for k, v := range h {
		if t.opts.redactHeaders[strings.ToLower(k)] {
			captured[k] = []string{"[REDACTED]"}
		} else {
			vals := make([]string, len(v))
			copy(vals, v)
			captured[k] = vals
		}
	}
	return captured
}

// captureRequestBody reads the request body up to maxBodySize, then returns
// the captured content, size, and a replacement body that still contains all data.
func (t *profilingTransport) captureRequestBody(body io.ReadCloser) (string, int64, io.ReadCloser) {
	buf := make([]byte, t.opts.maxBodySize+1)
	n, err := io.ReadFull(body, buf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		// Read failed; return body as-is with whatever we got
		if n > 0 {
			combined := io.NopCloser(io.MultiReader(bytes.NewReader(buf[:n]), body))
			return "", int64(n), combined
		}
		return "", 0, body
	}

	truncated := n > t.opts.maxBodySize
	captureSize := n
	if truncated {
		captureSize = t.opts.maxBodySize
	}

	captured := string(buf[:captureSize])
	if truncated {
		captured += "[truncated]"
	}

	// Reconstruct body: buffered bytes + remaining original stream
	var newBody io.ReadCloser
	if truncated {
		// There may be more data in the original body
		newBody = &compositeReadCloser{
			Reader: io.MultiReader(bytes.NewReader(buf[:n]), body),
			Closer: body,
		}
	} else {
		// We read everything
		body.Close()
		newBody = io.NopCloser(bytes.NewReader(buf[:n]))
	}

	return captured, int64(n), newBody
}

// captureResponseBody captures the response body while preserving it for the caller.
func (t *profilingTransport) captureResponseBody(body io.ReadCloser, contentLength int64) (string, int64, io.ReadCloser) {
	buf := make([]byte, t.opts.maxBodySize+1)
	n, err := io.ReadFull(body, buf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		if n > 0 {
			combined := &compositeReadCloser{
				Reader: io.MultiReader(bytes.NewReader(buf[:n]), body),
				Closer: body,
			}
			return "", int64(n), combined
		}
		return "", 0, body
	}

	truncated := n > t.opts.maxBodySize
	captureSize := n
	if truncated {
		captureSize = t.opts.maxBodySize
	}

	captured := string(buf[:captureSize])
	if truncated {
		captured += "[truncated]"
	}

	// Calculate total size
	var totalSize int64
	if contentLength >= 0 {
		totalSize = contentLength
	} else {
		totalSize = int64(n)
	}

	// Reconstruct body for the caller
	var newBody io.ReadCloser
	if truncated {
		newBody = &compositeReadCloser{
			Reader: io.MultiReader(bytes.NewReader(buf[:n]), body),
			Closer: body,
		}
	} else {
		body.Close()
		newBody = io.NopCloser(bytes.NewReader(buf[:n]))
	}

	return captured, totalSize, newBody
}

// compositeReadCloser combines a Reader with a separate Closer,
// allowing us to reconstruct a body from buffered + remaining data.
type compositeReadCloser struct {
	io.Reader
	Closer io.Closer
}

func (c *compositeReadCloser) Close() error {
	return c.Closer.Close()
}

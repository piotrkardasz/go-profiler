package collector

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// bodyContextKey is the context key for storing captured body data.
type bodyContextKey struct{}

// capturedBody holds the captured request body and its metadata.
type capturedBody struct {
	content   string // Body text or "[binary data: N bytes]"
	size      int64  // Original full body size in bytes
	truncated bool   // Whether content was truncated to max size
	binary    bool   // Whether binary content was detected
}

// textContentTypes lists content types that are captured as text.
// Any content type not matching this list is treated as binary.
var textContentTypes = []string{
	"application/json",
	"application/xml",
	"application/x-www-form-urlencoded",
	"application/graphql",
	"application/javascript",
	"application/yaml",
	"application/toml",
	"text/", // prefix match: text/plain, text/html, text/csv, etc.
}

// CaptureBody reads and buffers the request body, storing it in context.
// It replaces r.Body with a new reader so downstream handlers can still
// consume the body normally. Called by the profiler middleware before the handler.
func (c *RequestCollector) CaptureBody(ctx context.Context, r *http.Request) (context.Context, *http.Request) {
	if !c.options.bodyCaptureEnabled {
		return ctx, r
	}

	// Skip if no body to read
	if r.Body == nil || r.Body == http.NoBody {
		return ctx, r
	}

	// Skip methods that typically have no body
	if r.ContentLength == 0 {
		return ctx, r
	}

	contentType := r.Header.Get("Content-Type")

	// Check user-configured content type whitelist
	if !c.shouldCaptureContentType(contentType) {
		return ctx, r
	}

	// Read the full body
	allBytes, err := io.ReadAll(r.Body)
	if err != nil && len(allBytes) == 0 {
		// Complete read failure — restore empty body and skip
		r.Body = io.NopCloser(bytes.NewReader(nil))
		return ctx, r
	}
	// Close the original body
	r.Body.Close()

	// Replace r.Body so downstream handlers can read the full body
	r.Body = io.NopCloser(bytes.NewReader(allBytes))

	// Build captured body metadata
	captured := &capturedBody{
		size: int64(len(allBytes)),
	}

	// Check for binary content
	if !isTextContentType(contentType) {
		captured.binary = true
		captured.content = fmt.Sprintf("[binary data: %d bytes]", len(allBytes))
	} else if len(allBytes) > c.options.bodyMaxSize {
		// Truncate to max size
		captured.content = string(allBytes[:c.options.bodyMaxSize])
		captured.truncated = true
	} else {
		captured.content = string(allBytes)
	}

	ctx = context.WithValue(ctx, bodyContextKey{}, captured)
	return ctx, r
}

// BodyCaptureEnabled returns whether body capture is enabled.
func (c *RequestCollector) BodyCaptureEnabled() bool {
	return c.options.bodyCaptureEnabled
}

// bodyFromContext retrieves the captured body from context, or nil if not present.
func bodyFromContext(ctx context.Context) *capturedBody {
	v := ctx.Value(bodyContextKey{})
	if v == nil {
		return nil
	}
	return v.(*capturedBody)
}

// isTextContentType checks if the given content type is a text-based type
// that should be captured as-is (not treated as binary).
func isTextContentType(contentType string) bool {
	if contentType == "" {
		// No content type specified — assume text (best effort)
		return true
	}

	// Normalize: lowercase and strip parameters (e.g., "; charset=utf-8")
	ct := strings.ToLower(contentType)
	if idx := strings.Index(ct, ";"); idx != -1 {
		ct = strings.TrimSpace(ct[:idx])
	}

	for _, textType := range textContentTypes {
		if strings.HasSuffix(textType, "/") {
			// Prefix match (e.g., "text/" matches "text/plain")
			if strings.HasPrefix(ct, textType) {
				return true
			}
		} else {
			if ct == textType {
				return true
			}
		}
	}

	return false
}

// shouldCaptureContentType checks if the content type matches the user-configured
// whitelist. If no whitelist is configured, all content types are allowed.
func (c *RequestCollector) shouldCaptureContentType(contentType string) bool {
	if len(c.options.bodyContentTypes) == 0 {
		return true // No whitelist — capture all
	}

	ct := strings.ToLower(contentType)
	if idx := strings.Index(ct, ";"); idx != -1 {
		ct = strings.TrimSpace(ct[:idx])
	}

	for _, allowed := range c.options.bodyContentTypes {
		if strings.ToLower(allowed) == ct {
			return true
		}
	}

	return false
}

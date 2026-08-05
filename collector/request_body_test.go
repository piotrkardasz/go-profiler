package collector

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCaptureBodyDisabled(t *testing.T) {
	c := NewRequestCollector() // body capture disabled by default
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("body content"))
	req.Header.Set("Content-Type", "application/json")

	ctx, newReq := c.CaptureBody(context.Background(), req)

	body := bodyFromContext(ctx)
	if body != nil {
		t.Error("expected no body captured when disabled")
	}

	// Original request should be unchanged
	if newReq != req {
		t.Error("expected same request returned when capture disabled")
	}
}

func TestCaptureBodyNilBody(t *testing.T) {
	c := NewRequestCollector(WithBodyCapture(true))
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Body = nil

	ctx, _ := c.CaptureBody(context.Background(), req)

	body := bodyFromContext(ctx)
	if body != nil {
		t.Error("expected no body captured when body is nil")
	}
}

func TestCaptureBodyZeroContentLength(t *testing.T) {
	c := NewRequestCollector(WithBodyCapture(true))
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(""))
	req.ContentLength = 0

	ctx, _ := c.CaptureBody(context.Background(), req)

	body := bodyFromContext(ctx)
	if body != nil {
		t.Error("expected no body captured when content length is 0")
	}
}

func TestCaptureBodyJSON(t *testing.T) {
	c := NewRequestCollector(WithBodyCapture(true))
	jsonBody := `{"name":"John","email":"john@example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	ctx, newReq := c.CaptureBody(context.Background(), req)

	// Check captured body
	body := bodyFromContext(ctx)
	if body == nil {
		t.Fatal("expected body to be captured")
	}
	if body.content != jsonBody {
		t.Errorf("body content: got %q, want %q", body.content, jsonBody)
	}
	if body.size != int64(len(jsonBody)) {
		t.Errorf("body size: got %d, want %d", body.size, len(jsonBody))
	}
	if body.truncated {
		t.Error("body should not be truncated")
	}
	if body.binary {
		t.Error("body should not be binary")
	}

	// Downstream handler can still read the full body
	downstream, err := io.ReadAll(newReq.Body)
	if err != nil {
		t.Fatalf("reading downstream body: %v", err)
	}
	if string(downstream) != jsonBody {
		t.Errorf("downstream body: got %q, want %q", string(downstream), jsonBody)
	}
}

func TestCaptureBodyFormData(t *testing.T) {
	c := NewRequestCollector(WithBodyCapture(true))
	formBody := "username=john&password=secret"
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(formBody))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	ctx, _ := c.CaptureBody(context.Background(), req)

	body := bodyFromContext(ctx)
	if body == nil {
		t.Fatal("expected body to be captured")
	}
	if body.content != formBody {
		t.Errorf("body content: got %q, want %q", body.content, formBody)
	}
	if body.binary {
		t.Error("form data should not be binary")
	}
}

func TestCaptureBodyTruncation(t *testing.T) {
	maxSize := 100
	c := NewRequestCollector(WithBodyCapture(true), WithBodyMaxSize(maxSize))

	// Create body larger than max size
	largeBody := strings.Repeat("x", 200)
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(largeBody))
	req.Header.Set("Content-Type", "text/plain")

	ctx, newReq := c.CaptureBody(context.Background(), req)

	// Check truncation
	body := bodyFromContext(ctx)
	if body == nil {
		t.Fatal("expected body to be captured")
	}
	if !body.truncated {
		t.Error("body should be truncated")
	}
	if len(body.content) != maxSize {
		t.Errorf("truncated content length: got %d, want %d", len(body.content), maxSize)
	}
	if body.size != int64(len(largeBody)) {
		t.Errorf("original size: got %d, want %d", body.size, len(largeBody))
	}

	// Full body still available downstream
	downstream, _ := io.ReadAll(newReq.Body)
	if string(downstream) != largeBody {
		t.Errorf("downstream body length: got %d, want %d", len(downstream), len(largeBody))
	}
}

func TestCaptureBodyBinaryDetection(t *testing.T) {
	c := NewRequestCollector(WithBodyCapture(true))
	binaryData := bytes.Repeat([]byte{0x00, 0xFF}, 100)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(binaryData))
	req.Header.Set("Content-Type", "application/octet-stream")

	ctx, _ := c.CaptureBody(context.Background(), req)

	body := bodyFromContext(ctx)
	if body == nil {
		t.Fatal("expected body to be captured")
	}
	if !body.binary {
		t.Error("should detect binary content type")
	}
	if !strings.HasPrefix(body.content, "[binary data:") {
		t.Errorf("expected binary placeholder, got %q", body.content)
	}
}

func TestCaptureBodyContentTypeWhitelist(t *testing.T) {
	c := NewRequestCollector(
		WithBodyCapture(true),
		WithBodyContentTypes("application/json"),
	)

	// JSON should be captured
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"ok":true}`))
	req.Header.Set("Content-Type", "application/json")
	ctx, _ := c.CaptureBody(context.Background(), req)

	body := bodyFromContext(ctx)
	if body == nil {
		t.Fatal("expected JSON body to be captured with whitelist")
	}

	// XML should NOT be captured (not in whitelist)
	req2 := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("<root/>"))
	req2.Header.Set("Content-Type", "application/xml")
	ctx2, _ := c.CaptureBody(context.Background(), req2)

	body2 := bodyFromContext(ctx2)
	if body2 != nil {
		t.Error("XML body should not be captured when not in whitelist")
	}
}

func TestCaptureBodyContentTypeWhitelistMiss(t *testing.T) {
	c := NewRequestCollector(
		WithBodyCapture(true),
		WithBodyContentTypes("application/json", "text/plain"),
	)

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("form=data"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	ctx, _ := c.CaptureBody(context.Background(), req)

	body := bodyFromContext(ctx)
	if body != nil {
		t.Error("form data should not be captured when not in whitelist")
	}
}

func TestIsTextContentType(t *testing.T) {
	tests := []struct {
		contentType string
		isText      bool
	}{
		{"application/json", true},
		{"application/json; charset=utf-8", true},
		{"application/xml", true},
		{"application/x-www-form-urlencoded", true},
		{"application/graphql", true},
		{"application/javascript", true},
		{"application/yaml", true},
		{"text/plain", true},
		{"text/html", true},
		{"text/csv", true},
		{"text/xml; charset=utf-8", true},
		{"application/octet-stream", false},
		{"multipart/form-data", false},
		{"image/png", false},
		{"audio/mpeg", false},
		{"video/mp4", false},
		{"application/pdf", false},
		{"application/zip", false},
		{"", true}, // empty content type defaults to text
	}

	for _, tt := range tests {
		result := isTextContentType(tt.contentType)
		if result != tt.isText {
			t.Errorf("isTextContentType(%q): got %v, want %v", tt.contentType, result, tt.isText)
		}
	}
}

func TestBodyFromContextNil(t *testing.T) {
	body := bodyFromContext(context.Background())
	if body != nil {
		t.Error("expected nil when no body in context")
	}
}

func TestCaptureBodyDownstreamReadable(t *testing.T) {
	c := NewRequestCollector(WithBodyCapture(true))
	originalBody := `{"key":"value","nested":{"a":1}}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(originalBody))
	req.Header.Set("Content-Type", "application/json")

	_, newReq := c.CaptureBody(context.Background(), req)

	// Simulate handler reading body
	handlerBody, err := io.ReadAll(newReq.Body)
	if err != nil {
		t.Fatalf("handler read error: %v", err)
	}
	if string(handlerBody) != originalBody {
		t.Errorf("handler got %q, want %q", string(handlerBody), originalBody)
	}
}

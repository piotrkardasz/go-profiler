package collector

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestRequestCollectorName(t *testing.T) {
	c := NewRequestCollector()
	if c.Name() != "request" {
		t.Errorf("Name(): got %q, want %q", c.Name(), "request")
	}
}

func TestRequestCollectorCollect(t *testing.T) {
	c := NewRequestCollector()

	req := httptest.NewRequest(http.MethodPost, "http://localhost:8080/api/users?page=2&limit=10", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Custom", "value")

	res := ResponseData{
		StatusCode: 201,
		Headers:    http.Header{"Content-Type": []string{"application/json"}, "X-Request-Id": []string{"abc123"}},
		Size:       256,
	}

	result, err := c.Collect(context.Background(), req, res)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, ok := result.(*RequestData)
	if !ok {
		t.Fatalf("expected *RequestData, got %T", result)
	}

	if data.Method != "POST" {
		t.Errorf("Method: got %q, want %q", data.Method, "POST")
	}
	if data.Host != "localhost:8080" {
		t.Errorf("Host: got %q, want %q", data.Host, "localhost:8080")
	}
	if data.StatusCode != 201 {
		t.Errorf("StatusCode: got %d, want %d", data.StatusCode, 201)
	}

	if data.ResponseSize != 256 {
		t.Errorf("ResponseSize: got %d, want %d", data.ResponseSize, 256)
	}
	if data.ContentType != "application/json" {
		t.Errorf("ContentType: got %q, want %q", data.ContentType, "application/json")
	}

	// Query params
	if data.QueryParams == nil {
		t.Fatal("QueryParams is nil")
	}
	if page := data.QueryParams["page"]; len(page) == 0 || page[0] != "2" {
		t.Errorf("QueryParams[page]: got %v", page)
	}
	if limit := data.QueryParams["limit"]; len(limit) == 0 || limit[0] != "10" {
		t.Errorf("QueryParams[limit]: got %v", limit)
	}

	// Request headers
	if ct := data.Headers["Content-Type"]; len(ct) == 0 || ct[0] != "application/json" {
		t.Errorf("Headers[Content-Type]: got %v", ct)
	}
	if custom := data.Headers["X-Custom"]; len(custom) == 0 || custom[0] != "value" {
		t.Errorf("Headers[X-Custom]: got %v", custom)
	}

	// Response headers
	if rh := data.ResponseHeaders["X-Request-Id"]; len(rh) == 0 || rh[0] != "abc123" {
		t.Errorf("ResponseHeaders[X-Request-Id]: got %v", rh)
	}
}


func TestRequestCollectorRedactsSensitiveHeaders(t *testing.T) {
	c := NewRequestCollector()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	req.Header.Set("Cookie", "session=abc123")

	res := ResponseData{
		StatusCode: 200,
		Headers:    http.Header{"Set-Cookie": []string{"session=xyz789; Path=/"}},
	}

	result, err := c.Collect(context.Background(), req, res)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := result.(*RequestData)

	if auth := data.Headers["Authorization"]; len(auth) == 0 || auth[0] != "[REDACTED]" {
		t.Errorf("Authorization header not redacted: got %v", auth)
	}
	if cookie := data.Headers["Cookie"]; len(cookie) == 0 || cookie[0] != "[REDACTED]" {
		t.Errorf("Cookie header not redacted: got %v", cookie)
	}
	if sc := data.ResponseHeaders["Set-Cookie"]; len(sc) == 0 || sc[0] != "[REDACTED]" {
		t.Errorf("Set-Cookie response header not redacted: got %v", sc)
	}
}


func TestRequestCollectorRedactHeadersFalse(t *testing.T) {
	c := NewRequestCollector(WithRedactHeaders(false))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer my-token")
	req.Header.Set("Cookie", "session=abc123")

	res := ResponseData{
		StatusCode: 200,
		Headers:    http.Header{"Set-Cookie": []string{"session=xyz789"}},
	}

	result, err := c.Collect(context.Background(), req, res)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := result.(*RequestData)

	if auth := data.Headers["Authorization"]; len(auth) == 0 || auth[0] != "Bearer my-token" {
		t.Errorf("Authorization should not be redacted: got %v", auth)
	}
	if cookie := data.Headers["Cookie"]; len(cookie) == 0 || cookie[0] != "session=abc123" {
		t.Errorf("Cookie should not be redacted: got %v", cookie)
	}
	if sc := data.ResponseHeaders["Set-Cookie"]; len(sc) == 0 || sc[0] != "session=xyz789" {
		t.Errorf("Set-Cookie should not be redacted: got %v", sc)
	}
}


func TestRequestCollectorNoQueryParams(t *testing.T) {
	c := NewRequestCollector()

	req := httptest.NewRequest(http.MethodGet, "/simple", nil)
	res := ResponseData{StatusCode: 200}

	result, err := c.Collect(context.Background(), req, res)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := result.(*RequestData)
	if data.QueryParams != nil {
		t.Errorf("expected nil QueryParams for request without query string, got %v", data.QueryParams)
	}
}

func TestRequestCollectorPanelMeta(t *testing.T) {
	c := NewRequestCollector()
	meta := c.PanelMeta()

	if meta.Name != "request" {
		t.Errorf("PanelMeta.Name: got %q, want %q", meta.Name, "request")
	}
	if meta.Label != "Request / Response" {
		t.Errorf("PanelMeta.Label: got %q, want %q", meta.Label, "Request / Response")
	}
	if meta.Component != "RequestPanel" {
		t.Errorf("PanelMeta.Component: got %q, want %q", meta.Component, "RequestPanel")
	}
}


func TestRequestCollectorCurlCommandGenerated(t *testing.T) {
	c := NewRequestCollector()

	req := httptest.NewRequest(http.MethodGet, "http://localhost:8080/api/users", nil)
	req.Header.Set("Accept", "application/json")
	res := ResponseData{StatusCode: 200}

	result, err := c.Collect(context.Background(), req, res)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := result.(*RequestData)
	if data.CurlCommand == "" {
		t.Error("expected curl command to be generated")
	}
	if !strings.Contains(data.CurlCommand, "curl") {
		t.Errorf("expected 'curl' in command, got: %s", data.CurlCommand)
	}
	if !strings.Contains(data.CurlCommand, "localhost:8080/api/users") {
		t.Errorf("expected URL in curl command, got: %s", data.CurlCommand)
	}
}

func TestRequestCollectorCurlWithBody(t *testing.T) {
	c := NewRequestCollector(WithBodyCapture(true))
	jsonBody := `{"name":"test"}`
	req := httptest.NewRequest(http.MethodPost, "http://localhost:8080/api/data", strings.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	// Capture body first (as middleware would)
	ctx, req := c.CaptureBody(context.Background(), req)

	res := ResponseData{StatusCode: 201}
	result, err := c.Collect(ctx, req, res)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := result.(*RequestData)
	if !strings.Contains(data.CurlCommand, "-X POST") {
		t.Errorf("expected -X POST in curl, got: %s", data.CurlCommand)
	}
	if !strings.Contains(data.CurlCommand, "-d") {
		t.Errorf("expected -d in curl, got: %s", data.CurlCommand)
	}
	if !strings.Contains(data.CurlCommand, jsonBody) {
		t.Errorf("expected body in curl, got: %s", data.CurlCommand)
	}
}


func TestRequestCollectorDefaultOptions(t *testing.T) {
	c := NewRequestCollector()
	if c.options.bodyCaptureEnabled {
		t.Error("body capture should be disabled by default")
	}
	if c.options.bodyMaxSize != DefaultBodyMaxSize {
		t.Errorf("body max size: got %d, want %d", c.options.bodyMaxSize, DefaultBodyMaxSize)
	}
	if !c.options.redactHeaders {
		t.Error("header redaction should be enabled by default")
	}
	if c.options.bodyContentTypes != nil {
		t.Error("content type whitelist should be nil by default")
	}
}

func TestRequestCollectorWithBodyCapture(t *testing.T) {
	c := NewRequestCollector(WithBodyCapture(true))
	if !c.options.bodyCaptureEnabled {
		t.Error("body capture should be enabled via option")
	}
}

func TestRequestCollectorWithBodyMaxSize(t *testing.T) {
	c := NewRequestCollector(WithBodyMaxSize(2097152))
	if c.options.bodyMaxSize != 2097152 {
		t.Errorf("body max size: got %d, want %d", c.options.bodyMaxSize, 2097152)
	}
}

func TestRequestCollectorWithBodyContentTypes(t *testing.T) {
	c := NewRequestCollector(WithBodyContentTypes("application/json", "text/plain"))
	if len(c.options.bodyContentTypes) != 2 {
		t.Errorf("content types: got %d items, want 2", len(c.options.bodyContentTypes))
	}
}

func TestRequestCollectorWithRedactHeaders(t *testing.T) {
	c := NewRequestCollector(WithRedactHeaders(false))
	if c.options.redactHeaders {
		t.Error("header redaction should be disabled via option")
	}
}


func TestRequestCollectorEnvVarBodyCapture(t *testing.T) {
	os.Setenv(EnvCaptureBody, "true")
	defer os.Unsetenv(EnvCaptureBody)

	c := NewRequestCollector()
	if !c.options.bodyCaptureEnabled {
		t.Error("body capture should be enabled via env var")
	}
}

func TestRequestCollectorEnvVarMaxSize(t *testing.T) {
	os.Setenv(EnvBodyMaxSize, "2097152")
	defer os.Unsetenv(EnvBodyMaxSize)

	c := NewRequestCollector()
	if c.options.bodyMaxSize != 2097152 {
		t.Errorf("body max size from env: got %d, want %d", c.options.bodyMaxSize, 2097152)
	}
}

func TestRequestCollectorEnvVarRedactHeaders(t *testing.T) {
	os.Setenv(EnvRedactHeaders, "false")
	defer os.Unsetenv(EnvRedactHeaders)

	c := NewRequestCollector()
	if c.options.redactHeaders {
		t.Error("header redaction should be disabled via env var")
	}
}

func TestRequestCollectorOptionOverridesEnv(t *testing.T) {
	os.Setenv(EnvCaptureBody, "true")
	defer os.Unsetenv(EnvCaptureBody)

	// Programmatic option should override env
	c := NewRequestCollector(WithBodyCapture(false))
	if c.options.bodyCaptureEnabled {
		t.Error("programmatic option should override env var")
	}
}

func TestRequestCollectorBackwardCompatible(t *testing.T) {
	// NewRequestCollector with no args should work like before
	c := NewRequestCollector()
	if c.Name() != "request" {
		t.Errorf("Name(): got %q, want %q", c.Name(), "request")
	}

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	res := ResponseData{StatusCode: 200}

	result, err := c.Collect(context.Background(), req, res)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := result.(*RequestData)
	if data.Method != "GET" {
		t.Errorf("Method: got %q, want %q", data.Method, "GET")
	}
	if data.Body != "" {
		t.Error("body should be empty when capture is disabled")
	}
}

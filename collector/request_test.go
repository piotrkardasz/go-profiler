package collector

import (
	"context"
	"net/http"
	"net/http/httptest"
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

	// Authorization should be redacted
	if auth := data.Headers["Authorization"]; len(auth) == 0 || auth[0] != "[REDACTED]" {
		t.Errorf("Authorization header not redacted: got %v", auth)
	}

	// Cookie should be redacted
	if cookie := data.Headers["Cookie"]; len(cookie) == 0 || cookie[0] != "[REDACTED]" {
		t.Errorf("Cookie header not redacted: got %v", cookie)
	}

	// Set-Cookie in response should be redacted
	if sc := data.ResponseHeaders["Set-Cookie"]; len(sc) == 0 || sc[0] != "[REDACTED]" {
		t.Errorf("Set-Cookie response header not redacted: got %v", sc)
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

package http

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRoundTrip_NoProfileContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello"))
	}))
	defer server.Close()

	transport := NewTransport("test-svc", http.DefaultTransport)

	req, _ := http.NewRequest("GET", server.URL+"/test", nil)
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	// No profiling context, so no calls should be recorded anywhere
	calls := CallsFromContext(req.Context())
	if calls != nil {
		t.Error("expected no calls captured without profiling context")
	}
}

func TestRoundTrip_CapturesBasicCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom", "response-val")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("response body"))
	}))
	defer server.Close()

	ctx := WithContext(context.Background())
	transport := NewTransport("product-api", http.DefaultTransport)

	req, _ := http.NewRequestWithContext(ctx, "POST", server.URL+"/items", strings.NewReader("request data"))
	req.Header.Set("Content-Type", "application/json")

	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	// Read response to ensure it's intact
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "response body" {
		t.Errorf("expected response body 'response body', got %q", string(body))
	}

	calls := CallsFromContext(ctx)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}

	c := calls[0]
	if c.Index != 0 {
		t.Errorf("expected index 0, got %d", c.Index)
	}
	if c.Service != "product-api" {
		t.Errorf("expected service 'product-api', got %q", c.Service)
	}
	if c.Method != "POST" {
		t.Errorf("expected method POST, got %q", c.Method)
	}
	if !strings.Contains(c.URL, "/items") {
		t.Errorf("expected URL to contain '/items', got %q", c.URL)
	}
	if c.StatusCode != http.StatusCreated {
		t.Errorf("expected status 201, got %d", c.StatusCode)
	}
	if c.DurationMs <= 0 {
		t.Error("expected positive duration")
	}
	if c.Error != "" {
		t.Errorf("expected no error, got %q", c.Error)
	}
	if c.Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}
}

func TestRoundTrip_HeaderRedaction(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Set-Cookie", "session=abc123")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ctx := WithContext(context.Background())
	transport := NewTransport("auth-svc", http.DefaultTransport,
		WithHeaderCapture(true),
		WithRedactHeaders("Authorization", "X-Api-Key"),
	)

	req, _ := http.NewRequestWithContext(ctx, "GET", server.URL, nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	req.Header.Set("X-Api-Key", "my-key")
	req.Header.Set("Accept", "application/json")

	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()

	calls := CallsFromContext(ctx)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}

	c := calls[0]

	// Authorization should be redacted
	if vals, ok := c.RequestHeaders["Authorization"]; !ok {
		t.Error("expected Authorization header to be present")
	} else if vals[0] != "[REDACTED]" {
		t.Errorf("expected Authorization to be '[REDACTED]', got %q", vals[0])
	}

	// X-Api-Key should be redacted
	if vals, ok := c.RequestHeaders["X-Api-Key"]; !ok {
		t.Error("expected X-Api-Key header to be present")
	} else if vals[0] != "[REDACTED]" {
		t.Errorf("expected X-Api-Key to be '[REDACTED]', got %q", vals[0])
	}

	// Accept should not be redacted
	if vals, ok := c.RequestHeaders["Accept"]; !ok {
		t.Error("expected Accept header to be present")
	} else if vals[0] != "application/json" {
		t.Errorf("expected Accept 'application/json', got %q", vals[0])
	}
}

func TestRoundTrip_HeaderCaptureDisabled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ctx := WithContext(context.Background())
	transport := NewTransport("svc", http.DefaultTransport, WithHeaderCapture(false))

	req, _ := http.NewRequestWithContext(ctx, "GET", server.URL, nil)
	req.Header.Set("Accept", "text/html")

	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()

	calls := CallsFromContext(ctx)
	c := calls[0]

	if c.RequestHeaders != nil {
		t.Error("expected no request headers when capture is disabled")
	}
	if c.ResponseHeaders != nil {
		t.Error("expected no response headers when capture is disabled")
	}
}

func TestRoundTrip_BodyCapture(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the server can still read the full request body
		body, _ := io.ReadAll(r.Body)
		w.Write([]byte("echo:" + string(body)))
	}))
	defer server.Close()

	ctx := WithContext(context.Background())
	transport := NewTransport("svc", http.DefaultTransport, WithBodyCapture(true))

	req, _ := http.NewRequestWithContext(ctx, "POST", server.URL, strings.NewReader("hello world"))
	req.Header.Set("Content-Type", "text/plain")

	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify response body is still readable
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if string(respBody) != "echo:hello world" {
		t.Errorf("expected 'echo:hello world', got %q", string(respBody))
	}

	calls := CallsFromContext(ctx)
	c := calls[0]

	if c.RequestBody != "hello world" {
		t.Errorf("expected request body 'hello world', got %q", c.RequestBody)
	}
	if c.ResponseBody != "echo:hello world" {
		t.Errorf("expected response body 'echo:hello world', got %q", c.ResponseBody)
	}
}

func TestRoundTrip_BodyTruncation(t *testing.T) {
	largeBody := strings.Repeat("x", 200)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(strings.Repeat("y", 200)))
	}))
	defer server.Close()

	ctx := WithContext(context.Background())
	transport := NewTransport("svc", http.DefaultTransport,
		WithBodyCapture(true),
		WithMaxBodySize(100),
	)

	req, _ := http.NewRequestWithContext(ctx, "POST", server.URL, strings.NewReader(largeBody))
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify response body is still fully readable
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if len(respBody) != 200 {
		t.Errorf("expected full 200 byte response body, got %d bytes", len(respBody))
	}

	calls := CallsFromContext(ctx)
	c := calls[0]

	if !strings.HasSuffix(c.RequestBody, "[truncated]") {
		t.Errorf("expected request body to end with '[truncated]', got %q", c.RequestBody)
	}
	if !strings.HasSuffix(c.ResponseBody, "[truncated]") {
		t.Errorf("expected response body to end with '[truncated]', got %q", c.ResponseBody)
	}
}

func TestRoundTrip_TransportError(t *testing.T) {
	ctx := WithContext(context.Background())
	transport := NewTransport("failing-svc", &failingTransport{err: errors.New("connection refused")})

	req, _ := http.NewRequestWithContext(ctx, "GET", "http://unreachable:9999/api", nil)
	resp, err := transport.RoundTrip(req)

	if err == nil {
		t.Fatal("expected error")
	}
	if resp != nil {
		t.Error("expected nil response on transport error")
	}

	calls := CallsFromContext(ctx)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}

	c := calls[0]
	if c.Error != "connection refused" {
		t.Errorf("expected error 'connection refused', got %q", c.Error)
	}
	if c.StatusCode != 0 {
		t.Errorf("expected status code 0, got %d", c.StatusCode)
	}
	if c.DurationMs < 0 {
		t.Error("expected non-negative duration on error")
	}
}

func TestRoundTrip_ConcurrentCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ctx := WithContext(context.Background())
	transport := NewTransport("concurrent-svc", http.DefaultTransport)

	const numCalls = 20
	var wg sync.WaitGroup
	wg.Add(numCalls)

	for i := 0; i < numCalls; i++ {
		go func() {
			defer wg.Done()
			req, _ := http.NewRequestWithContext(ctx, "GET", server.URL, nil)
			resp, err := transport.RoundTrip(req)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			resp.Body.Close()
		}()
	}

	wg.Wait()

	calls := CallsFromContext(ctx)
	if len(calls) != numCalls {
		t.Errorf("expected %d calls, got %d", numCalls, len(calls))
	}
}

func TestRoundTrip_CurlGeneration(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ctx := WithContext(context.Background())
	transport := NewTransport("svc", http.DefaultTransport,
		WithCurlGeneration(true),
		WithBodyCapture(true),
	)

	req, _ := http.NewRequestWithContext(ctx, "POST", server.URL+"/api/items", strings.NewReader(`{"name":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := transport.RoundTrip(req)
	resp.Body.Close()

	calls := CallsFromContext(ctx)
	c := calls[0]

	if c.CurlCommand == "" {
		t.Error("expected curl command to be generated")
	}
	if !strings.Contains(c.CurlCommand, "curl -X POST") {
		t.Errorf("expected curl to contain 'curl -X POST', got %q", c.CurlCommand)
	}
	if !strings.Contains(c.CurlCommand, "/api/items") {
		t.Errorf("expected curl to contain URL path, got %q", c.CurlCommand)
	}
}

func TestRoundTrip_CurlGenerationDisabled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ctx := WithContext(context.Background())
	transport := NewTransport("svc", http.DefaultTransport, WithCurlGeneration(false))

	req, _ := http.NewRequestWithContext(ctx, "GET", server.URL, nil)
	resp, _ := transport.RoundTrip(req)
	resp.Body.Close()

	calls := CallsFromContext(ctx)
	if calls[0].CurlCommand != "" {
		t.Error("expected no curl command when generation is disabled")
	}
}

func TestRoundTrip_NilBaseTransport(t *testing.T) {
	// Should default to http.DefaultTransport
	transport := NewTransport("svc", nil)
	pt := transport.(*profilingTransport)
	if pt.base != http.DefaultTransport {
		t.Error("expected nil base to default to http.DefaultTransport")
	}
}

func TestNewTransport_EmptyServiceNamePanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for empty service name")
		}
	}()
	NewTransport("", http.DefaultTransport)
}

func TestRoundTrip_MultipleServicesDistinct(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ctx := WithContext(context.Background())
	transportA := NewTransport("service-a", http.DefaultTransport)
	transportB := NewTransport("service-b", http.DefaultTransport)

	req1, _ := http.NewRequestWithContext(ctx, "GET", server.URL+"/a", nil)
	resp1, _ := transportA.RoundTrip(req1)
	resp1.Body.Close()

	req2, _ := http.NewRequestWithContext(ctx, "GET", server.URL+"/b", nil)
	resp2, _ := transportB.RoundTrip(req2)
	resp2.Body.Close()

	calls := CallsFromContext(ctx)
	if len(calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(calls))
	}
	if calls[0].Service != "service-a" {
		t.Errorf("expected first call service 'service-a', got %q", calls[0].Service)
	}
	if calls[1].Service != "service-b" {
		t.Errorf("expected second call service 'service-b', got %q", calls[1].Service)
	}
}

// Benchmarks

func BenchmarkRoundTrip_NoProfile(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	transport := NewTransport("svc", http.DefaultTransport)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req, _ := http.NewRequest("GET", server.URL, nil)
		resp, _ := transport.RoundTrip(req)
		if resp != nil {
			resp.Body.Close()
		}
	}
}

func BenchmarkRoundTrip_WithProfile(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ctx := WithContext(context.Background())
	transport := NewTransport("svc", http.DefaultTransport)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req, _ := http.NewRequestWithContext(ctx, "GET", server.URL, nil)
		resp, _ := transport.RoundTrip(req)
		if resp != nil {
			resp.Body.Close()
		}
	}
}

// failingTransport is a mock RoundTripper that always returns an error.
type failingTransport struct {
	err error
}

func (f *failingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return nil, f.err
}

// slowTransport adds artificial delay.
type slowTransport struct {
	delay time.Duration
	base  http.RoundTripper
}

func (s *slowTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	time.Sleep(s.delay)
	return s.base.RoundTrip(req)
}

// responseTransport returns a fixed response without network.
type responseTransport struct {
	statusCode int
	body       string
	headers    http.Header
}

func (r *responseTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp := &http.Response{
		StatusCode:    r.statusCode,
		Header:        r.headers,
		Body:          io.NopCloser(bytes.NewBufferString(r.body)),
		ContentLength: int64(len(r.body)),
	}
	return resp, nil
}

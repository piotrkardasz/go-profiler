package http

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/piotrkardasz/go-profiler/collector"
)

func TestCollector_Name(t *testing.T) {
	c := New()
	if c.Name() != "http" {
		t.Errorf("expected name 'http', got %q", c.Name())
	}
}

func TestCollector_Reset(t *testing.T) {
	c := New()
	// Reset should not panic
	c.Reset()
}

func TestCollector_SetupContext(t *testing.T) {
	c := New()
	ctx := context.Background()
	ctx = c.SetupContext(ctx)

	rc := callsFromContext(ctx)
	if rc == nil {
		t.Error("expected SetupContext to initialize tracker")
	}
}

func TestCollector_PanelMeta(t *testing.T) {
	c := New()
	meta := c.PanelMeta()

	if meta.Name != "http" {
		t.Errorf("expected panel name 'http', got %q", meta.Name)
	}
	if meta.Label != "HTTP Clients" {
		t.Errorf("expected panel label 'HTTP Clients', got %q", meta.Label)
	}
	if meta.Icon != "world" {
		t.Errorf("expected panel icon 'world', got %q", meta.Icon)
	}
	if meta.Component != "HttpPanel" {
		t.Errorf("expected panel component 'HttpPanel', got %q", meta.Component)
	}
}

func TestCollector_Collect_NoCalls(t *testing.T) {
	c := New()
	ctx := c.SetupContext(context.Background())

	req, _ := http.NewRequest("GET", "/test", nil)
	result, err := c.Collect(ctx, req, collector.ResponseData{StatusCode: 200})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, ok := result.(HTTPData)
	if !ok {
		t.Fatal("expected result to be HTTPData")
	}
	if len(data.Calls) != 0 {
		t.Errorf("expected 0 calls, got %d", len(data.Calls))
	}
	if data.Summary.TotalCalls != 0 {
		t.Errorf("expected 0 total calls, got %d", data.Summary.TotalCalls)
	}
}

func TestCollector_Collect_WithCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer server.Close()

	c := New(WithSlowThreshold(50 * time.Millisecond))
	ctx := c.SetupContext(context.Background())

	// Simulate outbound calls
	transport := NewTransport("test-svc", http.DefaultTransport)
	for i := 0; i < 3; i++ {
		req, _ := http.NewRequestWithContext(ctx, "GET", server.URL+"/item", nil)
		resp, err := transport.RoundTrip(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		resp.Body.Close()
	}

	// Collect
	inboundReq, _ := http.NewRequest("GET", "/api/handler", nil)
	result, err := c.Collect(ctx, inboundReq, collector.ResponseData{StatusCode: 200})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := result.(HTTPData)
	if data.Summary.TotalCalls != 3 {
		t.Errorf("expected 3 total calls, got %d", data.Summary.TotalCalls)
	}
	if data.Summary.CallsPerService["test-svc"] != 3 {
		t.Errorf("expected 3 calls for test-svc, got %d", data.Summary.CallsPerService["test-svc"])
	}
}

func TestCollector_Collect_WithAnalysis(t *testing.T) {
	c := New(WithSlowThreshold(10 * time.Millisecond))
	ctx := c.SetupContext(context.Background())

	// Create a slow server
	slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(15 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer slowServer.Close()

	// Create a failing server
	failServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer failServer.Close()

	slowTransport := NewTransport("slow-svc", http.DefaultTransport)
	failTransport := NewTransport("fail-svc", http.DefaultTransport)

	// Make a slow call
	req1, _ := http.NewRequestWithContext(ctx, "GET", slowServer.URL, nil)
	resp1, _ := slowTransport.RoundTrip(req1)
	resp1.Body.Close()

	// Make a failed call
	req2, _ := http.NewRequestWithContext(ctx, "GET", failServer.URL, nil)
	resp2, _ := failTransport.RoundTrip(req2)
	resp2.Body.Close()

	// Collect
	inboundReq, _ := http.NewRequest("GET", "/test", nil)
	result, _ := c.Collect(ctx, inboundReq, collector.ResponseData{StatusCode: 200})
	data := result.(HTTPData)

	if data.Summary.SlowCount != 1 {
		t.Errorf("expected 1 slow call, got %d", data.Summary.SlowCount)
	}
	if data.Summary.FailedCount != 1 {
		t.Errorf("expected 1 failed call, got %d", data.Summary.FailedCount)
	}
}

func TestCollector_Collect_JSONSerializable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	c := New(WithBodyCapture(true))
	ctx := c.SetupContext(context.Background())

	transport := NewTransport("json-svc", http.DefaultTransport, WithBodyCapture(true))
	req, _ := http.NewRequestWithContext(ctx, "POST", server.URL, strings.NewReader(`{"action":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := transport.RoundTrip(req)
	io.ReadAll(resp.Body)
	resp.Body.Close()

	inboundReq, _ := http.NewRequest("GET", "/test", nil)
	result, _ := c.Collect(ctx, inboundReq, collector.ResponseData{StatusCode: 200})

	// Verify it marshals to JSON cleanly
	jsonBytes, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal HTTPData to JSON: %v", err)
	}

	// Verify it unmarshals back correctly
	var data HTTPData
	if err := json.Unmarshal(jsonBytes, &data); err != nil {
		t.Fatalf("failed to unmarshal HTTPData: %v", err)
	}

	if data.Summary.TotalCalls != 1 {
		t.Errorf("expected 1 call after JSON roundtrip, got %d", data.Summary.TotalCalls)
	}
	if data.Calls[0].RequestBody != `{"action":"test"}` {
		t.Errorf("expected request body preserved after JSON roundtrip, got %q", data.Calls[0].RequestBody)
	}
}

func TestCollector_ImplementsInterfaces(t *testing.T) {
	c := New()

	// Verify interface compliance at compile time
	var _ collector.Collector = c
	var _ collector.ContextSetup = c
	var _ collector.PanelProvider = c
}

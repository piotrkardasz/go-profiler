package profiler

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// BenchmarkMiddlewareOverhead measures the latency added by the profiler
// middleware to the HTTP response, excluding handler execution time.
// With async collection, this should be <5µs per request.
func BenchmarkMiddlewareOverhead(b *testing.B) {
	store := newTestStorage()
	p := New(DefaultConfig(), store)

	handler := p.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}
}

// BenchmarkMiddlewareDisabled measures the overhead when profiler is disabled.
// This should be near-zero (just a bool check).
func BenchmarkMiddlewareDisabled(b *testing.B) {
	store := newTestStorage()
	cfg := DefaultConfig()
	cfg.Enabled = false
	p := New(cfg, store)

	handler := p.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}
}

// BenchmarkMiddlewareSampled0 measures overhead when SampleRate=0 (all skipped).
func BenchmarkMiddlewareSampled0(b *testing.B) {
	store := newTestStorage()
	cfg := DefaultConfig()
	cfg.SampleRate = 0.0
	p := New(cfg, store)

	handler := p.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}
}

// BenchmarkHandlerBaseline measures handler execution without any middleware.
// Use to compare against BenchmarkMiddlewareOverhead.
func BenchmarkHandlerBaseline(b *testing.B) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}
}

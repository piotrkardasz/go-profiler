package profiler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/piotrkardasz/go-profiler/collector"
)

// slowCollector is a collector with artificial delay for testing async behavior.
type slowCollector struct {
	delay time.Duration
}

func (c *slowCollector) Name() string { return "slow" }
func (c *slowCollector) Collect(_ context.Context, _ *http.Request, _ collector.ResponseData) (any, error) {
	time.Sleep(c.delay)
	return "slow_data", nil
}
func (c *slowCollector) Reset() {}

func TestMiddlewareAsyncDoesNotBlockResponse(t *testing.T) {
	store := newTestStorage()
	p := New(DefaultConfig(), store)

	// Register a collector that takes 200ms to run
	p.AddCollector(&slowCollector{delay: 200 * time.Millisecond})

	handler := p.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("fast"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rec := httptest.NewRecorder()

	start := time.Now()
	handler.ServeHTTP(rec, req)
	elapsed := time.Since(start)

	// Response should return much faster than 200ms since collection is async
	if elapsed >= 100*time.Millisecond {
		t.Errorf("response took %v, expected < 100ms (collection should be async)", elapsed)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", rec.Code)
	}
	if rec.Body.String() != "fast" {
		t.Errorf("body: got %q, want %q", rec.Body.String(), "fast")
	}

	// But the profile should eventually be stored
	profilerID := rec.Header().Get(HeaderProfilerID)
	profile := waitForProfile(t, store, profilerID, 2*time.Second)
	if profile.CollectorData["slow"] != "slow_data" {
		t.Errorf("expected slow_data, got %v", profile.CollectorData["slow"])
	}
}

func TestProfilerShutdownWaitsForInflight(t *testing.T) {
	store := newTestStorage()
	p := New(DefaultConfig(), store)

	// Slow collector to ensure goroutine is in-flight during shutdown
	p.AddCollector(&slowCollector{delay: 100 * time.Millisecond})

	handler := p.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	profilerID := rec.Header().Get(HeaderProfilerID)

	// Shutdown should wait for the in-flight profile to complete
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := p.Shutdown(ctx)
	if err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	// After shutdown returns, profile must be stored
	profile, loadErr := store.Load(profilerID)
	if loadErr != nil {
		t.Fatalf("profile not stored after Shutdown: %v", loadErr)
	}
	if profile.ID != profilerID {
		t.Errorf("profile ID mismatch: got %q, want %q", profile.ID, profilerID)
	}
}

func TestProfilerShutdownTimeout(t *testing.T) {
	store := newTestStorage()
	p := New(DefaultConfig(), store)

	// Very slow collector — will outlast our shutdown timeout
	p.AddCollector(&slowCollector{delay: 5 * time.Second})

	handler := p.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Shutdown with very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err := p.Shutdown(ctx)
	if err == nil {
		t.Error("expected timeout error from Shutdown")
	}
	if err != context.DeadlineExceeded {
		t.Errorf("expected DeadlineExceeded, got %v", err)
	}
}

func TestProfilerShutdownSkipsNewRequests(t *testing.T) {
	store := newTestStorage()
	p := New(DefaultConfig(), store)

	// Trigger shutdown immediately (no inflight work)
	ctx := context.Background()
	if err := p.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	// New requests after shutdown should skip profiling
	handler := p.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("still works"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Handler still executes
	if rec.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", rec.Code)
	}
	if rec.Body.String() != "still works" {
		t.Errorf("body: got %q, want %q", rec.Body.String(), "still works")
	}

	// But no profiler header set
	if id := rec.Header().Get(HeaderProfilerID); id != "" {
		t.Errorf("X-Profiler-Id should not be set after shutdown, got %q", id)
	}
}

func TestMiddlewareSampling100Percent(t *testing.T) {
	store := newTestStorage()
	cfg := DefaultConfig()
	cfg.SampleRate = 1.0
	p := New(cfg, store)

	handler := p.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// All 10 requests should be profiled
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if id := rec.Header().Get(HeaderProfilerID); id == "" {
			t.Errorf("request %d: X-Profiler-Id not set at SampleRate=1.0", i)
		}
	}

	waitForProfileCount(t, store, 10, 2*time.Second)
}

func TestMiddlewareSampling0Percent(t *testing.T) {
	store := newTestStorage()
	cfg := DefaultConfig()
	cfg.SampleRate = 0.0
	p := New(cfg, store)

	handler := p.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// No requests should be profiled
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if id := rec.Header().Get(HeaderProfilerID); id != "" {
			t.Errorf("request %d: X-Profiler-Id should not be set at SampleRate=0.0, got %q", i, id)
		}
	}

	// Wait a bit then verify nothing stored
	time.Sleep(50 * time.Millisecond)
	summaries, _ := store.List(SearchCriteria{})
	if len(summaries) != 0 {
		t.Errorf("expected 0 profiles at SampleRate=0.0, got %d", len(summaries))
	}
}

func TestMiddlewareSamplingPartial(t *testing.T) {
	store := newTestStorage()
	cfg := DefaultConfig()
	cfg.SampleRate = 0.5
	p := New(cfg, store)

	handler := p.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	totalRequests := 200
	profiled := 0

	for i := 0; i < totalRequests; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if id := rec.Header().Get(HeaderProfilerID); id != "" {
			profiled++
		}
	}

	// With 200 requests at 50% rate, expect ~100 profiled.
	// Use wide tolerance (30-70%) to avoid flaky test.
	minExpected := totalRequests * 30 / 100
	maxExpected := totalRequests * 70 / 100
	if profiled < minExpected || profiled > maxExpected {
		t.Errorf("expected ~50%% profiled (%d-%d), got %d/%d",
			minExpected, maxExpected, profiled, totalRequests)
	}
}

func TestMiddlewareNoDataRaceUnderLoad(t *testing.T) {
	store := newTestStorage()
	p := New(DefaultConfig(), store)

	// Use a collector that is safe for concurrent use (stateless)
	p.AddCollector(&safeCollector{name: "safe", data: "value"})

	handler := p.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))

	// Launch 50 concurrent requests
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Errorf("status: got %d, want 200", rec.Code)
			}
		}()
	}

	wg.Wait()

	// Wait for all profiles to be stored
	waitForProfileCount(t, store, 50, 5*time.Second)
}

// safeCollector is a stateless thread-safe collector for concurrency tests.
type safeCollector struct {
	name string
	data any
}

func (c *safeCollector) Name() string { return c.name }
func (c *safeCollector) Collect(_ context.Context, _ *http.Request, _ collector.ResponseData) (any, error) {
	return c.data, nil
}
func (c *safeCollector) Reset() {}

func TestConfigCollectorCacheHit(t *testing.T) {
	// Create a reader that counts how many times Read() is called
	callCount := 0
	countingReader := &countingConfigReader{callCount: &callCount}

	cc := collector.NewConfigCollector(
		collector.WithoutEnvFile(),
		collector.WithoutEnvVars(),
		collector.WithoutBuildInfo(),
		collector.WithReader(countingReader),
	)

	// Construction calls Read() once to populate cache
	if callCount != 1 {
		t.Fatalf("expected 1 Read() call at construction, got %d", callCount)
	}

	// Multiple Collect() calls should not trigger additional Read() calls
	for i := 0; i < 10; i++ {
		_, err := cc.Collect(context.Background(), nil, collector.ResponseData{})
		if err != nil {
			t.Fatalf("Collect: %v", err)
		}
	}

	if callCount != 1 {
		t.Errorf("expected Read() called only once (cached), got %d calls", callCount)
	}
}

func TestConfigCollectorRefresh(t *testing.T) {
	callCount := 0
	countingReader := &countingConfigReader{callCount: &callCount}

	cc := collector.NewConfigCollector(
		collector.WithoutEnvFile(),
		collector.WithoutEnvVars(),
		collector.WithoutBuildInfo(),
		collector.WithReader(countingReader),
	)

	if callCount != 1 {
		t.Fatalf("expected 1 call at construction, got %d", callCount)
	}

	// Refresh should re-read
	cc.Refresh()
	if callCount != 2 {
		t.Errorf("expected 2 calls after Refresh(), got %d", callCount)
	}
}

// countingConfigReader counts how many times Read() is called.
type countingConfigReader struct {
	callCount *int
}

func (r *countingConfigReader) Name() string { return "counting" }
func (r *countingConfigReader) Read() ([]collector.ConfigEntry, error) {
	*r.callCount++
	return []collector.ConfigEntry{
		{Key: "TEST_KEY", Value: "test_value"},
	}, nil
}

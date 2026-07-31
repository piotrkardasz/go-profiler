package profiler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// testStorage is a simple in-memory storage for middleware tests,
// avoiding import cycles with the storage package.
type testStorage struct {
	mu       sync.RWMutex
	profiles map[string]*Profile
}

func newTestStorage() *testStorage {
	return &testStorage{profiles: make(map[string]*Profile)}
}

func (s *testStorage) Store(profile *Profile) error {
	if profile == nil || profile.ID == "" {
		return fmt.Errorf("invalid profile")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.profiles[profile.ID] = profile
	return nil
}

func (s *testStorage) Load(id string) (*Profile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.profiles[id]
	if !ok {
		return nil, fmt.Errorf("not found: %s", id)
	}
	return p, nil
}

func (s *testStorage) List(criteria SearchCriteria) ([]*ProfileSummary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*ProfileSummary
	for _, p := range s.profiles {
		result = append(result, p.Summary())
	}
	return result, nil
}

func (s *testStorage) Purge(maxAge time.Duration) (int, error) {
	return 0, nil
}

func TestMiddlewareSetsProfilerIDHeader(t *testing.T) {
	store := newTestStorage()
	p := New(DefaultConfig(), store)

	handler := p.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Check X-Profiler-Id header is set
	profilerID := rec.Header().Get(HeaderProfilerID)
	if profilerID == "" {
		t.Fatal("X-Profiler-Id header not set")
	}
	if len(profilerID) != 16 {
		t.Errorf("expected 16 char profile ID, got %d chars: %q", len(profilerID), profilerID)
	}

	// Check response body is intact
	if rec.Body.String() != "hello" {
		t.Errorf("body: got %q, want %q", rec.Body.String(), "hello")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestMiddlewareStoresProfile(t *testing.T) {
	store := newTestStorage()
	p := New(DefaultConfig(), store)
	p.AddCollector(&mockCollector{name: "test", data: "collected"})

	handler := p.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("created"))
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/orders?id=123", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	profilerID := rec.Header().Get(HeaderProfilerID)
	if profilerID == "" {
		t.Fatal("X-Profiler-Id header not set")
	}

	// Wait for async store
	time.Sleep(50 * time.Millisecond)

	// Load the stored profile
	profile, err := store.Load(profilerID)
	if err != nil {
		t.Fatalf("failed to load profile: %v", err)
	}

	if profile.ID != profilerID {
		t.Errorf("ID: got %q, want %q", profile.ID, profilerID)
	}
	if profile.Method != "POST" {
		t.Errorf("Method: got %q, want POST", profile.Method)
	}
	if profile.URL != "/api/orders?id=123" {
		t.Errorf("URL: got %q, want /api/orders?id=123", profile.URL)
	}
	if profile.StatusCode != 201 {
		t.Errorf("StatusCode: got %d, want 201", profile.StatusCode)
	}
	if profile.Duration <= 0 {
		t.Errorf("Duration should be > 0, got %v", profile.Duration)
	}
	if profile.CollectorData == nil {
		t.Fatal("CollectorData is nil")
	}
	if profile.CollectorData["test"] != "collected" {
		t.Errorf("CollectorData[test]: got %v, want 'collected'", profile.CollectorData["test"])
	}
}

func TestMiddlewareDisabled(t *testing.T) {
	store := newTestStorage()
	cfg := DefaultConfig()
	cfg.Enabled = false
	p := New(cfg, store)

	handlerCalled := false
	handler := p.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if !handlerCalled {
		t.Error("handler was not called when profiler is disabled")
	}

	// No profiler header should be set
	if id := rec.Header().Get(HeaderProfilerID); id != "" {
		t.Errorf("X-Profiler-Id should not be set when disabled, got %q", id)
	}

	// No profile stored
	time.Sleep(20 * time.Millisecond)
	summaries, _ := store.List(SearchCriteria{})
	if len(summaries) != 0 {
		t.Errorf("expected no stored profiles when disabled, got %d", len(summaries))
	}
}

func TestMiddlewareSkipsProfilerRoutes(t *testing.T) {
	store := newTestStorage()
	p := New(DefaultConfig(), store)

	handler := p.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Request to profiler's own route
	req := httptest.NewRequest(http.MethodGet, "/_profiler/api/profiles", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Should not set profiler header on its own routes
	if id := rec.Header().Get(HeaderProfilerID); id != "" {
		t.Errorf("X-Profiler-Id should not be set for profiler routes, got %q", id)
	}
}

func TestMiddlewareCapturesStatusCode(t *testing.T) {
	store := newTestStorage()
	p := New(DefaultConfig(), store)

	tests := []struct {
		name   string
		status int
	}{
		{"200 OK", http.StatusOK},
		{"404 Not Found", http.StatusNotFound},
		{"500 Internal Server Error", http.StatusInternalServerError},
		{"301 Moved", http.StatusMovedPermanently},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := p.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
			}))

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.status {
				t.Errorf("response status: got %d, want %d", rec.Code, tt.status)
			}

			profilerID := rec.Header().Get(HeaderProfilerID)
			time.Sleep(30 * time.Millisecond)

			profile, err := store.Load(profilerID)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if profile.StatusCode != tt.status {
				t.Errorf("profile StatusCode: got %d, want %d", profile.StatusCode, tt.status)
			}
		})
	}
}

func TestMiddlewareDefaultStatusCode(t *testing.T) {
	store := newTestStorage()
	p := New(DefaultConfig(), store)

	// Handler that writes body without explicit WriteHeader
	handler := p.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("implicit 200"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	profilerID := rec.Header().Get(HeaderProfilerID)
	time.Sleep(30 * time.Millisecond)

	profile, err := store.Load(profilerID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if profile.StatusCode != 200 {
		t.Errorf("expected implicit 200 status, got %d", profile.StatusCode)
	}
}

func TestResponseWriterCapturesSize(t *testing.T) {
	rw := newResponseWriter(httptest.NewRecorder())

	rw.Write([]byte("hello"))
	rw.Write([]byte(" world"))

	if rw.size != 11 {
		t.Errorf("size: got %d, want 11", rw.size)
	}
}

func TestResponseWriterFlush(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := newResponseWriter(rec)

	// Should not panic even if flushing
	rw.Flush()

	if !rec.Flushed {
		t.Error("expected recorder to be flushed")
	}
}

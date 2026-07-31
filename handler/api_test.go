package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	profiler "github.com/piotrkardasz/go-profiler"
	"github.com/piotrkardasz/go-profiler/collector"
)

// mockStorage implements profiler.Storage for testing.
type mockStorage struct {
	mu       sync.RWMutex
	profiles map[string]*profiler.Profile
}

func newMockStorage() *mockStorage {
	return &mockStorage{profiles: make(map[string]*profiler.Profile)}
}

func (s *mockStorage) Store(p *profiler.Profile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.profiles[p.ID] = p
	return nil
}

func (s *mockStorage) Load(id string) (*profiler.Profile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.profiles[id]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return p, nil
}

func (s *mockStorage) List(criteria profiler.SearchCriteria) ([]*profiler.ProfileSummary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*profiler.ProfileSummary
	for _, p := range s.profiles {
		if criteria.Method != "" && p.Method != criteria.Method {
			continue
		}
		result = append(result, p.Summary())
	}
	// Apply limit
	limit := criteria.Limit
	if limit <= 0 {
		limit = 100
	}
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (s *mockStorage) Purge(maxAge time.Duration) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cutoff := time.Now().Add(-maxAge)
	removed := 0
	for id, p := range s.profiles {
		if p.Timestamp.Before(cutoff) {
			delete(s.profiles, id)
			removed++
		}
	}
	return removed, nil
}

// mockCollector for test profiler setup
type mockCollector struct{}

func (m *mockCollector) Name() string { return "test" }
func (m *mockCollector) Collect(_ interface{ Value(any) any }, _ *http.Request, _ collector.ResponseData) (any, error) {
	return nil, nil
}
func (m *mockCollector) Reset() {}

func setupTestHandler(t *testing.T) (*APIHandler, *mockStorage, *http.ServeMux) {
	t.Helper()

	store := newMockStorage()
	cfg := profiler.DefaultConfig()
	p := profiler.New(cfg, store)
	h := NewAPIHandler(p)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux, "/_profiler")

	return h, store, mux
}

func seedProfiles(store *mockStorage, count int) {
	now := time.Now()
	for i := 0; i < count; i++ {
		method := "GET"
		if i%3 == 0 {
			method = "POST"
		}
		store.Store(&profiler.Profile{
			ID:            fmt.Sprintf("profile-%03d", i),
			Method:        method,
			URL:           fmt.Sprintf("/api/resource/%d", i),
			StatusCode:    200,
			Timestamp:     now.Add(-time.Duration(i) * time.Minute),
			Duration:      time.Duration(50+i) * time.Millisecond,
			CollectorData: map[string]any{"test": "data"},
		})
	}
}

func TestListProfiles(t *testing.T) {
	_, store, mux := setupTestHandler(t)
	seedProfiles(store, 5)

	req := httptest.NewRequest(http.MethodGet, "/_profiler/api/profiles", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	profiles, ok := resp["profiles"].([]any)
	if !ok {
		t.Fatal("expected 'profiles' array in response")
	}
	if len(profiles) != 5 {
		t.Errorf("expected 5 profiles, got %d", len(profiles))
	}
}

func TestListProfilesWithMethodFilter(t *testing.T) {
	_, store, mux := setupTestHandler(t)
	seedProfiles(store, 9) // 3 POST (indices 0, 3, 6), 6 GET

	req := httptest.NewRequest(http.MethodGet, "/_profiler/api/profiles?method=POST", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)

	profiles := resp["profiles"].([]any)
	if len(profiles) != 3 {
		t.Errorf("expected 3 POST profiles, got %d", len(profiles))
	}
}

func TestListProfilesWithLimit(t *testing.T) {
	_, store, mux := setupTestHandler(t)
	seedProfiles(store, 10)

	req := httptest.NewRequest(http.MethodGet, "/_profiler/api/profiles?limit=3", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)

	profiles := resp["profiles"].([]any)
	if len(profiles) != 3 {
		t.Errorf("expected 3 profiles with limit=3, got %d", len(profiles))
	}
}

func TestGetProfileByID(t *testing.T) {
	_, store, mux := setupTestHandler(t)

	profile := &profiler.Profile{
		ID:         "test-profile-123",
		Method:     "GET",
		URL:        "/api/users",
		StatusCode: 200,
		Timestamp:  time.Now(),
		Duration:   75 * time.Millisecond,
		CollectorData: map[string]any{
			"request": map[string]any{"host": "localhost"},
		},
	}
	store.Store(profile)

	req := httptest.NewRequest(http.MethodGet, "/_profiler/api/profiles/test-profile-123", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusOK)
	}

	var resp profiler.Profile
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}

	if resp.ID != "test-profile-123" {
		t.Errorf("ID: got %q, want %q", resp.ID, "test-profile-123")
	}
	if resp.Method != "GET" {
		t.Errorf("Method: got %q, want %q", resp.Method, "GET")
	}
	if resp.StatusCode != 200 {
		t.Errorf("StatusCode: got %d, want 200", resp.StatusCode)
	}
}

func TestGetProfileNotFound(t *testing.T) {
	_, _, mux := setupTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/_profiler/api/profiles/nonexistent", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want %d", rec.Code, http.StatusNotFound)
	}

	var resp map[string]string
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["error"] != "profile not found" {
		t.Errorf("error message: got %q", resp["error"])
	}
}

func TestPurgeProfiles(t *testing.T) {
	_, store, mux := setupTestHandler(t)

	// Add old and recent profiles
	store.Store(&profiler.Profile{
		ID:            "old-profile",
		Method:        "GET",
		URL:           "/old",
		StatusCode:    200,
		Timestamp:     time.Now().Add(-48 * time.Hour),
		CollectorData: map[string]any{},
	})
	store.Store(&profiler.Profile{
		ID:            "recent-profile",
		Method:        "GET",
		URL:           "/recent",
		StatusCode:    200,
		Timestamp:     time.Now(),
		CollectorData: map[string]any{},
	})

	req := httptest.NewRequest(http.MethodDelete, "/_profiler/api/profiles?max_age=24h", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)

	removed := int(resp["removed"].(float64))
	if removed != 1 {
		t.Errorf("expected 1 removed, got %d", removed)
	}
}

func TestGetCollectors(t *testing.T) {
	store := newMockStorage()
	cfg := profiler.DefaultConfig()
	p := profiler.New(cfg, store)
	p.AddCollector(collector.NewRequestCollector())
	p.AddCollector(collector.NewTimingCollector())
	p.AddCollector(collector.NewMemoryCollector())

	h := NewAPIHandler(p)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux, "/_profiler")

	req := httptest.NewRequest(http.MethodGet, "/_profiler/api/collectors", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)

	collectors, ok := resp["collectors"].([]any)
	if !ok {
		t.Fatal("expected 'collectors' array in response")
	}
	if len(collectors) != 3 {
		t.Fatalf("expected 3 collectors, got %d", len(collectors))
	}

	// Verify first collector has expected fields
	first := collectors[0].(map[string]any)
	if first["name"] != "request" {
		t.Errorf("first collector name: got %q, want %q", first["name"], "request")
	}
	if first["label"] != "Request / Response" {
		t.Errorf("first collector label: got %q, want %q", first["label"], "Request / Response")
	}
	if first["component"] != "RequestPanel" {
		t.Errorf("first collector component: got %q, want %q", first["component"], "RequestPanel")
	}
}

func TestMethodNotAllowed(t *testing.T) {
	_, _, mux := setupTestHandler(t)

	req := httptest.NewRequest(http.MethodPut, "/_profiler/api/profiles", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status: got %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestCORSHeaders(t *testing.T) {
	_, _, mux := setupTestHandler(t)

	req := httptest.NewRequest(http.MethodOptions, "/_profiler/api/profiles", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("OPTIONS status: got %d, want %d", rec.Code, http.StatusNoContent)
	}

	if origin := rec.Header().Get("Access-Control-Allow-Origin"); origin != "*" {
		t.Errorf("CORS origin: got %q, want '*'", origin)
	}
	if methods := rec.Header().Get("Access-Control-Allow-Methods"); methods == "" {
		t.Error("CORS methods header not set")
	}
}

func TestContentTypeJSON(t *testing.T) {
	_, store, mux := setupTestHandler(t)
	seedProfiles(store, 1)

	req := httptest.NewRequest(http.MethodGet, "/_profiler/api/profiles", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type: got %q, want 'application/json'", ct)
	}
}

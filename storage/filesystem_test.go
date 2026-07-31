package storage

import (
	"fmt"
	"sync"
	"testing"
	"time"

	profiler "github.com/piotrkardasz/go-profiler"
)

func newTestProfile(id, method, url string, status int, ts time.Time) *profiler.Profile {
	return &profiler.Profile{
		ID:            id,
		Method:        method,
		URL:           url,
		StatusCode:    status,
		Timestamp:     ts,
		Duration:      100 * time.Millisecond,
		CollectorData: map[string]any{"test": "data"},
	}
}

func TestFilesystemStorageNewCreatesDir(t *testing.T) {
	dir := t.TempDir() + "/subdir/profiles"
	store, err := NewFilesystemStorage(dir)
	if err != nil {
		t.Fatalf("NewFilesystemStorage: %v", err)
	}
	if store == nil {
		t.Fatal("expected non-nil store")
	}
}

func TestFilesystemStorageStoreAndLoad(t *testing.T) {
	store, err := NewFilesystemStorage(t.TempDir())
	if err != nil {
		t.Fatalf("NewFilesystemStorage: %v", err)
	}

	profile := newTestProfile("abc123", "GET", "/api/users", 200, time.Now())
	profile.CollectorData = map[string]any{
		"request": map[string]any{"host": "localhost"},
		"timing":  map[string]any{"duration_ms": 42.5},
	}

	// Store
	if err := store.Store(profile); err != nil {
		t.Fatalf("Store: %v", err)
	}

	// Load
	loaded, err := store.Load("abc123")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.ID != profile.ID {
		t.Errorf("ID: got %q, want %q", loaded.ID, profile.ID)
	}
	if loaded.Method != profile.Method {
		t.Errorf("Method: got %q, want %q", loaded.Method, profile.Method)
	}
	if loaded.URL != profile.URL {
		t.Errorf("URL: got %q, want %q", loaded.URL, profile.URL)
	}
	if loaded.StatusCode != profile.StatusCode {
		t.Errorf("StatusCode: got %d, want %d", loaded.StatusCode, profile.StatusCode)
	}
	if loaded.Duration != profile.Duration {
		t.Errorf("Duration: got %v, want %v", loaded.Duration, profile.Duration)
	}
	if loaded.CollectorData == nil {
		t.Fatal("CollectorData is nil")
	}
}

func TestFilesystemStorageLoadNotFound(t *testing.T) {
	store, _ := NewFilesystemStorage(t.TempDir())

	_, err := store.Load("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent profile")
	}
}

func TestFilesystemStorageLoadInvalidID(t *testing.T) {
	store, _ := NewFilesystemStorage(t.TempDir())

	_, err := store.Load("../etc/passwd")
	if err == nil {
		t.Fatal("expected error for path traversal ID")
	}
}

func TestFilesystemStorageLoadEmptyID(t *testing.T) {
	store, _ := NewFilesystemStorage(t.TempDir())

	_, err := store.Load("")
	if err == nil {
		t.Fatal("expected error for empty ID")
	}
}

func TestFilesystemStorageStoreNilProfile(t *testing.T) {
	store, _ := NewFilesystemStorage(t.TempDir())

	err := store.Store(nil)
	if err == nil {
		t.Fatal("expected error for nil profile")
	}
}

func TestFilesystemStorageStoreEmptyID(t *testing.T) {
	store, _ := NewFilesystemStorage(t.TempDir())

	err := store.Store(&profiler.Profile{})
	if err == nil {
		t.Fatal("expected error for empty ID")
	}
}

func TestFilesystemStorageList(t *testing.T) {
	store, _ := NewFilesystemStorage(t.TempDir())

	now := time.Now()
	profiles := []*profiler.Profile{
		newTestProfile("p1", "GET", "/api/users", 200, now.Add(-3*time.Second)),
		newTestProfile("p2", "POST", "/api/orders", 201, now.Add(-2*time.Second)),
		newTestProfile("p3", "GET", "/api/products", 200, now.Add(-1*time.Second)),
		newTestProfile("p4", "DELETE", "/api/users/1", 204, now),
		newTestProfile("p5", "GET", "/api/users", 500, now.Add(1*time.Second)),
	}

	for _, p := range profiles {
		if err := store.Store(p); err != nil {
			t.Fatalf("Store: %v", err)
		}
	}

	t.Run("list all", func(t *testing.T) {
		summaries, err := store.List(profiler.SearchCriteria{})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(summaries) != 5 {
			t.Fatalf("expected 5 profiles, got %d", len(summaries))
		}
		// Should be newest first
		if summaries[0].ID != "p5" {
			t.Errorf("first profile should be p5 (newest), got %q", summaries[0].ID)
		}
		if summaries[4].ID != "p1" {
			t.Errorf("last profile should be p1 (oldest), got %q", summaries[4].ID)
		}
	})

	t.Run("filter by method", func(t *testing.T) {
		summaries, err := store.List(profiler.SearchCriteria{Method: "GET"})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(summaries) != 3 {
			t.Fatalf("expected 3 GET profiles, got %d", len(summaries))
		}
		for _, s := range summaries {
			if s.Method != "GET" {
				t.Errorf("expected GET method, got %q", s.Method)
			}
		}
	})

	t.Run("filter by URL substring", func(t *testing.T) {
		summaries, err := store.List(profiler.SearchCriteria{URL: "/api/users"})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(summaries) != 3 {
			t.Fatalf("expected 3 profiles matching /api/users, got %d", len(summaries))
		}
	})

	t.Run("filter by status code", func(t *testing.T) {
		summaries, err := store.List(profiler.SearchCriteria{StatusCode: 200})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(summaries) != 2 {
			t.Fatalf("expected 2 profiles with status 200, got %d", len(summaries))
		}
	})

	t.Run("filter by min status code", func(t *testing.T) {
		summaries, err := store.List(profiler.SearchCriteria{MinStatusCode: 400})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(summaries) != 1 {
			t.Fatalf("expected 1 profile with status >= 400, got %d", len(summaries))
		}
		if summaries[0].StatusCode != 500 {
			t.Errorf("expected status 500, got %d", summaries[0].StatusCode)
		}
	})

	t.Run("filter by time range", func(t *testing.T) {
		summaries, err := store.List(profiler.SearchCriteria{
			Since: now.Add(-2500 * time.Millisecond),
			Until: now.Add(500 * time.Millisecond),
		})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(summaries) != 3 {
			t.Fatalf("expected 3 profiles in time range, got %d", len(summaries))
		}
	})

	t.Run("pagination with limit", func(t *testing.T) {
		summaries, err := store.List(profiler.SearchCriteria{Limit: 2})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(summaries) != 2 {
			t.Fatalf("expected 2 profiles with limit=2, got %d", len(summaries))
		}
	})

	t.Run("pagination with offset", func(t *testing.T) {
		summaries, err := store.List(profiler.SearchCriteria{Limit: 2, Offset: 2})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(summaries) != 2 {
			t.Fatalf("expected 2 profiles with offset=2 limit=2, got %d", len(summaries))
		}
		if summaries[0].ID != "p3" {
			t.Errorf("expected p3 at offset 2, got %q", summaries[0].ID)
		}
	})

	t.Run("offset beyond total", func(t *testing.T) {
		summaries, err := store.List(profiler.SearchCriteria{Offset: 100})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(summaries) != 0 {
			t.Fatalf("expected 0 profiles with large offset, got %d", len(summaries))
		}
	})
}

func TestFilesystemStoragePurge(t *testing.T) {
	store, _ := NewFilesystemStorage(t.TempDir())

	now := time.Now()
	profiles := []*profiler.Profile{
		newTestProfile("old1", "GET", "/old", 200, now.Add(-48*time.Hour)),
		newTestProfile("old2", "GET", "/old", 200, now.Add(-25*time.Hour)),
		newTestProfile("recent1", "GET", "/recent", 200, now.Add(-1*time.Hour)),
		newTestProfile("recent2", "GET", "/recent", 200, now),
	}

	for _, p := range profiles {
		store.Store(p)
	}

	// Purge profiles older than 24 hours
	removed, err := store.Purge(24 * time.Hour)
	if err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if removed != 2 {
		t.Errorf("expected 2 removed, got %d", removed)
	}

	// Verify remaining
	summaries, _ := store.List(profiler.SearchCriteria{})
	if len(summaries) != 2 {
		t.Fatalf("expected 2 remaining profiles, got %d", len(summaries))
	}

	// Old profiles should be gone
	_, err = store.Load("old1")
	if err == nil {
		t.Error("expected old1 to be purged")
	}
	_, err = store.Load("old2")
	if err == nil {
		t.Error("expected old2 to be purged")
	}

	// Recent profiles should still exist
	_, err = store.Load("recent1")
	if err != nil {
		t.Error("expected recent1 to still exist")
	}
	_, err = store.Load("recent2")
	if err != nil {
		t.Error("expected recent2 to still exist")
	}
}

func TestFilesystemStorageOverwrite(t *testing.T) {
	store, _ := NewFilesystemStorage(t.TempDir())

	profile := newTestProfile("overwrite-test", "GET", "/v1", 200, time.Now())
	store.Store(profile)

	// Overwrite with different data
	profile.URL = "/v2"
	profile.StatusCode = 404
	store.Store(profile)

	loaded, err := store.Load("overwrite-test")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.URL != "/v2" {
		t.Errorf("URL: got %q, want /v2", loaded.URL)
	}
	if loaded.StatusCode != 404 {
		t.Errorf("StatusCode: got %d, want 404", loaded.StatusCode)
	}
}

func TestFilesystemStorageConcurrentAccess(t *testing.T) {
	store, _ := NewFilesystemStorage(t.TempDir())

	var wg sync.WaitGroup
	errs := make(chan error, 50)

	// Concurrent writes
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			p := newTestProfile(
				fmt.Sprintf("concurrent-%03d", idx),
				"GET",
				fmt.Sprintf("/api/%d", idx),
				200,
				time.Now(),
			)
			if err := store.Store(p); err != nil {
				errs <- err
			}
		}(i)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent store error: %v", err)
	}

	// Verify all were stored
	summaries, err := store.List(profiler.SearchCriteria{Limit: 100})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(summaries) != 50 {
		t.Errorf("expected 50 profiles, got %d", len(summaries))
	}
}

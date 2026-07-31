package storage

import (
	"fmt"
	"sync"
	"testing"
	"time"

	profiler "github.com/piotrkardasz/go-profiler"
)

func TestMemoryStorageStoreAndLoad(t *testing.T) {
	store := NewMemoryStorage(100)

	profile := newTestProfile("mem1", "GET", "/api/users", 200, time.Now())

	if err := store.Store(profile); err != nil {
		t.Fatalf("Store: %v", err)
	}

	loaded, err := store.Load("mem1")
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
}

func TestMemoryStorageLoadNotFound(t *testing.T) {
	store := NewMemoryStorage(100)

	_, err := store.Load("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent profile")
	}
}

func TestMemoryStorageLoadEmptyID(t *testing.T) {
	store := NewMemoryStorage(100)

	_, err := store.Load("")
	if err == nil {
		t.Fatal("expected error for empty ID")
	}
}

func TestMemoryStorageStoreNilProfile(t *testing.T) {
	store := NewMemoryStorage(100)

	err := store.Store(nil)
	if err == nil {
		t.Fatal("expected error for nil profile")
	}
}

func TestMemoryStorageStoreEmptyID(t *testing.T) {
	store := NewMemoryStorage(100)

	err := store.Store(&profiler.Profile{})
	if err == nil {
		t.Fatal("expected error for empty ID")
	}
}

func TestMemoryStorageLRUEviction(t *testing.T) {
	store := NewMemoryStorage(3) // Max 3 entries

	now := time.Now()
	store.Store(newTestProfile("p1", "GET", "/1", 200, now))
	store.Store(newTestProfile("p2", "GET", "/2", 200, now.Add(time.Second)))
	store.Store(newTestProfile("p3", "GET", "/3", 200, now.Add(2*time.Second)))

	if store.Len() != 3 {
		t.Fatalf("expected 3 profiles, got %d", store.Len())
	}

	// Adding a 4th should evict p1 (least recently used)
	store.Store(newTestProfile("p4", "GET", "/4", 200, now.Add(3*time.Second)))

	if store.Len() != 3 {
		t.Fatalf("expected 3 profiles after eviction, got %d", store.Len())
	}

	// p1 should be evicted
	_, err := store.Load("p1")
	if err == nil {
		t.Error("expected p1 to be evicted")
	}

	// p2, p3, p4 should still be accessible
	if _, err := store.Load("p2"); err != nil {
		t.Error("expected p2 to still exist")
	}
	if _, err := store.Load("p3"); err != nil {
		t.Error("expected p3 to still exist")
	}
	if _, err := store.Load("p4"); err != nil {
		t.Error("expected p4 to still exist")
	}
}

func TestMemoryStorageLRUAccessOrder(t *testing.T) {
	store := NewMemoryStorage(3)

	now := time.Now()
	store.Store(newTestProfile("p1", "GET", "/1", 200, now))
	store.Store(newTestProfile("p2", "GET", "/2", 200, now.Add(time.Second)))
	store.Store(newTestProfile("p3", "GET", "/3", 200, now.Add(2*time.Second)))

	// Access p1 to move it to front (most recently used)
	store.Load("p1")

	// Adding p4 should now evict p2 (least recently used)
	store.Store(newTestProfile("p4", "GET", "/4", 200, now.Add(3*time.Second)))

	// p2 should be evicted
	_, err := store.Load("p2")
	if err == nil {
		t.Error("expected p2 to be evicted (LRU after p1 was accessed)")
	}

	// p1 should still exist (was recently accessed)
	if _, err := store.Load("p1"); err != nil {
		t.Error("expected p1 to still exist (recently accessed)")
	}
}

func TestMemoryStorageOverwrite(t *testing.T) {
	store := NewMemoryStorage(100)

	profile := newTestProfile("overwrite", "GET", "/v1", 200, time.Now())
	store.Store(profile)

	// Overwrite
	updated := newTestProfile("overwrite", "POST", "/v2", 201, time.Now())
	store.Store(updated)

	// Should still have 1 entry
	if store.Len() != 1 {
		t.Fatalf("expected 1 profile after overwrite, got %d", store.Len())
	}

	loaded, _ := store.Load("overwrite")
	if loaded.Method != "POST" {
		t.Errorf("Method: got %q, want POST", loaded.Method)
	}
	if loaded.URL != "/v2" {
		t.Errorf("URL: got %q, want /v2", loaded.URL)
	}
}

func TestMemoryStorageList(t *testing.T) {
	store := NewMemoryStorage(100)

	now := time.Now()
	store.Store(newTestProfile("p1", "GET", "/api/users", 200, now.Add(-3*time.Second)))
	store.Store(newTestProfile("p2", "POST", "/api/orders", 201, now.Add(-2*time.Second)))
	store.Store(newTestProfile("p3", "GET", "/api/products", 200, now.Add(-1*time.Second)))
	store.Store(newTestProfile("p4", "DELETE", "/api/users/1", 204, now))
	store.Store(newTestProfile("p5", "GET", "/api/users", 500, now.Add(1*time.Second)))

	t.Run("list all", func(t *testing.T) {
		summaries, err := store.List(profiler.SearchCriteria{})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(summaries) != 5 {
			t.Fatalf("expected 5 profiles, got %d", len(summaries))
		}
		// Newest first
		if summaries[0].ID != "p5" {
			t.Errorf("first should be p5, got %q", summaries[0].ID)
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
	})

	t.Run("filter by URL", func(t *testing.T) {
		summaries, err := store.List(profiler.SearchCriteria{URL: "/api/users"})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(summaries) != 3 {
			t.Fatalf("expected 3 profiles matching /api/users, got %d", len(summaries))
		}
	})

	t.Run("filter by status", func(t *testing.T) {
		summaries, err := store.List(profiler.SearchCriteria{MinStatusCode: 400})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(summaries) != 1 {
			t.Fatalf("expected 1 error profile, got %d", len(summaries))
		}
	})

	t.Run("pagination", func(t *testing.T) {
		summaries, err := store.List(profiler.SearchCriteria{Limit: 2, Offset: 1})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(summaries) != 2 {
			t.Fatalf("expected 2 profiles, got %d", len(summaries))
		}
	})
}

func TestMemoryStoragePurge(t *testing.T) {
	store := NewMemoryStorage(100)

	now := time.Now()
	store.Store(newTestProfile("old1", "GET", "/old", 200, now.Add(-48*time.Hour)))
	store.Store(newTestProfile("old2", "GET", "/old", 200, now.Add(-25*time.Hour)))
	store.Store(newTestProfile("recent", "GET", "/new", 200, now))

	removed, err := store.Purge(24 * time.Hour)
	if err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if removed != 2 {
		t.Errorf("expected 2 removed, got %d", removed)
	}

	if store.Len() != 1 {
		t.Errorf("expected 1 remaining, got %d", store.Len())
	}

	if _, err := store.Load("recent"); err != nil {
		t.Error("recent profile should still exist")
	}
}

func TestMemoryStorageConcurrentAccess(t *testing.T) {
	store := NewMemoryStorage(200)

	var wg sync.WaitGroup
	errs := make(chan error, 100)

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

	// Concurrent reads
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			store.List(profiler.SearchCriteria{Limit: 10})
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent error: %v", err)
	}

	if store.Len() != 50 {
		t.Errorf("expected 50 profiles, got %d", store.Len())
	}
}

func TestMemoryStorageDefaultMaxEntries(t *testing.T) {
	// Zero or negative should use default
	store := NewMemoryStorage(0)
	if store.maxEntries != DefaultMemoryMaxEntries {
		t.Errorf("expected default max entries %d, got %d", DefaultMemoryMaxEntries, store.maxEntries)
	}

	store = NewMemoryStorage(-5)
	if store.maxEntries != DefaultMemoryMaxEntries {
		t.Errorf("expected default max entries %d, got %d", DefaultMemoryMaxEntries, store.maxEntries)
	}
}

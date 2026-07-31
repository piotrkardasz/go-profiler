package storage

import (
	"container/list"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	profiler "github.com/piotrkardasz/go-profiler"
)

const (
	// DefaultMemoryMaxEntries is the default maximum number of profiles kept in memory.
	DefaultMemoryMaxEntries = 200
)

// MemoryStorage stores profiles in memory with LRU eviction.
// It implements the profiler.Storage interface and is primarily intended
// for testing and ephemeral environments.
type MemoryStorage struct {
	mu         sync.RWMutex
	maxEntries int
	profiles   map[string]*list.Element
	order      *list.List // front = most recently accessed
}

// memoryEntry wraps a profile in the LRU list.
type memoryEntry struct {
	profile *profiler.Profile
}

// NewMemoryStorage creates a new in-memory storage with the given maximum
// number of entries. When the limit is reached, the least recently used
// profile is evicted.
func NewMemoryStorage(maxEntries int) *MemoryStorage {
	if maxEntries <= 0 {
		maxEntries = DefaultMemoryMaxEntries
	}
	return &MemoryStorage{
		maxEntries: maxEntries,
		profiles:   make(map[string]*list.Element),
		order:      list.New(),
	}
}

// Store adds or updates a profile in memory. If the maximum number of entries
// is exceeded, the least recently used profile is evicted.
func (ms *MemoryStorage) Store(profile *profiler.Profile) error {
	if profile == nil {
		return fmt.Errorf("profiler/storage: cannot store nil profile")
	}
	if profile.ID == "" {
		return fmt.Errorf("profiler/storage: profile has empty ID")
	}

	ms.mu.Lock()
	defer ms.mu.Unlock()

	// If profile already exists, update it and move to front
	if elem, ok := ms.profiles[profile.ID]; ok {
		elem.Value.(*memoryEntry).profile = profile
		ms.order.MoveToFront(elem)
		return nil
	}

	// Add new entry at front
	entry := &memoryEntry{profile: profile}
	elem := ms.order.PushFront(entry)
	ms.profiles[profile.ID] = elem

	// Evict LRU if over capacity
	for ms.order.Len() > ms.maxEntries {
		ms.evictOldest()
	}

	return nil
}

// Load retrieves a profile by ID and marks it as recently accessed.
func (ms *MemoryStorage) Load(id string) (*profiler.Profile, error) {
	if id == "" {
		return nil, fmt.Errorf("profiler/storage: empty profile ID")
	}

	ms.mu.Lock()
	defer ms.mu.Unlock()

	elem, ok := ms.profiles[id]
	if !ok {
		return nil, fmt.Errorf("profiler/storage: profile %q not found", id)
	}

	// Move to front (most recently accessed)
	ms.order.MoveToFront(elem)
	return elem.Value.(*memoryEntry).profile, nil
}

// List returns profile summaries matching the given criteria, ordered by
// timestamp descending (newest first).
func (ms *MemoryStorage) List(criteria profiler.SearchCriteria) ([]*profiler.ProfileSummary, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	// Collect all profiles
	all := make([]*profiler.Profile, 0, len(ms.profiles))
	for elem := ms.order.Front(); elem != nil; elem = elem.Next() {
		all = append(all, elem.Value.(*memoryEntry).profile)
	}

	// Sort by timestamp descending
	sort.Slice(all, func(i, j int) bool {
		return all[i].Timestamp.After(all[j].Timestamp)
	})

	// Apply filters
	filtered := ms.applyFilters(all, criteria)

	// Apply pagination
	limit := criteria.Limit
	if limit <= 0 {
		limit = 100
	}
	offset := criteria.Offset
	if offset < 0 {
		offset = 0
	}

	if offset >= len(filtered) {
		return []*profiler.ProfileSummary{}, nil
	}

	end := offset + limit
	if end > len(filtered) {
		end = len(filtered)
	}

	// Convert to summaries
	summaries := make([]*profiler.ProfileSummary, 0, end-offset)
	for _, p := range filtered[offset:end] {
		summaries = append(summaries, p.Summary())
	}

	return summaries, nil
}

// Purge removes profiles older than maxAge. Returns the number of profiles removed.
func (ms *MemoryStorage) Purge(maxAge time.Duration) (int, error) {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	removed := 0

	// Iterate from back (oldest by access time) but check by timestamp
	var next *list.Element
	for elem := ms.order.Back(); elem != nil; elem = next {
		next = elem.Prev()
		entry := elem.Value.(*memoryEntry)
		if entry.profile.Timestamp.Before(cutoff) {
			delete(ms.profiles, entry.profile.ID)
			ms.order.Remove(elem)
			removed++
		}
	}

	return removed, nil
}

// Clear removes all profiles from memory.
func (ms *MemoryStorage) Clear() error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	ms.profiles = make(map[string]*list.Element)
	ms.order.Init()

	return nil
}

// Len returns the current number of profiles stored.
func (ms *MemoryStorage) Len() int {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	return len(ms.profiles)
}

// evictOldest removes the least recently used profile. Must be called with mu held.
func (ms *MemoryStorage) evictOldest() {
	elem := ms.order.Back()
	if elem == nil {
		return
	}
	entry := elem.Value.(*memoryEntry)
	delete(ms.profiles, entry.profile.ID)
	ms.order.Remove(elem)
}

// applyFilters filters profiles based on SearchCriteria.
func (ms *MemoryStorage) applyFilters(profiles []*profiler.Profile, criteria profiler.SearchCriteria) []*profiler.Profile {
	result := make([]*profiler.Profile, 0, len(profiles))

	for _, p := range profiles {
		if criteria.Method != "" && !strings.EqualFold(p.Method, criteria.Method) {
			continue
		}
		if criteria.URL != "" && !strings.Contains(p.URL, criteria.URL) {
			continue
		}
		if criteria.StatusCode != 0 && p.StatusCode != criteria.StatusCode {
			continue
		}
		if criteria.MinStatusCode != 0 && p.StatusCode < criteria.MinStatusCode {
			continue
		}
		if criteria.MaxStatusCode != 0 && p.StatusCode > criteria.MaxStatusCode {
			continue
		}
		if !criteria.Since.IsZero() && p.Timestamp.Before(criteria.Since) {
			continue
		}
		if !criteria.Until.IsZero() && p.Timestamp.After(criteria.Until) {
			continue
		}
		result = append(result, p)
	}

	return result
}

package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	profiler "github.com/piotrkardasz/go-profiler"
)

const (
	// DefaultMaxProfiles is the default maximum number of profiles to retain.
	DefaultMaxProfiles = 1000

	// filePermission is the permission mode for profile files.
	filePermission = 0644

	// dirPermission is the permission mode for the storage directory.
	dirPermission = 0755
)

// FilesystemStorage stores profiles as individual JSON files in a directory.
// It implements the profiler.Storage interface.
type FilesystemStorage struct {
	mu  sync.RWMutex
	dir string
}

// NewFilesystemStorage creates a new file-based storage that persists profiles
// in the given directory. The directory is created if it does not exist.
func NewFilesystemStorage(dir string) (*FilesystemStorage, error) {
	if err := os.MkdirAll(dir, dirPermission); err != nil {
		return nil, fmt.Errorf("profiler/storage: failed to create directory %q: %w", dir, err)
	}
	return &FilesystemStorage{dir: dir}, nil
}

// Store persists a profile as a JSON file named {id}.json.
// The write is atomic: data is written to a temporary file and then renamed.
func (fs *FilesystemStorage) Store(profile *profiler.Profile) error {
	if profile == nil {
		return fmt.Errorf("profiler/storage: cannot store nil profile")
	}
	if profile.ID == "" {
		return fmt.Errorf("profiler/storage: profile has empty ID")
	}

	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return fmt.Errorf("profiler/storage: failed to marshal profile %q: %w", profile.ID, err)
	}

	targetPath := fs.profilePath(profile.ID)

	// Atomic write: write to temp file then rename
	tmpFile, err := os.CreateTemp(fs.dir, ".profile-*.tmp")
	if err != nil {
		return fmt.Errorf("profiler/storage: failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("profiler/storage: failed to write temp file: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("profiler/storage: failed to close temp file: %w", err)
	}

	if err := os.Chmod(tmpPath, filePermission); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("profiler/storage: failed to chmod temp file: %w", err)
	}

	fs.mu.Lock()
	defer fs.mu.Unlock()

	if err := os.Rename(tmpPath, targetPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("profiler/storage: failed to rename temp file to %q: %w", targetPath, err)
	}

	return nil
}

// Load retrieves a profile by its ID from the filesystem.
func (fs *FilesystemStorage) Load(id string) (*profiler.Profile, error) {
	if id == "" {
		return nil, fmt.Errorf("profiler/storage: empty profile ID")
	}

	// Sanitize ID to prevent path traversal
	if strings.ContainsAny(id, "/\\..") {
		return nil, fmt.Errorf("profiler/storage: invalid profile ID %q", id)
	}

	fs.mu.RLock()
	defer fs.mu.RUnlock()

	path := fs.profilePath(id)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("profiler/storage: profile %q not found", id)
		}
		return nil, fmt.Errorf("profiler/storage: failed to read profile %q: %w", id, err)
	}

	var profile profiler.Profile
	if err := json.Unmarshal(data, &profile); err != nil {
		return nil, fmt.Errorf("profiler/storage: failed to unmarshal profile %q: %w", id, err)
	}

	return &profile, nil
}

// List returns profile summaries matching the given criteria, ordered by
// timestamp descending (newest first).
func (fs *FilesystemStorage) List(criteria profiler.SearchCriteria) ([]*profiler.ProfileSummary, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	entries, err := os.ReadDir(fs.dir)
	if err != nil {
		return nil, fmt.Errorf("profiler/storage: failed to read directory: %w", err)
	}

	var profiles []*profiler.Profile
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		// Skip temp files
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		path := filepath.Join(fs.dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue // Skip unreadable files
		}

		var profile profiler.Profile
		if err := json.Unmarshal(data, &profile); err != nil {
			continue // Skip corrupt files
		}

		profiles = append(profiles, &profile)
	}

	// Sort by timestamp descending (newest first)
	sort.Slice(profiles, func(i, j int) bool {
		return profiles[i].Timestamp.After(profiles[j].Timestamp)
	})

	// Apply filters
	filtered := fs.applyFilters(profiles, criteria)

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
func (fs *FilesystemStorage) Purge(maxAge time.Duration) (int, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	removed := 0

	entries, err := os.ReadDir(fs.dir)
	if err != nil {
		return 0, fmt.Errorf("profiler/storage: failed to read directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		path := filepath.Join(fs.dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var profile profiler.Profile
		if err := json.Unmarshal(data, &profile); err != nil {
			continue
		}

		if profile.Timestamp.Before(cutoff) {
			if err := os.Remove(path); err == nil {
				removed++
			}
		}
	}

	return removed, nil
}

// Clear removes all profile JSON files from the storage directory.
// The directory itself is preserved.
func (fs *FilesystemStorage) Clear() error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	entries, err := os.ReadDir(fs.dir)
	if err != nil {
		return fmt.Errorf("profiler/storage: failed to read directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		path := filepath.Join(fs.dir, entry.Name())
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("profiler/storage: failed to remove %q: %w", entry.Name(), err)
		}
	}

	return nil
}

// profilePath returns the filesystem path for a profile with the given ID.
func (fs *FilesystemStorage) profilePath(id string) string {
	return filepath.Join(fs.dir, id+".json")
}

// applyFilters filters profiles based on SearchCriteria.
func (fs *FilesystemStorage) applyFilters(profiles []*profiler.Profile, criteria profiler.SearchCriteria) []*profiler.Profile {
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

// Package profiler provides a framework-agnostic HTTP profiling middleware
// inspired by Symfony's profiler. It collects per-request profiling data via
// pluggable collectors, stores profiles, and exposes them through a JSON API
// and web UI.
package profiler

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// SearchCriteria defines filters for listing profiles.
type SearchCriteria struct {
	// Method filters by HTTP method (e.g., "GET", "POST"). Empty means all.
	Method string

	// URL filters profiles whose URL contains this substring. Empty means all.
	URL string

	// StatusCode filters by exact status code. Zero means all.
	StatusCode int

	// MinStatusCode filters profiles with status >= this value. Zero means no minimum.
	MinStatusCode int

	// MaxStatusCode filters profiles with status <= this value. Zero means no maximum.
	MaxStatusCode int

	// Since filters profiles created after this time. Zero value means no lower bound.
	Since time.Time

	// Until filters profiles created before this time. Zero value means no upper bound.
	Until time.Time

	// Limit is the maximum number of results to return. Zero means default (100).
	Limit int

	// Offset is the number of results to skip for pagination.
	Offset int
}

// Storage defines the interface for profile persistence.
// Implementations must be safe for concurrent use.
type Storage interface {
	// Store persists a profile. If a profile with the same ID already exists,
	// it is overwritten.
	Store(profile *Profile) error

	// Load retrieves a profile by its ID. Returns an error if the profile
	// is not found.
	Load(id string) (*Profile, error)

	// List returns profile summaries matching the given criteria, ordered by
	// timestamp descending (newest first).
	List(criteria SearchCriteria) ([]*ProfileSummary, error)

	// Purge removes profiles older than maxAge. Returns the number of profiles
	// removed.
	Purge(maxAge time.Duration) (int, error)

	// Clear removes all stored profiles. Returns nil on success, even if
	// there are no profiles to remove.
	Clear() error
}

// Profile represents a single profiled HTTP request with all collected data.
type Profile struct {
	// ID is a unique identifier for this profile (hex token).
	ID string `json:"id"`

	// Method is the HTTP method of the profiled request.
	Method string `json:"method"`

	// URL is the request URL that was profiled.
	URL string `json:"url"`

	// StatusCode is the HTTP response status code.
	StatusCode int `json:"status_code"`

	// Timestamp is when the request was received.
	Timestamp time.Time `json:"timestamp"`

	// Duration is the total time spent handling the request.
	Duration time.Duration `json:"duration"`

	// CollectorData holds the data collected by each registered collector,
	// keyed by the collector's Name().
	CollectorData map[string]any `json:"collector_data"`
}

// ProfileSummary is a lightweight representation of a profile used in listings.
type ProfileSummary struct {
	ID         string        `json:"id"`
	Method     string        `json:"method"`
	URL        string        `json:"url"`
	StatusCode int           `json:"status_code"`
	Timestamp  time.Time     `json:"timestamp"`
	Duration   time.Duration `json:"duration"`
}

// MarshalJSON implements custom JSON marshaling for Profile to handle Duration
// as milliseconds for JSON compatibility.
func (p *Profile) MarshalJSON() ([]byte, error) {
	type Alias Profile
	return json.Marshal(&struct {
		*Alias
		Duration float64 `json:"duration"`
	}{
		Alias:    (*Alias)(p),
		Duration: float64(p.Duration.Milliseconds()),
	})
}

// UnmarshalJSON implements custom JSON unmarshaling for Profile to handle
// Duration from milliseconds.
func (p *Profile) UnmarshalJSON(data []byte) error {
	type Alias Profile
	aux := &struct {
		*Alias
		Duration float64 `json:"duration"`
	}{
		Alias: (*Alias)(p),
	}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	p.Duration = time.Duration(aux.Duration) * time.Millisecond
	return nil
}

// MarshalJSON implements custom JSON marshaling for ProfileSummary.
func (ps *ProfileSummary) MarshalJSON() ([]byte, error) {
	type Alias ProfileSummary
	return json.Marshal(&struct {
		*Alias
		Duration float64 `json:"duration"`
	}{
		Alias:    (*Alias)(ps),
		Duration: float64(ps.Duration.Milliseconds()),
	})
}

// UnmarshalJSON implements custom JSON unmarshaling for ProfileSummary.
func (ps *ProfileSummary) UnmarshalJSON(data []byte) error {
	type Alias ProfileSummary
	aux := &struct {
		*Alias
		Duration float64 `json:"duration"`
	}{
		Alias: (*Alias)(ps),
	}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	ps.Duration = time.Duration(aux.Duration) * time.Millisecond
	return nil
}

// Summary returns a ProfileSummary from this profile.
func (p *Profile) Summary() *ProfileSummary {
	return &ProfileSummary{
		ID:         p.ID,
		Method:     p.Method,
		URL:        p.URL,
		StatusCode: p.StatusCode,
		Timestamp:  p.Timestamp,
		Duration:   p.Duration,
	}
}

// GenerateProfileID generates a unique profile identifier using crypto/rand.
// It produces a 16-character hex string (8 random bytes).
func GenerateProfileID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("profiler: failed to generate profile ID: %w", err)
	}
	return hex.EncodeToString(b), nil
}

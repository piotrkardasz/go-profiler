// Package handler provides HTTP handlers for the profiler's JSON API and UI.
package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	profiler "github.com/piotrkardasz/go-profiler"
)

// APIHandler provides REST API endpoints for accessing profiler data.
type APIHandler struct {
	profiler *profiler.Profiler
}

// NewAPIHandler creates a new API handler for the given profiler instance.
func NewAPIHandler(p *profiler.Profiler) *APIHandler {
	return &APIHandler{profiler: p}
}

// RegisterRoutes registers the API routes on the given mux under the specified prefix.
// Routes registered:
//   - GET    {prefix}/api/profiles       — list profiles
//   - GET    {prefix}/api/profiles/{id}   — get profile by ID
//   - DELETE {prefix}/api/profiles        — purge profiles
//   - DELETE {prefix}/api/profiles/all    — clear all profiles
//   - GET    {prefix}/api/collectors      — list registered collectors
func (h *APIHandler) RegisterRoutes(mux *http.ServeMux, prefix string) {
	prefix = strings.TrimRight(prefix, "/")

	mux.HandleFunc(prefix+"/api/profiles/all", h.handleClearAll)
	mux.HandleFunc(prefix+"/api/profiles", h.handleProfiles)
	mux.HandleFunc(prefix+"/api/profiles/", h.handleProfileByID)
	mux.HandleFunc(prefix+"/api/collectors", h.handleCollectors)
}

// handleProfiles handles GET (list) and DELETE (purge) for /api/profiles.
func (h *APIHandler) handleProfiles(w http.ResponseWriter, r *http.Request) {
	h.setCORSHeaders(w)

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.listProfiles(w, r)
	case http.MethodDelete:
		h.purgeProfiles(w, r)
	default:
		h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleProfileByID handles GET for /api/profiles/{id}.
func (h *APIHandler) handleProfileByID(w http.ResponseWriter, r *http.Request) {
	h.setCORSHeaders(w)

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if r.Method != http.MethodGet {
		h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Extract ID from path: {prefix}/api/profiles/{id}
	parts := strings.Split(r.URL.Path, "/")
	id := parts[len(parts)-1]
	if id == "" || id == "profiles" {
		h.writeError(w, http.StatusBadRequest, "profile ID required")
		return
	}

	storage := h.profiler.Storage()
	if storage == nil {
		h.writeError(w, http.StatusServiceUnavailable, "storage not configured")
		return
	}

	profile, err := storage.Load(id)
	if err != nil {
		h.writeError(w, http.StatusNotFound, "profile not found")
		return
	}

	h.writeJSON(w, http.StatusOK, profile)
}

// listProfiles returns profiles matching query parameters.
func (h *APIHandler) listProfiles(w http.ResponseWriter, r *http.Request) {
	storage := h.profiler.Storage()
	if storage == nil {
		h.writeError(w, http.StatusServiceUnavailable, "storage not configured")
		return
	}

	criteria := h.parseCriteria(r)

	summaries, err := storage.List(criteria)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "failed to list profiles")
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]any{
		"profiles": summaries,
		"total":    len(summaries),
	})
}

// purgeProfiles removes old profiles.
func (h *APIHandler) purgeProfiles(w http.ResponseWriter, r *http.Request) {
	storage := h.profiler.Storage()
	if storage == nil {
		h.writeError(w, http.StatusServiceUnavailable, "storage not configured")
		return
	}

	// Default: purge profiles older than 24 hours
	maxAge := 24 * time.Hour
	if ageStr := r.URL.Query().Get("max_age"); ageStr != "" {
		parsed, err := time.ParseDuration(ageStr)
		if err == nil && parsed > 0 {
			maxAge = parsed
		}
	}

	removed, err := storage.Purge(maxAge)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "failed to purge profiles")
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]any{
		"removed": removed,
		"max_age": maxAge.String(),
	})
}

// handleClearAll handles DELETE for /api/profiles/all — removes all profiles.
func (h *APIHandler) handleClearAll(w http.ResponseWriter, r *http.Request) {
	h.setCORSHeaders(w)

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if r.Method != http.MethodDelete {
		h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	storage := h.profiler.Storage()
	if storage == nil {
		h.writeError(w, http.StatusServiceUnavailable, "storage not configured")
		return
	}

	if err := storage.Clear(); err != nil {
		h.writeError(w, http.StatusInternalServerError, "failed to clear profiles")
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]any{
		"cleared": true,
	})
}

// handleCollectors returns metadata about registered collectors.
func (h *APIHandler) handleCollectors(w http.ResponseWriter, r *http.Request) {
	h.setCORSHeaders(w)

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if r.Method != http.MethodGet {
		h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	metas := h.profiler.PanelMetas()
	h.writeJSON(w, http.StatusOK, map[string]any{
		"collectors": metas,
	})
}

// parseCriteria extracts SearchCriteria from query parameters.
func (h *APIHandler) parseCriteria(r *http.Request) profiler.SearchCriteria {
	q := r.URL.Query()
	criteria := profiler.SearchCriteria{}

	criteria.Method = q.Get("method")
	criteria.URL = q.Get("url")

	if status := q.Get("status"); status != "" {
		if v, err := strconv.Atoi(status); err == nil {
			criteria.StatusCode = v
		}
	}
	if minStatus := q.Get("min_status"); minStatus != "" {
		if v, err := strconv.Atoi(minStatus); err == nil {
			criteria.MinStatusCode = v
		}
	}
	if maxStatus := q.Get("max_status"); maxStatus != "" {
		if v, err := strconv.Atoi(maxStatus); err == nil {
			criteria.MaxStatusCode = v
		}
	}
	if since := q.Get("since"); since != "" {
		if t, err := time.Parse(time.RFC3339, since); err == nil {
			criteria.Since = t
		}
	}
	if until := q.Get("until"); until != "" {
		if t, err := time.Parse(time.RFC3339, until); err == nil {
			criteria.Until = t
		}
	}
	if limit := q.Get("limit"); limit != "" {
		if v, err := strconv.Atoi(limit); err == nil && v > 0 {
			criteria.Limit = v
		}
	}
	if offset := q.Get("offset"); offset != "" {
		if v, err := strconv.Atoi(offset); err == nil && v >= 0 {
			criteria.Offset = v
		}
	}

	return criteria
}

// setCORSHeaders adds CORS headers to allow requests from the Vue dev server.
func (h *APIHandler) setCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept")
}

// writeJSON writes a JSON response with the given status code.
func (h *APIHandler) writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// writeError writes a JSON error response.
func (h *APIHandler) writeError(w http.ResponseWriter, status int, message string) {
	h.writeJSON(w, status, map[string]string{
		"error": message,
	})
}

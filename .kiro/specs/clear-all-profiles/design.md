# Design: Clear All Profiles

## Technical Design Document

### 1. Overview

This feature adds a `Clear()` method to the `Storage` interface and exposes it via a new API endpoint and a "Clear All" button in the Vue UI. The design follows existing patterns established in the codebase for storage operations, API handlers, and UI interactions.

### 2. Changes Diagram

```
┌─────────────────────────────────────────────────────────────┐
│  Vue UI (ProfileList.vue)                                    │
│  ┌──────────────────┐                                       │
│  │ [Clear All] btn  │ → confirm() → DELETE /api/profiles/all│
│  └──────────────────┘                                       │
├─────────────────────────────────────────────────────────────┤
│  API Layer (handler/api.go)                                  │
│  DELETE {prefix}/api/profiles/all → clearProfiles()          │
├─────────────────────────────────────────────────────────────┤
│  Storage Interface (profile.go)                              │
│  Clear() error  ← new method                                │
├─────────────────────────────────────────────────────────────┤
│  MemoryStorage         │  FilesystemStorage                  │
│  reset map + list      │  remove all *.json files            │
└─────────────────────────────────────────────────────────────┘
```

### 3. Storage Interface Change

#### 3.1 New Method

```go
// Storage interface in profile.go
type Storage interface {
    Store(profile *Profile) error
    Load(id string) (*Profile, error)
    List(criteria SearchCriteria) ([]*ProfileSummary, error)
    Purge(maxAge time.Duration) (int, error)
    Clear() error  // NEW
}
```

**Design decision:** `Clear()` returns only `error` (no count) because the caller doesn't need to know how many profiles existed — it's a destructive "reset everything" operation. This is simpler than `Purge()` which returns a count for reporting partial cleanup.

### 4. MemoryStorage Implementation

```go
// Clear removes all profiles from memory.
func (ms *MemoryStorage) Clear() error {
    ms.mu.Lock()
    defer ms.mu.Unlock()

    ms.profiles = make(map[string]*list.Element)
    ms.order.Init()

    return nil
}
```

**Design decisions:**
- Uses `ms.order.Init()` which resets the list to empty in O(1) — no iteration needed.
- Recreates the map to release all memory immediately (GC-friendly).
- Always returns `nil` — in-memory clear cannot fail.
- Holds write lock for the entire operation for thread safety.

### 5. FilesystemStorage Implementation

```go
// Clear removes all profile JSON files from the storage directory.
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
```

**Design decisions:**
- Only removes `.json` files (same filter as `List()` and `Purge()`).
- Skips dot-prefixed files (temp files from atomic writes).
- Skips subdirectories — only targets profile files.
- Fails fast on first error — partial clear is reported as an error (atomic intent).
- Does NOT remove the directory itself — keeps the storage location valid for future writes.
- Holds write lock for the entire operation for consistency.

### 6. API Endpoint

#### 6.1 Route Registration

The new endpoint is registered in `APIHandler.RegisterRoutes()`:

```go
mux.HandleFunc(prefix+"/api/profiles/all", h.handleClearAll)
```

**Design decision:** Using path `/api/profiles/all` with DELETE method rather than reusing the existing `DELETE /api/profiles` (which already handles purge). This avoids ambiguity and ensures the existing purge endpoint continues to work unchanged. The `/all` suffix makes the destructive intent explicit.

**Route precedence:** `{prefix}/api/profiles/all` must be registered BEFORE `{prefix}/api/profiles/` (the catch-all for profile-by-ID) to ensure proper routing. Go's `ServeMux` matches more specific patterns first when registered correctly.

#### 6.2 Handler

```go
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
```

**Design decisions:**
- Follows the same pattern as existing handlers (CORS, OPTIONS, method check, storage nil check).
- Returns `{"cleared": true}` for success — simple boolean confirmation.
- Returns 500 with error message if the storage operation fails.
- No request body or query params needed — it's an unconditional clear.

### 7. UI Changes

#### 7.1 API Client (`api.ts`)

Add a new function to the typed fetch client:

```typescript
export async function clearAllProfiles(): Promise<{ cleared: boolean }> {
    const response = await fetch(`${API_BASE}/api/profiles/all`, {
        method: 'DELETE',
    })
    return response.json()
}
```

#### 7.2 ProfileList.vue Button

The "Clear All" button is placed in the profile list toolbar, next to the existing purge action:

```html
<button
    class="btn btn-danger"
    :disabled="clearing"
    @click="handleClearAll"
>
    {{ clearing ? 'Clearing...' : 'Clear All' }}
</button>
```

#### 7.3 Interaction Flow

1. User clicks "Clear All"
2. Browser `confirm()` dialog: "Are you sure you want to delete all profiles? This cannot be undone."
3. If confirmed: set `clearing = true`, call `clearAllProfiles()` API
4. On success: refresh the profile list (will show empty), set `clearing = false`
5. On failure: show error state, set `clearing = false`

**Design decisions:**
- Uses native `confirm()` rather than a custom modal — keeps it simple, consistent with dev tooling UX.
- Button is disabled during the operation to prevent double-clicks.
- Text changes to "Clearing..." for loading feedback.
- Destructive styling (red/danger color) signals the irreversible action visually.
- After clear, the list auto-refreshes rather than requiring manual reload.

### 8. Route Ordering Consideration

Go's `http.ServeMux` (Go 1.22+) supports pattern matching with the most specific pattern winning. The route `{prefix}/api/profiles/all` is more specific than `{prefix}/api/profiles/` so it will be matched first regardless of registration order. However, for clarity and backwards compatibility with older Go versions, it should be registered before the trailing-slash pattern.

### 9. Testing Strategy

- **Unit tests for MemoryStorage.Clear():** verify empty after clear, verify operational after clear (can store again), verify clear on empty storage.
- **Unit tests for FilesystemStorage.Clear():** verify files removed, directory preserved, operational after clear.
- **API handler test:** verify DELETE /api/profiles/all returns `{"cleared": true}`, verify non-DELETE returns 405.
- **Integration:** existing tests continue to pass (interface change is additive in behavior).

### 10. Impact on Existing Code

| File | Change |
|------|--------|
| `profile.go` | Add `Clear() error` to `Storage` interface |
| `storage/memory.go` | Add `Clear()` method |
| `storage/filesystem.go` | Add `Clear()` method |
| `handler/api.go` | Add `handleClearAll` handler + route registration |
| `handler/api_test.go` | Add test for new endpoint |
| `storage/memory_test.go` | Add test for Clear() |
| `storage/filesystem_test.go` | Add test for Clear() |
| `collector/gorm/collector.go` | Add `Clear()` if it implements Storage (check needed) |

Any third-party code implementing the `Storage` interface will need to add a `Clear()` method. This is acceptable since the package is in active development and not yet at v1.

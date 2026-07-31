# Tasks: Clear All Profiles

## Implementation Tasks

### Task 1: Extend Storage interface and implement Clear() on MemoryStorage [DONE]

**Objective:** Add `Clear() error` to the `Storage` interface and implement it on `MemoryStorage`.

**Requirements addressed:** FR-1.1, FR-1.2, FR-1.3, FR-2.1, FR-2.4, NFR-1.1

**Implementation:**
- Add `Clear() error` method to the `Storage` interface in `profile.go`
- Implement `Clear()` on `MemoryStorage` in `storage/memory.go`:
  - Acquire write lock (`ms.mu.Lock()`)
  - Reset map: `ms.profiles = make(map[string]*list.Element)`
  - Reset list: `ms.order.Init()`
  - Return `nil`
- Add unit tests in `storage/memory_test.go`:
  - Test clear removes all profiles (store 3, clear, list returns empty)
  - Test clear on empty storage returns nil
  - Test storage is usable after clear (store after clear works)

**Files modified:**
- `profile.go`
- `storage/memory.go`
- `storage/memory_test.go`

---

### Task 2: Implement Clear() on FilesystemStorage [DONE]

**Objective:** Implement the `Clear()` method on `FilesystemStorage`.

**Requirements addressed:** FR-2.2, FR-2.3, FR-2.4, NFR-1.2

**Implementation:**
- Implement `Clear()` on `FilesystemStorage` in `storage/filesystem.go`:
  - Acquire write lock (`fs.mu.Lock()`)
  - Read directory entries with `os.ReadDir(fs.dir)`
  - Iterate entries, skip directories, skip non-`.json` files, skip dot-prefixed files
  - Remove each matching file with `os.Remove()`
  - Return error on first failure, nil on success
- Add unit tests in `storage/filesystem_test.go`:
  - Test clear removes all profile files
  - Test clear preserves the storage directory
  - Test clear on empty directory returns nil
  - Test storage is usable after clear (store after clear works)

**Files modified:**
- `storage/filesystem.go`
- `storage/filesystem_test.go`

---

### Task 3: Add Clear() to GORM collector storage (if applicable) [DONE]

**Objective:** Ensure any other Storage interface implementors in the codebase compile after the interface change.

**Requirements addressed:** NFR-3.1

**Implementation:**
- Check `collector/gorm/collector.go` for any type that implements the `Storage` interface
- If found, add a `Clear()` method that satisfies the interface
- If no other implementors exist, this task is a no-op (just verify compilation)
- Run `go build ./...` to confirm no compilation errors

**Files modified:**
- `collector/gorm/collector.go` (if needed)

---

### Task 4: Add API endpoint for clearing all profiles [DONE]

**Objective:** Expose `Clear()` via a `DELETE /_profiler/api/profiles/all` endpoint.

**Requirements addressed:** FR-3.1, FR-3.2, FR-3.3, FR-3.4, FR-3.5

**Implementation:**
- Add route registration in `APIHandler.RegisterRoutes()`:
  - `mux.HandleFunc(prefix+"/api/profiles/all", h.handleClearAll)` — registered BEFORE the `/api/profiles/` catch-all
- Implement `handleClearAll` handler:
  - Set CORS headers
  - Handle OPTIONS preflight (204 No Content)
  - Reject non-DELETE methods (405)
  - Check storage is configured (503)
  - Call `storage.Clear()`
  - Return `{"cleared": true}` on success (200)
  - Return `{"error": "failed to clear profiles"}` on failure (500)
- Add unit test in `handler/api_test.go`:
  - Test DELETE returns 200 with `{"cleared": true}`
  - Test GET returns 405
  - Test OPTIONS returns 204

**Files modified:**
- `handler/api.go`
- `handler/api_test.go`

---

### Task 5: Add clearAllProfiles to the Vue API client [DONE]

**Objective:** Add the typed fetch function for the new endpoint.

**Requirements addressed:** FR-4.1 (prerequisite)

**Implementation:**
- Add `clearAllProfiles()` function to the API client (likely in `ui/src/api.ts` or embedded in the Vue build):
  ```typescript
  export async function clearAllProfiles(): Promise<{ cleared: boolean }> {
      const response = await fetch(`${API_BASE}/api/profiles/all`, {
          method: 'DELETE',
      })
      return response.json()
  }
  ```
- Since the UI is pre-built and embedded (no `ui/` source directory in workspace), this will be implemented directly in the embedded JS or the Vue source if available

**Files modified:**
- UI source files (determine actual location)

---

### Task 6: Add "Clear All" button to ProfileList view [DONE]

**Objective:** Add the button to the UI with confirmation and loading state.

**Requirements addressed:** FR-4.1, FR-4.2, FR-4.3, FR-4.4, FR-4.5, NFR-2.1

**Implementation:**
- Add "Clear All" button to the profile list toolbar
- Button styling: red/danger color to indicate destructive action
- Add `clearing` reactive state variable
- `handleClearAll()` method:
  1. Show `confirm("Are you sure you want to delete all profiles? This cannot be undone.")`
  2. If cancelled, return early
  3. Set `clearing = true`
  4. Call `clearAllProfiles()` API
  5. On success: refresh profile list
  6. On error: show error (console or toast)
  7. Set `clearing = false`
- Button text: "Clear All" (idle) / "Clearing..." (loading)
- Button disabled when `clearing === true`

**Files modified:**
- UI source files (ProfileList.vue or equivalent in embedded build)

---

### Task 7: Build and verify [DONE]

**Objective:** Ensure everything compiles, tests pass, and the feature works end-to-end.

**Implementation:**
- Run `go build ./...` to verify compilation
- Run `go test ./...` to verify all tests pass (existing + new)
- Verify the new endpoint works with a manual curl test if possible:
  ```
  curl -X DELETE http://localhost:8080/_profiler/api/profiles/all
  ```

**Files modified:**
- None (verification only)

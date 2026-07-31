# Requirements: Clear All Profiles

## Overview

Add a "Clear All" button to the profiler UI and a corresponding backend API endpoint that allows users to delete all stored profiles at once. Currently the only removal mechanism is time-based purge (`Purge(maxAge)`), which cannot clear everything instantly. This feature provides a one-click way to reset the profiler state during development.

## Functional Requirements

### FR-1: Storage Interface Extension

- FR-1.1: The `Storage` interface MUST be extended with a `Clear() error` method that removes all stored profiles.
- FR-1.2: `Clear()` MUST be atomic — either all profiles are removed or the operation fails with an error.
- FR-1.3: `Clear()` MUST return `nil` on success, even if there are zero profiles to remove.

### FR-2: Storage Implementations

- FR-2.1: `MemoryStorage` MUST implement `Clear()` by resetting the internal list and map to empty state.
- FR-2.2: `FilesystemStorage` MUST implement `Clear()` by removing all `.json` profile files from the storage directory.
- FR-2.3: `FilesystemStorage.Clear()` MUST NOT remove the storage directory itself, only its profile file contents.
- FR-2.4: Both implementations MUST hold appropriate locks during the clear operation for thread safety.

### FR-3: API Endpoint

- FR-3.1: A new endpoint `DELETE /_profiler/api/profiles/all` MUST be added to clear all profiles.
- FR-3.2: The endpoint MUST return a JSON response with the format `{"cleared": true}` on success.
- FR-3.3: The endpoint MUST return an appropriate HTTP error (500) if the storage clear operation fails.
- FR-3.4: The endpoint MUST include CORS headers consistent with existing API endpoints.
- FR-3.5: The endpoint MUST support OPTIONS preflight requests.

### FR-4: UI Button

- FR-4.1: A "Clear All" button MUST be added to the profile list view in the Vue UI.
- FR-4.2: Clicking the button MUST prompt the user for confirmation before proceeding (e.g., a confirm dialog).
- FR-4.3: After a successful clear, the profile list MUST be refreshed to show an empty state.
- FR-4.4: The button MUST be visually distinct (destructive action styling — e.g., red/danger color).
- FR-4.5: The button MUST be disabled or show a loading state while the clear operation is in progress.

## Non-Functional Requirements

### NFR-1: Performance

- NFR-1.1: `Clear()` on MemoryStorage MUST complete in O(1) time (reset pointers, not iterate).
- NFR-1.2: `Clear()` on FilesystemStorage SHOULD handle thousands of profile files without timeout.

### NFR-2: Safety

- NFR-2.1: The UI MUST require user confirmation before clearing to prevent accidental data loss.
- NFR-2.2: The API endpoint MUST NOT require any authentication (consistent with existing profiler API design — profiler is dev-only tooling).

### NFR-3: Compatibility

- NFR-3.1: The new `Clear()` method MUST NOT break existing `Storage` interface consumers (this is a breaking interface change — acceptable for a dev tool in active development).
- NFR-3.2: The new endpoint MUST NOT conflict with the existing `DELETE /api/profiles` (purge) endpoint.

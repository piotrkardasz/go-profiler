# Requirements: Request Comparison Feature

## Overview

Add the ability to compare two profiled HTTP requests side-by-side in the go-profiler UI. A dedicated comparison page shows two profiles in parallel columns, with each collector panel rendered side-by-side and differences highlighted. This enables developers to quickly identify performance regressions, behavioral differences, and changes between requests — for example comparing the same endpoint before and after a code change, comparing a fast request vs. a slow one, or comparing requests to different endpoints to understand architectural differences.

**Chosen approach:** Option A — Side-by-Side Full Profile Comparison View  
**Scope:** Exactly 2 profiles compared at a time  
**Excluded:** OpenTelemetry (OTel) collector data is excluded from comparison  
**Included:** GORM query-level diffs (at minimum: show different queries between the two profiles)

## Functional Requirements

### FR-1: Profile Selection for Comparison

- FR-1.1: The ProfileList view MUST allow the user to select exactly two profiles for comparison.
- FR-1.2: Selection MUST be done via checkboxes on each row in the profile list table.
- FR-1.3: When exactly two profiles are selected, a "Compare" button MUST appear (e.g., in a toolbar or floating action area).
- FR-1.4: When fewer or more than two profiles are selected, the "Compare" button MUST be disabled or hidden.
- FR-1.5: Clicking "Compare" MUST navigate to the comparison view with both profile IDs.
- FR-1.6: The user MUST be able to clear the selection to deselect all profiles.

### FR-2: Comparison View — Layout

- FR-2.1: The comparison view MUST be accessible at the route `/_profiler/compare/:idA/:idB`.
- FR-2.2: The view MUST display two profiles in parallel columns (left = Profile A, right = Profile B).
- FR-2.3: A header section MUST show both profiles' summary info (method, URL, status, duration, timestamp) for quick identification.
- FR-2.4: Below the header, collector panels MUST be rendered side-by-side — one instance per column for each collector.
- FR-2.5: Panels MUST be organized by collector name in tab or accordion style, consistent with the existing ProfileDetail view pattern.
- FR-2.6: If a collector has data in one profile but not the other, the missing side MUST display a "No data" placeholder.
- FR-2.7: The OTel collector MUST be excluded from the comparison view entirely (not shown even if data exists).

### FR-3: Comparison — Performance Metrics (Timing & Memory)

- FR-3.1: The timing panel MUST show both profiles' duration side-by-side with the absolute difference and percentage change.
- FR-3.2: The memory panel MUST compare: `alloc_delta`, `heap_alloc`, `heap_inuse`, `heap_objects`, `goroutine_count`, `num_gc`.
- FR-3.3: Numeric differences MUST be color-coded: green for improvement (lower duration, lower memory), red for regression (higher values), neutral for no change.
- FR-3.4: Percentage change MUST be displayed alongside absolute values (e.g., "2.3ms → 5.1ms (+122%)").

### FR-4: Comparison — Request Data

- FR-4.1: The request panel MUST compare method, URL, host, protocol, content type side-by-side.
- FR-4.2: Headers MUST be shown in a merged table: headers present in both shown on the same row, headers unique to one profile highlighted (added/removed style).
- FR-4.3: Query parameters MUST be compared as key-value pairs with add/remove/change indicators.
- FR-4.4: Request body (when captured) MUST be shown side-by-side with text diff highlighting for differences.
- FR-4.5: For JSON bodies, the diff SHOULD be JSON-aware (structural comparison, not just text).
- FR-4.6: Response status codes MUST be shown side-by-side with visual indicator if they differ.
- FR-4.7: Response size MUST be compared with numeric diff.

### FR-5: Comparison — GORM Database Queries

- FR-5.1: The GORM panel in comparison mode MUST show queries from both profiles.
- FR-5.2: Queries that are identical (same SQL, same params) between both profiles MUST be shown as "common" with their respective durations compared.
- FR-5.3: Queries present in Profile A but not Profile B MUST be highlighted as "removed" (or "only in A").
- FR-5.4: Queries present in Profile B but not Profile A MUST be highlighted as "added" (or "only in B").
- FR-5.5: For matching queries, the duration difference MUST be shown (e.g., "1.2ms → 3.4ms").
- FR-5.6: Summary metrics MUST be compared: total queries, total DB time, duplicate count, N+1 count, failed count.
- FR-5.7: Query matching MUST be based on the SQL statement text (ignoring parameter values for matching purposes, but showing param differences when matched).

### FR-6: Comparison — Logger

- FR-6.1: The logger panel MUST compare log summary counts (total, debug, info, warn, error, fatal) with numeric diffs.
- FR-6.2: Log entries MAY be shown in a merged timeline view, but this is optional for v1.
- FR-6.3: At minimum, the count differences per level MUST be visible with color coding (more errors = red).

### FR-7: Comparison — Config/Runtime

- FR-7.1: The config panel MUST show runtime differences (Go version, GOMAXPROCS, CPU count) if they differ.
- FR-7.2: If runtime values are identical, the section MAY be collapsed or show "No differences".
- FR-7.3: Dependency differences (added/removed/version changed modules) MUST be shown when present.

### FR-8: Navigation and Deep Linking

- FR-8.1: The comparison URL (`/_profiler/compare/:idA/:idB`) MUST be shareable — opening it directly MUST load the comparison.
- FR-8.2: A "Swap" button MUST allow the user to swap left/right profiles (swap A and B).
- FR-8.3: Each profile ID in the comparison header MUST link back to the individual ProfileDetail view.
- FR-8.4: A "Back to list" navigation MUST be available from the comparison view.

## Non-Functional Requirements

### NFR-1: Performance

- NFR-1.1: The comparison view MUST load both profiles in parallel (concurrent API calls).
- NFR-1.2: The comparison MUST render within 500ms after both profiles are loaded.
- NFR-1.3: No new backend endpoints are required — the existing `getProfile(id)` API is sufficient.

### NFR-2: Responsiveness

- NFR-2.1: The side-by-side layout MUST work on screens 1280px and wider.
- NFR-2.2: On narrower screens, the layout MAY stack vertically (Profile A above Profile B) or show a notice that a wider screen is recommended.

### NFR-3: Compatibility

- NFR-3.1: The comparison view MUST work with profiles that have different sets of collectors (e.g., one has GORM data, the other doesn't).
- NFR-3.2: The comparison MUST handle missing or null collector data gracefully.
- NFR-3.3: The feature MUST NOT break existing ProfileList or ProfileDetail views.

### NFR-4: Extensibility

- NFR-4.1: The comparison panel pattern MUST be extensible — adding comparison support for a new collector should require only a new comparison panel component.
- NFR-4.2: The comparison logic (diff calculation) MUST be separated from the presentation (reusable utility functions).

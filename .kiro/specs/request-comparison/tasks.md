# Tasks: Request Comparison Feature

#[[file:design.md]]

## Task 1: Create comparison utility module (`ui/src/utils/compare.ts`)

- [ ] Create `ui/src/utils/compare.ts` with the following exported functions:
  - `compareNumbers(a, b, lowerIsBetter)` → returns `NumericDiff` with `valueA`, `valueB`, `delta`, `percentChange`, `direction`
  - `compareMaps(mapA, mapB)` → returns `MapDiff` with `common`, `onlyA`, `onlyB` arrays
  - `compareLists(listA, listB, keyFn)` → returns `ListDiff` with `common`, `onlyA`, `onlyB` arrays
- [ ] Export the `NumericDiff`, `MapDiff`, and `ListDiff` TypeScript interfaces
- [ ] `compareNumbers`: calculate `delta = b - a`, `percentChange = ((b - a) / a) * 100` (handle division by zero → `NaN`), `direction` based on `lowerIsBetter` flag
- [ ] `compareMaps`: iterate all keys from both maps, classify each key into common (exists in both), onlyA, or onlyB. For common keys, set `changed = true` if values differ (deep equality for arrays)
- [ ] `compareLists`: use `keyFn` to build maps of items by key, match items with same key into `common` pairs, remaining into `onlyA`/`onlyB`. For duplicate keys, match by order of appearance
- [ ] Verify the module compiles without errors (`npm run build` in `ui/`)

## Task 2: Create comparison panel registry (`ui/src/plugin/compare-registry.ts` and `compare-builtin.ts`)

- [ ] Create `ui/src/plugin/compare-registry.ts` with:
  - `registerComparePanel(collectorName: string, component: Component): void`
  - `getComparePanel(collectorName: string): Component` (falls back to `CompareGenericPanel`)
  - `hasComparePanel(collectorName: string): boolean`
- [ ] Create `ui/src/components/panels/compare/CompareGenericPanel.vue` — renders `dataA` and `dataB` as two side-by-side JSON trees (reuse `JsonNode` component pattern)
- [ ] Create `ui/src/plugin/compare-builtin.ts` with `initCompareBuiltins()` — initially register only `CompareGenericPanel` for all (specific panels will be registered as they are built in later tasks)
- [ ] Update `ui/src/plugin/index.ts` to export `getComparePanel`, `registerComparePanel`, and `initCompareBuiltins`
- [ ] Update `ui/src/main.ts` to call `initCompareBuiltins()` during app initialization
- [ ] Verify the module compiles without errors

## Task 3: Add profile selection to ProfileList.vue

- [ ] Add `selectedIds` reactive `Set<string>` state to `ProfileList.vue`
- [ ] Add a checkbox column as the first column in the profiles table (`<th>` + `<td>` with `<input type="checkbox">`)
- [ ] Checkbox click toggles the profile ID in `selectedIds` (stop propagation to prevent row navigation)
- [ ] Enforce max 2 selections: if a 3rd checkbox is checked, deselect the oldest (first added)
- [ ] Selected rows get a visual highlight (e.g., `background: #e8f4fd` or a `.selected` class)
- [ ] When `selectedIds.size === 2`, show a "Compare" button in the `.list-actions` toolbar area
- [ ] "Compare" button navigates to `/_profiler/compare/:idA/:idB` using `router.push`
- [ ] Add a "Clear Selection" link/button that clears `selectedIds` when selection count > 0
- [ ] Verify existing row click navigation to profile detail still works (clicks outside checkbox area)
- [ ] Verify the UI compiles and the selection UX works visually

## Task 4: Create ProfileCompare.vue (main comparison view)

- [ ] Create `ui/src/views/ProfileCompare.vue` accepting props `idA: string` and `idB: string`
- [ ] On mount, load both profiles in parallel: `Promise.all([getProfile(idA), getProfile(idB)])` and also `getCollectors()`
- [ ] Handle loading state (show "Loading comparison..." spinner)
- [ ] Handle errors: if either profile fails to load, show error with link back to list
- [ ] Build panel list: filter `collectors` to only those present in at least one profile's `collector_data`, exclude `otel`
- [ ] Render header section:
  - "← Back to Profiles" link to `/_profiler/`
  - "⇄ Swap" button that navigates to `/_profiler/compare/:idB/:idA`
  - Two summary cards side-by-side showing each profile's method badge, URL, status, duration, timestamp, and clickable ID linking to individual profile detail
- [ ] Render tab bar for available collector panels (same style as ProfileDetail)
- [ ] Render active comparison panel using `getComparePanel(activePanel)` with props `:data-a`, `:data-b`, `:profile-a`, `:profile-b`
- [ ] Add the route to `ui/src/router.ts`: `{ path: '/_profiler/compare/:idA/:idB', name: 'profile-compare', component: ProfileCompare, props: true }`
- [ ] Style with the two-column grid layout for summaries, responsive stacking below 1280px
- [ ] Verify navigation from ProfileList selection → Compare view works end-to-end

## Task 5: Implement CompareTimingPanel.vue

- [ ] Create `ui/src/components/panels/compare/CompareTimingPanel.vue`
- [ ] Accept props: `dataA`, `dataB`, `profileA`, `profileB`
- [ ] Render two "hero" duration values side-by-side (same style as existing TimingPanel `.timing-hero`)
- [ ] Between/below the two heroes, render a delta badge showing:
  - Absolute difference (e.g., "+2.47ms" or "-1.2ms")
  - Percentage change (e.g., "+94%")
  - Color: green if Profile B is faster, red if slower, gray if same (within 0.1ms)
- [ ] Below, render a details table with start_time/end_time for each profile
- [ ] Handle case where one profile has no timing data (show "No data" on that side)
- [ ] Register in `compare-builtin.ts`: `registerComparePanel('timing', CompareTimingPanel)`
- [ ] Verify it renders correctly in the comparison view

## Task 6: Implement CompareMemoryPanel.vue

- [ ] Create `ui/src/components/panels/compare/CompareMemoryPanel.vue`
- [ ] Accept props: `dataA`, `dataB`, `profileA`, `profileB`
- [ ] Render a comparison table with columns: Metric | Profile A | Profile B | Delta
- [ ] Rows: `alloc_delta`, `heap_alloc`, `heap_inuse`, `heap_objects`, `goroutine_count`, `num_gc`, `sys`
- [ ] Use `compareNumbers()` utility for each metric (lowerIsBetter = true for all memory metrics)
- [ ] Delta column: show absolute diff and colored indicator (green = less memory, red = more)
- [ ] Format byte values with human-readable units (B, KB, MB)
- [ ] Handle case where one profile has no memory data
- [ ] Register in `compare-builtin.ts`: `registerComparePanel('memory', CompareMemoryPanel)`
- [ ] Verify it renders correctly

## Task 7: Implement CompareRequestPanel.vue

- [ ] Create `ui/src/components/panels/compare/CompareRequestPanel.vue`
- [ ] Accept props: `dataA`, `dataB`, `profileA`, `profileB`
- [ ] **Metadata section**: side-by-side table comparing method, URL, host, proto, content_type, status_code, response_size. Highlight cells that differ between A and B.
- [ ] **Headers diff section**: use `compareMaps()` on `headers` objects. Render merged table:
  - Common headers: show on same row, highlight value column if values differ (`.diff-changed`)
  - Only in A: row with `.diff-removed` background
  - Only in B: row with `.diff-added` background
- [ ] **Query params diff section**: same approach as headers using `compareMaps()` on `query_params`
- [ ] **Body diff section**: show two code blocks side-by-side (Profile A body | Profile B body)
  - For JSON bodies: pretty-print both and visually mark lines that differ
  - For non-JSON: show as preformatted text side-by-side
  - If body is absent in one profile, show "Body not captured" placeholder
- [ ] **Response section**: compare status_code and response_size with delta indicators
- [ ] Register in `compare-builtin.ts`: `registerComparePanel('request', CompareRequestPanel)`
- [ ] Verify it renders correctly

## Task 8: Implement CompareGormPanel.vue

- [ ] Create `ui/src/components/panels/compare/CompareGormPanel.vue`
- [ ] Accept props: `dataA`, `dataB`, `profileA`, `profileB`
- [ ] **Summary comparison section**: table comparing total_queries, total_duration_ms, duplicate_count, n1_count, failed_count between profiles A and B with deltas
- [ ] **Query diff section**: implement query matching algorithm:
  - Extract all queries from all connections in each profile
  - Match queries by SQL text (exact match, ignoring params)
  - For duplicate SQL texts within a profile, match by order of appearance
  - Classify into: common (same SQL in both), onlyA, onlyB
- [ ] **Render common queries**: show SQL once, display duration A vs duration B with delta, show params if they differ
- [ ] **Render "Only in A" queries**: list with `.diff-removed` styling, show SQL, duration, operation badge
- [ ] **Render "Only in B" queries**: list with `.diff-added` styling, show SQL, duration, operation badge
- [ ] Handle case where one or both profiles have no GORM data (show "No database data" message)
- [ ] Register in `compare-builtin.ts`: `registerComparePanel('gorm', CompareGormPanel)`
- [ ] Verify it renders correctly

## Task 9: Implement CompareLoggerPanel.vue and CompareConfigPanel.vue

- [ ] Create `ui/src/components/panels/compare/CompareLoggerPanel.vue`
  - Compare log summary counts (total, debug, info, warn, error, fatal) in a table with deltas
  - Color code: more errors/warnings = red, fewer = green
  - Handle missing logger data in one profile
- [ ] Create `ui/src/components/panels/compare/CompareConfigPanel.vue`
  - Compare runtime fields (go_version, goos, goarch, num_cpu, gomaxprocs, compiler) — highlight differences
  - Compare build info (module_path, vcs_revision) — highlight differences
  - If all runtime values are identical, show collapsed "Runtime configuration is identical" message
  - Compare dependencies lists if present (show added/removed modules)
- [ ] Register both in `compare-builtin.ts`
- [ ] Verify both render correctly

## Task 10: Final integration, polish, and build verification

- [ ] Run full UI build (`npm run build` in `ui/`) — ensure zero errors
- [ ] Verify the complete flow end-to-end:
  1. ProfileList → select 2 profiles via checkboxes → Compare button appears
  2. Click Compare → navigates to ProfileCompare view
  3. Both profiles load, tabs show all collectors (except OTel)
  4. Each comparison panel renders correctly with diffs
  5. Swap button works
  6. Back to Profiles link works
  7. Profile ID links navigate to individual detail views
- [ ] Verify responsive behavior: below 1280px, side-by-side layout stacks vertically
- [ ] Verify edge case: comparing profiles where one lacks GORM/logger data shows "No data" gracefully
- [ ] Verify existing ProfileList and ProfileDetail views still work unchanged
- [ ] Run linting if configured (`npm run lint`) and fix any issues

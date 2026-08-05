# Design: Request Comparison Feature

## Technical Architecture

#[[file:requirements.md]]

### Overview

The comparison feature is entirely frontend-driven. No backend API changes are needed — the existing `GET /_profiler/api/profiles/{id}` endpoint provides full profile data. The frontend loads two profiles in parallel and renders them through a new comparison view with specialized comparison panels.

### Component Architecture

```
router.ts (new route: /_profiler/compare/:idA/:idB)
    │
    ▼
ProfileCompare.vue (new view)
    │
    ├── CompareHeader (profile summaries side-by-side)
    │
    ├── ComparePanel: Timing  ─── CompareTimingPanel.vue
    ├── ComparePanel: Memory  ─── CompareMemoryPanel.vue
    ├── ComparePanel: Request ─── CompareRequestPanel.vue
    ├── ComparePanel: GORM    ─── CompareGormPanel.vue
    ├── ComparePanel: Logger  ─── CompareLoggerPanel.vue
    └── ComparePanel: Config  ─── CompareConfigPanel.vue

utils/
    ├── compare.ts (diff calculation utilities)
    └── format.ts (shared formatting — extracted from existing panels)
```

### Data Flow

```
ProfileList.vue
    │ (user selects 2 profiles via checkboxes)
    │ (clicks "Compare" button)
    ▼
router.push({ name: 'profile-compare', params: { idA, idB } })
    │
    ▼
ProfileCompare.vue
    │ onMounted:
    │   Promise.all([getProfile(idA), getProfile(idB)])
    │   getCollectors()
    │
    ▼ (both profiles + collector metadata loaded)
    │
    │ Filter out "otel" collector
    │ For each remaining collector with data in either profile:
    │   render comparison panel side-by-side
    ▼
CompareXxxPanel.vue receives { dataA, dataB } props
    │
    └── Uses compare.ts utilities to compute diffs
```

---

## Detailed Component Design

### 1. Router Changes (`ui/src/router.ts`)

Add a new route:

```typescript
{
  path: '/_profiler/compare/:idA/:idB',
  name: 'profile-compare',
  component: ProfileCompare,
  props: true,
}
```

### 2. ProfileList.vue — Selection Mode

**New state:**
```typescript
const selectedIds = ref<Set<string>>(new Set())
const selectionMode = ref(false)
```

**Changes:**
- Add a checkbox column as the first column in the table
- Clicking checkbox toggles the profile ID in `selectedIds`
- When `selectedIds.size === 2`, show a "Compare" button in the list-actions toolbar
- "Compare" button navigates to `/_profiler/compare/:idA/:idB`
- A "Cancel" or "Clear" button clears selection
- Row click behavior: when selection mode is active (at least 1 selected), clicking row toggles selection instead of navigating to detail

**Selection constraints:**
- Maximum 2 selections enforced — selecting a 3rd deselects the oldest
- Visual feedback: selected rows get a highlighted background

### 3. ProfileCompare.vue (New View)

**Props:** `idA: string, idB: string`

**Template structure:**
```
<div class="profile-compare">
  <!-- Header: back link + swap button -->
  <div class="compare-header">
    <router-link to="/_profiler/">← Back to Profiles</router-link>
    <button @click="swap">⇄ Swap</button>
  </div>

  <!-- Profile summaries side-by-side -->
  <div class="compare-summaries">
    <div class="summary-card summary-a">
      <!-- Profile A: method badge, URL, status, duration, timestamp, ID link -->
    </div>
    <div class="summary-card summary-b">
      <!-- Profile B: method badge, URL, status, duration, timestamp, ID link -->
    </div>
  </div>

  <!-- Collector tabs (same tab pattern as ProfileDetail) -->
  <div class="compare-tabs">
    <button v-for="panel in panels" ...>{{ panel.label }}</button>
  </div>

  <!-- Active comparison panel -->
  <div class="compare-content">
    <component :is="getCompareComponent(activePanel)"
      :data-a="profileA.collector_data[activePanel]"
      :data-b="profileB.collector_data[activePanel]"
      :profile-a="profileA"
      :profile-b="profileB"
    />
  </div>
</div>
```

**Swap behavior:** Swaps `idA` ↔ `idB` by navigating to the swapped URL (updates route params).

**Panel filtering:** Exclude `otel` from the panel list. Show a panel tab if at least one profile has data for that collector.

### 4. Comparison Panel Registry

Extend the plugin system with a parallel comparison registry:

```typescript
// ui/src/plugin/compare-registry.ts
const comparePanelRegistry = new Map<string, Component>()

export function registerComparePanel(collectorName: string, component: Component): void
export function getComparePanel(collectorName: string): Component  // fallback: CompareGenericPanel
```

Register comparison panels in a new `initCompareBuiltins()`:
```typescript
registerComparePanel('timing', CompareTimingPanel)
registerComparePanel('memory', CompareMemoryPanel)
registerComparePanel('request', CompareRequestPanel)
registerComparePanel('gorm', CompareGormPanel)
registerComparePanel('logger', CompareLoggerPanel)
registerComparePanel('config', CompareConfigPanel)
```

### 5. Comparison Panels — Shared Props Interface

All comparison panels receive:
```typescript
interface ComparePanelProps {
  dataA: unknown       // collector_data[name] from profile A (may be undefined)
  dataB: unknown       // collector_data[name] from profile B (may be undefined)
  profileA: Profile    // full profile A (for top-level fields like duration)
  profileB: Profile    // full profile B
}
```

### 6. Individual Panel Designs

#### CompareTimingPanel.vue

```
┌─────────────────────────────┬─────────────────────────────┐
│        Profile A            │        Profile B            │
│                             │                             │
│     ┌──────────────┐        │     ┌──────────────┐        │
│     │   2.631 ms   │        │     │   5.102 ms   │        │
│     └──────────────┘        │     └──────────────┘        │
│                             │                             │
│  Start: 18:26:07.055        │  Start: 18:26:09.200        │
│  End:   18:26:07.057        │  End:   18:26:09.205        │
└─────────────────────────────┴─────────────────────────────┘
           ┌─────────────────────────┐
           │  Delta: +2.471ms (+94%) │  ← RED (regression)
           └─────────────────────────┘
```

Layout: Two "hero" duration values side-by-side, a delta badge between/below them showing absolute and percentage difference with color coding.

#### CompareMemoryPanel.vue

```
┌──────────────────┬─────────────┬─────────────┬──────────────┐
│ Metric           │ Profile A   │ Profile B   │ Delta        │
├──────────────────┼─────────────┼─────────────┼──────────────┤
│ Alloc Delta      │ 0 B         │ 24,576 B    │ +24,576 B 🔴 │
│ Heap Alloc       │ 1,653,872 B │ 1,678,448 B │ +24,576 B    │
│ Heap In-Use      │ 3,080,192 B │ 3,080,192 B │ 0 (same)     │
│ Heap Objects     │ 193         │ 201         │ +8           │
│ Goroutines       │ 13          │ 15          │ +2           │
│ GC Cycles        │ 0           │ 1           │ +1           │
└──────────────────┴─────────────┴─────────────┴──────────────┘
```

Layout: Table with metric name, value A, value B, and computed delta column with color indicators.

#### CompareRequestPanel.vue

Sections rendered vertically:

1. **Metadata comparison** — side-by-side table for method, URL, host, protocol, content type, status code, response size. Highlights cells that differ.

2. **Headers diff** — merged table:
   - Headers in both: show on same row, highlight value if different
   - Headers only in A: row highlighted with "removed" style (red background)
   - Headers only in B: row highlighted with "added" style (green background)

3. **Query params diff** — same merged table approach as headers.

4. **Body diff** — two code blocks side-by-side. For JSON, attempt pretty-print and show line-level differences. For non-JSON, plain text side-by-side.

#### CompareGormPanel.vue

Layout:

1. **Summary metrics comparison** — same table pattern as CompareMemoryPanel:
   - Total queries, total DB time, duplicate count, N+1 count, failed count

2. **Query diff** — the core feature:
   - Match queries by SQL text (normalized — ignore params for matching)
   - Display in three groups:
     - **Common queries** — present in both, show SQL once with duration comparison
     - **Only in A** — queries that appear only in profile A (highlighted as "removed")
     - **Only in B** — queries that appear only in profile B (highlighted as "added")
   - For common queries with different params, show params side-by-side

**Query matching algorithm:**
```typescript
function matchQueries(queriesA: QueryEntry[], queriesB: QueryEntry[]): {
  common: Array<{ queryA: QueryEntry; queryB: QueryEntry }>
  onlyA: QueryEntry[]
  onlyB: QueryEntry[]
}
```
Matching is done by SQL text equality. If the same SQL appears multiple times, match by order of appearance.

#### CompareLoggerPanel.vue

1. **Summary counts comparison** — table with total, debug, info, warn, error, fatal — values A vs B with deltas.
2. **Entries** (optional detail) — simple list showing which entries are common/unique to each profile. V1 can just show count diffs.

#### CompareConfigPanel.vue

1. **Runtime diff** — show Go version, GOMAXPROCS, num_cpu, goos, goarch. Only highlight rows that differ.
2. **Dependencies diff** — if both profiles have dependency lists, show added/removed/changed modules.
3. **If no differences** — collapse section with "Runtime configuration is identical" message.

---

## Utility Module: `ui/src/utils/compare.ts`

Reusable diff calculation functions:

```typescript
// Numeric comparison with percentage
export interface NumericDiff {
  valueA: number
  valueB: number
  delta: number       // B - A
  percentChange: number  // ((B - A) / A) * 100, NaN if A is 0
  direction: 'better' | 'worse' | 'same'
}

export function compareNumbers(
  a: number,
  b: number,
  lowerIsBetter: boolean = true
): NumericDiff

// Key-value map diff (for headers, query params)
export interface MapDiff<V = string[]> {
  common: Array<{ key: string; valueA: V; valueB: V; changed: boolean }>
  onlyA: Array<{ key: string; value: V }>
  onlyB: Array<{ key: string; value: V }>
}

export function compareMaps<V>(
  mapA: Record<string, V>,
  mapB: Record<string, V>
): MapDiff<V>

// List diff by key function
export interface ListDiff<T> {
  common: Array<{ itemA: T; itemB: T }>
  onlyA: T[]
  onlyB: T[]
}

export function compareLists<T>(
  listA: T[],
  listB: T[],
  keyFn: (item: T) => string
): ListDiff<T>
```

---

## Styling Approach

The comparison feature uses the existing project CSS patterns (scoped styles, no framework). New CSS classes:

```css
/* Delta indicators */
.delta-better { color: #2b8a3e; }   /* green — improvement */
.delta-worse  { color: #c92a2a; }   /* red — regression */
.delta-same   { color: #6c757d; }   /* gray — no change */

/* Diff rows */
.diff-added   { background: #d3f9d8; }  /* light green */
.diff-removed { background: #ffe3e3; }  /* light red */
.diff-changed { background: #fff3bf; }  /* light yellow */

/* Side-by-side layout */
.compare-columns {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 1.5rem;
}

/* Responsive: stack on narrow screens */
@media (max-width: 1280px) {
  .compare-columns {
    grid-template-columns: 1fr;
  }
}
```

---

## File Structure (New Files)

```
ui/src/
├── views/
│   └── ProfileCompare.vue              (new)
├── components/
│   └── panels/
│       ├── compare/
│       │   ├── CompareTimingPanel.vue   (new)
│       │   ├── CompareMemoryPanel.vue   (new)
│       │   ├── CompareRequestPanel.vue  (new)
│       │   ├── CompareGormPanel.vue     (new)
│       │   ├── CompareLoggerPanel.vue   (new)
│       │   ├── CompareConfigPanel.vue   (new)
│       │   └── CompareGenericPanel.vue  (new — fallback JSON side-by-side)
│       └── (existing panels unchanged)
├── plugin/
│   ├── compare-registry.ts             (new)
│   └── compare-builtin.ts             (new)
└── utils/
    └── compare.ts                      (new)
```

**Modified files:**
- `ui/src/router.ts` — add comparison route
- `ui/src/views/ProfileList.vue` — add selection checkboxes and compare button
- `ui/src/main.ts` — call `initCompareBuiltins()`
- `ui/src/plugin/index.ts` — export comparison registry

---

## Edge Cases

| Case | Handling |
|------|----------|
| Profile not found (404) | Show error message, link back to list |
| One profile has a collector, the other doesn't | Show data for the profile that has it, "No data" for the other |
| Both profiles lack GORM data | Hide GORM tab entirely |
| OTel data present | Always hide from comparison (per requirements) |
| Very long query lists (50+ queries) | Show first 20 with "Show all" toggle |
| Identical profiles (same ID twice) | Allow it but show "Profiles are identical" notice |
| Body not captured in either profile | Hide body section in CompareRequestPanel |

---

## What This Design Does NOT Include

- Backend API changes (none needed)
- OpenTelemetry comparison (explicitly excluded)
- More than 2 profiles comparison
- Auto-suggest "compare with similar" (future enhancement)
- Response body comparison (not currently captured)
- Persistent selection state across navigations (selection lives in ProfileList component state only)

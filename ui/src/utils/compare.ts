/**
 * Comparison utility functions for the request comparison feature.
 * These are pure functions that compute diffs between two data sets.
 */

// --- Numeric Comparison ---

export interface NumericDiff {
  valueA: number
  valueB: number
  delta: number
  percentChange: number
  direction: 'better' | 'worse' | 'same'
}

/**
 * Compare two numeric values and compute the diff.
 * @param a - Value from Profile A
 * @param b - Value from Profile B
 * @param lowerIsBetter - If true, a lower value in B is "better" (default: true)
 */
export function compareNumbers(a: number, b: number, lowerIsBetter: boolean = true): NumericDiff {
  const delta = b - a
  const percentChange = a !== 0 ? ((b - a) / Math.abs(a)) * 100 : (b !== 0 ? NaN : 0)

  let direction: 'better' | 'worse' | 'same'
  if (delta === 0) {
    direction = 'same'
  } else if (lowerIsBetter) {
    direction = delta < 0 ? 'better' : 'worse'
  } else {
    direction = delta > 0 ? 'better' : 'worse'
  }

  return { valueA: a, valueB: b, delta, percentChange, direction }
}

// --- Map (Key-Value) Comparison ---

export interface MapDiffEntry<V = string[]> {
  key: string
  valueA: V
  valueB: V
  changed: boolean
}

export interface MapDiff<V = string[]> {
  common: MapDiffEntry<V>[]
  onlyA: Array<{ key: string; value: V }>
  onlyB: Array<{ key: string; value: V }>
}

/**
 * Compare two key-value maps (e.g., headers, query params).
 * Returns entries classified as common (in both), onlyA, or onlyB.
 */
export function compareMaps<V>(
  mapA: Record<string, V> | undefined | null,
  mapB: Record<string, V> | undefined | null
): MapDiff<V> {
  const a = mapA || {}
  const b = mapB || {}

  const keysA = new Set(Object.keys(a))
  const keysB = new Set(Object.keys(b))

  const common: MapDiffEntry<V>[] = []
  const onlyA: Array<{ key: string; value: V }> = []
  const onlyB: Array<{ key: string; value: V }> = []

  for (const key of keysA) {
    if (keysB.has(key)) {
      const valA = a[key]
      const valB = b[key]
      const changed = JSON.stringify(valA) !== JSON.stringify(valB)
      common.push({ key, valueA: valA, valueB: valB, changed })
    } else {
      onlyA.push({ key, value: a[key] })
    }
  }

  for (const key of keysB) {
    if (!keysA.has(key)) {
      onlyB.push({ key, value: b[key] })
    }
  }

  return { common, onlyA, onlyB }
}

// --- List Comparison ---

export interface ListDiff<T> {
  common: Array<{ itemA: T; itemB: T }>
  onlyA: T[]
  onlyB: T[]
}

/**
 * Compare two lists by matching items using a key function.
 * Items with the same key are paired; unmatched items go to onlyA/onlyB.
 * For duplicate keys within a list, matches are made by order of appearance.
 */
export function compareLists<T>(
  listA: T[],
  listB: T[],
  keyFn: (item: T) => string
): ListDiff<T> {
  const common: Array<{ itemA: T; itemB: T }> = []
  const onlyA: T[] = []
  const onlyB: T[] = []

  // Build a map of key -> array of items for list B (to handle duplicates)
  const bByKey = new Map<string, T[]>()
  for (const item of listB) {
    const key = keyFn(item)
    if (!bByKey.has(key)) {
      bByKey.set(key, [])
    }
    bByKey.get(key)!.push(item)
  }

  // Track which B items have been matched
  const matchedBIndices = new Set<string>()

  for (const itemA of listA) {
    const key = keyFn(itemA)
    const bItems = bByKey.get(key)

    if (bItems && bItems.length > 0) {
      // Find the first unmatched B item with this key
      let matched = false
      for (let i = 0; i < bItems.length; i++) {
        const bKey = `${key}:${i}`
        if (!matchedBIndices.has(bKey)) {
          matchedBIndices.add(bKey)
          common.push({ itemA, itemB: bItems[i] })
          matched = true
          break
        }
      }
      if (!matched) {
        onlyA.push(itemA)
      }
    } else {
      onlyA.push(itemA)
    }
  }

  // Remaining unmatched B items
  for (const [key, bItems] of bByKey) {
    for (let i = 0; i < bItems.length; i++) {
      const bKey = `${key}:${i}`
      if (!matchedBIndices.has(bKey)) {
        onlyB.push(bItems[i])
      }
    }
  }

  return { common, onlyA, onlyB }
}

// --- Formatting Helpers ---

/**
 * Format a numeric delta as a human-readable string with sign.
 */
export function formatDelta(delta: number, unit: string = ''): string {
  const sign = delta > 0 ? '+' : ''
  return `${sign}${delta}${unit}`
}

/**
 * Format a percentage change.
 */
export function formatPercent(percent: number): string {
  if (isNaN(percent) || !isFinite(percent)) return ''
  const sign = percent > 0 ? '+' : ''
  return `(${sign}${percent.toFixed(1)}%)`
}

/**
 * Format bytes as human-readable (B, KB, MB, GB).
 */
export function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B'
  const sign = bytes < 0 ? '-' : ''
  const abs = Math.abs(bytes)
  if (abs < 1024) return `${sign}${abs} B`
  if (abs < 1024 * 1024) return `${sign}${(abs / 1024).toFixed(1)} KB`
  if (abs < 1024 * 1024 * 1024) return `${sign}${(abs / (1024 * 1024)).toFixed(1)} MB`
  return `${sign}${(abs / (1024 * 1024 * 1024)).toFixed(2)} GB`
}

/**
 * Format duration in milliseconds.
 */
export function formatDuration(ms: number | undefined): string {
  if (!ms && ms !== 0) return '—'
  if (ms === 0) return '0ms'
  if (Math.abs(ms) < 1) return `${(ms * 1000).toFixed(0)}us`
  if (Math.abs(ms) < 1000) return `${ms.toFixed(1)}ms`
  return `${(ms / 1000).toFixed(3)}s`
}

<template>
  <div class="compare-gorm-panel">
    <div v-if="!dataA && !dataB" class="no-data">No database data available for either profile.</div>

    <template v-else>
      <!-- Summary comparison -->
      <section class="compare-section">
        <h3 class="section-title">Summary</h3>
        <table class="compare-table">
          <thead>
            <tr>
              <th>Metric</th>
              <th class="val-col">Profile A</th>
              <th class="val-col">Profile B</th>
              <th class="val-col">Delta</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="row in summaryRows" :key="row.label">
              <td class="metric-name">{{ row.label }}</td>
              <td class="val-col">{{ row.formattedA }}</td>
              <td class="val-col">{{ row.formattedB }}</td>
              <td :class="['val-col', 'delta-cell', `delta-${row.direction}`]">
                {{ row.formattedDelta }}
              </td>
            </tr>
          </tbody>
        </table>
      </section>

      <!-- Query diff -->
      <section class="compare-section">
        <h3 class="section-title">Query Comparison</h3>

        <!-- Common queries -->
        <div v-if="queryDiff.common.length" class="query-group">
          <h4 class="group-title">Common Queries ({{ queryDiff.common.length }})</h4>
          <div v-for="(pair, idx) in queryDiff.common" :key="'common-' + idx" class="query-item">
            <div class="query-header">
              <span :class="['query-operation', `op-${pair.itemA.operation.toLowerCase()}`]">
                {{ pair.itemA.operation }}
              </span>
              <span class="query-duration-compare">
                {{ formatMs(pair.itemA.duration_ms) }}
                <span class="duration-arrow">&rarr;</span>
                {{ formatMs(pair.itemB.duration_ms) }}
                <span :class="['duration-delta', durationDeltaClass(pair.itemA.duration_ms, pair.itemB.duration_ms)]">
                  ({{ formatDurationDelta(pair.itemA.duration_ms, pair.itemB.duration_ms) }})
                </span>
              </span>
            </div>
            <div class="query-sql">
              <code>{{ pair.itemA.sql }}</code>
            </div>
            <div v-if="paramsChanged(pair.itemA.params, pair.itemB.params)" class="query-params-diff">
              <div class="params-row">
                <span class="params-label">A:</span>
                <code>{{ formatParams(pair.itemA.params) }}</code>
              </div>
              <div class="params-row">
                <span class="params-label">B:</span>
                <code>{{ formatParams(pair.itemB.params) }}</code>
              </div>
            </div>
          </div>
        </div>

        <!-- Only in A -->
        <div v-if="queryDiff.onlyA.length" class="query-group">
          <h4 class="group-title group-removed">Only in Profile A ({{ queryDiff.onlyA.length }})</h4>
          <div v-for="(query, idx) in queryDiff.onlyA" :key="'onlyA-' + idx" class="query-item diff-removed-item">
            <div class="query-header">
              <span :class="['query-operation', `op-${query.operation.toLowerCase()}`]">
                {{ query.operation }}
              </span>
              <span class="query-duration">{{ formatMs(query.duration_ms) }}</span>
            </div>
            <div class="query-sql">
              <code>{{ query.sql }}</code>
            </div>
            <div v-if="query.params && query.params.length" class="query-params">
              <span class="params-label">Params:</span>
              <code>{{ formatParams(query.params) }}</code>
            </div>
          </div>
        </div>

        <!-- Only in B -->
        <div v-if="queryDiff.onlyB.length" class="query-group">
          <h4 class="group-title group-added">Only in Profile B ({{ queryDiff.onlyB.length }})</h4>
          <div v-for="(query, idx) in queryDiff.onlyB" :key="'onlyB-' + idx" class="query-item diff-added-item">
            <div class="query-header">
              <span :class="['query-operation', `op-${query.operation.toLowerCase()}`]">
                {{ query.operation }}
              </span>
              <span class="query-duration">{{ formatMs(query.duration_ms) }}</span>
            </div>
            <div class="query-sql">
              <code>{{ query.sql }}</code>
            </div>
            <div v-if="query.params && query.params.length" class="query-params">
              <span class="params-label">Params:</span>
              <code>{{ formatParams(query.params) }}</code>
            </div>
          </div>
        </div>

        <div v-if="!queryDiff.common.length && !queryDiff.onlyA.length && !queryDiff.onlyB.length" class="no-data">
          No queries to compare.
        </div>
      </section>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { compareNumbers, compareLists, formatDuration } from '../../../utils/compare'
import type { Profile } from '../../../api'

interface QueryEntry {
  sql: string
  params?: unknown[]
  runnable_query?: string
  duration_ms: number
  rows_affected: number
  operation: string
  connection: string
  error?: string
  transaction_id?: string
  backtrace?: string[]
  timestamp: string
  index: number
}

interface ConnectionData {
  name: string
  queries: QueryEntry[]
  transactions: unknown[]
  total_duration_ms: number
  query_count: number
}

interface Summary {
  total_queries: number
  total_duration_ms: number
  queries_per_connection: Record<string, number>
  slowest_query?: QueryEntry
  duplicate_count: number
  n1_count: number
  failed_count: number
  transaction_count: number
}

interface GormData {
  connections: ConnectionData[]
  analysis: unknown
  summary: Summary
  failed_queries: QueryEntry[]
}

const props = defineProps<{
  dataA: unknown
  dataB: unknown
  profileA: Profile
  profileB: Profile
}>()

const gormA = computed<GormData>(() => {
  const d = (props.dataA || {}) as GormData
  return {
    connections: d.connections || [],
    analysis: d.analysis || {},
    summary: d.summary || { total_queries: 0, total_duration_ms: 0, queries_per_connection: {}, duplicate_count: 0, n1_count: 0, failed_count: 0, transaction_count: 0 },
    failed_queries: d.failed_queries || [],
  }
})

const gormB = computed<GormData>(() => {
  const d = (props.dataB || {}) as GormData
  return {
    connections: d.connections || [],
    analysis: d.analysis || {},
    summary: d.summary || { total_queries: 0, total_duration_ms: 0, queries_per_connection: {}, duplicate_count: 0, n1_count: 0, failed_count: 0, transaction_count: 0 },
    failed_queries: d.failed_queries || [],
  }
})

// Flatten all queries from all connections
function getAllQueries(gorm: GormData): QueryEntry[] {
  const queries: QueryEntry[] = []
  for (const conn of gorm.connections) {
    if (conn.queries) {
      for (const q of conn.queries) {
        queries.push(q)
      }
    }
  }
  return queries
}

const queryDiff = computed(() => {
  const queriesA = getAllQueries(gormA.value)
  const queriesB = getAllQueries(gormB.value)
  return compareLists(queriesA, queriesB, (q) => q.sql)
})

const summaryRows = computed(() => {
  const metrics = [
    { label: 'Total Queries', key: 'total_queries' as const, isDuration: false },
    { label: 'Total DB Time', key: 'total_duration_ms' as const, isDuration: true },
    { label: 'Duplicate Count', key: 'duplicate_count' as const, isDuration: false },
    { label: 'N+1 Count', key: 'n1_count' as const, isDuration: false },
    { label: 'Failed Count', key: 'failed_count' as const, isDuration: false },
    { label: 'Transactions', key: 'transaction_count' as const, isDuration: false },
  ]

  return metrics.map(m => {
    const valA = gormA.value.summary[m.key] as number || 0
    const valB = gormB.value.summary[m.key] as number || 0
    const diff = compareNumbers(valA, valB, true)

    const format = m.isDuration ? formatMs : (n: number) => n.toLocaleString()
    const formattedA = props.dataA ? format(valA) : '—'
    const formattedB = props.dataB ? format(valB) : '—'

    let formattedDelta: string
    if (!props.dataA || !props.dataB) {
      formattedDelta = '—'
    } else if (diff.delta === 0) {
      formattedDelta = 'same'
    } else {
      const sign = diff.delta > 0 ? '+' : ''
      formattedDelta = `${sign}${format(diff.delta)}`
    }

    return { label: m.label, formattedA, formattedB, formattedDelta, direction: diff.direction }
  })
})

function formatMs(ms: number | undefined): string {
  return formatDuration(ms)
}

function formatParams(params: unknown[] | undefined): string {
  if (!params || params.length === 0) return '—'
  return JSON.stringify(params)
}

function paramsChanged(paramsA: unknown[] | undefined, paramsB: unknown[] | undefined): boolean {
  return JSON.stringify(paramsA || []) !== JSON.stringify(paramsB || [])
}

function formatDurationDelta(msA: number, msB: number): string {
  const delta = msB - msA
  const sign = delta > 0 ? '+' : ''
  return `${sign}${formatDuration(delta)}`
}

function durationDeltaClass(msA: number, msB: number): string {
  const delta = msB - msA
  if (Math.abs(delta) < 0.1) return 'delta-same'
  return delta < 0 ? 'delta-better' : 'delta-worse'
}
</script>

<style scoped>
.compare-gorm-panel {
  display: flex;
  flex-direction: column;
  gap: 1.5rem;
}

.no-data {
  color: #6c757d;
  font-style: italic;
  padding: 1.5rem;
  text-align: center;
  background: #f8f9fa;
  border-radius: 6px;
}

.compare-section {
  border: 1px solid #e9ecef;
  border-radius: 6px;
  padding: 1rem;
}

.section-title {
  font-size: 0.85rem;
  font-weight: 600;
  color: #1a1a2e;
  margin-bottom: 0.75rem;
  padding-bottom: 0.5rem;
  border-bottom: 1px solid #f1f3f5;
}

/* Summary table */
.compare-table {
  width: 100%;
  border-collapse: collapse;
}

.compare-table th {
  text-align: left;
  padding: 0.5rem 0.75rem;
  font-size: 0.7rem;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: #6c757d;
  border-bottom: 1px solid #e9ecef;
}

.compare-table td {
  padding: 0.5rem 0.75rem;
  font-size: 0.8rem;
  border-bottom: 1px solid #f8f9fa;
  font-variant-numeric: tabular-nums;
}

.metric-name {
  font-weight: 500;
  color: #495057;
}

.val-col {
  text-align: right;
  min-width: 80px;
}

.delta-cell {
  font-weight: 500;
}

.delta-better { color: #2b8a3e; }
.delta-worse { color: #c92a2a; }
.delta-same { color: #6c757d; }

/* Query groups */
.query-group {
  margin-top: 1rem;
}

.group-title {
  font-size: 0.8rem;
  font-weight: 600;
  color: #495057;
  margin-bottom: 0.5rem;
  padding: 0.4rem 0.6rem;
  background: #f8f9fa;
  border-radius: 4px;
}

.group-removed { color: #c92a2a; background: #fff5f5; }
.group-added { color: #2b8a3e; background: #f0fff4; }

/* Query items */
.query-item {
  padding: 0.6rem 0.75rem;
  border: 1px solid #f1f3f5;
  border-radius: 4px;
  margin-bottom: 0.5rem;
}

.diff-removed-item {
  border-color: #ffc9c9;
  background: #fff5f5;
}

.diff-added-item {
  border-color: #b2f2bb;
  background: #f0fff4;
}

.query-header {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-bottom: 0.3rem;
  flex-wrap: wrap;
}

.query-operation {
  font-size: 0.65rem;
  font-weight: 700;
  padding: 0.1rem 0.35rem;
  border-radius: 3px;
  text-transform: uppercase;
}

.op-select { background: #d4edda; color: #155724; }
.op-insert { background: #cce5ff; color: #004085; }
.op-update { background: #fff3cd; color: #856404; }
.op-delete { background: #f8d7da; color: #721c24; }
.op-raw { background: #e2e3e5; color: #383d41; }

.query-duration {
  font-size: 0.75rem;
  color: #495057;
  font-variant-numeric: tabular-nums;
}

.query-duration-compare {
  font-size: 0.75rem;
  color: #495057;
  font-variant-numeric: tabular-nums;
  display: flex;
  align-items: center;
  gap: 0.3rem;
}

.duration-arrow {
  color: #adb5bd;
}

.duration-delta {
  font-weight: 500;
  font-size: 0.7rem;
}

.query-sql {
  margin-bottom: 0.2rem;
}

.query-sql code {
  font-size: 0.75rem;
  color: #1a1a2e;
  word-break: break-word;
  white-space: pre-wrap;
}

.query-params,
.query-params-diff {
  margin-top: 0.3rem;
  font-size: 0.7rem;
  color: #6c757d;
}

.query-params code,
.query-params-diff code {
  font-size: 0.7rem;
  color: #495057;
  word-break: break-all;
}

.params-label {
  font-weight: 600;
  margin-right: 0.25rem;
}

.params-row {
  margin-bottom: 0.15rem;
}
</style>

<template>
  <div class="gorm-panel">
    <!-- Summary Section -->
    <div class="summary-bar">
      <div class="summary-item">
        <span class="summary-value">{{ gormData.summary.total_queries }}</span>
        <span class="summary-label">Queries</span>
      </div>
      <div class="summary-item">
        <span class="summary-value">{{ formatDuration(gormData.summary.total_duration_ms) }}</span>
        <span class="summary-label">Total Time</span>
      </div>
      <div class="summary-item" v-if="gormData.summary.transaction_count > 0">
        <span class="summary-value">{{ gormData.summary.transaction_count }}</span>
        <span class="summary-label">Transactions</span>
      </div>
      <div class="summary-item warning" v-if="gormData.summary.duplicate_count > 0">
        <span class="summary-value">{{ gormData.summary.duplicate_count }}</span>
        <span class="summary-label">Duplicates</span>
      </div>
      <div class="summary-item danger" v-if="gormData.summary.n1_count > 0">
        <span class="summary-value">{{ gormData.summary.n1_count }}</span>
        <span class="summary-label">N+1 Issues</span>
      </div>
      <div class="summary-item danger" v-if="gormData.summary.failed_count > 0">
        <span class="summary-value">{{ gormData.summary.failed_count }}</span>
        <span class="summary-label">Failed</span>
      </div>
    </div>

    <!-- Tabs -->
    <div class="tabs">
      <button
        v-for="tab in availableTabs"
        :key="tab.id"
        :class="['tab-btn', { active: activeTab === tab.id }]"
        @click="activeTab = tab.id"
      >
        {{ tab.label }}
        <span v-if="tab.badge" :class="['badge', tab.badgeClass]">{{ tab.badge }}</span>
      </button>
    </div>

    <!-- Tab Content: Queries by Connection -->
    <div v-if="activeTab === 'queries'" class="tab-content">
      <div v-for="conn in gormData.connections" :key="conn.name" class="connection-group">
        <div class="connection-header">
          <span class="connection-name">{{ conn.name }}</span>
          <span class="connection-stats">
            {{ conn.query_count }} queries &middot; {{ formatDuration(conn.total_duration_ms) }}
          </span>
        </div>
        <div class="query-list">
          <div
            v-for="query in conn.queries"
            :key="query.index"
            :class="['query-item', { 'query-slow': isSlow(query), 'query-error': query.error }]"
          >
            <div class="query-header">
              <span class="query-index">#{{ query.index + 1 }}</span>
              <span :class="['query-operation', `op-${query.operation.toLowerCase()}`]">
                {{ query.operation }}
              </span>
              <span class="query-duration">{{ formatDuration(query.duration_ms) }}</span>
              <span v-if="query.rows_affected >= 0" class="query-rows">
                {{ query.rows_affected }} row{{ query.rows_affected !== 1 ? 's' : '' }}
              </span>
              <span v-if="query.transaction_id" class="query-tx">TX</span>
              <span v-if="isSlow(query)" class="badge badge-warning">SLOW</span>
              <span v-if="query.error" class="badge badge-danger">ERROR</span>
            </div>
            <div class="query-sql">
              <code>{{ query.sql }}</code>
            </div>
            <div v-if="query.params && query.params.length" class="query-params">
              <span class="params-label">Params:</span>
              <code>{{ JSON.stringify(query.params) }}</code>
            </div>
            <div v-if="query.runnable_query && query.runnable_query !== query.sql" class="query-runnable">
              <span class="runnable-label">Runnable:</span>
              <code>{{ query.runnable_query }}</code>
            </div>
            <div v-if="query.error" class="query-error-msg">
              {{ query.error }}
            </div>
            <div v-if="query.backtrace && query.backtrace.length" class="query-backtrace">
              <details>
                <summary>Backtrace ({{ query.backtrace.length }} frames)</summary>
                <pre>{{ query.backtrace.join('\n') }}</pre>
              </details>
            </div>
          </div>
        </div>
      </div>
      <div v-if="gormData.connections.length === 0" class="empty-state">
        No database queries were captured for this request.
      </div>
    </div>

    <!-- Tab Content: Transactions -->
    <div v-if="activeTab === 'transactions'" class="tab-content">
      <div v-for="conn in connectionsWithTransactions" :key="conn.name">
        <div class="connection-header">
          <span class="connection-name">{{ conn.name }}</span>
        </div>
        <div v-for="tx in conn.transactions" :key="tx.id" class="transaction-group">
          <div class="tx-header">
            <span class="tx-id">{{ tx.id }}</span>
            <span :class="['tx-status', `status-${tx.status}`]">{{ tx.status }}</span>
            <span class="tx-duration">{{ formatDuration(tx.total_duration_ms) }}</span>
            <span class="tx-count">{{ tx.queries.length }} queries</span>
          </div>
          <div class="tx-queries">
            <div v-for="query in tx.queries" :key="query.index" class="query-item compact">
              <div class="query-header">
                <span class="query-index">#{{ query.index + 1 }}</span>
                <span :class="['query-operation', `op-${query.operation.toLowerCase()}`]">
                  {{ query.operation }}
                </span>
                <span class="query-duration">{{ formatDuration(query.duration_ms) }}</span>
              </div>
              <div class="query-sql">
                <code>{{ query.sql }}</code>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Tab Content: Analysis -->
    <div v-if="activeTab === 'analysis'" class="tab-content">
      <!-- N+1 Queries -->
      <div v-if="gormData.analysis.n1_queries && gormData.analysis.n1_queries.length" class="analysis-section">
        <h3 class="analysis-title danger-title">N+1 Query Problems</h3>
        <div v-for="(group, idx) in gormData.analysis.n1_queries" :key="'n1-' + idx" class="analysis-item">
          <div class="analysis-header">
            <span class="analysis-count">{{ group.count }}x repeated</span>
            <span class="analysis-connection">on {{ group.connection }}</span>
          </div>
          <code class="analysis-sql">{{ group.sql }}</code>
        </div>
      </div>

      <!-- Duplicate Queries -->
      <div v-if="gormData.analysis.duplicate_queries && gormData.analysis.duplicate_queries.length" class="analysis-section">
        <h3 class="analysis-title warning-title">Duplicate Queries</h3>
        <div v-for="(group, idx) in gormData.analysis.duplicate_queries" :key="'dup-' + idx" class="analysis-item">
          <div class="analysis-header">
            <span class="analysis-count">{{ group.count }}x identical</span>
          </div>
          <code class="analysis-sql">{{ group.sql }}</code>
          <div v-if="group.params && group.params.length" class="analysis-params">
            Params: {{ JSON.stringify(group.params) }}
          </div>
        </div>
      </div>

      <!-- Similar Queries -->
      <div v-if="gormData.analysis.similar_queries && gormData.analysis.similar_queries.length" class="analysis-section">
        <h3 class="analysis-title info-title">Similar Queries</h3>
        <div v-for="(group, idx) in gormData.analysis.similar_queries" :key="'sim-' + idx" class="analysis-item">
          <div class="analysis-header">
            <span class="analysis-count">{{ group.count }}x same pattern</span>
          </div>
          <code class="analysis-sql">{{ group.sql }}</code>
        </div>
      </div>

      <!-- Slow Queries -->
      <div v-if="gormData.analysis.slow_queries && gormData.analysis.slow_queries.length" class="analysis-section">
        <h3 class="analysis-title warning-title">Slow Queries</h3>
        <div v-for="query in gormData.analysis.slow_queries" :key="'slow-' + query.index" class="analysis-item">
          <div class="analysis-header">
            <span class="query-duration">{{ formatDuration(query.duration_ms) }}</span>
            <span class="analysis-connection">on {{ query.connection }}</span>
          </div>
          <code class="analysis-sql">{{ query.runnable_query || query.sql }}</code>
        </div>
      </div>

      <div v-if="!hasAnalysisResults" class="empty-state">
        No issues detected. All queries look good!
      </div>
    </div>

    <!-- Tab Content: Failed Queries -->
    <div v-if="activeTab === 'failed'" class="tab-content">
      <div v-for="query in gormData.failed_queries" :key="'fail-' + query.index" class="query-item query-error">
        <div class="query-header">
          <span class="query-index">#{{ query.index + 1 }}</span>
          <span :class="['query-operation', `op-${query.operation.toLowerCase()}`]">
            {{ query.operation }}
          </span>
          <span class="query-duration">{{ formatDuration(query.duration_ms) }}</span>
          <span class="query-connection">{{ query.connection }}</span>
        </div>
        <div class="query-sql">
          <code>{{ query.runnable_query || query.sql }}</code>
        </div>
        <div class="query-error-msg">
          {{ query.error }}
        </div>
        <div v-if="query.backtrace && query.backtrace.length" class="query-backtrace">
          <details>
            <summary>Backtrace</summary>
            <pre>{{ query.backtrace.join('\n') }}</pre>
          </details>
        </div>
      </div>
      <div v-if="!gormData.failed_queries || gormData.failed_queries.length === 0" class="empty-state">
        No failed queries.
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'

interface QueryEntry {
  sql: string
  params?: any[]
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

interface TransactionGroup {
  id: string
  connection: string
  queries: QueryEntry[]
  total_duration_ms: number
  status: string
}

interface ConnectionData {
  name: string
  queries: QueryEntry[]
  transactions: TransactionGroup[]
  total_duration_ms: number
  query_count: number
  failed_queries: QueryEntry[]
}

interface DuplicateGroup {
  sql: string
  params?: any[]
  count: number
  indices: number[]
}

interface SimilarGroup {
  sql: string
  count: number
  indices: number[]
}

interface N1Group {
  sql: string
  count: number
  connection: string
  indices: number[]
}

interface AnalysisResult {
  duplicate_queries?: DuplicateGroup[]
  similar_queries?: SimilarGroup[]
  n1_queries?: N1Group[]
  slow_queries?: QueryEntry[]
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
  analysis: AnalysisResult
  summary: Summary
  failed_queries: QueryEntry[]
}

const props = defineProps<{
  data: unknown
  collectorName: string
}>()

const activeTab = ref('queries')

const gormData = computed<GormData>(() => {
  const d = (props.data || {}) as GormData
  return {
    connections: d.connections || [],
    analysis: d.analysis || {},
    summary: d.summary || {
      total_queries: 0,
      total_duration_ms: 0,
      queries_per_connection: {},
      duplicate_count: 0,
      n1_count: 0,
      failed_count: 0,
      transaction_count: 0,
    },
    failed_queries: d.failed_queries || [],
  }
})

const connectionsWithTransactions = computed(() => {
  return gormData.value.connections.filter(c => c.transactions && c.transactions.length > 0)
})

const hasAnalysisResults = computed(() => {
  const a = gormData.value.analysis
  return (
    (a.n1_queries && a.n1_queries.length > 0) ||
    (a.duplicate_queries && a.duplicate_queries.length > 0) ||
    (a.similar_queries && a.similar_queries.length > 0) ||
    (a.slow_queries && a.slow_queries.length > 0)
  )
})

const availableTabs = computed(() => {
  const tabs = [
    { id: 'queries', label: 'Queries', badge: gormData.value.summary.total_queries || null, badgeClass: '' },
    { id: 'analysis', label: 'Analysis', badge: analysisIssueCount.value || null, badgeClass: 'badge-warning' },
  ]

  if (gormData.value.summary.transaction_count > 0) {
    tabs.splice(1, 0, {
      id: 'transactions',
      label: 'Transactions',
      badge: gormData.value.summary.transaction_count,
      badgeClass: '',
    })
  }

  if (gormData.value.summary.failed_count > 0) {
    tabs.push({
      id: 'failed',
      label: 'Failed',
      badge: gormData.value.summary.failed_count,
      badgeClass: 'badge-danger',
    })
  }

  return tabs
})

const analysisIssueCount = computed(() => {
  const a = gormData.value.analysis
  return (
    (a.n1_queries?.length || 0) +
    (a.duplicate_queries?.length || 0) +
    (a.slow_queries?.length || 0)
  )
})

function isSlow(query: QueryEntry): boolean {
  // Default threshold for UI highlighting (100ms)
  return query.duration_ms >= 100
}

function formatDuration(ms: number | undefined): string {
  if (!ms) return '0ms'
  if (ms < 1) return `${(ms * 1000).toFixed(0)}µs`
  if (ms < 1000) return `${ms.toFixed(1)}ms`
  return `${(ms / 1000).toFixed(2)}s`
}
</script>

<style scoped>
.gorm-panel {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

/* Summary Bar */
.summary-bar {
  display: flex;
  gap: 1.5rem;
  padding: 1rem 1.25rem;
  background: #f8f9fa;
  border-radius: 8px;
  flex-wrap: wrap;
}

.summary-item {
  display: flex;
  flex-direction: column;
  align-items: center;
}

.summary-value {
  font-size: 1.5rem;
  font-weight: 700;
  color: #1a1a2e;
  font-variant-numeric: tabular-nums;
}

.summary-item.warning .summary-value {
  color: #e67e22;
}

.summary-item.danger .summary-value {
  color: #e74c3c;
}

.summary-label {
  font-size: 0.75rem;
  color: #6c757d;
  margin-top: 0.15rem;
}

/* Tabs */
.tabs {
  display: flex;
  gap: 0;
  border-bottom: 2px solid #e9ecef;
}

.tab-btn {
  padding: 0.5rem 1rem;
  background: none;
  border: none;
  border-bottom: 2px solid transparent;
  margin-bottom: -2px;
  cursor: pointer;
  font-size: 0.85rem;
  font-weight: 500;
  color: #6c757d;
  display: flex;
  align-items: center;
  gap: 0.4rem;
}

.tab-btn:hover {
  color: #1a1a2e;
}

.tab-btn.active {
  color: #1a1a2e;
  border-bottom-color: #1a1a2e;
}

/* Badges */
.badge {
  font-size: 0.7rem;
  padding: 0.1rem 0.4rem;
  border-radius: 10px;
  background: #e9ecef;
  color: #495057;
  font-weight: 600;
}

.badge-warning {
  background: #fff3cd;
  color: #856404;
}

.badge-danger {
  background: #f8d7da;
  color: #721c24;
}

/* Tab Content */
.tab-content {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

/* Connection Groups */
.connection-group {
  border: 1px solid #e9ecef;
  border-radius: 6px;
  overflow: hidden;
}

.connection-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0.6rem 1rem;
  background: #f1f3f5;
  border-bottom: 1px solid #e9ecef;
}

.connection-name {
  font-weight: 600;
  font-size: 0.85rem;
  color: #1a1a2e;
}

.connection-stats {
  font-size: 0.75rem;
  color: #6c757d;
}

/* Query List */
.query-list {
  display: flex;
  flex-direction: column;
}

.query-item {
  padding: 0.75rem 1rem;
  border-bottom: 1px solid #f1f3f5;
}

.query-item:last-child {
  border-bottom: none;
}

.query-item.query-slow {
  background: #fff9e6;
}

.query-item.query-error {
  background: #fff5f5;
}

.query-item.compact {
  padding: 0.5rem 1rem;
}

.query-header {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-bottom: 0.35rem;
}

.query-index {
  font-size: 0.7rem;
  color: #adb5bd;
  font-weight: 600;
  min-width: 2rem;
}

.query-operation {
  font-size: 0.7rem;
  font-weight: 700;
  padding: 0.1rem 0.4rem;
  border-radius: 3px;
  text-transform: uppercase;
}

.op-select {
  background: #d4edda;
  color: #155724;
}

.op-insert {
  background: #cce5ff;
  color: #004085;
}

.op-update {
  background: #fff3cd;
  color: #856404;
}

.op-delete {
  background: #f8d7da;
  color: #721c24;
}

.op-raw {
  background: #e2e3e5;
  color: #383d41;
}

.query-duration {
  font-size: 0.75rem;
  color: #495057;
  font-variant-numeric: tabular-nums;
}

.query-rows {
  font-size: 0.7rem;
  color: #6c757d;
}

.query-tx {
  font-size: 0.65rem;
  font-weight: 700;
  padding: 0.05rem 0.3rem;
  border-radius: 3px;
  background: #e8daef;
  color: #6c3483;
}

.query-connection {
  font-size: 0.7rem;
  color: #6c757d;
}

.query-sql {
  margin-bottom: 0.25rem;
}

.query-sql code {
  font-size: 0.8rem;
  color: #1a1a2e;
  word-break: break-word;
  white-space: pre-wrap;
}

.query-params,
.query-runnable {
  margin-top: 0.25rem;
  font-size: 0.75rem;
  color: #6c757d;
}

.query-params code,
.query-runnable code {
  font-size: 0.75rem;
  color: #495057;
  word-break: break-all;
}

.params-label,
.runnable-label {
  font-weight: 500;
  margin-right: 0.3rem;
}

.query-error-msg {
  margin-top: 0.35rem;
  font-size: 0.8rem;
  color: #e74c3c;
  font-weight: 500;
}

.query-backtrace {
  margin-top: 0.35rem;
}

.query-backtrace summary {
  font-size: 0.75rem;
  color: #6c757d;
  cursor: pointer;
}

.query-backtrace pre {
  font-size: 0.7rem;
  margin-top: 0.25rem;
  padding: 0.5rem;
  background: #f8f9fa;
  border-radius: 4px;
  overflow-x: auto;
  color: #495057;
}

/* Transaction Groups */
.transaction-group {
  border: 1px solid #e8daef;
  border-radius: 6px;
  margin-bottom: 0.75rem;
  overflow: hidden;
}

.tx-header {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.5rem 1rem;
  background: #f9f5fc;
  border-bottom: 1px solid #e8daef;
}

.tx-id {
  font-size: 0.8rem;
  font-weight: 600;
  color: #6c3483;
  font-family: monospace;
}

.tx-status {
  font-size: 0.7rem;
  font-weight: 700;
  padding: 0.1rem 0.4rem;
  border-radius: 3px;
}

.status-committed {
  background: #d4edda;
  color: #155724;
}

.status-rolled_back {
  background: #f8d7da;
  color: #721c24;
}

.status-pending {
  background: #fff3cd;
  color: #856404;
}

.tx-duration {
  font-size: 0.75rem;
  color: #495057;
}

.tx-count {
  font-size: 0.7rem;
  color: #6c757d;
}

.tx-queries {
  padding: 0.25rem 0;
}

/* Analysis Section */
.analysis-section {
  margin-bottom: 1.25rem;
}

.analysis-title {
  font-size: 0.9rem;
  font-weight: 600;
  margin-bottom: 0.5rem;
  padding-bottom: 0.25rem;
  border-bottom: 1px solid #e9ecef;
}

.danger-title {
  color: #e74c3c;
}

.warning-title {
  color: #e67e22;
}

.info-title {
  color: #3498db;
}

.analysis-item {
  padding: 0.5rem 0.75rem;
  margin-bottom: 0.5rem;
  background: #f8f9fa;
  border-radius: 4px;
}

.analysis-header {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-bottom: 0.25rem;
}

.analysis-count {
  font-size: 0.8rem;
  font-weight: 600;
  color: #495057;
}

.analysis-connection {
  font-size: 0.75rem;
  color: #6c757d;
}

.analysis-sql {
  display: block;
  font-size: 0.8rem;
  color: #1a1a2e;
  word-break: break-word;
  white-space: pre-wrap;
}

.analysis-params {
  font-size: 0.75rem;
  color: #6c757d;
  margin-top: 0.2rem;
}

/* Empty State */
.empty-state {
  text-align: center;
  padding: 2rem;
  color: #6c757d;
  font-size: 0.9rem;
}
</style>

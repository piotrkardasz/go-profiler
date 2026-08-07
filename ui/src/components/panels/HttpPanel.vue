<template>
  <div class="http-panel">
    <!-- Summary Section -->
    <div class="summary-bar">
      <div class="summary-item">
        <span class="summary-value">{{ httpData.summary.total_calls }}</span>
        <span class="summary-label">Calls</span>
      </div>
      <div class="summary-item">
        <span class="summary-value">{{ formatDuration(httpData.summary.total_duration_ms) }}</span>
        <span class="summary-label">Total Time</span>
      </div>
      <div class="summary-item" v-if="serviceCount > 1">
        <span class="summary-value">{{ serviceCount }}</span>
        <span class="summary-label">Services</span>
      </div>
      <div class="summary-item warning" v-if="httpData.summary.slow_count > 0">
        <span class="summary-value">{{ httpData.summary.slow_count }}</span>
        <span class="summary-label">Slow</span>
      </div>
      <div class="summary-item danger" v-if="httpData.summary.failed_count > 0">
        <span class="summary-value">{{ httpData.summary.failed_count }}</span>
        <span class="summary-label">Failed</span>
      </div>
      <div class="summary-item warning" v-if="httpData.summary.duplicate_count > 0">
        <span class="summary-value">{{ httpData.summary.duplicate_count }}</span>
        <span class="summary-label">Duplicates</span>
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

    <!-- Tab Content: All Calls -->
    <div v-if="activeTab === 'calls'" class="tab-content">
      <div class="call-list">
        <div
          v-for="call in httpData.calls"
          :key="call.index"
          :class="['call-item', { 'call-slow': isSlow(call), 'call-failed': isFailed(call) }]"
          @click="toggleExpand(call.index)"
        >
          <div class="call-header">
            <span class="call-index">#{{ call.index + 1 }}</span>
            <span class="call-service">{{ call.service }}</span>
            <span :class="['call-method', `method-${call.method.toLowerCase()}`]">
              {{ call.method }}
            </span>
            <span class="call-url" :title="call.url">{{ truncateUrl(call.url) }}</span>
            <span :class="['call-status', statusClass(call)]">
              {{ call.status_code || 'ERR' }}
            </span>
            <span class="call-duration">{{ formatDuration(call.duration_ms) }}</span>
            <span v-if="isSlow(call)" class="badge badge-warning">SLOW</span>
            <span v-if="call.error" class="badge badge-danger">ERROR</span>
          </div>

          <!-- Duration bar visualization -->
          <div class="duration-bar-container">
            <div
              class="duration-bar"
              :style="{ width: durationPercent(call) + '%' }"
              :class="{ 'bar-slow': isSlow(call), 'bar-failed': isFailed(call) }"
            ></div>
          </div>

          <!-- Expanded Detail -->
          <div v-if="expandedIndex === call.index" class="call-detail" @click.stop>
            <!-- Error -->
            <div v-if="call.error" class="detail-section error-section">
              <span class="detail-label">Error:</span>
              <span class="error-message">{{ call.error }}</span>
            </div>

            <!-- Request Headers -->
            <div v-if="call.request_headers && Object.keys(call.request_headers).length" class="detail-section">
              <details>
                <summary>Request Headers ({{ Object.keys(call.request_headers).length }})</summary>
                <div class="headers-grid">
                  <template v-for="(values, name) in call.request_headers" :key="'req-' + name">
                    <span class="header-name">{{ name }}:</span>
                    <span class="header-value">{{ values.join(', ') }}</span>
                  </template>
                </div>
              </details>
            </div>

            <!-- Request Body -->
            <div v-if="call.request_body" class="detail-section">
              <details>
                <summary>Request Body ({{ call.request_size }} bytes)</summary>
                <pre class="body-content">{{ call.request_body }}</pre>
              </details>
            </div>

            <!-- Response Headers -->
            <div v-if="call.response_headers && Object.keys(call.response_headers).length" class="detail-section">
              <details>
                <summary>Response Headers ({{ Object.keys(call.response_headers).length }})</summary>
                <div class="headers-grid">
                  <template v-for="(values, name) in call.response_headers" :key="'res-' + name">
                    <span class="header-name">{{ name }}:</span>
                    <span class="header-value">{{ values.join(', ') }}</span>
                  </template>
                </div>
              </details>
            </div>

            <!-- Response Body -->
            <div v-if="call.response_body" class="detail-section">
              <details>
                <summary>Response Body ({{ call.response_size }} bytes)</summary>
                <pre class="body-content">{{ call.response_body }}</pre>
              </details>
            </div>

            <!-- cURL Command -->
            <div v-if="call.curl_command" class="detail-section">
              <details>
                <summary>cURL Command</summary>
                <pre class="curl-content">{{ call.curl_command }}</pre>
              </details>
            </div>

            <!-- Backtrace -->
            <div v-if="call.backtrace && call.backtrace.length" class="detail-section">
              <details>
                <summary>Backtrace ({{ call.backtrace.length }} frames)</summary>
                <pre class="backtrace-content">{{ call.backtrace.join('\n') }}</pre>
              </details>
            </div>
          </div>
        </div>
      </div>
      <div v-if="httpData.calls.length === 0" class="empty-state">
        No outbound HTTP calls were captured for this request.
      </div>
    </div>

    <!-- Tab Content: By Service -->
    <div v-if="activeTab === 'services'" class="tab-content">
      <div v-for="(calls, service) in callsByService" :key="service" class="service-group">
        <div class="service-header">
          <span class="service-name">{{ service }}</span>
          <span class="service-stats">
            {{ calls.length }} calls &middot; {{ formatDuration(serviceDuration(calls)) }}
          </span>
        </div>
        <div class="call-list">
          <div
            v-for="call in calls"
            :key="call.index"
            :class="['call-item compact', { 'call-slow': isSlow(call), 'call-failed': isFailed(call) }]"
          >
            <div class="call-header">
              <span class="call-index">#{{ call.index + 1 }}</span>
              <span :class="['call-method', `method-${call.method.toLowerCase()}`]">
                {{ call.method }}
              </span>
              <span class="call-url" :title="call.url">{{ truncateUrl(call.url) }}</span>
              <span :class="['call-status', statusClass(call)]">
                {{ call.status_code || 'ERR' }}
              </span>
              <span class="call-duration">{{ formatDuration(call.duration_ms) }}</span>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Tab Content: Analysis -->
    <div v-if="activeTab === 'analysis'" class="tab-content">
      <!-- Slow Calls -->
      <div v-if="httpData.analysis.slow_calls && httpData.analysis.slow_calls.length" class="analysis-section">
        <h3 class="analysis-title warning-title">Slow Calls</h3>
        <div v-for="call in httpData.analysis.slow_calls" :key="'slow-' + call.index" class="analysis-item">
          <div class="analysis-header">
            <span class="call-service">{{ call.service }}</span>
            <span :class="['call-method', `method-${call.method.toLowerCase()}`]">{{ call.method }}</span>
            <span class="call-duration">{{ formatDuration(call.duration_ms) }}</span>
          </div>
          <code class="analysis-url">{{ call.url }}</code>
        </div>
      </div>

      <!-- Failed Calls -->
      <div v-if="httpData.analysis.failed_calls && httpData.analysis.failed_calls.length" class="analysis-section">
        <h3 class="analysis-title danger-title">Failed Calls</h3>
        <div v-for="call in httpData.analysis.failed_calls" :key="'fail-' + call.index" class="analysis-item">
          <div class="analysis-header">
            <span class="call-service">{{ call.service }}</span>
            <span :class="['call-method', `method-${call.method.toLowerCase()}`]">{{ call.method }}</span>
            <span :class="['call-status', statusClass(call)]">{{ call.status_code || 'ERR' }}</span>
          </div>
          <code class="analysis-url">{{ call.url }}</code>
          <div v-if="call.error" class="error-message">{{ call.error }}</div>
        </div>
      </div>

      <!-- Duplicate Calls -->
      <div v-if="httpData.analysis.duplicate_calls && httpData.analysis.duplicate_calls.length" class="analysis-section">
        <h3 class="analysis-title warning-title">Duplicate Calls</h3>
        <div v-for="(group, idx) in httpData.analysis.duplicate_calls" :key="'dup-' + idx" class="analysis-item">
          <div class="analysis-header">
            <span class="analysis-count">{{ group.count }}x repeated</span>
            <span :class="['call-method', `method-${group.method.toLowerCase()}`]">{{ group.method }}</span>
          </div>
          <code class="analysis-url">{{ group.url }}</code>
          <div class="analysis-indices">Calls: {{ group.indices.map((i: number) => '#' + (i + 1)).join(', ') }}</div>
        </div>
      </div>

      <div v-if="!hasAnalysisResults" class="empty-state">
        No issues detected. All HTTP calls look good!
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'

interface HTTPCallEntry {
  index: number
  service: string
  method: string
  url: string
  request_headers?: Record<string, string[]>
  request_body?: string
  request_size: number
  status_code: number
  response_headers?: Record<string, string[]>
  response_body?: string
  response_size: number
  duration_ms: number
  error?: string
  timestamp: string
  backtrace?: string[]
  curl_command?: string
}

interface DuplicateGroup {
  method: string
  url: string
  count: number
  indices: number[]
}

interface AnalysisResult {
  slow_calls?: HTTPCallEntry[]
  failed_calls?: HTTPCallEntry[]
  duplicate_calls?: DuplicateGroup[]
}

interface Summary {
  total_calls: number
  total_duration_ms: number
  calls_per_service: Record<string, number>
  failed_count: number
  slow_count: number
  duplicate_count: number
  slowest_call?: HTTPCallEntry
}

interface HTTPData {
  calls: HTTPCallEntry[]
  analysis: AnalysisResult
  summary: Summary
}

const props = defineProps<{
  data: unknown
  collectorName: string
}>()

const activeTab = ref('calls')
const expandedIndex = ref<number | null>(null)

const httpData = computed<HTTPData>(() => {
  const d = (props.data || {}) as HTTPData
  return {
    calls: d.calls || [],
    analysis: d.analysis || {},
    summary: d.summary || {
      total_calls: 0,
      total_duration_ms: 0,
      calls_per_service: {},
      failed_count: 0,
      slow_count: 0,
      duplicate_count: 0,
    },
  }
})

const serviceCount = computed(() => {
  return Object.keys(httpData.value.summary.calls_per_service || {}).length
})

const callsByService = computed(() => {
  const groups: Record<string, HTTPCallEntry[]> = {}
  for (const call of httpData.value.calls) {
    if (!groups[call.service]) {
      groups[call.service] = []
    }
    groups[call.service].push(call)
  }
  return groups
})

const maxDuration = computed(() => {
  if (httpData.value.calls.length === 0) return 1
  return Math.max(...httpData.value.calls.map(c => c.duration_ms), 1)
})

const hasAnalysisResults = computed(() => {
  const a = httpData.value.analysis
  return (
    (a.slow_calls && a.slow_calls.length > 0) ||
    (a.failed_calls && a.failed_calls.length > 0) ||
    (a.duplicate_calls && a.duplicate_calls.length > 0)
  )
})

const availableTabs = computed(() => {
  const tabs = [
    { id: 'calls', label: 'Calls', badge: httpData.value.summary.total_calls || null, badgeClass: '' },
  ]

  if (serviceCount.value > 1) {
    tabs.push({
      id: 'services',
      label: 'By Service',
      badge: serviceCount.value,
      badgeClass: '',
    })
  }

  const issueCount =
    (httpData.value.analysis.slow_calls?.length || 0) +
    (httpData.value.analysis.failed_calls?.length || 0) +
    (httpData.value.analysis.duplicate_calls?.length || 0)

  tabs.push({
    id: 'analysis',
    label: 'Analysis',
    badge: issueCount || null,
    badgeClass: issueCount > 0 ? 'badge-warning' : '',
  })

  return tabs
})

function toggleExpand(index: number) {
  expandedIndex.value = expandedIndex.value === index ? null : index
}

function isSlow(call: HTTPCallEntry): boolean {
  return (httpData.value.analysis.slow_calls || []).some(c => c.index === call.index)
}

function isFailed(call: HTTPCallEntry): boolean {
  if (call.error) return true
  return call.status_code > 0 && (call.status_code < 200 || call.status_code >= 300)
}

function statusClass(call: HTTPCallEntry): string {
  if (call.error || call.status_code === 0) return 'status-error'
  if (call.status_code >= 200 && call.status_code < 300) return 'status-success'
  if (call.status_code >= 300 && call.status_code < 400) return 'status-redirect'
  if (call.status_code >= 400 && call.status_code < 500) return 'status-client-error'
  return 'status-server-error'
}

function durationPercent(call: HTTPCallEntry): number {
  return Math.max((call.duration_ms / maxDuration.value) * 100, 2)
}

function serviceDuration(calls: HTTPCallEntry[]): number {
  return calls.reduce((sum, c) => sum + c.duration_ms, 0)
}

function truncateUrl(url: string): string {
  try {
    const u = new URL(url)
    const path = u.pathname + u.search
    return path.length > 60 ? path.substring(0, 57) + '...' : path
  } catch {
    return url.length > 60 ? url.substring(0, 57) + '...' : url
  }
}

function formatDuration(ms: number | undefined): string {
  if (!ms) return '0ms'
  if (ms < 1) return `${(ms * 1000).toFixed(0)}us`
  if (ms < 1000) return `${ms.toFixed(1)}ms`
  return `${(ms / 1000).toFixed(2)}s`
}
</script>

<style scoped>
.http-panel {
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
  gap: 0.5rem;
}

/* Call List */
.call-list {
  display: flex;
  flex-direction: column;
}

.call-item {
  padding: 0.6rem 1rem;
  border: 1px solid #e9ecef;
  border-radius: 6px;
  margin-bottom: 0.4rem;
  cursor: pointer;
  transition: background 0.15s;
}

.call-item:hover {
  background: #f8f9fa;
}

.call-item.call-slow {
  border-left: 3px solid #e67e22;
}

.call-item.call-failed {
  border-left: 3px solid #e74c3c;
}

.call-item.compact {
  padding: 0.4rem 1rem;
}

.call-header {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  flex-wrap: wrap;
}

.call-index {
  font-size: 0.7rem;
  color: #adb5bd;
  font-weight: 600;
  min-width: 1.8rem;
}

.call-service {
  font-size: 0.75rem;
  font-weight: 600;
  color: #495057;
  background: #e9ecef;
  padding: 0.1rem 0.4rem;
  border-radius: 3px;
}

.call-method {
  font-size: 0.7rem;
  font-weight: 700;
  padding: 0.1rem 0.4rem;
  border-radius: 3px;
  text-transform: uppercase;
}

.method-get {
  background: #d4edda;
  color: #155724;
}

.method-post {
  background: #cce5ff;
  color: #004085;
}

.method-put {
  background: #fff3cd;
  color: #856404;
}

.method-patch {
  background: #fff3cd;
  color: #856404;
}

.method-delete {
  background: #f8d7da;
  color: #721c24;
}

.method-head {
  background: #e2e3e5;
  color: #383d41;
}

.method-options {
  background: #e2e3e5;
  color: #383d41;
}

.call-url {
  font-size: 0.8rem;
  color: #1a1a2e;
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-family: monospace;
}

.call-status {
  font-size: 0.75rem;
  font-weight: 700;
  padding: 0.1rem 0.4rem;
  border-radius: 3px;
  font-variant-numeric: tabular-nums;
}

.status-success {
  background: #d4edda;
  color: #155724;
}

.status-redirect {
  background: #cce5ff;
  color: #004085;
}

.status-client-error {
  background: #fff3cd;
  color: #856404;
}

.status-server-error {
  background: #f8d7da;
  color: #721c24;
}

.status-error {
  background: #f8d7da;
  color: #721c24;
}

.call-duration {
  font-size: 0.75rem;
  color: #495057;
  font-variant-numeric: tabular-nums;
  min-width: 4rem;
  text-align: right;
}

/* Duration Bar */
.duration-bar-container {
  height: 3px;
  background: #f1f3f5;
  border-radius: 2px;
  margin-top: 0.4rem;
}

.duration-bar {
  height: 100%;
  background: #3498db;
  border-radius: 2px;
  transition: width 0.3s ease;
}

.duration-bar.bar-slow {
  background: #e67e22;
}

.duration-bar.bar-failed {
  background: #e74c3c;
}

/* Call Detail (expanded) */
.call-detail {
  margin-top: 0.75rem;
  padding-top: 0.75rem;
  border-top: 1px solid #e9ecef;
}

.detail-section {
  margin-bottom: 0.5rem;
}

.detail-section details {
  cursor: pointer;
}

.detail-section summary {
  font-size: 0.75rem;
  color: #6c757d;
  font-weight: 500;
  cursor: pointer;
  margin-bottom: 0.3rem;
}

.detail-label {
  font-size: 0.75rem;
  font-weight: 600;
  color: #495057;
}

.error-section {
  background: #fff5f5;
  padding: 0.5rem 0.75rem;
  border-radius: 4px;
}

.error-message {
  font-size: 0.8rem;
  color: #e74c3c;
  font-weight: 500;
}

.headers-grid {
  display: grid;
  grid-template-columns: auto 1fr;
  gap: 0.2rem 0.75rem;
  font-size: 0.75rem;
  padding: 0.5rem;
  background: #f8f9fa;
  border-radius: 4px;
}

.header-name {
  font-weight: 600;
  color: #495057;
  font-family: monospace;
}

.header-value {
  color: #6c757d;
  word-break: break-all;
  font-family: monospace;
}

.body-content,
.curl-content,
.backtrace-content {
  font-size: 0.75rem;
  padding: 0.5rem;
  background: #f8f9fa;
  border-radius: 4px;
  overflow-x: auto;
  white-space: pre-wrap;
  word-break: break-word;
  color: #1a1a2e;
  margin: 0;
}

/* Service Groups */
.service-group {
  border: 1px solid #e9ecef;
  border-radius: 6px;
  overflow: hidden;
  margin-bottom: 0.75rem;
}

.service-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0.6rem 1rem;
  background: #f1f3f5;
  border-bottom: 1px solid #e9ecef;
}

.service-name {
  font-weight: 600;
  font-size: 0.85rem;
  color: #1a1a2e;
}

.service-stats {
  font-size: 0.75rem;
  color: #6c757d;
}

/* Analysis */
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

.warning-title {
  color: #e67e22;
}

.danger-title {
  color: #e74c3c;
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

.analysis-url {
  display: block;
  font-size: 0.8rem;
  color: #1a1a2e;
  word-break: break-word;
}

.analysis-indices {
  font-size: 0.7rem;
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

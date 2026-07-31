<template>
  <div class="otel-panel">
    <div class="otel-tabs">
      <button
        :class="['otel-tab', { active: activeTab === 'traces' }]"
        @click="activeTab = 'traces'"
      >
        Traces ({{ spans.length }})
      </button>
      <button
        :class="['otel-tab', { active: activeTab === 'metrics' }]"
        @click="activeTab = 'metrics'"
      >
        Metrics ({{ metrics.length }})
      </button>
    </div>

    <div v-if="activeTab === 'traces'" class="otel-traces">
      <div v-if="spans.length === 0" class="empty-state">
        No traces captured for this request.
      </div>
      <div v-else class="waterfall">
        <div class="waterfall-header">
          <span class="wf-col-name">Span</span>
          <span class="wf-col-duration">Duration</span>
          <span class="wf-col-bar">Timeline</span>
        </div>
        <div
          v-for="span in sortedSpans"
          :key="span.span_id"
          class="waterfall-row"
          :style="{ paddingLeft: `${getDepth(span) * 1.5}rem` }"
        >
          <span class="wf-col-name">
            <span class="span-name">{{ span.name }}</span>
            <span v-if="span.status && span.status !== 'Unset'" :class="['span-status', `status-${span.status.toLowerCase()}`]">
              {{ span.status }}
            </span>
          </span>
          <span class="wf-col-duration">{{ formatDuration(span.duration_ms) }}</span>
          <span class="wf-col-bar">
            <div class="bar-track">
              <div
                class="bar-fill"
                :style="{ left: barLeft(span), width: barWidth(span) }"
                :title="`${formatDuration(span.duration_ms)}`"
              ></div>
            </div>
          </span>
        </div>
      </div>

      <div v-if="selectedSpan" class="span-detail">
        <h4>{{ selectedSpan.name }}</h4>
        <table class="detail-table">
          <tr><th>Trace ID</th><td><code>{{ selectedSpan.trace_id }}</code></td></tr>
          <tr><th>Span ID</th><td><code>{{ selectedSpan.span_id }}</code></td></tr>
          <tr v-if="selectedSpan.parent_id"><th>Parent ID</th><td><code>{{ selectedSpan.parent_id }}</code></td></tr>
          <tr><th>Duration</th><td>{{ formatDuration(selectedSpan.duration_ms) }}</td></tr>
          <tr><th>Status</th><td>{{ selectedSpan.status || 'Unset' }}</td></tr>
        </table>
        <div v-if="selectedSpan.attributes && Object.keys(selectedSpan.attributes).length > 0" class="span-attributes">
          <h5>Attributes</h5>
          <table class="detail-table">
            <tr v-for="[key, value] in Object.entries(selectedSpan.attributes)" :key="key">
              <th>{{ key }}</th>
              <td>{{ value }}</td>
            </tr>
          </table>
        </div>
        <div v-if="selectedSpan.events && selectedSpan.events.length > 0" class="span-events">
          <h5>Events</h5>
          <div v-for="(event, idx) in selectedSpan.events" :key="idx" class="event-item">
            <span class="event-name">{{ event.name }}</span>
            <span class="event-time">{{ formatTimestamp(event.timestamp) }}</span>
          </div>
        </div>
      </div>
    </div>

    <div v-if="activeTab === 'metrics'" class="otel-metrics">
      <div v-if="metrics.length === 0" class="empty-state">
        No metrics captured for this request.
      </div>
      <table v-else class="metrics-table">
        <thead>
          <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Value</th>
            <th>Unit</th>
            <th>Attributes</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="(metric, idx) in metrics" :key="idx">
            <td class="metric-name">
              {{ metric.name }}
              <span v-if="metric.description" class="metric-desc">{{ metric.description }}</span>
            </td>
            <td><span :class="['metric-type', `type-${metric.type}`]">{{ metric.type }}</span></td>
            <td class="metric-value">{{ formatMetricValue(metric.value) }}</td>
            <td class="metric-unit">{{ metric.unit || '—' }}</td>
            <td class="metric-attrs">
              <span
                v-for="[key, value] in Object.entries(metric.attributes || {})"
                :key="key"
                class="attr-badge"
              >
                {{ key }}={{ value }}
              </span>
              <span v-if="!metric.attributes || Object.keys(metric.attributes).length === 0">—</span>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'

interface SpanEvent {
  name: string
  timestamp: string
  attributes?: Record<string, string>
}

interface SpanInfo {
  name: string
  trace_id: string
  span_id: string
  parent_id?: string
  start_time: string
  end_time: string
  duration_ms: number
  status: string
  attributes?: Record<string, string>
  events?: SpanEvent[]
}

interface MetricInfo {
  name: string
  description?: string
  unit?: string
  type: string
  value: number
  attributes?: Record<string, string>
  timestamp: string
}

interface OtelData {
  spans: SpanInfo[]
  metrics: MetricInfo[]
}

const props = defineProps<{
  data: unknown
  collectorName: string
}>()

const activeTab = ref<'traces' | 'metrics'>('traces')
const selectedSpan = ref<SpanInfo | null>(null)

const otelData = computed<OtelData>(() => {
  return (props.data || { spans: [], metrics: [] }) as OtelData
})

const spans = computed(() => otelData.value.spans || [])
const metrics = computed(() => otelData.value.metrics || [])

// Sort spans: parents first, then children, preserving start time order
const sortedSpans = computed(() => {
  const spansArr = [...spans.value]
  // Sort by start_time
  spansArr.sort((a, b) => {
    const aTime = new Date(a.start_time).getTime()
    const bTime = new Date(b.start_time).getTime()
    return aTime - bTime
  })
  return spansArr
})

// Calculate the total time window for the waterfall
const timeWindow = computed(() => {
  if (spans.value.length === 0) return { start: 0, end: 1, duration: 1 }
  let minStart = Infinity
  let maxEnd = -Infinity
  for (const span of spans.value) {
    const start = new Date(span.start_time).getTime()
    const end = new Date(span.end_time).getTime()
    if (start < minStart) minStart = start
    if (end > maxEnd) maxEnd = end
  }
  const duration = maxEnd - minStart
  return { start: minStart, end: maxEnd, duration: duration || 1 }
})

function getDepth(span: SpanInfo): number {
  if (!span.parent_id) return 0
  const parent = spans.value.find(s => s.span_id === span.parent_id)
  if (!parent) return 0
  return getDepth(parent) + 1
}

function barLeft(span: SpanInfo): string {
  const start = new Date(span.start_time).getTime()
  const offset = start - timeWindow.value.start
  const pct = (offset / timeWindow.value.duration) * 100
  return `${Math.max(0, pct)}%`
}

function barWidth(span: SpanInfo): string {
  const pct = (span.duration_ms / (timeWindow.value.duration || 1)) * 100
  return `${Math.max(1, pct)}%`
}

function formatDuration(ms: number | undefined): string {
  if (!ms) return '0ms'
  if (ms < 1) return `${(ms * 1000).toFixed(0)}us`
  if (ms < 1000) return `${ms.toFixed(2)}ms`
  return `${(ms / 1000).toFixed(3)}s`
}

function formatTimestamp(ts: string | undefined): string {
  if (!ts) return '—'
  return new Date(ts).toLocaleTimeString()
}

function formatMetricValue(value: number): string {
  if (Number.isInteger(value)) return value.toLocaleString()
  return value.toFixed(4)
}
</script>

<style scoped>
.otel-panel {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.otel-tabs {
  display: flex;
  gap: 0;
  border-bottom: 1px solid #dee2e6;
}

.otel-tab {
  padding: 0.5rem 1.25rem;
  border: none;
  background: none;
  font-size: 0.875rem;
  font-weight: 500;
  color: #6c757d;
  cursor: pointer;
  border-bottom: 2px solid transparent;
  transition: color 0.2s, border-color 0.2s;
}

.otel-tab:hover {
  color: #212529;
}

.otel-tab.active {
  color: #1a1a2e;
  border-bottom-color: #4fc3f7;
}

.empty-state {
  padding: 2rem;
  text-align: center;
  color: #6c757d;
  background: #f8f9fa;
  border-radius: 6px;
}

/* Waterfall Trace View */
.waterfall {
  font-size: 0.85rem;
}

.waterfall-header {
  display: grid;
  grid-template-columns: 250px 80px 1fr;
  gap: 0.5rem;
  padding: 0.5rem 0;
  font-weight: 600;
  color: #495057;
  border-bottom: 1px solid #dee2e6;
  font-size: 0.8rem;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.waterfall-row {
  display: grid;
  grid-template-columns: 250px 80px 1fr;
  gap: 0.5rem;
  padding: 0.4rem 0;
  align-items: center;
  border-bottom: 1px solid #f1f3f5;
  cursor: pointer;
  transition: background 0.1s;
}

.waterfall-row:hover {
  background: #f8f9fa;
}

.span-name {
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.span-status {
  font-size: 0.7rem;
  padding: 0.1rem 0.3rem;
  border-radius: 3px;
  margin-left: 0.3rem;
}

.status-error {
  background: #ffe3e3;
  color: #c92a2a;
}

.status-ok {
  background: #d3f9d8;
  color: #2b8a3e;
}

.wf-col-duration {
  font-variant-numeric: tabular-nums;
  color: #495057;
}

.bar-track {
  position: relative;
  height: 16px;
  background: #f1f3f5;
  border-radius: 3px;
  overflow: hidden;
}

.bar-fill {
  position: absolute;
  top: 2px;
  bottom: 2px;
  background: linear-gradient(90deg, #4fc3f7, #0288d1);
  border-radius: 2px;
  min-width: 2px;
}

/* Span Detail */
.span-detail {
  margin-top: 1rem;
  padding: 1rem;
  background: #f8f9fa;
  border-radius: 6px;
}

.span-detail h4 {
  font-size: 0.95rem;
  margin-bottom: 0.75rem;
}

.span-detail h5 {
  font-size: 0.85rem;
  margin-top: 1rem;
  margin-bottom: 0.5rem;
  color: #495057;
}

.detail-table {
  width: 100%;
  border-collapse: collapse;
}

.detail-table th {
  text-align: left;
  padding: 0.3rem 1rem 0.3rem 0;
  font-size: 0.8rem;
  font-weight: 500;
  color: #6c757d;
  width: 120px;
}

.detail-table td {
  padding: 0.3rem 0;
  font-size: 0.8rem;
}

.detail-table code {
  background: #e9ecef;
  padding: 0.1rem 0.3rem;
  border-radius: 3px;
  font-size: 0.75rem;
}

.event-item {
  display: flex;
  gap: 1rem;
  padding: 0.25rem 0;
  font-size: 0.8rem;
}

.event-name {
  font-weight: 500;
}

.event-time {
  color: #6c757d;
}

/* Metrics Table */
.metrics-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 0.85rem;
}

.metrics-table th {
  text-align: left;
  padding: 0.5rem 0.75rem;
  font-size: 0.8rem;
  font-weight: 600;
  color: #495057;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  border-bottom: 1px solid #dee2e6;
}

.metrics-table td {
  padding: 0.5rem 0.75rem;
  border-bottom: 1px solid #f1f3f5;
}

.metric-name {
  font-weight: 500;
}

.metric-desc {
  display: block;
  font-size: 0.75rem;
  color: #6c757d;
  font-weight: 400;
}

.metric-type {
  display: inline-block;
  padding: 0.1rem 0.4rem;
  border-radius: 3px;
  font-size: 0.75rem;
  font-weight: 500;
}

.type-gauge { background: #d0ebff; color: #1864ab; }
.type-sum { background: #d3f9d8; color: #2b8a3e; }
.type-histogram { background: #fff3bf; color: #e67700; }

.metric-value {
  font-variant-numeric: tabular-nums;
  font-weight: 600;
}

.metric-unit {
  color: #6c757d;
}

.attr-badge {
  display: inline-block;
  padding: 0.1rem 0.35rem;
  background: #e9ecef;
  border-radius: 3px;
  font-size: 0.75rem;
  margin-right: 0.25rem;
  margin-bottom: 0.15rem;
}
</style>

<template>
  <div class="logger-panel">
    <!-- Summary Bar -->
    <div class="summary-bar">
      <div class="summary-item">
        <span class="summary-value">{{ loggerData.summary.total }}</span>
        <span class="summary-label">Total</span>
      </div>
      <div class="summary-item" v-if="loggerData.summary.debug > 0">
        <span class="summary-value level-badge level-debug">{{ loggerData.summary.debug }}</span>
        <span class="summary-label">Debug</span>
      </div>
      <div class="summary-item" v-if="loggerData.summary.info > 0">
        <span class="summary-value level-badge level-info">{{ loggerData.summary.info }}</span>
        <span class="summary-label">Info</span>
      </div>
      <div class="summary-item" v-if="loggerData.summary.warn > 0">
        <span class="summary-value level-badge level-warn">{{ loggerData.summary.warn }}</span>
        <span class="summary-label">Warn</span>
      </div>
      <div class="summary-item" v-if="loggerData.summary.error > 0">
        <span class="summary-value level-badge level-error">{{ loggerData.summary.error }}</span>
        <span class="summary-label">Error</span>
      </div>
      <div class="summary-item" v-if="loggerData.summary.fatal > 0">
        <span class="summary-value level-badge level-fatal">{{ loggerData.summary.fatal }}</span>
        <span class="summary-label">Fatal</span>
      </div>
      <div class="summary-item" v-if="loggerData.truncated">
        <span class="summary-value truncated-badge">Truncated (max: {{ loggerData.max_entries }})</span>
        <span class="summary-label">Warning</span>
      </div>
    </div>

    <!-- Filter Controls -->
    <div class="filter-controls">
      <div class="level-toggles">
        <button
          v-for="level in levels"
          :key="level"
          :class="['toggle-btn', `toggle-${level.toLowerCase()}`, { active: activeLevels[level] }]"
          @click="activeLevels[level] = !activeLevels[level]"
        >
          {{ level }}
        </button>
      </div>
      <input
        v-model="searchText"
        type="text"
        placeholder="Search messages..."
        class="search-input"
      />
    </div>

    <!-- Log Entry List -->
    <div v-if="filteredEntries.length > 0" class="log-list">
      <div
        v-for="(entry, index) in filteredEntries"
        :key="index"
        :class="['log-entry', { 'entry-error': entry.level === 'ERROR' || entry.level === 'FATAL' }]"
      >
        <div class="entry-header">
          <span class="entry-timestamp">+{{ relativeTimestamp(entry.timestamp) }}ms</span>
          <span :class="['entry-level', `level-${entry.level.toLowerCase()}`]">{{ entry.level }}</span>
          <span class="entry-source">[{{ entry.source }}]</span>
          <span class="entry-message">{{ entry.message }}</span>
        </div>
        <div v-if="entry.caller" class="entry-caller">{{ entry.caller }}</div>
        <div v-if="entry.attributes && Object.keys(entry.attributes).length > 0" class="entry-expandable">
          <button class="expand-btn" @click="toggleExpand(index, 'attrs')">
            {{ isExpanded(index, 'attrs') ? '- Attributes' : '+ Attributes' }}
          </button>
          <table v-if="isExpanded(index, 'attrs')" class="attrs-table">
            <tbody>
              <tr v-for="(value, key) in entry.attributes" :key="key">
                <th>{{ key }}</th>
                <td>{{ formatAttrValue(value) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <div v-if="entry.stack" class="entry-expandable">
          <button class="expand-btn" @click="toggleExpand(index, 'stack')">
            {{ isExpanded(index, 'stack') ? '- Stack Trace' : '+ Stack Trace' }}
          </button>
          <pre v-if="isExpanded(index, 'stack')" class="stack-trace">{{ entry.stack }}</pre>
        </div>
      </div>
    </div>

    <!-- Empty State -->
    <div v-else class="empty-state">
      No log entries captured
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, reactive } from 'vue'

interface LogEntry {
  timestamp: string
  level: string
  message: string
  source: string
  attributes?: Record<string, any>
  caller?: string
  stack?: string
}

interface LogSummary {
  total: number
  debug: number
  info: number
  warn: number
  error: number
  fatal: number
}

interface LoggerData {
  entries: LogEntry[]
  summary: LogSummary
  truncated: boolean
  max_entries: number
}

const props = defineProps<{
  data: unknown
  collectorName: string
}>()

const levels = ['DEBUG', 'INFO', 'WARN', 'ERROR', 'FATAL'] as const

const activeLevels = reactive<Record<string, boolean>>({
  DEBUG: true,
  INFO: true,
  WARN: true,
  ERROR: true,
  FATAL: true,
})

const searchText = ref('')
const expandedSections = reactive<Record<string, boolean>>({})

const loggerData = computed<LoggerData>(() => {
  const d = (props.data || {}) as LoggerData
  return {
    entries: d.entries || [],
    summary: d.summary || { total: 0, debug: 0, info: 0, warn: 0, error: 0, fatal: 0 },
    truncated: d.truncated || false,
    max_entries: d.max_entries || 0,
  }
})

const firstTimestamp = computed(() => {
  if (loggerData.value.entries.length === 0) return 0
  return new Date(loggerData.value.entries[0].timestamp).getTime()
})

const filteredEntries = computed(() => {
  const search = searchText.value.toLowerCase()
  return loggerData.value.entries.filter(entry => {
    if (!activeLevels[entry.level]) return false
    if (search) {
      const messageMatch = entry.message.toLowerCase().includes(search)
      const attrsMatch = entry.attributes
        ? JSON.stringify(entry.attributes).toLowerCase().includes(search)
        : false
      if (!messageMatch && !attrsMatch) return false
    }
    return true
  })
})

function relativeTimestamp(timestamp: string): string {
  const ms = new Date(timestamp).getTime() - firstTimestamp.value
  return ms.toFixed(0)
}

function toggleExpand(index: number, section: string): void {
  const key = `${index}:${section}`
  expandedSections[key] = !expandedSections[key]
}

function isExpanded(index: number, section: string): boolean {
  return !!expandedSections[`${index}:${section}`]
}

function formatAttrValue(value: any): string {
  if (typeof value === 'object') return JSON.stringify(value)
  return String(value)
}
</script>

<style scoped>
.logger-panel {
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
  align-items: flex-start;
}

.summary-item {
  display: flex;
  flex-direction: column;
  align-items: center;
}

.summary-value {
  font-size: 1.1rem;
  font-weight: 700;
  color: #1a1a2e;
  font-variant-numeric: tabular-nums;
}

.summary-label {
  font-size: 0.7rem;
  color: #6c757d;
  margin-top: 0.15rem;
  text-transform: uppercase;
  letter-spacing: 0.03em;
}

.level-badge {
  font-size: 0.85rem;
  padding: 0.1rem 0.5rem;
  border-radius: 10px;
  font-weight: 600;
}

.level-debug {
  background: #e9ecef;
  color: #495057;
}

.level-info {
  background: #d0ebff;
  color: #1864ab;
}

.level-warn {
  background: #fff3cd;
  color: #856404;
}

.level-error {
  background: #fde8e8;
  color: #c0392b;
}

.level-fatal {
  background: #f5c6cb;
  color: #721c24;
}

.truncated-badge {
  font-size: 0.75rem;
  padding: 0.2rem 0.5rem;
  border-radius: 4px;
  background: #fff3cd;
  color: #856404;
  font-weight: 600;
}

/* Filter Controls */
.filter-controls {
  display: flex;
  gap: 0.75rem;
  align-items: center;
  flex-wrap: wrap;
}

.level-toggles {
  display: flex;
  gap: 0.25rem;
}

.toggle-btn {
  padding: 0.3rem 0.6rem;
  border: 1px solid #dee2e6;
  border-radius: 4px;
  background: #fff;
  font-size: 0.75rem;
  font-weight: 500;
  cursor: pointer;
  opacity: 0.4;
  transition: opacity 0.15s, background 0.15s;
}

.toggle-btn.active {
  opacity: 1;
}

.toggle-debug.active {
  background: #e9ecef;
  border-color: #adb5bd;
  color: #495057;
}

.toggle-info.active {
  background: #d0ebff;
  border-color: #74c0fc;
  color: #1864ab;
}

.toggle-warn.active {
  background: #fff3cd;
  border-color: #ffc107;
  color: #856404;
}

.toggle-error.active {
  background: #fde8e8;
  border-color: #e74c3c;
  color: #c0392b;
}

.toggle-fatal.active {
  background: #f5c6cb;
  border-color: #a71d2a;
  color: #721c24;
}

.search-input {
  flex: 1;
  min-width: 150px;
  padding: 0.4rem 0.75rem;
  border: 1px solid #dee2e6;
  border-radius: 6px;
  font-size: 0.83rem;
  outline: none;
  transition: border-color 0.15s;
}

.search-input:focus {
  border-color: #1a1a2e;
}

/* Log List */
.log-list {
  max-height: 400px;
  overflow-y: auto;
  border: 1px solid #e9ecef;
  border-radius: 6px;
}

.log-entry {
  padding: 0.5rem 0.75rem;
  border-bottom: 1px solid #f1f3f5;
}

.log-entry:last-child {
  border-bottom: none;
}

.log-entry.entry-error {
  border-left: 3px solid #c0392b;
}

.entry-header {
  display: flex;
  align-items: baseline;
  gap: 0.5rem;
  flex-wrap: wrap;
}

.entry-timestamp {
  font-size: 0.75rem;
  font-family: 'SF Mono', Monaco, 'Cascadia Mono', monospace;
  color: #6c757d;
  white-space: nowrap;
}

.entry-level {
  font-size: 0.7rem;
  font-weight: 600;
  padding: 0.05rem 0.35rem;
  border-radius: 8px;
  white-space: nowrap;
}

.entry-source {
  font-size: 0.75rem;
  font-family: 'SF Mono', Monaco, 'Cascadia Mono', monospace;
  color: #6c757d;
}

.entry-message {
  font-size: 0.83rem;
  color: #1a1a2e;
  word-break: break-word;
}

.entry-caller {
  font-size: 0.75rem;
  color: #adb5bd;
  font-family: 'SF Mono', Monaco, 'Cascadia Mono', monospace;
  margin-top: 0.2rem;
}

/* Expandable Sections */
.entry-expandable {
  margin-top: 0.3rem;
}

.expand-btn {
  background: none;
  border: none;
  font-size: 0.75rem;
  color: #495057;
  cursor: pointer;
  padding: 0.1rem 0;
  font-weight: 500;
}

.expand-btn:hover {
  color: #1a1a2e;
}

.attrs-table {
  width: 100%;
  border-collapse: collapse;
  margin-top: 0.25rem;
  font-size: 0.8rem;
}

.attrs-table th {
  text-align: left;
  padding: 0.2rem 0.5rem;
  font-weight: 500;
  color: #6c757d;
  width: 140px;
  border-bottom: 1px solid #f1f3f5;
  font-family: 'SF Mono', Monaco, 'Cascadia Mono', monospace;
}

.attrs-table td {
  padding: 0.2rem 0.5rem;
  color: #495057;
  border-bottom: 1px solid #f1f3f5;
  font-family: 'SF Mono', Monaco, 'Cascadia Mono', monospace;
  word-break: break-all;
}

.stack-trace {
  font-size: 0.75rem;
  font-family: 'SF Mono', Monaco, 'Cascadia Mono', monospace;
  background: #f8f9fa;
  padding: 0.5rem;
  border-radius: 4px;
  margin-top: 0.25rem;
  overflow-x: auto;
  white-space: pre-wrap;
  color: #495057;
}

/* Empty State */
.empty-state {
  text-align: center;
  padding: 2rem;
  color: #6c757d;
  font-size: 0.9rem;
}
</style>

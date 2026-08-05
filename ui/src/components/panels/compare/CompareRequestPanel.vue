<template>
  <div class="compare-request-panel">
    <!-- Metadata comparison -->
    <section class="compare-section">
      <h3 class="section-title">Request Metadata</h3>
      <table class="compare-table">
        <thead>
          <tr>
            <th>Field</th>
            <th>Profile A</th>
            <th>Profile B</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="field in metadataFields" :key="field.label" :class="{ 'diff-changed': field.differs }">
            <td class="field-name">{{ field.label }}</td>
            <td>{{ field.valueA }}</td>
            <td>{{ field.valueB }}</td>
          </tr>
        </tbody>
      </table>
    </section>

    <!-- Response comparison -->
    <section class="compare-section">
      <h3 class="section-title">Response</h3>
      <table class="compare-table">
        <thead>
          <tr>
            <th>Field</th>
            <th>Profile A</th>
            <th>Profile B</th>
          </tr>
        </thead>
        <tbody>
          <tr :class="{ 'diff-changed': reqA.status_code !== reqB.status_code }">
            <td class="field-name">Status Code</td>
            <td>
              <span :class="['status-badge', statusClass(reqA.status_code)]">{{ reqA.status_code || '—' }}</span>
            </td>
            <td>
              <span :class="['status-badge', statusClass(reqB.status_code)]">{{ reqB.status_code || '—' }}</span>
            </td>
          </tr>
          <tr :class="{ 'diff-changed': reqA.response_size !== reqB.response_size }">
            <td class="field-name">Response Size</td>
            <td>{{ reqA.response_size != null ? formatSize(reqA.response_size) : '—' }}</td>
            <td>{{ reqB.response_size != null ? formatSize(reqB.response_size) : '—' }}</td>
          </tr>
        </tbody>
      </table>
    </section>

    <!-- Headers diff -->
    <section class="compare-section" v-if="headersDiff.common.length || headersDiff.onlyA.length || headersDiff.onlyB.length">
      <h3 class="section-title">Request Headers</h3>
      <table class="compare-table headers-table">
        <thead>
          <tr>
            <th>Header</th>
            <th>Profile A</th>
            <th>Profile B</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="entry in headersDiff.common"
            :key="'c-' + entry.key"
            :class="{ 'diff-changed': entry.changed }"
          >
            <td class="field-name">{{ entry.key }}</td>
            <td class="header-value">{{ formatHeaderValue(entry.valueA) }}</td>
            <td class="header-value">{{ formatHeaderValue(entry.valueB) }}</td>
          </tr>
          <tr v-for="entry in headersDiff.onlyA" :key="'a-' + entry.key" class="diff-removed">
            <td class="field-name">{{ entry.key }}</td>
            <td class="header-value">{{ formatHeaderValue(entry.value) }}</td>
            <td class="header-value absent">—</td>
          </tr>
          <tr v-for="entry in headersDiff.onlyB" :key="'b-' + entry.key" class="diff-added">
            <td class="field-name">{{ entry.key }}</td>
            <td class="header-value absent">—</td>
            <td class="header-value">{{ formatHeaderValue(entry.value) }}</td>
          </tr>
        </tbody>
      </table>
    </section>

    <!-- Query params diff -->
    <section class="compare-section" v-if="paramsDiff.common.length || paramsDiff.onlyA.length || paramsDiff.onlyB.length">
      <h3 class="section-title">Query Parameters</h3>
      <table class="compare-table">
        <thead>
          <tr>
            <th>Param</th>
            <th>Profile A</th>
            <th>Profile B</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="entry in paramsDiff.common"
            :key="'pc-' + entry.key"
            :class="{ 'diff-changed': entry.changed }"
          >
            <td class="field-name">{{ entry.key }}</td>
            <td>{{ formatParamValue(entry.valueA) }}</td>
            <td>{{ formatParamValue(entry.valueB) }}</td>
          </tr>
          <tr v-for="entry in paramsDiff.onlyA" :key="'pa-' + entry.key" class="diff-removed">
            <td class="field-name">{{ entry.key }}</td>
            <td>{{ formatParamValue(entry.value) }}</td>
            <td class="absent">—</td>
          </tr>
          <tr v-for="entry in paramsDiff.onlyB" :key="'pb-' + entry.key" class="diff-added">
            <td class="field-name">{{ entry.key }}</td>
            <td class="absent">—</td>
            <td>{{ formatParamValue(entry.value) }}</td>
          </tr>
        </tbody>
      </table>
    </section>

    <!-- Body diff -->
    <section class="compare-section" v-if="reqA.body || reqB.body">
      <h3 class="section-title">Request Body</h3>
      <div class="body-columns">
        <div class="body-col">
          <div class="body-col-label">Profile A</div>
          <pre v-if="reqA.body" class="body-content">{{ formatBody(reqA.body, reqA.content_type) }}</pre>
          <div v-else class="no-data">Body not captured</div>
        </div>
        <div class="body-col">
          <div class="body-col-label">Profile B</div>
          <pre v-if="reqB.body" class="body-content">{{ formatBody(reqB.body, reqB.content_type) }}</pre>
          <div v-else class="no-data">Body not captured</div>
        </div>
      </div>
    </section>

    <!-- Response headers diff -->
    <section class="compare-section" v-if="responseHeadersDiff.common.length || responseHeadersDiff.onlyA.length || responseHeadersDiff.onlyB.length">
      <h3 class="section-title">Response Headers</h3>
      <table class="compare-table headers-table">
        <thead>
          <tr>
            <th>Header</th>
            <th>Profile A</th>
            <th>Profile B</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="entry in responseHeadersDiff.common"
            :key="'rc-' + entry.key"
            :class="{ 'diff-changed': entry.changed }"
          >
            <td class="field-name">{{ entry.key }}</td>
            <td class="header-value">{{ formatHeaderValue(entry.valueA) }}</td>
            <td class="header-value">{{ formatHeaderValue(entry.valueB) }}</td>
          </tr>
          <tr v-for="entry in responseHeadersDiff.onlyA" :key="'ra-' + entry.key" class="diff-removed">
            <td class="field-name">{{ entry.key }}</td>
            <td class="header-value">{{ formatHeaderValue(entry.value) }}</td>
            <td class="header-value absent">—</td>
          </tr>
          <tr v-for="entry in responseHeadersDiff.onlyB" :key="'rb-' + entry.key" class="diff-added">
            <td class="field-name">{{ entry.key }}</td>
            <td class="header-value absent">—</td>
            <td class="header-value">{{ formatHeaderValue(entry.value) }}</td>
          </tr>
        </tbody>
      </table>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { compareMaps } from '../../../utils/compare'
import type { Profile } from '../../../api'

interface RequestData {
  method: string
  url: string
  host: string
  remote_addr: string
  proto: string
  headers: Record<string, string[]>
  query_params: Record<string, string[]>
  content_type: string
  status_code: number
  response_headers: Record<string, string[]>
  response_size: number
  body: string
  body_size: number
  body_truncated: boolean
  curl_command: string
}

const props = defineProps<{
  dataA: unknown
  dataB: unknown
  profileA: Profile
  profileB: Profile
}>()

const reqA = computed<RequestData>(() => {
  return (props.dataA || {}) as RequestData
})

const reqB = computed<RequestData>(() => {
  return (props.dataB || {}) as RequestData
})

const metadataFields = computed(() => {
  const fields = [
    { label: 'Method', valueA: reqA.value.method || '—', valueB: reqB.value.method || '—' },
    { label: 'URL', valueA: reqA.value.url || '—', valueB: reqB.value.url || '—' },
    { label: 'Host', valueA: reqA.value.host || '—', valueB: reqB.value.host || '—' },
    { label: 'Protocol', valueA: reqA.value.proto || '—', valueB: reqB.value.proto || '—' },
    { label: 'Content-Type', valueA: reqA.value.content_type || '—', valueB: reqB.value.content_type || '—' },
  ]
  return fields.map(f => ({
    ...f,
    differs: f.valueA !== f.valueB,
  }))
})

const headersDiff = computed(() => {
  return compareMaps(reqA.value.headers, reqB.value.headers)
})

const paramsDiff = computed(() => {
  return compareMaps(reqA.value.query_params, reqB.value.query_params)
})

const responseHeadersDiff = computed(() => {
  return compareMaps(reqA.value.response_headers, reqB.value.response_headers)
})

function formatHeaderValue(val: unknown): string {
  if (!val) return '—'
  if (Array.isArray(val)) return val.join(', ')
  return String(val)
}

function formatParamValue(val: unknown): string {
  if (!val) return '—'
  if (Array.isArray(val)) return val.join(', ')
  return String(val)
}

function formatSize(bytes: number): string {
  if (bytes === 0) return '0 B'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

function formatBody(body: string, contentType: string): string {
  if (!body) return ''
  // Try to pretty-print JSON
  if (contentType && contentType.includes('json')) {
    try {
      return JSON.stringify(JSON.parse(body), null, 2)
    } catch {
      return body
    }
  }
  return body
}

function statusClass(code: number): string {
  if (!code) return ''
  if (code >= 500) return 'status-5xx'
  if (code >= 400) return 'status-4xx'
  if (code >= 300) return 'status-3xx'
  return 'status-2xx'
}
</script>

<style scoped>
.compare-request-panel {
  display: flex;
  flex-direction: column;
  gap: 1.5rem;
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
  padding: 0.45rem 0.75rem;
  font-size: 0.8rem;
  border-bottom: 1px solid #f8f9fa;
  vertical-align: top;
}

.field-name {
  font-weight: 500;
  color: #495057;
  white-space: nowrap;
  width: 150px;
}

.header-value {
  word-break: break-all;
  max-width: 300px;
}

.absent {
  color: #adb5bd;
  font-style: italic;
}

.diff-changed {
  background: #fff9db;
}

.diff-added {
  background: #d3f9d8;
}

.diff-removed {
  background: #ffe3e3;
}

.body-columns {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 1rem;
}

@media (max-width: 1280px) {
  .body-columns {
    grid-template-columns: 1fr;
  }
}

.body-col {
  min-width: 0;
}

.body-col-label {
  font-size: 0.7rem;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  color: #6c757d;
  margin-bottom: 0.4rem;
}

.body-content {
  font-size: 0.75rem;
  background: #f8f9fa;
  padding: 0.75rem;
  border-radius: 4px;
  overflow-x: auto;
  white-space: pre-wrap;
  word-break: break-word;
  max-height: 300px;
  overflow-y: auto;
  margin: 0;
}

.no-data {
  color: #6c757d;
  font-style: italic;
  padding: 0.75rem;
  text-align: center;
  background: #f8f9fa;
  border-radius: 4px;
  font-size: 0.8rem;
}

.status-badge {
  display: inline-block;
  padding: 0.1rem 0.4rem;
  border-radius: 4px;
  font-size: 0.75rem;
  font-weight: 600;
}

.status-2xx { background: #d3f9d8; color: #2b8a3e; }
.status-3xx { background: #d0ebff; color: #1864ab; }
.status-4xx { background: #fff3bf; color: #e67700; }
.status-5xx { background: #ffe3e3; color: #c92a2a; }

.headers-table td {
  font-family: 'SF Mono', 'Fira Code', monospace;
  font-size: 0.75rem;
}
</style>

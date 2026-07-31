<template>
  <div class="request-panel">
    <div class="panel-section">
      <h3>Request</h3>
      <table class="info-table">
        <tbody>
          <tr>
            <th>Method</th>
            <td><span :class="['method-badge', `method-${requestData.method?.toLowerCase()}`]">{{ requestData.method }}</span></td>
          </tr>
          <tr>
            <th>URL</th>
            <td>{{ requestData.url }}</td>
          </tr>
          <tr>
            <th>Host</th>
            <td>{{ requestData.host }}</td>
          </tr>
          <tr>
            <th>Remote Address</th>
            <td>{{ requestData.remote_addr }}</td>
          </tr>
          <tr>
            <th>Protocol</th>
            <td>{{ requestData.proto }}</td>
          </tr>
          <tr>
            <th>Content-Type</th>
            <td>{{ requestData.content_type || '—' }}</td>
          </tr>
        </tbody>
      </table>
    </div>

    <div class="panel-section" v-if="requestData.query_params && Object.keys(requestData.query_params).length > 0">
      <h3>Query Parameters</h3>
      <table class="info-table">
        <tbody>
          <tr v-for="[key, values] in Object.entries(requestData.query_params)" :key="key">
            <th>{{ key }}</th>
            <td>{{ (values as string[]).join(', ') }}</td>
          </tr>
        </tbody>
      </table>
    </div>

    <div class="panel-section">
      <h3>Request Headers</h3>
      <table class="info-table">
        <tbody>
          <tr v-for="[key, values] in sortedHeaders(requestData.headers)" :key="key">
            <th>{{ key }}</th>
            <td>
              <span v-if="isRedacted(values)" class="redacted">[REDACTED]</span>
              <span v-else>{{ (values as string[]).join(', ') }}</span>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <div class="panel-section">
      <h3>Response</h3>
      <table class="info-table">
        <tbody>
          <tr>
            <th>Status Code</th>
            <td>
              <span :class="['status-badge', statusClass(requestData.status_code)]">
                {{ requestData.status_code }}
              </span>
            </td>
          </tr>
          <tr>
            <th>Response Size</th>
            <td>{{ formatBytes(requestData.response_size) }}</td>
          </tr>
        </tbody>
      </table>
    </div>

    <div class="panel-section" v-if="requestData.response_headers && Object.keys(requestData.response_headers).length > 0">
      <h3>Response Headers</h3>
      <table class="info-table">
        <tbody>
          <tr v-for="[key, values] in sortedHeaders(requestData.response_headers)" :key="key">
            <th>{{ key }}</th>
            <td>
              <span v-if="isRedacted(values)" class="redacted">[REDACTED]</span>
              <span v-else>{{ (values as string[]).join(', ') }}</span>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

interface RequestData {
  method: string
  url: string
  host: string
  remote_addr: string
  proto: string
  content_type: string
  headers: Record<string, string[]>
  query_params: Record<string, string[]> | null
  status_code: number
  response_headers: Record<string, string[]>
  response_size: number
}

const props = defineProps<{
  data: unknown
  collectorName: string
}>()

const requestData = computed<RequestData>(() => {
  return (props.data || {}) as RequestData
})

function sortedHeaders(headers: Record<string, string[]> | null): [string, string[]][] {
  if (!headers) return []
  return Object.entries(headers).sort(([a], [b]) => a.localeCompare(b))
}

function isRedacted(values: unknown): boolean {
  if (Array.isArray(values) && values.length === 1 && values[0] === '[REDACTED]') {
    return true
  }
  return false
}

function statusClass(code: number): string {
  if (code >= 500) return 'status-5xx'
  if (code >= 400) return 'status-4xx'
  if (code >= 300) return 'status-3xx'
  return 'status-2xx'
}

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(2)} MB`
}
</script>

<style scoped>
.request-panel {
  display: flex;
  flex-direction: column;
  gap: 1.5rem;
}

.panel-section h3 {
  font-size: 0.9rem;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: #495057;
  margin-bottom: 0.5rem;
}

.info-table {
  width: 100%;
  border-collapse: collapse;
}

.info-table th {
  text-align: left;
  padding: 0.4rem 1rem 0.4rem 0;
  font-size: 0.85rem;
  font-weight: 500;
  color: #6c757d;
  white-space: nowrap;
  width: 200px;
  vertical-align: top;
}

.info-table td {
  padding: 0.4rem 0;
  font-size: 0.85rem;
  word-break: break-all;
}

.info-table tr + tr {
  border-top: 1px solid #f1f3f5;
}

.redacted {
  color: #dc3545;
  font-style: italic;
  font-size: 0.8rem;
}

.method-badge {
  display: inline-block;
  padding: 0.1rem 0.4rem;
  border-radius: 3px;
  font-size: 0.75rem;
  font-weight: 600;
}

.method-get { background: #d3f9d8; color: #2b8a3e; }
.method-post { background: #d0ebff; color: #1864ab; }
.method-put { background: #fff3bf; color: #e67700; }
.method-patch { background: #fff3bf; color: #e67700; }
.method-delete { background: #ffe3e3; color: #c92a2a; }

.status-badge {
  display: inline-block;
  padding: 0.1rem 0.4rem;
  border-radius: 3px;
  font-size: 0.8rem;
  font-weight: 600;
}

.status-2xx { background: #d3f9d8; color: #2b8a3e; }
.status-3xx { background: #d0ebff; color: #1864ab; }
.status-4xx { background: #fff3bf; color: #e67700; }
.status-5xx { background: #ffe3e3; color: #c92a2a; }
</style>

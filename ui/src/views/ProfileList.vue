<template>
  <div class="profile-list">
    <div class="list-header">
      <h1>Profiles</h1>
      <div class="list-actions">
        <template v-if="selectedIds.size > 0">
          <span class="selection-count">{{ selectedIds.size }} selected</span>
          <button
            class="btn btn-primary"
            :disabled="selectedIds.size !== 2"
            @click="compareSelected"
          >
            Compare
          </button>
          <button class="btn btn-secondary" @click="clearSelection">Clear</button>
        </template>
        <button class="btn btn-secondary" @click="refresh">Refresh</button>
        <button class="btn btn-danger" @click="handlePurge">Purge</button>
        <button class="btn btn-danger btn-clear-all" :disabled="clearing" @click="handleClearAll">
          {{ clearing ? 'Clearing...' : 'Clear All' }}
        </button>
      </div>
    </div>

    <div class="filters">
      <select v-model="filters.method" @change="loadProfiles" class="filter-select">
        <option value="">All Methods</option>
        <option value="GET">GET</option>
        <option value="POST">POST</option>
        <option value="PUT">PUT</option>
        <option value="PATCH">PATCH</option>
        <option value="DELETE">DELETE</option>
      </select>
      <input
        v-model="filters.url"
        @input="debouncedLoad"
        placeholder="Filter by URL..."
        class="filter-input"
      />
      <select v-model="filters.statusGroup" @change="loadProfiles" class="filter-select">
        <option value="">All Status</option>
        <option value="2xx">2xx Success</option>
        <option value="3xx">3xx Redirect</option>
        <option value="4xx">4xx Client Error</option>
        <option value="5xx">5xx Server Error</option>
      </select>
    </div>

    <div v-if="loading" class="loading">Loading profiles...</div>
    <div v-else-if="error" class="error">{{ error }}</div>
    <div v-else-if="profiles.length === 0" class="empty">No profiles found.</div>

    <table v-else class="profiles-table">
      <thead>
        <tr>
          <th class="checkbox-col"></th>
          <th>Method</th>
          <th>URL</th>
          <th>Status</th>
          <th>Duration</th>
          <th>Timestamp</th>
          <th>Profile ID</th>
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="profile in profiles"
          :key="profile.id"
          @click="viewProfile(profile.id)"
          :class="['profile-row', { selected: selectedIds.has(profile.id) }]"
        >
          <td class="checkbox-col" @click.stop>
            <input
              type="checkbox"
              :checked="selectedIds.has(profile.id)"
              @change="toggleSelection(profile.id)"
              class="profile-checkbox"
            />
          </td>
          <td>
            <span :class="['method-badge', `method-${profile.method.toLowerCase()}`]">
              {{ profile.method }}
            </span>
          </td>
          <td class="url-cell">{{ profile.url }}</td>
          <td>
            <span :class="['status-badge', statusClass(profile.status_code)]">
              {{ profile.status_code }}
            </span>
          </td>
          <td class="duration-cell">{{ formatDuration(profile.duration) }}</td>
          <td class="timestamp-cell">{{ formatTime(profile.timestamp) }}</td>
          <td class="id-cell">
            <code>{{ profile.id }}</code>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { listProfiles, purgeProfiles, clearAllProfiles, type ProfileSummary } from '../api'

const router = useRouter()
const profiles = ref<ProfileSummary[]>([])
const loading = ref(false)
const clearing = ref(false)
const error = ref('')

// Selection state for comparison
const selectedIds = ref<Set<string>>(new Set())
const selectionOrder = ref<string[]>([])

const filters = reactive({
  method: '',
  url: '',
  statusGroup: '',
})

let debounceTimer: ReturnType<typeof setTimeout> | null = null

function debouncedLoad() {
  if (debounceTimer) clearTimeout(debounceTimer)
  debounceTimer = setTimeout(loadProfiles, 300)
}

async function loadProfiles() {
  loading.value = true
  error.value = ''
  try {
    const params: Record<string, string | number> = {}
    if (filters.method) params.method = filters.method
    if (filters.url) params.url = filters.url
    if (filters.statusGroup) {
      const base = parseInt(filters.statusGroup) * 100
      params.min_status = base
      params.max_status = base + 99
    }
    const data = await listProfiles(params)
    profiles.value = data.profiles || []
  } catch (e: unknown) {
    error.value = e instanceof Error ? e.message : 'Failed to load profiles'
  } finally {
    loading.value = false
  }
}

function refresh() {
  loadProfiles()
}

async function handlePurge() {
  if (!confirm('Purge profiles older than 24 hours?')) return
  await purgeProfiles('24h')
  loadProfiles()
}

async function handleClearAll() {
  if (!confirm('Are you sure you want to delete all profiles? This cannot be undone.')) return
  clearing.value = true
  try {
    await clearAllProfiles()
    await loadProfiles()
  } catch (e: unknown) {
    error.value = e instanceof Error ? e.message : 'Failed to clear profiles'
  } finally {
    clearing.value = false
  }
}

function toggleSelection(id: string) {
  const newSet = new Set(selectedIds.value)
  const newOrder = [...selectionOrder.value]

  if (newSet.has(id)) {
    newSet.delete(id)
    const idx = newOrder.indexOf(id)
    if (idx !== -1) newOrder.splice(idx, 1)
  } else {
    // Enforce max 2: if already 2 selected, remove the oldest
    if (newSet.size >= 2) {
      const oldest = newOrder.shift()!
      newSet.delete(oldest)
    }
    newSet.add(id)
    newOrder.push(id)
  }

  selectedIds.value = newSet
  selectionOrder.value = newOrder
}

function clearSelection() {
  selectedIds.value = new Set()
  selectionOrder.value = []
}

function compareSelected() {
  if (selectionOrder.value.length === 2) {
    const [idA, idB] = selectionOrder.value
    router.push({ name: 'profile-compare', params: { idA, idB } })
  }
}

function viewProfile(id: string) {
  router.push({ name: 'profile-detail', params: { id } })
}

function statusClass(code: number): string {
  if (code >= 500) return 'status-5xx'
  if (code >= 400) return 'status-4xx'
  if (code >= 300) return 'status-3xx'
  return 'status-2xx'
}

function formatDuration(ms: number): string {
  if (ms < 1) return '<1ms'
  if (ms < 1000) return `${Math.round(ms)}ms`
  return `${(ms / 1000).toFixed(2)}s`
}

function formatTime(timestamp: string): string {
  const date = new Date(timestamp)
  return date.toLocaleTimeString(undefined, {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  })
}

onMounted(loadProfiles)
</script>

<style scoped>
.profile-list {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.list-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.list-header h1 {
  font-size: 1.5rem;
  font-weight: 600;
}

.list-actions {
  display: flex;
  gap: 0.5rem;
  align-items: center;
}

.selection-count {
  font-size: 0.85rem;
  color: #495057;
  font-weight: 500;
}

.btn {
  padding: 0.5rem 1rem;
  border: none;
  border-radius: 6px;
  font-size: 0.875rem;
  cursor: pointer;
  font-weight: 500;
  transition: opacity 0.2s;
}

.btn:hover {
  opacity: 0.85;
}

.btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.btn-primary {
  background: #4fc3f7;
  color: #1a1a2e;
}

.btn-secondary {
  background: #6c757d;
  color: #fff;
}

.btn-danger {
  background: #dc3545;
  color: #fff;
}

.btn-danger:disabled {
  background: #dc3545;
  opacity: 0.6;
  cursor: not-allowed;
}

.btn-clear-all {
  background: #a71d2a;
}

.filters {
  display: flex;
  gap: 0.75rem;
  flex-wrap: wrap;
}

.filter-select,
.filter-input {
  padding: 0.5rem 0.75rem;
  border: 1px solid #dee2e6;
  border-radius: 6px;
  font-size: 0.875rem;
  background: #fff;
}

.filter-input {
  flex: 1;
  min-width: 200px;
}

.loading,
.error,
.empty {
  padding: 2rem;
  text-align: center;
  color: #6c757d;
  background: #fff;
  border-radius: 8px;
  border: 1px solid #dee2e6;
}

.error {
  color: #dc3545;
}

.profiles-table {
  width: 100%;
  border-collapse: collapse;
  background: #fff;
  border-radius: 8px;
  overflow: hidden;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.08);
}

.profiles-table th {
  background: #f1f3f5;
  padding: 0.75rem 1rem;
  text-align: left;
  font-size: 0.8rem;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: #495057;
}

.profiles-table td {
  padding: 0.75rem 1rem;
  border-top: 1px solid #f1f3f5;
  font-size: 0.875rem;
}

.profile-row {
  cursor: pointer;
  transition: background 0.15s;
}

.profile-row:hover {
  background: #f8f9fa;
}

.profile-row.selected {
  background: #e8f4fd;
}

.profile-row.selected:hover {
  background: #d4ecfb;
}

.checkbox-col {
  width: 40px;
  text-align: center;
}

.profile-checkbox {
  cursor: pointer;
  width: 16px;
  height: 16px;
}

.method-badge {
  display: inline-block;
  padding: 0.15rem 0.5rem;
  border-radius: 4px;
  font-size: 0.75rem;
  font-weight: 600;
  text-transform: uppercase;
}

.method-get { background: #d3f9d8; color: #2b8a3e; }
.method-post { background: #d0ebff; color: #1864ab; }
.method-put { background: #fff3bf; color: #e67700; }
.method-patch { background: #fff3bf; color: #e67700; }
.method-delete { background: #ffe3e3; color: #c92a2a; }

.status-badge {
  display: inline-block;
  padding: 0.15rem 0.5rem;
  border-radius: 4px;
  font-size: 0.8rem;
  font-weight: 600;
}

.status-2xx { background: #d3f9d8; color: #2b8a3e; }
.status-3xx { background: #d0ebff; color: #1864ab; }
.status-4xx { background: #fff3bf; color: #e67700; }
.status-5xx { background: #ffe3e3; color: #c92a2a; }

.url-cell {
  max-width: 300px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.duration-cell {
  font-variant-numeric: tabular-nums;
}

.timestamp-cell {
  white-space: nowrap;
  color: #6c757d;
}

.id-cell code {
  font-size: 0.75rem;
  background: #f1f3f5;
  padding: 0.15rem 0.4rem;
  border-radius: 3px;
  color: #495057;
}
</style>

<template>
  <div class="config-panel">
    <!-- Summary Bar -->
    <div class="summary-bar">
      <div class="summary-item">
        <span class="summary-value">{{ configData.runtime.go_version || 'N/A' }}</span>
        <span class="summary-label">Go Version</span>
      </div>
      <div class="summary-item">
        <span class="summary-value">{{ configData.runtime.goos }}/{{ configData.runtime.goarch }}</span>
        <span class="summary-label">OS / Arch</span>
      </div>
      <div class="summary-item">
        <span class="summary-value">{{ configData.runtime.num_cpu }}</span>
        <span class="summary-label">CPUs</span>
      </div>
      <div class="summary-item" v-if="configData.build.module_path">
        <span class="summary-value summary-module">{{ shortModule }}</span>
        <span class="summary-label">Module</span>
      </div>
      <div class="summary-item">
        <span :class="['summary-value', 'mask-badge', configData.mask_enabled ? 'mask-on' : 'mask-off']">
          {{ configData.mask_enabled ? 'Secrets Masked' : 'Secrets Visible' }}
        </span>
        <span class="summary-label">Masking</span>
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
        <span v-if="tab.badge" class="badge">{{ tab.badge }}</span>
      </button>
    </div>

    <!-- Tab Content: Runtime -->
    <div v-if="activeTab === 'runtime'" class="tab-content">
      <table class="info-table">
        <tbody>
          <tr>
            <th>Go Version</th>
            <td>{{ configData.runtime.go_version }}</td>
          </tr>
          <tr>
            <th>OS</th>
            <td>{{ configData.runtime.goos }}</td>
          </tr>
          <tr>
            <th>Architecture</th>
            <td>{{ configData.runtime.goarch }}</td>
          </tr>
          <tr>
            <th>CPUs</th>
            <td>{{ configData.runtime.num_cpu }}</td>
          </tr>
          <tr>
            <th>GOMAXPROCS</th>
            <td>{{ configData.runtime.gomaxprocs }}</td>
          </tr>
          <tr>
            <th>Compiler</th>
            <td>{{ configData.runtime.compiler }}</td>
          </tr>
          <tr v-if="configData.build.module_path">
            <th>Module</th>
            <td>{{ configData.build.module_path }}</td>
          </tr>
          <tr v-if="configData.build.go_version">
            <th>Build Go Version</th>
            <td>{{ configData.build.go_version }}</td>
          </tr>
          <tr v-if="configData.build.vcs_revision">
            <th>VCS Revision</th>
            <td>
              <code>{{ configData.build.vcs_revision.substring(0, 12) }}</code>
              <span v-if="configData.build.vcs_modified" class="badge badge-warning">dirty</span>
            </td>
          </tr>
          <tr v-if="configData.build.vcs_time">
            <th>VCS Time</th>
            <td>{{ configData.build.vcs_time }}</td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- Tab Content: Environment Variables -->
    <div v-if="activeTab === 'environment'" class="tab-content">
      <div class="search-bar">
        <input
          v-model="envSearch"
          type="text"
          placeholder="Filter by key name..."
          class="search-input"
        />
        <span class="search-count">{{ filteredEnvEntries.length }} entries</span>
      </div>
      <table class="entries-table" v-if="filteredEnvEntries.length > 0">
        <thead>
          <tr>
            <th class="col-key">Key</th>
            <th class="col-value">Value</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="entry in filteredEnvEntries" :key="entry.key">
            <td class="cell-key">{{ entry.key }}</td>
            <td class="cell-value">
              <span v-if="entry.value === '********'" class="masked-value">
                <span class="lock-icon">&#128274;</span> {{ entry.value }}
              </span>
              <code v-else>{{ entry.value }}</code>
            </td>
          </tr>
        </tbody>
      </table>
      <div v-else class="empty-state">
        No environment variables match the filter.
      </div>
    </div>

    <!-- Tab Content: .env File(s) -->
    <div v-if="activeTab === 'dotenv'" class="tab-content">
      <div v-for="source in dotenvSources" :key="source.name" class="source-group">
        <div class="source-header">
          <span class="source-name">{{ source.name }}</span>
          <span class="source-count">{{ source.entries.length }} entries</span>
        </div>
        <table class="entries-table">
          <thead>
            <tr>
              <th class="col-key">Key</th>
              <th class="col-value">Value</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="entry in source.entries" :key="entry.key">
              <td class="cell-key">{{ entry.key }}</td>
              <td class="cell-value">
                <span v-if="entry.value === '********'" class="masked-value">
                  <span class="lock-icon">&#128274;</span> {{ entry.value }}
                </span>
                <code v-else>{{ entry.value }}</code>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <div v-if="dotenvSources.length === 0" class="empty-state">
        No .env files were loaded.
      </div>
    </div>

    <!-- Tab Content: Dependencies -->
    <div v-if="activeTab === 'dependencies'" class="tab-content">
      <table class="entries-table" v-if="configData.dependencies && configData.dependencies.length > 0">
        <thead>
          <tr>
            <th class="col-path">Module Path</th>
            <th class="col-version">Version</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="dep in configData.dependencies" :key="dep.path">
            <td class="cell-key">{{ dep.path }}</td>
            <td><code>{{ dep.version }}</code></td>
          </tr>
        </tbody>
      </table>
      <div v-else class="empty-state">
        No dependency information available.
      </div>
    </div>

    <!-- Tab Content: Custom Sources -->
    <div v-if="isCustomTab(activeTab)" class="tab-content">
      <div v-for="source in customSourcesForTab(activeTab)" :key="source.name">
        <table class="entries-table">
          <thead>
            <tr>
              <th class="col-key">Key</th>
              <th class="col-value">Value</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="entry in source.entries" :key="entry.key">
              <td class="cell-key">{{ entry.key }}</td>
              <td class="cell-value">
                <span v-if="entry.value === '********'" class="masked-value">
                  <span class="lock-icon">&#128274;</span> {{ entry.value }}
                </span>
                <code v-else>{{ entry.value }}</code>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'

interface ConfigEntry {
  key: string
  value: string
  source: string
  default?: string
  required?: boolean
}

interface RuntimeInfo {
  go_version: string
  goos: string
  goarch: string
  num_cpu: number
  gomaxprocs: number
  compiler: string
}

interface BuildInfo {
  module_path: string
  go_version: string
  vcs_revision: string
  vcs_time: string
  vcs_modified: boolean
}

interface DependencyInfo {
  path: string
  version: string
}

interface ConfigSource {
  name: string
  entries: ConfigEntry[]
}

interface ConfigData {
  runtime: RuntimeInfo
  build: BuildInfo
  dependencies: DependencyInfo[]
  sources: ConfigSource[]
  mask_enabled: boolean
}

const props = defineProps<{
  data: unknown
  collectorName: string
}>()

const activeTab = ref('runtime')
const envSearch = ref('')

const configData = computed<ConfigData>(() => {
  const d = (props.data || {}) as ConfigData
  return {
    runtime: d.runtime || { go_version: '', goos: '', goarch: '', num_cpu: 0, gomaxprocs: 0, compiler: '' },
    build: d.build || { module_path: '', go_version: '', vcs_revision: '', vcs_time: '', vcs_modified: false },
    dependencies: d.dependencies || [],
    sources: d.sources || [],
    mask_enabled: d.mask_enabled || false,
  }
})

const shortModule = computed(() => {
  const mod = configData.value.build.module_path
  if (!mod) return ''
  const parts = mod.split('/')
  if (parts.length > 2) {
    return parts.slice(-2).join('/')
  }
  return mod
})

// Separate sources into categories
const envSources = computed(() =>
  configData.value.sources.filter(s => s.name === 'environment')
)

const dotenvSources = computed(() =>
  configData.value.sources.filter(s => s.name.startsWith('.') || s.name.endsWith('.env'))
)

const customSources = computed(() =>
  configData.value.sources.filter(s => s.name !== 'environment' && !s.name.startsWith('.') && !s.name.endsWith('.env'))
)

// Filtered environment entries (search)
const allEnvEntries = computed(() => {
  const entries: ConfigEntry[] = []
  for (const src of envSources.value) {
    entries.push(...src.entries)
  }
  return entries
})

const filteredEnvEntries = computed(() => {
  if (!envSearch.value) return allEnvEntries.value
  const search = envSearch.value.toLowerCase()
  return allEnvEntries.value.filter(e => e.key.toLowerCase().includes(search))
})

// Available tabs
const availableTabs = computed(() => {
  const tabs: { id: string; label: string; badge: string | null }[] = [
    { id: 'runtime', label: 'Runtime', badge: null },
  ]

  if (envSources.value.length > 0 && allEnvEntries.value.length > 0) {
    tabs.push({ id: 'environment', label: 'Environment', badge: String(allEnvEntries.value.length) })
  }

  if (dotenvSources.value.length > 0) {
    const count = dotenvSources.value.reduce((sum, s) => sum + s.entries.length, 0)
    if (count > 0) {
      tabs.push({ id: 'dotenv', label: '.env File', badge: String(count) })
    }
  }

  if (configData.value.dependencies && configData.value.dependencies.length > 0) {
    tabs.push({ id: 'dependencies', label: 'Dependencies', badge: String(configData.value.dependencies.length) })
  }

  // Custom reader tabs
  for (const src of customSources.value) {
    tabs.push({ id: `custom:${src.name}`, label: src.name, badge: String(src.entries.length) })
  }

  return tabs
})

function isCustomTab(tabId: string): boolean {
  return tabId.startsWith('custom:')
}

function customSourcesForTab(tabId: string): ConfigSource[] {
  const name = tabId.replace('custom:', '')
  return configData.value.sources.filter(s => s.name === name)
}
</script>

<style scoped>
.config-panel {
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

.summary-module {
  font-size: 0.9rem;
  font-family: 'SF Mono', Monaco, 'Cascadia Mono', monospace;
}

.summary-label {
  font-size: 0.7rem;
  color: #6c757d;
  margin-top: 0.15rem;
  text-transform: uppercase;
  letter-spacing: 0.03em;
}

.mask-badge {
  font-size: 0.75rem;
  padding: 0.2rem 0.5rem;
  border-radius: 4px;
  font-weight: 600;
}

.mask-on {
  background: #fff3cd;
  color: #856404;
}

.mask-off {
  background: #d4edda;
  color: #155724;
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
  font-size: 0.7rem;
  padding: 0.1rem 0.4rem;
  border-radius: 4px;
  margin-left: 0.5rem;
  font-weight: 600;
}

/* Tab Content */
.tab-content {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

/* Info Table (Runtime) */
.info-table {
  width: 100%;
  border-collapse: collapse;
}

.info-table th {
  text-align: left;
  padding: 0.5rem 1rem;
  font-weight: 500;
  font-size: 0.85rem;
  color: #6c757d;
  width: 160px;
  border-bottom: 1px solid #f1f3f5;
}

.info-table td {
  padding: 0.5rem 1rem;
  font-size: 0.85rem;
  color: #1a1a2e;
  border-bottom: 1px solid #f1f3f5;
}

.info-table code {
  font-family: 'SF Mono', Monaco, 'Cascadia Mono', monospace;
  font-size: 0.8rem;
  background: #f1f3f5;
  padding: 0.1rem 0.3rem;
  border-radius: 3px;
}

/* Entries Table (Env, .env, Dependencies) */
.entries-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 0.83rem;
}

.entries-table thead th {
  text-align: left;
  padding: 0.4rem 0.75rem;
  font-weight: 600;
  font-size: 0.75rem;
  color: #6c757d;
  text-transform: uppercase;
  letter-spacing: 0.03em;
  border-bottom: 2px solid #e9ecef;
  background: #f8f9fa;
}

.entries-table tbody tr {
  border-bottom: 1px solid #f1f3f5;
}

.entries-table tbody tr:hover {
  background: #f8f9fa;
}

.entries-table td {
  padding: 0.4rem 0.75rem;
  vertical-align: top;
}

.col-key {
  width: 280px;
}

.col-path {
  width: 60%;
}

.col-version {
  width: 40%;
}

.cell-key {
  font-family: 'SF Mono', Monaco, 'Cascadia Mono', monospace;
  font-size: 0.8rem;
  font-weight: 500;
  color: #1a1a2e;
}

.cell-value code {
  font-family: 'SF Mono', Monaco, 'Cascadia Mono', monospace;
  font-size: 0.8rem;
  color: #495057;
  word-break: break-all;
}

.masked-value {
  color: #856404;
  font-size: 0.8rem;
  font-style: italic;
}

.lock-icon {
  font-size: 0.7rem;
}

/* Search Bar */
.search-bar {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.search-input {
  flex: 1;
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

.search-count {
  font-size: 0.75rem;
  color: #6c757d;
  white-space: nowrap;
}

/* Source Group (.env files) */
.source-group {
  border: 1px solid #e9ecef;
  border-radius: 6px;
  overflow: hidden;
}

.source-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0.5rem 0.75rem;
  background: #f1f3f5;
  border-bottom: 1px solid #e9ecef;
}

.source-name {
  font-weight: 600;
  font-size: 0.85rem;
  font-family: 'SF Mono', Monaco, 'Cascadia Mono', monospace;
  color: #1a1a2e;
}

.source-count {
  font-size: 0.75rem;
  color: #6c757d;
}

/* Empty State */
.empty-state {
  text-align: center;
  padding: 2rem;
  color: #6c757d;
  font-size: 0.9rem;
}
</style>

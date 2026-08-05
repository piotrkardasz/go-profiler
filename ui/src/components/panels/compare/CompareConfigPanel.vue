<template>
  <div class="compare-config-panel">
    <div v-if="!dataA && !dataB" class="no-data">No config data available for either profile.</div>

    <template v-else>
      <!-- Runtime comparison -->
      <section class="compare-section">
        <h3 class="section-title">Runtime</h3>
        <div v-if="!hasRuntimeDifferences" class="identical-notice">
          Runtime configuration is identical.
        </div>
        <table v-else class="compare-table">
          <thead>
            <tr>
              <th>Field</th>
              <th>Profile A</th>
              <th>Profile B</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="field in runtimeFields" :key="field.label" :class="{ 'diff-changed': field.differs }">
              <td class="field-name">{{ field.label }}</td>
              <td>{{ field.valueA }}</td>
              <td>{{ field.valueB }}</td>
            </tr>
          </tbody>
        </table>
      </section>

      <!-- Build info comparison -->
      <section class="compare-section" v-if="hasBuildDifferences">
        <h3 class="section-title">Build</h3>
        <table class="compare-table">
          <thead>
            <tr>
              <th>Field</th>
              <th>Profile A</th>
              <th>Profile B</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="field in buildFields" :key="field.label" :class="{ 'diff-changed': field.differs }">
              <td class="field-name">{{ field.label }}</td>
              <td class="mono-value">{{ field.valueA }}</td>
              <td class="mono-value">{{ field.valueB }}</td>
            </tr>
          </tbody>
        </table>
      </section>

      <!-- Dependencies diff -->
      <section class="compare-section" v-if="depsDiff.added.length || depsDiff.removed.length || depsDiff.changed.length">
        <h3 class="section-title">Dependencies</h3>
        <table class="compare-table">
          <thead>
            <tr>
              <th>Module</th>
              <th>Profile A</th>
              <th>Profile B</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="dep in depsDiff.changed" :key="'c-' + dep.path" class="diff-changed">
              <td class="field-name">{{ dep.path }}</td>
              <td class="mono-value">{{ dep.versionA }}</td>
              <td class="mono-value">{{ dep.versionB }}</td>
            </tr>
            <tr v-for="dep in depsDiff.removed" :key="'r-' + dep.path" class="diff-removed">
              <td class="field-name">{{ dep.path }}</td>
              <td class="mono-value">{{ dep.version }}</td>
              <td class="absent">—</td>
            </tr>
            <tr v-for="dep in depsDiff.added" :key="'a-' + dep.path" class="diff-added">
              <td class="field-name">{{ dep.path }}</td>
              <td class="absent">—</td>
              <td class="mono-value">{{ dep.version }}</td>
            </tr>
          </tbody>
        </table>
      </section>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { Profile } from '../../../api'

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

interface Dependency {
  path: string
  version: string
}

interface ConfigData {
  runtime: RuntimeInfo
  build: BuildInfo
  dependencies: Dependency[] | null
  sources: unknown[]
}

const props = defineProps<{
  dataA: unknown
  dataB: unknown
  profileA: Profile
  profileB: Profile
}>()

const configA = computed<ConfigData>(() => {
  const d = (props.dataA || {}) as ConfigData
  return {
    runtime: d.runtime || {} as RuntimeInfo,
    build: d.build || {} as BuildInfo,
    dependencies: d.dependencies || null,
    sources: d.sources || [],
  }
})

const configB = computed<ConfigData>(() => {
  const d = (props.dataB || {}) as ConfigData
  return {
    runtime: d.runtime || {} as RuntimeInfo,
    build: d.build || {} as BuildInfo,
    dependencies: d.dependencies || null,
    sources: d.sources || [],
  }
})

const runtimeFields = computed(() => {
  const rA = configA.value.runtime
  const rB = configB.value.runtime
  const fields = [
    { label: 'Go Version', valueA: rA.go_version || '—', valueB: rB.go_version || '—' },
    { label: 'OS', valueA: rA.goos || '—', valueB: rB.goos || '—' },
    { label: 'Arch', valueA: rA.goarch || '—', valueB: rB.goarch || '—' },
    { label: 'CPUs', valueA: String(rA.num_cpu || '—'), valueB: String(rB.num_cpu || '—') },
    { label: 'GOMAXPROCS', valueA: String(rA.gomaxprocs || '—'), valueB: String(rB.gomaxprocs || '—') },
    { label: 'Compiler', valueA: rA.compiler || '—', valueB: rB.compiler || '—' },
  ]
  return fields.map(f => ({ ...f, differs: f.valueA !== f.valueB }))
})

const hasRuntimeDifferences = computed(() => {
  return runtimeFields.value.some(f => f.differs)
})

const buildFields = computed(() => {
  const bA = configA.value.build
  const bB = configB.value.build
  const fields = [
    { label: 'Module Path', valueA: bA.module_path || '—', valueB: bB.module_path || '—' },
    { label: 'VCS Revision', valueA: bA.vcs_revision || '—', valueB: bB.vcs_revision || '—' },
    { label: 'VCS Time', valueA: bA.vcs_time || '—', valueB: bB.vcs_time || '—' },
    { label: 'VCS Modified', valueA: String(bA.vcs_modified ?? '—'), valueB: String(bB.vcs_modified ?? '—') },
  ]
  return fields.map(f => ({ ...f, differs: f.valueA !== f.valueB }))
})

const hasBuildDifferences = computed(() => {
  return buildFields.value.some(f => f.differs)
})

const depsDiff = computed(() => {
  const depsA = configA.value.dependencies || []
  const depsB = configB.value.dependencies || []

  const mapA = new Map<string, string>()
  const mapB = new Map<string, string>()

  for (const dep of depsA) {
    mapA.set(dep.path, dep.version)
  }
  for (const dep of depsB) {
    mapB.set(dep.path, dep.version)
  }

  const added: Array<{ path: string; version: string }> = []
  const removed: Array<{ path: string; version: string }> = []
  const changed: Array<{ path: string; versionA: string; versionB: string }> = []

  for (const [path, version] of mapA) {
    if (!mapB.has(path)) {
      removed.push({ path, version })
    } else if (mapB.get(path) !== version) {
      changed.push({ path, versionA: version, versionB: mapB.get(path)! })
    }
  }

  for (const [path, version] of mapB) {
    if (!mapA.has(path)) {
      added.push({ path, version })
    }
  }

  return { added, removed, changed }
})
</script>

<style scoped>
.compare-config-panel {
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

.identical-notice {
  color: #6c757d;
  font-size: 0.85rem;
  padding: 0.75rem;
  background: #f8f9fa;
  border-radius: 4px;
  text-align: center;
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
}

.field-name {
  font-weight: 500;
  color: #495057;
  white-space: nowrap;
}

.mono-value {
  font-family: 'SF Mono', 'Fira Code', monospace;
  font-size: 0.75rem;
  word-break: break-all;
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
</style>

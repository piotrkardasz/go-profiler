<template>
  <div class="compare-logger-panel">
    <div v-if="!dataA && !dataB" class="no-data">No logger data available for either profile.</div>

    <template v-else>
      <section class="compare-section">
        <h3 class="section-title">Log Summary</h3>
        <table class="compare-table">
          <thead>
            <tr>
              <th>Level</th>
              <th class="val-col">Profile A</th>
              <th class="val-col">Profile B</th>
              <th class="val-col">Delta</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="row in summaryRows" :key="row.label">
              <td class="metric-name">
                <span :class="['level-badge', `level-${row.key}`]">{{ row.label }}</span>
              </td>
              <td class="val-col">{{ row.formattedA }}</td>
              <td class="val-col">{{ row.formattedB }}</td>
              <td :class="['val-col', 'delta-cell', `delta-${row.direction}`]">
                {{ row.formattedDelta }}
              </td>
            </tr>
          </tbody>
        </table>
      </section>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { compareNumbers } from '../../../utils/compare'
import type { Profile } from '../../../api'

interface LogSummary {
  total: number
  debug: number
  info: number
  warn: number
  error: number
  fatal: number
}

interface LoggerData {
  entries: unknown[]
  summary: LogSummary
  truncated: boolean
  max_entries: number
}

const props = defineProps<{
  dataA: unknown
  dataB: unknown
  profileA: Profile
  profileB: Profile
}>()

const loggerA = computed<LoggerData>(() => {
  const d = (props.dataA || {}) as LoggerData
  return {
    entries: d.entries || [],
    summary: d.summary || { total: 0, debug: 0, info: 0, warn: 0, error: 0, fatal: 0 },
    truncated: d.truncated || false,
    max_entries: d.max_entries || 0,
  }
})

const loggerB = computed<LoggerData>(() => {
  const d = (props.dataB || {}) as LoggerData
  return {
    entries: d.entries || [],
    summary: d.summary || { total: 0, debug: 0, info: 0, warn: 0, error: 0, fatal: 0 },
    truncated: d.truncated || false,
    max_entries: d.max_entries || 0,
  }
})

const levels = [
  { key: 'total' as const, label: 'Total', lowerIsBetter: true },
  { key: 'debug' as const, label: 'Debug', lowerIsBetter: true },
  { key: 'info' as const, label: 'Info', lowerIsBetter: true },
  { key: 'warn' as const, label: 'Warn', lowerIsBetter: true },
  { key: 'error' as const, label: 'Error', lowerIsBetter: true },
  { key: 'fatal' as const, label: 'Fatal', lowerIsBetter: true },
]

const summaryRows = computed(() => {
  return levels.map(l => {
    const valA = loggerA.value.summary[l.key] || 0
    const valB = loggerB.value.summary[l.key] || 0
    const diff = compareNumbers(valA, valB, l.lowerIsBetter)

    const formattedA = props.dataA ? String(valA) : '—'
    const formattedB = props.dataB ? String(valB) : '—'

    let formattedDelta: string
    if (!props.dataA || !props.dataB) {
      formattedDelta = '—'
    } else if (diff.delta === 0) {
      formattedDelta = 'same'
    } else {
      const sign = diff.delta > 0 ? '+' : ''
      formattedDelta = `${sign}${diff.delta}`
    }

    return { label: l.label, key: l.key, formattedA, formattedB, formattedDelta, direction: diff.direction }
  })
})
</script>

<style scoped>
.compare-logger-panel {
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

.level-badge {
  font-size: 0.7rem;
  font-weight: 600;
  padding: 0.1rem 0.4rem;
  border-radius: 3px;
  text-transform: uppercase;
}

.level-total { background: #e9ecef; color: #495057; }
.level-debug { background: #e9ecef; color: #6c757d; }
.level-info { background: #d0ebff; color: #1864ab; }
.level-warn { background: #fff3cd; color: #856404; }
.level-error { background: #f8d7da; color: #721c24; }
.level-fatal { background: #c92a2a; color: #fff; }
</style>

<template>
  <div class="compare-memory-panel">
    <div v-if="!dataA && !dataB" class="no-data">No memory data available for either profile.</div>
    <table v-else class="compare-table">
      <thead>
        <tr>
          <th>Metric</th>
          <th class="val-col">Profile A</th>
          <th class="val-col">Profile B</th>
          <th class="val-col">Delta</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="row in rows" :key="row.label">
          <td class="metric-name">{{ row.label }}</td>
          <td class="val-col">{{ row.formattedA }}</td>
          <td class="val-col">{{ row.formattedB }}</td>
          <td :class="['val-col', 'delta-cell', `delta-${row.diff.direction}`]">
            <span class="delta-value">{{ row.formattedDelta }}</span>
            <span v-if="row.percentLabel" class="delta-percent">{{ row.percentLabel }}</span>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { compareNumbers, formatBytes } from '../../../utils/compare'
import type { Profile } from '../../../api'

interface MemoryData {
  alloc_before: number
  alloc_after: number
  alloc_delta: number
  total_alloc: number
  heap_alloc: number
  heap_inuse: number
  heap_objects: number
  num_gc: number
  goroutine_count: number
  sys: number
}

const props = defineProps<{
  dataA: unknown
  dataB: unknown
  profileA: Profile
  profileB: Profile
}>()

const memA = computed<MemoryData>(() => {
  return (props.dataA || {}) as MemoryData
})

const memB = computed<MemoryData>(() => {
  return (props.dataB || {}) as MemoryData
})

interface MetricRow {
  label: string
  key: keyof MemoryData
  isBytes: boolean
}

const metrics: MetricRow[] = [
  { label: 'Alloc Delta', key: 'alloc_delta', isBytes: true },
  { label: 'Heap Alloc', key: 'heap_alloc', isBytes: true },
  { label: 'Heap In-Use', key: 'heap_inuse', isBytes: true },
  { label: 'Heap Objects', key: 'heap_objects', isBytes: false },
  { label: 'Goroutines', key: 'goroutine_count', isBytes: false },
  { label: 'GC Cycles', key: 'num_gc', isBytes: false },
  { label: 'Sys (Total OS)', key: 'sys', isBytes: true },
]

const rows = computed(() => {
  return metrics.map(m => {
    const valA = (memA.value[m.key] as number) || 0
    const valB = (memB.value[m.key] as number) || 0
    const diff = compareNumbers(valA, valB, true)

    const format = m.isBytes ? formatBytes : formatNumber
    const formattedA = props.dataA ? format(valA) : '—'
    const formattedB = props.dataB ? format(valB) : '—'

    let formattedDelta: string
    if (!props.dataA || !props.dataB) {
      formattedDelta = '—'
    } else if (diff.delta === 0) {
      formattedDelta = 'same'
    } else {
      const sign = diff.delta > 0 ? '+' : ''
      formattedDelta = `${sign}${format(diff.delta)}`
    }

    let percentLabel = ''
    if (props.dataA && props.dataB && diff.delta !== 0) {
      const p = diff.percentChange
      if (!isNaN(p) && isFinite(p)) {
        const sign = p > 0 ? '+' : ''
        percentLabel = `(${sign}${p.toFixed(1)}%)`
      }
    }

    return {
      label: m.label,
      formattedA,
      formattedB,
      formattedDelta,
      percentLabel,
      diff,
    }
  })
})

function formatNumber(n: number): string {
  return n.toLocaleString()
}
</script>

<style scoped>
.compare-memory-panel {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.no-data {
  color: #6c757d;
  font-style: italic;
  padding: 1.5rem;
  text-align: center;
  background: #f8f9fa;
  border-radius: 6px;
}

.compare-table {
  width: 100%;
  border-collapse: collapse;
}

.compare-table th {
  text-align: left;
  padding: 0.6rem 1rem;
  font-size: 0.75rem;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: #6c757d;
  border-bottom: 2px solid #e9ecef;
}

.compare-table td {
  padding: 0.6rem 1rem;
  font-size: 0.85rem;
  border-bottom: 1px solid #f1f3f5;
  font-variant-numeric: tabular-nums;
}

.metric-name {
  font-weight: 500;
  color: #1a1a2e;
}

.val-col {
  text-align: right;
  min-width: 100px;
}

.delta-cell {
  font-weight: 500;
}

.delta-value {
  margin-right: 0.3rem;
}

.delta-percent {
  font-size: 0.75rem;
}

.delta-better {
  color: #2b8a3e;
}

.delta-worse {
  color: #c92a2a;
}

.delta-same {
  color: #6c757d;
}
</style>

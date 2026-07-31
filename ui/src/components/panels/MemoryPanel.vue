<template>
  <div class="memory-panel">
    <div class="memory-cards">
      <div class="stat-card">
        <div class="stat-value">{{ formatBytes(memoryData.heap_alloc) }}</div>
        <div class="stat-label">Heap Alloc</div>
      </div>
      <div class="stat-card">
        <div class="stat-value" :class="deltaClass">{{ formatBytesDelta(memoryData.alloc_delta) }}</div>
        <div class="stat-label">Alloc Delta</div>
      </div>
      <div class="stat-card">
        <div class="stat-value">{{ memoryData.goroutine_count }}</div>
        <div class="stat-label">Goroutines</div>
      </div>
      <div class="stat-card">
        <div class="stat-value">{{ memoryData.num_gc }}</div>
        <div class="stat-label">GC Cycles</div>
      </div>
    </div>

    <div class="memory-details">
      <h3>Memory Statistics</h3>
      <table class="info-table">
        <tbody>
          <tr>
            <th>Alloc Before</th>
            <td>{{ formatBytes(memoryData.alloc_before) }}</td>
          </tr>
          <tr>
            <th>Alloc After</th>
            <td>{{ formatBytes(memoryData.alloc_after) }}</td>
          </tr>
          <tr>
            <th>Alloc Delta</th>
            <td :class="deltaClass">{{ formatBytesDelta(memoryData.alloc_delta) }}</td>
          </tr>
          <tr>
            <th>Total Alloc</th>
            <td>{{ formatBytes(memoryData.total_alloc) }}</td>
          </tr>
          <tr>
            <th>Heap Alloc</th>
            <td>{{ formatBytes(memoryData.heap_alloc) }}</td>
          </tr>
          <tr>
            <th>Heap In-Use</th>
            <td>{{ formatBytes(memoryData.heap_inuse) }}</td>
          </tr>
          <tr>
            <th>Heap Objects</th>
            <td>{{ memoryData.heap_objects?.toLocaleString() }}</td>
          </tr>
          <tr>
            <th>System Memory</th>
            <td>{{ formatBytes(memoryData.sys) }}</td>
          </tr>
          <tr>
            <th>GC Cycles</th>
            <td>{{ memoryData.num_gc }}</td>
          </tr>
          <tr>
            <th>Goroutines</th>
            <td>{{ memoryData.goroutine_count }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

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
  data: unknown
  collectorName: string
}>()

const memoryData = computed<MemoryData>(() => {
  return (props.data || {}) as MemoryData
})

const deltaClass = computed(() => {
  if (!memoryData.value.alloc_delta) return ''
  return memoryData.value.alloc_delta > 0 ? 'delta-positive' : 'delta-negative'
})

function formatBytes(bytes: number | undefined): string {
  if (!bytes) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB']
  let unitIndex = 0
  let value = bytes
  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024
    unitIndex++
  }
  return `${value.toFixed(unitIndex > 0 ? 1 : 0)} ${units[unitIndex]}`
}

function formatBytesDelta(bytes: number | undefined): string {
  if (!bytes) return '0 B'
  const prefix = bytes > 0 ? '+' : ''
  return prefix + formatBytes(Math.abs(bytes))
}
</script>

<style scoped>
.memory-panel {
  display: flex;
  flex-direction: column;
  gap: 1.5rem;
}

.memory-cards {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  gap: 1rem;
}

.stat-card {
  background: #f8f9fa;
  border-radius: 8px;
  padding: 1.25rem;
  text-align: center;
}

.stat-value {
  font-size: 1.5rem;
  font-weight: 700;
  color: #1a1a2e;
  font-variant-numeric: tabular-nums;
}

.stat-label {
  font-size: 0.8rem;
  color: #6c757d;
  margin-top: 0.25rem;
}

.memory-details h3 {
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
  padding: 0.5rem 1rem 0.5rem 0;
  font-size: 0.85rem;
  font-weight: 500;
  color: #6c757d;
  width: 180px;
}

.info-table td {
  padding: 0.5rem 0;
  font-size: 0.85rem;
  font-variant-numeric: tabular-nums;
}

.info-table tr + tr {
  border-top: 1px solid #f1f3f5;
}

.delta-positive {
  color: #e67700;
}

.delta-negative {
  color: #2b8a3e;
}
</style>

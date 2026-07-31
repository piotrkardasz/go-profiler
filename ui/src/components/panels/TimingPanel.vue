<template>
  <div class="timing-panel">
    <div class="timing-hero">
      <div class="timing-value">{{ formatDuration(timingData.duration_ms) }}</div>
      <div class="timing-label">Total Duration</div>
    </div>

    <div class="timing-details">
      <table class="info-table">
        <tbody>
          <tr>
            <th>Start Time</th>
            <td>{{ formatTimestamp(timingData.start_time) }}</td>
          </tr>
          <tr>
            <th>End Time</th>
            <td>{{ formatTimestamp(timingData.end_time) }}</td>
          </tr>
          <tr>
            <th>Duration</th>
            <td>{{ timingData.duration_ms?.toFixed(3) }} ms</td>
          </tr>
        </tbody>
      </table>
    </div>

    <div class="timing-bar">
      <div class="bar-track">
        <div class="bar-fill" :style="{ width: barWidth }"></div>
      </div>
      <div class="bar-labels">
        <span>0ms</span>
        <span>{{ formatDuration(timingData.duration_ms) }}</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

interface TimingData {
  start_time: string
  end_time: string
  duration_ms: number
}

const props = defineProps<{
  data: unknown
  collectorName: string
}>()

const timingData = computed<TimingData>(() => {
  return (props.data || {}) as TimingData
})

const barWidth = computed(() => {
  // Visual: scale to show relative time (max bar = full width)
  return '100%'
})

function formatDuration(ms: number | undefined): string {
  if (!ms) return '0ms'
  if (ms < 1) return `${(ms * 1000).toFixed(0)}us`
  if (ms < 1000) return `${ms.toFixed(1)}ms`
  return `${(ms / 1000).toFixed(3)}s`
}

function formatTimestamp(ts: string | undefined): string {
  if (!ts) return '—'
  const date = new Date(ts)
  const time = date.toLocaleTimeString(undefined, {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  })
  const ms = date.getMilliseconds().toString().padStart(3, '0')
  return `${time}.${ms}`
}
</script>

<style scoped>
.timing-panel {
  display: flex;
  flex-direction: column;
  gap: 1.5rem;
}

.timing-hero {
  text-align: center;
  padding: 1.5rem;
  background: #f8f9fa;
  border-radius: 8px;
}

.timing-value {
  font-size: 2.5rem;
  font-weight: 700;
  color: #1a1a2e;
  font-variant-numeric: tabular-nums;
}

.timing-label {
  font-size: 0.85rem;
  color: #6c757d;
  margin-top: 0.25rem;
}

.timing-details .info-table {
  width: 100%;
  border-collapse: collapse;
}

.timing-details .info-table th {
  text-align: left;
  padding: 0.5rem 1rem 0.5rem 0;
  font-size: 0.85rem;
  font-weight: 500;
  color: #6c757d;
  width: 150px;
}

.timing-details .info-table td {
  padding: 0.5rem 0;
  font-size: 0.85rem;
  font-variant-numeric: tabular-nums;
}

.timing-details .info-table tr + tr {
  border-top: 1px solid #f1f3f5;
}

.timing-bar {
  padding: 1rem 0;
}

.bar-track {
  height: 8px;
  background: #e9ecef;
  border-radius: 4px;
  overflow: hidden;
}

.bar-fill {
  height: 100%;
  background: linear-gradient(90deg, #4fc3f7, #1a1a2e);
  border-radius: 4px;
  transition: width 0.3s ease;
}

.bar-labels {
  display: flex;
  justify-content: space-between;
  margin-top: 0.35rem;
  font-size: 0.75rem;
  color: #6c757d;
}
</style>

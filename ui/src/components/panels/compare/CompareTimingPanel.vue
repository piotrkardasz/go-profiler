<template>
  <div class="compare-timing-panel">
    <div class="timing-heroes">
      <div class="timing-hero hero-a">
        <div class="hero-label">Profile A</div>
        <div class="hero-value">{{ formatMs(timingA.duration_ms) }}</div>
        <div class="hero-times">
          <span>{{ formatTime(timingA.start_time) }}</span>
          <span>&rarr;</span>
          <span>{{ formatTime(timingA.end_time) }}</span>
        </div>
      </div>

      <div class="timing-delta" :class="deltaClass">
        <div class="delta-value">{{ deltaLabel }}</div>
        <div class="delta-percent" v-if="percentLabel">{{ percentLabel }}</div>
        <div class="delta-direction">{{ directionLabel }}</div>
      </div>

      <div class="timing-hero hero-b">
        <div class="hero-label">Profile B</div>
        <div class="hero-value">{{ formatMs(timingB.duration_ms) }}</div>
        <div class="hero-times">
          <span>{{ formatTime(timingB.start_time) }}</span>
          <span>&rarr;</span>
          <span>{{ formatTime(timingB.end_time) }}</span>
        </div>
      </div>
    </div>

    <div class="timing-bars">
      <div class="bar-row">
        <span class="bar-label">A</span>
        <div class="bar-track">
          <div class="bar-fill bar-fill-a" :style="{ width: barWidthA }"></div>
        </div>
        <span class="bar-value">{{ formatMs(timingA.duration_ms) }}</span>
      </div>
      <div class="bar-row">
        <span class="bar-label">B</span>
        <div class="bar-track">
          <div class="bar-fill bar-fill-b" :style="{ width: barWidthB }"></div>
        </div>
        <span class="bar-value">{{ formatMs(timingB.duration_ms) }}</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { compareNumbers, formatDuration } from '../../../utils/compare'
import type { Profile } from '../../../api'

interface TimingData {
  start_time: string
  end_time: string
  duration_ms: number
}

const props = defineProps<{
  dataA: unknown
  dataB: unknown
  profileA: Profile
  profileB: Profile
}>()

const timingA = computed<TimingData>(() => {
  return (props.dataA || { start_time: '', end_time: '', duration_ms: 0 }) as TimingData
})

const timingB = computed<TimingData>(() => {
  return (props.dataB || { start_time: '', end_time: '', duration_ms: 0 }) as TimingData
})

const diff = computed(() => {
  return compareNumbers(timingA.value.duration_ms, timingB.value.duration_ms, true)
})

const deltaClass = computed(() => {
  return `delta-${diff.value.direction}`
})

const deltaLabel = computed(() => {
  const d = diff.value.delta
  const sign = d > 0 ? '+' : ''
  return `${sign}${formatDuration(d)}`
})

const percentLabel = computed(() => {
  const p = diff.value.percentChange
  if (isNaN(p) || !isFinite(p)) return ''
  const sign = p > 0 ? '+' : ''
  return `(${sign}${p.toFixed(1)}%)`
})

const directionLabel = computed(() => {
  if (diff.value.direction === 'better') return 'faster'
  if (diff.value.direction === 'worse') return 'slower'
  return 'same'
})

const barWidthA = computed(() => {
  const max = Math.max(timingA.value.duration_ms, timingB.value.duration_ms)
  if (max === 0) return '0%'
  return `${(timingA.value.duration_ms / max) * 100}%`
})

const barWidthB = computed(() => {
  const max = Math.max(timingA.value.duration_ms, timingB.value.duration_ms)
  if (max === 0) return '0%'
  return `${(timingB.value.duration_ms / max) * 100}%`
})

function formatMs(ms: number | undefined): string {
  return formatDuration(ms)
}

function formatTime(ts: string | undefined): string {
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
.compare-timing-panel {
  display: flex;
  flex-direction: column;
  gap: 2rem;
}

.timing-heroes {
  display: grid;
  grid-template-columns: 1fr auto 1fr;
  gap: 1rem;
  align-items: center;
}

@media (max-width: 1280px) {
  .timing-heroes {
    grid-template-columns: 1fr;
    gap: 0.75rem;
  }
}

.timing-hero {
  text-align: center;
  padding: 1.5rem;
  background: #f8f9fa;
  border-radius: 8px;
}

.hero-label {
  font-size: 0.7rem;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  color: #6c757d;
  margin-bottom: 0.5rem;
}

.hero-a .hero-label { color: #1a8ecf; }
.hero-b .hero-label { color: #e65100; }

.hero-value {
  font-size: 2.25rem;
  font-weight: 700;
  color: #1a1a2e;
  font-variant-numeric: tabular-nums;
}

.hero-times {
  margin-top: 0.5rem;
  font-size: 0.75rem;
  color: #6c757d;
  display: flex;
  justify-content: center;
  gap: 0.4rem;
}

.timing-delta {
  text-align: center;
  padding: 1rem;
  border-radius: 8px;
  min-width: 120px;
}

.delta-value {
  font-size: 1.25rem;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
}

.delta-percent {
  font-size: 0.85rem;
  margin-top: 0.15rem;
}

.delta-direction {
  font-size: 0.7rem;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  margin-top: 0.25rem;
}

.delta-better {
  background: #d3f9d8;
}
.delta-better .delta-value,
.delta-better .delta-percent {
  color: #2b8a3e;
}
.delta-better .delta-direction {
  color: #2b8a3e;
}

.delta-worse {
  background: #ffe3e3;
}
.delta-worse .delta-value,
.delta-worse .delta-percent {
  color: #c92a2a;
}
.delta-worse .delta-direction {
  color: #c92a2a;
}

.delta-same {
  background: #f1f3f5;
}
.delta-same .delta-value,
.delta-same .delta-percent,
.delta-same .delta-direction {
  color: #6c757d;
}

.timing-bars {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.bar-row {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.bar-label {
  font-size: 0.75rem;
  font-weight: 700;
  color: #6c757d;
  width: 1rem;
  text-align: center;
}

.bar-track {
  flex: 1;
  height: 10px;
  background: #e9ecef;
  border-radius: 5px;
  overflow: hidden;
}

.bar-fill {
  height: 100%;
  border-radius: 5px;
  transition: width 0.3s ease;
}

.bar-fill-a {
  background: linear-gradient(90deg, #4fc3f7, #1a8ecf);
}

.bar-fill-b {
  background: linear-gradient(90deg, #ff8a65, #e65100);
}

.bar-value {
  font-size: 0.75rem;
  font-variant-numeric: tabular-nums;
  color: #495057;
  min-width: 60px;
}
</style>

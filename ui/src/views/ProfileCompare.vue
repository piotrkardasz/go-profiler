<template>
  <div class="profile-compare">
    <div class="compare-header">
      <router-link to="/_profiler/" class="back-link">&larr; Back to Profiles</router-link>
      <button class="btn btn-swap" @click="swap">&hArr; Swap</button>
    </div>

    <div v-if="loading" class="loading">Loading comparison...</div>
    <div v-else-if="error" class="error">{{ error }}</div>

    <template v-else-if="profileA && profileB">
      <div class="compare-summaries">
        <div class="summary-card summary-a">
          <div class="summary-label">Profile A</div>
          <div class="summary-title">
            <span :class="['method-badge', `method-${profileA.method.toLowerCase()}`]">
              {{ profileA.method }}
            </span>
            <span class="summary-url">{{ profileA.url }}</span>
          </div>
          <div class="summary-meta">
            <span :class="['status-badge', statusClass(profileA.status_code)]">
              {{ profileA.status_code }}
            </span>
            <span class="meta-item">{{ formatDuration(profileA.duration) }}</span>
            <span class="meta-item">{{ formatTimestamp(profileA.timestamp) }}</span>
            <router-link
              :to="{ name: 'profile-detail', params: { id: profileA.id } }"
              class="meta-id"
              @click.stop
            >
              {{ profileA.id }}
            </router-link>
          </div>
        </div>
        <div class="summary-card summary-b">
          <div class="summary-label">Profile B</div>
          <div class="summary-title">
            <span :class="['method-badge', `method-${profileB.method.toLowerCase()}`]">
              {{ profileB.method }}
            </span>
            <span class="summary-url">{{ profileB.url }}</span>
          </div>
          <div class="summary-meta">
            <span :class="['status-badge', statusClass(profileB.status_code)]">
              {{ profileB.status_code }}
            </span>
            <span class="meta-item">{{ formatDuration(profileB.duration) }}</span>
            <span class="meta-item">{{ formatTimestamp(profileB.timestamp) }}</span>
            <router-link
              :to="{ name: 'profile-detail', params: { id: profileB.id } }"
              class="meta-id"
              @click.stop
            >
              {{ profileB.id }}
            </router-link>
          </div>
        </div>
      </div>

      <div class="compare-body">
        <div class="panel-tabs">
          <button
            v-for="panel in panels"
            :key="panel.name"
            :class="['tab-btn', { active: activePanel === panel.name }]"
            @click="activePanel = panel.name"
          >
            {{ panel.label }}
          </button>
        </div>
        <div class="panel-content">
          <component
            :is="getComparePanelComponent(activePanel)"
            :data-a="profileA.collector_data[activePanel]"
            :data-b="profileB.collector_data[activePanel]"
            :profile-a="profileA"
            :profile-b="profileB"
          />
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import { getProfile, getCollectors, type Profile, type CollectorMeta } from '../api'
import { getComparePanel } from '../plugin'

const props = defineProps<{
  idA: string
  idB: string
}>()

const router = useRouter()
const profileA = ref<Profile | null>(null)
const profileB = ref<Profile | null>(null)
const panels = ref<CollectorMeta[]>([])
const activePanel = ref('')
const loading = ref(false)
const error = ref('')

// Collectors to exclude from comparison
const EXCLUDED_COLLECTORS = new Set(['otel'])

async function load() {
  loading.value = true
  error.value = ''
  try {
    const [pA, pB, collectorsData] = await Promise.all([
      getProfile(props.idA),
      getProfile(props.idB),
      getCollectors(),
    ])
    profileA.value = pA
    profileB.value = pB

    // Filter panels: include if at least one profile has data, exclude OTel
    panels.value = collectorsData.collectors.filter(c => {
      if (EXCLUDED_COLLECTORS.has(c.name)) return false
      const hasA = c.name in (pA.collector_data || {})
      const hasB = c.name in (pB.collector_data || {})
      return hasA || hasB
    })

    if (panels.value.length > 0) {
      activePanel.value = panels.value[0].name
    }
  } catch (e: unknown) {
    error.value = e instanceof Error ? e.message : 'Failed to load profiles for comparison'
  } finally {
    loading.value = false
  }
}

function getComparePanelComponent(collectorName: string) {
  return getComparePanel(collectorName)
}

function swap() {
  router.replace({
    name: 'profile-compare',
    params: { idA: props.idB, idB: props.idA },
  })
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

function formatTimestamp(timestamp: string): string {
  return new Date(timestamp).toLocaleString()
}

// Reload when route params change (swap)
watch(() => [props.idA, props.idB], () => {
  load()
})

onMounted(load)
</script>

<style scoped>
.profile-compare {
  display: flex;
  flex-direction: column;
  gap: 1.5rem;
}

.compare-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.back-link {
  color: #495057;
  text-decoration: none;
  font-size: 0.875rem;
}

.back-link:hover {
  color: #212529;
}

.btn-swap {
  padding: 0.4rem 0.85rem;
  border: 1px solid #dee2e6;
  border-radius: 6px;
  background: #fff;
  font-size: 0.85rem;
  cursor: pointer;
  color: #495057;
  font-weight: 500;
  transition: background 0.15s, border-color 0.15s;
}

.btn-swap:hover {
  background: #f8f9fa;
  border-color: #adb5bd;
}

.compare-summaries {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 1rem;
}

@media (max-width: 1280px) {
  .compare-summaries {
    grid-template-columns: 1fr;
  }
}

.summary-card {
  background: #fff;
  border-radius: 8px;
  padding: 1rem 1.25rem;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.08);
  border-top: 3px solid transparent;
}

.summary-a {
  border-top-color: #4fc3f7;
}

.summary-b {
  border-top-color: #ff8a65;
}

.summary-label {
  font-size: 0.7rem;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  color: #6c757d;
  margin-bottom: 0.4rem;
}

.summary-title {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 1rem;
  font-weight: 600;
}

.summary-url {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.summary-meta {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  margin-top: 0.5rem;
  font-size: 0.8rem;
  color: #6c757d;
}

.meta-item {
  font-variant-numeric: tabular-nums;
}

.meta-id {
  font-size: 0.7rem;
  font-family: monospace;
  background: #f1f3f5;
  padding: 0.15rem 0.4rem;
  border-radius: 3px;
  color: #495057;
  text-decoration: none;
}

.meta-id:hover {
  color: #1a1a2e;
  background: #e9ecef;
}

.compare-body {
  background: #fff;
  border-radius: 8px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.08);
  overflow: hidden;
}

.panel-tabs {
  display: flex;
  border-bottom: 1px solid #dee2e6;
  overflow-x: auto;
}

.tab-btn {
  padding: 0.75rem 1.25rem;
  border: none;
  background: none;
  font-size: 0.875rem;
  font-weight: 500;
  color: #6c757d;
  cursor: pointer;
  border-bottom: 2px solid transparent;
  white-space: nowrap;
  transition: color 0.2s, border-color 0.2s;
}

.tab-btn:hover {
  color: #212529;
}

.tab-btn.active {
  color: #1a1a2e;
  border-bottom-color: #4fc3f7;
}

.panel-content {
  padding: 1.5rem;
}

.loading,
.error {
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
</style>

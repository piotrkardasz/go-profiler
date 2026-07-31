<template>
  <div class="profile-detail">
    <div class="detail-header">
      <router-link to="/_profiler/" class="back-link">&larr; Back to Profiles</router-link>
      <div v-if="profile" class="detail-summary">
        <h1>
          <span :class="['method-badge', `method-${profile.method.toLowerCase()}`]">
            {{ profile.method }}
          </span>
          {{ profile.url }}
        </h1>
        <div class="summary-meta">
          <span :class="['status-badge', statusClass(profile.status_code)]">
            {{ profile.status_code }}
          </span>
          <span class="meta-item">{{ formatDuration(profile.duration) }}</span>
          <span class="meta-item">{{ formatTimestamp(profile.timestamp) }}</span>
          <code class="meta-id">{{ profile.id }}</code>
        </div>
      </div>
    </div>

    <div v-if="loading" class="loading">Loading profile...</div>
    <div v-else-if="error" class="error">{{ error }}</div>
    <div v-else-if="profile" class="detail-body">
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
          :is="getComponent(activePanel)"
          :data="profile.collector_data[activePanel]"
          :collector-name="activePanel"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { getProfile, getCollectors, type Profile, type CollectorMeta } from '../api'
import { getPanel } from '../plugin/registry'

const props = defineProps<{
  id: string
}>()

const profile = ref<Profile | null>(null)
const panels = ref<CollectorMeta[]>([])
const activePanel = ref('')
const loading = ref(false)
const error = ref('')

async function load() {
  loading.value = true
  error.value = ''
  try {
    const [profileData, collectorsData] = await Promise.all([
      getProfile(props.id),
      getCollectors(),
    ])
    profile.value = profileData
    panels.value = collectorsData.collectors.filter(
      c => c.name in (profileData.collector_data || {})
    )
    if (panels.value.length > 0) {
      activePanel.value = panels.value[0].name
    }
  } catch (e: unknown) {
    error.value = e instanceof Error ? e.message : 'Failed to load profile'
  } finally {
    loading.value = false
  }
}

function getComponent(collectorName: string) {
  return getPanel(collectorName)
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

onMounted(load)
</script>

<style scoped>
.profile-detail {
  display: flex;
  flex-direction: column;
  gap: 1.5rem;
}

.back-link {
  color: #495057;
  text-decoration: none;
  font-size: 0.875rem;
}

.back-link:hover {
  color: #212529;
}

.detail-summary h1 {
  font-size: 1.25rem;
  font-weight: 600;
  display: flex;
  align-items: center;
  gap: 0.75rem;
  margin-top: 0.5rem;
}

.summary-meta {
  display: flex;
  align-items: center;
  gap: 1rem;
  margin-top: 0.5rem;
  font-size: 0.875rem;
  color: #6c757d;
}

.meta-id {
  font-size: 0.75rem;
  background: #f1f3f5;
  padding: 0.15rem 0.4rem;
  border-radius: 3px;
}

.detail-body {
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

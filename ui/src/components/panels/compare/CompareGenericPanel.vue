<template>
  <div class="compare-generic-panel">
    <div class="compare-columns">
      <div class="compare-col">
        <h3 class="col-label">Profile A</h3>
        <div class="json-container">
          <JsonNode v-if="dataA" :data="dataA" :depth="0" />
          <div v-else class="no-data">No data</div>
        </div>
      </div>
      <div class="compare-col">
        <h3 class="col-label">Profile B</h3>
        <div class="json-container">
          <JsonNode v-if="dataB" :data="dataB" :depth="0" />
          <div v-else class="no-data">No data</div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import JsonNode from '../JsonNode.vue'
import type { Profile } from '../../../api'

defineProps<{
  dataA: unknown
  dataB: unknown
  profileA: Profile
  profileB: Profile
}>()
</script>

<style scoped>
.compare-generic-panel {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.compare-columns {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 1.5rem;
}

@media (max-width: 1280px) {
  .compare-columns {
    grid-template-columns: 1fr;
  }
}

.compare-col {
  min-width: 0;
  overflow: hidden;
}

.col-label {
  font-size: 0.8rem;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: #6c757d;
  margin-bottom: 0.75rem;
  padding-bottom: 0.5rem;
  border-bottom: 1px solid #e9ecef;
}

.json-container {
  font-family: 'SF Mono', 'Fira Code', monospace;
  font-size: 0.8rem;
  overflow-x: auto;
}

.no-data {
  color: #6c757d;
  font-style: italic;
  padding: 1rem;
  text-align: center;
  background: #f8f9fa;
  border-radius: 6px;
}
</style>

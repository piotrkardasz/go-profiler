<template>
  <div class="generic-panel">
    <h3 class="panel-title">{{ collectorName }}</h3>
    <div class="json-tree">
      <JsonNode :data="data" :depth="0" />
    </div>
  </div>
</template>

<script setup lang="ts">
import JsonNode from './JsonNode.vue'

defineProps<{
  data: unknown
  collectorName: string
}>()
</script>

<style scoped>
.generic-panel {
  font-family: 'SF Mono', Monaco, 'Cascadia Mono', monospace;
  font-size: 0.85rem;
}

.panel-title {
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
  font-size: 1rem;
  font-weight: 600;
  margin-bottom: 1rem;
  text-transform: capitalize;
}

.json-tree {
  background: #f8f9fa;
  border-radius: 6px;
  padding: 1rem;
  overflow-x: auto;
}
</style>

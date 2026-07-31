<template>
  <div class="panel-registry">
    <component
      :is="panelComponent"
      :data="data"
      :collector-name="collectorName"
    />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { getPanel } from '../plugin/registry'

const props = defineProps<{
  collectorName: string
  data: unknown
}>()

const panelComponent = computed(() => {
  return getPanel(props.collectorName)
})
</script>

<style scoped>
.panel-registry {
  min-height: 100px;
}
</style>

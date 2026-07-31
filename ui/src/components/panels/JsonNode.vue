<template>
  <div class="json-node" :style="{ paddingLeft: depth > 0 ? '1.25rem' : '0' }">
    <template v-if="isObject(data)">
      <div
        v-for="[key, value] in objectEntries(data)"
        :key="key"
        class="json-entry"
      >
        <span class="json-key">{{ key }}:</span>
        <template v-if="isPrimitive(value)">
          <span :class="valueClass(value)">{{ formatValue(value) }}</span>
        </template>
        <template v-else>
          <span class="json-bracket">{{ isArray(value) ? '[' : '{' }}</span>
          <JsonNode :data="value" :depth="depth + 1" />
          <span class="json-bracket">{{ isArray(value) ? ']' : '}' }}</span>
        </template>
      </div>
    </template>
    <template v-else-if="isArray(data)">
      <div
        v-for="(item, index) in (data as unknown[])"
        :key="index"
        class="json-entry"
      >
        <span class="json-index">{{ index }}:</span>
        <template v-if="isPrimitive(item)">
          <span :class="valueClass(item)">{{ formatValue(item) }}</span>
        </template>
        <template v-else>
          <span class="json-bracket">{{ isArray(item) ? '[' : '{' }}</span>
          <JsonNode :data="item" :depth="depth + 1" />
          <span class="json-bracket">{{ isArray(item) ? ']' : '}' }}</span>
        </template>
      </div>
    </template>
    <template v-else>
      <span :class="valueClass(data)">{{ formatValue(data) }}</span>
    </template>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  data: unknown
  depth: number
}>()

function isObject(val: unknown): val is Record<string, unknown> {
  return val !== null && typeof val === 'object' && !Array.isArray(val)
}

function isArray(val: unknown): val is unknown[] {
  return Array.isArray(val)
}

function isPrimitive(val: unknown): boolean {
  return val === null || (typeof val !== 'object')
}

function objectEntries(val: unknown): [string, unknown][] {
  if (isObject(val)) {
    return Object.entries(val)
  }
  return []
}

function valueClass(val: unknown): string {
  if (val === null) return 'json-null'
  if (typeof val === 'string') return 'json-string'
  if (typeof val === 'number') return 'json-number'
  if (typeof val === 'boolean') return 'json-boolean'
  return ''
}

function formatValue(val: unknown): string {
  if (val === null) return 'null'
  if (typeof val === 'string') return `"${val}"`
  return String(val)
}
</script>

<style scoped>
.json-node {
  line-height: 1.6;
}

.json-entry {
  display: flex;
  flex-wrap: wrap;
  gap: 0.35rem;
}

.json-key {
  color: #e45649;
  font-weight: 500;
}

.json-index {
  color: #6c757d;
}

.json-string {
  color: #50a14f;
}

.json-number {
  color: #986801;
}

.json-boolean {
  color: #0184bc;
}

.json-null {
  color: #a0a1a7;
  font-style: italic;
}

.json-bracket {
  color: #6c757d;
}
</style>

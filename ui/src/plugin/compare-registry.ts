import { type Component, markRaw } from 'vue'
import CompareGenericPanel from '../components/panels/compare/CompareGenericPanel.vue'

// Registry of comparison panel components keyed by collector name
const comparePanelRegistry = new Map<string, Component>()

/**
 * Register a comparison-aware Vue component for a specific collector.
 * If no comparison component is registered, CompareGenericPanel is used.
 */
export function registerComparePanel(collectorName: string, component: Component): void {
  comparePanelRegistry.set(collectorName, markRaw(component))
}

/**
 * Get the comparison panel component for a given collector.
 * Returns the registered component or falls back to CompareGenericPanel.
 */
export function getComparePanel(collectorName: string): Component {
  return comparePanelRegistry.get(collectorName) || CompareGenericPanel
}

/**
 * Check if a comparison panel is registered for the given collector.
 */
export function hasComparePanel(collectorName: string): boolean {
  return comparePanelRegistry.has(collectorName)
}

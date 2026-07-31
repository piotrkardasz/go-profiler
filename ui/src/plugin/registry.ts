import { type Component, markRaw } from 'vue'
import GenericPanel from '../components/panels/GenericPanel.vue'

// Registry of panel components keyed by collector name
const panelRegistry = new Map<string, Component>()

/**
 * Register a custom Vue component for a specific collector's panel.
 * If no custom component is registered, the generic JSON panel is used.
 */
export function registerPanel(collectorName: string, component: Component): void {
  panelRegistry.set(collectorName, markRaw(component))
}

/**
 * Get the panel component for a given collector.
 * Returns the registered custom component or falls back to GenericPanel.
 */
export function getPanel(collectorName: string): Component {
  return panelRegistry.get(collectorName) || GenericPanel
}

/**
 * Check if a custom panel is registered for the given collector.
 */
export function hasCustomPanel(collectorName: string): boolean {
  return panelRegistry.has(collectorName)
}

/**
 * Get all registered panel names.
 */
export function getRegisteredPanels(): string[] {
  return Array.from(panelRegistry.keys())
}

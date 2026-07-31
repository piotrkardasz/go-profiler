/**
 * Panel Plugin System for Go Profiler UI
 *
 * This module provides the extensible panel registration system.
 * Custom collectors can register their own Vue components to render
 * their data in a rich format. Collectors without a registered panel
 * will use the generic JSON tree view.
 *
 * Usage:
 *   import { registerPanel } from '@/plugin'
 *   import MyCustomPanel from './MyCustomPanel.vue'
 *   registerPanel('my-collector', MyCustomPanel)
 */

export { registerPanel, getPanel, hasCustomPanel, getRegisteredPanels } from './registry'
export { initBuiltinPanels } from './builtin'

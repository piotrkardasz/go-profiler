import { registerComparePanel } from './compare-registry'
import CompareGenericPanel from '../components/panels/compare/CompareGenericPanel.vue'
import CompareTimingPanel from '../components/panels/compare/CompareTimingPanel.vue'
import CompareMemoryPanel from '../components/panels/compare/CompareMemoryPanel.vue'
import CompareRequestPanel from '../components/panels/compare/CompareRequestPanel.vue'
import CompareGormPanel from '../components/panels/compare/CompareGormPanel.vue'
import CompareLoggerPanel from '../components/panels/compare/CompareLoggerPanel.vue'
import CompareConfigPanel from '../components/panels/compare/CompareConfigPanel.vue'

/**
 * Register all built-in comparison panel components.
 * This should be called once during app initialization.
 */
export function initCompareBuiltins(): void {
  registerComparePanel('_generic', CompareGenericPanel)
  registerComparePanel('timing', CompareTimingPanel)
  registerComparePanel('memory', CompareMemoryPanel)
  registerComparePanel('request', CompareRequestPanel)
  registerComparePanel('gorm', CompareGormPanel)
  registerComparePanel('logger', CompareLoggerPanel)
  registerComparePanel('config', CompareConfigPanel)
}

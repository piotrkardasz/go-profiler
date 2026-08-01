import { registerPanel } from './registry'
import RequestPanel from '../components/panels/RequestPanel.vue'
import TimingPanel from '../components/panels/TimingPanel.vue'
import MemoryPanel from '../components/panels/MemoryPanel.vue'
import OtelPanel from '../components/panels/OtelPanel.vue'
import GormPanel from '../components/panels/GormPanel.vue'
import ConfigPanel from '../components/panels/ConfigPanel.vue'
import LoggerPanel from '../components/panels/LoggerPanel.vue'

/**
 * Register all built-in panel components.
 * This should be called once during app initialization.
 */
export function initBuiltinPanels(): void {
  registerPanel('request', RequestPanel)
  registerPanel('timing', TimingPanel)
  registerPanel('memory', MemoryPanel)
  registerPanel('otel', OtelPanel)
  registerPanel('gorm', GormPanel)
  registerPanel('config', ConfigPanel)
  registerPanel('logger', LoggerPanel)
}

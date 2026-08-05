import { createApp } from 'vue'
import App from './App.vue'
import { router } from './router'
import { initBuiltinPanels, initCompareBuiltins } from './plugin'

// Register built-in panels for Request, Timing, Memory collectors
initBuiltinPanels()

// Register built-in comparison panels
initCompareBuiltins()

const app = createApp(App)
app.use(router)
app.mount('#app')

import { createApp } from 'vue'
import App from './App.vue'
import { router } from './router'
import { initBuiltinPanels } from './plugin'

// Register built-in panels for Request, Timing, Memory collectors
initBuiltinPanels()

const app = createApp(App)
app.use(router)
app.mount('#app')

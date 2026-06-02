import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import { useThemeStore } from './stores/themeStore'
import { container } from './core/container'
import './styles.css'

const app = createApp(App)
const pinia = createPinia()
app.use(pinia)

// 初始化主题
useThemeStore().initializeTheme()

// 初始化依赖注入容器
function initializeServices(): void {
  const feedback = container.feedback
  console.debug('[WindDAQ] Feedback service initialized')
  
  const motion = container.motion
  console.debug('[WindDAQ] Motion service initialized')
  
  if (import.meta.env.DEV) {
    (window as any).__WINDDAQ_CONTAINER__ = container
    console.debug('[WindDAQ] Container exposed to window for debugging')
  }
}

initializeServices()

app.mount('#app')
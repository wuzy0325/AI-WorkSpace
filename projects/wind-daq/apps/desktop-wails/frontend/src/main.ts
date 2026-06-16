import { createApp, type Component } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'
import { useThemeStore } from './stores/themeStore'
import { container } from './core/container'
import './styles.css'

async function resolveRootComponent(): Promise<Component> {
  if (import.meta.env.VITE_UI_SPIKE === '1') {
    return (await import('./spikes/UiLibrarySpikeView.vue')).default
  }

  return App
}

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

async function bootstrap(): Promise<void> {
  const app = createApp(await resolveRootComponent())
  const pinia = createPinia()
  app.use(pinia)
  app.use(router)

  // 初始化主题
  useThemeStore().initializeTheme()

  if (import.meta.env.VITE_UI_SPIKE !== '1') {
    initializeServices()
  }

  app.mount('#app')
}

void bootstrap()

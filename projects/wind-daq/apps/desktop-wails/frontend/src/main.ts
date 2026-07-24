import { createApp, type Component } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'
import { useThemeStore } from './stores/themeStore'
import { useI18nStore } from './stores/i18nStore'
import { container } from './core/container'
import { wailsApi, isWailsAvailable } from './api/wails-adapter'
import { setMotionStandaloneMode } from './api/motionApi'
import { initWebVitals } from './utils/webVitals'
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

  // 初始化语言（localStorage → 安装程序注册表 → 默认中文）
  await useI18nStore().initLocale()

  if (import.meta.env.VITE_UI_SPIKE !== '1') {
    initializeServices()
  }

  // 检查启动模式：如果是运动控制器独立窗口模式，自动跳转到 /motion 路由
  if (isWailsAvailable()) {
    try {
      const mode = await wailsApi.app.getStartupMode()
      if (mode === 'motion') {
        // 标记当前为运动控制器独立窗口（motion 子进程）
        // 之后 motionApi 的所有调用会通过主进程 HTTP API（127.0.0.1:8900）转发，
        // 避免连接状态分裂在子进程内存中导致关闭窗口后丢失。
        setMotionStandaloneMode(true)
        await router.replace({ name: 'motion-standalone' })
      }
    } catch (e) {
      console.warn('[WindDAQ] 获取启动模式失败，使用默认路由', e)
    }
  }

  app.mount('#app')

  // 启动 Web Vitals 上报（在 mount 后，确保 first paint 已发生）
  initWebVitals()
}

void bootstrap()

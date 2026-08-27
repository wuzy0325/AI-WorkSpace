import { createApp } from 'vue'
import { createPinia } from 'pinia'
import ElementPlus from 'element-plus'
import * as ElementPlusIconsVue from '@element-plus/icons-vue'
import 'element-plus/dist/index.css'
import '@fontsource/dm-sans/400.css'
import '@fontsource/dm-sans/500.css'
import '@fontsource/dm-sans/600.css'
import '@fontsource/dm-sans/700.css'

import App from './App.vue'
import router from './router'
import { initDesktopApiBase } from './api/client'
import { useEventHub } from './composables/useEventHub'
import { EVENT_HARDWARE_COMMAND, EVENT_HARDWARE_RESPONSE, EVENT_SYSTEM_ERROR } from './shared/events'
import { useHardwareLogStore } from './stores/hardwareLog'
import { useGatesStore } from './stores/app/gates'
import { installFrontendDiagnostics, logVueError } from './services/frontendDiagnostics'
import type { StreamEventPayload } from './types/api'
import './styles/global.scss'
// P2-3：全局补齐自定义 button / input / [tabindex] 的 focus ring，
// 提升键盘用户可见性（Element Plus 组件自身 focus 样式不受影响）。
// 注意：从 TS 引入 SCSS partial（_focus-ring.scss）需带下划线前缀，
// Vite/Rollup 不会自动应用 SCSS partial 名称解析规则。
import './styles/_focus-ring.scss'

async function bootstrap() {
  // 在桌面模式下，初始化 API 基础路径指向内嵌 HTTP 服务器。
  await initDesktopApiBase()

  const app = createApp(App)

  // 注册所有图标
  for (const [key, component] of Object.entries(ElementPlusIconsVue)) {
    app.component(key, component)
  }

  const pinia = createPinia()
  app.use(pinia)
  app.use(router)
  app.use(ElementPlus)

  // 拉取启动门禁开关（阀门=校准模式 是否为标定/计量启动必要条件），
  // 与后端 /api/v1/config/gates 同源，避免前端硬编码。
  // 失败时 store 保持默认值（严格门禁），不阻塞应用启动。
  void useGatesStore().refresh()

  // 全局监听硬件通讯事件与系统错误事件
  const hardwareLog = useHardwareLogStore()
  const { subscribeGlobal } = useEventHub()
  subscribeGlobal((payload: StreamEventPayload) => {
    if (payload.type === EVENT_HARDWARE_COMMAND) {
      const data = payload.data as { model?: string; proto?: string; cmd?: string; poll?: boolean }
      hardwareLog.addEntry('hw-cmd', data?.model ?? '', data?.proto ?? '', data?.cmd ?? '', data?.poll)
    }
    if (payload.type === EVENT_HARDWARE_RESPONSE) {
      const data = payload.data as { model?: string; proto?: string; resp?: string; cmd?: string; poll?: boolean }
      const detail = data?.resp ?? ''
      hardwareLog.addEntry('hw-res', data?.model ?? '', data?.proto ?? '', detail.length > 200 ? detail.slice(0, 200) + '...' : detail, data?.poll)
    }
    if (payload.type === EVENT_SYSTEM_ERROR) {
      const data = payload.data as { code?: string; status?: number; message?: string }
      hardwareLog.addEntry('sys-error', data?.code ?? '', String(data?.status ?? ''), data?.message ?? '')
    }
  })

  // Vue 全局错误处理：捕获渲染/观察者异常，防止静默崩溃
  app.config.errorHandler = (err, instance, info) => {
    console.error(`[Vue error] ${info}:`, err)
    logVueError(err, info)
  }

  window.onerror = (message, source, lineno, colno, error) => {
    console.error('[window.onerror]', message, source, lineno, colno, error)
  }

  window.onunhandledrejection = (event) => {
    console.error('[unhandled rejection]', event.reason)
  }

  installFrontendDiagnostics(router)
  app.mount('#app')
}

bootstrap()

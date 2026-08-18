import { isWailsAvailable } from './wails-adapter'

export class ApiError extends Error {
  status: number
  constructor(message: string, status: number) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

// Wails 桌面端必须走主进程启动的本地 API 服务器。
// 采集数据轮询依赖 /api/daq/latest/{id}，如果这里误用相对路径，请求会打到
// Wails/Vite 的前端资源服务而不是 Go API，表现为“开始采集后 UI 没数据”。
// Wails dev/build 的页面 origin 不稳定：可能是 wails:，也可能是 http://127.0.0.1:9245。
// 因此用 Wails runtime 能力判断桌面环境，而不是用 location.protocol 判断。
//
// 注意：apiBase 必须延迟求值（在 request 调用时再算），不能在模块顶层立即调用
// isWailsAvailable()。原因：wails-adapter.ts 顶层 import 了本文件的 request，
// 形成循环依赖；如果在模块加载阶段调用 isWailsAvailable()，此时 wails-adapter
// 模块尚未执行完，isWailsAvailable 处于 TDZ，会抛
// "Cannot access 'isWailsAvailable' before initialization"，导致整个前端白屏。
let cachedApiBase: string | null = null
function resolveApiBase(): string {
  if (cachedApiBase !== null) return cachedApiBase
  const base = import.meta.env.VITE_API_BASE || (isWailsAvailable() ? 'http://127.0.0.1:8900' : '')
  cachedApiBase = base
  return base
}

/**
 * 判断当前是否运行在 motion-only 子进程窗口中。
 *
 * 主窗口 origin: http://127.0.0.1:8900
 * motion 独立窗口 origin: http://127.0.0.1:8901
 *
 * motion 独立窗口加载 http://127.0.0.1:8901/#/motion，页面 origin 是 8901。
 * 必须用 location.origin 判断，不能用 isWailsAvailable()（两个窗口都注入了 electronAPI）。
 */
function isMotionWindowOrigin(): boolean {
  if (typeof window === 'undefined' || !window.location) return false
  return window.location.origin === 'http://127.0.0.1:8901'
}

export async function request<T>(path: string, init?: RequestInit): Promise<T> {
  // path 可能已被上游（如 traversalApi.ts / otherApis.ts）拼成绝对 URL，
  // 此时直接用它，避免 apiBase 重复拼接导致 "http://host:porthttp://host:port/..." 错误。
  let url: string
  if (path.startsWith('http')) {
    url = path
  } else if (isMotionWindowOrigin() && path === '/api/app/startup-mode') {
    // 例外：motion 独立窗口查询启动模式时，必须走当前 origin（8901 子进程），
    // 而非主进程 8900。否则主进程返回 "normal"，前端无法进入 standalone 模式，
    // 导致 motionApi 的状态轮询走错后端，连接后画面不刷新。
    url = `${window.location.origin}${path}`
  } else {
    url = `${resolveApiBase()}${path}`
  }
  const response = await fetch(url, {
    headers: { 'Content-Type': 'application/json', ...(init?.headers ?? {}) },
    ...init,
  })

  const text = await response.text().catch(() => '')

  if (!response.ok) {
    let message = text || `HTTP ${response.status}`
    try {
      const body = JSON.parse(text) as { error?: unknown }
      if (typeof body.error === 'string' && body.error) message = body.error
    } catch {
      // Keep the raw response when the backend did not return JSON.
    }
    throw new ApiError(message, response.status)
  }

  try {
    return JSON.parse(text) as T
  } catch (e) {
    const detail = e instanceof Error ? e.message : String(e)
    console.error(`[HTTP] JSON 解析失败: ${path}`, detail, '响应内容:', text.slice(0, 500))
    throw new ApiError(`服务端返回了非 JSON 数据 (${detail}): ${text.slice(0, 200)}`, response.status)
  }
}

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
const apiBase = import.meta.env.VITE_API_BASE || (isWailsAvailable() ? 'http://127.0.0.1:8900' : '')

export async function request<T>(path: string, init?: RequestInit): Promise<T> {
  // path 可能已被上游（如 traversalApi.ts / otherApis.ts）拼成绝对 URL，
  // 此时直接用它，避免 apiBase 重复拼接导致 "http://host:porthttp://host:port/..." 错误。
  const url = path.startsWith('http') ? path : `${apiBase}${path}`
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

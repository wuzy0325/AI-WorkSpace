import type { ApiResponse, HealthResponse, StreamEventPayload } from '@/types/api'

// ---------------------------------------------------------------------------
// API 基础路径：桌面模式下指向内嵌 HTTP 服务器，Web 模式下使用相对路径。
// ---------------------------------------------------------------------------
let API_BASE = '/api/v1'

/**
 * 初始化桌面模式下的 API 基础路径。
 * 在 Wails 桌面环境中，通过 Wails 绑定获取内嵌 HTTP 服务器的端口。
 */
export async function initDesktopApiBase(): Promise<void> {
  const w = window as unknown as { go?: { main?: { App?: { GetAPIPort: () => Promise<number> } } } }
  if (typeof window !== 'undefined' && w.go?.main?.App) {
    try {
      const port: number = await w.go.main.App.GetAPIPort()
      API_BASE = `http://127.0.0.1:${port}/api/v1`
    } catch (e) {
      console.warn('Failed to detect Wails API port, falling back to relative path:', e)
    }
    return
  }

  // E2E 测试环境：前端独立运行在 4173 端口时，直接指向后端 API
  if (
    typeof window !== 'undefined' &&
    window.location.hostname === 'localhost' &&
    window.location.port === '4173'
  ) {
    API_BASE = 'http://localhost:18080/api/v1'
  }
}

/** 返回当前 API 基础路径。 */
export function getApiBase(): string {
  return API_BASE
}

export async function fetchHealth(): Promise<HealthResponse> {
  const resp = await fetch(`${API_BASE}/health`)
  if (!resp.ok) {
    throw new Error(`health request failed: ${resp.status}`)
  }

  return (await resp.json()) as HealthResponse
}

// ---------------------------------------------------------------------------
// 通用请求辅助函数
// ---------------------------------------------------------------------------

/** GET 请求，自动解包 ApiResponse.data */
export async function apiGet<T>(path: string): Promise<T> {
  const resp = await requestJSON<ApiResponse<T>>(path)
  return resp.data
}

/** POST 请求，自动序列化 JSON body 并解包 ApiResponse.data */
export async function apiPost<T>(path: string, body?: unknown): Promise<T> {
  const init: RequestInit = { method: 'POST' }
  if (body !== undefined) {
    init.headers = { 'Content-Type': 'application/json' }
    init.body = JSON.stringify(body)
  }
  const resp = await requestJSON<ApiResponse<T>>(path, init)
  return resp.data
}

/** DELETE 请求，自动解包 ApiResponse.data */
export async function apiDelete<T>(path: string): Promise<T> {
  const resp = await requestJSON<ApiResponse<T>>(path, { method: 'DELETE' })
  return resp.data
}

/** PUT 请求，自动序列化 JSON body 并解包 ApiResponse.data */
export async function apiPut<T>(path: string, body?: unknown): Promise<T> {
  const init: RequestInit = { method: 'PUT' }
  if (body !== undefined) {
    init.headers = { 'Content-Type': 'application/json' }
    init.body = JSON.stringify(body)
  }
  const resp = await requestJSON<ApiResponse<T>>(path, init)
  return resp.data
}

export async function requestJSON<T>(path: string, init?: RequestInit): Promise<T> {
  const resp = await fetch(`${API_BASE}${path}`, init)
  if (!resp.ok) {
    let detail = ''
    try {
      const body = await resp.json()
      if (body.message) detail = body.message
      else if (body.code) detail = body.code
    } catch { /* 响应体非 JSON，忽略 */ }
    const msg = detail ? `request failed: ${resp.status} - ${detail}` : `request failed: ${resp.status}`
    throw new Error(msg)
  }

  return (await resp.json()) as T
}

export interface EventStreamOptions {
  onEvent: (payload: StreamEventPayload) => void
  onError?: (error: Event) => void
  onOpen?: () => void
}

export function createEventStream(options: EventStreamOptions): EventSource {
  const { onEvent, onError, onOpen } = options
  const source = new EventSource(`${API_BASE}/events/stream`)

  if (onOpen) {
    source.onopen = onOpen
  }

  if (onError) {
    source.onerror = (error) => {
      onError(error)
    }
  }

  source.onmessage = (event) => {
    try {
      const payload = JSON.parse(event.data) as StreamEventPayload
      onEvent(payload)
    } catch {
      // 忽略解析失败的事件，避免影响后续消息处理。
    }
  }

  return source
}

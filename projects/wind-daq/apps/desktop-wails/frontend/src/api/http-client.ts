export class ApiError extends Error {
  status: number
  constructor(message: string, status: number) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

// Wails 桌面端必须走本地 API 服务器；浏览器开发态保留相对路径以命中 Vite proxy。
const apiBase = import.meta.env.VITE_API_BASE || (window.location.protocol === 'wails:' ? 'http://127.0.0.1:8900' : '')

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
    throw new ApiError(text || `HTTP ${response.status}`, response.status)
  }

  try {
    return JSON.parse(text) as T
  } catch (e) {
    const detail = e instanceof Error ? e.message : String(e)
    console.error(`[HTTP] JSON 解析失败: ${path}`, detail, '响应内容:', text.slice(0, 500))
    throw new ApiError(`服务端返回了非 JSON 数据 (${detail}): ${text.slice(0, 200)}`, response.status)
  }
}

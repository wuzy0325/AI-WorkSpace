export class ApiError extends Error {
  status: number
  constructor(message: string, status: number) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

const apiBase = import.meta.env.VITE_API_BASE ?? ''

export async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${apiBase}${path}`, {
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

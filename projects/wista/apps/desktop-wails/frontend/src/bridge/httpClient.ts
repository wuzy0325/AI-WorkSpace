// HTTP 客户端封装 —— Win7 分支替代 Wails 生成的 Service 绑定。
//
// 设计要点：
//   - 统一响应信封解包：后端所有 RPC 返回 { ok: true, data } | { ok: false, error }
//   - 失败时 reject Error(error)，调用方用 try/catch 或 .catch 处理；
//   - JSON 请求体自动序列化，GET 请求不需要 body；
//   - 后端地址固定为 http://127.0.0.1:18181（与 main.go listenAddr 对应）。
//
// 与原 Wails 绑定的差异：
//   - Wails 绑定是类型安全的（自动生成 .d.ts），HTTP 调用方需手动类型注解；
//   - Wails 绑定调用失败直接 reject 字符串，HTTP 客户端统一 reject Error 对象。

/** 后端 API 根地址 */
const API_BASE = 'http://127.0.0.1:18181'

/** 后端响应信封（与 httpserver/helpers.go apiOK/apiErr 对应） */
interface APIEnvelope<T = unknown> {
  ok: boolean
  data?: T
  error?: string
}

/**
 * 发起 JSON POST/GET 请求并解包响应信封。
 *
 * @param method HTTP 方法（POST/GET/DELETE）
 * @param path 路径，以 / 开头（如 /api/device/scan）
 * @param body 请求体对象（POST/PUT 时序列化为 JSON，GET/DELETE 时忽略）
 * @returns 成功时返回 data 字段值；失败时 reject Error(error 信息)
 */
async function request<T>(
  method: 'POST' | 'GET' | 'DELETE',
  path: string,
  body?: unknown,
): Promise<T> {
  const init: RequestInit = {
    method,
    headers: { 'Content-Type': 'application/json' },
  }
  if (body !== undefined && method === 'POST') {
    init.body = JSON.stringify(body)
  }

  let resp: Response
  try {
    resp = await fetch(API_BASE + path, init)
  } catch (err) {
    // fetch 抛错通常是网络问题（后端未启动、连接拒绝）
    throw new Error(`网络请求失败: ${err instanceof Error ? err.message : String(err)}`)
  }

  // 后端始终返回 JSON（即使 4xx/5xx 也写入了信封），统一解析
  let env: APIEnvelope<T>
  try {
    env = (await resp.json()) as APIEnvelope<T>
  } catch (err) {
    throw new Error(`响应解析失败: ${err instanceof Error ? err.message : String(err)}`)
  }

  if (!env.ok) {
    // 失败信封：reject 携带后端 error 字符串
    throw new Error(env.error ?? `请求失败 (${resp.status})`)
  }

  // 成功信封：返回 data（无返回值的 RPC data 为 undefined，调用方按 void 处理）
  return env.data as T
}

/** POST 请求便捷封装 */
export function post<T = void>(path: string, body?: unknown): Promise<T> {
  return request<T>('POST', path, body)
}

/** GET 请求便捷封装 */
export function get<T = unknown>(path: string): Promise<T> {
  return request<T>('GET', path)
}

/** DELETE 请求便捷封装 */
export function del<T = void>(path: string): Promise<T> {
  return request<T>('DELETE', path)
}

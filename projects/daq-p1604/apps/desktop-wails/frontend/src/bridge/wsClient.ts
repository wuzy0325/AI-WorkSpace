// WebSocket 客户端单例 —— Win7 分支替代 @wailsio/runtime 的 Events.On/Off。
//
// 设计要点：
//   - 单例 WebSocket：所有事件订阅复用同一连接，避免每订阅一次开一条连接；
//   - 事件路由：onmessage 解析 { event, data } 信封后按 event 名分发到各 handler；
//   - 自动重连：连接断开时指数退避重试（1s → 2s → 4s → ... 上限 10s），
//     后端 Go 进程尚未启动或重启时前端能自愈；
//   - 订阅句柄：on() 返回 unsubscribe 函数，调用方在 onUnmounted 中调用即可，
//     也可调用 off(name, handler) 显式解绑。
//
// 与原 @wailsio/runtime 的差异：
//   - Events.On 返回 unsubscribe 函数；本实现 on() 也返回 unsubscribe，签名兼容；
//   - 原实现回调入参形态为 `{ data: T }`，本实现直接传 data，bridge 层做兼容转换。
//
// daq-p1604 特殊说明：
//   - daq:device-state 是多参数事件，后端 WSHub.Emit(name, id, state) 会把
//     [id, state] 打包为数组推送，前端 onmessage 收到的 data 是数组；
//     deviceBridge 层订阅时需解构数组：on<[string, DeviceState]>('daq:device-state', ([id, state]) => ...)

/** 后端推送的 WebSocket 消息信封（与 httpserver/ws_hub.go wsEnvelope 对应） */
interface WSEnvelope {
  event: string
  data: unknown
}

/** 事件 handler 类型：接收 data（已从信封解出） */
type EventHandler<T = unknown> = (data: T) => void

/** 事件名 → handler 集合。一个事件可被多个订阅者监听。 */
const handlers = new Map<string, Set<EventHandler>>()

/** 单例 WebSocket 实例 */
let ws: WebSocket | null = null

/** 当前重连间隔（毫秒），指数退避 */
let reconnectDelay = 1000

/** 重连定时器句柄，连接成功或显式 close 时清除 */
let reconnectTimer: ReturnType<typeof setTimeout> | null = null

/** 标记是否处于显式关闭状态（true 时不再自动重连） */
let manuallyClosed = false

/** 标记是否已初始化（避免重复 attach 同一连接） */
let initialized = false

/**
 * 初始化 WebSocket 单例连接。
 *
 * 幂等：多次调用不会重复创建连接，仅首次调用时生效。
 * 后端地址固定为 ws://127.0.0.1:18182/ws（与 main.go listenAddr 对应）。
 */
export function initWebSocket(): void {
  if (initialized) return
  initialized = true
  manuallyClosed = false
  connect()
}

/**
 * 建立一次 WebSocket 连接，绑定 onopen/onclose/onerror/onmessage 回调。
 *
 * 注意：构造函数立即返回，连接异步建立。onopen 触发后重置退避。
 */
function connect(): void {
  // 浏览器/Electron renderer 环境才有 WebSocket；SSR 或测试环境跳过
  if (typeof WebSocket === 'undefined') return

  try {
    ws = new WebSocket('ws://127.0.0.1:18182/ws')
  } catch (err) {
    // 构造失败（极少见，通常是 URL 非法）→ 触发重连
    console.error('[wsClient] WebSocket construction failed:', err)
    scheduleReconnect()
    return
  }

  ws.onopen = () => {
    reconnectDelay = 1000 // 连接成功，重置退避
    console.info('[wsClient] connected')
  }

  ws.onclose = () => {
    ws = null
    if (!manuallyClosed) {
      scheduleReconnect()
    }
  }

  ws.onerror = (err) => {
    // 不在此处 scheduleReconnect，让 onclose 兜底
    // 浏览器对 ws error 事件不暴露详情，统一在 close 时重连
    console.warn('[wsClient] error event:', err)
  }

  ws.onmessage = (event: MessageEvent) => {
    if (typeof event.data !== 'string') return
    let env: WSEnvelope
    try {
      env = JSON.parse(event.data) as WSEnvelope
    } catch (err) {
      console.warn('[wsClient] failed to parse message:', err)
      return
    }
    dispatch(env.event, env.data)
  }
}

/**
 * 指数退避重连。
 *
 * 间隔序列：1s → 2s → 4s → 8s → 10s（上限 10s）
 * 上限设为 10s 是平衡：太短会增加无意义请求，太长会让用户感觉"卡住"。
 */
function scheduleReconnect(): void {
  if (reconnectTimer !== null) return
  const delay = Math.min(reconnectDelay, 10000)
  reconnectTimer = setTimeout(() => {
    reconnectTimer = null
    reconnectDelay = Math.min(reconnectDelay * 2, 10000)
    connect()
  }, delay)
}

/**
 * 分发事件到所有订阅者。
 *
 * 同步遍历 handlers 集合；handler 内部抛错会被 try/catch 兜住，
 * 避免单个订阅者异常影响其他订阅者收到事件。
 */
function dispatch(event: string, data: unknown): void {
  const set = handlers.get(event)
  if (!set || set.size === 0) return
  // 复制一份遍历，避免 handler 中调用 off() 修改集合导致迭代异常
  for (const handler of Array.from(set)) {
    try {
      handler(data)
    } catch (err) {
      console.error(`[wsClient] handler for "${event}" threw:`, err)
    }
  }
}

/**
 * 订阅事件。
 *
 * @param event 事件名（与后端 core.EventXxx 常量对应）
 * @param handler 回调函数，入参为信封中的 data 字段
 * @returns unsubscribe 函数，调用后解除订阅
 */
export function on<T = unknown>(event: string, handler: EventHandler<T>): () => void {
  let set = handlers.get(event)
  if (!set) {
    set = new Set()
    handlers.set(event, set)
  }
  set.add(handler as EventHandler)
  // 确保连接已初始化（幂等）
  initWebSocket()
  return () => off(event, handler)
}

/**
 * 解除订阅。
 *
 * 同时传 (event, handler) 时只移除该 handler；
 * 只传 event 时移除该事件全部 handler；
 * 都不传时清空所有事件订阅。
 */
export function off<T = unknown>(event?: string, handler?: EventHandler<T>): void {
  if (event === undefined) {
    handlers.clear()
    return
  }
  if (handler === undefined) {
    handlers.delete(event)
    return
  }
  const set = handlers.get(event)
  if (set) {
    set.delete(handler as EventHandler)
    if (set.size === 0) handlers.delete(event)
  }
}

/**
 * 显式关闭 WebSocket 连接，不再自动重连。
 *
 * 仅应用卸载或测试清理时调用。正常情况下应让连接保持，
 * 由后端断开时自动重连。
 */
export function closeWebSocket(): void {
  manuallyClosed = true
  if (reconnectTimer !== null) {
    clearTimeout(reconnectTimer)
    reconnectTimer = null
  }
  if (ws !== null) {
    ws.onclose = null // 阻止 onclose 触发重连
    ws.close()
    ws = null
  }
  handlers.clear()
  initialized = false
}

// motionApi.ts 是前端调用 Go 后端"运动控制"相关 HTTP API 的封装层（Win7 版）。
//
// 与 trunk 主分支的差异：
//   - 移除 Wails adapter 依赖（isWailsAvailable / wailsApi），所有调用统一走 HTTP fetch
//   - 所有 motion 调用使用 MOTION_HTTP_BASE 绝对地址（http://127.0.0.1:16888），
//     不再依赖 http-client.ts 的 apiBase（避免 dev/prod 配置漂移）
//   - 状态轮询周期保持 200ms（与原 Wails adapter onStatusEvent 一致）
//
// 路由由 shared.local/motion-control/go/httpapi.RegisterMotionRoutes 注册：
//   GET    /api/motion/profiles       列出所有控制器配置
//   PUT    /api/motion/profiles       新增/更新控制器配置
//   DELETE /api/motion/profiles/{id}  删除控制器配置
//   GET    /api/motion/status         获取所有控制器实时状态
//   POST   /api/motion/connect        连接控制器
//   POST   /api/motion/disconnect     断开控制器
//   POST   /api/motion/home           回零
//   POST   /api/motion/moveTo         绝对移动
//   POST   /api/motion/moveBy         相对移动
//   POST   /api/motion/jog            点动
//   POST   /api/motion/stop           停止
//   POST   /api/motion/emergencyStop  紧急停止
//   POST   /api/motion/resetEmergencyStop 解除紧急停止
//   POST   /api/motion/definePosition 定义当前位置

import type { AxisName, MotionControllerProfile, MotionControllerStatus } from '@shared/types/motion'

export interface MotionAxisStatus {
  name: string
  position: number
  homed: boolean
  moving: boolean
}

export interface MotionStatus {
  connected: boolean
  axes: MotionAxisStatus[]
}

const MOTION_STORAGE_KEY = 'motion-controller.profiles'

// HTTP API 基础 URL：与 main.go listenAddr 一致。
// 端口 16888：与 wind-daq（8900/8901）/ daq-t1603（18181）/ daq-p1604（18182）/ probe-interpolator（18183）区分。
export const MOTION_HTTP_BASE = 'http://127.0.0.1:16888'

const DEFAULT_AXES: import('@shared/types/motion').AxisConfig[] = [
  { name: 'X', enabled: true, kind: 'LINEAR' as const, maxSpeed: 10, stepsPerRev: 1.8, microSteps: 4, lead: 4, gearRatio: 1, positionSource: 'register', encoderScale: 0.005 },
  { name: 'Y', enabled: true, kind: 'LINEAR' as const, maxSpeed: 10, stepsPerRev: 1.8, microSteps: 4, lead: 4, gearRatio: 1, positionSource: 'register', encoderScale: 0.005 },
  { name: 'Z', enabled: true, kind: 'LINEAR' as const, maxSpeed: 10, stepsPerRev: 1.8, microSteps: 4, lead: 4, gearRatio: 1, positionSource: 'register', encoderScale: 0.005 },
  { name: 'U', enabled: true, kind: 'ROTARY' as const, maxSpeed: 10, stepsPerRev: 1.8, microSteps: 4, lead: 4, gearRatio: 1, positionSource: 'register', encoderScale: 0.005 },
]

function normalizeMotionProfile(profile: MotionControllerProfile): MotionControllerProfile {
  let axes: import('@shared/types/motion').AxisConfig[]
  if (Array.isArray(profile?.axes) && profile.axes.length > 0) {
    axes = profile.axes.map((a) => ({ ...a, enabled: true }))
  } else {
    axes = DEFAULT_AXES.map((a) => ({ ...a }))
  }
  return { ...profile, axes }
}

function normalizeMotionProfiles(profiles: unknown): MotionControllerProfile[] {
  if (!Array.isArray(profiles)) return []
  return profiles
    .filter((profile): profile is MotionControllerProfile => typeof profile === 'object' && profile !== null)
    .map((profile) => normalizeMotionProfile(profile))
}

// storedProfiles 在后端不可用时返回 localStorage 缓存的配置（兜底，避免空白页）。
// 包含一个 simulated 默认配置，便于首次启动展示示例。
function storedProfiles(): MotionControllerProfile[] {
  try {
    const raw = window.localStorage.getItem(MOTION_STORAGE_KEY)
    if (raw) return normalizeMotionProfiles(JSON.parse(raw))
  } catch { /* 忽略 localStorage 不可用（隐私模式等） */ }
  return [{
    id: 'sim-mc-1',
    name: 'Simulated Controller 1',
    type: 'SIMULATED-MC' as const,
    address: '127.0.0.1',
    port: 5176,
    autoConnect: false,
    axes: DEFAULT_AXES.map((a) => ({ ...a })),
  }]
}

function saveProfiles(profiles: MotionControllerProfile[]): void {
  window.localStorage.setItem(MOTION_STORAGE_KEY, JSON.stringify(profiles))
}

// motionFetch 是所有 motion HTTP 调用的统一封装：
//   - 拼 MOTION_HTTP_BASE 前缀，避免依赖 http-client.ts 的 apiBase
//   - 默认 Content-Type: application/json
//   - 非 2xx 响应抛 Error，错误信息为响应体文本（便于诊断后端错误）
async function motionFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const resp = await fetch(`${MOTION_HTTP_BASE}${path}`, {
    headers: { 'Content-Type': 'application/json', ...(init?.headers ?? {}) },
    ...init,
  })
  if (!resp.ok) {
    const detail = await resp.text().catch(() => `HTTP ${resp.status}`)
    throw new Error(detail)
  }
  // 204 No Content 或空 body 不解析 JSON
  const text = await resp.text()
  if (!text) return undefined as T
  return JSON.parse(text) as T
}

// goStatusToControllerStatus 把后端 /api/motion/status 返回的实时状态合并到本地配置上，
// 保证未连接的控制器也能展示轴结构（位置/回零状态用默认值）。
async function goStatusToControllerStatus(profiles: MotionControllerProfile[]): Promise<MotionControllerStatus[]> {
  let raw: MotionControllerStatus[] = []
  let statusError = ''
  try {
    raw = await motionFetch<MotionControllerStatus[]>('/api/motion/status')
  } catch (e) {
    statusError = e instanceof Error ? e.message : '状态服务不可用'
  }
  if (!Array.isArray(raw) || raw.length === 0) {
    raw = []
  }
  return normalizeMotionProfiles(profiles).map((p) => {
    const live = raw.find((s) => s.id === p.id)
    const profileAxes = Array.isArray(p.axes) ? p.axes : []

    let enabledAxes = profileAxes.filter((a) => a.enabled !== false)
    if (enabledAxes.length === 0 && live && Array.isArray(live.axes) && live.axes.length > 0) {
      enabledAxes = live.axes.map((axis) => ({
        name: axis.name as import('@shared/types/motion').AxisName,
        enabled: true,
        kind: 'LINEAR' as const,
      }))
    }

    return {
      id: p.id,
      name: p.name,
      type: p.type,
      connected: live?.connected ?? false,
      emergencyStopped: live?.emergencyStopped ?? false,
      axes: enabledAxes.map((a) => {
        const axisLive = live?.axes?.find((x) => x.name === a.name)
        return {
          name: a.name,
          position: axisLive?.position ?? 0,
          homed: axisLive?.homed ?? false,
          moving: axisLive?.moving ?? false,
          posLimit: axisLive?.posLimit ?? false,
          negLimit: axisLive?.negLimit ?? false,
        }
      }),
      lastError: live?.lastError ?? statusError,
    }
  })
}

type StatusCallback = (status: MotionControllerStatus[]) => void

// statusListeners 用 Set 收集所有状态订阅者，单实例轮询复用，避免每个组件都起一个轮询 goroutine
const statusListeners = new Set<StatusCallback>()

export const motionApi = {
  getProfiles: async (): Promise<MotionControllerProfile[]> => {
    // 优先从后端拉取最新配置，失败时降级到 localStorage 缓存
    try {
      const profiles = await motionFetch<MotionControllerProfile[]>('/api/motion/profiles')
      const normalized = normalizeMotionProfiles(profiles)
      saveProfiles(normalized)
      return normalized
    } catch {
      return storedProfiles()
    }
  },

  getStatusAll: async (): Promise<MotionControllerStatus[]> => {
    const profiles = await motionApi.getProfiles()
    return goStatusToControllerStatus(profiles)
  },

  upsertProfile: async (profile: MotionControllerProfile): Promise<void> => {
    await motionFetch('/api/motion/profiles', { method: 'PUT', body: JSON.stringify(profile) })
    // 同步更新 localStorage 缓存，后端不可用时下次 getProfiles 仍能返回最新值
    const profiles = storedProfiles()
    const idx = profiles.findIndex((p) => p.id === profile.id)
    if (idx >= 0) profiles[idx] = profile
    else profiles.push(profile)
    saveProfiles(profiles)
  },

  deleteProfile: async (id: string): Promise<void> => {
    await motionFetch(`/api/motion/profiles/${encodeURIComponent(id)}`, { method: 'DELETE' })
    const profiles = storedProfiles().filter((p) => p.id !== id)
    saveProfiles(profiles)
  },

  connect: async (id: string): Promise<boolean> => {
    await motionFetch('/api/motion/connect', { method: 'POST', body: JSON.stringify({ id }) })
    return true
  },

  disconnect: async (id: string): Promise<void> => {
    await motionFetch('/api/motion/disconnect', { method: 'POST', body: JSON.stringify({ id }) })
  },

  moveTo: async (id: string, axis: AxisName, position: number): Promise<boolean> => {
    await motionFetch('/api/motion/moveTo', { method: 'POST', body: JSON.stringify({ id, axis, position }) })
    return true
  },

  moveBy: async (id: string, axis: AxisName, delta: number): Promise<boolean> => {
    await motionFetch('/api/motion/moveBy', { method: 'POST', body: JSON.stringify({ id, axis, delta }) })
    return true
  },

  jog: async (id: string, axis: AxisName, direction: 'forward' | 'reverse', speed?: number): Promise<boolean> => {
    // jog 速度方向：forward 为正速度，reverse 为负速度
    const velocity = (direction === 'forward' ? 1 : -1) * (speed ?? 1)
    await motionFetch('/api/motion/jog', { method: 'POST', body: JSON.stringify({ id, axis, velocity }) })
    return true
  },

  home: async (id: string, axis: AxisName): Promise<boolean> => {
    await motionFetch('/api/motion/home', { method: 'POST', body: JSON.stringify({ id, axis }) })
    return true
  },

  stop: async (id: string, axis?: AxisName): Promise<boolean> => {
    // axis 为空时表示停止所有轴，后端按空串处理
    await motionFetch('/api/motion/stop', { method: 'POST', body: JSON.stringify({ id, axis: axis ?? '' }) })
    return true
  },

  emergencyStop: async (id: string): Promise<boolean> => {
    await motionFetch('/api/motion/emergencyStop', { method: 'POST', body: JSON.stringify({ id }) })
    return true
  },

  resetEmergencyStop: async (id: string): Promise<boolean> => {
    await motionFetch('/api/motion/resetEmergencyStop', { method: 'POST', body: JSON.stringify({ id }) })
    return true
  },

  definePosition: async (id: string, axis: AxisName, position: number): Promise<boolean> => {
    await motionFetch('/api/motion/definePosition', { method: 'POST', body: JSON.stringify({ id, axis, position }) })
    return true
  },

  // onStatusUpdated 订阅状态更新，200ms 轮询一次（与原 Wails adapter 周期一致）。
  // 返回取消订阅函数，组件 unmount 时必须调用，避免内存泄漏。
  onStatusUpdated: (cb: StatusCallback): (() => void) => {
    statusListeners.add(cb)
    ensureStatusPolling()
    return () => { statusListeners.delete(cb) }
  },
}

// statusPollingStarted 保证全局只启动一个轮询 goroutine，
// 当所有 listener 都取消订阅时停止轮询，避免空转浪费 CPU。
let statusPollingStarted = false

function ensureStatusPolling(): void {
  if (statusPollingStarted) return
  statusPollingStarted = true
  const poll = async () => {
    if (statusListeners.size === 0) {
      statusPollingStarted = false
      return
    }
    try {
      const all = await motionApi.getStatusAll()
      statusListeners.forEach((listener) => {
        try { listener(all) } catch { /* 忽略单个回调异常，避免影响其他订阅者 */ }
      })
    } finally {
      setTimeout(poll, 200)
    }
  }
  poll()
}

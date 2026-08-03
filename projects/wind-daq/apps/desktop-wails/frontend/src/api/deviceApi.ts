import { ApiError, request } from '@api/http-client'
import type { DeviceProfile, DeviceStatus, DataPayload, ScanResult, DSA3217ScanConfig, CalibrationResult, CalibrationRecord, CalibrationProgress } from '@api/types'
import { subscribeDaqStream, type SseSubscription } from '@api/sse-client'
import { isWailsAvailable, wailsApi } from '@api/wails-adapter'
export { motionApi } from './motionApi'
export { calibrationApi } from './calibrationApi'
export { traversalApi, storageApi, reportApi } from './otherApis'
export type { TraversalPoint, MotionAxisStatus, MotionStatus } from './otherApis'

const DEFAULT_PUBLISH_RATE_HZ = 20
const MIN_POLL_INTERVAL_MS = 50

// 同步备忘：默认设备配置目前共有 4 处副本，修改单位 / 默认范围 / 精度时必须保持一致：
//   1) projects/wind-daq/apps/desktop-wails/config/device-profiles.json
//   2) projects/wind-daq/services/api-go/config/device-profiles.json
//   3) projects/wind-daq/services/api-go/pkg/apiserver/config/device-profiles.json
//   4) defaultSimulatedProfile()（本文件）
// 持久化数据中如出现旧 kPa 单位，由 deviceStore.migrateAtmPressureUnit 在加载阶段做兼容升级。
// TODO(架构): 后续应集中到 shared/device-presets/ 由后端 API 暴露，避免漂移。

export function defaultSimulatedProfile(): DeviceProfile {
  return {
    id: 'sim-1',
    name: 'Simulator 1',
    type: 'SIMULATED',
    transport: 'tcp',
    address: '127.0.0.1',
    port: 0,
    serialPort: '',
    baudRate: 115200,
    autoConnect: true,
    samplingRate: 20,
    channels: [
      ...Array.from({ length: 16 }, (_, index) => ({
        index,
        name: `CH${index + 1}`,
        enabled: true,
        unit: 'V',
        precision: 3,
        rangeMin: -10,
        rangeMax: 10,
      })),
      { index: 16, name: '大气压', enabled: true, unit: 'Pa', precision: 2, rangeMin: 99000, rangeMax: 106000 },
      { index: 17, name: '大气温度', enabled: true, unit: 'degC', precision: 2, rangeMin: 20, rangeMax: 25 },
    ],
  }
}

type SnapshotCallback = (payload: DataPayload) => void
type StatusCallback = (status: DeviceStatus[]) => void
// 设备异常退出回调：轮询 getLatest 拿到 404 时触发，
// 让 deviceStore 更新 UI 状态为 Error，避免 UI 永远显示"采集中"。
type DeviceLostCallback = (deviceId: string) => void
type DeviceSubscription = { unsubscribe: () => void; restart?: () => void }

type WailsResponse = { Success?: boolean; Error?: string; success?: boolean; error?: string }

function publishRateToPollIntervalMs(hz: number): number {
  if (!Number.isFinite(hz) || hz <= 0) hz = DEFAULT_PUBLISH_RATE_HZ
  return Math.max(MIN_POLL_INTERVAL_MS, Math.round(1000 / hz))
}

function normalizeDeviceProfile(profile: DeviceProfile): DeviceProfile {
  return {
    ...profile,
    channels: Array.isArray(profile?.channels) ? profile.channels : [],
  }
}

function normalizeDeviceProfiles(profiles: unknown): DeviceProfile[] {
  if (!Array.isArray(profiles)) return []
  return profiles
    .filter((profile): profile is DeviceProfile => typeof profile === 'object' && profile !== null)
    .map((profile) => normalizeDeviceProfile(profile))
}

function normalizeDataPayload(raw: unknown, deviceIdFallback?: string): DataPayload {
  const d = raw as Record<string, unknown> | undefined | null
  return {
    deviceId: typeof d?.deviceId === 'string' ? d.deviceId : (deviceIdFallback ?? ''),
    timestamp: typeof d?.timestamp === 'number' ? d.timestamp : 0,
    channels: Array.isArray(d?.channels) ? d.channels as number[] : [],
    channelIndices: Array.isArray(d?.channelIndices) ? d.channelIndices as number[] : [],
  }
}

function wailsOk(res: WailsResponse): { success: boolean } {
  const success = res.Success ?? res.success ?? false
  const error = res.Error ?? res.error
  if (!success && error) {
    throw new Error(error)
  }
  return { success }
}

export const deviceApi = {
  scanDevices: async (): Promise<ScanResult[]> => {
    if (isWailsAvailable()) {
      const results = await wailsApi.device.scanDevices()
      return (results ?? []) as ScanResult[]
    }
    return request<ScanResult[]>('/api/device/scan')
  },

  getProfiles: async (): Promise<DeviceProfile[]> => {
    if (isWailsAvailable()) {
      return normalizeDeviceProfiles(await wailsApi.device.getProfiles())
    }
    return normalizeDeviceProfiles(await request<DeviceProfile[]>('/api/device/profiles'))
  },

  upsertProfile: async (profile: DeviceProfile): Promise<{ success: boolean }> => {
    if (isWailsAvailable()) {
      return wailsOk(await wailsApi.device.upsertProfile(profile as any))
    }
    return request<{ success: boolean }>('/api/device/profiles', { method: 'PUT', body: JSON.stringify(profile) })
  },

  deleteProfile: async (id: string): Promise<{ success: boolean }> => {
    if (isWailsAvailable()) {
      return wailsOk(await wailsApi.device.deleteProfile(id))
    }
    return request<{ success: boolean }>(`/api/device/profiles/${id}`, { method: 'DELETE' })
  },

  connect: async (id: string): Promise<{ success: boolean }> => {
    if (isWailsAvailable()) {
      return wailsOk(await wailsApi.device.connect(id))
    }
    return request<{ success: boolean }>(`/api/device/${id}/connect`, { method: 'POST' })
  },

  disconnect: async (id: string): Promise<{ success: boolean }> => {
    if (isWailsAvailable()) {
      return wailsOk(await wailsApi.device.disconnect(id))
    }
    return request<{ success: boolean }>(`/api/device/${id}/disconnect`, { method: 'POST' })
  },

  startAcquisition: async (id: string): Promise<{ success: boolean }> => {
    if (isWailsAvailable()) {
      return wailsOk(await wailsApi.device.startAcquisition(id))
    }
    return request<{ success: boolean }>(`/api/device/${id}/startAcquisition`, { method: 'POST' })
  },

  stopAcquisition: async (id: string): Promise<{ success: boolean }> => {
    if (isWailsAvailable()) {
      return wailsOk(await wailsApi.device.stopAcquisition(id))
    }
    return request<{ success: boolean }>(`/api/device/${id}/stopAcquisition`, { method: 'POST' })
  },

  getStatus: async (id: string): Promise<DeviceStatus> => {
    if (isWailsAvailable()) {
      const result = await wailsApi.device.getStatus(id)
      // Wails binding 偶尔返回 boolean/null（旧版签名遗留），用 typeof 而非 === true/false
      // 避免 TS 严格模式判定 DeviceStatus 与 boolean 无交集报错
      if (result == null || typeof result === 'boolean') {
        throw new Error('设备状态不可用')
      }
      return result as DeviceStatus
    }
    return request<DeviceStatus>(`/api/device/${id}/status`)
  },

  getLatest: async (id: string): Promise<DataPayload> => {
    try {
      const result = await request<DataPayload>(`/api/daq/latest/${id}`)
      return normalizeDataPayload(result, id)
    } catch (err) {
      // 404 表示设备已断开/异常退出（DeviceManager 从 map 删除）。
      // 必须向上抛出，让轮询层感知断连并更新 UI 状态，避免 UI 永远显示"采集中"。
      // 其它错误（网络抖动、临时 5xx）仍返回空 payload，保持轮询容错。
      if (err instanceof ApiError && err.status === 404) {
        throw err
      }
      return { deviceId: id, timestamp: 0, channels: [], channelIndices: [] }
    }
  },

  getPublishRate: async (): Promise<number> => {
    if (isWailsAvailable()) {
      const hz = await wailsApi.device.getPublishRate()
      deviceApi._publishRateHz = hz
      return hz
    }
    return request<{ hz: number }>('/api/daq/publishRate').then((result) => {
      deviceApi._publishRateHz = result.hz
      return result.hz
    })
  },

  setPublishRate: async (hz: number): Promise<{ success: boolean }> => {
    const result = isWailsAvailable()
      ? wailsOk(await wailsApi.device.setPublishRate(hz))
      : await request<{ success: boolean }>('/api/daq/publishRate', { method: 'PUT', body: JSON.stringify({ hz }) })
    if (result.success) {
      deviceApi._publishRateHz = hz
      deviceApi._restartPollingSubscriptions()
    }
    return result
  },

  getDsa3217ScanConfig: async (
    id: string
  ): Promise<{ success: boolean; data?: DSA3217ScanConfig; error?: string }> => {
    if (isWailsAvailable()) {
      const res = await wailsApi.device.getDsa3217ScanConfig(id) as any
      return {
        success: res.Success ?? res.success ?? false,
        data: (res.Data ?? res.data) as DSA3217ScanConfig | undefined,
        error: res.Error ?? res.error,
      }
    }
    return request<{ success: boolean; data?: DSA3217ScanConfig; error?: string }>(`/api/device/${id}/dsa3217ScanConfig`)
  },

  applyDsa3217ScanConfig: async (
    id: string,
    config: { avg: number; period: number }
  ): Promise<{ success: boolean; data?: DSA3217ScanConfig; error?: string }> => {
    if (isWailsAvailable()) {
      const res = await wailsApi.device.applyDsa3217ScanConfig(id, config.avg, config.period) as any
      return {
        success: res.Success ?? res.success ?? false,
        data: (res.Data ?? res.data) as DSA3217ScanConfig | undefined,
        error: res.Error ?? res.error,
      }
    }
    return request<{ success: boolean; data?: DSA3217ScanConfig; error?: string }>(`/api/device/${id}/dsa3217ScanConfig`, {
      method: 'PUT',
      body: JSON.stringify(config),
    })
  },

  // DAQ-P-1603 配置 API
  // 与 getLatest 模式对齐：完全走本地 HTTP API（Wails 环境下 backend 启动
  // 了 127.0.0.1:8900 本地 API server），不通过 Wails binding 调用，
  // 避免在 backend/app.go 中新增 binding 方法并重新生成 bindings。
  // 后端 server.go 的 daqP1603Config 端点已统一处理 Wails/HTTP 两种环境。
  getDaqP1603Config: async (
    id: string
  ): Promise<{ success: boolean; data?: DeviceProfile; error?: string }> => {
    return request<{ success: boolean; data?: DeviceProfile; error?: string }>(`/api/device/${id}/daqP1603Config`)
  },

  applyDaqP1603Config: async (
    id: string,
    profile: DeviceProfile
  ): Promise<{ success: boolean; data?: DeviceProfile; error?: string }> => {
    return request<{ success: boolean; data?: DeviceProfile; error?: string }>(`/api/device/${id}/daqP1603Config`, {
      method: 'PUT',
      body: JSON.stringify(profile),
    })
  },

  calibrate: async (id: string, channelIndex?: number, signal?: AbortSignal): Promise<CalibrationResult[]> => {
    const query = channelIndex == null ? '' : `?channelIndex=${encodeURIComponent(channelIndex)}`
    const result = await request<{ success: boolean; data: CalibrationResult[] }>(`/api/v1/devices/${id}/calibrate${query}`, {
      method: 'PUT',
      signal,
    })
    return result.data ?? []
  },

  getCalibration: async (id: string, channelIndex: number): Promise<CalibrationRecord> =>
    request<CalibrationRecord>(`/api/v1/devices/${id}/calibration?channelIndex=${encodeURIComponent(channelIndex)}`),

  getCalibrationProgress: async (id: string): Promise<CalibrationProgress> =>
    request<CalibrationProgress>(`/api/v1/devices/${id}/calibrationProgress`),

  clearCalibration: async (id: string, channelIndex: number): Promise<{ success: boolean }> =>
    request<{ success: boolean }>(`/api/v1/devices/${id}/clearCalibration?channelIndex=${encodeURIComponent(channelIndex)}`, { method: 'POST' }),

  setCalibrationEnabled: async (id: string, channelIndex: number, enabled: boolean): Promise<{ success: boolean }> =>
    request<{ success: boolean }>(`/api/v1/devices/${id}/calibrationEnabled`, {
      method: 'PUT',
      body: JSON.stringify({ channelIndex, enabled }),
    }),

  getCalibrationEnabled: async (id: string, channelIndex: number): Promise<boolean> => {
    const result = await request<{ enabled: boolean }>(`/api/v1/devices/${id}/calibrationEnabled?channelIndex=${encodeURIComponent(channelIndex)}`)
    return result.enabled
  },

  // 校零配置元数据（采样时长等），供前端避免硬编码 5s。
  getCalibrationConfig: async (): Promise<{ durationSec: number }> =>
    request<{ durationSec: number }>(`/api/v1/calibrationConfig`),

  _snapshotListeners: new Set<SnapshotCallback>(),
  _statusListeners: new Set<StatusCallback>(),
  _deviceLostListeners: new Set<DeviceLostCallback>(),
  _subscriptions: new Map<string, DeviceSubscription>(),
  // 同一设备的底层轮询由多个页面共享；最后一个 owner 释放后才真正关闭。
  _subscriptionOwners: new Map<string, Set<string>>(),
  _publishRateHz: DEFAULT_PUBLISH_RATE_HZ,

  onSnapshot: (cb: SnapshotCallback): (() => void) => {
    deviceApi._snapshotListeners.add(cb)
    return () => { deviceApi._snapshotListeners.delete(cb) }
  },

  onStatusUpdated: (cb: StatusCallback): (() => void) => {
    deviceApi._statusListeners.add(cb)
    return () => { deviceApi._statusListeners.delete(cb) }
  },

  // 注册设备异常退出回调。轮询 getLatest 拿到 404 时触发，
  // 让订阅者（deviceStore）更新 UI 状态为 Error。
  onDeviceLost: (cb: DeviceLostCallback): (() => void) => {
    deviceApi._deviceLostListeners.add(cb)
    return () => { deviceApi._deviceLostListeners.delete(cb) }
  },

  _notifyDeviceLost: (deviceId: string): void => {
    deviceApi._deviceLostListeners.forEach((cb) => cb(deviceId))
  },

  _notifyListeners: (payload: unknown) => {
    deviceApi._snapshotListeners.forEach((cb) => cb(normalizeDataPayload(payload)))
  },

  _restartPollingSubscriptions: (): void => {
    if (!isWailsAvailable()) return
    deviceApi._subscriptions.forEach((subscription) => subscription.restart?.())
  },

  /** 注册设备订阅，返回取消订阅函数（DRY：抽取 Wails/非Wails 分支的公共注册逻辑） */
  _registerSubscription: (deviceId: string, subscription: DeviceSubscription): void => {
    deviceApi._subscriptions.set(deviceId, subscription)
  },

  // Wails 模式下采用 HTTP 轮询而非 Wails Event 推送：
  // 后端 AcquisitionHub 默认 20Hz 节流，若用 app.Event.Emit("daq:payload") 推送，
  // Wails v3 的 Emit 内部通过 InvokeSync 在 GUI 主线程同步执行 WebView2 ExecuteScript，
  // 20Hz 高频同步主线程 JS 调用会让 WebView2 返回 EINVAL
  // ("[WebView2] Eval failed: invalid argument")，同时阻塞 GUI 主线程，
  // 导致 startAcquisition 等 Wails binding 调用延迟甚至失败，UI 表现为
  // "开始采集后无数据更新"。后端 startDataRelay 已配合改为仅 drain payload，
  // 不再调用 app.Event.Emit；前端这里按全局刷新频率轮询 Go 标准库 HTTP，
  // 绕开 Wails 反射桥，稳定可靠。AcquisitionHub.OnData 始终更新 latestByDevice，
  // 不受 publishHz 节流影响；轮询间隔使用全局刷新频率设置，并在保存设置后重建。
  subscribeToDevice: (deviceId: string, owner = 'default'): void => {
    let owners = deviceApi._subscriptionOwners.get(deviceId)
    if (!owners) {
      owners = new Set<string>()
      deviceApi._subscriptionOwners.set(deviceId, owners)
    }
    owners.add(owner)
    if (deviceApi._subscriptions.has(deviceId)) return

    if (isWailsAvailable()) {
      void wailsApi.device.subscribeStream(deviceId, true)
      let active = true
      let timer: number | null = null
      // generation token 用于识别"哪一轮轮询"。
      // 当 restart() 在 pollLatest 等待 getLatest() 期间被调用时，
      // 旧 generation 的 pollLatest 即使 finally 块执行，
      // 也会因为 myGen !== generation 而跳过 setTimeout 调度，
      // 避免旧轮询 goroutine 仍持有 timer 引用导致重叠轮询。
      let generation = 0
      const clearTimer = () => {
        if (timer !== null) {
          window.clearTimeout(timer)
          timer = null
        }
      }
      const pollLatest = async () => {
        const myGen = generation
        if (!active) return
        const startedAt = Date.now()
        try {
          const payload = await deviceApi.getLatest(deviceId)
          // 以 channels 长度判断是否有有效数据，比 timestamp > 0 更可靠：
          // 模拟设备第一帧的 timestamp 可能恰好为 0（极低概率但边界存在）。
          if (active && myGen === generation && payload.channels.length > 0) {
            deviceApi._notifyListeners(payload)
          }
        } catch (error) {
          // 404 表示设备已断开/异常退出：通知 deviceStore 更新 UI 状态为 Error，
          // 并停止当前设备的轮询（设备已不在 map 中，继续轮询浪费资源）。
          // 其它错误（网络抖动等）仍按原逻辑静默吞掉，保留轮询容错。
          if (error instanceof ApiError && error.status === 404) {
            active = false
            generation++
            clearTimer()
            deviceApi._notifyDeviceLost(deviceId)
            return
          }
          console.log(`Wails polling for ${deviceId}:`, error)
        } finally {
          // 仅当仍属于当前 generation 时才调度下一轮，防止旧轮询 goroutine 重叠
          if (!active || myGen !== generation) return
          const intervalMs = publishRateToPollIntervalMs(deviceApi._publishRateHz)
          const elapsedMs = Date.now() - startedAt
          timer = window.setTimeout(() => { void pollLatest() }, Math.max(0, intervalMs - elapsedMs))
        }
      }
      const restart = () => {
        // 提升 generation 使旧轮询 goroutine 失效，
        // 清理旧 timer 后启动新一轮轮询
        generation++
        clearTimer()
        void pollLatest()
      }
      restart()
      deviceApi._registerSubscription(deviceId, {
        restart,
        unsubscribe: () => {
          active = false
          // 提升 generation 防止 finally 块重新调度
          generation++
          clearTimer()
          void wailsApi.device.subscribeStream(deviceId, false)
        },
      })
      return
    }

    // 提前声明 subscription：onError 在 sse-client 内部异步触发（fetch 完成后才调），
    // 当前不会触发 TDZ，但显式 let 提前声明可消除"调用时是否已赋值"的隐性时序耦合。
    let subscription: SseSubscription
    subscription = subscribeDaqStream(
      deviceId,
      deviceApi._notifyListeners,
      (error) => {
        // SSE 拿到 404 表示设备已断开/异常退出：通知 deviceStore 更新 UI 状态为 Error。
        // 通知后由 deviceStore 通过 unsubscribeFromDevice 统一清理订阅
        // （_registerSubscription 注册的 unsubscribe wrapper 会调 subscription.unsubscribe()），
        // 这里不重复调用，避免职责重叠。
        // 严格相等匹配 sse-client.ts:28 的 `SSE HTTP ${status}` 错误格式，
        // 避免 'redirect HTTP 404 ...' 等子串误匹配。
        // 其它错误（连接丢失/解析错误等）仍按原逻辑静默重连，保留容错。
        if (error === 'SSE HTTP 404') {
          deviceApi._notifyDeviceLost(deviceId)
          return
        }
        console.log(`SSE for ${deviceId}:`, error)
      },
    )

    deviceApi._registerSubscription(deviceId, { unsubscribe: () => subscription.unsubscribe() })
  },

  unsubscribeFromDevice: (deviceId: string, owner = 'default'): void => {
    const owners = deviceApi._subscriptionOwners.get(deviceId)
    owners?.delete(owner)
    if (owners && owners.size > 0) return
    deviceApi._subscriptionOwners.delete(deviceId)
    deviceApi.unsubscribeAllFromDevice(deviceId)
  },

  unsubscribeAllFromDevice: (deviceId: string): void => {
    deviceApi._subscriptionOwners.delete(deviceId)
    const subscription = deviceApi._subscriptions.get(deviceId)
    if (subscription) {
      subscription.unsubscribe()
      deviceApi._subscriptions.delete(deviceId)
    }
  },
}

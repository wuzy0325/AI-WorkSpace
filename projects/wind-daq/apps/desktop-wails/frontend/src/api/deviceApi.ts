import { request } from '@api/http-client'
import type { DeviceProfile, DeviceStatus, DataPayload, ScanResult, DSA3217ScanConfig } from '@api/types'
import { subscribeDaqStream } from '@api/sse-client'
import { isWailsAvailable, wailsApi } from '@api/wails-adapter'
export { motionApi } from './motionApi'
export { calibrationApi } from './calibrationApi'
export { traversalApi, storageApi, reportApi } from './otherApis'
export type { TraversalPoint, MotionAxisStatus, MotionStatus } from './otherApis'

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
    channels: Array.from({ length: 4 }, (_, index) => ({
      index,
      name: `CH${index + 1}`,
      enabled: true,
      unit: 'V',
      precision: 3,
      rangeMin: -10,
      rangeMax: 10,
    })),
  }
}

type SnapshotCallback = (payload: DataPayload) => void
type StatusCallback = (status: DeviceStatus[]) => void

type WailsResponse = { Success?: boolean; Error?: string; success?: boolean; error?: string }

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
      return (await wailsApi.device.getProfiles()) as DeviceProfile[]
    }
    return request<DeviceProfile[]>('/api/device/profiles')
  },

  upsertProfile: async (profile: DeviceProfile): Promise<{ success: boolean }> => {
    if (isWailsAvailable()) {
      return wailsOk(await wailsApi.device.upsertProfile(profile as any))
    }
    return request<{ success: boolean }>('/api/device/profiles', { method: 'PUT', body: JSON.stringify(profile) })
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
      if (result === false || result === true) {
        throw new Error('设备状态不可用')
      }
      return result as DeviceStatus
    }
    return request<DeviceStatus>(`/api/device/${id}/status`)
  },

  getLatest: async (id: string): Promise<DataPayload> => {
    if (isWailsAvailable()) {
      const result = await wailsApi.device.getLatestData(id)
      if (result === false || result === true) {
        return { deviceId: id, timestamp: 0, channels: [], channelIndices: [] }
      }
      return result as DataPayload
    }
    return request<DataPayload>(`/api/daq/latest/${id}`)
  },

  getPublishRate: async (): Promise<number> => {
    if (isWailsAvailable()) {
      return await wailsApi.device.getPublishRate()
    }
    return request<{ hz: number }>('/api/daq/publishRate').then((result) => result.hz)
  },

  setPublishRate: async (hz: number): Promise<{ success: boolean }> => {
    if (isWailsAvailable()) {
      return wailsOk(await wailsApi.device.setPublishRate(hz))
    }
    return request<{ success: boolean }>('/api/daq/publishRate', { method: 'PUT', body: JSON.stringify({ hz }) })
  },

  getProfilesList: async (): Promise<DeviceProfile[]> => {
    if (isWailsAvailable()) {
      return (await wailsApi.device.getProfiles()) as DeviceProfile[]
    }
    return request<DeviceProfile[]>('/api/device/profiles')
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

  _snapshotListeners: new Set<SnapshotCallback>(),
  _statusListeners: new Set<StatusCallback>(),
  _subscriptions: new Map<string, ReturnType<typeof subscribeDaqStream>>(),
  _wailsPayloadUnsubscribe: null as (() => void) | null,

  onSnapshot: (cb: SnapshotCallback): (() => void) => {
    deviceApi._snapshotListeners.add(cb)
    return () => { deviceApi._snapshotListeners.delete(cb) }
  },

  onStatusUpdated: (cb: StatusCallback): (() => void) => {
    deviceApi._statusListeners.add(cb)
    return () => { deviceApi._statusListeners.delete(cb) }
  },

  subscribeToDevice: (deviceId: string): void => {
    if (deviceApi._subscriptions.has(deviceId)) return

    if (isWailsAvailable()) {
      // Wails 桌面模式：使用 Wails Events 机制替代 HTTP SSE
      void wailsApi.device.subscribeStream(deviceId, true)
      if (!deviceApi._wailsPayloadUnsubscribe) {
        deviceApi._wailsPayloadUnsubscribe = wailsApi.device.onPayload((payload) => {
          deviceApi._snapshotListeners.forEach((cb) => cb(payload))
        })
      }
      deviceApi._subscriptions.set(deviceId, {
        unsubscribe: () => {
          void wailsApi.device.subscribeStream(deviceId, false)
        },
      })
      return
    }

    // Web 模式：使用 HTTP SSE
    const subscription = subscribeDaqStream(
      deviceId,
      (payload) => {
        deviceApi._snapshotListeners.forEach((cb) => cb(payload))
      },
      (error) => {
        console.log(`SSE for ${deviceId}:`, error)
      }
    )

    deviceApi._subscriptions.set(deviceId, subscription)
  },

  unsubscribeFromDevice: (deviceId: string): void => {
    const subscription = deviceApi._subscriptions.get(deviceId)
    if (subscription) {
      subscription.unsubscribe()
      deviceApi._subscriptions.delete(deviceId)
      if (isWailsAvailable() && deviceApi._subscriptions.size === 0 && deviceApi._wailsPayloadUnsubscribe) {
        deviceApi._wailsPayloadUnsubscribe()
        deviceApi._wailsPayloadUnsubscribe = null
      }
    }
  },
}

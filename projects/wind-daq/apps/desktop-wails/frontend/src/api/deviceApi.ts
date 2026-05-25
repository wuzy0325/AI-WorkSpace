import { request } from '@api/http-client'
import type { DeviceProfile, DeviceStatus, DataPayload, ScanResult } from '@api/types'
import { subscribeDaqStream } from '@api/sse-client'
import { isWailsAvailable, wailsApi } from '@api/wails-adapter'
export { motionApi } from './motionApi'
export { calibrationApi } from './calibrationApi'
export { traversalApi, storageApi, reportApi } from './otherApis'
export type { TraversalPoint, MotionAxisStatus, MotionStatus } from './otherApis'

const useWails = isWailsAvailable

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

function wailsOk(res: { Success: boolean; Error?: string }): { success: boolean } {
  if (!res.Success && res.Error) {
    throw new Error(res.Error)
  }
  return { success: res.Success }
}

export const deviceApi = {
  scanDevices: async (): Promise<ScanResult[]> => {
    if (useWails()) {
      const results = await wailsApi.device.scanDevices()
      return (results ?? []) as ScanResult[]
    }
    return request<ScanResult[]>('/api/device/scan')
  },

  getProfiles: async (): Promise<DeviceProfile[]> => {
    if (useWails()) {
      return (await wailsApi.device.getProfiles()) as DeviceProfile[]
    }
    return request<DeviceProfile[]>('/api/device/profiles')
  },

  upsertProfile: async (profile: DeviceProfile): Promise<{ success: boolean }> => {
    if (useWails()) {
      return wailsOk(await wailsApi.device.upsertProfile(profile as any))
    }
    return request<{ success: boolean }>('/api/device/profiles', { method: 'PUT', body: JSON.stringify(profile) })
  },

  connect: async (id: string): Promise<{ success: boolean }> => {
    if (useWails()) {
      return wailsOk(await wailsApi.device.connect(id))
    }
    return request<{ success: boolean }>(`/api/device/${id}/connect`, { method: 'POST' })
  },

  disconnect: async (id: string): Promise<{ success: boolean }> => {
    if (useWails()) {
      return wailsOk(await wailsApi.device.disconnect(id))
    }
    return request<{ success: boolean }>(`/api/device/${id}/disconnect`, { method: 'POST' })
  },

  startAcquisition: async (id: string): Promise<{ success: boolean }> => {
    if (useWails()) {
      return wailsOk(await wailsApi.device.startAcquisition(id))
    }
    return request<{ success: boolean }>(`/api/device/${id}/startAcquisition`, { method: 'POST' })
  },

  stopAcquisition: async (id: string): Promise<{ success: boolean }> => {
    if (useWails()) {
      return wailsOk(await wailsApi.device.stopAcquisition(id))
    }
    return request<{ success: boolean }>(`/api/device/${id}/stopAcquisition`, { method: 'POST' })
  },

  getStatus: async (id: string): Promise<DeviceStatus> => {
    if (useWails()) {
      const result = await wailsApi.device.getStatus(id)
      if (result === false || result === true) {
        throw new Error('设备状态不可用')
      }
      return result as DeviceStatus
    }
    return request<DeviceStatus>(`/api/device/${id}/status`)
  },

  getLatest: async (id: string): Promise<DataPayload> => {
    if (useWails()) {
      const result = await wailsApi.device.getLatestData(id)
      if (result === false || result === true) {
        return { deviceId: id, timestamp: 0, channels: [], channelIndices: [] }
      }
      return result as DataPayload
    }
    return request<DataPayload>(`/api/daq/latest/${id}`)
  },

  getPublishRate: async (): Promise<number> => {
    if (useWails()) {
      return await wailsApi.device.getPublishRate()
    }
    return request<{ hz: number }>('/api/daq/publishRate').then((result) => result.hz)
  },

  setPublishRate: async (hz: number): Promise<{ success: boolean }> => {
    if (useWails()) {
      return wailsOk(await wailsApi.device.setPublishRate(hz))
    }
    return request<{ success: boolean }>('/api/daq/publishRate', { method: 'PUT', body: JSON.stringify({ hz }) })
  },

  getProfilesList: async (): Promise<DeviceProfile[]> => {
    if (useWails()) {
      return (await wailsApi.device.getProfiles()) as DeviceProfile[]
    }
    return request<DeviceProfile[]>('/api/device/profiles')
  },

  _snapshotListeners: new Set<SnapshotCallback>(),
  _statusListeners: new Set<StatusCallback>(),
  _subscriptions: new Map<string, ReturnType<typeof subscribeDaqStream>>(),

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

    if (useWails()) {
      // Wails 桌面模式：使用 Wails Events 机制替代 HTTP SSE
      void wailsApi.device.subscribeStream(deviceId, true)
      const unsubscribe = wailsApi.device.onPayload((payload) => {
        deviceApi._snapshotListeners.forEach((cb) => cb(payload))
      })
      deviceApi._subscriptions.set(deviceId, {
        unsubscribe: () => {
          unsubscribe()
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
    }
  },
}

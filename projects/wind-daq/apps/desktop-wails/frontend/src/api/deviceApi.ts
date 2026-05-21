import { request } from '@api/http-client'
import type { DeviceProfile, DeviceStatus, DataPayload } from '@api/types'

export function defaultSimulatedProfile(): DeviceProfile {
  return {
    id: 'sim-1',
    name: 'Simulator 1',
    type: 'SIMULATED',
    samplingRate: 20,
    channels: Array.from({ length: 4 }, (_, index) => ({
      index,
      name: `CH${index + 1}`,
      enabled: true,
      unit: 'V',
      precision: 3,
    })),
  }
}

export const deviceApi = {
  getProfiles: () => request<DeviceProfile[]>('/api/device/profiles'),
  upsertProfile: (profile: DeviceProfile) =>
    request<{ success: boolean }>('/api/device/profiles', { method: 'PUT', body: JSON.stringify(profile) }),
  connect: (id: string) =>
    request<{ success: boolean }>(`/api/device/${id}/connect`, { method: 'POST' }),
  disconnect: (id: string) =>
    request<{ success: boolean }>(`/api/device/${id}/disconnect`, { method: 'POST' }),
  startAcquisition: (id: string) =>
    request<{ success: boolean }>(`/api/device/${id}/startAcquisition`, { method: 'POST' }),
  stopAcquisition: (id: string) =>
    request<{ success: boolean }>(`/api/device/${id}/stopAcquisition`, { method: 'POST' }),
  getStatus: (id: string) => request<DeviceStatus>(`/api/device/${id}/status`),
  getLatest: (id: string) => request<DataPayload>(`/api/daq/latest/${id}`),
  getProfilesList: () => request<DeviceProfile[]>('/api/device/profiles'),
}

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

export const motionApi = {
  connect: () => request<{ success: boolean }>('/api/motion/connect', { method: 'POST' }),
  disconnect: () => request<{ success: boolean }>('/api/motion/disconnect', { method: 'POST' }),
  status: () => request<MotionStatus>('/api/motion/status'),
  moveTo: (axis: string, position: number) =>
    request<{ success: boolean }>('/api/motion/moveTo', { method: 'POST', body: JSON.stringify({ axis, position }) }),
  moveBy: (axis: string, delta: number) =>
    request<{ success: boolean }>('/api/motion/moveBy', { method: 'POST', body: JSON.stringify({ axis, delta }) }),
  jog: (axis: string, velocity: number) =>
    request<{ success: boolean }>('/api/motion/jog', { method: 'POST', body: JSON.stringify({ axis, velocity }) }),
  home: () => request<{ success: boolean }>('/api/motion/home', { method: 'POST' }),
  stop: () => request<{ success: boolean }>('/api/motion/stop', { method: 'POST' }),
  emergencyStop: () => request<{ success: boolean }>('/api/motion/emergencyStop', { method: 'POST' }),
}

export interface CalibrationPointResult {
  pointIndex: number
  targetPressure: number
  timestamp: number
  values: Record<number, number>
}

export interface CalibrationStatus {
  taskId: string
  state: string
  currentPoint: number
  totalPoints: number
  lastError?: string
  results?: CalibrationPointResult[]
}

export interface CalibrationConfig {
  taskId: string
  deviceId: string
  type: string
  channels: number[]
  pressurePoints: number[]
  averageSamples: number
}

export const calibrationApi = {
  start: (cfg: CalibrationConfig) =>
    request<{ success: boolean }>('/api/calibration/start', { method: 'POST', body: JSON.stringify(cfg) }),
  status: () => request<CalibrationStatus>('/api/calibration/status'),
  collect: () => request<{ success: boolean }>('/api/calibration/collect', { method: 'POST' }),
  pause: () => request<{ success: boolean }>('/api/calibration/pause', { method: 'POST' }),
  resume: () => request<{ success: boolean }>('/api/calibration/resume', { method: 'POST' }),
  stop: () => request<{ success: boolean }>('/api/calibration/stop', { method: 'POST' }),
  getResult: (taskId: string) => request<CalibrationStatus>(`/api/calibration/result?taskId=${taskId}`),
}

export interface TraversalPoint {
  x: number
  y: number
  z: number
}

export const traversalApi = {
  start: (taskId: string, deviceId: string, channels: number[], path: TraversalPoint[]) =>
    request<{ success: boolean }>('/api/traversal/start', {
      method: 'POST', body: JSON.stringify({ taskId, deviceId, channels, path }),
    }),
  status: () => request<{ state: string; currentPoint: number; totalPoints: number }>('/api/traversal/status'),
  runPoint: () => request<{ success: boolean }>('/api/traversal/runPoint', { method: 'POST' }),
  pause: () => request<{ success: boolean }>('/api/traversal/pause', { method: 'POST' }),
  resume: () => request<{ success: boolean }>('/api/traversal/resume', { method: 'POST' }),
  stop: () => request<{ success: boolean }>('/api/traversal/stop', { method: 'POST' }),
}

export const storageApi = {
  status: () => request<{ recording: boolean; outputDir?: string }>('/api/storage/status'),
  start: (outputDir: string, filePrefix: string) =>
    request<{ success: boolean }>('/api/storage/start', {
      method: 'POST', body: JSON.stringify({ outputDir, filePrefix }),
    }),
  stop: () => request<{ success: boolean }>('/api/storage/stop', { method: 'POST' }),
}

export const reportApi = {
  generate: (outputDir: string, filePrefix: string, deviceId: string) =>
    request<{ path: string; size: number; records: number }>('/api/report/generate', {
      method: 'POST', body: JSON.stringify({ outputDir, filePrefix, deviceId }),
    }),
  status: () => request<{ generating: boolean }>('/api/report/status'),
}

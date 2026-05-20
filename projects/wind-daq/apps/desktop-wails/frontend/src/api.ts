export interface ChannelConfig {
  index: number
  name: string
  enabled: boolean
  unit: string
  precision: number
}

export interface DeviceProfile {
  id: string
  name: string
  type: 'SIMULATED'
  samplingRate: number
  channels: ChannelConfig[]
}

export interface DeviceStatus {
  id: string
  name: string
  type: string
  connection: string
  acquiring: boolean
  lastError?: string
}

export interface DataPayload {
  deviceId: string
  timestamp: number
  channels: number[]
  channelIndices: number[]
}

const apiBase = import.meta.env.VITE_API_BASE ?? 'http://localhost:8080'

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${apiBase}${path}`, {
    headers: { 'Content-Type': 'application/json', ...(init?.headers ?? {}) },
    ...init,
  })
  if (!response.ok) {
    const text = await response.text()
    throw new Error(text || `HTTP ${response.status}`)
  }
  return response.json() as Promise<T>
}

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

export const api = {
  getProfiles: () => request<DeviceProfile[]>('/api/device/profiles'),
  upsertProfile: (profile: DeviceProfile) =>
    request<{ success: boolean }>('/api/device/profiles', {
      method: 'PUT',
      body: JSON.stringify(profile),
    }),
  connect: (id: string) => request<{ success: boolean }>(`/api/device/${id}/connect`, { method: 'POST' }),
  startAcquisition: (id: string) =>
    request<{ success: boolean }>(`/api/device/${id}/startAcquisition`, { method: 'POST' }),
  stopAcquisition: (id: string) =>
    request<{ success: boolean }>(`/api/device/${id}/stopAcquisition`, { method: 'POST' }),
  getStatus: (id: string) => request<DeviceStatus>(`/api/device/${id}/status`),
  getLatest: (id: string) => request<DataPayload>(`/api/daq/latest/${id}`),
}

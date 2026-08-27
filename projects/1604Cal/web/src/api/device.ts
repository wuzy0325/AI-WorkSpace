import type { DeviceDTO, UnitConsistencyDTO, DeviceConnectConfigDTO } from '@/types/device'
import { apiGet, apiPost, apiDelete } from './client'

export async function fetchDevices(): Promise<DeviceDTO[]> {
  return (await apiGet<DeviceDTO[]>('/devices')) ?? []
}

export async function upsertDevice(payload: DeviceDTO): Promise<DeviceDTO> {
  return apiPost<DeviceDTO>('/devices', payload)
}

export async function connectDevice(id: string): Promise<DeviceDTO> {
  return apiPost<DeviceDTO>('/devices/connect', { id })
}

export async function disconnectDevice(id: string): Promise<DeviceDTO> {
  return apiPost<DeviceDTO>('/devices/disconnect', { id })
}

export async function deleteDevice(id: string): Promise<{ id: string }> {
  return apiDelete<{ id: string }>(`/devices/${encodeURIComponent(id)}`)
}

export async function setDeviceStatus(
  id: string,
  status: DeviceDTO['status']
): Promise<{ id: string; status: DeviceDTO['status'] }> {
  return apiPost<{ id: string; status: DeviceDTO['status'] }>('/devices/status', { id, status })
}

export async function fetchUnitConsistency(): Promise<UnitConsistencyDTO> {
  return apiGet<UnitConsistencyDTO>('/checks/unit-consistency')
}

export async function fetchDeviceConnectConfig(): Promise<DeviceConnectConfigDTO> {
  return apiGet<DeviceConnectConfigDTO>('/config/device-connect')
}

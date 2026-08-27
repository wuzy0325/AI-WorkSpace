import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

import { useMeasurementDeviceStore } from '../deviceStore'
import * as deviceApi from '@/api/device'
import * as multipressApi from '@/api/multipress'

// ── Mock API 层 ──

vi.mock('@/api/device', () => ({
  fetchDevices: vi.fn(),
  connectDevice: vi.fn(),
  disconnectDevice: vi.fn(),
  upsertDevice: vi.fn()
}))

vi.mock('@/api/multipress', () => ({
  multipressRegister: vi.fn(),
  multipressUnregister: vi.fn(),
  multipressListDevices: vi.fn(),
  multipressReadPressure: vi.fn()
}))

function measureDto(status: 'connected' | 'disconnected' | 'connecting' | 'error') {
  return { id: 'm1', name: '计量设备', type: 'measure', model: 'P1603', host: '192.168.1.10', port: 9000, unit: 'kPa', status }
}

function pressureDto(status: 'connected' | 'disconnected' | 'connecting' | 'error') {
  return { id: 'p1', name: '打压设备', type: 'pressure', model: 'SPC4000', host: '192.168.1.20', port: 9001, unit: 'kPa', status }
}

describe('useMeasurementDeviceStore.loadDevices', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })

  it('降级：后端报 disconnected 时，本地 connected 同步为 disconnected', async () => {
    const store = useMeasurementDeviceStore()

    // 先以 connected 状态加载，模拟业务模块已建立本地"已连接"状态。
    vi.mocked(deviceApi.fetchDevices).mockResolvedValueOnce([
      measureDto('connected'),
      pressureDto('connected')
    ] as never)
    await store.loadDevices()
    expect(store.measureDevices[0].status).toBe('connected')
    expect(store.pressureDevices[0].status).toBe('connected')

    // 设备管理模块断开后，后端状态变为 disconnected。
    vi.mocked(deviceApi.fetchDevices).mockResolvedValueOnce([
      measureDto('disconnected'),
      pressureDto('disconnected')
    ] as never)
    await store.loadDevices()

    // 本地状态必须跟随后端降级，避免跨模块残留"已连接"的过期状态。
    expect(store.measureDevices[0].status).toBe('disconnected')
    expect(store.pressureDevices[0].status).toBe('disconnected')
  })

  it('后端报 connecting 时覆盖本地状态', async () => {
    const store = useMeasurementDeviceStore()

    vi.mocked(deviceApi.fetchDevices).mockResolvedValueOnce([
      pressureDto('connected')
    ] as never)
    await store.loadDevices()
    expect(store.pressureDevices[0].status).toBe('connected')

    vi.mocked(deviceApi.fetchDevices).mockResolvedValueOnce([
      pressureDto('connecting')
    ] as never)
    await store.loadDevices()
    expect(store.pressureDevices[0].status).toBe('connecting')
  })

  it('后端报 connected 时本地未连接升级为 connected', async () => {
    const store = useMeasurementDeviceStore()

    vi.mocked(deviceApi.fetchDevices).mockResolvedValueOnce([
      measureDto('connected'),
      pressureDto('connected')
    ] as never)
    await store.loadDevices()
    expect(store.measureDevices[0].status).toBe('connected')
    expect(store.pressureDevices[0].status).toBe('connected')
  })
})
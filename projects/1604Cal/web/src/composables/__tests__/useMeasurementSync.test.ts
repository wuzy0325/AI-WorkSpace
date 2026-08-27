import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { useMeasurementStore } from '@/stores/measurement'
import * as measurementApi from '@/api/measurement'
import { useMeasurementSync } from '../useMeasurementSync'

const subscribe = vi.fn(() => vi.fn())
const registerPoll = vi.fn(() => vi.fn())

vi.mock('@/composables/useEventHub', () => ({
  useEventHub: () => ({ subscribe, registerPoll })
}))

vi.mock('@/stores/device/inventoryStore', () => ({
  useDeviceInventoryStore: () => ({ updateDevicePressure: vi.fn() })
}))

vi.mock('@/api/multipress', () => ({
  multipressListDevices: vi.fn().mockResolvedValue([])
}))

vi.mock('@/api/session', () => ({
  readPressure: vi.fn(),
  readStability: vi.fn(),
  readMeasureData: vi.fn(),
  bindDevices: vi.fn(),
  bindMeasureDevice: vi.fn(),
  unbindMeasureDevices: vi.fn(),
  readMeasureDataAllDevices: vi.fn(),
  readValveStatus: vi.fn(),
  readValveStatusAll: vi.fn(),
  setValveStatus: vi.fn(),
  readMeasureUnit: vi.fn(),
  readMeasureUnitAll: vi.fn(),
  setMeasureUnit: vi.fn(),
  setMeasureUnitAll: vi.fn(),
  readSessionUnitConsistency: vi.fn(),
  readDeviceInfo: vi.fn(),
  resetDevice: vi.fn(),
  calibrateZero: vi.fn()
}))

vi.mock('@/api/measurement', () => ({
  fetchMeasurementState: vi.fn(),
  fetchMeasurementData: vi.fn(),
  fetchMeasurementPoints: vi.fn(),
  getMeasurementAlarmConfig: vi.fn(),
  checkMeasurementAlarmPending: vi.fn(),
  fetchStabilityTimeoutPending: vi.fn(),
  getMeasurementParamsConfig: vi.fn(),
  saveMeasurementParamsConfig: vi.fn(),
  startMeasurement: vi.fn(),
  pauseMeasurement: vi.fn(),
  stopMeasurement: vi.fn(),
  generateMeasurementPoints: vi.fn(),
  resolveMeasurementAlarm: vi.fn(),
  autoCollectMeasurement: vi.fn(),
  manualPressurizeMeasurement: vi.fn(),
  manualCollectMeasurement: vi.fn(),
  manualStartMeasurement: vi.fn(),
  saveMeasurementAlarmConfig: vi.fn(),
  resolveStabilityTimeout: vi.fn()
}))

function mountSync() {
  return mount({
    setup() {
      useMeasurementSync()
      return () => null
    }
  })
}

describe('useMeasurementSync', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    vi.mocked(measurementApi.getMeasurementAlarmConfig).mockResolvedValue({
      enabled: true,
      enabledChannels: [1],
      confirmOnAlarm: false,
      soundEnabled: true
    })
    vi.mocked(measurementApi.fetchMeasurementState).mockResolvedValue('collecting')
    vi.mocked(measurementApi.fetchMeasurementPoints).mockResolvedValue([])
    vi.mocked(measurementApi.fetchMeasurementData).mockResolvedValue({ rows: [], total: 0 })
    vi.mocked(measurementApi.checkMeasurementAlarmPending).mockResolvedValue({ pending: false, alarm: null })
    vi.mocked(measurementApi.fetchStabilityTimeoutPending).mockResolvedValue({ pending: false, pointIndex: 0 })
  })

  it('does not register subscriptions after async setup finishes after unmount', async () => {
    let resolveState!: (state: string) => void
    vi.mocked(measurementApi.fetchMeasurementState).mockImplementationOnce(
      () => new Promise(resolve => { resolveState = resolve })
    )

    const wrapper = mountSync()
    wrapper.unmount()
    resolveState('collecting')
    await Promise.resolve()
    await Promise.resolve()

    expect(subscribe).not.toHaveBeenCalled()
    expect(registerPoll).not.toHaveBeenCalled()
  })

  it('rehydrates the active workflow after mounting', async () => {
    const rows = [{ timestamp: '2026-08-25T10:00:00Z', channels: { '1': 1.2 } }]
    const points = [{ id: 'p1', index: 1, targetPressure: 1, direction: 'up', status: 'completed' }]
    const alarmDetail = {
      pointId: 'p1',
      deviceId: 'd1',
      targetPressure: 1,
      actualPressure: 2,
      threshold: 0.05,
      maxDeviation: 0.5,
      overLimitChannels: [2, 4]
    }
    vi.mocked(measurementApi.fetchMeasurementData).mockResolvedValue({ rows, total: 1 })
    vi.mocked(measurementApi.fetchMeasurementPoints).mockResolvedValue(points)
    vi.mocked(measurementApi.checkMeasurementAlarmPending)
      .mockResolvedValue({ pending: true, alarm: alarmDetail })

    const wrapper = mountSync()
    await vi.waitFor(() => expect(useMeasurementStore().state).toBe('collecting'))

    const store = useMeasurementStore()
    expect(store.state).toBe('collecting')
    expect(store.rows).toEqual(rows)
    expect(store.points).toEqual(points)
    expect(store.alarmPending).toBe(true)
    // 报警详情必须一并恢复：非确认模式自动放行与弹窗展示都依赖 alarmData
    expect(store.alarmData).toEqual(alarmDetail)
    expect(subscribe).toHaveBeenCalled()
    expect(registerPoll).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })

  it('restores pending stability timeout decision after refresh', async () => {
    vi.mocked(measurementApi.fetchStabilityTimeoutPending)
      .mockResolvedValue({ pending: true, pointIndex: 3 })

    const wrapper = mountSync()
    const store = useMeasurementStore()
    await vi.waitFor(() => expect(store.stabilityTimeoutPending).toBe(true))
    wrapper.unmount()
  })

  it('bounds rows appended by realtime events', async () => {
    const wrapper = mountSync()
    await vi.waitFor(() => expect(subscribe).toHaveBeenCalled())
    const subscriptionCalls = subscribe.mock.calls as unknown as Array<[
      string,
      (payload: { data: unknown }) => void
    ]>
    const dataSubscription = subscriptionCalls.find(([type]) => type === 'measurement.data_updated')
    expect(dataSubscription).toBeDefined()
    const handler = dataSubscription![1]

    for (let index = 0; index < 250; index++) {
      handler({ data: { timestamp: String(index), channels: { '1': index } } })
    }

    const store = useMeasurementStore()
    // 实时行按 500ms 攒批刷新，等待缓冲 flush 后再断言上限与窗口内容
    await vi.waitFor(() => expect(store.rows).toHaveLength(200))
    expect(store.rows[0].timestamp).toBe('50')
    expect(store.rows[199].timestamp).toBe('249')
    wrapper.unmount()
  })
})

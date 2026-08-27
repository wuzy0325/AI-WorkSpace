import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

import { useMeasurementStore } from '../index'
import * as sessionApi from '@/api/session'
import * as measurementApi from '@/api/measurement'
import * as deviceApi from '@/api/device'

// ── Mock API 层 ──

vi.mock('@/api/device', () => ({
  fetchDevices: vi.fn().mockResolvedValue([]),
  connectDevice: vi.fn(),
  disconnectDevice: vi.fn(),
  upsertDevice: vi.fn()
}))

vi.mock('@/api/multipress', () => ({
  multipressRegister: vi.fn(),
  multipressUnregister: vi.fn(),
  multipressListDevices: vi.fn().mockResolvedValue([]),
  multipressReadPressure: vi.fn()
}))

vi.mock('@/api/session', () => ({
  bindDevices: vi.fn(),
  bindMeasureDevice: vi.fn(),
  unbindMeasureDevices: vi.fn(),
  readPressure: vi.fn(),
  readStability: vi.fn(),
  readMeasureData: vi.fn(),
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
  resetDevice: vi.fn()
}))

vi.mock('@/api/measurement', () => ({
  fetchMeasurementState: vi.fn(),
  startMeasurement: vi.fn(),
  pauseMeasurement: vi.fn(),
  stopMeasurement: vi.fn(),
  fetchMeasurementData: vi.fn(),
  generateMeasurementPoints: vi.fn(),
  saveMeasurementParamsConfig: vi.fn()
}))

vi.mock('element-plus', async () => {
  const actual = await vi.importActual<typeof import('element-plus')>('element-plus')
  return {
    ...actual,
    ElMessage: { success: vi.fn(), warning: vi.fn(), error: vi.fn(), info: vi.fn() }
  }
})

describe('useMeasurementStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })

  // ── 初始状态 ──

  describe('initial state', () => {
    it('starts idle with empty data', () => {
      const store = useMeasurementStore()
      expect(store.state).toBe('idle')
      expect(store.rows).toEqual([])
      expect(store.channels).toHaveLength(16)
      expect(store.measureDeviceId).toBe('')
      expect(store.pressureDeviceId).toBe('')
      expect(store.currentPressure).toBe(0)
      expect(store.isStable).toBe(false)
    })

    it('computes isCollecting/isPaused/isIdle correctly', () => {
      const store = useMeasurementStore()
      expect(store.isIdle).toBe(true)
      expect(store.isRunning).toBe(false)
      expect(store.isCollecting).toBe(false)
      expect(store.isPaused).toBe(false)
      expect(store.deviceBound).toBe(false)
    })
  })

  describe('isRunning', () => {
    it('is true for pressurizing/stabilizing/collecting', () => {
      const store = useMeasurementStore()

      store.syncState('pressurizing')
      expect(store.isRunning).toBe(true)

      store.syncState('stabilizing')
      expect(store.isRunning).toBe(true)

      store.syncState('collecting')
      expect(store.isRunning).toBe(true)
    })

    it('is false for idle/paused/completed/error', () => {
      const store = useMeasurementStore()

      store.syncState('idle')
      expect(store.isRunning).toBe(false)

      store.syncState('paused')
      expect(store.isRunning).toBe(false)

      store.syncState('completed')
      expect(store.isRunning).toBe(false)

      store.syncState('error')
      expect(store.isRunning).toBe(false)
    })
  })

  // ── 设备绑定 ──

  describe('bindDevices', () => {
    it('keeps a legacy single device ID intact', async () => {
      vi.mocked(sessionApi.bindDevices).mockResolvedValue(undefined)
      const store = useMeasurementStore()

      await store.bindDevices('dev-1604-01', 'p1')

      expect(sessionApi.bindDevices).toHaveBeenCalledWith(['dev-1604-01'], 'p1', 'measurement')
      expect(store.measureDeviceIds).toEqual(['dev-1604-01'])
      expect(store.measureDeviceId).toBe('dev-1604-01')
    })

    it('calls API and stores both device IDs', async () => {
      vi.mocked(sessionApi.bindDevices).mockResolvedValue(undefined)
      const store = useMeasurementStore()
      await store.bindDevices(['m1'], 'p1')
      expect(sessionApi.bindDevices).toHaveBeenCalledWith(['m1'], 'p1', 'measurement')
      expect(store.measureDeviceId).toBe('m1')
      expect(store.measureDeviceIds).toEqual(['m1'])
      expect(store.pressureDeviceId).toBe('p1')
      expect(store.deviceBound).toBe(true)
    })

    it('supports multiple measure devices', async () => {
      vi.mocked(sessionApi.bindDevices).mockResolvedValue(undefined)
      const store = useMeasurementStore()
      await store.bindDevices(['m1', 'm2'], 'p1')
      expect(sessionApi.bindDevices).toHaveBeenCalledWith(['m1', 'm2'], 'p1', 'measurement')
      expect(store.measureDeviceIds).toEqual(['m1', 'm2'])
      expect(store.measureDeviceId).toBe('m1')
      expect(store.deviceBound).toBe(true)
    })

    it('removes blank and duplicate device IDs', async () => {
      vi.mocked(sessionApi.bindDevices).mockResolvedValue(undefined)
      const store = useMeasurementStore()

      await store.bindDevices([' m1 ', '', 'm1', 'm2'], 'p1')

      expect(sessionApi.bindDevices).toHaveBeenCalledWith(['m1', 'm2'], 'p1', 'measurement')
      expect(store.measureDeviceIds).toEqual(['m1', 'm2'])
    })
  })

  describe('bindMeasureDevice', () => {
    it('calls API and stores measure device ID', async () => {
      vi.mocked(sessionApi.bindMeasureDevice).mockResolvedValue(undefined)
      const store = useMeasurementStore()
      await store.bindMeasureDevice(['m2'])
      expect(sessionApi.bindMeasureDevice).toHaveBeenCalledWith(['m2'], 'measurement')
      expect(store.measureDeviceId).toBe('m2')
      expect(store.deviceBound).toBe(true)
    })
  })

  describe('unbind device ids', () => {
    it('clears pressure device id when unbinding pressure device', async () => {
      vi.mocked(sessionApi.bindDevices).mockResolvedValue(undefined)
      const store = useMeasurementStore()

      await store.bindDevices(['m1'], 'p1')
      expect(store.pressureDeviceId).toBe('p1')

      store.unbindPressureDevice()
      expect(store.pressureDeviceId).toBe('')
      expect(store.measureDeviceId).toBe('m1')
      expect(store.deviceBound).toBe(true)
    })

    it('clears both ids when resetting binding state locally', async () => {
      vi.mocked(sessionApi.bindDevices).mockResolvedValue(undefined)
      const store = useMeasurementStore()

      await store.bindDevices(['m1'], 'p1')
      expect(store.deviceBound).toBe(true)

      store.resetBindingState()
      expect(store.measureDeviceId).toBe('')
      expect(store.measureDeviceIds).toEqual([])
      expect(store.pressureDeviceId).toBe('')
      expect(store.deviceBound).toBe(false)
    })
  })

  // ── 实时数据刷新 ──

  describe('refreshPressure', () => {
    it('updates currentPressure on success', async () => {
      vi.mocked(sessionApi.readPressure).mockResolvedValue(42.5)
      const store = useMeasurementStore()
      await store.refreshPressure()
      expect(store.currentPressure).toBe(42.5)
    })

    it('silently ignores errors', async () => {
      vi.mocked(sessionApi.readPressure).mockRejectedValue(new Error('no device'))
      const store = useMeasurementStore()
      await expect(store.refreshPressure()).resolves.toBeUndefined()
      expect(store.currentPressure).toBe(0)
    })
  })

  describe('refreshStability', () => {
    it('updates isStable', async () => {
      vi.mocked(sessionApi.readStability).mockResolvedValue(true)
      const store = useMeasurementStore()
      await store.refreshStability()
      expect(store.isStable).toBe(true)
    })
  })

  describe('refreshMeasureData', () => {
    it('updates channelData from first device and channelDataByDevice from all devices', async () => {
      vi.mocked(sessionApi.readMeasureDataAllDevices).mockResolvedValue({
        m1: [1.1, 2.2, 3.3],
        m2: [4.4, 5.5, 6.6]
      })
      const store = useMeasurementStore()
      await store.bindMeasureDevice(['m1', 'm2'])
      await store.refreshMeasureData()
      expect(store.channelData).toEqual([1.1, 2.2, 3.3])
      expect(store.channelDataByDevice).toEqual({
        m1: [1.1, 2.2, 3.3],
        m2: [4.4, 5.5, 6.6]
      })
    })
  })

  describe('refreshValveStatus', () => {
    it('updates valveStatus', async () => {
      vi.mocked(sessionApi.readValveStatus).mockResolvedValue('measurement')
      const store = useMeasurementStore()
      await store.refreshValveStatus()
      expect(store.valveStatus).toBe('measurement')
    })
  })

  describe('setValveStatus', () => {
    it('calls API and updates local state', async () => {
      vi.mocked(sessionApi.setValveStatus).mockResolvedValue(undefined)
      const store = useMeasurementStore()
      await store.setValveStatus('calibration')
      expect(sessionApi.setValveStatus).toHaveBeenCalledWith('calibration')
      expect(store.valveStatus).toBe('calibration')
    })
  })

  describe('refreshMeasureUnit', () => {
    it('updates measureUnit', async () => {
      vi.mocked(sessionApi.readMeasureUnit).mockResolvedValue('MPa')
      const store = useMeasurementStore()
      await store.refreshMeasureUnit()
      expect(store.measureUnit).toBe('MPa')
    })
  })

  describe('refreshDeviceInfo', () => {
    it('updates deviceInfo', async () => {
      vi.mocked(sessionApi.readDeviceInfo).mockResolvedValue({ model: 'WTN1604' })
      const store = useMeasurementStore()
      await store.refreshDeviceInfo()
      expect(store.deviceInfo).toEqual({ model: 'WTN1604' })
    })
  })

  // ── 采集工作流 ──

  describe('start', () => {
    it('fails with warning when no device bound', async () => {
      const store = useMeasurementStore()
      const result = await store.start([1, 2])
      expect(measurementApi.startMeasurement).not.toHaveBeenCalled()
      expect(result).toEqual({ ok: false, error: 'DEVICE_NOT_BOUND', detail: '请先绑定计量设备' })
    })

    it('calls API, updates state, clears rows on success', async () => {
      vi.mocked(sessionApi.bindMeasureDevice).mockResolvedValue(undefined)
      vi.mocked(measurementApi.saveMeasurementParamsConfig).mockResolvedValue(undefined)
      vi.mocked(measurementApi.generateMeasurementPoints).mockResolvedValue([])
      vi.mocked(measurementApi.startMeasurement).mockResolvedValue('collecting')
      const store = useMeasurementStore()
      await store.bindMeasureDevice(['m1'])
      // 阀门=校准模式是启动的必要条件，先把状态置为 calibration。
      store.valveStatus = 'calibration'
      store.rows = [{ timestamp: 'old', channels: { '1': 0 } }]

      const result = await store.start([1, 2, 3])

      expect(measurementApi.startMeasurement).toHaveBeenCalledWith([1, 2, 3])
      expect(store.state).toBe('collecting')
      expect(store.channels).toEqual([1, 2, 3])
      expect(store.rows).toEqual([])
      expect(result).toEqual({ ok: true })
    })

    it('shows error on API failure', async () => {
      vi.mocked(sessionApi.bindMeasureDevice).mockResolvedValue(undefined)
      vi.mocked(measurementApi.saveMeasurementParamsConfig).mockResolvedValue(undefined)
      vi.mocked(measurementApi.generateMeasurementPoints).mockResolvedValue([])
      vi.mocked(measurementApi.startMeasurement).mockRejectedValue(new Error('transition denied'))
      const store = useMeasurementStore()
      await store.bindMeasureDevice(['m1'])
      store.valveStatus = 'calibration'

      const result = await store.start([1])

      expect(result).toEqual({ ok: false, error: 'START_FAILED', detail: 'transition denied' })
      expect(store.state).toBe('idle')
    })

    it('rejects start when valve is not in calibration mode', async () => {
      // 阀门门禁：valve != calibration 时 store 应直接拒绝，不调用 API。
      const store = useMeasurementStore()
      await store.bindMeasureDevice(['m1'])
      store.valveStatus = 'measurement'

      const result = await store.start([1])

      expect(measurementApi.startMeasurement).not.toHaveBeenCalled()
      expect(result).toEqual({ ok: false, error: 'VALVE_NOT_READY', detail: '请先将阀门切换到校准模式' })
    })
  })

  describe('pause', () => {
    it('updates state on success', async () => {
      vi.mocked(measurementApi.pauseMeasurement).mockResolvedValue('paused')
      const store = useMeasurementStore()
      await store.pause()
      expect(store.state).toBe('paused')
    })
  })

  describe('syncMeasureDevicesWithStatus', () => {
    it('剔除明确已断开的设备，保留仍在连接中的设备', async () => {
      vi.mocked(deviceApi.fetchDevices).mockResolvedValue([
        { id: 'm1', name: 'a', type: 'measure', model: 'P1603', host: 'h', port: 9000, unit: 'kPa', status: 'connected' },
        { id: 'm2', name: 'b', type: 'measure', model: 'P1603', host: 'h', port: 9000, unit: 'kPa', status: 'disconnected' }
      ] as never)
      const store = useMeasurementStore()
      await store.bindMeasureDevice(['m1', 'm2'])
      expect(store.measureDeviceIds).toEqual(['m1', 'm2'])

      await store.syncMeasureDevicesWithStatus()

      expect(store.measureDeviceIds).toEqual(['m1'])
    })

    it('清单拉取失败时保守保留，不误删设备', async () => {
      vi.mocked(deviceApi.fetchDevices).mockRejectedValue(new Error('network down'))
      const store = useMeasurementStore()
      await store.bindMeasureDevice(['m1', 'm2'])

      await store.syncMeasureDevicesWithStatus()

      expect(store.measureDeviceIds).toEqual(['m1', 'm2'])
    })
  })

  describe('stop', () => {
    it('updates state on success', async () => {
      vi.mocked(measurementApi.stopMeasurement).mockResolvedValue('idle')
      const store = useMeasurementStore()
      await store.stop()
      expect(store.state).toBe('idle')
    })
  })

  describe('refreshData', () => {
    it('updates rows from API response', async () => {
      const mockRows = [
        { timestamp: '2026-04-21T10:00:00Z', channels: { '1': 1.1, '2': 2.2 } }
      ]
      vi.mocked(measurementApi.fetchMeasurementData).mockResolvedValue({
        rows: mockRows,
        total: 1
      })
      const store = useMeasurementStore()
      await store.refreshData()
      expect(store.rows).toEqual(mockRows)
    })

    it('keeps only the latest browser row window', async () => {
      const mockRows = Array.from({ length: 250 }, (_, index) => ({
        timestamp: String(index),
        channels: { '1': index }
      }))
      vi.mocked(measurementApi.fetchMeasurementData).mockResolvedValue({
        rows: mockRows,
        total: mockRows.length
      })
      const store = useMeasurementStore()

      await store.refreshData()

      expect(store.rows).toHaveLength(200)
      expect(store.rows[0].timestamp).toBe('50')
      expect(store.rows[199].timestamp).toBe('249')
    })
  })

  describe('aggregate device state', () => {
    it('stores per-device valve and unit results and keeps first-device compatibility values', async () => {
      vi.mocked(sessionApi.readValveStatusAll).mockResolvedValue({
        m1: { value: 'calibration' },
        m2: { value: 'measurement' }
      })
      vi.mocked(sessionApi.readMeasureUnitAll).mockResolvedValue({
        m1: { value: 'kPa' },
        m2: { value: 'MPa' }
      })
      const store = useMeasurementStore()
      await store.bindMeasureDevice(['m1', 'm2'])

      await store.refreshDeviceSettingsAll()

      expect(store.valveStatusByDevice).toEqual({ m1: 'calibration', m2: 'measurement' })
      expect(store.measureUnitByDevice).toEqual({ m1: 'kPa', m2: 'MPa' })
      expect(store.valveStatus).toBe('calibration')
      expect(store.measureUnit).toBe('kPa')
      expect(store.activeDeviceId).toBe('m1')
    })

    it('repairs focused device when bindings change', async () => {
      const store = useMeasurementStore()
      await store.bindMeasureDevice(['m1', 'm2'])
      store.setActiveDevice('m2')
      expect(store.activeDeviceId).toBe('m2')

      await store.bindMeasureDevice(['m1'])

      expect(store.activeDeviceId).toBe('m1')
    })
  })

  describe('fetchCurrentState', () => {
    it('syncs state from API', async () => {
      vi.mocked(measurementApi.fetchMeasurementState).mockResolvedValue('collecting')
      const store = useMeasurementStore()
      await store.fetchCurrentState()
      expect(store.state).toBe('collecting')
    })

    it('silently ignores errors', async () => {
      vi.mocked(measurementApi.fetchMeasurementState).mockRejectedValue(new Error('fail'))
      const store = useMeasurementStore()
      await expect(store.fetchCurrentState()).resolves.toBeUndefined()
      expect(store.state).toBe('idle')
    })
  })

  describe('syncState', () => {
    it('directly sets state', () => {
      const store = useMeasurementStore()
      store.syncState('paused')
      expect(store.state).toBe('paused')
      expect(store.isPaused).toBe(true)
    })
  })

  describe('totalRows', () => {
    it('counts rows', () => {
      const store = useMeasurementStore()
      expect(store.totalRows).toBe(0)
      store.rows = [
        { timestamp: 't1', channels: {} },
        { timestamp: 't2', channels: {} }
      ]
      expect(store.totalRows).toBe(2)
    })
  })
})

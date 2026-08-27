import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

import { useCalibrationStore } from '../index'
import { usePressurePointStore } from '../pressurePoints'
import { useDeviceControlStore } from '../deviceControl'
import * as sessionApi from '@/api/session'

vi.mock('@/api/session', async () => {
  const actual = await vi.importActual<typeof import('@/api/session')>('@/api/session')
  return {
    ...actual,
    readValveStatus: vi.fn(),
    readMeasureUnit: vi.fn(),
    readDeviceInfo: vi.fn()
  }
})

describe('calibration store refreshDeviceInfo', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.resetAllMocks()
  })

  it('returns true when valve and unit are loaded even if device info fails', async () => {
    vi.mocked(sessionApi.readValveStatus).mockResolvedValue('measurement')
    vi.mocked(sessionApi.readMeasureUnit).mockResolvedValue('kPa')
    vi.mocked(sessionApi.readDeviceInfo).mockRejectedValue(new Error('device info not supported'))

    const deviceControlStore = useDeviceControlStore()
    const loaded = await deviceControlStore.refreshDeviceInfo({ retries: 1 })

    expect(loaded).toBe(true)
    expect(deviceControlStore.valveStatus).toBe('measurement')
    expect(deviceControlStore.measureUnit).toBe('kPa')
  })

  it('returns false when valve or unit is still unavailable', async () => {
    vi.mocked(sessionApi.readValveStatus).mockRejectedValue(new Error('valve read failed'))
    vi.mocked(sessionApi.readMeasureUnit).mockResolvedValue('kPa')
    vi.mocked(sessionApi.readDeviceInfo).mockResolvedValue({ model: 'WTN1604' })

    const deviceControlStore = useDeviceControlStore()
    const loaded = await deviceControlStore.refreshDeviceInfo({ retries: 1 })

    expect(loaded).toBe(false)
  })

  it('returns true when valve and unit succeed on different retries', async () => {
    vi.mocked(sessionApi.readValveStatus)
      .mockResolvedValueOnce('measurement')
      .mockRejectedValueOnce(new Error('valve timeout'))
    vi.mocked(sessionApi.readMeasureUnit)
      .mockRejectedValueOnce(new Error('unit timeout'))
      .mockResolvedValueOnce('kPa')
    vi.mocked(sessionApi.readDeviceInfo).mockRejectedValue(new Error('device info not supported'))

    const deviceControlStore = useDeviceControlStore()
    const loaded = await deviceControlStore.refreshDeviceInfo({ retries: 2 })

    expect(loaded).toBe(true)
    expect(deviceControlStore.valveStatus).toBe('measurement')
    expect(deviceControlStore.measureUnit).toBe('kPa')
  })
})

describe('calibration store point_done actions', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.resetAllMocks()
    localStorage.clear()
  })

  it('shows 拟合 as primary and 重新开始 as secondary when all points are collected', () => {
    const pressurePointStore = usePressurePointStore()
    pressurePointStore.pressurePoints = [
      { id: 'p1', index: 1, targetPressure: 10, status: 'completed' },
      { id: 'p2', index: 2, targetPressure: 20, status: 'completed' }
    ]

    const calibrationStore = useCalibrationStore()
    calibrationStore.syncSessionState('point_done')

    expect(calibrationStore.primaryAction.key).toBe('fit')
    expect(calibrationStore.primaryAction.label).toBe('拟合')
    expect(calibrationStore.secondaryActions.map(a => a.key)).toEqual(['reset'])
  })

  it('keeps 暂停/停止 while collection is still in progress at point_done', () => {
    const pressurePointStore = usePressurePointStore()
    pressurePointStore.pressurePoints = [
      { id: 'p1', index: 1, targetPressure: 10, status: 'completed' },
      { id: 'p2', index: 2, targetPressure: 20, status: 'pending' }
    ]

    const calibrationStore = useCalibrationStore()
    calibrationStore.syncSessionState('point_done')

    expect(calibrationStore.primaryAction.key).toBe('pause')
    expect(calibrationStore.primaryAction.label).toBe('暂停')
    expect(calibrationStore.secondaryActions.map(a => a.key)).toEqual(['stop'])
  })

  it('shows 重新开始 as primary when all points were skipped (no data to fit)', () => {
    const pressurePointStore = usePressurePointStore()
    pressurePointStore.pressurePoints = [
      { id: 'p1', index: 1, targetPressure: 10, status: 'skipped' },
      { id: 'p2', index: 2, targetPressure: 20, status: 'skipped' }
    ]

    const calibrationStore = useCalibrationStore()
    calibrationStore.syncSessionState('point_done')

    expect(calibrationStore.primaryAction.key).toBe('reset')
    expect(calibrationStore.primaryAction.label).toBe('重新开始')
    expect(calibrationStore.secondaryActions).toEqual([])
  })
})

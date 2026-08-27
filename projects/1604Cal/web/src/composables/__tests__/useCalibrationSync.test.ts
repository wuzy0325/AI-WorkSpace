import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import * as deviceApi from '@/api/device'
import * as sessionApi from '@/api/session'
import { useCalibrationSync } from '../useCalibrationSync'

const subscribe = vi.fn(() => vi.fn())
const subscribeGlobal = vi.fn(() => vi.fn())
const registerPoll = vi.fn(() => vi.fn())

vi.mock('@/composables/useEventHub', () => ({
  useEventHub: () => ({ subscribe, subscribeGlobal, registerPoll })
}))

vi.mock('@/api/device', () => ({
  connectDevice: vi.fn().mockResolvedValue({ id: 'm1', status: 'connected' }),
  fetchDevices: vi.fn().mockResolvedValue([
    {
      id: 'm1',
      name: '计量1604',
      type: 'measure',
      model: 'WTN1604',
      host: '127.0.0.1',
      port: 9000,
      unit: 'kPa',
      status: 'connected'
    }
  ]),
  disconnectDevice: vi.fn(),
  upsertDevice: vi.fn(),
  fetchUnitConsistency: vi.fn().mockResolvedValue({ consistent: true, conflicts: [] })
}))

vi.mock('@/api/multipress', () => ({
  multipressListDevices: vi.fn().mockResolvedValue([])
}))

vi.mock('@/api/session', () => ({
  bindMeasureDevice: vi.fn().mockResolvedValue(undefined),
  bindDevices: vi.fn().mockResolvedValue(undefined),
  readPressure: vi.fn(),
  readStability: vi.fn().mockResolvedValue(true),
  readMeasureData: vi.fn().mockResolvedValue([]),
  readValveStatus: vi.fn().mockResolvedValue('calibration'),
  readValveStatusAll: vi.fn().mockResolvedValue({}),
  setValveStatus: vi.fn(),
  readMeasureUnit: vi.fn().mockResolvedValue('kPa'),
  readMeasureUnitAll: vi.fn().mockResolvedValue({}),
  setMeasureUnit: vi.fn(),
  setMeasureUnitAll: vi.fn(),
  readSessionUnitConsistency: vi.fn().mockResolvedValue({ consistent: true, conflicts: [] }),
  readDeviceInfo: vi.fn().mockResolvedValue({ model: 'WTN1604' }),
  resetDevice: vi.fn(),
  calibrateZero: vi.fn().mockResolvedValue([]),
  unbindMeasureDevices: vi.fn()
}))

// 标定 store 挂载时会拉取会话状态/压力点等，全部给安全默认值
vi.mock('@/api/calibration', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api/calibration')>()
  return {
    ...actual,
    fetchSessionState: vi.fn().mockResolvedValue({ state: 'idle' })
  }
})

function mountSync() {
  return mount({
    setup() {
      useCalibrationSync()
      return () => null
    }
  })
}

describe('useCalibrationSync 自动绑定', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    // fetchDevices 的默认实现返回已连接设备；单测里可按需覆盖
    vi.mocked(deviceApi.fetchDevices).mockResolvedValue([
      {
        id: 'm1',
        name: '计量1604',
        type: 'measure',
        model: 'WTN1604',
        host: '127.0.0.1',
        port: 9000,
        unit: 'kPa',
        status: 'connected'
      } as never
    ])
    vi.mocked(sessionApi.bindMeasureDevice).mockResolvedValue(undefined)
    vi.mocked(sessionApi.readValveStatus).mockResolvedValue('calibration')
    vi.mocked(sessionApi.readMeasureUnit).mockResolvedValue('kPa')
    vi.mocked(sessionApi.readDeviceInfo).mockResolvedValue({ model: 'WTN1604' })
  })

  it('在标定视图中以 calibration 模块身份重绑计量设备（而非默认的 measurement）', async () => {
    mountSync()
    await vi.waitFor(() => {
      expect(sessionApi.bindMeasureDevice).toHaveBeenCalled()
    })

    expect(sessionApi.bindMeasureDevice).toHaveBeenCalledWith('m1', 'calibration')
  })

  it('设备信息读取失败进入静默修复时，重绑同样以 calibration 身份', async () => {
    // 让 refreshDeviceInfo 失败：阀门/单位读取全部报错，触发静默修复分支
    vi.mocked(sessionApi.readValveStatus).mockRejectedValue(new Error('WTN1604: not connected'))
    vi.mocked(sessionApi.readMeasureUnit).mockRejectedValue(new Error('WTN1604: not connected'))

    mountSync()
    await vi.waitFor(
      () => {
        expect(deviceApi.connectDevice).toHaveBeenCalled()
      },
      // refreshDeviceInfo 重试 3 次 × 500ms 后才进入静默修复分支
      { timeout: 8000 }
    )

    expect(sessionApi.bindMeasureDevice).toHaveBeenCalledWith('m1', 'calibration')
  }, 15000)
})

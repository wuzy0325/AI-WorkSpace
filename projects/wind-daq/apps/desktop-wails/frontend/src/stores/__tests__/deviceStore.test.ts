import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useDeviceStore } from '@stores/deviceStore'
import { deviceApi } from '@api/deviceApi'

// mock useStorageStore：默认 historyWindowSec=30 + refreshRateHz=10 = 容量 300
// 测试用例通过 setWindowSec/setRefreshHz 模拟用户改配置
const mockSettings = vi.hoisted(() => {
  let windowSec = 30
  let refreshHz = 10
  return {
    getWindowSec: () => windowSec,
    setWindowSec: (v: number) => { windowSec = v },
    getRefreshHz: () => refreshHz,
    setRefreshHz: (v: number) => { refreshHz = v },
    settings: () => ({
      historyWindowSec: windowSec,
      refreshRateHz: refreshHz,
    }),
  }
})

// 复用 storageStore 的真实 computeHistoryCapacity 和 HISTORY_CAPACITY_HARD_CAP，
// 这样测试能真正验证生产代码的容量计算逻辑；只 mock useStorageStore 的返回值。
vi.mock('@stores/storageStore', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@stores/storageStore')>()
  return {
    ...actual,
    useStorageStore: () => ({
      settings: {
        get historyWindowSec() { return mockSettings.getWindowSec() },
        get refreshRateHz() { return mockSettings.getRefreshHz() },
      },
    }),
  }
})

// 默认容量：30 秒 × 10Hz = 300 点（保持与 mock 默认值一致）
const DEFAULT_CAP = 30 * 10

describe('deviceStore', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    setActivePinia(createPinia())
    // 重置 mock 配置为默认值：30 秒 × 10Hz = 300 点
    mockSettings.setWindowSec(30)
    mockSettings.setRefreshHz(10)
  })

  it('initializes with no profiles', () => {
    const store = useDeviceStore()
    expect(store.profiles).toEqual([])
  })

  it('selects and deselects a device', () => {
    const store = useDeviceStore()
    store.selectDevice('sim-1')
    expect(store.selectedDeviceId).toBe('sim-1')
    store.selectDevice(null)
    expect(store.selectedDeviceId).toBeNull()
  })

  it('formats backend-calibrated values without applying a frontend offset', () => {
    const store = useDeviceStore()
    expect(store.formatValue('sim-1', 0, 1.25)).toBe('1.250')
    expect('applyDisplayTare' in store).toBe(false)
    expect('tareAllEnabled' in store).toBe(false)
  })

  it('runs device calibration and refreshes persisted profile metadata', async () => {
    const profile = {
      id: 'pressure-1', name: 'Pressure', type: 'DAQ-P-1604' as const, samplingRate: 20,
      channels: [{ index: 0, name: 'CH1', enabled: true, unit: 'Pa', precision: 2 }],
    }
    vi.spyOn(deviceApi, 'getProfiles').mockResolvedValue([profile])
    vi.spyOn(deviceApi, 'calibrate').mockResolvedValue([
      { channelIndex: 0, offset: 12, unit: 'Pa', at: 123, sampleCount: 10 },
    ])
    vi.spyOn(deviceApi, 'getCalibrationProgress').mockResolvedValue({ running: true, elapsedMs: 1000, sampleCount: 5 })
    const store = useDeviceStore()
    await store.refreshProfiles()
    store.updateStatus('pressure-1', { id: 'pressure-1', name: 'Pressure', type: 'DAQ-P-1604', connection: 'Acquiring', acquiring: true })

    const results = await store.calibrate('pressure-1', 0)

    expect(deviceApi.calibrate).toHaveBeenCalledWith('pressure-1', 0, expect.any(AbortSignal))
    expect(results[0]?.offset).toBe(12)
    expect(store.calibrationOperationFor('pressure-1')).toMatchObject({ state: 'completed', sampleCount: 10 })
  })

  it('rejects calibration before acquisition starts', async () => {
    const store = useDeviceStore()
    await expect(store.calibrate('pressure-1')).rejects.toThrow('请先开始采集')
  })

  it('pushes a snapshot and makes it available via latestFor', () => {
    const store = useDeviceStore()
    store.pushSnapshot({
      deviceId: 'sim-1',
      timestamp: 123,
      channels: [1.5, 2.5],
      channelIndices: [0, 1],
    })
    expect(store.latestSnapshots.length).toBe(1)
    expect(store.latestFor('sim-1')?.channels[0]).toBe(1.5)
  })

  it('updates latest but does not append duplicate timestamps to history', () => {
    const store = useDeviceStore()

    store.pushSnapshot({
      deviceId: 'sim-1',
      timestamp: 123,
      channels: [1.5],
      channelIndices: [0],
    })
    store.flushPendingToHistory()
    store.pushSnapshot({
      deviceId: 'sim-1',
      timestamp: 123,
      channels: [2.5],
      channelIndices: [0],
    })
    store.flushPendingToHistory()
    store.pushSnapshot({
      deviceId: 'sim-1',
      timestamp: 124,
      channels: [2.5],
      channelIndices: [0],
    })
    store.flushPendingToHistory()

    expect(store.latestFor('sim-1')?.channels[0]).toBe(2.5)
    expect(store.historyFor('sim-1')).toHaveLength(2)
    expect(store.historyFor('sim-1').map((snapshot) => snapshot.timestamp)).toEqual([123, 124])
  })

  it('does not restart acquisition state from a stale snapshot', () => {
    const store = useDeviceStore()
    store.updateStatus('sim-1', {
      id: 'sim-1',
      name: 'Simulated',
      type: 'SIMULATED',
      connection: 'Connected',
      acquiring: false,
    })

    store.pushSnapshot({
      deviceId: 'sim-1',
      timestamp: 123,
      channels: [1.5],
      channelIndices: [0],
    })

    expect(store.acquiringFor('sim-1')).toBe(false)
  })

  it('keeps history buffer within capacity（环形缓冲默认 300 = 30s × 10Hz）', () => {
    const store = useDeviceStore()
    const payload = {
      deviceId: 'sim-1',
      timestamp: 0,
      channels: [1],
      channelIndices: [0],
    }
    // 模拟 renderTick 节流：每次 push 后立即 flush（与 store 内 setInterval 行为等价）
    for (let i = 0; i < DEFAULT_CAP + 100; i++) {
      store.pushSnapshot({ ...payload, timestamp: i })
      store.flushPendingToHistory()
    }
    // 默认 30s × 10Hz = 300 点，缓冲区不会超过这个值
    expect(store.historyFor('sim-1').length).toBe(DEFAULT_CAP)
  })

  it('normalizes profiles with null channels during refresh', async () => {
    const store = useDeviceStore()
    store.profilesLoadError = 'previous error'
    vi.spyOn(deviceApi, 'getProfiles').mockResolvedValueOnce([
      {
        id: 'legacy-1',
        name: 'Legacy Device',
        type: 'SIMULATED',
        samplingRate: 20,
        channels: null,
      } as never,
    ])

    await store.refreshProfiles()

    expect(store.profiles).toHaveLength(1)
    expect(store.profiles[0].channels).toEqual([])
    expect(store.profilesLoading).toBe(false)
    expect(store.profilesLoadError).toBeNull()
    expect(() => store.selectDevice('legacy-1')).not.toThrow()
  })

  it('keeps existing profiles when refresh fails', async () => {
    const store = useDeviceStore()
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
    store.profiles = [{
      id: 'daq-1',
      name: 'DAQ 1',
      type: 'SIMULATED',
      samplingRate: 20,
      channels: [],
    }]
    vi.spyOn(deviceApi, 'getProfiles').mockRejectedValueOnce(new Error('backend temporarily unavailable'))

    await expect(store.refreshProfiles()).rejects.toThrow('backend temporarily unavailable')

    expect(warn).toHaveBeenCalledWith(
      '[deviceStore] refreshProfiles failed, keeping previous profiles:',
      expect.any(Error),
    )
    expect(store.profilesLoading).toBe(false)
    expect(store.profilesLoadError).toBe('backend temporarily unavailable')
    expect(store.profiles.map((profile) => profile.id)).toEqual(['daq-1'])
  })

  it('keeps profile list stable when starting all acquisitions', async () => {
    const store = useDeviceStore()
    store.profiles = [{
      id: 'daq-1',
      name: 'DAQ 1',
      type: 'SIMULATED',
      samplingRate: 20,
      channels: [],
    }]
    store.updateStatus('daq-1', {
      id: 'daq-1',
      name: 'DAQ 1',
      type: 'SIMULATED',
      connection: 'Connected',
      acquiring: false,
    })

    const getProfiles = vi.spyOn(deviceApi, 'getProfiles').mockResolvedValue([{
      id: 'unexpected',
      name: 'Unexpected Device',
      type: 'SIMULATED',
      samplingRate: 20,
      channels: [],
    }])
    vi.spyOn(deviceApi, 'startAcquisition').mockResolvedValue({ success: true })
    vi.spyOn(deviceApi, 'getStatus').mockResolvedValue({
      id: 'daq-1',
      name: 'DAQ 1',
      type: 'SIMULATED',
      connection: 'Connected',
      acquiring: true,
    })
    vi.spyOn(deviceApi, 'subscribeToDevice').mockImplementation(() => undefined)

    await store.startAllAcquisitions()

    expect(getProfiles).not.toHaveBeenCalled()
    expect(store.profiles.map((profile) => profile.id)).toEqual(['daq-1'])
    expect(store.acquiringFor('daq-1')).toBe(true)
  })

  it('stops acquisition through store and clears acquiring status', async () => {
    const store = useDeviceStore()
    store.profiles = [{
      id: 'daq-1',
      name: 'DAQ 1',
      type: 'SIMULATED',
      samplingRate: 20,
      channels: [],
    }]
    store.updateStatus('daq-1', {
      id: 'daq-1',
      name: 'DAQ 1',
      type: 'SIMULATED',
      connection: 'Acquiring',
      acquiring: true,
    })

    const stop = vi.spyOn(deviceApi, 'stopAcquisition').mockResolvedValue({ success: true })
    const unsubscribe = vi.spyOn(deviceApi, 'unsubscribeFromDevice').mockImplementation(() => undefined)
    vi.spyOn(deviceApi, 'getStatus').mockResolvedValue({
      id: 'daq-1',
      name: 'DAQ 1',
      type: 'SIMULATED',
      connection: 'Connected',
      acquiring: false,
    })

    await store.stopAcquisition('daq-1')

    expect(stop).toHaveBeenCalledWith('daq-1')
    expect(unsubscribe).toHaveBeenCalledWith('daq-1')
    expect(store.acquiringFor('daq-1')).toBe(false)
    expect(store.statusFor('daq-1')).toBe('Connected')
  })

  // ===== 环形缓冲区新增测试 =====

  it('环形缓冲满后覆盖最旧，length 不超过 capacity', () => {
    const store = useDeviceStore()
    const cap = DEFAULT_CAP // 默认 30s × 10Hz = 300
    for (let i = 0; i < cap + 10; i++) {
      store.pushSnapshot({
        deviceId: 'sim-1',
        timestamp: i,
        channels: [i],
        channelIndices: [0],
      })
      store.flushPendingToHistory()
    }
    const history = store.historyFor('sim-1')
    expect(history.length).toBe(cap)
    // 最旧元素应该是第 10 条（前 10 条已被覆盖）
    expect(history[0].timestamp).toBe(10)
    expect(history[history.length - 1].timestamp).toBe(cap + 9)
  })

  it('每次 flush 后 historyVersion 递增', () => {
    const store = useDeviceStore()
    const v0 = store.historyVersion
    store.pushSnapshot({
      deviceId: 'sim-1',
      timestamp: 1,
      channels: [1],
      channelIndices: [0],
    })
    store.flushPendingToHistory()
    expect(store.historyVersion).toBe(v0 + 1)
    store.pushSnapshot({
      deviceId: 'sim-1',
      timestamp: 2,
      channels: [1],
      channelIndices: [0],
    })
    store.flushPendingToHistory()
    expect(store.historyVersion).toBe(v0 + 2)
  })

  it('容量对齐：mock historyWindowSec=50 + refreshRateHz=10 → ring.capacity=300（被 HARD_CAP 截断）', () => {
    // 50s × 10Hz = 500 点，但当前 HISTORY_CAPACITY_HARD_CAP=300 兜底
    mockSettings.setWindowSec(50)
    mockSettings.setRefreshHz(10)
    const store = useDeviceStore()
    for (let i = 0; i < 300 + 10; i++) {
      store.pushSnapshot({
        deviceId: 'sim-1',
        timestamp: i,
        channels: [i],
        channelIndices: [0],
      })
      store.flushPendingToHistory()
    }
    expect(store.historyFor('sim-1').length).toBe(300)
  })

  it('安全上限：mock historyWindowSec=30 + refreshRateHz=10 → ring.capacity=300（HARD_CAP 兜底）', () => {
    // 30s × 10Hz = 300 点，正好等于 HISTORY_CAPACITY_HARD_CAP=300
    mockSettings.setWindowSec(30)
    mockSettings.setRefreshHz(10)
    const store = useDeviceStore()
    for (let i = 0; i < 350; i++) {
      store.pushSnapshot({
        deviceId: 'sim-1',
        timestamp: i,
        channels: [i],
        channelIndices: [0],
      })
      store.flushPendingToHistory()
    }
    // HISTORY_CAPACITY_HARD_CAP = 300 硬上限
    expect(store.historyFor('sim-1').length).toBe(300)
  })

  it('容量变化重建：先写满默认 300，缩小时间窗口 → 下次 flush 后 ring 重建', () => {
    const store = useDeviceStore()
    // 先写 DEFAULT_CAP 帧（30s × 10Hz = 300）
    for (let i = 0; i < DEFAULT_CAP; i++) {
      store.pushSnapshot({
        deviceId: 'sim-1',
        timestamp: i,
        channels: [i],
        channelIndices: [0],
      })
      store.flushPendingToHistory()
    }
    expect(store.historyFor('sim-1').length).toBe(DEFAULT_CAP)

    // 缩小时间窗口至 5s × 10Hz = 50 点
    mockSettings.setWindowSec(5)
    mockSettings.setRefreshHz(10)
    // 下次 flush 检测 ring.capacity(300) ≠ 期望容量(50) → 自动重建
    store.pushSnapshot({
      deviceId: 'sim-1',
      timestamp: 200,
      channels: [200],
      channelIndices: [0],
    })
    store.flushPendingToHistory()
    // 旧 ring 被丢弃，新 ring capacity=50，只有刚 push 的 1 条
    expect(store.historyFor('sim-1').length).toBe(1)

    // 继续推 60 条
    for (let i = 1; i < 60; i++) {
      store.pushSnapshot({
        deviceId: 'sim-1',
        timestamp: 200 + i,
        channels: [200 + i],
        channelIndices: [0],
      })
      store.flushPendingToHistory()
    }
    expect(store.historyFor('sim-1').length).toBe(50)
  })

  it('多个设备各自独立的环形缓冲', () => {
    const store = useDeviceStore()
    // dev-a 超过 DEFAULT_CAP（300），会被裁剪
    for (let i = 0; i < DEFAULT_CAP + 50; i++) {
      store.pushSnapshot({
        deviceId: 'dev-a',
        timestamp: i,
        channels: [i],
        channelIndices: [0],
      })
      store.flushPendingToHistory()
    }
    // dev-b 未满
    for (let i = 0; i < 60; i++) {
      store.pushSnapshot({
        deviceId: 'dev-b',
        timestamp: i,
        channels: [i * 10],
        channelIndices: [0],
      })
      store.flushPendingToHistory()
    }
    // dev-a 超出容量 300，被裁剪
    expect(store.historyFor('dev-a').length).toBe(DEFAULT_CAP)
    expect(store.historyFor('dev-a')[0].timestamp).toBe(50)
    // dev-b 未满
    expect(store.historyFor('dev-b').length).toBe(60)
  })

  it('renderTick 节流：连续 push 多次同设备，pending 只保留最新一帧', () => {
    // 验证节流架构：后端按 refreshRateHz 推送但 renderTick 10Hz 消费时，
    // pendingSnapshots 覆盖式写入，flush 后只写入 1 个点
    const store = useDeviceStore()
    store.pushSnapshot({ deviceId: 'sim-1', timestamp: 1, channels: [1], channelIndices: [0] })
    store.pushSnapshot({ deviceId: 'sim-1', timestamp: 2, channels: [2], channelIndices: [0] })
    store.pushSnapshot({ deviceId: 'sim-1', timestamp: 3, channels: [3], channelIndices: [0] })
    // 三次 push 都没 flush，pending 只保留 timestamp=3 的帧
    store.flushPendingToHistory()
    expect(store.historyFor('sim-1').length).toBe(1)
    expect(store.historyFor('sim-1')[0].timestamp).toBe(3)
    // 但 latestSnapshots 仍是最新值（通道卡片实时性不受影响）
    expect(store.latestFor('sim-1')?.channels[0]).toBe(3)
  })
})

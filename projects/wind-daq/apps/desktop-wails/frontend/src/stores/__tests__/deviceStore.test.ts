import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useDeviceStore } from '@stores/deviceStore'
import { deviceApi } from '@api/deviceApi'

// mock useStorageStore：默认 waveformBufferSize=100
const mockWaveformBufferSize = vi.hoisted(() => {
  let value = 100
  return {
    get: () => value,
    set: (v: number) => { value = v },
    settings: () => ({ waveformBufferSize: value }),
  }
})

vi.mock('@stores/storageStore', () => ({
  useStorageStore: () => ({
    settings: {
      get waveformBufferSize() { return mockWaveformBufferSize.get() },
    },
  }),
  WAVEFORM_BUFFER_MAX: 2000,
  WAVEFORM_BUFFER_MIN: 50,
  WAVEFORM_BUFFER_STEP: 50,
}))

describe('deviceStore', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    setActivePinia(createPinia())
    mockWaveformBufferSize.set(100)
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
    store.pushSnapshot({
      deviceId: 'sim-1',
      timestamp: 123,
      channels: [2.5],
      channelIndices: [0],
    })
    store.pushSnapshot({
      deviceId: 'sim-1',
      timestamp: 124,
      channels: [2.5],
      channelIndices: [0],
    })

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

  it('keeps history buffer within capacity（环形缓冲默认 100）', () => {
    const store = useDeviceStore()
    const payload = {
      deviceId: 'sim-1',
      timestamp: 0,
      channels: [1],
      channelIndices: [0],
    }
    for (let i = 0; i < 300; i++) {
      store.pushSnapshot({ ...payload, timestamp: i })
    }
    // 默认 waveformBufferSize=100，缓冲区不会超过这个值
    expect(store.historyFor('sim-1').length).toBe(100)
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
    const cap = 100 // 默认 waveformBufferSize
    for (let i = 0; i < cap + 10; i++) {
      store.pushSnapshot({
        deviceId: 'sim-1',
        timestamp: i,
        channels: [i],
        channelIndices: [0],
      })
    }
    const history = store.historyFor('sim-1')
    expect(history.length).toBe(cap)
    // 最旧元素应该是第 10 条（前 10 条已被覆盖）
    expect(history[0].timestamp).toBe(10)
    expect(history[history.length - 1].timestamp).toBe(cap + 9)
  })

  it('每次 push 后 historyVersion 递增', () => {
    const store = useDeviceStore()
    const v0 = store.historyVersion
    store.pushSnapshot({
      deviceId: 'sim-1',
      timestamp: 1,
      channels: [1],
      channelIndices: [0],
    })
    expect(store.historyVersion).toBe(v0 + 1)
    store.pushSnapshot({
      deviceId: 'sim-1',
      timestamp: 2,
      channels: [1],
      channelIndices: [0],
    })
    expect(store.historyVersion).toBe(v0 + 2)
  })

  it('容量对齐：mock waveformBufferSize=1500 → ring.capacity=1500', () => {
    mockWaveformBufferSize.set(1500)
    const store = useDeviceStore()
    store.pushSnapshot({
      deviceId: 'sim-1',
      timestamp: 1,
      channels: [1],
      channelIndices: [0],
    })
    // 还未写满，但 capacity 已经是 1500
    for (let i = 0; i < 1500 + 10; i++) {
      store.pushSnapshot({
        deviceId: 'sim-1',
        timestamp: i + 2,
        channels: [i],
        channelIndices: [0],
      })
    }
    expect(store.historyFor('sim-1').length).toBe(1500)
  })

  it('安全上限：mock waveformBufferSize=2500 → ring.capacity=2000（MAX 兜底）', () => {
    mockWaveformBufferSize.set(2500)
    const store = useDeviceStore()
    for (let i = 0; i < 2100; i++) {
      store.pushSnapshot({
        deviceId: 'sim-1',
        timestamp: i,
        channels: [i],
        channelIndices: [0],
      })
    }
    // MAX_HISTORY_POINTS = 2000 硬上限
    expect(store.historyFor('sim-1').length).toBe(2000)
  })

  it('容量变化重建：先写满默认 100，缩小 waveformBufferSize=50 → 下次 push 后 ring 重建', () => {
    const store = useDeviceStore()
    // 先写 100 帧（默认波形缓容量 100）
    for (let i = 0; i < 100; i++) {
      store.pushSnapshot({
        deviceId: 'sim-1',
        timestamp: i,
        channels: [i],
        channelIndices: [0],
      })
    }
    expect(store.historyFor('sim-1').length).toBe(100)

    // 缩小容量至 50
    mockWaveformBufferSize.set(50)
    // 下次 push 检测 ring.capacity(100) ≠ 期望容量(50) → 自动重建
    store.pushSnapshot({
      deviceId: 'sim-1',
      timestamp: 200,
      channels: [200],
      channelIndices: [0],
    })
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
    }
    expect(store.historyFor('sim-1').length).toBe(50)
  })

  it('多个设备各自独立的环形缓冲', () => {
    const store = useDeviceStore()
    for (let i = 0; i < 120; i++) {
      store.pushSnapshot({
        deviceId: 'dev-a',
        timestamp: i,
        channels: [i],
        channelIndices: [0],
      })
    }
    for (let i = 0; i < 60; i++) {
      store.pushSnapshot({
        deviceId: 'dev-b',
        timestamp: i,
        channels: [i * 10],
        channelIndices: [0],
      })
    }
    // dev-a 超出容量 100，被裁剪
    expect(store.historyFor('dev-a').length).toBe(100)
    expect(store.historyFor('dev-a')[0].timestamp).toBe(20)
    // dev-b 未满
    expect(store.historyFor('dev-b').length).toBe(60)
  })
})

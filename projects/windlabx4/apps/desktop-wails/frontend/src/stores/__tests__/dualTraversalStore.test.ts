import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

const mockEnsureDeviceSubscribed = vi.hoisted(() => vi.fn())

// Mock wails-adapter：测试走 fetch 分支
vi.mock('@api/wails-adapter', () => ({
  isWailsAvailable: () => false,
  wailsApi: {},
}))

// Mock traversalApi：probe-aware 函数全部由用例编程返回值
vi.mock('@api/traversalApi', () => ({
  traversalProbeApi: {
    getConfig: vi.fn(async () => ({ success: true, data: null })),
    saveConfig: vi.fn(async () => ({ success: true })),
    start: vi.fn(async () => ({ success: true, data: { taskId: 'server-task-1' } })),
    pause: vi.fn(async () => ({ success: true })),
    resume: vi.fn(async () => ({ success: true })),
    stop: vi.fn(async () => ({ success: true })),
    close: vi.fn(async () => ({ success: true })),
    calculateRealtime: vi.fn(async () => ({ success: true, data: { isValid: true, alpha: 1 } })),
    importPrb: vi.fn(async () => ({ success: true, data: { filePath: 'a.prb' } })),
    importMultiPrb: vi.fn(async () => ({ success: true, data: { files: [], machNumbers: [], warnings: [] } })),
    importCalibrationCsv: vi.fn(async () => ({ success: true, data: { filePath: 'a.csv' } })),
    importSevenHolePrb: vi.fn(async () => ({ success: true, data: { files: [], validRange: {} } })),
    importSevenHoleCalibrationCsv: vi.fn(async () => ({ success: true, data: { files: [], validRange: {} } })),
    clearInterpolator: vi.fn(async () => ({ success: true, data: { cleared: true } })),
    checkPreconditions: vi.fn(async () => ({ success: true, data: { allPassed: true, checks: [] } })),
    getStatus: vi.fn(async () => ({ success: true, data: null })),
    onStatus: vi.fn(() => () => {}),
    onProgress: vi.fn(() => () => {}),
    onComplete: vi.fn(() => () => {}),
    onError: vi.fn(() => () => {}),
  },
}))

// Mock keyed polling：记录 invalidate 调用
vi.mock('@api/traversalPolling', () => ({
  invalidateProbePolling: vi.fn(),
}))

// Mock deviceApi：记录订阅/退订
vi.mock('@api/deviceApi', () => ({
  deviceApi: {
    onSnapshot: vi.fn(() => () => {}),
    subscribeToDevice: vi.fn(),
    unsubscribeFromDevice: vi.fn(),
  },
}))

vi.mock('@stores/i18nStore', () => ({
  // C7 修复后 dualTraversalStore 大量使用 i18n.t.dualErr* 键。
  // 用 Proxy 返回包含占位符的真实模板，让 safeInterpolate 能正确替换 {error} 等，
  // 测试可以验证错误消息透传逻辑。未知键回退到 '{error}' 模板以保留可调试性。
  useI18nStore: () => ({
    t: new Proxy({}, {
      get: (_target, prop: string) => {
        const templates: Record<string, string> = {
          dualErrVerifyInterpolator: 'verify failed: {error}',
          dualErrVerifyInterpolatorException: 'verify exception: {error}',
          dualErrInterpolatorNotLoaded: 'not loaded',
          dualErrRecoverRuntime: 'recover failed',
          dualErrSaveConfig: 'save failed',
          dualErrImportPrb: 'prb import failed',
          dualErrImportCalibrationCsv: 'csv import failed',
          dualErrImportMultiPrb: 'multi prb import failed',
          dualErrImportSevenHolePrb: '7hole prb import failed',
          dualErrImportSevenHoleCalibrationCsv: '7hole csv import failed',
          dualErrClearInterpolator: 'clear failed',
          dualErrStart: 'start failed',
          dualErrPause: 'pause failed',
          dualErrResume: 'resume failed',
          dualErrStop: 'stop failed',
          dualErrClose: 'close failed',
          travErrResponseEmpty: 'response empty',
        }
        return templates[prop] ?? ''
      },
    }),
    locale: 'zh',
  }),
}))

vi.mock('@stores/storageStore', () => ({
  useStorageStore: () => ({ settings: { refreshRateHz: 5 } }),
}))

vi.mock('@stores/deviceStore', () => ({
  useDeviceStore: () => ({ ensureSubscribed: mockEnsureDeviceSubscribed }),
}))

import {
  dualDeviceRefCount,
  resetDualDeviceRefCounts,
  useDualTraversalStore,
} from '@stores/dualTraversalStore'
import { traversalProbeApi } from '@api/traversalApi'
import { invalidateProbePolling } from '@api/traversalPolling'
import { deviceApi } from '@api/deviceApi'
import type { TraversalTestConfig } from '@shared/types/traversal'
import type { ProbeChannelRole } from '@shared/types/calibration'

const mockApi = traversalProbeApi as unknown as Record<string, ReturnType<typeof vi.fn>>
const mockInvalidate = invalidateProbePolling as ReturnType<typeof vi.fn>
const mockDevice = deviceApi as unknown as {
  onSnapshot: ReturnType<typeof vi.fn>
  subscribeToDevice: ReturnType<typeof vi.fn>
  unsubscribeFromDevice: ReturnType<typeof vi.fn>
}

function configWithDevices(...deviceIds: string[]): TraversalTestConfig {
  return {
    probeType: 'five-hole',
    channels: {
      probeChannels: deviceIds.map((deviceId, i) => ({
        name: `P${i + 1}`,
        channel: { deviceId, channelIndex: i },
        enabled: true,
      })),
      motionAxes: [],
    },
  } as unknown as TraversalTestConfig
}

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
  resetDualDeviceRefCounts()
})

describe('dualTraversalStore keyed session 隔离', () => {
  it('两路 session 完全隔离：一路 reset 不修改另一路', async () => {
    const store = useDualTraversalStore()
    store.sessions.probe1.config = configWithDevices('dev-a')
    store.sessions.probe1.error = 'boom'
    store.sessions.probe2.config = configWithDevices('dev-b')
    store.sessions.probe2.hasLoadedInterpolator = true

    store.reset('probe1')

    expect(store.sessions.probe1.config).toBeNull()
    expect(store.sessions.probe1.error).toBeNull()
    // 另一路不受影响
    expect(store.sessions.probe2.config).not.toBeNull()
    expect(store.sessions.probe2.hasLoadedInterpolator).toBe(true)
  })

  it('start 路由到正确 probe 且只初始化本 probe 运行期', async () => {
    const store = useDualTraversalStore()
    store.sessions.probe1.config = configWithDevices('dev-a')
    const ok = await store.start('probe1')

    expect(ok).toBe(true)
    expect(mockApi.start).toHaveBeenCalledWith('probe1', store.sessions.probe1.config)
    expect(store.sessions.probe1.status?.taskId).toBe('server-task-1')
    // probe2 完全未动
    expect(store.sessions.probe2.status).toBeNull()
    expect(traversalProbeApi.onProgress).toHaveBeenCalledWith('probe1', expect.any(Function))
    expect(traversalProbeApi.onProgress).not.toHaveBeenCalledWith('probe2', expect.any(Function))
  })

  it('start 失败只污染本 probe error，isStarting 复位', async () => {
    const store = useDualTraversalStore()
    store.sessions.probe1.config = configWithDevices('dev-a')
    store.sessions.probe2.config = configWithDevices('dev-b')
    mockApi.start.mockResolvedValueOnce({ success: false, error: 'resource_conflict: 占用' })

    const ok = await store.start('probe1')
    expect(ok).toBe(false)
    expect(store.sessions.probe1.error).toContain('resource_conflict')
    expect(store.sessions.probe1.isStarting).toBe(false)
    expect(store.sessions.probe2.error).toBeNull()
  })

  it('POST start pending 时 anyActive/isActive 阻止模式切换', async () => {
    let resolveStart!: (value: { success: boolean; data: { taskId: string } }) => void
    mockApi.start.mockReturnValueOnce(new Promise((resolve) => { resolveStart = resolve }))
    const store = useDualTraversalStore()
    store.sessions.probe1.config = configWithDevices('dev-a')

    const pending = store.start('probe1')

    expect(store.sessions.probe1.isStarting).toBe(true)
    expect(store.isActive('probe1')).toBe(true)
    expect(store.anyActive).toBe(true)
    resolveStart({ success: true, data: { taskId: 'deferred-task' } })
    await pending
  })

  it('start 请求被后端接受前 reset 时不得复活运行状态，并停止已创建任务', async () => {
    let resolveStart!: (value: { success: boolean; data: { taskId: string } }) => void
    mockApi.start.mockReturnValueOnce(new Promise((resolve) => { resolveStart = resolve }))
    const store = useDualTraversalStore()
    store.sessions.probe1.config = configWithDevices('dev-a')

    const pending = store.start('probe1')
    await vi.waitFor(() => expect(mockApi.start).toHaveBeenCalledOnce())
    store.reset('probe1')
    resolveStart({ success: true, data: { taskId: 'orphan-task' } })

    expect(await pending).toBe(false)
    expect(mockApi.stop).toHaveBeenCalledWith('probe1')
    expect(store.sessions.probe1.status).toBeNull()
    expect(mockApi.onProgress).not.toHaveBeenCalled()
    expect(dualDeviceRefCount('dev-a')).toBe(0)
  })

  it('start 使该 probe 轮询代际失效（丢弃旧响应），另一路不失效', async () => {
    const store = useDualTraversalStore()
    store.sessions.probe2.config = configWithDevices('dev-b')
    await store.start('probe2')
    expect(mockInvalidate).toHaveBeenCalledWith('probe2')
    expect(mockInvalidate).not.toHaveBeenCalledWith('probe1')
  })

  it('生命周期 actions 全部显式按 probe 路由', async () => {
    const store = useDualTraversalStore()
    await store.pause('probe2')
    await store.resume('probe1')
    await store.stop('probe2')
    await store.close('probe1')

    expect(mockApi.pause).toHaveBeenCalledWith('probe2')
    expect(mockApi.resume).toHaveBeenCalledWith('probe1')
    expect(mockApi.stop).toHaveBeenCalledWith('probe2')
    expect(mockApi.close).toHaveBeenCalledWith('probe1')
  })

  it('轮询原始状态与成功 pause/resume 立即更新本地 session 状态', async () => {
    let statusCallback!: (status: { taskId: string; status: string; state: string }) => void
    mockApi.onStatus.mockImplementationOnce((_probeId, callback) => {
      statusCallback = callback
      return () => {}
    })
    const store = useDualTraversalStore()
    store.sessions.probe1.config = configWithDevices('dev-a')
    await store.start('probe1')

    statusCallback({ taskId: 'server-task-1', status: 'paused', state: 'paused' })
    expect(store.sessions.probe1.status?.status).toBe('paused')

    expect(await store.resume('probe1')).toBe(true)
    expect(store.sessions.probe1.status?.status).toBe('running')
    expect(await store.pause('probe1')).toBe(true)
    expect(store.sessions.probe1.status?.status).toBe('paused')
  })

  it('恢复 paused session 后 resume 立即切换为 running 控制状态', async () => {
    const store = useDualTraversalStore()
    mockApi.getStatus.mockResolvedValueOnce({
      success: true,
      data: { taskId: 'recovered-task', status: 'paused', state: 'paused' },
    })
    await store.recoverRuntime('probe2')

    expect(store.sessions.probe2.status?.status).toBe('paused')
    expect(await store.resume('probe2')).toBe(true)
    expect(store.sessions.probe2.status?.status).toBe('running')
  })
})

describe('dualTraversalStore 设备订阅引用计数', () => {
  it('配置加载后从 DAQ 快照更新本 probe 实时压力与插值输入', async () => {
    let snapshotCallback!: (payload: {
      deviceId: string
      channelIndices: number[]
      channels: number[]
    }) => void
    mockDevice.onSnapshot.mockImplementationOnce((callback) => {
      snapshotCallback = callback
      return () => {}
    })
    mockApi.getConfig.mockResolvedValueOnce({
      success: true,
      data: {
        ...configWithDevices('dev-a', 'dev-a', 'dev-a', 'dev-a', 'dev-a', 'dev-a', 'dev-a'),
        prbFile: { filePath: 'probe.prb' },
        channels: {
          probeChannels: [
            ['fiveHole.p1', 0],
            ['fiveHole.p2', 1],
            ['fiveHole.p3', 2],
            ['fiveHole.p4', 3],
            ['fiveHole.p5', 4],
            ['fiveHole.pAtm', 5],
            ['fiveHole.tAtm', 6],
          ].map(([role, channelIndex]) => ({
            name: role,
            role: role as ProbeChannelRole,
            channel: { deviceId: 'dev-a', channelIndex },
            enabled: true,
          })),
          motionAxes: [],
        },
      },
    })
    const store = useDualTraversalStore()

    await store.loadConfig('probe1')
    snapshotCallback({
      deviceId: 'dev-a',
      channelIndices: [0, 1, 2, 3, 4, 5, 6],
      channels: [101, 102, 103, 104, 105, 100800, 23.5],
    })

    expect(store.sessions.probe1.realtimePressures).toMatchObject({
      P1: 101,
      P5: 105,
      Patm: 100800,
      Tatm: 23.5,
    })
    expect(mockApi.calculateRealtime).toHaveBeenCalledWith(
      'probe1',
      expect.objectContaining({ P1: 101, P5: 105, Patm: 100800, Tatm: 23.5 }),
      expect.anything(),
      'five-hole',
    )
  })

  it('运行态（running/moving/stabilizing/acquiring）DAQ 快照持续更新实时压力', async () => {
    let snapshotCallback!: (payload: {
      deviceId: string
      channelIndices: number[]
      channels: number[]
    }) => void
    mockDevice.onSnapshot.mockImplementationOnce((callback) => {
      snapshotCallback = callback
      return () => {}
    })
    const sevenChannels = [
      ['fiveHole.p1', 0],
      ['fiveHole.p2', 1],
      ['fiveHole.p3', 2],
      ['fiveHole.p4', 3],
      ['fiveHole.p5', 4],
      ['fiveHole.pAtm', 5],
      ['fiveHole.tAtm', 6],
    ].map(([role, channelIndex]) => ({
      name: role,
      role: role as ProbeChannelRole,
      channel: { deviceId: 'dev-a', channelIndex },
      enabled: true,
    }))
    mockApi.getConfig.mockResolvedValueOnce({
      success: true,
      data: {
        ...configWithDevices('dev-a', 'dev-a', 'dev-a', 'dev-a', 'dev-a', 'dev-a', 'dev-a'),
        prbFile: { filePath: 'probe.prb' },
        channels: { probeChannels: sevenChannels, motionAxes: [] },
      },
    })
    const store = useDualTraversalStore()
    await store.loadConfig('probe1')

    // 模拟运行态：status 为 running，验证 onSnapshot 不被抑制
    store.sessions.probe1.status = { taskId: 'active', status: 'moving' } as never
    snapshotCallback({
      deviceId: 'dev-a',
      channelIndices: [0, 1, 2, 3, 4, 5, 6],
      channels: [201, 202, 203, 204, 205, 101000, 24.1],
    })
    expect(store.sessions.probe1.realtimePressures).toMatchObject({
      P1: 201,
      P5: 205,
      Patm: 101000,
      Tatm: 24.1,
    })

    // 状态切换为 acquiring 后新快照仍能推送
    store.sessions.probe1.status = { taskId: 'active', status: 'acquiring' } as never
    snapshotCallback({
      deviceId: 'dev-a',
      channelIndices: [0, 1, 2, 3, 4, 5, 6],
      channels: [301, 302, 303, 304, 305, 101200, 24.3],
    })
    expect(store.sessions.probe1.realtimePressures).toMatchObject({
      P1: 301,
      P5: 305,
    })
  })

  it('测点进度不覆盖 DAQ 快照的实时压力和插值结果', async () => {
    let snapshotCallback!: (payload: {
      deviceId: string
      channelIndices: number[]
      channels: number[]
    }) => void
    let progressCallback!: (event: {
      latestData: {
        rawPressure: Record<string, number>
        interpolationResult: Record<string, number | boolean>
      }
    }) => void
    mockDevice.onSnapshot.mockImplementationOnce((callback) => {
      snapshotCallback = callback
      return () => {}
    })
    mockApi.onProgress.mockImplementationOnce((_probeId, callback) => {
      progressCallback = callback
      return () => {}
    })
    const channels = [
      ['sevenHole.p1', 0],
      ['sevenHole.p2', 1],
      ['sevenHole.p3', 2],
      ['sevenHole.p4', 3],
      ['sevenHole.p5', 4],
      ['sevenHole.p6', 5],
      ['sevenHole.p7', 6],
      ['sevenHole.pAtm', 7],
      ['sevenHole.tAtm', 8],
    ].map(([role, channelIndex]) => ({
      name: role,
      role: role as ProbeChannelRole,
      channel: { deviceId: 'dev-seven', channelIndex },
      enabled: true,
    }))
    const config = {
      ...configWithDevices(...Array(9).fill('dev-seven')),
      probeType: 'seven-hole',
      channels: { probeChannels: channels, motionAxes: [] },
    } as TraversalTestConfig
    const store = useDualTraversalStore()
    store.sessions.probe1.config = config
    await store.start('probe1')
    store.markInterpolatorLoaded('probe1')

    snapshotCallback({
      deviceId: 'dev-seven',
      channelIndices: [0, 1, 2, 3, 4, 5, 6, 7, 8],
      channels: [101, 102, 103, 104, 105, 106, 107, 100800, 23.5],
    })
    await vi.waitFor(() => {
      expect(store.sessions.probe1.realtimeResult).toMatchObject({ isValid: true, alpha: 1 })
    })
    progressCallback({
      latestData: {
        rawPressure: {
          P1: 9_999_999,
          P2: 9_999_999,
          P3: 9_999_999,
          P4: 9_999_999,
          P5: 9_999_999,
          P6: 9_999_999,
          P7: 9_999_999,
          Patm: 9_999_999,
          Tatm: 9_999_999,
        },
        interpolationResult: { isValid: true, alpha: 12.5 },
      },
    })

    expect(store.sessions.probe1.realtimePressures).toMatchObject({
      P1: 101,
      P7: 107,
      Patm: 100800,
      Tatm: 23.5,
    })
    expect(store.sessions.probe1.realtimeResult).toMatchObject({ isValid: true, alpha: 1 })
  })

  it('两路共享设备时一路卸载不取消另一路订阅', async () => {
    const store = useDualTraversalStore()
    store.sessions.probe1.config = configWithDevices('dev-shared', 'dev-a')
    store.sessions.probe2.config = configWithDevices('dev-shared')

    await store.start('probe1')
    await store.start('probe2')
    expect(mockEnsureDeviceSubscribed).toHaveBeenCalledWith('dev-shared')
    expect(mockDevice.subscribeToDevice).toHaveBeenCalledTimes(2) // dev-shared 一次 + dev-a 一次
    expect(dualDeviceRefCount('dev-shared')).toBe(2)
    expect(dualDeviceRefCount('dev-a')).toBe(1)

    // probe1 reset：只释放 probe1 的引用；dev-shared 仍被 probe2 持有
    store.reset('probe1')
    expect(mockDevice.unsubscribeFromDevice).toHaveBeenCalledWith('dev-a')
    expect(mockDevice.unsubscribeFromDevice).not.toHaveBeenCalledWith('dev-shared')
    expect(dualDeviceRefCount('dev-shared')).toBe(1)

    // probe2 reset：归零后真正退订
    store.reset('probe2')
    expect(mockDevice.unsubscribeFromDevice).toHaveBeenCalledWith('dev-shared')
    expect(dualDeviceRefCount('dev-shared')).toBe(0)
  })

  it('close 释放本 probe 全部设备引用', async () => {
    const store = useDualTraversalStore()
    store.sessions.probe1.config = configWithDevices('dev-a', 'dev-b')
    await store.start('probe1')
    expect(dualDeviceRefCount('dev-a')).toBe(1)

    await store.close('probe1')
    expect(dualDeviceRefCount('dev-a')).toBe(0)
    expect(dualDeviceRefCount('dev-b')).toBe(0)
  })

  it('close 失败后 cleanupLocal 释放本地资源并保留可重试状态与错误', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(0)
    try {
      const unsubscribeProgress = vi.fn()
      const unsubscribeComplete = vi.fn()
      const unsubscribeError = vi.fn()
      mockApi.onProgress.mockReturnValueOnce(unsubscribeProgress)
      mockApi.onComplete.mockReturnValueOnce(unsubscribeComplete)
      mockApi.onError.mockReturnValueOnce(unsubscribeError)
      mockApi.close.mockResolvedValueOnce({ success: false, error: 'server close failed' })
      const store = useDualTraversalStore()
      store.sessions.probe1.config = configWithDevices('dev-a')
      await store.start('probe1')
      store.markInterpolatorLoaded('probe1')
      store.syncRealtimeInput('probe1', { P1: 1 } as never)

      expect(await store.close('probe1')).toBe(false)
      expect(store.sessions.probe1.status?.status).toBe('running')
      expect(store.sessions.probe1.error).toBe('server close failed')

      store.cleanupLocal('probe1')
      await vi.advanceTimersByTimeAsync(250)

      expect(unsubscribeProgress).toHaveBeenCalledOnce()
      expect(unsubscribeComplete).toHaveBeenCalledOnce()
      expect(unsubscribeError).toHaveBeenCalledOnce()
      expect(mockDevice.unsubscribeFromDevice).toHaveBeenCalledWith('dev-a')
      expect(dualDeviceRefCount('dev-a')).toBe(0)
      expect(mockInvalidate).toHaveBeenCalledWith('probe1')
      expect(mockApi.calculateRealtime).not.toHaveBeenCalled()
      expect(store.sessions.probe1.status?.status).toBe('running')
      expect(store.sessions.probe1.error).toBe('server close failed')
    } finally {
      vi.useRealTimers()
    }
  })
})

describe('dualTraversalStore 实时计算', () => {
  it('实时计算 timer 按 probe 独立调度，输入路由正确', async () => {
    vi.useFakeTimers()
    try {
      const store = useDualTraversalStore()
      store.markInterpolatorLoaded('probe1')
      store.markInterpolatorLoaded('probe2')
      const input1 = { P1: 1 } as never
      const input2 = { P1: 2 } as never

      store.syncRealtimeInput('probe1', input1)
      store.syncRealtimeInput('probe2', input2)
      // refreshRateHz=5 → 200ms 节流；首距 0 → delay=200ms
      await vi.advanceTimersByTimeAsync(250)

      expect(mockApi.calculateRealtime).toHaveBeenCalledWith('probe1', input1, undefined, undefined)
      expect(mockApi.calculateRealtime).toHaveBeenCalledWith('probe2', input2, undefined, undefined)
      expect(store.sessions.probe1.realtimeResult).toEqual({ isValid: true, alpha: 1 })
      expect(store.sessions.probe2.realtimeResult).toEqual({ isValid: true, alpha: 1 })
    } finally {
      vi.useRealTimers()
    }
  })

  it('未加载插值器时不发起计算且清空结果（不影响另一路）', async () => {
    const store = useDualTraversalStore()
    store.markInterpolatorLoaded('probe2')
    store.sessions.probe1.realtimeResult = { isValid: true } as never

    store.syncRealtimeInput('probe1', { P1: 1 } as never)
    expect(mockApi.calculateRealtime).not.toHaveBeenCalled()
    expect(store.sessions.probe1.realtimeResult).toBeNull()

    store.syncRealtimeInput('probe2', { P1: 2 } as never)
    await new Promise((r) => setTimeout(r, 0))
    expect(mockApi.calculateRealtime).toHaveBeenCalledWith('probe2', { P1: 2 }, undefined, undefined)
  })

  it('实时计算透传当前 session probeType', async () => {
    const store = useDualTraversalStore()
    store.sessions.probe1.config = configWithDevices('dev-a')
    store.markInterpolatorLoaded('probe1')

    store.syncRealtimeInput('probe1', { P1: 1 } as never)
    await new Promise((resolve) => setTimeout(resolve, 0))

    expect(mockApi.calculateRealtime).toHaveBeenCalledWith(
      'probe1',
      { P1: 1 },
      store.sessions.probe1.config,
      'five-hole',
    )
  })

  it('计算失败时置占位结果（IsValid=false + warning）', async () => {
    const store = useDualTraversalStore()
    store.markInterpolatorLoaded('probe1')
    mockApi.calculateRealtime.mockResolvedValueOnce({ success: false, error: 'prb missing' })

    store.syncRealtimeInput('probe1', { P1: 1 } as never)
    await new Promise((r) => setTimeout(r, 0))

    const result = store.sessions.probe1.realtimeResult
    expect(result?.isValid).toBe(false)
    expect(result?.warning).toContain('prb missing')
    expect(store.sessions.probe2.realtimeResult).toBeNull()
  })
})

describe('dualTraversalStore 配置', () => {
  it('PRB operations 按 probe 路由并只更新该 session 插值器状态', async () => {
    const store = useDualTraversalStore()

    await store.importPrbFile('probe2', 'D:/probe2.prb')
    await store.importMultiPrbFiles('probe1', ['D:/a.prb'], [0.2], 'nearest')
    await store.importCalibrationCsvFile('probe2', 'D:/probe2.csv')
    await store.importSevenHolePrbFiles('probe1', '7.prb', ['1.prb'])
    await store.importSevenHoleCalibrationCsvFiles('probe2', '7.csv', ['1.csv'])

    expect(mockApi.importPrb).toHaveBeenCalledWith('probe2', 'D:/probe2.prb')
    expect(mockApi.importMultiPrb).toHaveBeenCalledWith('probe1', ['D:/a.prb'], [0.2], 'nearest')
    expect(mockApi.importCalibrationCsv).toHaveBeenCalledWith('probe2', 'D:/probe2.csv')
    expect(mockApi.importSevenHolePrb).toHaveBeenCalledWith('probe1', '7.prb', ['1.prb'])
    expect(mockApi.importSevenHoleCalibrationCsv).toHaveBeenCalledWith('probe2', '7.csv', ['1.csv'])
    expect(store.sessions.probe1.hasLoadedInterpolator).toBe(true)
    expect(store.sessions.probe2.hasLoadedInterpolator).toBe(true)

    await store.clearInterpolator('probe2', 'seven-hole')
    expect(mockApi.clearInterpolator).toHaveBeenCalledWith('probe2', 'seven-hole')
    expect(store.sessions.probe2.hasLoadedInterpolator).toBe(false)
    expect(store.sessions.probe1.hasLoadedInterpolator).toBe(true)
  })

  it('PRB operation 失败只写入目标 session error 且保留其已有插值器状态', async () => {
    const store = useDualTraversalStore()
    store.markInterpolatorLoaded('probe1')
    mockApi.importPrb.mockResolvedValueOnce({ success: false, error: 'bad prb' })

    expect(await store.importPrbFile('probe1', 'bad.prb')).toBeNull()
    expect(store.sessions.probe1.error).toBe('bad prb')
    expect(store.sessions.probe1.hasLoadedInterpolator).toBe(true)
    expect(store.sessions.probe2.error).toBeNull()
  })

  it('loadConfig/saveConfig 按 probe 读写各自配置', async () => {
    const store = useDualTraversalStore()
    const cfg = configWithDevices('dev-a')
    mockApi.getConfig.mockResolvedValueOnce({ success: true, data: cfg })

    await store.loadConfig('probe1')
    expect(store.sessions.probe1.config).toEqual(cfg)
    expect(store.sessions.probe2.config).toBeNull()

    await store.saveConfig('probe2', cfg)
    expect(mockApi.saveConfig).toHaveBeenCalledWith('probe2', cfg)
    expect(store.sessions.probe2.config).toEqual(cfg)
  })

  it('loadConfig 从五孔持久化 PRB 推断并经 probe-aware 后端校验恢复插值器', async () => {
    const store = useDualTraversalStore()
    const cfg = { ...configWithDevices('dev-a'), prbFile: { filePath: 'D:/five.prb' } } as TraversalTestConfig
    mockApi.getConfig.mockResolvedValueOnce({ success: true, data: cfg })

    await store.loadConfig('probe1')

    expect(store.sessions.probe1.hasLoadedInterpolator).toBe(true)
    expect(store.sessions.probe1.interpolatorRestoreMessage).toBeNull()
    expect(mockApi.checkPreconditions).toHaveBeenCalledWith('probe1', cfg)
  })

  it('loadConfig 从完整七孔持久化 PRB 集恢复插值器', async () => {
    const store = useDualTraversalStore()
    const files = Array.from({ length: 6 }, (_, index) => ({ filePath: `D:/outer-${index}.prb` }))
    const cfg = {
      ...configWithDevices('dev-b'),
      probeType: 'seven-hole',
      sevenHolePrb: {
        kind: 'seven-hole-prb-set',
        innerFile: { filePath: 'D:/inner.prb' },
        outerFiles: files,
      },
    } as TraversalTestConfig
    mockApi.getConfig.mockResolvedValueOnce({ success: true, data: cfg })

    await store.loadConfig('probe2')

    expect(store.sessions.probe2.hasLoadedInterpolator).toBe(true)
    expect(mockApi.checkPreconditions).toHaveBeenCalledWith('probe2', cfg)
  })

  it('loadConfig 后端校验网络失败保留推断 true 并暴露恢复消息', async () => {
    const store = useDualTraversalStore()
    const cfg = { ...configWithDevices('dev-a'), calibrationCsvFile: { filePath: 'D:/five.csv' } } as TraversalTestConfig
    mockApi.getConfig.mockResolvedValueOnce({ success: true, data: cfg })
    mockApi.checkPreconditions.mockResolvedValueOnce({ success: false, error: 'network unavailable' })

    await store.loadConfig('probe1')

    expect(store.sessions.probe1.hasLoadedInterpolator).toBe(true)
    expect(store.sessions.probe1.interpolatorRestoreMessage).toContain('network unavailable')
  })

  it('loadConfig 后端明确 PRB 检查失败时清除插值器并透传消息', async () => {
    const store = useDualTraversalStore()
    const cfg = { ...configWithDevices('dev-a'), prbFile: { filePath: 'D:/missing.prb' } } as TraversalTestConfig
    mockApi.getConfig.mockResolvedValueOnce({ success: true, data: cfg })
    mockApi.checkPreconditions.mockResolvedValueOnce({
      success: true,
      data: { allPassed: false, checks: [{ name: 'PRB', passed: false, message: 'PRB file missing' }] },
    })

    await store.loadConfig('probe1')

    expect(store.sessions.probe1.hasLoadedInterpolator).toBe(false)
    expect(store.sessions.probe1.interpolatorRestoreMessage).toBe('PRB file missing')
  })

  it('recoverRuntime 仅为活动 probe 恢复状态、监控和设备，且重复调用不重复订阅', async () => {
    const store = useDualTraversalStore()
    store.sessions.probe1.config = configWithDevices('dev-a')
    store.sessions.probe2.config = configWithDevices('dev-b')
    mockApi.getStatus
      .mockResolvedValueOnce({ success: true, data: { taskId: 'active-1', status: 'paused' } })
      .mockResolvedValueOnce({ success: true, data: { taskId: 'done-2', status: 'completed' } })

    await store.recoverRuntime('probe1')
    await store.recoverRuntime('probe2')

    expect(store.sessions.probe1.status?.status).toBe('paused')
    expect(store.sessions.probe2.status?.status).toBe('completed')
    expect(mockApi.onProgress).toHaveBeenCalledTimes(1)
    expect(mockApi.onProgress).toHaveBeenCalledWith('probe1', expect.any(Function))
    expect(mockDevice.subscribeToDevice).toHaveBeenCalledWith('dev-a')
    expect(mockDevice.subscribeToDevice).not.toHaveBeenCalledWith('dev-b')

    mockApi.getStatus.mockResolvedValueOnce({ success: true, data: { taskId: 'active-1', status: 'paused' } })
    await store.recoverRuntime('probe1')

    expect(mockApi.onProgress).toHaveBeenCalledTimes(1)
    expect(mockDevice.subscribeToDevice).toHaveBeenCalledTimes(1)
    expect(dualDeviceRefCount('dev-a')).toBe(1)
  })

  it('cleanup 后重新进入可恢复活动 probe 的监控和设备订阅', async () => {
    const firstUnsubscribe = vi.fn()
    mockApi.onProgress.mockReturnValueOnce(firstUnsubscribe)
    const store = useDualTraversalStore()
    store.sessions.probe1.config = configWithDevices('dev-a')
    mockApi.getStatus.mockResolvedValue({ success: true, data: { taskId: 'active-1', status: 'running' } })

    await store.recoverRuntime('probe1')
    store.cleanupLocal('probe1')
    await store.recoverRuntime('probe1')

    expect(firstUnsubscribe).toHaveBeenCalledOnce()
    expect(mockApi.onProgress).toHaveBeenCalledTimes(2)
    expect(mockDevice.subscribeToDevice).toHaveBeenCalledTimes(2)
    expect(mockDevice.unsubscribeFromDevice).toHaveBeenCalledOnce()
    expect(dualDeviceRefCount('dev-a')).toBe(1)
  })

  it('anyActive 派生：任一路活动时模式开关门禁', async () => {
    const store = useDualTraversalStore()
    expect(store.anyActive).toBe(false)
    store.sessions.probe1.status = { taskId: 't', status: 'running' } as never
    expect(store.anyActive).toBe(true)
    expect(store.isActive('probe1')).toBe(true)
    expect(store.isActive('probe2')).toBe(false)
    store.sessions.probe1.status = { taskId: 't', status: 'completed' } as never
    expect(store.anyActive).toBe(false)
  })
})

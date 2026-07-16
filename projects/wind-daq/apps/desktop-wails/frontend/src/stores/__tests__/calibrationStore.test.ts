import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

// Mock wails-adapter：让 isWailsAvailable 返回 false，store.startCalibration 会走 calibrationApi 分支
vi.mock('@api/wails-adapter', () => ({
  isWailsAvailable: () => false,
  wailsApi: {},
}))

// Mock calibrationApi：startCalibration 直接返回 success，status 由各测试用例通过 mockImplementation 覆盖
vi.mock('@api/calibrationApi', () => ({
  calibrationApi: {
    startCalibration: vi.fn(async () => ({ success: true })),
    pauseCalibration: vi.fn(async () => ({ success: true })),
    resumeCalibration: vi.fn(async () => ({ success: true })),
    stopCalibration: vi.fn(async () => ({ success: true })),
    status: vi.fn(async () => ({
      taskId: 'test',
      state: 'idle',
      currentPoint: 0,
      totalPoints: 0,
    })),
  },
}))

import { useCalibrationStore } from '@stores/calibrationStore'
import { calibrationApi } from '@api/calibrationApi'
import type { CalibrationConfig } from '@shared/types/calibration'

const baseConfig: CalibrationConfig = {
  type: 'five-hole',
  name: 'test',
  taskId: 'cal-test',
  points: [],
  probeChannels: [],
  motionAxes: [],
  samplesPerPoint: 1,
  savePath: '',
} as unknown as CalibrationConfig

describe('calibrationStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })

  // 测试前置：构造实时压力，Patm>0 且 P0=Ps（绝对压相等），Ttunnel 有效
  // 测试步骤：调用 store.updateRealtimePressures
  // 期待结果：calculatedPhysics 为 { machNumber: 0, velocity: 0 }（ratio=1 时 Ma=0）
  it('shows zero aerodynamic values when tunnel total and static pressure are equal', () => {
    const store = useCalibrationStore()

    store.updateRealtimePressures({
      P1: 0,
      P2: 0,
      P3: 0,
      P4: 0,
      P5: 0,
      Patm: 101325,
      Tatm: 25,
      P0: 0,
      Ps: 0,
      Ttunnel: 25,
    })

    expect(store.calculatedPhysics).toEqual({ machNumber: 0, velocity: 0 })
  })

  // 测试前置：构造实时压力，Patm=0（必需通道缺失），P0>0
  // 测试步骤：调用 store.updateRealtimePressures
  // 期待结果：calculatedPhysics 为 null（与后端 §22 对齐：pAtm 为必需通道，缺失时不兜底）
  it('returns null when Patm channel is zero (required channel, no fallback)', () => {
    const store = useCalibrationStore()

    store.updateRealtimePressures({
      P1: 0,
      P2: 0,
      P3: 0,
      P4: 0,
      P5: 0,
      Patm: 0,
      Tatm: 25,
      P0: 1000,
      Ps: 0,
      Ttunnel: 25,
    })

    expect(store.calculatedPhysics).toBeNull()
  })

  it('initializes timeInfo with non-null elapsedTime immediately after startCalibration', async () => {
    // 测试前置：mock wails 不可用 + calibrationApi.startCalibration 返回 success
    // 测试步骤：调用 store.startCalibration(baseConfig)
    // 期待结果：status.startTime 已设置；timeInfo 非 null；elapsedTime 接近 0（< 200ms）
    const store = useCalibrationStore()
    await store.startCalibration(baseConfig)
    expect(calibrationApi.startCalibration).toHaveBeenCalledTimes(1)
    expect(store.status?.startTime).toBeGreaterThan(0)
    expect(store.timeInfo).not.toBeNull()
    expect(store.timeInfo?.elapsedTime).toBeGreaterThanOrEqual(0)
    expect(store.timeInfo!.elapsedTime).toBeLessThan(1000)
  })

  it('freezes elapsedTime during paused state and resumes after resume', async () => {
    // 测试前置：用 fake timers 控制时间流逝，启动校准后推进 1s 暂停，再推进 2s 后恢复，再推进 1s
    // 测试步骤：分别在暂停瞬间、暂停 2s 后、恢复 1s 后取 elapsedTime
    // 期待结果：暂停期间 elapsed 不变；恢复后 elapsed 仅增加约 1s（不含暂停 2s）
    // 注意：只 fake setInterval/clearInterval/Date，不动 setTimeout——
    // 否则会污染并行执行的 http-client.test.ts 中对 window.setTimeout 的精确断言
    vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval', 'Date'] })
    try {
      const store = useCalibrationStore()
      await store.startCalibration(baseConfig)
      const elapsedAtStart = store.timeInfo?.elapsedTime ?? 0

      // 推进 1s 让 tick 触发一次
      vi.advanceTimersByTime(1000)
      const elapsedAfter1s = store.timeInfo?.elapsedTime ?? 0
      expect(elapsedAfter1s - elapsedAtStart).toBeGreaterThanOrEqual(900)

      // 暂停
      await store.pause()
      const elapsedAtPause = store.timeInfo?.elapsedTime ?? 0

      // 暂停期间推进 2s，tick 触发但 elapsed 不应增加
      vi.advanceTimersByTime(2000)
      const elapsedDuringPause = store.timeInfo?.elapsedTime ?? 0
      expect(elapsedDuringPause).toBe(elapsedAtPause)

      // 恢复运行后再推进 1s，elapsed 应只增加约 1s（不含暂停 2s）
      await store.resume()
      vi.advanceTimersByTime(1000)
      const elapsedAfterResume = store.timeInfo?.elapsedTime ?? 0
      const delta = elapsedAfterResume - elapsedAtPause
      // 容差 200ms
      expect(delta).toBeGreaterThanOrEqual(900)
      expect(delta).toBeLessThan(1300)
    } finally {
      vi.useRealTimers()
    }
  })

  // ============ spec Task 9: recovery / acquire-release / stop / start 清空 ============

  describe('recoveryFromBackend', () => {
    // 测试前置：mock calibrationApi.status 返回 running 状态
    // 测试步骤：调 store.recoveryFromBackend()
    // 期待结果：store.status.status === 'running'，isRunning=true，isPaused=false，isRecovering=false，recoveryError=null
    it('syncs running state from backend status', async () => {
      const store = useCalibrationStore()
      vi.mocked(calibrationApi.status).mockResolvedValueOnce({
        taskId: 'cal-running',
        type: 'five-hole',
        state: 'running',
        currentPoint: 2,
        totalPoints: 10,
        completedPoints: 1,
        progress: 10,
      } as any)

      await store.recoveryFromBackend()

      expect(store.status?.status).toBe('running')
      expect(store.isRunning).toBe(true)
      expect(store.isPaused).toBe(false)
      expect(store.isRecovering).toBe(false)
      expect(store.recoveryError).toBeNull()
      expect(store.lastRecoveryAt).toBeGreaterThan(0)
    })

    it('excludes historical pauses when recovering a running task', async () => {
      vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval', 'Date'] })
      try {
        vi.setSystemTime(new Date('2026-07-15T12:00:10.000Z'))
        const store = useCalibrationStore()
        vi.mocked(calibrationApi.status).mockResolvedValueOnce({
          taskId: 'cal-running-after-pause',
          type: 'five-hole',
          state: 'running',
          currentPoint: 2,
          totalPoints: 10,
          completedPoints: 1,
          progress: 10,
          startTime: Date.now() - 10_000,
          pausedDurationMs: 4_000,
        } as any)

        await store.recoveryFromBackend()

        expect(store.timeInfo?.elapsedTime).toBe(6_000)
        vi.advanceTimersByTime(1_000)
        expect(store.timeInfo?.elapsedTime).toBe(7_000)
      } finally {
        vi.useRealTimers()
      }
    })

    // 测试前置：mock calibrationApi.status 返回 paused 状态
    // 期待结果：status.status === 'paused'，isPaused=true
    it('syncs paused state from backend status', async () => {
      const store = useCalibrationStore()
      vi.mocked(calibrationApi.status).mockResolvedValueOnce({
        taskId: 'cal-paused',
        type: 'three-hole',
        state: 'paused',
        currentPoint: 3,
        totalPoints: 10,
        completedPoints: 3,
        progress: 30,
      } as any)

      await store.recoveryFromBackend()

      expect(store.status?.status).toBe('paused')
      expect(store.isPaused).toBe(true)
      expect(store.isRunning).toBe(false)
    })

    it('freezes elapsed time from a paused backend snapshot after cold recovery', async () => {
      vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval', 'Date'] })
      try {
        vi.setSystemTime(new Date('2026-07-15T12:00:10.000Z'))
        const store = useCalibrationStore()
        vi.mocked(calibrationApi.status).mockResolvedValueOnce({
          taskId: 'cal-cold-paused',
          type: 'five-hole',
          state: 'paused',
          currentPoint: 2,
          totalPoints: 10,
          completedPoints: 1,
          progress: 10,
          startTime: Date.now() - 10_000,
          pausedDurationMs: 4_000,
        } as any)

        await store.recoveryFromBackend()

        expect(store.timeInfo?.elapsedTime).toBe(6_000)
        vi.advanceTimersByTime(2_000)
        expect(store.timeInfo?.elapsedTime).toBe(6_000)
      } finally {
        vi.useRealTimers()
      }
    })

    // 测试前置：mock calibrationApi.status 返回 stopped 状态（spec Decision #4 / I7）
    // 期待结果：status.status === 'stopped'，与 idle 区分
    it('syncs stopped state from backend status (distinguishable from idle)', async () => {
      const store = useCalibrationStore()
      vi.mocked(calibrationApi.status).mockResolvedValueOnce({
        taskId: 'cal-stopped',
        type: 'five-hole',
        state: 'stopped',
        currentPoint: 5,
        totalPoints: 10,
        completedPoints: 5,
        progress: 50,
      } as any)

      await store.recoveryFromBackend()

      expect(store.status?.status).toBe('stopped')
      expect(store.isRunning).toBe(false)
      expect(store.isPaused).toBe(false)
    })

    // 测试前置：mock calibrationApi.status 返回 idle 状态
    // 期待结果：status.status === 'idle'，isRunning=false，isPaused=false
    it('syncs idle state from backend status', async () => {
      const store = useCalibrationStore()
      vi.mocked(calibrationApi.status).mockResolvedValueOnce({
        taskId: '',
        type: 'five-hole',
        state: 'idle',
        currentPoint: 0,
        totalPoints: 0,
        completedPoints: 0,
        progress: 0,
      } as any)

      await store.recoveryFromBackend()

      expect(store.status?.status).toBe('idle')
      expect(store.isRunning).toBe(false)
      expect(store.isPaused).toBe(false)
    })

    // 测试前置：先 startCalibration 让 store 有 running 状态 + dataPoints，再 mock status 抛错
    // 测试步骤：调 store.recoveryFromBackend()
    // 期待结果：recoveryError 非空，旧 status.dataPoints 保留（spec Recovery UX：失败不 reset）
    it('preserves existing store state when status() throws', async () => {
      const store = useCalibrationStore()
      await store.startCalibration(baseConfig)
      // 模拟旧 dataPoints：用 any 强类型断言避免联合类型字段差异（不同探针 dataPoint 结构不同）
      const oldPoint = { pointId: 'p1' } as any
      store.dataPoints = [oldPoint]

      vi.mocked(calibrationApi.status).mockRejectedValueOnce(new Error('network down'))

      await store.recoveryFromBackend()

      expect(store.recoveryError).toBe('network down')
      expect(store.isRecovering).toBe(false)
      // 旧状态保留：dataPoints 不被清空
      expect(store.dataPoints.length).toBe(1)
      expect((store.dataPoints[0] as any).pointId).toBe('p1')
    })
  })

  describe('acquireView / releaseView', () => {
    // 测试前置：store 处于 idle，无 running/paused 任务
    // 测试步骤：连续 acquireView 两次、releaseView 一次
    // 期待结果：activeViewCount 从 0→1→2→1，acquire/release 不修改 status/dataPoints
    it('increments and decrements activeViewCount without clearing session state', () => {
      const store = useCalibrationStore()
      // 预设一个 status，验证 acquire/release 不会清空它
      store.startCalibration(baseConfig)

      const initialStatus = store.status
      const initialDataPoints = store.dataPoints.length

      store.acquireView()
      expect(store.activeViewCount).toBe(1)

      store.acquireView()
      expect(store.activeViewCount).toBe(2)

      store.releaseView()
      expect(store.activeViewCount).toBe(1)

      // 状态保留：acquire/release 不动 status / dataPoints
      expect(store.status).toBe(initialStatus)
      expect(store.dataPoints.length).toBe(initialDataPoints)
    })

    // 测试前置：store.activeViewCount=0
    // 测试步骤：releaseView() 调用多次
    // 期待结果：activeViewCount 不为负（下限 0），spec Task 1 明确要求
    it('clamps activeViewCount at 0 when releaseView is called too many times', () => {
      const store = useCalibrationStore()
      store.releaseView()
      store.releaseView()
      store.releaseView()
      expect(store.activeViewCount).toBe(0)
    })
  })

  describe('stop', () => {
    // 测试前置：startCalibration 后塞入 dataPoints
    // 测试步骤：调 store.stop()
    // 期待结果：status.status === 'stopped'，dataPoints 保留（spec Decision #4 / I7）
    it('keeps dataPoints and sets status to stopped (not idle)', async () => {
      const store = useCalibrationStore()
      await store.startCalibration(baseConfig)
      store.dataPoints = [
        { pointId: 'p1' } as any,
        { pointId: 'p2' } as any,
      ]

      await store.stop()

      expect(store.status?.status).toBe('stopped')
      expect(store.isRunning).toBe(false)
      expect(store.isPaused).toBe(false)
      // 关键不变量：dataPoints 保留供导出 / 复盘
      expect(store.dataPoints.length).toBe(2)
    })
  })

  describe('startCalibration session reset', () => {
    // 测试前置：第一趟 start + stop 后 store 有 stopped 状态 + dataPoints + completeEvent
    // 测试步骤：第二趟 startCalibration
    // 期待结果：旧 dataPoints 清空、completeEvent=null、status 重新初始化为 running
    it('clears previous session dataPoints and completeEvent on new start', async () => {
      const store = useCalibrationStore()
      // 第一趟
      await store.startCalibration(baseConfig)
      store.dataPoints = [{ pointId: 'old' } as any]
      await store.stop()
      // stop 后 status='stopped'，dataPoints 保留
      expect(store.dataPoints.length).toBe(1)

      // 第二趟：resetSession 应清旧会话
      await store.startCalibration(baseConfig)

      expect(store.dataPoints.length).toBe(0)
      expect(store.completeEvent).toBeNull()
      expect(store.status?.status).toBe('running')
      expect(store.isRunning).toBe(true)
    })
  })
})

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
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

  // ============ spec Task 15: store 只映射后端 physics，页面保持纯展示 ============
  //
  // 验收标准：
  //   1. 无 atmospheric 常量/公式（calibrationStore 中无 ATM_GAMMA/ATM_C_COEFF/calculateAtmosphericPhysics）
  //   2. missing 显示 "--"：backend livePhysics 缺失 → calculatedPhysics=null
  //   3. zero 显示格式化 0：backend livePhysics={machNumber:0, velocity:0} → 透传（零不被 truthiness 丢失）
  //   4. raw pressure 更新不触发公式：updateRealtimePressures 后 calculatedPhysics 仍为初始值
  //   5. 正常值映射：backend livePhysics={machNumber:0.3, velocity:102.5} → 透传
  //   6. 字段三态：仅 machNumber 提供时 velocity=undefined（UI 显示 "--"）
  //   7. 七孔使用同一 store contract：livePhysics 映射与校准类型无关，无需算法改动
  describe('Task 15: backend-driven live physics mapping', () => {
    // 测试前置：构造实时压力（Patm>0、P0/Ps/Ttunnel 齐全）—— 旧版本会触发本地公式计算
    // 测试步骤：调用 store.updateRealtimePressures
    // 期待结果：calculatedPhysics 仍为 null（raw pressure 更新不再触发任何公式）
    //           spec Task 15 验收：raw pressure 更新不触发公式
    it('does not calculate physics on raw pressure update (formula deleted)', () => {
      const store = useCalibrationStore()

      store.updateRealtimePressures({
        P1: 0,
        P2: 0,
        P3: 0,
        P4: 0,
        P5: 0,
        Patm: 101325,
        Tatm: 25,
        P0: 1000,
        Ps: 0,
        Ttunnel: 25,
      })

      // 旧版本会返回 { machNumber: ..., velocity: ... }，Task 15 后必须为 null
      expect(store.calculatedPhysics).toBeNull()
    })

    // 测试前置：mock status 返回无 livePhysics 字段的 running 状态
    // 测试步骤：调 store.recoveryFromBackend()（内部调 updateStatusFromBackend）
    // 期待结果：calculatedPhysics=null（missing 显示 "--"）
    it('maps missing livePhysics to null (UI shows "--")', async () => {
      const store = useCalibrationStore()
      vi.mocked(calibrationApi.status).mockResolvedValueOnce({
        taskId: 'cal-missing',
        type: 'five-hole',
        state: 'running',
        currentPoint: 0,
        totalPoints: 10,
        completedPoints: 0,
        progress: 0,
        // livePhysics 字段缺失
      } as any)

      await store.recoveryFromBackend()

      expect(store.calculatedPhysics).toBeNull()
    })

    // 测试前置：mock status 返回 livePhysics={machNumber:0, velocity:0}（Pt==Ps 等压有效零）
    // 测试步骤：调 store.recoveryFromBackend()
    // 期待结果：calculatedPhysics={machNumber:0, velocity:0}（零是有效零，不被 truthiness 丢失）
    //           spec Task 15 验收：zero 显示格式化 0
    it('maps zero livePhysics (Pt==Ps valid zero) without truthiness loss', async () => {
      const store = useCalibrationStore()
      vi.mocked(calibrationApi.status).mockResolvedValueOnce({
        taskId: 'cal-zero',
        type: 'five-hole',
        state: 'running',
        currentPoint: 1,
        totalPoints: 10,
        completedPoints: 0,
        progress: 0,
        livePhysics: { machNumber: 0, velocity: 0 },
      } as any)

      await store.recoveryFromBackend()

      // 关键：全零对象是 truthy，映射逻辑必须基于"对象存在性"而非 truthiness
      expect(store.calculatedPhysics).toEqual({ machNumber: 0, velocity: 0 })
      // 字段三态：0 是有效零，区别于 undefined（missing）
      expect(store.calculatedPhysics?.machNumber).toBe(0)
      expect(store.calculatedPhysics?.velocity).toBe(0)
    })

    // 测试前置：mock status 返回 livePhysics={machNumber:0.3, velocity:102.5}
    // 测试步骤：调 store.recoveryFromBackend()
    // 期待结果：calculatedPhysics={machNumber:0.3, velocity:102.5}（正常值透传）
    it('maps normal livePhysics values from backend', async () => {
      const store = useCalibrationStore()
      vi.mocked(calibrationApi.status).mockResolvedValueOnce({
        taskId: 'cal-normal',
        type: 'five-hole',
        state: 'running',
        currentPoint: 2,
        totalPoints: 10,
        completedPoints: 1,
        progress: 10,
        livePhysics: { machNumber: 0.3, velocity: 102.5 },
      } as any)

      await store.recoveryFromBackend()

      expect(store.calculatedPhysics).toEqual({ machNumber: 0.3, velocity: 102.5 })
    })

    // 测试前置：mock status 返回 livePhysics={machNumber:0.3}（velocity 缺失）
    // 测试步骤：调 store.recoveryFromBackend()
    // 期待结果：calculatedPhysics={machNumber:0.3, velocity:undefined}
    //           字段三态：machNumber 显示 "0.300"，velocity 显示 "--"（UI 用 !== undefined 判断）
    it('preserves field-level three-state semantics (machNumber only, velocity missing)', async () => {
      const store = useCalibrationStore()
      vi.mocked(calibrationApi.status).mockResolvedValueOnce({
        taskId: 'cal-partial',
        type: 'three-hole',
        state: 'running',
        currentPoint: 1,
        totalPoints: 10,
        completedPoints: 0,
        progress: 0,
        livePhysics: { machNumber: 0.3 },
      } as any)

      await store.recoveryFromBackend()

      // machNumber 透传，velocity 保持 undefined（不存在的字段）
      expect(store.calculatedPhysics?.machNumber).toBe(0.3)
      expect(store.calculatedPhysics?.velocity).toBeUndefined()
    })

    // 测试前置：mock status 返回 seven-hole 类型 + livePhysics（验证七孔走同一映射路径）
    // 测试步骤：调 store.recoveryFromBackend()
    // 期待结果：calculatedPhysics 透传——spec Task 15 验收：七孔使用同一 store contract，无需算法改动
    it('uses same store contract for seven-hole (no algorithm-specific handling)', async () => {
      const store = useCalibrationStore()
      vi.mocked(calibrationApi.status).mockResolvedValueOnce({
        taskId: 'cal-seven',
        type: 'seven-hole',
        state: 'running',
        currentPoint: 5,
        totalPoints: 169,
        completedPoints: 4,
        progress: 2,
        livePhysics: { machNumber: 0.45, velocity: 153.2 },
      } as any)

      await store.recoveryFromBackend()

      // 七孔与五孔/三孔/总压走同一 updateStatusFromBackend 映射路径，无类型分支
      expect(store.calculatedPhysics).toEqual({ machNumber: 0.45, velocity: 153.2 })
    })

    // 测试前置：先 mock status 返回 livePhysics={machNumber:0.3}，再 mock 返回无 livePhysics
    // 测试步骤：两次 recoveryFromBackend
    // 期待结果：第二次后 calculatedPhysics=null（终态/任务切换时 stale physics 被清空）
    it('clears calculatedPhysics when backend stops sending livePhysics (stale clearing)', async () => {
      const store = useCalibrationStore()

      // 第一次：running 态，有 livePhysics
      vi.mocked(calibrationApi.status).mockResolvedValueOnce({
        taskId: 'cal-stale',
        type: 'five-hole',
        state: 'running',
        currentPoint: 1,
        totalPoints: 10,
        completedPoints: 0,
        progress: 0,
        livePhysics: { machNumber: 0.3, velocity: 102.5 },
      } as any)
      await store.recoveryFromBackend()
      expect(store.calculatedPhysics?.machNumber).toBe(0.3)

      // 第二次：stopped 终态，后端已 StaleClearing（livePhysics=nil）
      vi.mocked(calibrationApi.status).mockResolvedValueOnce({
        taskId: 'cal-stale',
        type: 'five-hole',
        state: 'stopped',
        currentPoint: 5,
        totalPoints: 10,
        completedPoints: 5,
        progress: 50,
        // livePhysics 字段缺失——后端 spec Task 13 StaleClearing
      } as any)
      await store.recoveryFromBackend()

      // 终态后 stale physics 被清空，UI 显示 "--"
      expect(store.calculatedPhysics).toBeNull()
    })
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

  // ============ spec Task 14: 统一 HTTP/Wails status polling ============
  //
  // 验收标准：
  //   1. HTTP/Wails 均轮询既有 status
  //   2. 请求不重叠（inFlight 保护）
  //   3. 终态停止 polling
  //   4. 错误不产生 synthetic success（pause/resume/stop 失败时抛错）
  //   5. zero 不被 truthiness 丢失（全零 status 仍被处理）
  //
  // 测试环境：mock isWailsAvailable=false，所有测试走 HTTP 路径。
  describe('Task 14: unified HTTP/Wails status polling', () => {
    // polling 测试可能用 mockResolvedValue 持久覆盖 status，afterEach 恢复默认 idle
    afterEach(() => {
      vi.mocked(calibrationApi.status).mockResolvedValue({
        taskId: 'test',
        state: 'idle',
        currentPoint: 0,
        totalPoints: 0,
      } as any)
    })

    // 测试前置：HTTP 模式（isWailsAvailable=false），startCalibration 后启动 1Hz polling
    // 测试步骤：advanceTimersByTime(1000) 触发 polling 周期
    // 期待结果：calibrationApi.status 被调用（HTTP 模式也轮询，spec Task 14 核心验收）
    it('polls calibrationApi.status in HTTP mode after startCalibration', async () => {
      vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval', 'Date'] })
      try {
        const store = useCalibrationStore()
        // 持久返回 running，避免 idle 转态干扰 call count 验证
        vi.mocked(calibrationApi.status).mockResolvedValue({
          taskId: 'cal-test',
          type: 'five-hole',
          state: 'running',
          currentPoint: 0,
          totalPoints: 10,
          completedPoints: 0,
          progress: 0,
        } as any)

        await store.startCalibration(baseConfig)
        vi.mocked(calibrationApi.status).mockClear()

        // activeViewCount=0 时默认 1Hz polling
        vi.advanceTimersByTime(1000)

        // 核心：HTTP 模式 polling 已触发 calibrationApi.status
        expect(calibrationApi.status).toHaveBeenCalledTimes(1)
      } finally {
        vi.useRealTimers()
      }
    })

    // 测试前置：mock status 返回不立即 resolve 的 promise（模拟慢响应）
    // 测试步骤：连续 advanceTimersByTime 两次，然后 resolve，再 advance 一次
    // 期待结果：第二次 polling 被 inFlight 跳过；resolve 后第三次正常调用
    it('skips overlapping status requests when previous is still in flight', async () => {
      vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval', 'Date'] })
      try {
        const store = useCalibrationStore()
        // controllable promise 模拟慢响应
        // 用对象包装避免 TypeScript CFA 将 resolveStatus 窄化为 null（闭包内赋值不被 CFA 追踪）
        const statusController: { resolve: ((v: any) => void) | null } = { resolve: null }
        vi.mocked(calibrationApi.status).mockImplementation(
          () => new Promise(r => { statusController.resolve = r }),
        )

        await store.startCalibration(baseConfig)
        vi.mocked(calibrationApi.status).mockClear()

        // 第一次 polling：调用 status，inFlight=true，promise pending
        vi.advanceTimersByTime(1000)
        expect(calibrationApi.status).toHaveBeenCalledTimes(1)

        // 第二次 polling：inFlight=true，跳过——spec Task 14 "请求不重叠" 验收
        vi.advanceTimersByTime(1000)
        expect(calibrationApi.status).toHaveBeenCalledTimes(1)

        // resolve 后 inFlight=false
        statusController.resolve?.({ state: 'running' } as any)
        await Promise.resolve()
        await Promise.resolve()

        // 第三次 polling：inFlight=false，调用 status
        vi.advanceTimersByTime(1000)
        expect(calibrationApi.status).toHaveBeenCalledTimes(2)
      } finally {
        vi.useRealTimers()
      }
    })

    // 测试前置：startCalibration 后 mock status 返回 stopped（终态）
    // 测试步骤：advanceTimersByTime 触发 polling，flush microtask 让 updateStatusFromBackend 执行，
    //           再 advance 一次验证 polling 已停
    // 期待结果：终态后 stopStatusPolling 生效，status 不再被调用
    it('stops polling when backend returns terminal state', async () => {
      vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval', 'Date'] })
      try {
        const store = useCalibrationStore()
        vi.mocked(calibrationApi.status).mockResolvedValue({
          taskId: 'cal-test',
          type: 'five-hole',
          state: 'stopped',
          currentPoint: 5,
          totalPoints: 10,
          completedPoints: 5,
          progress: 50,
        } as any)

        await store.startCalibration(baseConfig)
        vi.mocked(calibrationApi.status).mockClear()

        // 第一次 polling：返回 stopped，触发 stopStatusPolling
        vi.advanceTimersByTime(1000)
        // flush microtask 让 promise resolve + updateStatusFromBackend 执行
        await Promise.resolve()
        await Promise.resolve()
        await Promise.resolve()

        // 终态已生效
        expect(store.status?.status).toBe('stopped')

        // 再推进 3s，polling 应已停止
        vi.mocked(calibrationApi.status).mockClear()
        vi.advanceTimersByTime(3000)
        expect(calibrationApi.status).not.toHaveBeenCalled()
      } finally {
        vi.useRealTimers()
      }
    })

    // 测试前置：mock pauseCalibration 返回 { success: false, error: 'backend error' }
    // 测试步骤：调 store.pause()
    // 期待结果：store.pause() 抛错——不产生 synthetic success（spec Task 14 验收）
    it('pause throws on backend failure (no synthetic success)', async () => {
      const store = useCalibrationStore()
      await store.startCalibration(baseConfig)
      vi.mocked(calibrationApi.pauseCalibration).mockResolvedValueOnce({
        success: false,
        error: 'backend pause error',
      })

      await expect(store.pause()).rejects.toThrow('backend pause error')
    })

    // 测试前置：mock resumeCalibration 返回 { success: false, error: 'backend error' }
    // 测试步骤：调 store.resume()
    // 期待结果：store.resume() 抛错——不产生 synthetic success
    it('resume throws on backend failure (no synthetic success)', async () => {
      const store = useCalibrationStore()
      await store.startCalibration(baseConfig)
      await store.pause()
      vi.mocked(calibrationApi.resumeCalibration).mockResolvedValueOnce({
        success: false,
        error: 'backend resume error',
      })

      await expect(store.resume()).rejects.toThrow('backend resume error')
    })

    // 测试前置：mock stopCalibration 返回 { success: false, error: 'backend error' }
    // 测试步骤：调 store.stop()
    // 期待结果：store.stop() 抛错——不产生 synthetic success
    it('stop throws on backend failure (no synthetic success)', async () => {
      const store = useCalibrationStore()
      await store.startCalibration(baseConfig)
      vi.mocked(calibrationApi.stopCalibration).mockResolvedValueOnce({
        success: false,
        error: 'backend stop error',
      })

      await expect(store.stop()).rejects.toThrow('backend stop error')
    })

    // 测试前置：mock status 返回全零值的 idle 状态（taskId='', currentPoint=0, totalPoints=0）
    // 测试步骤：advanceTimersByTime 触发 polling，flush microtask
    // 期待结果：updateStatusFromBackend 被调用（if (calStatus) 检查对象存在性，非 truthiness）
    //           store.status 更新为 idle——spec Task 14 "zero 不被 truthiness 丢失" 验收
    it('processes status with all-zero values (not lost to truthiness)', async () => {
      vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval', 'Date'] })
      try {
        const store = useCalibrationStore()
        // 全零 idle 状态——if (calStatus) 必须基于对象存在性，不能因零值跳过
        vi.mocked(calibrationApi.status).mockResolvedValue({
          taskId: '',
          type: 'five-hole',
          state: 'idle',
          currentPoint: 0,
          totalPoints: 0,
          completedPoints: 0,
          progress: 0,
        } as any)

        await store.startCalibration(baseConfig)
        vi.mocked(calibrationApi.status).mockClear()

        // polling 触发——status 被调用说明没有因全零跳过
        vi.advanceTimersByTime(1000)
        expect(calibrationApi.status).toHaveBeenCalledTimes(1)

        // flush microtask 让 updateStatusFromBackend 执行
        await Promise.resolve()
        await Promise.resolve()
        await Promise.resolve()

        // store 被更新为 idle——证明全零 status 被正常处理
        expect(store.status?.status).toBe('idle')
        expect(store.isRunning).toBe(false)
      } finally {
        vi.useRealTimers()
      }
    })

    // 测试前置：recoveryFromBackend 同步 running 状态（不启动 polling），然后调 acquireView
    // 测试步骤：acquireView 后 advanceTimersByTime(200)（5Hz = 200ms）
    // 期待结果：calibrationApi.status 被调用——HTTP 模式 acquireView 也启动 polling
    //           （旧代码 if (isWailsAvailable()) 守卫导致 HTTP 模式不启动）
    it('acquireView starts high-frequency polling in HTTP mode when running', async () => {
      vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval', 'Date'] })
      try {
        const store = useCalibrationStore()
        vi.mocked(calibrationApi.status).mockResolvedValue({
          taskId: 'cal-running',
          type: 'five-hole',
          state: 'running',
          currentPoint: 1,
          totalPoints: 10,
          completedPoints: 0,
          progress: 0,
          startTime: Date.now(),
        } as any)

        // recoveryFromBackend 同步 running 状态但不启动 polling
        await store.recoveryFromBackend()
        expect(store.isRunning).toBe(true)
        vi.mocked(calibrationApi.status).mockClear()

        // acquireView：0→1，应启动高频 polling（5Hz = 200ms）
        store.acquireView()
        vi.advanceTimersByTime(200)

        expect(calibrationApi.status).toHaveBeenCalledTimes(1)
      } finally {
        vi.useRealTimers()
      }
    })

    // 测试前置：recoveryFromBackend 同步 running + acquireView 后 releaseView
    // 测试步骤：releaseView 后 advanceTimersByTime(1000)
    // 期待结果：1Hz 心跳 polling 仍在运行——HTTP 模式不因 releaseView 停止 polling
    //           （旧代码 && isWailsAvailable() 守卫导致 HTTP 模式 releaseView 后无心跳）
    it('releaseView keeps 1Hz heartbeat polling in HTTP mode when running', async () => {
      vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval', 'Date'] })
      try {
        const store = useCalibrationStore()
        vi.mocked(calibrationApi.status).mockResolvedValue({
          taskId: 'cal-running',
          type: 'five-hole',
          state: 'running',
          currentPoint: 1,
          totalPoints: 10,
          completedPoints: 0,
          progress: 0,
          startTime: Date.now(),
        } as any)

        await store.recoveryFromBackend()
        store.acquireView() // 0→1，高频 polling
        store.releaseView() // 1→0，应降到 1Hz 心跳

        vi.mocked(calibrationApi.status).mockClear()
        vi.advanceTimersByTime(1000) // 1Hz 心跳

        expect(calibrationApi.status).toHaveBeenCalledTimes(1)
      } finally {
        vi.useRealTimers()
      }
    })

    // 测试前置：recoveryFromBackend 同步 running 状态（不启动 polling）
    // 测试步骤：调 restartPollingForCurrentState，advanceTimersByTime(1000)
    // 期待结果：calibrationApi.status 被调用——HTTP 模式也重启 polling
    //           （旧代码 if (!isWailsAvailable()) return 导致 HTTP 模式永不重启）
    it('restartPollingForCurrentState restarts polling in HTTP mode', async () => {
      vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval', 'Date'] })
      try {
        const store = useCalibrationStore()
        vi.mocked(calibrationApi.status).mockResolvedValue({
          taskId: 'cal-running',
          type: 'five-hole',
          state: 'running',
          currentPoint: 1,
          totalPoints: 10,
          completedPoints: 0,
          progress: 0,
          startTime: Date.now(),
        } as any)

        // recoveryFromBackend 同步 running 状态但不启动 polling
        await store.recoveryFromBackend()
        expect(store.isRunning).toBe(true)
        vi.mocked(calibrationApi.status).mockClear()

        // restartPollingForCurrentState：HTTP 模式也应启动 polling
        store.restartPollingForCurrentState()
        vi.advanceTimersByTime(1000)

        expect(calibrationApi.status).toHaveBeenCalledTimes(1)
      } finally {
        vi.useRealTimers()
      }
    })

    // 测试前置：startCalibration 后调 stop()，mock status 返回 stopped（终态快照）
    // 测试步骤：stop 后 advanceTimersByTime 触发 1Hz polling 捕获终态回包
    // 期待结果：HTTP 模式 stop 后 polling 继续运行——捕获后端最终 stopped 快照
    //           （旧代码 if (isWailsAvailable()) 守卫导致 HTTP 模式 stop 后不轮询，
    //            UI 卡在本地 stopped 状态无法刷新后端最终 dataPoints/lastError）
    it('stop starts 1Hz polling in HTTP mode to capture backend terminal snapshot', async () => {
      vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval', 'Date'] })
      try {
        const store = useCalibrationStore()
        vi.mocked(calibrationApi.status).mockResolvedValue({
          taskId: 'cal-test',
          type: 'five-hole',
          state: 'stopped',
          currentPoint: 5,
          totalPoints: 10,
          completedPoints: 5,
          progress: 50,
        } as any)

        await store.startCalibration(baseConfig)
        vi.mocked(calibrationApi.status).mockClear()

        await store.stop()

        // stop 应启动 1Hz polling 等终态回包
        vi.mocked(calibrationApi.status).mockClear()
        vi.advanceTimersByTime(1000)

        // HTTP 模式 stop 后 polling 仍在运行
        expect(calibrationApi.status).toHaveBeenCalledTimes(1)
      } finally {
        vi.useRealTimers()
      }
    })
  })
})

import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

// Mock wails-adapter：让 isWailsAvailable 返回 false，store.startCalibration 会走 calibrationApi 分支
vi.mock('@api/wails-adapter', () => ({
  isWailsAvailable: () => false,
  wailsApi: {},
}))

// Mock calibrationApi：startCalibration 直接返回 success，不发 HTTP 请求
vi.mock('@api/calibrationApi', () => ({
  calibrationApi: {
    startCalibration: vi.fn(async () => ({ success: true })),
    pauseCalibration: vi.fn(async () => ({ success: true })),
    resumeCalibration: vi.fn(async () => ({ success: true })),
    stopCalibration: vi.fn(async () => ({ success: true })),
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
})

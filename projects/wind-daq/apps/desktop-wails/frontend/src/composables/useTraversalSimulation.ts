// DEMO ONLY — not for production use
/**
 * 遍历测试模拟模式 composable（从 Cursor DAQ 移植）
 *
 * 在无硬件环境下，通过纯前端定时器与模拟数据驱动 traversalStore 状态机，
 * 复现遍历测试的 moving/stabilizing/acquiring/saving 四阶段流程。
 *
 * 移植来源：Cursor DAQ src/renderer/src/composables/useTraversalSimulation.ts
 * 适配点：
 *   - 导入路径由 @renderer/* 改为 wind-daq 的别名 (@stores/*, @utils/*)
 *   - buildSimStatus 补充 wind-daq TraversalTestStatus 必需的 state 字段
 */

import { ref, onUnmounted } from 'vue'
import { useTraversalStore } from '@stores/traversalStore'
import {
  generateSimulatedTraversalPoints,
  DEFAULT_SIMULATION_CONFIG,
  type TraversalSimulationConfig
} from '@utils/simulateTraversalRun'
import type {
  TraversalTestConfig,
  TraversalTestStatus,
  TraversalPointPhase,
  TraversalCompleteEvent
} from '@shared/types/traversal'

/** 各阶段模拟延迟（毫秒） */
export interface SimulationPhaseDelays {
  /** 移动到目标点 */
  moving: number
  /** 稳定等待 */
  stabilizing: number
  /** 采集数据 */
  acquiring: number
  /** 保存数据 */
  saving: number
}

/** 默认阶段延迟，与 Cursor DAQ 保持一致 */
export const DEFAULT_PHASE_DELAYS: SimulationPhaseDelays = {
  moving: 1300,
  stabilizing: 1800,
  acquiring: 1500,
  saving: 500
}

/** 实时压力刷新间隔（毫秒） */
const SIMULATION_REFRESH_INTERVAL_MS = 120

/**
 * 遍历测试模拟模式 composable
 *
 * 提供：
 *   - runSimulation: 启动一次模拟遍历测试
 *   - cancelSimulation: 取消正在进行的模拟
 *   - isSimulating: 当前是否处于模拟中
 */
export function useTraversalSimulation() {
  const store = useTraversalStore()
  const abortController = ref<AbortController | null>(null)
  const isSimulating = ref(false)

  let originalConfig: TraversalTestConfig | null = null

  /** 恢复原始配置：模拟结束后将 store 的 config/isSimulation 还原 */
  function restoreOriginalConfig(): void {
    store.config = originalConfig
    store.isSimulation = false
    originalConfig = null
  }

  /** 构造一份用于模拟模式的 TraversalTestConfig（16×16 矩形布点） */
  function buildSimulationConfig(): TraversalTestConfig {
    return {
      name: 'Simulation 16×16',
      channels: {
        motionAxes: [
          { name: 'X', controllerId: 'sim', axis: 'X' },
          { name: 'Y', controllerId: 'sim', axis: 'Y' }
        ],
        probeChannels: []
      },
      layout: {
        pattern: 'rectangle',
        snakeOrder: true,
        rectangle: {
          xMin: DEFAULT_SIMULATION_CONFIG.alphaRange[0],
          xMax: DEFAULT_SIMULATION_CONFIG.alphaRange[1],
          xStepSegments: [{ start: DEFAULT_SIMULATION_CONFIG.alphaRange[0], end: DEFAULT_SIMULATION_CONFIG.alphaRange[1], step: 6 }],
          yMin: DEFAULT_SIMULATION_CONFIG.betaRange[0],
          yMax: DEFAULT_SIMULATION_CONFIG.betaRange[1],
          yStepSegments: [{ start: DEFAULT_SIMULATION_CONFIG.betaRange[0], end: DEFAULT_SIMULATION_CONFIG.betaRange[1], step: 6 }]
        }
      },
      prbFile: null,
      interpolationAlgorithm: 'old',
      dwellTimeMs: 1000,
      samplesPerPoint: 10,
      savePath: '',
      saveFileName: 'simulation',
      saveOptions: {
        savePointId: false,
        saveTimestamp: false,
        saveRawPressure: false,
        saveCalculatedResult: false
      }
    }
  }

  /**
   * 构造模拟状态对象
   * @param status 终端状态或运行中
   * @param completedPoints 已完成点数
   * @param totalPoints 总点数
   * @param currentPoint 当前点坐标
   * @param currentPointPhase 当前点阶段
   */
  function buildSimStatus(
    status: 'running' | 'completed' | 'stopped',
    completedPoints: number,
    totalPoints: number,
    currentPoint?: { alpha: number; beta: number },
    currentPointPhase?: TraversalPointPhase
  ): TraversalTestStatus {
    return {
      taskId: 'sim',
      // wind-daq 的 TraversalTestStatus 需要 state 字段记录后端原始状态，模拟模式下与 status 同步
      state: status,
      status,
      totalPoints,
      completedPoints,
      currentPoint,
      currentPointPhase,
      progress: totalPoints > 0 ? (completedPoints / totalPoints) * 100 : 0,
      startTime: Date.now()
    }
  }

  /** 可被 AbortSignal 取消的延迟函数 */
  function delay(ms: number, signal?: AbortSignal): Promise<void> {
    return new Promise((resolve, reject) => {
      if (signal?.aborted) {
        resolve()
        return
      }
      const timer = setTimeout(resolve, ms)
      const onAbort = () => {
        clearTimeout(timer)
        resolve()
      }
      signal?.addEventListener('abort', onAbort, { once: true })
    })
  }

  /**
   * 在指定时长内周期性刷新实时压力（带微量噪声），模拟采集过程中的压力波动
   * @param basePressures 基准压力
   * @param durationMs 持续时长
   * @param signal 取消信号
   * @param finalize 结束时的回调（如锁定最终压力值）
   */
  async function animateRealtimePressures(
    basePressures: NonNullable<typeof store.realtimePressures>,
    durationMs: number,
    signal?: AbortSignal,
    finalize?: () => void
  ): Promise<void> {
    const startedAt = Date.now()

    while (!signal?.aborted) {
      store.realtimePressures = {
        ...basePressures,
        P1: basePressures.P1 + (Math.random() - 0.5) * 0.5,
        P2: basePressures.P2 + (Math.random() - 0.5) * 0.5,
        P3: basePressures.P3 + (Math.random() - 0.5) * 0.5,
        P4: basePressures.P4 + (Math.random() - 0.5) * 0.5,
        P5: basePressures.P5 + (Math.random() - 0.5) * 0.5,
        Patm: basePressures.Patm + (Math.random() - 0.5) * 0.1,
        Tatm: basePressures.Tatm + (Math.random() - 0.5) * 0.05
      }

      const elapsed = Date.now() - startedAt
      const remaining = durationMs - elapsed
      if (remaining <= 0) {
        break
      }

      await delay(Math.min(SIMULATION_REFRESH_INTERVAL_MS, remaining), signal)
    }

    if (!signal?.aborted && finalize) {
      finalize()
    }
  }

  /**
   * 运行一次完整的模拟遍历测试
   * @param simConfig 模拟配置覆盖项
   * @param phaseDelays 各阶段延迟覆盖项
   */
  async function runSimulation(
    simConfig?: Partial<TraversalSimulationConfig>,
    phaseDelays?: Partial<SimulationPhaseDelays>
  ): Promise<void> {
    if (isSimulating.value) return

    const delays: SimulationPhaseDelays = { ...DEFAULT_PHASE_DELAYS, ...phaseDelays }
    const config: TraversalSimulationConfig = { ...DEFAULT_SIMULATION_CONFIG, ...simConfig }

    const points = generateSimulatedTraversalPoints(config)
    const totalPoints = points.length

    isSimulating.value = true
    abortController.value = new AbortController()
    const signal = abortController.value.signal

    // 备份原始配置，模拟结束后恢复
    originalConfig = store.config
    store.isSimulation = true
    store.config = buildSimulationConfig()
    store.dataPoints = []
    store.completeEvent = null
    store.error = null
    store.realtimePressures = null
    store.realtimeResult = null
    store.status = buildSimStatus('running', 0, totalPoints)

    try {
      for (let i = 0; i < totalPoints; i++) {
        if (signal.aborted) break

        const point = points[i]
        const currentCoord = { alpha: point.coordinates.alpha, beta: point.coordinates.beta }

        // 阶段 1: 移动
        store.status = buildSimStatus('running', i, totalPoints, currentCoord, 'moving')
        await delay(delays.moving, signal)
        if (signal.aborted) break

        // 阶段 2: 稳定（实时压力波动）
        store.status = buildSimStatus('running', i, totalPoints, currentCoord, 'stabilizing')
        await animateRealtimePressures(point.rawPressure, delays.stabilizing, signal)
        if (signal.aborted) break

        // 阶段 3: 采集（实时压力波动 + 锁定最终压力）
        store.status = buildSimStatus('running', i, totalPoints, currentCoord, 'acquiring')
        store.realtimeResult = point.interpolationResult
        await animateRealtimePressures(point.rawPressure, delays.acquiring, signal, () => {
          store.realtimePressures = point.rawPressure
        })
        if (signal.aborted) break

        // 阶段 4: 保存
        store.status = buildSimStatus('running', i, totalPoints, currentCoord, 'saving')
        await delay(delays.saving, signal)
        if (signal.aborted) break

        // 将该点加入数据点列表
        store.dataPoints.push(point)
      }

      // 终态：被取消或正常完成
      if (signal.aborted) {
        store.status = buildSimStatus('stopped', store.dataPoints.length, totalPoints)
      } else {
        store.status = buildSimStatus('completed', totalPoints, totalPoints)
        store.realtimePressures = null
        store.realtimeResult = null
      }
    } finally {
      restoreOriginalConfig()
      isSimulating.value = false
      abortController.value = null
    }
  }

  /** 取消正在进行的模拟 */
  function cancelSimulation(): void {
    if (abortController.value) {
      abortController.value.abort()
    }
  }

  // 组件卸载时自动取消模拟，避免悬挂的异步任务
  onUnmounted(() => {
    cancelSimulation()
  })

  return {
    runSimulation,
    cancelSimulation,
    isSimulating
  }
}

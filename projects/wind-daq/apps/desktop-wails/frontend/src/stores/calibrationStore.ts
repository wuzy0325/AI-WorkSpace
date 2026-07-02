import { defineStore } from 'pinia'
import { ref, computed, watch } from 'vue'
import type {
  CalibrationConfig,
  CalibrationType,
  CalibrationTaskStatus,
  CalibrationCompleteEvent,
  CalibrationDataPoint,
  CalibrationAnyDataPoint,
  FiveHoleCoefficients,
} from '@shared/types/calibration'
import { calibrationApi } from '@api/calibrationApi'
import { wailsApi, isWailsAvailable } from '@api/wails-adapter'

export interface RealtimePressures {
  P1: number
  P2: number
  P3: number
  P4: number
  P5: number
  Patm: number
  Tatm: number
  P0?: number
  Ps?: number
  Ttunnel?: number
}

export interface CalculatedPhysics {
  machNumber?: number
  velocity?: number
}

// 大气数据计算常数（与后端 AtmosphericDataCalculator 保持一致）
const ATM_GAMMA = 1.4       // 空气绝热指数
const ATM_C_COEFF = 20.047  // 声速计算系数
const ATM_RECOVERY = 0.9    // 温度传感器恢复系数
const ATM_STANDARD_PRESSURE_PA = 101325

/**
 * 根据实时压力计算气动参数（马赫数、流速）。
 * 公式与后端 AtmosphericDataCalculator / formulas.go 一致，用于 UI 实时显示，非校准算法。
 *
 * 关键：风洞总压/静压通道通常以大气压为参考点输出差压（表压），
 *   后端公式约定 Pt_abs = P0 + Patm, Ps_abs = Ps + Patm。
 *   若 Patm 缺失或为 0，实时 UI 使用标准大气压兜底，避免差压通道导致整块空白。
 *
 * 马赫数: Ma = sqrt((2/(γ-1)) * ((Pt_abs/Ps_abs)^((γ-1)/γ) - 1))
 * 静温:   SAT = TAT / (1 + 0.2 * r * Ma^2)   （TAT 取风洞温度，需转开尔文）
 * 流速:   V = Ma * 20.047 * sqrt(SAT)
 *
 * 当 P0/Ps/Ttunnel 任一缺失或非法时返回 null（UI 显示 "--"）。
 */
function calculateAtmosphericPhysics(p: RealtimePressures): CalculatedPhysics | null {
  const ptGauge = p.P0
  const psGauge = p.Ps
  // 风洞温度通道单位为 ℃，需转换为开尔文
  const tatK = p.Ttunnel === undefined ? undefined : p.Ttunnel + 273.15
  // 大气压，用于差压转绝对压；实时 UI 中通道未映射时用标准大气压兜底。
  const patm = Number.isFinite(p.Patm) && p.Patm > 0 ? p.Patm : ATM_STANDARD_PRESSURE_PA

  if (ptGauge === undefined || psGauge === undefined || tatK === undefined) return null
  if (!Number.isFinite(ptGauge) || !Number.isFinite(psGauge) || !Number.isFinite(tatK)) return null

  const ptAbs = ptGauge + patm
  const psAbs = psGauge + patm

  if (psAbs <= 0 || ptAbs < psAbs || tatK <= 0) return null
  if (ptAbs === psAbs) return { machNumber: 0, velocity: 0 }

  const ratio = ptAbs / psAbs
  const ma = Math.sqrt((2 / (ATM_GAMMA - 1)) * (Math.pow(ratio, (ATM_GAMMA - 1) / ATM_GAMMA) - 1))
  if (!Number.isFinite(ma) || ma < 0) return null

  const sat = tatK / (1 + ((ATM_GAMMA - 1) / 2) * ATM_RECOVERY * ma * ma)
  if (!Number.isFinite(sat) || sat <= 0) return null

  const velocity = ma * ATM_C_COEFF * Math.sqrt(sat)
  if (!Number.isFinite(velocity)) return null

  return { machNumber: ma, velocity }
}

export interface TimeInfo {
  elapsedTime: number
  estimatedRemaining: number
}

export const useCalibrationStore = defineStore('calibration', () => {
  const status = ref<CalibrationTaskStatus | null>(null)
  const isRunning = ref(false)
  const isPaused = ref(false)
  const completeEvent = ref<CalibrationCompleteEvent | null>(null)
  const dataPoints = ref<CalibrationAnyDataPoint[]>([])
  const realtimePressures = ref<RealtimePressures | null>(null)
  const calculatedPhysics = ref<CalculatedPhysics | null>(null)
  const timeInfo = ref<TimeInfo | null>(null)
  // 默认 5Hz：兼顾实时性与性能。CalibrationConfig.uiRefreshHz 在 startCalibration 中同步覆盖。
  const uiRefreshHz = ref(5)
  const uiRefreshIntervalMs = computed(() => 1000 / uiRefreshHz.value)

  // 状态轮询定时器：必须放在 store 内部（而非模块级），
  // 否则多实例 / HMR 重载时旧 timer 不会随 store dispose 而清理，导致泄漏与重复轮询。
  let statusPollingTimer: ReturnType<typeof setInterval> | null = null

  // 压力数据节流控制
  let lastPressureUpdateAt = 0
  let pendingPressureUpdate: RealtimePressures | null = null
  let pressureThrottleTimer: ReturnType<typeof setTimeout> | null = null

  function cancelPressureThrottle(): void {
    if (pressureThrottleTimer !== null) {
      clearTimeout(pressureThrottleTimer)
      pressureThrottleTimer = null
    }
    pendingPressureUpdate = null
  }

  function flushPendingPressureIfReady(): void {
    if (pendingPressureUpdate === null) return

    if (pressureThrottleTimer !== null) {
      clearTimeout(pressureThrottleTimer)
      pressureThrottleTimer = null
    }

    const now = Date.now()
    const intervalMs = Math.round(uiRefreshIntervalMs.value)

    const applyPressureUpdate = (pressures: RealtimePressures) => {
      realtimePressures.value = pressures
      // 同步计算气动参数（马赫数、流速），公式与后端 AtmosphericDataCalculator 一致：
      //   Ma = sqrt((2/(γ-1)) * ((Pt/Ps)^((γ-1)/γ) - 1))
      //   SAT = TAT / (1 + 0.2 * r * Ma^2)   （TAT 取风洞温度，开尔文）
      //   V   = Ma * 20.047 * sqrt(SAT)
      // 这是实时显示用的标准大气数据计算，非校准算法。
      calculatedPhysics.value = calculateAtmosphericPhysics(pressures)
    }

    if (now - lastPressureUpdateAt >= intervalMs) {
      applyPressureUpdate(pendingPressureUpdate)
      pendingPressureUpdate = null
      lastPressureUpdateAt = now
    } else {
      const delay = intervalMs - (now - lastPressureUpdateAt)
      pressureThrottleTimer = setTimeout(() => {
        if (pendingPressureUpdate !== null) {
          applyPressureUpdate(pendingPressureUpdate)
          pendingPressureUpdate = null
        }
        lastPressureUpdateAt = Date.now()
        pressureThrottleTimer = null
      }, delay)
    }
  }

  function setUiRefreshHz(hz: number) {
    uiRefreshHz.value = Math.max(1, Math.min(60, hz))
  }

  function updateRealtimePressures(pressures: RealtimePressures) {
    pendingPressureUpdate = pressures
    flushPendingPressureIfReady()
  }

  // 开始轮询校准状态
  function startStatusPolling() {
    stopStatusPolling()
    statusPollingTimer = setInterval(async () => {
      if (!isWailsAvailable()) return
      try {
        const calStatus = await wailsApi.calibration.status()
        if (calStatus) {
          // 更新本地状态
          updateStatusFromBackend(calStatus)
        }
      } catch (err) {
        console.error('轮询校准状态失败:', err)
      }
    }, Math.round(uiRefreshIntervalMs.value))
  }

  // 停止轮询
  function stopStatusPolling() {
    if (statusPollingTimer) {
      clearInterval(statusPollingTimer)
      statusPollingTimer = null
    }
  }

  // 从后端状态更新本地状态
  function updateStatusFromBackend(calStatus: any) {
    const state = calStatus.state ?? calStatus.State
    const taskId = calStatus.taskId ?? calStatus.TaskID ?? calStatus.TaskId ?? ''
    const type = calStatus.type ?? calStatus.Type ?? 'five-hole'
    const totalPoints = calStatus.totalPoints ?? calStatus.TotalPoints ?? 0
    const completedPoints = calStatus.completedPoints ?? calStatus.CompletedPoints ?? calStatus.currentPoint ?? calStatus.CurrentPoint ?? 0
    const backendDataPoints = calStatus.dataPoints ?? calStatus.DataPoints
    const progress = calStatus.progress ?? calStatus.Progress ?? (totalPoints > 0 ? (completedPoints / totalPoints) * 100 : 0)
    const mappedState = mapCalibrationState(state)

    if (!status.value) {
      // 初始化状态
      status.value = {
        taskId,
        type,
        status: mappedState,
        totalPoints,
        completedPoints,
        progress,
        dataPoints: Array.isArray(backendDataPoints) ? backendDataPoints : [],
      }
    } else {
      status.value.status = mappedState
      status.value.completedPoints = completedPoints
      status.value.totalPoints = totalPoints
      status.value.progress = progress
      if (Array.isArray(backendDataPoints)) {
        status.value.dataPoints = backendDataPoints
      }
    }

    if (Array.isArray(backendDataPoints)) {
      dataPoints.value = backendDataPoints
    }

    // 更新运行状态
    isRunning.value = state === 'running'
    isPaused.value = state === 'paused'

    // 检查是否完成
    if (state === 'completed' || state === 'error' || state === 'stopped') {
      if (status.value) {
        completeEvent.value = {
          taskId: status.value.taskId || `cal-${Date.now()}`,
          success: state === 'completed',
          totalPoints: status.value.totalPoints,
          duration: 0,
          error: calStatus.lastError ?? calStatus.LastError,
        }
        isRunning.value = false
        isPaused.value = false
        stopStatusPolling()
      }
    }
  }

  // 映射后端状态字符串到前端状态
  function mapCalibrationState(state: string): any {
    switch (state) {
      case 'running': return 'running'
      case 'paused': return 'paused'
      case 'completed': return 'completed'
      case 'error': return 'error'
      default: return 'idle'
    }
  }

  watch(uiRefreshHz, () => {
    flushPendingPressureIfReady()
    if (isRunning.value && isWailsAvailable()) {
      startStatusPolling()
    }
  })

  async function startCalibration(config: CalibrationConfig) {
    const taskId = config.taskId || `cal-${Date.now()}`
    const configToStart: CalibrationConfig = { ...config, taskId }
    // 启动校准时同步配置中持久化的刷新频率，确保用户在设置对话框中的选择立即生效
    if (typeof config.uiRefreshHz === 'number' && Number.isFinite(config.uiRefreshHz)) {
      setUiRefreshHz(config.uiRefreshHz)
    }
    const wails = isWailsAvailable()
    if (wails) {
      const res = await wailsApi.calibration.start(configToStart)
      if (!res.Success) throw new Error(res.Error || '启动校准失败')
    } else {
      const res = await calibrationApi.startCalibration(configToStart)
      if (!res.success) throw new Error(res.error || '启动校准失败')
    }
    isRunning.value = true
    isPaused.value = false
    completeEvent.value = null
    dataPoints.value = []
    status.value = {
      taskId,
      type: configToStart.type,
      status: 'running',
      totalPoints: configToStart.points.length,
      completedPoints: 0,
      progress: 0,
      dataPoints: [],
    }
    if (wails) startStatusPolling()
  }

  async function pause() {
    if (!status.value) return
    if (isWailsAvailable()) {
      const res = await wailsApi.calibration.pause()
      if (!res.Success) {
        throw new Error(res.Error || '暂停校准失败')
      }
    } else {
      await calibrationApi.pauseCalibration(status.value.taskId)
    }
    isPaused.value = true
    if (status.value) status.value.status = 'paused'
  }

  async function resume() {
    if (!status.value) return
    if (isWailsAvailable()) {
      const res = await wailsApi.calibration.resume()
      if (!res.Success) {
        throw new Error(res.Error || '恢复校准失败')
      }
    } else {
      await calibrationApi.resumeCalibration(status.value.taskId)
    }
    isPaused.value = false
    if (status.value) status.value.status = 'running'
  }

  async function stop() {
    if (!status.value) return
    if (isWailsAvailable()) {
      const res = await wailsApi.calibration.stop()
      if (!res.Success) {
        throw new Error(res.Error || '停止校准失败')
      }
    } else {
      await calibrationApi.stopCalibration(status.value.taskId)
    }
    isRunning.value = false
    isPaused.value = false
    stopStatusPolling()
    if (status.value) status.value.status = 'idle'
  }

  async function saveData(savePath: string): Promise<{ success: boolean; filepath?: string; error?: string }> {
    if (!status.value) return { success: false, error: '无活跃任务' }
    return calibrationApi.saveData(status.value.taskId, savePath)
  }

  async function exportReport(): Promise<{ success: boolean; filepath?: string; error?: string }> {
    if (!status.value) return { success: false, error: '无活跃任务' }
    return calibrationApi.exportReport(status.value.taskId)
  }

  function reset() {
    cancelPressureThrottle()
    stopStatusPolling()
    status.value = null
    isRunning.value = false
    isPaused.value = false
    completeEvent.value = null
    dataPoints.value = []
    realtimePressures.value = null
    calculatedPhysics.value = null
    timeInfo.value = null
  }

  return {
    status,
    isRunning,
    isPaused,
    completeEvent,
    dataPoints,
    realtimePressures,
    calculatedPhysics,
    timeInfo,
    uiRefreshHz,
    uiRefreshIntervalMs,
    setUiRefreshHz,
    updateRealtimePressures,
    startCalibration,
    pause,
    resume,
    stop,
    saveData,
    exportReport,
    reset,
  }
})

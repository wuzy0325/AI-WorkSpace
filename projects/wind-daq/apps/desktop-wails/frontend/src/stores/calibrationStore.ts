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

  // 已用时 tick 定时器：固定 1Hz，独立于 statusPolling 频率。
  // 已用时显示精度到秒，1Hz 足够；不必跟随 uiRefreshHz（5Hz~60Hz）浪费主线程。
  let elapsedTickTimer: ReturnType<typeof setInterval> | null = null

  // 暂停累计时长（ms）：用于从总 elapsed 中扣除暂停段，让"已用时"在暂停期间真正停住。
  // lastPauseAt 记录本次暂停开始时刻，resume 时累加到 pausedAccumulatedMs。
  // 这两个字段不进 ref：不需要响应式，只在 tick 计算时读取。
  let pausedAccumulatedMs = 0
  let lastPauseAt: number | null = null

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

  // 启动已用时 tick：每秒刷新 timeInfo，让 UI "已用时"控件真正动起来。
  // 与 statusPolling 同启停，但暂停时 tick 仍保留运行（只是 updateTimeInfo 内部会跳过 elapsed 累加），
  // 这样恢复运行后无需重启 timer，elapsed 立即恢复递增。
  function startElapsedTick() {
    stopElapsedTick()
    elapsedTickTimer = setInterval(() => {
      updateTimeInfo()
    }, 1000)
  }

  function stopElapsedTick() {
    if (elapsedTickTimer) {
      clearInterval(elapsedTickTimer)
      elapsedTickTimer = null
    }
  }

  // 基于 status.startTime + pausedAccumulatedMs 计算 elapsedTime 与 estimatedRemaining。
  // 暂停态下直接跳过更新（保留上次 timeInfo），让"已用时"在暂停期间冻结。
  function updateTimeInfo() {
    const startTime = status.value?.startTime
    if (!startTime || startTime <= 0) return
    // 暂停期间不更新 elapsed，避免 UI 数字继续前进
    if (lastPauseAt !== null) return

    const now = Date.now()
    const elapsedTime = Math.max(0, now - startTime - pausedAccumulatedMs)

    // 估算剩余时间：用已完成的平均单点耗时 × 剩余点数。
    // 仅在已完成点数 > 0 且总点数 > 已完成点数时有效，否则置 0 让 UI 显示 "--:--"。
    const completed = status.value?.completedPoints ?? 0
    const total = status.value?.totalPoints ?? 0
    let estimatedRemaining = 0
    if (completed > 0 && elapsedTime > 0 && total > completed) {
      const avgPerPoint = elapsedTime / completed
      estimatedRemaining = Math.round(avgPerPoint * (total - completed))
    }

    timeInfo.value = { elapsedTime, estimatedRemaining }
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
    // 读取后端返回的启动时间戳（calibration.Status.StartTime，JSON 字段 startTime）。
    // 这是 elapsed 计时的基准——比前端本地记 Date.now() 更准：用户切换页面/刷新后仍能基于后端真实启动时刻恢复已用时。
    const startTime = calStatus.startTime ?? calStatus.StartTime
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
        startTime: typeof startTime === 'number' && startTime > 0 ? startTime : undefined,
        dataPoints: Array.isArray(backendDataPoints) ? backendDataPoints : [],
      }
    } else {
      status.value.status = mappedState
      status.value.completedPoints = completedPoints
      status.value.totalPoints = totalPoints
      status.value.progress = progress
      // 后端在 Start 时才写入 StartTime；首次轮询可能还没拿到，需要每次都尝试补齐
      if (typeof startTime === 'number' && startTime > 0) {
        status.value.startTime = startTime
      }
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

    // 运行/暂停态都需要 tick：暂停时 tick 内部会跳过 elapsed 累加，但保留 timer
    // 以便 resume 后立即恢复递增，无需重启 timer。
    if (state === 'running' || state === 'paused') {
      if (status.value?.startTime) {
        if (state === 'running' && lastPauseAt !== null) {
          // 后端从 paused 切回 running，但本地 lastPauseAt 未清——说明 resume 路径未走 store
          // （例如页面刷新后后端已是 running）：补一次累计避免 elapsed 偏大
          pausedAccumulatedMs += Date.now() - lastPauseAt
          lastPauseAt = null
        }
        if (state === 'paused' && lastPauseAt === null) {
          // 反向边缘情况：页面刷新后从后端读到 paused 态，本地没有 lastPauseAt——
          // 用当前时刻兜底，避免 tick 把暂停期间的时长计入 elapsed
          lastPauseAt = Date.now()
        }
        updateTimeInfo()
        startElapsedTick()
      }
    }

    // 检查是否完成
    if (state === 'completed' || state === 'error' || state === 'stopped') {
      if (status.value) {
        // 完成时刷新一次最终 elapsed，让 UI 显示真实总耗时
        if (lastPauseAt !== null) {
          pausedAccumulatedMs += Date.now() - lastPauseAt
          lastPauseAt = null
        }
        updateTimeInfo()
        completeEvent.value = {
          taskId: status.value.taskId || `cal-${Date.now()}`,
          success: state === 'completed',
          totalPoints: status.value.totalPoints,
          duration: timeInfo.value?.elapsedTime ?? 0,
          error: calStatus.lastError ?? calStatus.LastError,
        }
        isRunning.value = false
        isPaused.value = false
        stopStatusPolling()
        stopElapsedTick()
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
    // 启动时刻前端先记录一份本地 startTime 作为兜底：后端 status 首次轮询返回前，UI 已能用上 elapsed。
    // 后端返回真实 StartTime 后会在 updateStatusFromBackend 中覆盖，误差通常 < 一次轮询间隔（200ms）。
    const localStartTime = Date.now()
    pausedAccumulatedMs = 0
    lastPauseAt = null
    status.value = {
      taskId,
      type: configToStart.type,
      status: 'running',
      totalPoints: configToStart.points.length,
      completedPoints: 0,
      progress: 0,
      startTime: localStartTime,
      dataPoints: [],
    }
    updateTimeInfo()
    if (wails) {
      startStatusPolling()
    }
    // 已用时 tick 在 wails / http 两种模式下都需要启动：
    // http 模式没有 statusPolling，但 UI "已用时"控件同样需要每秒刷新。
    startElapsedTick()
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
    // 记录暂停开始时刻，tick 据此冻结 elapsed；resume 时累加到 pausedAccumulatedMs
    if (lastPauseAt === null) {
      lastPauseAt = Date.now()
    }
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
    // 把暂停段累加到 pausedAccumulatedMs，让 elapsed 从恢复时刻继续递增
    if (lastPauseAt !== null) {
      pausedAccumulatedMs += Date.now() - lastPauseAt
      lastPauseAt = null
    }
    if (status.value) status.value.status = 'running'
    // 立即刷新一次，避免 UI 等到下一个 tick 才更新
    updateTimeInfo()
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
    // 停止时同样把暂停段结算到 pausedAccumulatedMs，确保最终 timeInfo 准确
    if (lastPauseAt !== null) {
      pausedAccumulatedMs += Date.now() - lastPauseAt
      lastPauseAt = null
    }
    updateTimeInfo()
    stopStatusPolling()
    stopElapsedTick()
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
    stopElapsedTick()
    pausedAccumulatedMs = 0
    lastPauseAt = null
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

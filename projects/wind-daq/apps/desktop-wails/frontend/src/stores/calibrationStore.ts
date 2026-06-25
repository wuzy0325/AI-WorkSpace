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

export interface AngleInfo {
  alpha?: number
  beta?: number
}

export interface TimeInfo {
  elapsedTime: number
  estimatedRemaining: number
}

// 状态轮询定时器
let statusPollingTimer: ReturnType<typeof setInterval> | null = null

export const useCalibrationStore = defineStore('calibration', () => {
  const status = ref<CalibrationTaskStatus | null>(null)
  const isRunning = ref(false)
  const isPaused = ref(false)
  const completeEvent = ref<CalibrationCompleteEvent | null>(null)
  const dataPoints = ref<CalibrationAnyDataPoint[]>([])
  const realtimePressures = ref<RealtimePressures | null>(null)
  const calculatedPhysics = ref<CalculatedPhysics | null>(null)
  const angleInfo = ref<AngleInfo | null>(null)
  const timeInfo = ref<TimeInfo | null>(null)
  const uiRefreshHz = ref(5)
  const uiRefreshIntervalMs = computed(() => 1000 / uiRefreshHz.value)

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

    if (now - lastPressureUpdateAt >= intervalMs) {
      realtimePressures.value = pendingPressureUpdate
      pendingPressureUpdate = null
      lastPressureUpdateAt = now
    } else {
      const delay = intervalMs - (now - lastPressureUpdateAt)
      pressureThrottleTimer = setTimeout(() => {
        if (pendingPressureUpdate !== null) {
          realtimePressures.value = pendingPressureUpdate
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
    angleInfo.value = null
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
    angleInfo,
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

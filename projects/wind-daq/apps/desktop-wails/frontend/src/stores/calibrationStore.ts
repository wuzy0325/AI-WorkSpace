import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
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

  function setUiRefreshHz(hz: number) {
    uiRefreshHz.value = Math.max(1, Math.min(60, hz))
  }

  function updateRealtimePressures(pressures: RealtimePressures) {
    realtimePressures.value = pressures
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
    }, uiRefreshIntervalMs.value)
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
    if (!status.value) {
      // 初始化状态
      status.value = {
        taskId: calStatus.TaskId || '',
        type: 'five-hole',
        status: mapCalibrationState(calStatus.State),
        totalPoints: calStatus.TotalPoints || 0,
        completedPoints: calStatus.CurrentPoint || 0,
        progress: calStatus.TotalPoints > 0 
          ? (calStatus.CurrentPoint / calStatus.TotalPoints) * 100 
          : 0,
        dataPoints: [],
      }
    } else {
      status.value.status = mapCalibrationState(calStatus.State)
      status.value.completedPoints = calStatus.CurrentPoint || 0
      status.value.totalPoints = calStatus.TotalPoints || 0
      status.value.progress = calStatus.TotalPoints > 0 
        ? (calStatus.CurrentPoint / calStatus.TotalPoints) * 100 
        : 0
    }

    // 更新运行状态
    isRunning.value = calStatus.State === 'running'
    isPaused.value = calStatus.State === 'paused'

    // 检查是否完成
    if (calStatus.State === 'completed' || calStatus.State === 'idle') {
      if (isRunning.value) {
        completeEvent.value = {
          taskId: status.value?.taskId || `cal-${Date.now()}`,
          success: calStatus.State === 'completed',
          totalPoints: status.value.totalPoints,
          duration: 0,
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

  async function startCalibration(config: CalibrationConfig) {
    let taskId: string
    const wails = isWailsAvailable()
    if (wails) {
      const res = await wailsApi.calibration.start(config)
      if (!res.Success) throw new Error(res.Error || '启动校准失败')
      taskId = config.taskId || `cal-${Date.now()}`
    } else {
      const res = await calibrationApi.startCalibration(config)
      if (!res.success) throw new Error(res.error || '启动校准失败')
      taskId = res.taskId || ''
    }
    isRunning.value = true
    isPaused.value = false
    completeEvent.value = null
    dataPoints.value = []
    status.value = {
      taskId,
      type: config.type,
      status: 'running',
      totalPoints: config.points.length,
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

  async function saveData(): Promise<{ success: boolean; filepath?: string; error?: string }> {
    if (!status.value) return { success: false, error: '无活跃任务' }
    return calibrationApi.saveData(status.value.taskId)
  }

  async function exportReport(): Promise<{ success: boolean; filepath?: string; error?: string }> {
    if (!status.value) return { success: false, error: '无活跃任务' }
    return calibrationApi.exportReport(status.value.taskId)
  }

  function reset() {
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

import { ref, computed, onMounted } from 'vue'
import { useCalibrationStore } from '@stores/calibrationStore'
import { useDeviceStore } from '@stores/deviceStore'
import { useMotionStore } from '@stores/motionStore'
import { useFeedbackStore } from '@stores/feedbackStore'
import { useSphereTankGate } from '@composables/useSphereTankGate'
import { calibrationApi } from '@api/calibrationApi'
import type { CalibrationConfig, CalibrationType, ProbeChannelRole } from '@shared/types/calibration'
import { applyCalibrationPrecisionDefaults } from '@shared/calibrationPrecision'

export function useCalibrationWorkflow(calibrationType: CalibrationType) {
  const calibrationStore = useCalibrationStore()
  const deviceStore = useDeviceStore()
  const motionStore = useMotionStore()
  const feedbackStore = useFeedbackStore()

  const isLoading = ref(true)
  const hasConfig = ref(false)
  const currentConfig = ref<CalibrationConfig | null>(null)

  const sphereTankGate = useSphereTankGate({
    calibrationType,
    config: currentConfig,
    onError: (message) => {
      feedbackStore.pushToast('球罐判定更新失败: ' + message, 'error')
    },
  })

  async function loadSavedConfig() {
    try {
      const res = await calibrationApi.getConfig(calibrationType)
      if (res.success && res.data) {
        currentConfig.value = applyCalibrationPrecisionDefaults(res.data)
        hasConfig.value = true
      }
    } catch (err) {
      console.error('Failed to load config:', err)
    }
  }

  async function saveSphereTankGate(enabled: boolean, waitTimeSec: number): Promise<void> {
    await sphereTankGate.saveGate(enabled, waitTimeSec)
  }

  function hasConfiguredProbeChannel(roles: ProbeChannelRole[]): boolean {
    if (!currentConfig.value || !currentConfig.value.probeChannels) return false
    return currentConfig.value.probeChannels.some((channel) => {
      if (!channel.enabled || !channel.channel.deviceId || channel.channel.channelIndex < 0) return false
      return channel.role ? roles.includes(channel.role) : false
    })
  }

  async function startCalibration(configOverrides?: Partial<CalibrationConfig>) {
    if (!canStartCalibration.value || !currentConfig.value) {
      feedbackStore.pushToast(startDisabledReason.value || '请先完成校准前检查', 'warning')
      return
    }
    try {
      await calibrationStore.startCalibration(currentConfig.value)
    } catch (err) {
      console.error('Failed to start calibration:', err)
      feedbackStore.pushToast('启动校准失败: ' + (err instanceof Error ? err.message : String(err)), 'error')
    }
  }

  async function pauseCalibration() {
    try { await calibrationStore.pause() } catch (err) { console.error('Failed to pause:', err) }
  }

  async function resumeCalibration() {
    try { await calibrationStore.resume() } catch (err) { console.error('Failed to resume:', err) }
  }

  async function stopCalibration() {
    const accepted = await feedbackStore.confirm('确定要停止校准吗？当前进度将丢失。', {
      title: '停止校准',
      confirmText: '停止',
      cancelText: '取消',
    })
    if (accepted) {
      try {
        await calibrationStore.stop()
      } catch (err) {
        console.error('Failed to stop:', err)
        feedbackStore.pushToast('停止校准失败: ' + (err instanceof Error ? err.message : String(err)), 'error')
      }
    }
  }

  async function saveCsv() {
    const res = await calibrationStore.saveData()
    if (!res.success) {
      feedbackStore.pushToast('保存失败: ' + (res.error || '未知错误'), 'error')
      return
    }
    feedbackStore.pushToast('已保存: ' + (res.filepath || ''), 'success')
  }

  async function exportReport() {
    const res = await calibrationStore.exportReport()
    if (!res.success) {
      feedbackStore.pushToast('导出失败: ' + (res.error || '未知错误'), 'error')
      return
    }
    feedbackStore.pushToast('已导出: ' + (res.filepath || ''), 'success')
  }

  const progressInfo = computed(() => {
    const status = calibrationStore.status
    if (!status) return null
    return {
      current: status.completedPoints,
      total: status.totalPoints,
      percent: status.progress.toFixed(1),
      currentPoint: status.currentPoint?.coordinates,
    }
  })

  const formattedTimeInfo = computed(() => {
    const ti = calibrationStore.timeInfo
    if (!ti) return null
    const elapsed = ti.elapsedTime ? formatDuration(ti.elapsedTime) : '00:00'
    const remaining = ti.estimatedRemaining ? formatDuration(ti.estimatedRemaining) : '--:--'
    return { elapsed, remaining }
  })

  const isAcquisitionDeviceConnected = computed(() => {
    if (!currentConfig.value?.probeChannels) return false
    const deviceIds = new Set<string>()
    currentConfig.value.probeChannels.forEach((ch) => {
      if (ch.enabled && ch.channel.deviceId) deviceIds.add(ch.channel.deviceId)
    })
    if (deviceIds.size === 0) return false
    for (const deviceId of deviceIds) {
      const device = deviceStore.profiles?.find((d) => d.id === deviceId)
      if (!device) return false
      if (deviceStore.statusFor(deviceId) !== 'Connected') return false
    }
    return true
  })

  const isMotionControllerConnected = computed(() => {
    if (!currentConfig.value?.motionAxes) return false
    const controllerIds = new Set<string>()
    currentConfig.value.motionAxes.forEach((axis) => {
      if (axis.controllerId) controllerIds.add(axis.controllerId)
    })
    if (controllerIds.size === 0) return false
    for (const controllerId of controllerIds) {
      const status = motionStore.statusList.find((s) => s.id === controllerId)
      if (!status?.connected) return false
    }
    return true
  })

  const configuredDeviceNames = computed(() => {
    if (!currentConfig.value?.probeChannels) return []
    const names: string[] = []
    const seen = new Set<string>()
    currentConfig.value.probeChannels.forEach((channel) => {
      if (channel.enabled && channel.channel.deviceId && !seen.has(channel.channel.deviceId)) {
        seen.add(channel.channel.deviceId)
        const device = deviceStore.profiles?.find((d) => d.id === channel.channel.deviceId)
        if (device) names.push(device.name)
      }
    })
    return names
  })

  const configuredControllerNames = computed(() => {
    if (!currentConfig.value?.motionAxes) return []
    const names: string[] = []
    const seen = new Set<string>()
    currentConfig.value.motionAxes.forEach((axis) => {
      if (axis.controllerId && !seen.has(axis.controllerId)) {
        seen.add(axis.controllerId)
        const controller = motionStore.profiles.find((c) => c.id === axis.controllerId)
        if (controller) names.push(controller.name)
      }
    })
    return names
  })

  const statusText = computed(() => {
    if (!calibrationStore.status) return '空闲'
    switch (calibrationStore.status.status) {
      case 'idle': return '空闲'
      case 'running': return '运行中'
      case 'paused': return '已暂停'
      case 'completed': return '已完成'
      case 'error': return '错误'
      default: return '空闲'
    }
  })

  const statusColor = computed(() => {
    if (!calibrationStore.status) return 'normal'
    switch (calibrationStore.status.status) {
      case 'running': return 'success'
      case 'paused': return 'warning'
      case 'completed': return 'info'
      case 'error': return 'danger'
      default: return 'normal'
    }
  })

  const canStartCalibration = computed(() => {
    return hasConfig.value && 
           isAcquisitionDeviceConnected.value && 
           isMotionControllerConnected.value
  })

  const startDisabledReason = computed(() => {
    if (isLoading.value) return '正在加载配置，请稍候'
    if (!hasConfig.value) return '请先配置校准参数'
    if (!isAcquisitionDeviceConnected.value) return '采集设备未连接'
    if (!isMotionControllerConnected.value) return '运动控制器未连接'
    return ''
  })

  onMounted(async () => {
    try {
      await Promise.all([
        deviceStore.refreshProfiles(),
        motionStore.refreshProfiles(),
        motionStore.refreshStatus(),
        loadSavedConfig(),
      ])
    } finally {
      isLoading.value = false
    }
  })

  return {
    calibrationStore,
    deviceStore,
    motionStore,
    feedbackStore,
    isLoading,
    hasConfig,
    currentConfig,
    sphereTankGate,
    loadSavedConfig,
    startCalibration,
    pauseCalibration,
    resumeCalibration,
    stopCalibration,
    saveCsv,
    exportReport,
    saveSphereTankGate,
    progressInfo,
    formattedTimeInfo,
    isAcquisitionDeviceConnected,
    isMotionControllerConnected,
    configuredDeviceNames,
    configuredControllerNames,
    statusText,
    statusColor,
    canStartCalibration,
    startDisabledReason,
  }
}

function formatDuration(ms: number): string {
  const totalSec = Math.max(0, Math.round(ms / 1000))
  const minutes = Math.floor(totalSec / 60)
  const seconds = totalSec % 60
  return `${String(minutes).padStart(2, '0')}:${String(seconds).padStart(2, '0')}`
}

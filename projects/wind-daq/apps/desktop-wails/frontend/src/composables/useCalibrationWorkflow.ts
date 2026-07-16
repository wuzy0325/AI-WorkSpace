import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { useCalibrationStore } from '@stores/calibrationStore'
import { useDeviceStore } from '@stores/deviceStore'
import { useMotionStore } from '@stores/motionStore'
import { useFeedbackStore } from '@stores/feedbackStore'
import { useI18nStore } from '@stores/i18nStore'
import { useSphereTankGate } from '@composables/useSphereTankGate'
import { calibrationApi } from '@api/calibrationApi'
import type { CalibrationConfig, CalibrationType, ProbeChannelRole } from '@shared/types/calibration'
import { applyCalibrationPrecisionDefaults } from '@shared/calibrationPrecision'
import { buildCalibrationCsvName } from '@shared/calibrationCsvPath'
import { reportAllSettledFailures } from '@utils/allSettledReport'

export function useCalibrationWorkflow(calibrationType: CalibrationType) {
  const calibrationStore = useCalibrationStore()
  const deviceStore = useDeviceStore()
  const motionStore = useMotionStore()
  const feedbackStore = useFeedbackStore()
  const i18n = useI18nStore()

  const isLoading = ref(true)
  const hasConfig = ref(false)
  const currentConfig = ref<CalibrationConfig | null>(null)

  const sphereTankGate = useSphereTankGate({
    calibrationType,
    config: currentConfig,
    onError: (message) => {
      feedbackStore.pushToast(i18n.t.wf_sphereTankUpdateFailed + ': ' + message, 'error')
    },
  })

  async function loadSavedConfig() {
    try {
      const res = await calibrationApi.getConfig(calibrationType)
      if (res.success && res.data) {
        currentConfig.value = applyCalibrationPrecisionDefaults(res.data)
        // coordinates 键名迁移：旧版前端生成 'Theta'/'Alpha'（英文），新版统一使用 'θ'/'α'（希腊字母）
        // 与后端 normalizer / total_pressure.go:101 point.Coordinates["α"] 对齐。
        // 存量配置若不迁移：三孔 coordinates['θ'] 读出 undefined → UI 显示 '--'；
        // 总压第一个点就报"测点缺少 α 坐标"，校准启动后进度永远 0。
        // 单次遍历同时处理两个键，避免重复扫两遍 points。
        currentConfig.value.points?.forEach((p) => {
          if (!p.coordinates) return
          if (typeof p.coordinates['θ'] !== 'number' && typeof p.coordinates['Theta'] === 'number') {
            p.coordinates['θ'] = p.coordinates['Theta']
            delete p.coordinates['Theta']
          }
          if (typeof p.coordinates['α'] !== 'number' && typeof p.coordinates['Alpha'] === 'number') {
            p.coordinates['α'] = p.coordinates['Alpha']
            delete p.coordinates['Alpha']
          }
        })
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

  async function startCalibration() {
    if (!canStartCalibration.value || !currentConfig.value) {
      feedbackStore.pushToast(startDisabledReason.value || i18n.t.wf_pleaseCompletePreCheck, 'warning')
      return
    }
    // CSV 文件覆盖检测：配置名不改时 savePath 与上次相同，后端追加模式 writer 会在
    // 旧文件末尾续写（已修表头重复 bug），但两次校准数据混在一起容易混淆分析。
    // 因此检测到文件已存在时弹窗让用户决定：覆盖（删旧文件后 Start）或取消（改路径再来）。
    const savePath = currentConfig.value.savePath?.trim()
    if (savePath) {
      try {
        const { useStorageStore } = await import('@stores/storageStore')
        const storageStore = useStorageStore()
        const exists = await storageStore.fileExists(savePath)
        if (exists) {
          const accepted = await feedbackStore.confirm(
            i18n.t.wf_csvOverwriteConfirm.replace('{path}', savePath),
            {
              title: i18n.t.wf_fileExists,
              confirmText: i18n.t.wf_overwrite,
              cancelText: i18n.t.cancel,
            },
          )
          if (!accepted) {
            feedbackStore.pushToast(i18n.t.wf_startCancelled, 'info')
            return
          }
          // 检查 removeFile 返回值：Wails 不可用或权限不足时返回 false，
          // 若不阻断启动，后端会按追加模式续写，两次校准数据混在一起。
          const removed = await storageStore.removeFile(savePath)
          if (!removed) {
            feedbackStore.pushToast(i18n.t.wf_removeOldCsvFailed, 'warning')
            return
          }
        }
      } catch (err) {
        console.error('Failed to check/remove CSV file:', err)
        feedbackStore.pushToast(i18n.t.wf_checkCsvFailed + ': ' + (err instanceof Error ? err.message : String(err)), 'warning')
        // 检测失败不阻断启动，让后端按追加模式处理
      }
    }
    try {
      await calibrationStore.startCalibration(currentConfig.value)
    } catch (err) {
      console.error('Failed to start calibration:', err)
      feedbackStore.pushToast(i18n.t.wf_startCalibrationFailed + ': ' + (err instanceof Error ? err.message : String(err)), 'error')
    }
  }

  async function pauseCalibration() {
    try { await calibrationStore.pause() } catch (err) { console.error('Failed to pause:', err) }
  }

  async function resumeCalibration() {
    try { await calibrationStore.resume() } catch (err) { console.error('Failed to resume:', err) }
  }

  async function stopCalibration() {
    const accepted = await feedbackStore.confirm(i18n.t.wf_stopCalibrationConfirm, {
      title: i18n.t.wf_stopCalibrationTitle,
      confirmText: i18n.t.stop,
      cancelText: i18n.t.cancel,
    })
    if (accepted) {
      try {
        await calibrationStore.stop()
      } catch (err) {
        console.error('Failed to stop:', err)
        feedbackStore.pushToast(i18n.t.wf_stopCalibrationFailed + ': ' + (err instanceof Error ? err.message : String(err)), 'error')
      }
    }
  }

  async function saveCsv() {
    // 校准过程已实时写入 config.savePath；此按钮作为"导出到指定位置"入口，
    // 弹出文件选择器让用户选目标路径，由后端 SaveCsv 全量覆盖写入。
    try {
      const { useStorageStore } = await import('@stores/storageStore')
      const storageStore = useStorageStore()
      // 默认文件名清洗：buildCalibrationCsvName 统一处理非法字符替换 + 日期去重，
      // 与四个 Settings 组件的 pickSavePath 共用同一份清洗逻辑。
      const defaultName = buildCalibrationCsvName(currentConfig.value?.name || '', 'calibration')
      const target = await storageStore.pickSaveFile(i18n.t.wf_selectCsvExportLocation, defaultName, [
        { displayName: i18n.t.wf_csvFileFilter, pattern: '*.csv' },
      ])
      if (!target) return
      const res = await calibrationStore.saveData(target)
      if (!res.success) {
        feedbackStore.pushToast(i18n.t.wf_exportFailed + ': ' + (res.error || i18n.t.unknownError), 'error')
        return
      }
      feedbackStore.pushToast(i18n.t.wf_exported + ': ' + (res.filepath || target), 'success')
    } catch (e) {
      feedbackStore.pushToast(i18n.t.wf_exportFailed + ': ' + (e instanceof Error ? e.message : String(e)), 'error')
    }
  }

  async function exportReport() {
    const res = await calibrationStore.exportReport()
    if (!res.success) {
      feedbackStore.pushToast(i18n.t.wf_exportFailed + ': ' + (res.error || i18n.t.unknownError), 'error')
      return
    }
    feedbackStore.pushToast(i18n.t.wf_exported + ': ' + (res.filepath || ''), 'success')
  }

  const progressInfo = computed(() => {
    const status = calibrationStore.status
    if (!status) return null
    // 目标点索引取 max(currentPointIndex, completedPoints)：
    //   - 自动模式（五孔/三孔/总压）：autoEngine 在 processPoint 循环顶部推进 currentPointIdx，
    //     使其领先 completedPoints 1 个点，实现"目标角度先于实际角度变化"的直觉。
    //   - 手动模式（总温）：autoEngine 为 nil，后端 CurrentPoint 始终为 0（int 无 omitempty），
    //     此时 max(0, completedPoints) = completedPoints，正确指向下一个待采点。
    //   - 边界：两者均超出 points.length 时由 Math.min 截断到最后一个点，不会越界。
    // completedPoints 仍用于进度条/百分比（表示"已完成采集的点数"）。
    const points = currentConfig.value?.points ?? []
    const autoIdx = typeof status.currentPointIndex === 'number' ? status.currentPointIndex : 0
    const targetIdx = Math.max(autoIdx, status.completedPoints)
    const idx = Math.min(targetIdx, points.length - 1)
    // 仅在运行中或暂停时才返回当前目标点，idle/completed/error 态下无"当前目标"
    const validState = status.status === 'running' || status.status === 'paused'
    const currentPoint = validState && idx >= 0 ? points[idx] : undefined
    return {
      current: status.completedPoints,
      total: status.totalPoints,
      percent: status.progress.toFixed(1),
      // currentPoint 直接返回 coordinates（Record<string, number>），
      // 与三孔/总压/总温组件读法（progressInfo.currentPoint?.['θ']）保持一致。
      currentPoint: currentPoint?.coordinates,
      // currentPointId 单独暴露：FiveHoleMain 高亮当前点时需要 id 匹配数据点 pointId，
      // 不让 currentPoint 升级为 CalPoint 对象破坏其他三处组件的现有用法。
      currentPointId: currentPoint?.id,
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
      if (!deviceStore.acquiringFor(deviceId)) return false
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
    if (!calibrationStore.status) return i18n.t.idle
    switch (calibrationStore.status.status) {
      case 'idle': return i18n.t.idle
      case 'running': return i18n.t.running
      case 'paused': return i18n.t.statusPaused
      // spec Decision #15 / I7：stop 后显示「已停止」，与 idle 区分（保留数据可导出）
      case 'stopped': return i18n.t.wf_statusStopped
      case 'completed': return i18n.t.completed
      case 'error': return i18n.t.error
      default: return i18n.t.idle
    }
  })

  const statusColor = computed(() => {
    if (!calibrationStore.status) return 'normal'
    switch (calibrationStore.status.status) {
      case 'running': return 'success'
      case 'paused': return 'warning'
      // 已停止：黄色警示色，与 idle（normal）区分
      case 'stopped': return 'warning'
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

  // 设备连接态细分：区分"未连接"和"已连接未采集"，给操作员更精准的下一步指引
  const acquisitionDeviceState = computed<'noDevice' | 'disconnected' | 'connectedNotAcquiring' | 'ready'>(() => {
    if (!currentConfig.value?.probeChannels) return 'noDevice'
    const deviceIds = new Set<string>()
    currentConfig.value.probeChannels.forEach((ch) => {
      if (ch.enabled && ch.channel.deviceId) deviceIds.add(ch.channel.deviceId)
    })
    if (deviceIds.size === 0) return 'noDevice'
    for (const deviceId of deviceIds) {
      const device = deviceStore.profiles?.find((d) => d.id === deviceId)
      if (!device) return 'disconnected'
      if (deviceStore.statusFor(deviceId) !== 'Connected') return 'disconnected'
      if (!deviceStore.acquiringFor(deviceId)) return 'connectedNotAcquiring'
    }
    return 'ready'
  })

  const startDisabledReason = computed(() => {
    if (isLoading.value) return i18n.t.wf_loadingConfig
    if (!hasConfig.value) return i18n.t.wf_pleaseConfigCalibration
    // 细分采集设备状态：未连接 vs 已连接但未开始采集，后者给操作员直接可执行的指引
    if (acquisitionDeviceState.value === 'noDevice') return i18n.t.wf_noAcquisitionChannel
    if (acquisitionDeviceState.value === 'disconnected') return i18n.t.wf_acquisitionDeviceDisconnected
    if (acquisitionDeviceState.value === 'connectedNotAcquiring') return i18n.t.wf_acquisitionDeviceNotAcquiring
    if (!isMotionControllerConnected.value) return i18n.t.wf_motionControllerDisconnected
    return ''
  })

  onMounted(async () => {
    // spec I4 / Recovery UX：进入校准画面统一恢复协议
    //   acquireView → 并行 recovery + 资源加载 → 依后端状态设频
    // acquireView 先行：引用计数+1，按需升频 polling + 重启 elapsedTick（仅 running/paused）
    calibrationStore.acquireView()

    // spec Decision #14：lastRecoveryAt 2s 内跳过二次 recovery，复用 Window 模块级恢复结果，
    // 避免 isRecovering 闪两次 / 重复 loading
    const recoveryAgeMs = Date.now() - (calibrationStore.lastRecoveryAt || 0)
    const shouldRecover = recoveryAgeMs > 2000
    // recoveryPromise 与其他 Promise<void> 资源请求类型对齐，避免 allSettled 推断出混合类型
    const recoveryPromise: Promise<void> = shouldRecover
      ? calibrationStore.recoveryFromBackend()
      : Promise.resolve()

    // 进入校准画面时并行拉取四类资源 + recovery。
    // 使用 allSettled 而非 all：任一请求失败不应阻塞其他资源加载（例如运动控制器离线时仍要展示已保存的配置）。
    // 失败项由 reportAllSettledFailures 统一弹 toast 提示，store 自身负责保留旧状态 + 暴露 error 字段。
    // refreshInstances 同时刷新 profiles 与 statuses：
    // 进入校准画面时必须拿到最新的 acquiring 状态，attachStatusListener →
    // syncSnapshotSubscriptions 才会调用 subscribeToDevice 启动 HTTP 轮询，
    // 否则 onSnapshot 回调不会被触发，UI 实时压力/气动参数会一直显示 "--"。
    // 与 TraversalMain.vue 的做法保持一致。
    const results = await Promise.allSettled([
      deviceStore.refreshInstances(),
      motionStore.refreshProfiles(),
      motionStore.refreshStatus(),
      loadSavedConfig(),
      recoveryPromise,
    ])
    reportAllSettledFailures(
      results,
      [
        i18n.t.wf_labelDeviceInstances,
        i18n.t.wf_labelMotionProfiles,
        i18n.t.wf_labelMotionStatus,
        i18n.t.wf_labelCalibrationConfig,
        i18n.t.wf_labelCalibrationStatus,
      ],
      (msg, level) => feedbackStore.pushToast(msg, level ?? 'warning'),
    )

    // recovery 失败提示（不阻塞渲染，UI 用 recoveryError 显示错误条 + 可重试）
    if (calibrationStore.recoveryError) {
      feedbackStore.pushToast(i18n.t.wf_recoveryFailed + ': ' + calibrationStore.recoveryError, 'error')
    }

    // U16：running/paused 态下若 hasConfig=false（loadSavedConfig 失败），强制再 load 一次，
    // 避免通道面板/按钮因 hasConfig=false 空白
    if ((calibrationStore.isRunning || calibrationStore.isPaused) && !hasConfig.value) {
      await loadSavedConfig()
    }

    // 依后端状态设频：running/paused 升 uiRefreshHz，否则 1Hz 心跳（recovery 内 stopStatusPolling 防竞态）
    calibrationStore.restartPollingForCurrentState()

    isLoading.value = false
  })

  // spec I3 / I4：unmount 只释放视图级资源（引用计数-1），不清空会话状态
  // releaseView 由 composable 统一处理，Main 组件的 onBeforeUnmount 只清局部订阅 / 定时器
  onBeforeUnmount(() => {
    calibrationStore.releaseView()
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
    // spec I8：暴露 recovery 状态给 Main 渲染 loading / 错误条
    isRecovering: computed(() => calibrationStore.isRecovering),
    recoveryError: computed(() => calibrationStore.recoveryError),
  }
}

function formatDuration(ms: number): string {
  const totalSec = Math.max(0, Math.round(ms / 1000))
  const minutes = Math.floor(totalSec / 60)
  const seconds = totalSec % 60
  return `${String(minutes).padStart(2, '0')}:${String(seconds).padStart(2, '0')}`
}

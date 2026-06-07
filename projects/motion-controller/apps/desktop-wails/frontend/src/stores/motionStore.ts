import { defineStore } from 'pinia'
import { ref, toRaw } from 'vue'
import type { MotionControllerProfile, MotionControllerStatus, AxisName } from '@shared/types/motion'
import { motionApi } from '@api/motionApi'
import { useFeedbackStore } from '@stores/feedbackStore'

export const useMotionStore = defineStore('motion', () => {
  const profiles = ref<MotionControllerProfile[]>([])
  const statusList = ref<MotionControllerStatus[]>([])

  function statusById(id: string): MotionControllerStatus | undefined {
    return statusList.value.find((s) => s.id === id)
  }

  async function refreshProfiles(): Promise<void> {
    try {
      const result = await motionApi.getProfiles()
      profiles.value = Array.isArray(result) ? result : []
    } catch (e) {
      const feedback = useFeedbackStore()
      feedback.pushToast(`加载配置失败: ${e instanceof Error ? e.message : '未知错误'}`, 'error')
    }
  }

  async function refreshStatus(): Promise<void> {
    try {
      const result = await motionApi.getStatusAll()
      statusList.value = Array.isArray(result) ? result : []
    } catch {
      // 状态刷新失败静默处理，避免频繁弹窗
    }
  }

  function attachStatusListener(): () => void {
    return motionApi.onStatusUpdated((all) => {
      if (!Array.isArray(all)) return
      // 合并事件数据：更新已连接控制器的状态，保留未连接控制器的条目
      const updatedMap = new Map<string, MotionControllerStatus>()
      for (const s of statusList.value) {
        updatedMap.set(s.id, s)
      }
      for (const s of all) {
        updatedMap.set(s.id, s)
      }
      statusList.value = Array.from(updatedMap.values())
    })
  }

  async function upsertProfile(profile: MotionControllerProfile): Promise<void> {
    const feedback = useFeedbackStore()
    try {
      const safeProfile = toRaw(profile)
      await motionApi.upsertProfile(safeProfile)
      await refreshProfiles()
    } catch (e) {
      feedback.pushToast(`保存配置失败: ${e instanceof Error ? e.message : '未知错误'}`, 'error')
      throw e
    }
  }

  async function deleteProfile(id: string): Promise<void> {
    const feedback = useFeedbackStore()
    try {
      await motionApi.deleteProfile(id)
      await refreshProfiles()
    } catch (e) {
      feedback.pushToast(`删除配置失败: ${e instanceof Error ? e.message : '未知错误'}`, 'error')
      throw e
    }
  }

  async function connect(id: string): Promise<void> {
    const feedback = useFeedbackStore()
    try {
      await motionApi.connect(id)
      await refreshStatus()
    } catch (e) {
      feedback.pushToast(`连接失败: ${e instanceof Error ? e.message : '未知错误'}`, 'error')
      throw e
    }
  }

  async function disconnect(id: string): Promise<void> {
    const feedback = useFeedbackStore()
    try {
      await motionApi.disconnect(id)
      await refreshStatus()
    } catch (e) {
      feedback.pushToast(`断开失败: ${e instanceof Error ? e.message : '未知错误'}`, 'error')
    }
  }

  async function moveTo(id: string, axis: AxisName, position: number): Promise<void> {
    const feedback = useFeedbackStore()
    try {
      await motionApi.moveTo(id, axis, position)
      await refreshStatus()
    } catch (e) {
      feedback.pushToast(`移动失败: ${e instanceof Error ? e.message : '未知错误'}`, 'error')
    }
  }

  async function moveBy(id: string, axis: AxisName, delta: number): Promise<void> {
    const feedback = useFeedbackStore()
    try {
      await motionApi.moveBy(id, axis, delta)
      await refreshStatus()
    } catch (e) {
      feedback.pushToast(`移动失败: ${e instanceof Error ? e.message : '未知错误'}`, 'error')
    }
  }

  async function jog(id: string, axis: AxisName, direction: 'forward' | 'reverse', speed?: number): Promise<void> {
    const feedback = useFeedbackStore()
    try {
      await motionApi.jog(id, axis, direction, speed)
      await refreshStatus()
    } catch (e) {
      feedback.pushToast(`点动失败: ${e instanceof Error ? e.message : '未知错误'}`, 'error')
    }
  }

  async function home(id: string, axis: AxisName): Promise<void> {
    const feedback = useFeedbackStore()
    try {
      await motionApi.home(id, axis)
      await refreshStatus()
    } catch (e) {
      feedback.pushToast(`回零失败: ${e instanceof Error ? e.message : '未知错误'}`, 'error')
    }
  }

  async function stop(id: string, axis?: AxisName): Promise<void> {
    const feedback = useFeedbackStore()
    try {
      await motionApi.stop(id, axis)
      await refreshStatus()
    } catch (e) {
      feedback.pushToast(`停止失败: ${e instanceof Error ? e.message : '未知错误'}`, 'error')
    }
  }

  async function emergencyStop(id: string): Promise<void> {
    const feedback = useFeedbackStore()
    try {
      await motionApi.emergencyStop(id)
      await refreshStatus()
    } catch (e) {
      feedback.pushToast(`急停失败: ${e instanceof Error ? e.message : '未知错误'}`, 'error')
    }
  }

  async function resetEmergencyStop(id: string): Promise<void> {
    const feedback = useFeedbackStore()
    try {
      await motionApi.resetEmergencyStop(id)
      await refreshStatus()
    } catch (e) {
      feedback.pushToast(`解除急停失败: ${e instanceof Error ? e.message : '未知错误'}`, 'error')
    }
  }

  async function definePosition(id: string, axis: AxisName, position: number): Promise<void> {
    const feedback = useFeedbackStore()
    try {
      await motionApi.definePosition(id, axis, position)
      await refreshStatus()
    } catch (e) {
      feedback.pushToast(`定义位置失败: ${e instanceof Error ? e.message : '未知错误'}`, 'error')
    }
  }

  /**
   * 自动连接所有配置了 autoConnect 的控制器
   * 在应用启动时调用，遍历 profiles 中 autoConnect 为 true 且当前未连接的控制器并执行连接
   */
  async function autoConnectAll(): Promise<void> {
    const profilesToConnect = profiles.value.filter((p) => p.autoConnect)
    if (profilesToConnect.length === 0) return

    const feedback = useFeedbackStore()
    // 并行连接所有需要自动连接的控制器
    const results = await Promise.allSettled(
      profilesToConnect.map(async (p) => {
        const status = statusList.value.find((s) => s.id === p.id)
        if (status?.connected) return // 已连接的跳过
        try {
          await motionApi.connect(p.id)
        } catch (e) {
          throw new Error(`${p.name}: ${e instanceof Error ? e.message : '未知错误'}`)
        }
      })
    )

    // 收集失败结果，限制错误信息长度避免 Toast 过长
    const failures = results.filter((r) => r.status === 'rejected') as PromiseRejectedResult[]
    if (failures.length > 0) {
      const messages = failures.map((f) => f.reason instanceof Error ? f.reason.message : '未知错误')
      const summary = messages.length > 3
        ? `${messages.length} 个控制器连接失败`
        : messages.join('; ')
      feedback.pushToast(`自动连接失败: ${summary}`, 'error')
    }

    await refreshStatus()
  }

  function clearError(id: string): void {
    const status = statusList.value.find((s) => s.id === id)
    if (status && status.lastError) {
      status.lastError = ''
    }
  }

  return {
    profiles,
    statusList,
    statusById,
    refreshProfiles,
    refreshStatus,
    attachStatusListener,
    upsertProfile,
    deleteProfile,
    connect,
    disconnect,
    moveTo,
    moveBy,
    jog,
    home,
    stop,
    emergencyStop,
    resetEmergencyStop,
    definePosition,
    autoConnectAll,
    clearError,
  }
})

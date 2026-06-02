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
    clearError,
  }
})

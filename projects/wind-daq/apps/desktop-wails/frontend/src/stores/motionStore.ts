import { defineStore } from 'pinia'
import { ref, toRaw } from 'vue'
import type { MotionControllerProfile, MotionControllerStatus, AxisName } from '@shared/types/motion'
import { motionApi } from '@api/motionApi'

export const useMotionStore = defineStore('motion', () => {
  const profiles = ref<MotionControllerProfile[]>([])
  const statusList = ref<MotionControllerStatus[]>([])

  function statusById(id: string): MotionControllerStatus | undefined {
    return statusList.value.find((s) => s.id === id)
  }

  async function refreshProfiles(): Promise<void> {
    const result = await motionApi.getProfiles()
    profiles.value = Array.isArray(result) ? result : []
  }

  async function refreshStatus(): Promise<void> {
    const result = await motionApi.getStatusAll()
    statusList.value = Array.isArray(result) ? result : []
  }

  function attachStatusListener(): () => void {
    return motionApi.onStatusUpdated((all) => {
      statusList.value = Array.isArray(all) ? all : []
    })
  }

  async function upsertProfile(profile: MotionControllerProfile): Promise<void> {
    const safeProfile = toRaw(profile)
    await motionApi.upsertProfile(safeProfile)
    await refreshProfiles()
  }

  async function deleteProfile(id: string): Promise<void> {
    await motionApi.deleteProfile(id)
    await refreshProfiles()
  }

  async function connect(id: string): Promise<void> {
    await motionApi.connect(id)
    await refreshStatus()
  }

  async function disconnect(id: string): Promise<void> {
    await motionApi.disconnect(id)
    await refreshStatus()
  }

  async function moveTo(id: string, axis: AxisName, position: number): Promise<void> {
    await motionApi.moveTo(id, axis, position)
  }

  async function moveBy(id: string, axis: AxisName, delta: number): Promise<void> {
    await motionApi.moveBy(id, axis, delta)
  }

  async function jog(id: string, axis: AxisName, direction: 'forward' | 'reverse', speed?: number): Promise<void> {
    await motionApi.jog(id, axis, direction, speed)
  }

  async function home(id: string, axis: AxisName): Promise<void> {
    await motionApi.home(id, axis)
  }

  async function stop(id: string, axis?: AxisName): Promise<void> {
    await motionApi.stop(id, axis)
  }

  async function emergencyStop(id: string): Promise<void> {
    await motionApi.emergencyStop(id)
  }

  async function definePosition(id: string, axis: AxisName, position: number): Promise<void> {
    await motionApi.definePosition(id, axis, position)
  }

  async function resetEmergencyStop(id: string): Promise<void> {
    await motionApi.resetEmergencyStop(id)
  }

  /**
   * 自动连接所有配置了 autoConnect 的控制器。
   * 在应用启动时遍历 profiles 中 autoConnect 为 true
   * 且当前未连接的控制器并执行连接。
   */
  async function autoConnectAll(): Promise<void> {
    const profilesToConnect = profiles.value.filter((p) => p.autoConnect)
    if (profilesToConnect.length === 0) return

    const results = await Promise.allSettled(
      profilesToConnect.map(async (p) => {
        const status = statusList.value.find((s) => s.id === p.id)
        if (status?.connected) return
        try {
          await motionApi.connect(p.id)
        } catch (e) {
          throw new Error(`${p.name}: ${e instanceof Error ? e.message : '未知错误'}`)
        }
      })
    )

    const failures = results.filter((r) => r.status === 'rejected') as PromiseRejectedResult[]
    if (failures.length > 0) {
      const messages = failures.map((f) => f.reason instanceof Error ? f.reason.message : '未知错误')
      const summary = messages.length > 3
        ? `${messages.length} 个控制器连接失败`
        : messages.join('; ')
      console.warn(`[motionStore] autoConnectAll: ${summary}`)
    }

    await refreshStatus()
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
    definePosition,
    resetEmergencyStop,
    autoConnectAll,
  }
})

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
  }

  async function disconnect(id: string): Promise<void> {
    await motionApi.disconnect(id)
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
  }
})

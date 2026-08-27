import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { fetchDevices } from "@/api/device"
import {
  multipressRegister,
  multipressUnregister,
  multipressSetPressure,
  multipressStop,
  multipressExhaust,
  multipressSetUnit,
  multipressListDevices,
  multipressStopAll
} from "@/api/multipress"
import type { DeviceDTO } from "@/types/device"
import type { MultiPressDeviceState } from "@/types/multipress"
import type { StreamEventPayload } from "@/types/api"

/** 设备元信息（来自设备管理模块） */
export interface DeviceMeta {
  name: string
  model: string
  host: string
  port: number
  status: DeviceDTO['status']
}

export const useMultiPressStore = defineStore('multipress', () => {
  // ---- State ----

  /** 已注册打压设备的运行状态 */
  const devices = ref<Map<string, MultiPressDeviceState>>(new Map())

  /** 设备元信息（从设备管理模块获取） */
  const deviceMetadata = ref<Map<string, DeviceMeta>>(new Map())

  /** 加载状态 */
  const loading = ref(false)

  /** 轮询定时器 */
  let pollingTimer: ReturnType<typeof setInterval> | null = null

  // ---- Getters ----

  /** 已注册设备列表 */
  const registeredDevices = computed(() => Array.from(devices.value.values()))

  /** 注册设备数量 */
  const registeredCount = computed(() => devices.value.size)

  /** 打压中设备数量 */
  const pressurizingCount = computed(
    () => Array.from(devices.value.values()).filter((d) => d.status === 'pressurizing').length
  )

  /** 可用（未注册的）打压设备 */
  const availableDevices = computed(() => {
    const registered = new Set(devices.value.keys())
    return Array.from(deviceMetadata.value.entries())
      .filter(([id]) => !registered.has(id))
      .map(([id, meta]) => ({ id, ...meta }))
  })

  // ---- Actions ----

  /** 从设备管理模块加载所有打压设备的元信息 */
  async function loadPressureDevices(): Promise<void> {
    loading.value = true
    try {
      const allDevices = await fetchDevices()
      const pressureDevices = allDevices.filter((d) => d.type === 'pressure')
      const newMeta = new Map<string, DeviceMeta>()
      for (const d of pressureDevices) {
        newMeta.set(d.id, {
          name: d.name,
          model: d.model,
          host: d.host,
          port: d.port,
          status: d.status
        })
      }
      deviceMetadata.value = newMeta
    } finally {
      loading.value = false
    }
  }

  /** 注册打压设备 */
  async function registerDevice(id: string): Promise<void> {
    await multipressRegister(id)
    // 注册后立即拉取最新状态
    await refreshDeviceStates()
  }

  /** 注销打压设备 */
  async function unregisterDevice(id: string): Promise<void> {
    await multipressUnregister(id)
    devices.value.delete(id)
  }

  /** 设置目标压力 */
  async function setPressure(id: string, target: number): Promise<void> {
    await multipressSetPressure(id, target)
    const d = devices.value.get(id)
    if (d) {
      d.targetPressure = target
      d.status = 'pressurizing'
    }
  }

  /** 停止打压 */
  async function stopDevice(id: string): Promise<void> {
    await multipressStop(id)
    const d = devices.value.get(id)
    if (d) {
      d.status = 'idle'
    }
  }

  /** 排空压力 */
  async function exhaustDevice(id: string): Promise<void> {
    await multipressExhaust(id)
    const d = devices.value.get(id)
    if (d) {
      d.status = 'exhausting'
    }
  }

  /** 紧急停止所有设备 */
  async function stopAll(): Promise<void> {
    await multipressStopAll()
    for (const d of devices.value.values()) {
      d.status = 'idle'
    }
  }

  /** 设置压力单位 */
  async function setUnit(id: string, unit: string): Promise<void> {
    await multipressSetUnit(id, unit)
    const d = devices.value.get(id)
    if (d) {
      d.unit = unit
    }
  }

  /** 拉取所有已注册设备的最新状态 */
  async function refreshDeviceStates(): Promise<void> {
    const states = await multipressListDevices()
    const newMap = new Map<string, MultiPressDeviceState>()
    for (const s of states) {
      newMap.set(s.deviceId, s)
    }
    devices.value = newMap
  }

  /** 处理 SSE 事件 */
  function handleSSEEvent(payload: StreamEventPayload): void {
    if (!payload.type.startsWith('multipress.')) return

    const data = payload.data as Record<string, unknown>
    const deviceId = data?.deviceId as string | undefined
    if (!deviceId) return

    switch (payload.type) {
      case 'multipress.pressure.update': {
        const d = devices.value.get(deviceId)
        if (d) {
          if (typeof data.currentPressure === 'number') d.currentPressure = data.currentPressure
          if (typeof data.stable === 'boolean') d.stable = data.stable
          if (typeof data.status === 'string') d.status = data.status as MultiPressDeviceState['status']
        }
        break
      }
      case 'multipress.device.status': {
        const d = devices.value.get(deviceId)
        if (d) {
          if (typeof data.status === 'string') d.status = data.status as MultiPressDeviceState['status']
          if (typeof data.errorMessage === 'string') d.errorMessage = data.errorMessage
        }
        break
      }
      case 'multipress.pressure.changed': {
        const d = devices.value.get(deviceId)
        if (d && typeof data.targetPressure === 'number') {
          d.targetPressure = data.targetPressure
        }
        break
      }
      case 'multipress.device.unregistered': {
        devices.value.delete(deviceId)
        break
      }
    }
  }

  /** 启动轮询 */
  function startPolling(): void {
    stopPolling()
    pollingTimer = setInterval(() => {
      refreshDeviceStates().catch(() => {})
    }, 2000)
  }

  /** 停止轮询 */
  function stopPolling(): void {
    if (pollingTimer !== null) {
      clearInterval(pollingTimer)
      pollingTimer = null
    }
  }

  /** 获取设备元信息 */
  function getMeta(deviceId: string): DeviceMeta | undefined {
    return deviceMetadata.value.get(deviceId)
  }

  return {
    // state
    devices,
    deviceMetadata,
    loading,
    // getters
    registeredDevices,
    registeredCount,
    pressurizingCount,
    availableDevices,
    // actions
    loadPressureDevices,
    registerDevice,
    unregisterDevice,
    setPressure,
    stopDevice,
    exhaustDevice,
    stopAll,
    setUnit,
    handleSSEEvent,
    startPolling,
    stopPolling,
    getMeta,
    refreshDeviceStates,
    setupListeners: startPolling,
    cleanup: stopPolling
  }
})

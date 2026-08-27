import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import type { ActionResult } from '@/types/api'
import {
  bindMeasureDevice,
  bindDevices,
  unbindMeasureDevices,
  readPressure,
  readStability,
  readMeasureData,
  readValveStatus,
  setValveStatus as apiSetValveStatus,
  readMeasureUnit,
  setMeasureUnit as apiSetMeasureUnit,
  readDeviceInfo,
  resetDevice as apiResetDevice
} from "@/api/session"
import {
  multipressRegister,
  multipressUnregister,
  multipressListDevices
} from "@/api/multipress"
import { useDeviceInventoryStore } from '@/stores/device/inventoryStore'
import { fetchDevices, upsertDevice } from '@/api/device'

export const useDeviceControlStore = defineStore('deviceControl', () => {
  const deviceStore = useDeviceInventoryStore()

  // State
  const currentPressure = ref(0)
  const isStable = ref(false)
  const channelData = ref<number[]>([])
  const valveStatus = ref<string>('')
  const measureUnit = ref<string>('')
  const deviceInfo = ref<Record<string, string>>({})

  // Getters
  const device1604Connected = computed(() => {
    return deviceStore.measureDevices.some(d => d.status === 'connected')
  })

  const pressDeviceConnected = computed(() => {
    return deviceStore.pressureDevices.some(d => d.status === 'connected')
  })

  // Actions
  const sleep = (ms: number) => new Promise<void>((resolve) => {
    setTimeout(resolve, ms)
  })

  // 连接1604设备
  const connectDevice1604 = async (deviceId: string): Promise<ActionResult> => {
    try {
      const result = await deviceStore.connectMeasureDevice(deviceId)
      if (!result.ok) {
        return result
      }

      // 连接成功后立即绑定到设备会话，使其能读取阀门/单位/设备信息
      // 显式传 moduleName='calibration'，与计量模块区分（防冲突覆盖）。
      try {
        await bindMeasureDevice(deviceId, 'calibration')
      } catch (err) {
        console.error('bindMeasureDevice failed:', err)
        return { ok: false, error: 'BIND_FAILED', detail: '绑定计量设备会话失败，无法读取阀门/单位信息' }
      }

      // 读取设备信息、阀门状态和单位（连接后增加重试，避免设备刚建链时读数失败）
      const loaded = await refreshDeviceInfo({ retries: 3, retryDelayMs: 500 })
      if (!loaded) {
        console.warn('设备已连接，但阀门/单位信息读取失败，请稍后重试')
      }

      // 把从硬件读取到的实际单位同步到设备配置，确保 CheckUnitConsistency 比较的是真实单位
      if (measureUnit.value) {
        try {
          const devices = await fetchDevices()
          const dto = devices.find(d => d.id === deviceId)
          if (dto) {
            await upsertDevice({ ...dto, unit: measureUnit.value })
          }
        } catch (syncErr) {
          console.warn('同步计量设备单位到配置失败:', syncErr)
        }
      }

      return { ok: true }
    } catch (error) {
      console.error('连接1604设备失败:', error)
      return { ok: false, error: 'CONNECT_FAILED', detail: String(error) }
    }
  }

  // 连接多台1604设备：逐台连接（跳过已连接设备，避免重复建链），
  // 全部成功后整批绑定到会话。
  // 与 connectDevice1604 的区别：连接阶段不逐台绑定，避免后端绑定集合被反复覆盖，
  // 全部连接成功后再一次性绑定整批设备。
  const connectMeasureDevices = async (deviceIds: string[]): Promise<ActionResult> => {
    try {
      for (const deviceId of deviceIds) {
        const device = deviceStore.measureDevices.find(d => d.id === deviceId)
        if (!device || device.status === 'connected') continue
        const result = await deviceStore.connectMeasureDevice(deviceId)
        if (!result.ok) {
          return result
        }
      }

      // 全部连接成功后整批绑定到设备会话，使其能读取阀门/单位/设备信息
      try {
        await bindDevices(deviceIds, '', 'calibration')
      } catch (err) {
        console.error('bindDevices failed:', err)
        return { ok: false, error: 'BIND_FAILED', detail: '绑定计量设备会话失败，无法读取阀门/单位信息' }
      }

      // 读取设备信息、阀门状态和单位（连接后增加重试，避免设备刚建链时读数失败）
      const loaded = await refreshDeviceInfo({ retries: 3, retryDelayMs: 500 })
      if (!loaded) {
        console.warn('设备已连接，但阀门/单位信息读取失败，请稍后重试')
      }

      // 把从硬件读取到的实际单位同步到设备配置，确保 CheckUnitConsistency 比较的是真实单位
      if (measureUnit.value) {
        try {
          const devices = await fetchDevices()
          for (const deviceId of deviceIds) {
            const dto = devices.find(d => d.id === deviceId)
            if (dto) {
              await upsertDevice({ ...dto, unit: measureUnit.value })
            }
          }
        } catch (syncErr) {
          console.warn('同步计量设备单位到配置失败:', syncErr)
        }
      }

      return { ok: true }
    } catch (error) {
      console.error('连接1604设备失败:', error)
      return { ok: false, error: 'CONNECT_FAILED', detail: String(error) }
    }
  }

  // 断开1604设备
  const disconnectDevice1604 = async (deviceId: string): Promise<ActionResult> => {
    try {
      const result = await deviceStore.disconnectMeasureDevice(deviceId)
      if (!result.ok) return result
      // 断开后同步会话绑定：后端 Disconnect 只断 TCP 不清理绑定，
      // 残留的旧驱动会让后续读取持续报 "not connected"，重连时还会
      // 因绑定集合与模块名不一致触发 device binding conflict。
      await syncBindingAfterDisconnect([deviceId])
      return { ok: true }
    } catch (error) {
      console.error('断开1604设备失败:', error)
      return { ok: false, error: 'DISCONNECT_FAILED', detail: String(error) }
    }
  }

  // 断开多台1604设备：逐台断开。
  const disconnectMeasureDevices = async (deviceIds: string[]): Promise<ActionResult> => {
    try {
      for (const deviceId of deviceIds) {
        const result = await deviceStore.disconnectMeasureDevice(deviceId)
        if (!result.ok) {
          return result
        }
      }
      await syncBindingAfterDisconnect(deviceIds)
      return { ok: true }
    } catch (error) {
      console.error('断开1604设备失败:', error)
      return { ok: false, error: 'DISCONNECT_FAILED', detail: String(error) }
    }
  }

  // 断开后同步会话绑定（仅计量侧；打压设备绑定保持不变）：
  // 已无连接中的计量设备时整体解绑并清空本地阀门/单位状态；
  // 仍有连接中的设备时以 'calibration' 身份重绑剩余集合，
  // 保证会话中的驱动始终是活跃驱动、绑定集合与实际连接一致。
  const syncBindingAfterDisconnect = async (disconnectedIds: string[]) => {
    const remaining = deviceStore.measureDevices
      .filter(d => d.status === 'connected' && !disconnectedIds.includes(d.id))
      .map(d => d.id)

    if (remaining.length === 0) {
      await unbindMeasureDevices()
      valveStatus.value = ''
      measureUnit.value = ''
      deviceInfo.value = {}
      return
    }
    await bindMeasureDevice(remaining, 'calibration')
  }

  // 连接打压设备：通过 multipress 服务注册（创建驱动 + TCP连接 + 注册到压力控制模块）。
  // 注册请求是同步阻塞的（TCP 建链 + 单位读取），耗时不定；必须先置为 connecting，
  // 让按钮 loading 与状态徽章即时反馈，否则连接期间界面表现为"点连接没反应"。
  const connectPressDevice = async (deviceId: string): Promise<ActionResult> => {
    const device = deviceStore.pressureDevices.find(d => d.id === deviceId)
    if (device) device.status = 'connecting'
    try {
      await multipressRegister(deviceId)
      // multipress 服务不更新 DeviceManager 状态，需手动同步前端 store
      deviceStore.updateDeviceStatus(deviceId, 'connected')
      // 从 multipress 服务拉取实际读取到的单位与压力，覆盖配置中的默认值
      try {
        const states = await multipressListDevices()
        const state = states.find(s => s.deviceId === deviceId)
        const dev = deviceStore.pressureDevices.find(d => d.id === deviceId)
        if (state && dev) {
          if (state.unit) {
            dev.unit = state.unit
            // 同步实际单位到设备配置，确保 CheckUnitConsistency 比较的是硬件真实单位
            try {
              const devices = await fetchDevices()
              const dto = devices.find(d => d.id === deviceId)
              if (dto) {
                await upsertDevice({ ...dto, unit: state.unit })
              }
            } catch (syncErr) {
              console.warn('同步打压设备单位到配置失败:', syncErr)
            }
          }
          dev.currentPressure = state.currentPressure
        }
      } catch {
        // 静默失败，使用设备配置中的单位
      }
      return { ok: true }
    } catch (error) {
      // 失败回滚为异常态，避免停留在"连接中"假象
      deviceStore.updateDeviceStatus(deviceId, 'error')
      console.error('连接打压设备失败:', error)
      return { ok: false, error: 'CONNECT_FAILED', detail: '连接打压设备失败' }
    }
  }

  // 断开打压设备：通过 multipress 服务注销（停止控制 + 断开TCP + 移除注册）
  const disconnectPressDevice = async (deviceId: string): Promise<ActionResult> => {
    try {
      await multipressUnregister(deviceId)
      deviceStore.updateDeviceStatus(deviceId, 'disconnected')
      return { ok: true }
    } catch (error) {
      console.error('断开打压设备失败:', error)
      return { ok: false, error: 'DISCONNECT_FAILED', detail: '断开打压设备失败' }
    }
  }

  // 设置校准设备（通知后端）
  // 显式传 moduleName='calibration'：与计量模块（默认 'measurement'）区分，
  // 避免绑定冲突检查（后端 boundBy）因同 moduleName 而静默互相覆盖。
  const setDevices = async (measureDeviceId: string, pressureDeviceId: string): Promise<ActionResult> => {
    try {
      await bindDevices(measureDeviceId, pressureDeviceId, 'calibration')
      return { ok: true }
    } catch (error) {
      console.error('绑定设备会话失败:', error)
      return { ok: false, error: 'BIND_FAILED', detail: '绑定设备会话失败' }
    }
  }

  // 设置多台校准计量设备（选 1 台时行为与 setDevices 一致）
  const setMeasureDevices = async (measureDeviceIds: string[], pressureDeviceId: string): Promise<ActionResult> => {
    try {
      await bindDevices(measureDeviceIds, pressureDeviceId, 'calibration')
      return { ok: true }
    } catch (error) {
      console.error('绑定多设备会话失败:', error)
      return { ok: false, error: 'BIND_FAILED', detail: '绑定多设备会话失败' }
    }
  }

  // 读取实时压力
  const refreshPressure = async () => {
    try {
      currentPressure.value = await readPressure()
    } catch {
      // 静默失败
    }
  }

  // 读取稳定状态
  const refreshStability = async () => {
    try {
      isStable.value = await readStability()
    } catch {
      // 静默失败
    }
  }

  // 读取计量设备数据
  const refreshMeasureData = async () => {
    try {
      channelData.value = await readMeasureData()
    } catch {
      // 静默失败
    }
  }

  // 刷新设备信息（阀门状态、单位、设备信息）
  const refreshDeviceInfo = async (
    options: { retries?: number; retryDelayMs?: number } = {}
  ): Promise<boolean> => {
    const retries = Math.max(1, options.retries ?? 1)
    const retryDelayMs = Math.max(0, options.retryDelayMs ?? 0)
    let valveReady = false
    let unitReady = false

    const tryRead = async <T>(reader: () => Promise<T>): Promise<PromiseSettledResult<T>> => {
      try {
        return { status: 'fulfilled', value: await reader() }
      } catch (error) {
        return { status: 'rejected', reason: error }
      }
    }

    for (let attempt = 1; attempt <= retries; attempt++) {
      const [valve, unit] = await Promise.allSettled([
        readValveStatus(),
        readMeasureUnit()
      ])

      if (valve.status === 'fulfilled') {
        valveStatus.value = valve.value
        valveReady = true
      }
      if (unit.status === 'fulfilled') {
        measureUnit.value = unit.value
        unitReady = true
      }
      // 连接阶段只要求阀门和单位可读；且允许在不同重试轮次分别成功。
      if (valveReady && unitReady) {
        // 设备信息是增强信息，避免其失败拖累阀门/单位可用性判定。
        const info = await tryRead(readDeviceInfo)
        if (info.status === 'fulfilled') {
          deviceInfo.value = info.value
        }
        return true
      }

      if (attempt < retries && retryDelayMs > 0) {
        await sleep(retryDelayMs)
      }
    }

    return false
  }

  // 读取阀门状态
  const refreshValveStatus = async () => {
    try {
      valveStatus.value = await readValveStatus()
    } catch {
      // 静默失败
    }
  }

  // 设置阀门状态
  const setValveStatus = async (status: string): Promise<ActionResult> => {
    try {
      await apiSetValveStatus(status)
      valveStatus.value = status
      return { ok: true }
    } catch (error) {
      // 把设备拒绝（N09 等）/网络错误的 message 一起带回，
      // 由调用方决定如何展示给用户（ElMessage.error）。
      const detail = error instanceof Error ? error.message : '设置阀门状态失败'
      console.error('设置阀门状态失败:', error)
      return { ok: false, error: 'VALVE_SET_FAILED', detail }
    }
  }

  // 读取计量设备单位
  const refreshMeasureUnit = async () => {
    try {
      measureUnit.value = await readMeasureUnit()
    } catch { /* unit read failed, continue with default */ }
  }

  // 设置计量设备单位
  const setMeasureUnit = async (unit: string): Promise<ActionResult> => {
    try {
      await apiSetMeasureUnit(unit)
      measureUnit.value = await readMeasureUnit()
      return { ok: true }
    } catch (error) {
      console.error('设置计量设备单位失败:', error)
      return { ok: false, error: 'UNIT_SET_FAILED', detail: '设置计量设备单位失败' }
    }
  }

  // 复位计量设备
  const resetDevice = async (): Promise<ActionResult> => {
    try {
      await apiResetDevice()
      return { ok: true }
    } catch (error) {
      console.error('复位设备失败:', error)
      return { ok: false, error: 'RESET_FAILED', detail: '复位设备失败' }
    }
  }

  return {
    // State
    currentPressure,
    isStable,
    channelData,
    valveStatus,
    measureUnit,
    deviceInfo,
    // Getters
    device1604Connected,
    pressDeviceConnected,
    // Actions
    connectDevice1604,
    connectMeasureDevices,
    disconnectDevice1604,
    disconnectMeasureDevices,
    connectPressDevice,
    disconnectPressDevice,
    setDevices,
    setMeasureDevices,
    refreshPressure,
    refreshStability,
    refreshMeasureData,
    refreshDeviceInfo,
    refreshValveStatus,
    setValveStatus,
    refreshMeasureUnit,
    setMeasureUnit,
    resetDevice
  }
})

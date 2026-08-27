import { defineStore } from 'pinia'
import { ref } from 'vue'
import {
  fetchDevices,
  connectDevice,
  disconnectDevice,
  upsertDevice
} from "@/api/device"
import {
  multipressRegister,
  multipressUnregister,
  multipressListDevices,
  multipressReadPressure
} from "@/api/multipress"
import type { DeviceDTO } from "@/types/device"
import type { ActionResult } from '@/types/api'

// 前端设备模型
export interface PressureDevice {
  id: string
  name: string
  model: string
  ip: string
  port: number
  status: 'connected' | 'disconnected' | 'connecting' | 'error'
  currentPressure?: number
  unit: string
}

export interface MeasureDevice {
  id: string
  name: string
  model: string
  channels: number
  ip?: string
  port?: number
  status: 'connected' | 'disconnected' | 'connecting' | 'error'
}

// DTO转换函数
function dtoToPressureDevice(dto: DeviceDTO): PressureDevice {
  return {
    id: dto.id,
    name: dto.name,
    model: dto.model,
    ip: dto.host,
    port: dto.port,
    status: dto.status === 'connected' ? 'connected' :
            dto.status === 'connecting' ? 'connecting' :
            dto.status === 'error' ? 'error' : 'disconnected',
    currentPressure: dto.status === 'connected' ? 0 : undefined,
    unit: dto.unit || 'kPa'
  }
}

function dtoToMeasureDevice(dto: DeviceDTO): MeasureDevice {
  return {
    id: dto.id,
    name: dto.name,
    model: dto.model,
    channels: 16, // 默认为16通道
    status: dto.status === 'connected' ? 'connected' : 
            dto.status === 'connecting' ? 'connecting' :
            dto.status === 'error' ? 'error' : 'disconnected'
  }
}

function pressureDeviceToDto(device: PressureDevice): DeviceDTO {
  return {
    id: device.id,
    name: device.name,
    type: 'pressure',
    model: device.model,
    host: device.ip,
    port: device.port,
    unit: device.unit,
    status: device.status === 'connected' ? 'connected' :
            device.status === 'connecting' ? 'connecting' :
            device.status === 'error' ? 'error' : 'disconnected'
  }
}

function measureDeviceToDto(device: MeasureDevice): DeviceDTO {
  return {
    id: device.id,
    name: device.name,
    type: 'measure',
    model: device.model,
    host: device.ip || '192.168.1.100',
    port: device.port || 9000,
    unit: 'kPa',
    status: mapDTOStatus(device.status)
  }
}

function mapDTOStatus(s: 'connected' | 'disconnected' | 'connecting' | 'error'): DeviceDTO['status'] {
  if (s === 'connected') return 'connected'
  if (s === 'connecting') return 'connecting'
  if (s === 'error') return 'error'
  return 'disconnected'
}

/** 合并本地状态与后端状态：后端状态为权威来源。
 *  后端明确报 disconnected（设备管理模块或业务模块主动断开）时，
 *  本地 connected 必须降级，否则跨模块会残留"已连接"的过期状态。
 *  connecting/error 为瞬时或故障态，直接以后端为准。 */
function mergeStatus(
  local: 'connected' | 'disconnected' | 'connecting' | 'error',
  remote: DeviceDTO['status']
): 'connected' | 'disconnected' | 'connecting' | 'error' {
  if (remote === 'connecting') return 'connecting'
  if (remote === 'error') return 'error'
  if (remote === 'disconnected') return 'disconnected'
  return local
}

export const useMeasurementDeviceStore = defineStore('measurementDevices', () => {
  // State
  const pressureDevices = ref<PressureDevice[]>([])
  const measureDevices = ref<MeasureDevice[]>([])
  const loading = ref(false)

  // 从后端加载设备列表（增量合并，不覆盖本地实时状态如 currentPressure）
  const loadDevices = async (): Promise<ActionResult> => {
    try {
      loading.value = true
      const devices = await fetchDevices()

      // 增量合并打压设备列表，保留本地实时状态
      const backendPressureIds = new Set(
        devices.filter(d => d.type === 'pressure').map(d => d.id)
      )
      // 移除后端已删除的设备
      pressureDevices.value = pressureDevices.value.filter(
        p => backendPressureIds.has(p.id)
      )

      for (const dto of devices.filter(d => d.type === 'pressure')) {
        const local = pressureDevices.value.find(p => p.id === dto.id)
        if (local) {
          // 保留 currentPressure 等本地实时状态，只更新配置信息
          local.name = dto.name
          local.model = dto.model
          local.ip = dto.host
          local.port = dto.port
          local.unit = dto.unit || 'kPa'
          // 同步连接状态：如果后端状态为 connected 但本地不是，标记为 connected
          if (dto.status === 'connected' && local.status !== 'connected') {
            local.status = 'connected'
          } else if (dto.status !== 'connected') {
            local.status = mergeStatus(local.status, dto.status)
          }
        } else {
          // 新设备
          pressureDevices.value.push(dtoToPressureDevice(dto))
        }
      }

      // 增量合并计量设备列表
      const backendMeasureIds = new Set(
        devices.filter(d => d.type === 'measure').map(d => d.id)
      )
      measureDevices.value = measureDevices.value.filter(
        m => backendMeasureIds.has(m.id)
      )

      for (const dto of devices.filter(d => d.type === 'measure')) {
        const local = measureDevices.value.find(m => m.id === dto.id)
        if (local) {
          local.name = dto.name
          local.model = dto.model
          local.ip = dto.host
          local.port = dto.port
          if (dto.status === 'connected' && local.status !== 'connected') {
            local.status = 'connected'
          } else if (dto.status !== 'connected') {
            local.status = mergeStatus(local.status, dto.status)
          }
        } else {
          measureDevices.value.push(dtoToMeasureDevice(dto))
        }
      }
      return { ok: true }
    } catch (error) {
      console.error('加载设备列表失败:', error)
      return { ok: false, error: 'LOAD_FAILED', detail: String(error) }
    } finally {
      loading.value = false
    }
  }

  // Actions
  const connectPressureDevice = async (id: string): Promise<ActionResult> => {
    const device = pressureDevices.value.find(d => d.id === id)
    if (!device) return { ok: false, error: 'DEVICE_NOT_FOUND' }

    try {
      device.status = 'connecting'
      await multipressRegister(id)

      device.status = 'connected'
      if (typeof device.currentPressure !== 'number') {
        device.currentPressure = 0
      }

      // 注册后从 multipress 服务拉取实际读取到的单位与压力，覆盖配置中的默认值
      try {
        const states = await multipressListDevices()
        const state = states.find(s => s.deviceId === id)
        if (state) {
          if (state.unit) {
            device.unit = state.unit
            // 同步实际单位到后端设备配置，确保 CheckUnitConsistency 比较的是硬件实际单位
            try {
              await upsertDevice(pressureDeviceToDto(device))
            } catch (syncErr) {
              console.warn('同步打压设备单位到配置失败:', syncErr)
            }
          }
          device.currentPressure = state.currentPressure
        }
      } catch {
        // 静默失败，使用设备配置中的单位
      }

      // 连接成功后尝试读取初始压力值。
      await refreshPressureForDevice(id)
      return { ok: true }
    } catch (error) {
      device.status = 'error'
      console.error('连接设备失败:', error)
      return { ok: false, error: 'CONNECT_FAILED', detail: String(error) }
    }
  }

  // 刷新打压设备的实时压力值。
  // 走 multipress 直读接口而非改绑会话：此前以 'pressureRefresh' 模块名
  // 改绑会话会抢占绑定所有权（携带任意一台计量设备），
  // 导致计量模块随后以多设备集合绑定时触发 device binding conflict，
  // 多设备采集无法启动。multipress 服务在连接时已注册该设备，可直读。
  const refreshPressureForDevice = async (pressureId: string) => {
    try {
      const pressure = await multipressReadPressure(pressureId)
      const device = pressureDevices.value.find(d => d.id === pressureId)
      if (device) {
        device.currentPressure = pressure
      }
    } catch (err) {
      console.warn('读取初始压力失败:', err)
    }
  }

  // 刷新所有已连接打压设备的压力值。
  // 同样走 multipress 直读，不改绑会话；单台失败静默跳过，不影响其他设备。
  const refreshAllConnectedPressures = async () => {
    for (const device of pressureDevices.value) {
      if (device.status !== 'connected') continue
      try {
        device.currentPressure = await multipressReadPressure(device.id)
      } catch {
        // 静默失败，不影响其他设备
      }
    }
  }

  const disconnectPressureDevice = async (id: string): Promise<ActionResult> => {
    const device = pressureDevices.value.find(d => d.id === id)
    if (!device) return { ok: false, error: 'DEVICE_NOT_FOUND' }

    try {
      await multipressUnregister(id)
      device.status = 'disconnected'
      device.currentPressure = undefined
      return { ok: true }
    } catch (error) {
      console.error('断开设备失败:', error)
      return { ok: false, error: 'DISCONNECT_FAILED', detail: String(error) }
    }
  }

  const connectMeasureDevice = async (id: string): Promise<ActionResult> => {
    const device = measureDevices.value.find(d => d.id === id)
    if (!device) return { ok: false, error: 'DEVICE_NOT_FOUND' }

    try {
      device.status = 'connecting'
      const updatedDto = await connectDevice(id)
      const updated = dtoToMeasureDevice(updatedDto)
      Object.assign(device, updated)

      if (updated.status !== 'connected') {
        const reason = updatedDto.lastErrorReason || '未知原因'
        return { ok: false, error: 'CONNECT_FAILED', detail: reason }
      }

      return { ok: true }
    } catch (error) {
      device.status = 'error'
      console.error('连接设备失败:', error)
      return { ok: false, error: 'CONNECT_FAILED', detail: String(error) }
    }
  }

  const disconnectMeasureDevice = async (id: string): Promise<ActionResult> => {
    const device = measureDevices.value.find(d => d.id === id)
    if (!device) return { ok: false, error: 'DEVICE_NOT_FOUND' }

    try {
      const updatedDto = await disconnectDevice(id)
      const updated = dtoToMeasureDevice(updatedDto)
      Object.assign(device, updated)
      return { ok: true }
    } catch (error) {
      console.error('断开设备失败:', error)
      return { ok: false, error: 'DISCONNECT_FAILED', detail: String(error) }
    }
  }

  // 添加新设备
  const addPressureDevice = async (device: Omit<PressureDevice, 'id' | 'status'>): Promise<ActionResult> => {
    try {
      const dto = await upsertDevice({
        ...pressureDeviceToDto({ ...device, id: '', status: 'disconnected' }),
        id: crypto.randomUUID()
      })
      pressureDevices.value.push(dtoToPressureDevice(dto))
      return { ok: true }
    } catch (error) {
      console.error('添加设备失败:', error)
      return { ok: false, error: 'ADD_FAILED', detail: String(error) }
    }
  }

  const addMeasureDevice = async (device: Omit<MeasureDevice, 'id' | 'status'>): Promise<ActionResult> => {
    try {
      const dto = await upsertDevice({
        ...measureDeviceToDto({ ...device, id: '', status: 'disconnected' }),
        id: crypto.randomUUID()
      })
      measureDevices.value.push(dtoToMeasureDevice(dto))
      return { ok: true }
    } catch (error) {
      console.error('添加设备失败:', error)
      return { ok: false, error: 'ADD_FAILED', detail: String(error) }
    }
  }

  // 更新设备压力值（用于SSE更新）
  const updateDevicePressure = (id: string, pressure: number) => {
    const device = pressureDevices.value.find(d => d.id === id)
    if (device) {
      device.currentPressure = pressure
    }
  }

  // 更新设备状态（用于SSE更新）
  const updateDeviceStatus = (id: string, status: PressureDevice['status']) => {
    const pressureDevice = pressureDevices.value.find(d => d.id === id)
    const measureDevice = measureDevices.value.find(d => d.id === id)
    
    if (pressureDevice) {
      pressureDevice.status = status
      if (status === 'connected') {
        pressureDevice.currentPressure = 0
      } else if (status === 'disconnected') {
        pressureDevice.currentPressure = undefined
      }
    }
    
    if (measureDevice) {
      measureDevice.status = status
    }
  }

  return {
    pressureDevices,
    measureDevices,
    loading,
    loadDevices,
    connectPressureDevice,
    disconnectPressureDevice,
    connectMeasureDevice,
    disconnectMeasureDevice,
    addPressureDevice,
    addMeasureDevice,
    updateDevicePressure,
    updateDeviceStatus,
    refreshPressureForDevice,
    refreshAllConnectedPressures
  }
})

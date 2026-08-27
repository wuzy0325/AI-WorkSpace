import { apiGet, apiPost } from './client'

export interface DeviceValueResult {
  value?: string
  error?: string
}

export interface UnitConsistencyResult {
  consistent: boolean
  conflicts: string[]
}

/** 绑定计量设备（支持多台）和打压设备到会话。
 * 传入单个设备 ID 或数组均可；内部做 trim + 去重，保证后端绑定集合干净有序。 */
export async function bindDevices(
  measureDeviceId: string | string[],
  pressureDeviceId: string,
  moduleName = 'measurement'
): Promise<void> {
  const ids = [...new Set(
    (Array.isArray(measureDeviceId) ? measureDeviceId : [measureDeviceId])
      .map(id => id.trim())
      .filter(Boolean)
  )]
  await apiPost('/session/devices', {
    measureDeviceId: ids[0] ?? '',
    measureDeviceIds: ids,
    pressureDeviceId,
    moduleName
  })
}

/** 仅绑定计量设备（保留当前打压设备绑定）；支持多设备 */
export async function bindMeasureDevice(
  measureDeviceId: string | string[],
  moduleName = 'measurement'
): Promise<void> {
  const ids = Array.isArray(measureDeviceId) ? measureDeviceId : [measureDeviceId]
  await apiPost('/session/measure-device', {
    measureDeviceId: ids[0] ?? '',
    measureDeviceIds: ids,
    moduleName
  })
}

/** 读取当前压力 */
export async function readPressure(): Promise<number> {
  return (await apiGet<{ pressure: number }>('/session/pressure')).pressure
}

/** 读取稳定状态 */
export async function readStability(): Promise<boolean> {
  return (await apiGet<{ stable: boolean }>('/session/stability')).stable
}

/** 读取计量设备实时数据（首个绑定设备，兼容单设备场景） */
export async function readMeasureData(): Promise<number[]> {
  return (await apiGet<{ data: number[] }>('/session/measure-data')).data
}

/** 读取所有已绑定计量设备的实时数据（deviceID -> 通道数据），供多设备展示 */
export async function readMeasureDataAllDevices(): Promise<Record<string, number[]>> {
  return (await apiGet<{ data: number[]; devices: Record<string, number[]> }>('/session/measure-data')).devices
}

/** 读取阀门状态 */
export async function readValveStatus(): Promise<string> {
  return (await apiGet<{ status: string }>('/session/valve')).status
}

/** 清除当前会话的计量设备绑定，释放模块所有权。 */
export async function unbindMeasureDevices(): Promise<void> {
  await apiPost('/session/measure-device/unbind')
}

/** 逐台读取所有已绑定计量设备的阀门状态，单台错误保留在对应结果中。 */
export async function readValveStatusAll(): Promise<Record<string, DeviceValueResult>> {
  return (await apiGet<{ devices: Record<string, DeviceValueResult> }>('/session/valve/all')).devices
}

/** 设置阀门状态 */
export async function setValveStatus(status: string): Promise<void> {
  await apiPost('/session/valve', { status })
}

/** 读取压力单位 */
export async function readMeasureUnit(): Promise<string> {
  return (await apiGet<{ unit: string }>('/session/measure-unit')).unit
}

/** 逐台读取所有已绑定计量设备的压力单位。 */
export async function readMeasureUnitAll(): Promise<Record<string, DeviceValueResult>> {
  return (await apiGet<{ devices: Record<string, DeviceValueResult> }>('/session/measure-unit/all')).devices
}

/** 设置压力单位 */
export async function setMeasureUnit(unit: string): Promise<void> {
  await apiPost('/session/measure-unit', { unit })
}

/** 将所有已绑定计量设备统一为同一压力单位。 */
export async function setMeasureUnitAll(unit: string): Promise<void> {
  await apiPost('/session/measure-unit/all', { unit })
}

/** 检查当前会话绑定设备的单位一致性，不包含其它流程连接的设备。 */
export async function readSessionUnitConsistency(): Promise<UnitConsistencyResult> {
  return apiGet<UnitConsistencyResult>('/session/unit-consistency')
}

/** 读取设备信息 */
export async function readDeviceInfo(): Promise<Record<string, string>> {
  return (await apiGet<{ info: Record<string, string> }>('/session/device-info')).info
}

/** 对指定通道执行校零，返回各通道校零偏移；偏移会持久化到本地并在重连后自动应用 */
export async function calibrateZero(channels: number[], deviceId = ''): Promise<number[]> {
  return (await apiPost<{ data: number[] }>('/session/calibrate-zero', { deviceId, channels })).data
}

/** 复位设备 */
export async function resetDevice(deviceId = ''): Promise<void> {
  await apiPost('/session/reset', { deviceId })
}

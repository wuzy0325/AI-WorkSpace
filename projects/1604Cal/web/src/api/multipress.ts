import type { MultiPressDeviceState } from '@/types/multipress'
import { apiGet, apiPost } from './client'

/** 注册打压设备到多设备控制模块 */
export async function multipressRegister(deviceId: string): Promise<void> {
  await apiPost('/multipress/register', { deviceId })
}

/** 注销打压设备 */
export async function multipressUnregister(deviceId: string): Promise<void> {
  await apiPost('/multipress/unregister', { deviceId })
}

/** 设置目标压力 */
export async function multipressSetPressure(deviceId: string, targetPressure: number): Promise<void> {
  await apiPost('/multipress/set-pressure', { deviceId, targetPressure })
}

/** 停止打压 */
export async function multipressStop(deviceId: string): Promise<void> {
  await apiPost('/multipress/stop', { deviceId })
}

/** 排空压力 */
export async function multipressExhaust(deviceId: string): Promise<void> {
  await apiPost('/multipress/exhaust', { deviceId })
}

/** 读取指定设备当前压力 */
export async function multipressReadPressure(deviceId: string): Promise<number> {
  return (await apiGet<{ pressure: number; deviceId: string }>(
    `/multipress/pressure?deviceId=${encodeURIComponent(deviceId)}`
  )).pressure
}

/** 读取指定设备稳定状态 */
export async function multipressReadStability(deviceId: string): Promise<boolean> {
  return (await apiGet<{ stable: boolean; deviceId: string }>(
    `/multipress/stability?deviceId=${encodeURIComponent(deviceId)}`
  )).stable
}

/** 设置指定设备压力单位 */
export async function multipressSetUnit(deviceId: string, unit: string): Promise<void> {
  await apiPost('/multipress/unit', { deviceId, unit })
}

/** 获取所有已注册设备状态 */
export async function multipressListDevices(): Promise<MultiPressDeviceState[]> {
  return (await apiGet<MultiPressDeviceState[]>('/multipress/devices')) ?? []
}

/** 紧急停止所有设备 */
export async function multipressStopAll(): Promise<void> {
  await apiPost('/multipress/stop-all')
}

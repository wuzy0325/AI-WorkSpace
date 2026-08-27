// 启动门禁开关 API：与后端 /api/v1/config/gates 同源，
// 让前端的「阀门=校准模式是启动必要条件」配置可由后端运维统一下发，
// 避免在前端硬编码 true/false 短路后端配置。

import { apiGet } from './client'

export interface GatesConfig {
  enforceValveCalibrationGate: boolean
}

/** 上次成功绑定的设备集合（供页面加载时恢复勾选；多设备按勾选顺序） */
export interface LastDevicesConfig {
  pressureDeviceId: string
  measureDeviceIds: string[]
}

/** 拉取当前启动门禁开关（标定 + 计量启动是否要求阀门=校准） */
export async function getGatesConfig(): Promise<GatesConfig> {
  return apiGet<GatesConfig>('/config/gates')
}

/** 拉取上次成功绑定的设备集合，恢复设备勾选 */
export async function fetchLastDevices(): Promise<LastDevicesConfig> {
  return apiGet<LastDevicesConfig>('/config/last-devices')
}

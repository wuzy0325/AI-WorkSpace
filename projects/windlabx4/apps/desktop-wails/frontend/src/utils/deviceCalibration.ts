/**
 * 校零业务共享常量与工具函数。
 *
 * 抽离原因：DeviceDetailPanel.vue 的 shouldDisableTare 与 DeviceOverviewPanel.vue 的
 * calibrateAllDevices 原本各自硬编码了设备类型白名单和温度单位列表，新增设备类型时
 * 容易漏改一处。集中到此文件后，两处消费同一份定义，消除双重维护。
 */
import type { DeviceType } from '@api/types'

/**
 * 支持校零的设备类型白名单。
 * 与后端 usecase.calibrationEnabledForProfile 行为对齐：
 *   - DAQ-P-1604 / DAQ-P-1604Pre：16 个压力通道可校零，CH17/CH18 为大气辅助通道不可校零
 *   - DAQ-P-1603：仅 calibrationEnabled=true 的压力通道可校零
 *   - DSA3217：压力通道可校零
 * 其他设备类型（SIMULATED / DAQ-T-1603 / DAQ-T-1602 / WTN_PXI）不支持校零。
 */
export const CALIBRATABLE_DEVICE_TYPES: readonly DeviceType[] = [
  'DAQ-P-1604',
  'DAQ-P-1604Pre',
  'DAQ-P-1603',
  'DSA3217',
] as const

/** 温度单位集合，温度通道不支持校零。与后端 UnitConverter.temperatureFamily 对齐。 */
export const TEMPERATURE_UNITS: readonly string[] = ['℃', '°C', 'degC', '℉', '°F', 'degF'] as const

/**
 * 判断设备类型是否支持校零。
 * 用于设备级 UI（如 DeviceOverviewPanel 的"全部校零"筛选）快速过滤目标设备。
 */
export function isCalibratableDeviceType(type: string): boolean {
  return (CALIBRATABLE_DEVICE_TYPES as readonly string[]).includes(type)
}

/**
 * 判断通道是否为温度通道（基于单位）。
 * 温度通道不参与校零，与后端 UnitConverter.SupportsZeroCalibration 行为一致。
 */
export function isTemperatureUnit(unit: string): boolean {
  return (TEMPERATURE_UNITS as readonly string[]).includes(unit)
}

/** 与后端 calibrationEnabledForProfile 对齐，DAQ-P-1603 尊重通道校零应用开关。 */
export function isChannelCalibrationEnabled(type: string, enabled: boolean | undefined): boolean {
  return type !== 'DAQ-P-1603' || enabled === true
}

/**
 * 通道单位展示工具函数。
 *
 * DAQ-P-1604 / DAQ-P-1604Pre 共 18 通道：
 *   - CH1~CH16（index 0-15）：压力通道，单位跟随全局压力单位
 *   - CH17（index 16）：大气压力 —— 物理量固定，UI 固定显示 Pa
 *   - CH18（index 17）：大气温度 —— 物理量固定，UI 固定显示 ℃
 *
 * 设计原则：无论全局压力单位如何切换，或 profile 中通道单位被其他命令改写，
 * UI 上这两个通道的单位始终固定，避免把大气通道误标为压力单位。
 * 与后端 defaultDAQP1604Channels / syncChannelsUnit 的语义保持一致。
 */
import type { DeviceType } from '@api/types'

/** 返回 DAQ-P-1604/Pre 大气辅助通道的固定单位；非此类设备或通道返回 null。 */
export function fixedAtmosphericUnit(deviceType: string | DeviceType, channelIndex: number): string | null {
  if (deviceType !== 'DAQ-P-1604' && deviceType !== 'DAQ-P-1604Pre') return null
  if (channelIndex === 16) return 'Pa'
  if (channelIndex === 17) return '℃'
  return null
}

/** 通道单位展示：大气辅助通道固定单位优先，其余通道回退到传入的 fallback 单位。 */
export function channelUnit(deviceType: string | DeviceType, channelIndex: number, fallback: string): string {
  return fixedAtmosphericUnit(deviceType, channelIndex) ?? fallback
}

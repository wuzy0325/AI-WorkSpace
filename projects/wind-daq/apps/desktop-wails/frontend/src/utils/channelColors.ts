/**
 * 通道颜色生成工具。
 *
 * 设计目标：
 *   - 保证 RealtimeChart 曲线颜色与 DeviceDetailPanel/ChartSelector 通道卡片颜色一致
 *   - DAQ-P-1603 按 SensorType 分色系（压力蓝、温度橙），其他设备沿用 8 色循环
 *   - 颜色与通道 index 一一对应，与用户在 ChartSelector 中选中的通道集合无关
 *
 * 实现要点：
 *   - 颜色映射按 profile.channels 顺序生成（而非按用户选中的 channelIndices），
 *     这样无论用户选哪些通道，每个通道的颜色都稳定
 *   - DAQ-P-1603 同类型在色板内按出现顺序取色，16 通道下最多重复 4 次（5 档色板），
 *     配合 tooltip 仍可区分
 */

/** DAQ-P-1603 压力通道色板（blue-300 ~ blue-700） */
export const PRESSURE_PALETTE = ['#60a5fa', '#3b82f6', '#2563eb', '#1d4ed8', '#1e40af']

/** DAQ-P-1603 温度通道色板（orange-300 ~ orange-700） */
export const TEMPERATURE_PALETTE = ['#fdba74', '#fb923c', '#f97316', '#ea580c', '#c2410c']

/** 通用 8 色循环（非 DAQ-P-1603 设备使用） */
export const CHANNEL_COLORS = [
  '#3b82f6',
  '#10b981',
  '#f59e0b',
  '#a855f7',
  '#f43f5e',
  '#06b6d4',
  '#f97316',
  '#6366f1',
] as const

/** 颜色映射输入：通道 index + 可选 sensorType */
export interface ChannelColorInput {
  index: number
  sensorType?: 'pressure' | 'temperature'
}

/**
 * 为设备所有通道生成稳定的颜色映射。
 *
 * @param profileType 设备类型（'DAQ-P-1603' 触发 SensorType 着色）
 * @param channels 通道列表（按 profile.channels 顺序）
 * @returns Map<channelIndex, color>
 *
 * 颜色稳定性：相同 profileType + 相同 channels 顺序 → 相同颜色映射。
 * 调用方应把结果缓存到 computed 中，避免每次渲染重复计算。
 */
export function buildChannelColorMap(
  profileType: string,
  channels: ChannelColorInput[],
): Map<number, string> {
  const map = new Map<number, string>()

  if (profileType === 'DAQ-P-1603') {
    let pressureIdx = 0
    let tempIdx = 0
    for (const ch of channels) {
      if (ch.sensorType === 'temperature') {
        map.set(ch.index, TEMPERATURE_PALETTE[tempIdx % TEMPERATURE_PALETTE.length])
        tempIdx += 1
      } else {
        map.set(ch.index, PRESSURE_PALETTE[pressureIdx % PRESSURE_PALETTE.length])
        pressureIdx += 1
      }
    }
    return map
  }

  channels.forEach((ch, i) => {
    map.set(ch.index, CHANNEL_COLORS[i % CHANNEL_COLORS.length])
  })
  return map
}

/**
 * 单通道颜色查询的便捷函数。
 *
 * 内部基于 buildChannelColorMap 实现，每次调用都会重建映射，
 * 性能敏感场景（如 series.map 循环内）应直接使用 buildChannelColorMap 缓存结果。
 */
export function pickChannelColor(
  profileType: string,
  channels: ChannelColorInput[],
  channelIndex: number,
): string {
  const map = buildChannelColorMap(profileType, channels)
  return map.get(channelIndex) ?? CHANNEL_COLORS[0]
}

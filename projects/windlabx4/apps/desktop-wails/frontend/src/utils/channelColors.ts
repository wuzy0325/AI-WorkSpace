/**
 * 通道颜色生成工具。
 *
 * 设计目标：
 *   - 保证 RealtimeChart 曲线颜色与 DeviceDetailPanel/ChartSelector 通道卡片颜色一致
 *   - DAQ-P-1603 按 SensorType 分色系：
 *       压力（16 档冷色族：蓝/青/绿/紫 等 8 色相 × 亮/深两档）
 *       温度（4 档暖色族：橙/红/黄/玫红）
 *   - 颜色与通道 index 一一对应，与用户在 ChartSelector 中选中的通道集合无关
 *
 * 实现要点：
 *   - 颜色映射按 profile.channels 顺序生成（而非按用户选中的 channelIndices），
 *     这样无论用户选哪些通道，每个通道的颜色都稳定
 *   - 压力色板 16 档恰好覆盖 DAQ-P-1603 最大 16 通道，零循环：
 *       前 8 档亮色（CH1-CH8，L≈55%），后 8 档深色（CH9-CH16，L≈45%）
 *       相邻通道色相不同，隔 8 位明度不同，全 16 通道互不重色
 *   - 温度色板 4 档覆盖实际场景（温度通道少，一般 1-2 个，最多 4 个）：
 *       暖色族与压力的冷色族形成强对比，温度通道在压力曲线中极其醒目
 *   - 第一档温度色保持橙色 #f97316，与历史版本一致，保留用户视觉记忆
 */

/**
 * DAQ-P-1603 压力通道色板（16 档冷色族）。
 * 8 个色相 × 2 明度：前 8 档亮色，后 8 档同色相深色版。
 * 16 通道全压力时零循环，相邻通道色相不同、隔 8 位明度不同。
 */
export const PRESSURE_PALETTE = [
  // 亮色族（CH1-CH8，L≈55%）
  '#3b82f6', // 蓝
  '#06b6d4', // 青
  '#0ea5e9', // 天蓝
  '#14b8a6', // 青绿
  '#10b981', // 翠绿
  '#65a30d', // 黄绿（lime-600，原 #84cc16 因 light 主题对比度 1.89 不足 3:1 调暗）
  '#8b5cf6', // 紫
  '#a855f7', // 紫红
  // 深色族（CH9-CH16，与 CH1-CH8 同色相但明度更低，L≈45%）
  '#2563eb', // 深蓝
  '#0891b2', // 深青
  '#0369a1', // 深天蓝
  '#0f766e', // 深青绿
  '#047857', // 深翠绿
  '#4d7c0f', // 深黄绿
  '#7c3aed', // 深紫
  '#c026d3', // 深紫红（fuchsia-600，原 #86198f 因 dark 主题对比度 2.17 不足 3:1 调亮）
] as const

/**
 * DAQ-P-1603 温度通道色板（4 档暖色族）。
 * 温度通道少（一般 1-2 个，最多 4 个），4 档够用。
 * 暖色与压力冷色形成强对比，温度通道在压力曲线中视觉上极其醒目。
 * 第一档保持橙色 #f97316，与历史版本一致，保留用户视觉记忆。
 */
export const TEMPERATURE_PALETTE = [
  '#f97316', // 橙
  '#ef4444', // 红
  '#a16207', // 黄（yellow-700，原 #eab308 因 light 主题对比度 1.83 不足 3:1 调暗）
  '#ec4899', // 玫红
] as const

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

import { describe, it, expect } from 'vitest'
import {
  PRESSURE_PALETTE,
  TEMPERATURE_PALETTE,
  CHANNEL_COLORS,
  buildChannelColorMap,
  pickChannelColor,
  type ChannelColorInput,
} from '@utils/channelColors'

/** 构造 N 个压力通道输入 */
function makePressureChannels(n: number): ChannelColorInput[] {
  return Array.from({ length: n }, (_, i) => ({ index: i, sensorType: 'pressure' as const }))
}

/** 构造 N 个温度通道输入 */
function makeTemperatureChannels(n: number): ChannelColorInput[] {
  return Array.from({ length: n }, (_, i) => ({ index: i, sensorType: 'temperature' as const }))
}

describe('PRESSURE_PALETTE - 压力色板', () => {
  it('长度为 16，覆盖 DAQ-P-1603 最大 16 通道', () => {
    expect(PRESSURE_PALETTE.length).toBe(16)
  })

  it('16 档颜色互不重复，零循环', () => {
    expect(new Set(PRESSURE_PALETTE).size).toBe(16)
  })

  it('所有颜色为小写 hex 格式，与现有色板风格一致', () => {
    for (const c of PRESSURE_PALETTE) {
      expect(c).toMatch(/^#[0-9a-f]{6}$/)
    }
  })
})

describe('TEMPERATURE_PALETTE - 温度色板', () => {
  it('长度为 4，覆盖温度通道实际场景（1-2 个，最多 4 个）', () => {
    expect(TEMPERATURE_PALETTE.length).toBe(4)
  })

  it('首档保持橙色 #f97316，保留用户视觉记忆', () => {
    expect(TEMPERATURE_PALETTE[0]).toBe('#f97316')
  })

  it('4 档颜色互不重复', () => {
    expect(new Set(TEMPERATURE_PALETTE).size).toBe(4)
  })
})

describe('buildChannelColorMap - DAQ-P-1603 压力色板', () => {
  it('16 通道全压力时零循环，颜色互不重复', () => {
    // 测试前置：构造 16 通道全压力输入
    const channels = makePressureChannels(16)

    // 测试步骤：调用 buildChannelColorMap
    const map = buildChannelColorMap('DAQ-P-1603', channels)

    // 期待结果：16 个颜色互不重复
    expect(map.size).toBe(16)
    const colors = Array.from(map.values())
    expect(new Set(colors).size).toBe(16)
  })

  it('压力通道颜色按出现顺序取自 PRESSURE_PALETTE', () => {
    const channels = makePressureChannels(3)
    const map = buildChannelColorMap('DAQ-P-1603', channels)
    expect(map.get(0)).toBe(PRESSURE_PALETTE[0])
    expect(map.get(1)).toBe(PRESSURE_PALETTE[1])
    expect(map.get(2)).toBe(PRESSURE_PALETTE[2])
  })

  it('单压力通道返回色板首档', () => {
    const map = buildChannelColorMap('DAQ-P-1603', [{ index: 0, sensorType: 'pressure' }])
    expect(map.get(0)).toBe(PRESSURE_PALETTE[0])
  })

  it('超过 16 个压力通道时按色板循环（理论场景，实际 DAQ-P-1603 不会触发）', () => {
    const channels = makePressureChannels(20)
    const map = buildChannelColorMap('DAQ-P-1603', channels)
    // 第 17 个通道（index=16）应循环回色板首档
    expect(map.get(16)).toBe(PRESSURE_PALETTE[0])
    expect(map.get(19)).toBe(PRESSURE_PALETTE[3])
  })
})

describe('buildChannelColorMap - DAQ-P-1603 温度色板', () => {
  it('单温度通道返回橙色 #f97316，视觉记忆不破坏', () => {
    const map = buildChannelColorMap('DAQ-P-1603', [{ index: 0, sensorType: 'temperature' }])
    expect(map.get(0)).toBe('#f97316')
  })

  it('多温度通道按出现顺序取自 TEMPERATURE_PALETTE', () => {
    const channels = makeTemperatureChannels(4)
    const map = buildChannelColorMap('DAQ-P-1603', channels)
    expect(map.get(0)).toBe(TEMPERATURE_PALETTE[0])
    expect(map.get(1)).toBe(TEMPERATURE_PALETTE[1])
    expect(map.get(2)).toBe(TEMPERATURE_PALETTE[2])
    expect(map.get(3)).toBe(TEMPERATURE_PALETTE[3])
  })

  it('超过 4 个温度通道时按色板循环（理论场景，实际不会触发）', () => {
    const channels = makeTemperatureChannels(6)
    const map = buildChannelColorMap('DAQ-P-1603', channels)
    expect(map.get(4)).toBe(TEMPERATURE_PALETTE[0])
    expect(map.get(5)).toBe(TEMPERATURE_PALETTE[1])
  })
})

describe('buildChannelColorMap - DAQ-P-1603 混合场景', () => {
  it('14 压力 + 2 温度：16 个颜色互不重复', () => {
    // 测试前置：14 个压力通道 + 2 个温度通道
    const channels: ChannelColorInput[] = [
      ...makePressureChannels(14),
      ...makeTemperatureChannels(2).map((c, i) => ({ ...c, index: 14 + i })),
    ]

    // 测试步骤：调用 buildChannelColorMap
    const map = buildChannelColorMap('DAQ-P-1603', channels)

    // 期待结果：16 个颜色互不重复
    expect(map.size).toBe(16)
    expect(new Set(map.values()).size).toBe(16)
  })

  it('温度通道颜色为暖色族，与压力冷色族形成强对比', () => {
    const channels: ChannelColorInput[] = [
      ...makePressureChannels(15),
      { index: 15, sensorType: 'temperature' },
    ]
    const map = buildChannelColorMap('DAQ-P-1603', channels)

    // 温度通道必须是 TEMPERATURE_PALETTE 中的颜色
    const tempColor = map.get(15)!
    expect([...TEMPERATURE_PALETTE]).toContain(tempColor)

    // 压力通道必须是 PRESSURE_PALETTE 中的颜色
    const pressureColor = map.get(0)!
    expect([...PRESSURE_PALETTE]).toContain(pressureColor)
  })

  it('无 sensorType 的通道默认按压力处理', () => {
    const channels: ChannelColorInput[] = [
      { index: 0 }, // 无 sensorType
      { index: 1, sensorType: 'temperature' },
    ]
    const map = buildChannelColorMap('DAQ-P-1603', channels)
    expect(map.get(0)).toBe(PRESSURE_PALETTE[0])
    expect(map.get(1)).toBe(TEMPERATURE_PALETTE[0])
  })
})

describe('buildChannelColorMap - 非 DAQ-P-1603 设备', () => {
  it('DAQ-P-1604 走 CHANNEL_COLORS 8 色循环，不受本次改动影响', () => {
    const channels: ChannelColorInput[] = Array.from({ length: 8 }, (_, i) => ({ index: i }))
    const map = buildChannelColorMap('DAQ-P-1604', channels)
    expect(map.size).toBe(8)
    expect(map.get(0)).toBe(CHANNEL_COLORS[0])
    expect(map.get(7)).toBe(CHANNEL_COLORS[7])
  })

  it('SIMULATED 设备也走 8 色循环', () => {
    const channels: ChannelColorInput[] = Array.from({ length: 4 }, (_, i) => ({ index: i }))
    const map = buildChannelColorMap('SIMULATED', channels)
    expect(map.get(0)).toBe(CHANNEL_COLORS[0])
  })

  it('非 DAQ-P-1603 设备 sensorType 不影响颜色（沿用 8 色循环）', () => {
    // 即使带了 sensorType='temperature'，非 P-1603 设备也不分色系
    const channels: ChannelColorInput[] = [
      { index: 0, sensorType: 'temperature' },
      { index: 1, sensorType: 'pressure' },
    ]
    const map = buildChannelColorMap('DAQ-P-1604', channels)
    expect(map.get(0)).toBe(CHANNEL_COLORS[0])
    expect(map.get(1)).toBe(CHANNEL_COLORS[1])
  })
})

describe('buildChannelColorMap - 边界条件', () => {
  it('空通道列表返回空 Map', () => {
    const map = buildChannelColorMap('DAQ-P-1603', [])
    expect(map.size).toBe(0)
  })

  it('空通道列表对非 P-1603 设备也返回空 Map', () => {
    const map = buildChannelColorMap('DAQ-P-1604', [])
    expect(map.size).toBe(0)
  })

  it('通道 index 不连续也能正常工作', () => {
    const channels: ChannelColorInput[] = [
      { index: 5, sensorType: 'pressure' },
      { index: 10, sensorType: 'pressure' },
    ]
    const map = buildChannelColorMap('DAQ-P-1603', channels)
    // 按出现顺序取色，与 index 数值无关
    expect(map.get(5)).toBe(PRESSURE_PALETTE[0])
    expect(map.get(10)).toBe(PRESSURE_PALETTE[1])
  })
})

describe('pickChannelColor - 便捷函数', () => {
  it('单通道查询与 buildChannelColorMap 结果一致', () => {
    const channels = makePressureChannels(5)
    const map = buildChannelColorMap('DAQ-P-1603', channels)

    for (let i = 0; i < 5; i++) {
      expect(pickChannelColor('DAQ-P-1603', channels, i)).toBe(map.get(i))
    }
  })

  it('查询不存在的通道返回 CHANNEL_COLORS 首档作为兜底', () => {
    const channels = makePressureChannels(2)
    // index=99 不在 channels 中
    expect(pickChannelColor('DAQ-P-1603', channels, 99)).toBe(CHANNEL_COLORS[0])
  })
})

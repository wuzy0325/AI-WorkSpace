import type { core } from '../../wailsjs/go/models'

// 复用 Wails 生成的类型，避免重复定义导致与后端模型不同步
export type MockT1603Config = core.T1603Config
export type MockChannelConfig = core.ChannelConfig
export type MockDeviceProfile = core.TemperatureProfile
export type MockScanResult = core.ScanResult

export const CHANNEL_COLORS = [
  '#3b82f6', '#10b981', '#f59e0b', '#a855f7',
  '#f43f5e', '#06b6d4', '#f97316', '#6366f1',
  '#84cc16', '#14b8a6', '#d946ef', '#0ea5e9',
  '#eab308', '#22c55e', '#ef4444', '#8b5cf6',
]

/** Mock 通道数 */
const CHANNEL_COUNT = 16

export function defaultChannels(): MockChannelConfig[] {
  return Array.from({ length: CHANNEL_COUNT }, (_, i) => ({
    index: i,
    name: `通道 ${i + 1}`,
    enabled: true,
    unit: '°C',
    color: CHANNEL_COLORS[i % CHANNEL_COLORS.length],
    precision: 2,
    rangeMin: 0,
    rangeMax: 200,
    thermocoupleType: 'K',
  }))
}

export function defaultT1603Config(): MockT1603Config {
  return {
    thermocoupleTypes: 'KKKKKKKKKKKKKKKK',
    channelMask: 'FFFF',
    samplingRate: 10,
    averageCount: 4,
    showTimestamp: true,
    showSequence: false,
    autoConnect: false,
  }
}

export function createProfile(id: string, overrides?: Partial<MockDeviceProfile>): MockDeviceProfile {
  return {
    id,
    name: `Device-${id}`,
    address: '192.168.1.10',
    port: 9000,
    samplingRate: 10,
    channels: defaultChannels(),
    t1603Config: defaultT1603Config(),
    createdAt: Date.now(),
    ...overrides,
  }
}

export function createMultiProfiles(count: number): MockDeviceProfile[] {
  return Array.from({ length: count }, (_, i) =>
    createProfile(`dev_${i + 1}`, {
      name: `T1603-${i + 1}`,
      address: `192.168.3.${101 + i}`,
      port: 9000,
    })
  )
}

export const SINGLE_DEVICE = createProfile('test_dev_1', {
  name: '测试设备',
  address: '192.168.1.10',
  port: 9000,
})

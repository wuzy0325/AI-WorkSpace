/**
 * 遍历测试实时数据 composable
 *
 * 从 TraversalMain.vue 中提取的实时压力数据处理逻辑：
 *   - DAQ snapshot 订阅与最新快照管理
 *   - 从快照构建实时压力映射（LivePressureMap）
 *   - 将压力映射转换为插值输入（TraversalInterpolationInput）
 *   - 压力通道 UI 展示项（pressureItems）
 *   - 是否存在实时计算结果的判断（hasRealtimeResult）
 */

import { computed, type ComputedRef, type Ref, ref } from 'vue'
import type {
  TraversalTestConfig,
  TraversalInterpolationInput
} from '@shared/types/traversal'
import type { DataPayload } from '@api/types'
import type { ProbeChannelRole } from '@shared/types/calibration'
import { deviceApi } from '@api/deviceApi'
import { useDeviceStore } from '@stores/deviceStore'
import { useTraversalStore } from '@stores/traversalStore'

/** 实时压力通道键名 */
type LivePressureKey = 'P1' | 'P2' | 'P3' | 'P4' | 'P5' | 'Patm' | 'Tatm'

/** 实时压力映射（部分通道可能缺失） */
type LivePressureMap = Partial<Record<LivePressureKey, number>>

/** 压力通道 UI 展示项 */
export interface PressureItem {
  key: string
  label: string
  unit: string
  value: string
  disabled: boolean
}

/**
 * 从 DAQ 快照数据构建实时压力映射
 *
 * 遍历配置中的探针通道，按角色（fiveHole.p1 等）匹配快照中的
 * 设备 ID 与通道索引，将采集值映射到 P1~P5、Patm、Tatm。
 *
 * @param config 遍历测试配置
 * @param snapshots 最新 DAQ 快照数组
 * @returns 匹配到至少一个通道时返回 LivePressureMap，否则返回 null
 */
function buildRealtimePressuresFromSnapshots(
  config: TraversalTestConfig,
  snapshots: DataPayload[]
): LivePressureMap | null {
  // 根据设备 ID 和通道索引从快照中提取数值
  const toValue = (deviceId: string, channelIndex: number): number | undefined => {
    const payload = snapshots.find((entry) => entry.deviceId === deviceId)
    if (!payload) return undefined
    const indices = Array.isArray(payload.channelIndices) ? payload.channelIndices : []
    const channels = Array.isArray(payload.channels) ? payload.channels : []
    const index = indices.indexOf(channelIndex)
    if (index < 0) return undefined
    const value = channels[index]
    return typeof value === 'number' ? value : undefined
  }

  const result: LivePressureMap = {}
  let matchedChannelCount = 0

  for (const channel of config.channels.probeChannels) {
    if (!channel.enabled || !channel.channel.deviceId) continue

    const value = toValue(channel.channel.deviceId, channel.channel.channelIndex)
    if (typeof value !== 'number') continue

    switch (channel.role) {
      case 'fiveHole.p1':
        result.P1 = value
        matchedChannelCount += 1
        break
      case 'fiveHole.p2':
        result.P2 = value
        matchedChannelCount += 1
        break
      case 'fiveHole.p3':
        result.P3 = value
        matchedChannelCount += 1
        break
      case 'fiveHole.p4':
        result.P4 = value
        matchedChannelCount += 1
        break
      case 'fiveHole.p5':
        result.P5 = value
        matchedChannelCount += 1
        break
      case 'fiveHole.pAtm':
        result.Patm = value
        matchedChannelCount += 1
        break
      case 'fiveHole.tAtm':
        result.Tatm = value
        matchedChannelCount += 1
        break
      default:
        break
    }
  }

  return matchedChannelCount > 0 ? result : null
}

/**
 * 将实时压力映射转换为插值输入
 *
 * 仅当所有 7 个通道（P1~P5、Patm、Tatm）均存在时才返回有效输入，
 * 否则返回 null（插值计算需要完整的压力数据）。
 *
 * @param pressures 实时压力映射
 * @returns 完整的插值输入，或 null
 */
function toRealtimeInterpolationInput(pressures: LivePressureMap | null): TraversalInterpolationInput | null {
  if (!pressures) {
    return null
  }

  const { P1, P2, P3, P4, P5, Patm, Tatm } = pressures
  if (
    typeof P1 !== 'number'
    || typeof P2 !== 'number'
    || typeof P3 !== 'number'
    || typeof P4 !== 'number'
    || typeof P5 !== 'number'
    || typeof Patm !== 'number'
    || typeof Tatm !== 'number'
  ) {
    return null
  }

  return { P1, P2, P3, P4, P5, Patm, Tatm }
}

/**
 * 遍历测试实时数据 composable
 *
 * @param config 遍历测试配置的响应式引用（可为 null 表示未配置）
 * @returns 实时数据相关的响应式状态与方法
 */
export function useTraversalRealtimeData(config: Ref<TraversalTestConfig | null>) {
  const traversalStore = useTraversalStore()
  const deviceStore = useDeviceStore()

  /**
   * 根据探针通道角色从设备配置中获取通道单位
   * 查找逻辑：probeChannels 中匹配 role → 设备配置 channels 中对应索引的 unit
   */
  function getChannelUnit(role: ProbeChannelRole, fallback: string): string {
    if (!config.value) return fallback
    const ch = config.value.channels.probeChannels.find((c) => c.role === role)
    if (!ch?.channel.deviceId) return fallback
    const device = deviceStore.profiles?.find((d) => d.id === ch.channel.deviceId)
    const channelConfig = device?.channels[ch.channel.channelIndex]
    return channelConfig?.unit ?? fallback
  }

  // 最新 DAQ 快照（按 deviceId 去重，保留每个设备的最新数据）
  const latestSnapshots = ref<DataPayload[]>([])

  // 实时压力映射：从快照中提取的各通道压力值
  const livePressures: ComputedRef<LivePressureMap | null> = computed(() => {
    if (!config.value || latestSnapshots.value.length === 0) {
      return null
    }
    return buildRealtimePressuresFromSnapshots(config.value, latestSnapshots.value)
  })

  // 实时插值输入：所有通道齐全时可用于插值计算
  const liveInterpolationInput: ComputedRef<TraversalInterpolationInput | null> = computed(
    () => toRealtimeInterpolationInput(livePressures.value)
  )

  // 是否存在实时计算结果
  const hasRealtimeResult: ComputedRef<boolean> = computed(
    () => traversalStore.realtimeResult !== null
  )

  // 压力通道 UI 展示项：优先使用实时数据，回退到 store 中的历史压力
  const pressureItems: ComputedRef<PressureItem[]> = computed(() => {
    const data = livePressures.value ?? traversalStore.realtimePressures
    const formatValue = (value?: number): string => (typeof value === 'number' ? value.toFixed(3) : '--')
    const hasConfig = config.value !== null
    return [
      { key: 'P1', label: 'P1', unit: getChannelUnit('fiveHole.p1', 'Pa'), value: formatValue(data?.P1), disabled: !hasConfig },
      { key: 'P2', label: 'P2', unit: getChannelUnit('fiveHole.p2', 'Pa'), value: formatValue(data?.P2), disabled: !hasConfig },
      { key: 'P3', label: 'P3', unit: getChannelUnit('fiveHole.p3', 'Pa'), value: formatValue(data?.P3), disabled: !hasConfig },
      { key: 'P4', label: 'P4', unit: getChannelUnit('fiveHole.p4', 'Pa'), value: formatValue(data?.P4), disabled: !hasConfig },
      { key: 'P5', label: 'P5', unit: getChannelUnit('fiveHole.p5', 'Pa'), value: formatValue(data?.P5), disabled: !hasConfig },
      { key: 'Patm', label: 'Patm', unit: getChannelUnit('fiveHole.pAtm', 'Pa'), value: formatValue(data?.Patm), disabled: !hasConfig },
      { key: 'Tatm', label: 'Tatm', unit: getChannelUnit('fiveHole.tAtm', '°C'), value: formatValue(data?.Tatm), disabled: !hasConfig }
    ]
  })

  /**
   * 订阅 DAQ snapshot 事件
   *
   * 每次收到快照时，按 deviceId 替换已有条目，确保 latestSnapshots
   * 始终保存每个设备的最新一帧数据。
   *
   * @returns 取消订阅函数
   */
  function subscribeSnapshot(): () => void {
    return deviceApi.onSnapshot((snapshot: DataPayload) => {
      const next = latestSnapshots.value.filter((item) => item.deviceId !== snapshot.deviceId)
      latestSnapshots.value = [...next, snapshot]
    })
  }

  return {
    livePressures,
    liveInterpolationInput,
    pressureItems,
    latestSnapshots,
    subscribeSnapshot,
    hasRealtimeResult
  }
}

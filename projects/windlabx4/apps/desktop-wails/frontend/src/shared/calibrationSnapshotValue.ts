/**
 * 校准/遍历实时采集快照通道值查找工具。
 *
 * 4 处调用方（FiveHoleMain / ThreeHoleMain / TotalPressureMain / useTraversalRealtimeData）
 * 之前各自维护一份相同的 toValue 实现，任一接口变更需同步改 4 处。
 * 抽到 shared 统一维护，调用方仅保留 role → field 映射逻辑（因各校准类型通道语义不同）。
 */
import type { DataPayload } from '@api/types'

/**
 * 根据设备 ID 与通道索引从采集快照数组中提取数值。
 *
 * @param snapshots DAQ 推送的快照数组（每个元素含 deviceId + channelIndices + channels）
 * @param deviceId 设备 ID
 * @param channelIndex 通道索引（对应 channelIndices 中的值）
 * @returns 找到且为数值时返回该值；设备不存在、通道未找到或值非数值时返回 null
 */
export function findChannelValue(
  snapshots: DataPayload[],
  deviceId: string,
  channelIndex: number,
): number | null {
  const payload = snapshots.find((p) => p.deviceId === deviceId)
  if (!payload) return null
  const indices = Array.isArray(payload.channelIndices) ? payload.channelIndices : []
  const channels = Array.isArray(payload.channels) ? payload.channels : []
  const i = indices.indexOf(channelIndex)
  if (i === -1) return null
  const v = channels[i]
  return typeof v === 'number' ? v : null
}

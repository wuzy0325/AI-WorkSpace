// 通道批量操作 composable
//
// 仿照遍历测试 TraversalHardwareStep.vue 的"统一选择设备 + 应用到全部通道"
// 与"起始通道号 + 自动递增填充"两个批量操作，提取为通用 composable，
// 供五孔 / 三孔 / 总压 / 总温探针校准配置复用。
//
// 设计目标：
//   1. 单一真源：批量操作逻辑集中维护，避免四处复制粘贴导致的漂移；
//   2. 类型安全：通过泛型 TChannel 约束通道结构，保证 channel.deviceId / channelIndex 字段存在；
//   3. 响应式：batchDeviceId / autoFillStartIndex 暴露为 ref，组件可直接 v-model 绑定；
//   4. 副作用隔离：只操作传入的 channels ref，不引入其它全局状态。
//
// 使用方式：
//   const { batchDeviceId, autoFillStartIndex, applyDeviceToAll, autoFillChannelIndices }
//     = useChannelBatchOperations(probeChannels)

import { ref, type Ref } from 'vue'

// 通道结构的最低约束：必须包含 channel.deviceId 与 channel.channelIndex
export interface BatchChannelLike {
  enabled: boolean
  channel: {
    deviceId: string
    channelIndex: number
  }
}

/**
 * 通道批量操作
 *
 * @param channels 通道列表的响应式 ref（组件中通常为 ref<ProbeChannelConfig[]>）
 * @returns batchDeviceId / autoFillStartIndex 响应式状态，以及两个批量操作函数
 */
export function useChannelBatchOperations<T extends BatchChannelLike>(channels: Ref<T[]>) {
  // 统一设备选择：选定后通过 applyDeviceToAll 一次性写入所有已启用通道
  const batchDeviceId = ref<string>('')

  // 自动递增起始通道号：null 表示未选择，避免 0 与"未选择"歧义
  const autoFillStartIndex = ref<number | null>(null)

  /**
   * 将当前选中的设备应用到所有已启用通道
   *
   * 仅覆盖已启用（enabled=true）的通道，禁用通道保留原值以便用户随时启用。
   * 通过展开运算符创建新的 channel 对象，保证 Vue 响应式追踪触发。
   */
  function applyDeviceToAll(): void {
    if (!batchDeviceId.value) return
    channels.value.forEach((ch) => {
      if (ch.enabled) {
        ch.channel = { ...ch.channel, deviceId: batchDeviceId.value }
      }
    })
  }

  /**
   * 从起始通道号开始自动递增填充所有已启用通道
   *
   * 按通道在列表中的顺序依次赋值 channelIndex：start, start+1, start+2, ...
   * 仅覆盖已启用通道，禁用通道不占用序号，避免启用后顺序错乱。
   */
  function autoFillChannelIndices(): void {
    if (autoFillStartIndex.value === null) return
    let idx = autoFillStartIndex.value
    channels.value.forEach((ch) => {
      if (ch.enabled) {
        ch.channel = { ...ch.channel, channelIndex: idx }
        idx++
      }
    })
  }

  return {
    batchDeviceId,
    autoFillStartIndex,
    applyDeviceToAll,
    autoFillChannelIndices,
  }
}
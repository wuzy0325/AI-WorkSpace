<template>
  <section class="batch-group-view">
    <header class="panel-header">
      <h3>自动分组结果</h3>
      <span class="batch-count">共 {{ batches.length }} 批</span>
    </header>

    <!-- 一致性警告 -->
    <div
      v-if="conflictChannels.length > 0"
      class="warning-banner"
    >
      <span class="warning-icon">⚠</span>
      <span>以下通道量程与所在批次不一致：{{ conflictChannels.join(', ') }}</span>
    </div>

    <!-- 批次卡片列表 -->
    <div class="batch-cards">
      <div
        v-for="(batch, batchIdx) in batches"
        :key="batch.batchId"
        class="batch-card"
        :class="{ conflict: hasConflictInBatch(batch) }"
      >
        <header class="batch-card-header">
          <span class="batch-title">批次 {{ batch.batchIndex }}</span>
          <span class="batch-range">{{ batch.rangeMin }}~{{ batch.rangeMax }} {{ batch.rangeUnit }}</span>
        </header>

        <!-- 通道列表（可移除） -->
        <div class="channel-list">
          <span
            v-for="ch in batch.channels"
            :key="ch.channelId"
            class="channel-chip"
            :class="{ conflict: !channelMatchesBatch(ch, batch) }"
          >
            CH{{ ch.channelId }}
            <button
              class="remove-btn"
              title="移出本批"
              @click="removeChannel(batchIdx, ch.channelId)"
            >×</button>
          </span>
          <span
            v-if="batch.channels.length === 0"
            class="empty-hint"
          >无通道</span>
        </div>

        <!-- 待分配通道可加入 -->
        <div
          v-if="unassignedChannels.length > 0"
          class="add-channel-area"
        >
          <select
            v-model="pendingAdd[batchIdx]"
            class="add-select"
          >
            <option :value="null">
              选择通道加入...
            </option>
            <option
              v-for="ch in unassignedChannels"
              :key="ch.channelId"
              :value="ch.channelId"
            >
              CH{{ ch.channelId }} ({{ ch.rangeMin }}~{{ ch.rangeMax }} {{ ch.rangeUnit }})
            </option>
          </select>
          <button
            class="add-btn"
            :disabled="pendingAdd[batchIdx] == null"
            @click="addChannel(batchIdx)"
          >
            加入
          </button>
        </div>
      </div>
    </div>

    <footer class="panel-footer">
      <button
        class="confirm-btn"
        :disabled="!allBatchesValid"
        @click="handleConfirm"
      >
        确认分组并开始
      </button>
      <span
        v-if="!allBatchesValid"
        class="hint"
      >每个批次内通道量程必须一致</span>
    </footer>
  </section>
</template>

<script setup lang="ts">
import { ref, computed, reactive } from 'vue'
import { type ChannelRange, type BatchGroup } from '@/types/batch'

const props = defineProps<{
  /** 16 通道量程配置（来自 BatchRangeInput） */
  channelRanges: ChannelRange[]
}>()

const emit = defineEmits<{
  /** 分组确认时触发 */
  confirm: [batches: BatchGroup[]]
}>()

// 待加入通道的临时选择状态：batchIndex -> channelId | null
const pendingAdd = reactive<Record<number, number | null>>({})

// 自动分组：按 (rangeMin, rangeMax, rangeUnit) 三元组分组，按 rangeMax 升序
const batches = ref<BatchGroup[]>(generateBatches(props.channelRanges))

// 当 props.channelRanges 变化时重新分组（由父组件控制时机）
const regenerate = (newRanges: ChannelRange[]): void => {
  batches.value = generateBatches(newRanges)
}

// 暴露给父组件
defineExpose({ regenerate })

// 自动分组算法
//
// 设计：相同量程区间 (min, max, unit) 的通道归为一批。
// 排序：按 rangeMax 升序——满量程上限小的批次先执行，
// 避免大压力批次对小量程传感器造成损坏风险。
function generateBatches(ranges: ChannelRange[]): BatchGroup[] {
  // 过滤掉 skipped 通道和无效量程（max 必须 > min）
  const valid = ranges.filter((r) => !r.skipped && r.rangeMax > r.rangeMin)

  // 按三元组 (rangeMin, rangeMax, rangeUnit) 分组
  const groupMap = new Map<string, ChannelRange[]>()
  for (const ch of valid) {
    const key = `${ch.rangeMin}|${ch.rangeMax}|${ch.rangeUnit}`
    if (!groupMap.has(key)) {
      groupMap.set(key, [])
    }
    groupMap.get(key)!.push(ch)
  }

  // 转为 BatchGroup 数组，按 rangeMax 升序排列
  // 同 rangeMax 时按 rangeMin 升序作为次要排序键，保证排序稳定可预期
  const result: BatchGroup[] = []
  let idx = 1
  const sortedKeys = Array.from(groupMap.keys()).sort((a, b) => {
    const [minA, maxA] = a.split('|').map(parseFloat)
    const [minB, maxB] = b.split('|').map(parseFloat)
    if (maxA !== maxB) return maxA - maxB
    return minA - minB
  })

  for (const key of sortedKeys) {
    const channels = groupMap.get(key)!
    const first = channels[0]
    result.push({
      batchId: `batch-${idx}`,
      batchIndex: idx,
      rangeMin: first.rangeMin,
      rangeMax: first.rangeMax,
      rangeUnit: first.rangeUnit,
      channels: [...channels],
      status: 'pending'
    })
    idx++
  }

  return result
}

// 通道量程是否与批次一致（三元组全等）
const channelMatchesBatch = (ch: ChannelRange, batch: BatchGroup): boolean => {
  return (
    ch.rangeMin === batch.rangeMin &&
    ch.rangeMax === batch.rangeMax &&
    ch.rangeUnit === batch.rangeUnit
  )
}

// 批次内是否存在量程不一致的通道
const hasConflictInBatch = (batch: BatchGroup): boolean => {
  return batch.channels.some((ch) => !channelMatchesBatch(ch, batch))
}

// 所有冲突通道列表（用于顶部警告）
const conflictChannels = computed<string[]>(() => {
  const result: string[] = []
  for (const batch of batches.value) {
    for (const ch of batch.channels) {
      if (!channelMatchesBatch(ch, batch)) {
        result.push(`CH${ch.channelId}`)
      }
    }
  }
  return result
})

// 未分配到任何批次的通道
const unassignedChannels = computed<ChannelRange[]>(() => {
  const assignedIds = new Set<number>()
  for (const batch of batches.value) {
    for (const ch of batch.channels) {
      assignedIds.add(ch.channelId)
    }
  }
  return props.channelRanges.filter((ch) => !assignedIds.has(ch.channelId) && ch.rangeMax > ch.rangeMin)
})

// 所有批次均合法（无冲突，且每批至少 1 个通道）
const allBatchesValid = computed<boolean>(() => {
  if (batches.value.length === 0) return false
  return batches.value.every((b) => b.channels.length > 0 && !hasConflictInBatch(b))
})

// 从批次中移除通道
const removeChannel = (batchIdx: number, channelId: number): void => {
  const batch = batches.value[batchIdx]
  batch.channels = batch.channels.filter((ch) => ch.channelId !== channelId)
}

// 向批次中添加通道
const addChannel = (batchIdx: number): void => {
  const channelId = pendingAdd[batchIdx]
  if (channelId == null) return

  const ch = props.channelRanges.find((c) => c.channelId === channelId)
  if (!ch) return

  // 如果该通道已在其他批次中，先移除
  for (const b of batches.value) {
    b.channels = b.channels.filter((c) => c.channelId !== channelId)
  }

  batches.value[batchIdx].channels.push({ ...ch })
  pendingAdd[batchIdx] = null
}

// 确认分组
const handleConfirm = (): void => {
  if (!allBatchesValid.value) return
  emit('confirm', batches.value.map((b) => ({ ...b })))
}
</script>

<style scoped lang="scss">
.batch-group-view {
  background: #fff;
  border: 1px solid $slate-200;
  border-radius: 8px;
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.panel-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.panel-header h3 {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
  color: $slate-800;
}

.batch-count {
  font-size: 14px;
  color: $slate-500;
}

/* 警告横幅： amber 色系 */
.warning-banner {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  background: $warning-50;
  border: 1px solid $warning-500;
  border-radius: 6px;
  color: $warning-700;
  font-size: 13px;
}

.warning-icon {
  font-size: 16px;
}

.batch-cards {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.batch-card {
  border: 1px solid $slate-200;
  border-radius: 6px;
  padding: 12px;
  background: $slate-50;
}

.batch-card.conflict {
  border-color: $danger-500;
  background: $danger-50;
}

.batch-card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 8px;
}

.batch-title {
  font-weight: 600;
  font-size: 14px;
  color: $slate-800;
}

.batch-range {
  font-size: 18px;
  font-weight: 700;
  color: $mint-dark;
}

.channel-list {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  min-height: 28px;
}

.channel-chip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 2px 8px;
  background: rgba(16, 185, 129, 0.12);
  border: 1px solid rgba(16, 185, 129, 0.3);
  border-radius: 12px;
  font-size: 12px;
  color: $slate-700;
}

.channel-chip.conflict {
  background: $danger-100;
  border-color: $danger-300;
}

.remove-btn {
  border: none;
  background: transparent;
  cursor: pointer;
  font-size: 14px;
  color: $slate-500;
  padding: 0;
  line-height: 1;
  transition: color 0.15s ease;

  &:hover {
    color: $danger-500;
  }
}

.empty-hint {
  color: $slate-400;
  font-size: 12px;
}

.add-channel-area {
  display: flex;
  gap: 4px;
  margin-top: 8px;
}

.add-select {
  flex: 1;
  padding: 4px 6px;
  border: 1px solid $slate-200;
  border-radius: 4px;
  font-size: 12px;
}

/* 添加按钮：小号 mint 主按钮 */
.add-btn {
  padding: 4px 12px;
  background: linear-gradient(135deg, $mint, $mint-dark);
  color: #fff;
  border: none;
  border-radius: 6px;
  cursor: pointer;
  font-size: 12px;
  font-weight: 600;
  font-family: $font-sans;
  transition: all 0.15s ease;

  &:disabled {
    background: $slate-300;
    cursor: not-allowed;
  }

  &:not(:disabled):hover {
    background: linear-gradient(135deg, $mint-light, $mint);
  }
}

.panel-footer {
  display: flex;
  align-items: center;
  gap: 12px;
}

/* 确认按钮：mint 渐变主按钮 */
.confirm-btn {
  padding: 8px 20px;
  background: linear-gradient(135deg, $mint, $mint-dark);
  color: #fff;
  border: none;
  border-radius: 8px;
  cursor: pointer;
  font-size: 12px;
  font-weight: 600;
  font-family: $font-sans;
  min-width: 120px;
  box-shadow: 0 2px 8px rgba(16, 185, 129, 0.3);
  transition: all 0.15s ease;

  &:disabled {
    background: $slate-300;
    cursor: not-allowed;
    box-shadow: none;
  }

  &:not(:disabled):hover {
    background: linear-gradient(135deg, $mint-light, $mint);
    box-shadow: 0 4px 12px rgba(16, 185, 129, 0.4);
    transform: translateY(-1px);
  }
}

.hint {
  color: $danger-500;
  font-size: 13px;
}
</style>

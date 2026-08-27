<template>
  <!--
    批次进度条：横向单行布局。
    节点条 + 整体百分比 同行排列，避免占用过多垂直空间。
    节点点击行为：已完成批次弹回退确认，当前批次切换视图，未完成不可点击。
  -->
  <div class="batch-progress-bar">
    <span class="progress-label">批次进度</span>
    <div class="batch-nodes">
      <div
        v-for="(batch, idx) in batches"
        :key="batch.batchId"
        class="batch-node"
        :class="{
          completed: batch.status === 'completed',
          running: batch.status === 'running',
          pending: batch.status === 'pending',
          current: idx === currentBatchIndex,
          clickable: canClickBatch(batch, idx)
        }"
        @click="handleNodeClick(batch, idx)"
      >
        <span class="node-index">{{ batch.batchIndex }}</span>
        <span class="node-range">{{ batch.rangeMin }}~{{ batch.rangeMax }} {{ batch.rangeUnit }}</span>
      </div>
    </div>
    <span class="progress-count">{{ progressPercent }}%</span>
    <span class="progress-count">{{ currentBatchIndex + 1 }}/{{ batches.length }}</span>

    <!-- 回退确认弹窗：改用 element-plus 的 ElDialog 与项目其他弹窗风格统一 -->
    <el-dialog
      v-model="resetDialogVisible"
      title="确认回退重跑？"
      width="400px"
      :close-on-click-modal="false"
      append-to-body
    >
      <p>
        将重置批次 {{ resetTarget?.batchIndex }}（{{ resetTarget?.rangeMin }}~{{ resetTarget?.rangeMax }} {{ resetTarget?.rangeUnit }}），
        已采集的数据将被清空，需要重新通过核对码校验。
      </p>
      <template #footer>
        <el-button @click="resetDialogVisible = false">
          取消
        </el-button>
        <el-button
          type="danger"
          @click="confirmReset"
        >
          确认重跑
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { ElDialog, ElButton } from 'element-plus'
import { type BatchGroup } from '@/types/batch'

const props = defineProps<{
  /** 所有批次列表 */
  batches: BatchGroup[]
  /** 当前执行中的批次索引（0-based，-1 表示未开始） */
  currentBatchIndex: number
}>()

const emit = defineEmits<{
  /** 点击批次节点（用于切换当前批次） */
  select: [batchId: string]
  /** 回退重跑确认 */
  reset: [batchId: string]
}>()

// 回退确认弹窗的目标批次
const resetTarget = ref<BatchGroup | null>(null)

// el-dialog 双向绑定可见状态：
// - get：有目标批次时显示
// - set(false)：用户关闭时清空目标批次
const resetDialogVisible = computed<boolean>({
  get: () => resetTarget.value !== null,
  set: (val: boolean) => {
    if (!val) {
      resetTarget.value = null
    }
  }
})

// 整体进度百分比
const progressPercent = computed<number>(() => {
  if (props.batches.length === 0) return 0
  const completed = props.batches.filter((b) => b.status === 'completed').length
  return Math.round((completed / props.batches.length) * 100)
})

// 状态标签已移除：节点状态用颜色区分（completed=绿、running=深绿高亮、pending=灰），
// 文字标签会让节点变高、占垂直空间，与横向单行布局目标冲突。

// 判断批次是否可点击：
// - 已完成的批次可点击回退重跑
// - 当前批次可点击查看
// - 未完成的批次不可点击（流程必须线性）
const canClickBatch = (batch: BatchGroup, idx: number): boolean => {
  if (batch.status === 'completed') return true
  if (idx === props.currentBatchIndex) return true
  return false
}

// 点击批次节点
const handleNodeClick = (batch: BatchGroup, idx: number): void => {
  if (!canClickBatch(batch, idx)) return

  if (batch.status === 'completed' && idx !== props.currentBatchIndex) {
    // 已完成的批次：弹确认重跑
    resetTarget.value = batch
  } else {
    // 当前批次：仅切换视图
    emit('select', batch.batchId)
  }
}

// 确认回退重跑
const confirmReset = (): void => {
  if (resetTarget.value) {
    emit('reset', resetTarget.value.batchId)
    resetTarget.value = null
  }
}
</script>

<style scoped lang="scss">
/* 横向单行：标签 + 节点条 + 百分比 + 当前序号，全部同行 */
.batch-progress-bar {
  background: #fff;
  border: 1px solid $slate-200;
  border-radius: 8px;
  padding: 6px 12px;
  display: flex;
  align-items: center;
  gap: 12px;
  flex-shrink: 0;
  font-family: $font-sans;
}

.progress-label {
  font-size: 12px;
  font-weight: 600;
  color: $slate-700;
  white-space: nowrap;
  flex-shrink: 0;
}

.batch-nodes {
  display: flex;
  gap: 4px;
  flex: 1;
  min-width: 0;
}

/* 节点：横向紧凑，序号+量程一行内显示 */
.batch-node {
  flex: 1;
  min-width: 0;
  padding: 4px 8px;
  border: 1px solid $slate-200;
  border-radius: 4px;
  background: $slate-50;
  display: flex;
  align-items: center;
  gap: 4px;
  cursor: default;
  transition: all 0.15s ease;

  &.clickable {
    cursor: pointer;

    &:hover {
      border-color: $mint;
      background: rgba(16, 185, 129, 0.08);
    }
  }

  &.completed {
    border-color: $mint;
    background: rgba(16, 185, 129, 0.1);
  }

  &.running,
  &.current {
    border-color: $mint-dark;
    background: rgba(16, 185, 129, 0.18);
    box-shadow: 0 0 0 2px rgba(16, 185, 129, 0.2);
  }
}

.node-index {
  font-size: 11px;
  font-weight: 700;
  color: $slate-600;
  flex-shrink: 0;
}

.node-range {
  font-size: 11px;
  font-weight: 600;
  color: $slate-700;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* 百分比 + 序号：右侧紧凑显示 */
.progress-count {
  font-size: 12px;
  font-weight: 600;
  color: $slate-600;
  font-family: $font-mono;
  white-space: nowrap;
  flex-shrink: 0;
}

/* 回退确认弹窗内容文案 */
:deep(.el-dialog__body p) {
  margin: 0;
  font-size: 14px;
  color: $slate-600;
  line-height: 1.5;
}
</style>

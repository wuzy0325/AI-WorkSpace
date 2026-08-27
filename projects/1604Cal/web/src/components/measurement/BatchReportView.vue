<template>
  <section class="batch-report-view">
    <header class="panel-header">
      <h3>合并报告预览</h3>
      <span class="report-filename">{{ reportData.filename }}</span>
    </header>

    <div class="report-content">
      <div
        v-for="batch in reportData.batches"
        :key="batch.batchId"
        class="batch-section"
      >
        <h4 class="batch-title">
          批次 {{ batch.batchIndex }}：{{ batch.rangeMin }}~{{ batch.rangeMax }} {{ batch.rangeUnit }}
        </h4>

        <div class="batch-info">
          <span class="info-label">通道列表：</span>
          <span class="info-value">
            {{ batch.channels.map((ch) => 'CH' + ch.channelId).join(', ') }}
          </span>
        </div>

        <div class="batch-info">
          <span class="info-label">状态：</span>
          <span class="info-value">{{ statusLabel(batch.status) }}</span>
        </div>

        <!-- 已采集数据（如有） -->
        <div
          v-if="batch.collectedData && Object.keys(batch.collectedData).length > 0"
          class="collected-data"
        >
          <h5>采集数据</h5>
          <table class="data-table">
            <thead>
              <tr>
                <th>通道</th>
                <th>采集值</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="(values, chId) in batch.collectedData"
                :key="chId"
              >
                <td>CH{{ chId }}</td>
                <td>{{ values.join(', ') }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>

    <footer class="panel-footer">
      <button @click="emit('reset')">
        重新开始
      </button>
    </footer>
  </section>
</template>

<script setup lang="ts">
import type { BatchGroup, BatchStatus } from '@/types/batch'

interface ReportData {
  filename: string
  batches: BatchGroup[]
}

defineProps<{
  /** 报告数据 */
  reportData: ReportData
}>()

const emit = defineEmits<{
  /** 重置流程 */
  reset: []
}>()

// 状态标签
const statusLabel = (status: BatchStatus): string => {
  const labels: Record<BatchStatus, string> = {
    pending: '待执行',
    running: '执行中',
    completed: '已完成'
  }
  return labels[status] ?? status
}
</script>

<style scoped lang="scss">
.batch-report-view {
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

.report-filename {
  font-size: 13px;
  color: $slate-500;
  font-family: 'Consolas', 'Monaco', monospace;
}

.report-content {
  display: flex;
  flex-direction: column;
  gap: 12px;
  max-height: 500px;
  overflow-y: auto;
}

.batch-section {
  border: 1px solid $slate-200;
  border-radius: 6px;
  padding: 12px;
  background: $slate-50;
}

.batch-title {
  margin: 0 0 8px;
  font-size: 14px;
  font-weight: 600;
  color: $slate-800;
}

.batch-info {
  display: flex;
  gap: 4px;
  margin-bottom: 4px;
  font-size: 13px;
}

.info-label {
  color: $slate-500;
  min-width: 72px;
}

.info-value {
  color: $slate-800;
}

.collected-data {
  margin-top: 8px;
}

.collected-data h5 {
  margin: 0 0 4px;
  font-size: 13px;
  color: $slate-700;
}

.data-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 12px;
}

.data-table th,
.data-table td {
  border: 1px solid $slate-200;
  padding: 4px 8px;
  text-align: left;
}

.data-table th {
  background: $slate-100;
  font-weight: 600;
  color: $slate-700;
}

.panel-footer {
  display: flex;
  justify-content: flex-end;
}

/* 重新开始按钮：slate 按钮 */
.panel-footer button {
  padding: 8px 16px;
  background: rgba(55, 65, 81, 0.08);
  border: 1px solid $slate-200;
  color: $slate-700;
  border-radius: 8px;
  cursor: pointer;
  font-size: 12px;
  font-weight: 600;
  font-family: $font-sans;
  transition: all 0.15s ease;

  &:hover {
    background: rgba(55, 65, 81, 0.14);
    border-color: $slate-300;
  }

  &:active {
    transform: translateY(1px);
  }
}
</style>

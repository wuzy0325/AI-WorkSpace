<template>
  <div class="channels-section">
    <div class="section-header">
      <div class="section-title">
        <el-icon><Grid /></el-icon>
        <h3>通道读数</h3>
      </div>
      <span class="channel-count">{{ activeChannels }}/{{ totalChannels }} 通道活跃</span>
    </div>
    <div class="channels-grid">
      <div
        v-for="(channel, index) in channelData"
        :key="index"
        class="channel-item"
        :class="{ 'channel-active': channel.isActive }"
      >
        <div class="channel-header">
          <span class="channel-name">CH{{ index + 1 }}</span>
          <span
            class="channel-status"
            :class="channel.status"
          >
            {{ channelStatusText(channel.status) }}
          </span>
        </div>
        <div class="channel-value">
          {{ formatChannelValue(channel.value) }}
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { Grid } from '@element-plus/icons-vue'

interface ChannelInfo {
  value: number | null
  status: 'ok' | 'warning' | 'error' | 'idle'
  isActive: boolean
}

defineProps<{
  channelData: ChannelInfo[]
  activeChannels: number
  totalChannels: number
}>()

// ---- 工具函数 ----

function formatChannelValue(value: number | null): string {
  if (value === null) return '---'
  return value.toFixed(3)
}

function channelStatusText(status: string): string {
  const map: Record<string, string> = {
    ok: '正常',
    warning: '警告',
    error: '异常',
    idle: '空闲'
  }
  return map[status] || status
}
</script>

<style scoped lang="scss">
.channels-section {
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.section-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: var(--spacing-sm);
}

.section-title {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);

  .el-icon {
    font-size: 16px;
    color: var(--accent-primary);
  }
}

.section-header h3 {
  margin: 0;
  font-size: 13px;
  font-weight: 600;
  color: var(--text-primary);
}

.channel-count {
  font-size: 11px;
  color: var(--text-secondary);
  background: var(--bg-tertiary);
  padding: 2px 6px;
  border-radius: 3px;
}

.channels-grid {
  display: grid;
  grid-template-columns: repeat(8, 1fr);
  gap: var(--spacing-xs);
  min-height: 0;
  overflow: auto;
  align-content: start;
  padding-right: 2px;
}

.channel-item {
  background: var(--bg-tertiary);
  border: 1px solid var(--border-color);
  border-radius: 2px;
  padding: 4px;
  text-align: center;
}

.channel-item.channel-active {
  border-color: var(--accent-primary);
  background: rgba(255, 215, 0, 0.06);
}

.channel-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 2px;
}

.channel-name {
  font-size: 10px;
  font-weight: 600;
  color: var(--text-muted);
}

.channel-status {
  font-size: 9px;
  padding: 1px 3px;
  border-radius: 2px;
  font-weight: 500;
}

.channel-status.ok {
  background: var(--status-success-bg);
  color: var(--status-success);
}

.channel-status.warning {
  background: var(--status-warning-bg);
  color: var(--status-warning);
}

.channel-status.error {
  background: var(--status-error-bg);
  color: var(--status-error);
}

.channel-status.idle {
  background: var(--bg-secondary);
  color: var(--text-muted);
}

.channel-value {
  font-family: Consolas, monospace;
  font-size: 13px;
  font-weight: 600;
  color: var(--text-primary);
}

@media (max-width: 1200px) {
  .channels-grid {
    grid-template-columns: repeat(4, 1fr);
  }
}

@media (max-width: 768px) {
  .channels-grid {
    grid-template-columns: repeat(2, 1fr);
  }
}
</style>
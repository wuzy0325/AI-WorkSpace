<template>
  <el-dialog
    v-model="visible"
    title="报警通知"
    width="480px"
    :close-on-click-modal="false"
    :show-close="false"
    draggable
  >
    <div class="alarm-details">
      <el-alert
        type="warning"
        :closable="false"
        title="采集数据超出阈值"
      >
        <template #default>
          <div>压力点 #{{ pointIndex }} | 目标: {{ targetPressure }} | 最大偏差: {{ maxDeviation }}</div>
        </template>
      </el-alert>

      <div
        v-if="overLimitChannels && overLimitChannels.length > 0"
        class="channel-list"
      >
        <p class="channel-label">
          超限通道：
        </p>
        <el-tag
          v-for="ch in overLimitChannels"
          :key="ch"
          type="danger"
          size="small"
          class="channel-tag"
        >
          通道 {{ ch }}
        </el-tag>
      </div>
    </div>

    <template #footer>
      <el-button @click="handleDecision('recollect')">
        重新采集
      </el-button>
      <el-button
        type="primary"
        @click="handleDecision('continue')"
      >
        确认继续
      </el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
const visible = defineModel<boolean>('visible', { default: false })

defineProps<{
  pointIndex?: number
  targetPressure?: number
  maxDeviation?: number
  overLimitChannels?: number[]
  channelDetails?: Record<string, number>
}>()

const emit = defineEmits<{
  'decision': [action: 'continue' | 'recollect']
}>()

function handleDecision(action: 'continue' | 'recollect') {
  emit('decision', action)
  visible.value = false
}
</script>

<style scoped>
.alarm-details { display: flex; flex-direction: column; gap: 12px; }
.channel-list { margin-top: 8px; }
.channel-label { font-size: 13px; color: var(--text-secondary); margin-bottom: 6px; }
.channel-tag { margin-right: 4px; margin-bottom: 4px; }
</style>

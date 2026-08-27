<template>
  <Transition name="slide-fade">
    <div
      v-if="showWarning"
      class="unit-consistency-indicator"
    >
      <div class="indicator-icon">
        <el-icon><Warning /></el-icon>
      </div>
      <div class="indicator-content">
        <div class="indicator-title">
          单位不一致警告
        </div>
        <div class="indicator-message">
          {{ consistency.message || '设备压力单位不一致，请检查并统一单位后再操作' }}
        </div>
        <div
          v-if="detailMessage"
          class="indicator-detail"
        >
          当前各设备单位: {{ detailMessage }}
        </div>
      </div>
    </div>
  </Transition>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Warning } from '@element-plus/icons-vue'

interface UnitItem {
  deviceName: string
  unit: string
}

const props = defineProps<{
  consistency: {
    consistent: boolean
    message?: string
    units?: UnitItem[]
  }
}>()

const showWarning = computed(() => !props.consistency.consistent)

const detailMessage = computed(() => {
  if (!props.consistency.units || props.consistency.units.length === 0) return ''
  return props.consistency.units.map(u => `${u.deviceName}: ${u.unit}`).join(', ')
})
</script>

<style scoped lang="scss">
.unit-consistency-indicator {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  padding: 12px 16px;
  background: rgba(255, 185, 0, 0.1);
  border: 1px solid rgba(255, 185, 0, 0.3);
  border-radius: 8px;
  backdrop-filter: blur(12px);
}
.indicator-icon {
  width: 36px; height: 36px;
  display: flex; align-items: center; justify-content: center;
  background: rgba(255, 185, 0, 0.15);
  border-radius: 6px;
  color: #fbbf24;
  font-size: 20px;
  flex-shrink: 0;
}
.indicator-content {
  display: flex; flex-direction: column; gap: 4px; flex: 1;
}
.indicator-title {
  font-size: 13px; font-weight: 600; color: #fbbf24;
}
.indicator-message {
  font-size: 13px; color: var(--text-secondary); line-height: 1.4;
}
.indicator-detail {
  font-size: 11px; color: var(--text-muted);
  margin-top: 4px; padding-top: 6px;
  border-top: 1px solid rgba(255, 185, 0, 0.2);
}
.slide-fade-enter-active { transition: all 0.3s ease-out; }
.slide-fade-leave-active { transition: all 0.2s ease-in; }
.slide-fade-enter-from, .slide-fade-leave-to { transform: translateY(-10px); opacity: 0; }
</style>

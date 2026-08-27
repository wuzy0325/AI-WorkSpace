<template>
  <span
    class="status-badge"
    :class="status"
    role="status"
  >
    {{ statusText }}
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  status: 'connected' | 'disconnected' | 'connecting' | 'error'
}>()

const statusText = computed(() => {
  const map = {
    connected: '已连接',
    disconnected: '未连接',
    connecting: '连接中',
    error: '错误'
  }
  return map[props.status]
})
</script>

<style scoped lang="scss">
.status-badge {
  display: inline-block;
  padding: 2px 8px;
  border-radius: 3px;
  font-size: 11px;
  font-weight: 600;

  &.connected {
    background: var(--status-success-bg);
    color: var(--status-success);
  }

  &.disconnected {
    background: var(--status-muted-bg);
    color: var(--text-secondary);
  }

  &.connecting {
    background: var(--status-info-bg);
    color: var(--status-info);
  }

  @media (prefers-reduced-motion: no-preference) {
    &.connecting {
      animation: pulse 1.5s infinite;
    }
  }

  @media (prefers-reduced-motion: reduce) {
    &.connecting {
      opacity: 0.7;
    }
  }

  &.error {
    background: var(--status-error-bg);
    color: var(--status-error);
  }
}

@keyframes pulse {
  0%, 100% {
    opacity: 1;
  }
  50% {
    opacity: 0.6;
  }
}
</style>

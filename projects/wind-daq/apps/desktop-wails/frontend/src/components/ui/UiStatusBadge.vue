<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(
  defineProps<{
    status?: 'idle' | 'connected' | 'acquiring' | 'error' | 'warning'
    pulse?: boolean
  }>(),
  { status: 'idle', pulse: false },
)

const dotClass = computed(() => {
  if (props.pulse) return 'ui-status-badge--pulse'
  return `ui-status-badge--${props.status}`
})
</script>

<template>
  <span class="ui-status-badge" :class="dotClass">
    <span class="ui-status-badge__dot" />
    <span class="ui-status-badge__label"><slot /></span>
  </span>
</template>

<style scoped>
.ui-status-badge {
  display: inline-flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.2rem 0.65rem;
  border-radius: 999px;
  font-size: 0.65rem;
  font-weight: 700;
  letter-spacing: 0.05em;
  text-transform: uppercase;
}

.ui-status-badge__dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
}

.ui-status-badge--idle {
  background: rgba(148, 163, 184, 0.1);
  color: #64748b;
}

.ui-status-badge--idle .ui-status-badge__dot {
  background: #64748b;
}

.ui-status-badge--connected {
  background: rgba(16, 185, 129, 0.1);
  color: #10b981;
}

.ui-status-badge--connected .ui-status-badge__dot {
  background: #10b981;
}

.ui-status-badge--acquiring {
  background: rgba(16, 185, 129, 0.12);
  color: #10b981;
}

.ui-status-badge--acquiring .ui-status-badge__dot {
  background: #10b981;
  box-shadow: 0 0 8px #10b981;
}

.ui-status-badge--error {
  background: rgba(244, 63, 94, 0.1);
  color: #f43f5e;
}

.ui-status-badge--error .ui-status-badge__dot {
  background: #f43f5e;
}

.ui-status-badge--warning {
  background: rgba(245, 158, 11, 0.1);
  color: #f59e0b;
}

.ui-status-badge--warning .ui-status-badge__dot {
  background: #f59e0b;
}

.ui-status-badge--pulse .ui-status-badge__dot {
  animation: pulse 1.5s ease-in-out infinite;
}

@keyframes pulse {
  0%, 100% { opacity: 1; transform: scale(1); }
  50% { opacity: 0.4; transform: scale(0.7); }
}
</style>

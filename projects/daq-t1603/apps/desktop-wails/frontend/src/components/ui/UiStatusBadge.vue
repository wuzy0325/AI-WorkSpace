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
  font-weight: 600;
}

.ui-status-badge__dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
}

.ui-status-badge--idle {
  background: var(--card-bg);
  color: var(--text-muted);
}

.ui-status-badge--idle .ui-status-badge__dot {
  background: var(--text-muted);
}

.ui-status-badge--connected {
  background: var(--success-muted);
  color: var(--success);
}

.ui-status-badge--connected .ui-status-badge__dot {
  background: var(--success);
}

.ui-status-badge--acquiring {
  background: var(--success-muted);
  color: var(--success);
}

.ui-status-badge--acquiring .ui-status-badge__dot {
  background: var(--success);
  box-shadow: 0 0 8px var(--success);
}

.ui-status-badge--error {
  background: var(--danger-muted);
  color: var(--danger);
}

.ui-status-badge--error .ui-status-badge__dot {
  background: var(--danger);
}

.ui-status-badge--warning {
  background: var(--warning-muted);
  color: var(--warning);
}

.ui-status-badge--warning .ui-status-badge__dot {
  background: var(--warning);
}

.ui-status-badge--pulse .ui-status-badge__dot {
  animation: pulse 1.5s ease-in-out infinite;
}

@keyframes pulse {
  0%, 100% { opacity: 1; transform: scale(1); }
  50% { opacity: 0.4; transform: scale(0.7); }
}
</style>

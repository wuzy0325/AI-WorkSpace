<script setup lang="ts">
import type { Status } from '../api/wails'

defineProps<{ status: Status }>()
const emit = defineEmits<{ start: []; stop: [] }>()
</script>

<template>
  <div class="control-bar">
    <div class="bar-left">
      <span class="app-title">DAQ MVP</span>
    </div>

    <div class="bar-center">
      <button
        class="btn btn-start"
        :disabled="status.state === 'running'"
        @click="emit('start')"
      >
        &#9654; Start
      </button>
      <button
        class="btn btn-stop"
        :disabled="status.state === 'idle'"
        @click="emit('stop')"
      >
        &#9632; Stop
      </button>
    </div>

    <div class="bar-right">
      <span class="state-badge" :class="'state-' + status.state">
        {{ status.state === 'running' ? '&#9679; Acquiring' : '&#9675; Idle' }}
      </span>
    </div>
  </div>
</template>

<style scoped>
.control-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 48px;
  padding: 0 16px;
  background: rgba(30, 41, 59, 0.75);
  backdrop-filter: blur(12px);
  border-bottom: 1px solid var(--border-default);
  flex-shrink: 0;
}

.app-title {
  font-size: 16px;
  font-weight: 700;
  color: var(--accent-primary);
}

.bar-center {
  display: flex;
  gap: 8px;
}

.btn {
  min-width: 90px;
  height: 32px;
  border-radius: 3px;
  border: 1px solid var(--border-default);
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
  transition: all 120ms ease;
  color: var(--text-primary);
  background: var(--bg-panel);
}

.btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.btn-start:not(:disabled):hover {
  border-color: var(--accent-success);
  color: var(--accent-success);
}

.btn-stop:not(:disabled):hover {
  border-color: var(--accent-danger);
  color: var(--accent-danger);
}

.state-badge {
  font-size: 12px;
  font-weight: 600;
  padding: 4px 12px;
  border-radius: 999px;
  background: var(--bg-panel);
  border: 1px solid var(--border-default);
}

.state-running {
  color: var(--accent-success);
  border-color: rgba(34, 197, 94, 0.3);
}

.state-idle {
  color: var(--text-muted);
}
</style>

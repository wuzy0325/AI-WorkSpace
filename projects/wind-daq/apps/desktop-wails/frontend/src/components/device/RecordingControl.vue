<script setup lang="ts">
import { NButton } from 'naive-ui'

defineProps<{
  isRecording?: boolean
}>()

const emit = defineEmits<{
  (e: 'toggle'): void
}>()
</script>

<template>
  <NButton
    size="tiny"
    class="recording-control"
    :class="{ active: isRecording }"
    @click="emit('toggle')"
    :title="isRecording ? '停止记录' : '开始记录'"
  >
    <span class="recording-control__dot" />
    <span class="recording-control__label">
      {{ isRecording ? 'REC' : 'REC' }}
    </span>
    <small class="recording-control__hint">
      {{ isRecording ? '点击停止' : '录制数据' }}
    </small>
  </NButton>
</template>

<style scoped>
.recording-control {
  display: inline-flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.3rem 0.75rem;
  border-radius: 0.5rem;
  background: rgba(148, 163, 184, 0.1);
  color: var(--text-muted);
  border: 1px solid rgba(148, 163, 184, 0.2);
  font-size: 0.75rem;
  font-weight: 800;
  transition: all 0.2s ease;
}

.recording-control.active {
  color: var(--accent-danger);
  border-color: rgba(239, 68, 68, 0.3);
  background: rgba(239, 68, 68, 0.1);
}

.recording-control__dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: currentColor;
}

.recording-control.active .recording-control__dot {
  animation: pulse-rec 2s infinite;
}

@keyframes pulse-rec {
  0%, 100% { box-shadow: 0 0 0 0 rgba(239, 68, 68, 0.4); }
  50% { box-shadow: 0 0 0 8px rgba(239, 68, 68, 0); }
}

.recording-control__hint {
  display: none;
  font-weight: 500;
  color: var(--text-muted);
}

.recording-control:hover .recording-control__hint {
  display: inline;
}
</style>

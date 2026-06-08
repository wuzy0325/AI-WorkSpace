<script setup lang="ts">
import { onBeforeUnmount, watch } from 'vue'
import { useFeedbackStore } from '@stores/feedbackStore'
import { NButton } from 'naive-ui'

const feedbackStore = useFeedbackStore()
const timerMap = new Map<number, ReturnType<typeof setTimeout>>()

function levelClass(level: 'info' | 'success' | 'warning' | 'error'): string {
  if (level === 'success') return 'toast-success'
  if (level === 'warning') return 'toast-warning'
  if (level === 'error') return 'toast-error'
  return 'toast-info'
}

function scheduleRemove(id: number, durationMs: number): void {
  if (timerMap.has(id)) return
  const timer = setTimeout(() => {
    timerMap.delete(id)
    feedbackStore.removeToast(id)
  }, durationMs)
  timerMap.set(id, timer)
}

function dismiss(id: number): void {
  const timer = timerMap.get(id)
  if (timer) {
    clearTimeout(timer)
    timerMap.delete(id)
  }
  feedbackStore.removeToast(id)
}

onBeforeUnmount(() => {
  timerMap.forEach((timer) => clearTimeout(timer))
  timerMap.clear()
})

watch(
  () => feedbackStore.toasts,
  (toasts) => {
    for (const toast of toasts) {
      scheduleRemove(toast.id, toast.durationMs)
    }
  },
  { deep: true }
)
</script>

<template>
  <div class="toast-host">
    <div
      v-for="toast in feedbackStore.toasts"
      :key="toast.id"
      class="toast-item"
      :class="levelClass(toast.level)"
    >
      <div class="toast-content">
        <span class="toast-message">{{ toast.message }}</span>
        <NButton quaternary size="tiny" @click="dismiss(toast.id)">
          <template #icon>✕</template>
        </NButton>
      </div>
    </div>
  </div>
</template>

<style scoped>
.toast-host {
  position: fixed;
  top: 1rem;
  right: 1rem;
  z-index: 300;
  display: flex;
  width: 380px;
  max-width: calc(100vw - 2rem);
  flex-direction: column;
  gap: 0.75rem;
}

.toast-item {
  border-radius: 0.75rem;
  border: 2px solid;
  padding: 0.875rem 1rem;
  font-size: 0.875rem;
  font-weight: 500;
  box-shadow: 0 10px 40px -10px rgba(0, 0, 0, 0.5);
  backdrop-filter: blur(12px);
  animation: toast-slide-in 0.3s ease;
}

@keyframes toast-slide-in {
  from {
    opacity: 0;
    transform: translateX(100%);
  }
  to {
    opacity: 1;
    transform: translateX(0);
  }
}

.toast-success {
  border-color: rgba(16, 185, 129, 0.8);
  background: rgba(16, 185, 129, 0.25);
  color: #ecfdf5;
  box-shadow: 0 10px 40px -10px rgba(16, 185, 129, 0.4);
}

.toast-warning {
  border-color: rgba(245, 158, 11, 0.8);
  background: rgba(245, 158, 11, 0.25);
  color: #fffbeb;
  box-shadow: 0 10px 40px -10px rgba(245, 158, 11, 0.4);
}

.toast-error {
  border-color: rgba(239, 68, 68, 0.8);
  background: rgba(239, 68, 68, 0.25);
  color: #fef2f2;
  box-shadow: 0 10px 40px -10px rgba(239, 68, 68, 0.4);
}

.toast-info {
  border-color: rgba(59, 130, 246, 0.8);
  background: rgba(59, 130, 246, 0.25);
  color: #eff6ff;
  box-shadow: 0 10px 40px -10px rgba(59, 130, 246, 0.4);
}

.toast-content {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 0.75rem;
}

.toast-message {
  line-height: 1.5;
  flex: 1;
  text-shadow: 0 1px 2px rgba(0, 0, 0, 0.2);
}

.toast-close {
  flex-shrink: 0;
  width: 22px;
  height: 22px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  background: rgba(255, 255, 255, 0.15);
  border: 1px solid rgba(255, 255, 255, 0.2);
  color: inherit;
  cursor: pointer;
  transition: all 0.2s ease;
}

.toast-close:hover {
  background: rgba(255, 255, 255, 0.25);
  transform: scale(1.1);
}

:root[data-theme='light'] .toast-success {
  border-color: #10b981;
  background: #d1fae5;
  color: #065f46;
  box-shadow: 0 10px 40px -10px rgba(16, 185, 129, 0.3);
}

:root[data-theme='light'] .toast-warning {
  border-color: #f59e0b;
  background: #fef3c7;
  color: #92400e;
  box-shadow: 0 10px 40px -10px rgba(245, 158, 11, 0.3);
}

:root[data-theme='light'] .toast-error {
  border-color: #ef4444;
  background: #fee2e2;
  color: #991b1b;
  box-shadow: 0 10px 40px -10px rgba(239, 68, 68, 0.3);
}

:root[data-theme='light'] .toast-info {
  border-color: #3b82f6;
  background: #dbeafe;
  color: #1e40af;
  box-shadow: 0 10px 40px -10px rgba(59, 130, 246, 0.3);
}

:root[data-theme='light'] .toast-message {
  text-shadow: none;
}

:root[data-theme='light'] .toast-close {
  background: rgba(0, 0, 0, 0.1);
  border-color: rgba(0, 0, 0, 0.15);
}

:root[data-theme='light'] .toast-close:hover {
  background: rgba(0, 0, 0, 0.15);
}
</style>

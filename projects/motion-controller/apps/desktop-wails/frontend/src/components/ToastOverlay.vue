<script setup lang="ts">
import { computed } from 'vue'
import { Info, CheckCircle, AlertTriangle, XCircle } from '@lucide/vue'
import { useFeedbackStore, type ToastLevel } from '@stores/feedbackStore'

const feedback = useFeedbackStore()

const levelIcon: Record<ToastLevel, object> = {
  info: Info,
  success: CheckCircle,
  warning: AlertTriangle,
  error: XCircle,
}

const levelClass: Record<ToastLevel, string> = {
  info: 'toast--info',
  success: 'toast--success',
  warning: 'toast--warning',
  error: 'toast--error',
}

function dismiss(id: number): void {
  feedback.removeToast(id)
}
</script>

<template>
  <Teleport to="body">
    <div class="toast-container" v-if="feedback.toasts.length > 0">
      <TransitionGroup
        enter-active-class="transition ease-out duration-200"
        enter-from-class="opacity-0 translate-x-4"
        enter-to-class="opacity-100 translate-x-0"
        leave-active-class="transition ease-in duration-150"
        leave-from-class="opacity-100 translate-x-0"
        leave-to-class="opacity-0 translate-x-4"
      >
        <div
          v-for="toast in feedback.toasts"
          :key="toast.id"
          class="toast-item"
          :class="levelClass[toast.level]"
          @click="dismiss(toast.id)"
        >
          <component :is="levelIcon[toast.level]" class="toast-icon w-3.5 h-3.5" />
          <span class="toast-message">{{ toast.message }}</span>
          <button class="toast-close" @click.stop="dismiss(toast.id)">✕</button>
        </div>
      </TransitionGroup>
    </div>
  </Teleport>
</template>

<style scoped>
.toast-container {
  position: fixed;
  top: 1rem;
  right: 1rem;
  z-index: 9999;
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  max-width: 24rem;
  pointer-events: none;
}

.toast-item {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.625rem 0.875rem;
  border-radius: var(--radius-md, 0.375rem);
  font-size: 0.8125rem;
  font-weight: 500;
  cursor: pointer;
  pointer-events: auto;
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
  border: 1px solid var(--border-default);
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.2);
  transition: opacity 0.15s ease, transform 0.15s ease;
}

.toast-item:hover {
  opacity: 0.9;
}

.toast--info {
  background: color-mix(in srgb, var(--accent-info, #38bdf8) 15%, var(--bg-panel, #172338));
  color: var(--text-primary, #e2e8f0);
  border-color: color-mix(in srgb, var(--accent-info, #38bdf8) 30%, transparent);
}

.toast--success {
  background: color-mix(in srgb, var(--accent-success, #22c55e) 15%, var(--bg-panel, #172338));
  color: var(--text-primary, #e2e8f0);
  border-color: color-mix(in srgb, var(--accent-success, #22c55e) 30%, transparent);
}

.toast--warning {
  background: color-mix(in srgb, var(--accent-warning, #f59e0b) 15%, var(--bg-panel, #172338));
  color: var(--text-primary, #e2e8f0);
  border-color: color-mix(in srgb, var(--accent-warning, #f59e0b) 30%, transparent);
}

.toast--error {
  background: color-mix(in srgb, var(--accent-danger, #ef5b47) 15%, var(--bg-panel, #172338));
  color: var(--text-primary, #e2e8f0);
  border-color: color-mix(in srgb, var(--accent-danger, #ef5b47) 30%, transparent);
}

.toast-icon {
  flex-shrink: 0;
}

.toast--info .toast-icon { color: var(--accent-info, #38bdf8); }
.toast--success .toast-icon { color: var(--accent-success, #22c55e); }
.toast--warning .toast-icon { color: var(--accent-warning, #f59e0b); }
.toast--error .toast-icon { color: var(--accent-danger, #ef5b47); }

.toast-message {
  flex: 1;
  min-width: 0;
  line-height: 1.4;
}

.toast-close {
  flex-shrink: 0;
  width: 1.25rem;
  height: 1.25rem;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 0.25rem;
  font-size: 0.625rem;
  color: var(--text-muted, #94a3b8);
  background: transparent;
  border: none;
  cursor: pointer;
  transition: color 0.15s ease, background-color 0.15s ease;
}

.toast-close:hover {
  color: var(--text-primary, #e2e8f0);
  background: rgba(255, 255, 255, 0.1);
}
</style>

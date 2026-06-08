<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(
  defineProps<{
    variant?: 'primary' | 'secondary' | 'danger' | 'ghost'
    size?: 'sm' | 'md'
    disabled?: boolean
    type?: 'button' | 'submit' | 'reset'
    loading?: boolean
  }>(),
  {
    variant: 'primary',
    size: 'md',
    disabled: false,
    type: 'button',
    loading: false
  }
)

const buttonClasses = computed<string[]>(() => [
  'ui-btn',
  `ui-btn--${props.variant}`,
  `ui-btn--${props.size}`
])
</script>

<template>
  <button
    :type="type"
    :disabled="disabled || loading"
    :class="buttonClasses"
  >
    <span v-if="loading" class="ui-btn__spinner" />
    <slot />
  </button>
</template>

<style scoped>
.ui-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: var(--space-2);
  min-height: 34px;
  padding: 0 var(--space-4);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-sm);
  background: var(--bg-panel);
  color: var(--text-secondary);
  font-size: var(--font-size-sm);
  font-weight: var(--font-weight-semibold);
  letter-spacing: 0.02em;
  transition:
    background-color var(--motion-fast) var(--easing-standard),
    border-color var(--motion-fast) var(--easing-standard),
    color var(--motion-fast) var(--easing-standard),
    transform var(--motion-fast) var(--easing-standard),
    box-shadow var(--motion-fast) var(--easing-standard);
  user-select: none;
  cursor: pointer;
}

.ui-btn:hover:not(:disabled) {
  border-color: var(--border-strong);
  background: var(--bg-panel-strong);
}

.ui-btn:focus-visible {
  outline: none;
  box-shadow: 0 0 0 1px var(--focus-ring), 0 0 0 3px var(--focus-ring-soft);
}

.ui-btn:active:not(:disabled) {
  transform: scale(0.96);
}

.ui-btn:disabled {
  cursor: not-allowed;
  opacity: 0.55;
  transform: none;
}

/* 尺寸变体 */
.ui-btn--sm {
  min-height: 28px;
  padding: 0 var(--space-3);
  font-size: var(--font-size-xs);
}

.ui-btn--md {
  min-height: 34px;
}

/* 主题变体 */
.ui-btn--primary {
  border-color: color-mix(in srgb, var(--accent-success) 50%, var(--border-default));
  background: var(--accent-success);
  color: #ffffff;
  box-shadow: 0 4px 10px color-mix(in srgb, var(--accent-success) 30%, transparent);
}

.ui-btn--primary:hover:not(:disabled) {
  background: color-mix(in srgb, var(--accent-success) 92%, white 6%);
}

.ui-btn--secondary {
  background: var(--bg-panel);
  color: var(--text-secondary);
}

.ui-btn--secondary:hover:not(:disabled) {
  background: var(--bg-panel-strong);
  color: var(--text-primary);
}

.ui-btn--danger {
  border-color: color-mix(in srgb, var(--accent-danger) 55%, var(--border-default));
  background: var(--accent-danger);
  color: #ffffff;
}

.ui-btn--danger:hover:not(:disabled) {
  background: color-mix(in srgb, var(--accent-danger) 90%, white 8%);
}

.ui-btn--ghost {
  background: transparent;
  border-color: transparent;
  color: var(--text-muted);
  box-shadow: none;
}

.ui-btn--ghost:hover:not(:disabled) {
  background: var(--bg-panel-strong);
  color: var(--text-primary);
}

/* 加载旋转动画 */
.ui-btn__spinner {
  width: 14px;
  height: 14px;
  border: 2px solid color-mix(in srgb, currentColor 30%, transparent);
  border-top-color: currentColor;
  border-radius: 50%;
  animation: ui-btn-spin 0.6s linear infinite;
}

@keyframes ui-btn-spin {
  to { transform: rotate(360deg); }
}
</style>

<script setup lang="ts">
withDefaults(
  defineProps<{
    variant?: 'primary' | 'secondary' | 'danger' | 'ghost' | 'warning'
    size?: 'sm' | 'md' | 'lg'
    disabled?: boolean
  }>(),
  { variant: 'primary', size: 'md', disabled: false },
)
</script>

<template>
  <button
    class="ui-btn"
    :class="[`ui-btn--${variant}`, `ui-btn--${size}`]"
    :disabled="disabled"
  >
    <slot />
  </button>
</template>

<style scoped>
.ui-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 0.4rem;
  border-radius: 0.4rem;
  font-weight: 700;
  cursor: pointer;
  transition: all 0.2s ease;
  border: 1px solid transparent;
}

.ui-btn:disabled {
  opacity: 0.55;
  cursor: not-allowed;
}

.ui-btn--sm { min-height: 28px; padding: 0 0.6rem; font-size: 0.72rem; }
.ui-btn--md { min-height: 34px; padding: 0 0.9rem; font-size: 0.8rem; }
.ui-btn--lg { min-height: 40px; padding: 0 1.2rem; font-size: 0.9rem; }

.ui-btn--primary {
  background: var(--accent-success);
  color: #f8fbff;
}

.ui-btn--secondary {
  background: rgba(255, 255, 255, 0.06);
  color: var(--text-secondary);
  border-color: rgba(255, 255, 255, 0.1);
}

.ui-btn--danger {
  background: rgba(244, 63, 94, 0.12);
  color: var(--accent-danger);
  border-color: rgba(244, 63, 94, 0.25);
}

.ui-btn--ghost {
  background: transparent;
  color: var(--text-muted);
}

.ui-btn--ghost:hover {
  color: var(--text-primary);
  background: rgba(255, 255, 255, 0.05);
}

.ui-btn--warning {
  background: rgba(245, 158, 11, 0.12);
  color: #f59e0b;
  border-color: rgba(245, 158, 11, 0.25);
}
</style>

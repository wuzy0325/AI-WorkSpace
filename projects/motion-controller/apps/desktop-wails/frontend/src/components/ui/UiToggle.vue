<script setup lang="ts">
withDefaults(
  defineProps<{
    modelValue: boolean | undefined
    disabled?: boolean
    size?: 'sm' | 'md'
  }>(),
  {
    disabled: false,
    size: 'md',
    modelValue: false,
  },
)

const emit = defineEmits<{
  (e: 'update:modelValue', value: boolean): void
}>()

function onChange(e: Event): void {
  emit('update:modelValue', (e.target as HTMLInputElement).checked)
}
</script>

<template>
  <label
    class="ui-toggle"
    :class="[
      `ui-toggle--${size}`,
      { 'ui-toggle--checked': modelValue, 'ui-toggle--disabled': disabled },
    ]"
  >
    <input
      type="checkbox"
      :checked="modelValue"
      :disabled="disabled"
      @change="onChange"
    />
    <span class="ui-toggle__track">
      <span class="ui-toggle__thumb" />
    </span>
  </label>
</template>

<style scoped>
.ui-toggle {
  display: inline-flex;
  align-items: center;
  cursor: pointer;
}

.ui-toggle input {
  display: none;
}

.ui-toggle__track {
  border-radius: 9999px;
  background: var(--border-strong);
  position: relative;
  transition: background var(--motion-fast) var(--easing-standard);
  flex-shrink: 0;
}

.ui-toggle__thumb {
  position: absolute;
  border-radius: 50%;
  background: white;
  transition: transform var(--motion-fast) var(--easing-standard);
}

.ui-toggle--checked .ui-toggle__track {
  background: var(--toggle-color, var(--accent-success));
}

.ui-toggle--disabled {
  cursor: not-allowed;
}

.ui-toggle--disabled .ui-toggle__track {
  opacity: 0.4;
}

/* md size (default) */
.ui-toggle--md .ui-toggle__track {
  width: 2.25rem;
  height: 1.125rem;
}

.ui-toggle--md .ui-toggle__thumb {
  left: 2px;
  top: 2px;
  width: calc(1.125rem - 4px);
  height: calc(1.125rem - 4px);
}

.ui-toggle--md.ui-toggle--checked .ui-toggle__thumb {
  transform: translateX(1.125rem);
}

/* sm size */
.ui-toggle--sm .ui-toggle__track {
  width: 1.625rem;
  height: 0.875rem;
}

.ui-toggle--sm .ui-toggle__thumb {
  left: 2px;
  top: 2px;
  width: calc(0.875rem - 4px);
  height: calc(0.875rem - 4px);
}

.ui-toggle--sm.ui-toggle--checked .ui-toggle__thumb {
  transform: translateX(0.75rem);
}
</style>

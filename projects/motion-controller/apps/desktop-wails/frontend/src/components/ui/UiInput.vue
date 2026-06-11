<script setup lang="ts">
const props = withDefaults(
  defineProps<{
    modelValue: string | number
    type?: string
    placeholder?: string
    disabled?: boolean
    label?: string
    compact?: boolean
    error?: string
    min?: number
    max?: number
    step?: number | string
  }>(),
  {
    type: 'text',
    placeholder: '',
    disabled: false,
    label: '',
    compact: false,
    error: ''
  }
)

const emit = defineEmits<{
  (e: 'update:modelValue', value: string | number): void
  (e: 'blur'): void
}>()

function onInput(e: Event): void {
  const target = e.target as HTMLInputElement
  if (props.type === 'number') {
    const n = Number(target.value)
    if (!Number.isFinite(n)) return
    emit('update:modelValue', n)
  } else {
    emit('update:modelValue', target.value)
  }
}

function onBlur(): void {
  emit('blur')
}
</script>

<template>
  <div class="ui-field" :class="[props.compact ? 'ui-field--compact' : '', props.error ? 'ui-field--error' : '']">
    <label v-if="label" class="ui-field__label">
      {{ label }}
    </label>
    <input
      :type="type"
      :value="modelValue"
      :placeholder="placeholder"
      :disabled="disabled"
      :min="min"
      :max="max"
      :step="step"
      @input="onInput"
      @blur="onBlur"
      class="ui-input"
      :class="[props.compact ? 'ui-input--compact' : '', props.error ? 'ui-input--error' : '']"
    />
    <!-- 字段级错误提示 -->
    <div v-if="error" class="ui-field__error">
      <span>●</span> {{ error }}
    </div>
  </div>
</template>

<style scoped>
.ui-field {
  display: grid;
  gap: var(--space-1);
}

.ui-field__label {
  display: block;
  color: var(--text-muted);
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.ui-input {
  width: 100%;
  min-height: 34px;
  padding: 0 var(--space-3);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  background: var(--bg-panel-strong);
  color: var(--text-primary);
  font-size: var(--font-size-xs);
  font-weight: 600;
  transition:
    border-color var(--motion-fast) var(--easing-standard),
    background-color var(--motion-fast) var(--easing-standard),
    box-shadow var(--motion-fast) var(--easing-standard);
}

.ui-input::placeholder {
  color: var(--text-muted);
  font-weight: 400;
}

.ui-input:hover:not(:disabled) {
  border-color: var(--border-strong);
  background: var(--bg-panel);
}

.ui-input:focus {
  outline: none;
  border-color: var(--accent-success);
  box-shadow: 0 0 0 1px var(--focus-ring), 0 0 0 3px var(--focus-ring-soft);
}

.ui-input:disabled {
  cursor: not-allowed;
  opacity: 0.6;
}

/* 错误状态 */
.ui-input--error {
  border-color: var(--accent-danger);
  background: color-mix(in srgb, var(--accent-danger) 5%, var(--bg-panel-strong));
}

.ui-input--error:focus {
  border-color: var(--accent-danger);
  box-shadow: 0 0 0 1px var(--accent-danger), 0 0 0 3px color-mix(in srgb, var(--accent-danger) 20%, transparent);
}

.ui-field__error {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 10px;
  font-weight: 700;
  color: var(--accent-danger);
}

/* 紧凑模式 */
.ui-field--compact {
  gap: 2px;
}

.ui-field--compact .ui-field__label {
  font-size: 11px;
  letter-spacing: 0.04em;
}

.ui-input--compact {
  min-height: 28px;
  padding: 0 var(--space-2);
  font-size: 11px;
  border-radius: var(--radius-sm);
}
</style>

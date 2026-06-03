<script setup lang="ts">
withDefaults(
  defineProps<{
    modelValue?: string | number
    type?: 'text' | 'number'
    placeholder?: string
  }>(),
  { modelValue: '', type: 'text', placeholder: '' },
)

const emit = defineEmits<{ (e: 'update:modelValue', v: string | number): void }>()

function onInput(e: Event) {
  emit('update:modelValue', (e.target as HTMLInputElement).value)
}
</script>

<template>
  <input
    class="ui-input"
    :type="type"
    :value="modelValue"
    :placeholder="placeholder"
    @input="onInput"
  />
</template>

<style scoped>
.ui-input {
  width: 100%;
  padding: 0.5rem 0.75rem;
  border-radius: 0.4rem;
  border: 1px solid var(--border-default);
  background: var(--card-bg);
  color: var(--text-primary);
  font: inherit;
  font-size: 0.85rem;
}

.ui-input::placeholder {
  color: var(--text-muted);
}

.ui-input:focus {
  outline: none;
  border-color: var(--accent);
  box-shadow: 0 0 0 2px var(--accent-muted);
}
</style>

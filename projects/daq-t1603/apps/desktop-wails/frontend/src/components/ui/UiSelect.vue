<script setup lang="ts">
withDefaults(
  defineProps<{
    modelValue?: string
    options?: Array<{ value: string; label: string }>
    placeholder?: string
  }>(),
  { modelValue: '', options: () => [], placeholder: '' },
)

const emit = defineEmits<{ (e: 'update:modelValue', v: string): void }>()
</script>

<template>
  <select
    class="ui-select"
    :value="modelValue"
    @change="emit('update:modelValue', ($event.target as HTMLSelectElement).value)"
  >
    <option v-if="placeholder" value="" disabled>{{ placeholder }}</option>
    <option v-for="opt in options" :key="opt.value" :value="opt.value">
      {{ opt.label }}
    </option>
  </select>
</template>

<style scoped>
.ui-select {
  width: 100%;
  padding: 0.5rem 0.75rem;
  border-radius: 0.4rem;
  border: 1px solid var(--border-default);
  background: var(--card-bg);
  color: var(--text-primary);
  font: inherit;
  font-size: 0.85rem;
  cursor: pointer;
  appearance: auto;
}
</style>

<script setup lang="ts">
withDefaults(
  defineProps<{
    modelValue?: boolean
    disabled?: boolean
  }>(),
  { modelValue: false, disabled: false },
)

const emit = defineEmits<{ (e: 'update:modelValue', v: boolean): void }>()
</script>

<template>
  <button
    type="button"
    class="ui-toggle"
    :class="{ 'ui-toggle--on': modelValue }"
    :disabled="disabled"
    @click="emit('update:modelValue', !modelValue)"
  >
    <span class="ui-toggle__thumb" />
  </button>
</template>

<style scoped>
.ui-toggle {
  width: 36px;
  height: 20px;
  border-radius: 999px;
  background: var(--btn-bg);
  border: none;
  cursor: pointer;
  position: relative;
  transition: background 0.2s ease;
}

.ui-toggle--on {
  background: var(--accent);
}

.ui-toggle__thumb {
  position: absolute;
  top: 2px;
  left: 2px;
  width: 16px;
  height: 16px;
  border-radius: 50%;
  background: #f8fbff;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.2);
  transition: transform 0.2s ease;
}

.ui-toggle--on .ui-toggle__thumb {
  transform: translateX(16px);
}

.ui-toggle:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}
</style>

<script setup lang="ts">
defineProps<{
  label?: string
  error?: string
  hint?: string
  required?: boolean
}>()
</script>

<template>
  <div class="ui-form-field">
    <label v-if="label" class="ui-form-field__label">
      {{ label }}<span v-if="required" class="ui-form-field__required">*</span>
    </label>
    <div class="ui-form-field__control">
      <slot />
    </div>
    <p v-if="error" class="ui-form-field__error">{{ error }}</p>
    <p v-else-if="hint" class="ui-form-field__hint">{{ hint }}</p>
  </div>
</template>

<style scoped>
.ui-form-field {
  margin-bottom: var(--space-3);
}
.ui-form-field__label {
  display: block;
  margin-bottom: var(--space-1);
  font-size: var(--font-size-2xs);
  font-weight: var(--font-weight-bold);
  color: var(--text-muted);
  text-transform: uppercase;
  letter-spacing: 0.06em;
}
.ui-form-field__required {
  color: var(--accent-danger);
  margin-left: var(--space-1);
}
.ui-form-field__control {
  width: 100%;
}
.ui-form-field__error {
  margin: var(--space-1) 0 0;
  font-size: var(--font-size-xs);
  color: var(--accent-danger);
  line-height: var(--line-height-base);
}
.ui-form-field__hint {
  margin: var(--space-1) 0 0;
  font-size: var(--font-size-xs);
  color: var(--text-tertiary);
  line-height: var(--line-height-base);
}
</style>

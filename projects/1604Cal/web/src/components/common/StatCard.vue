<template>
  <div
    class="stat-chip"
    :class="{ highlighted: !!color }"
  >
    <span
      v-if="color"
      class="chip-dot"
      :style="{ background: color }"
    />
    <span class="chip-label">{{ label }}</span>
    <strong
      class="chip-value"
      :style="color ? { color } : {}"
    >{{ value }}</strong>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  label: string
  value: string | number
  color?: string
}>()
</script>

<style scoped lang="scss">
.stat-chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 14px;
  background: #ffffff;
  border: 1px solid $slate-200;
  border-radius: 8px;
  font-family: $font-sans;
  transition: border-color 0.2s ease;

  &.highlighted {
    background: $slate-50;
  }
}

.chip-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  flex-shrink: 0;
}

.chip-label {
  font-size: 12px;
  color: $slate-500;
  font-weight: 500;
}

.chip-value {
  font-size: 15px;
  font-weight: 600;
  color: $slate-800;
  font-family: $font-mono;
  line-height: 1;
}
</style>

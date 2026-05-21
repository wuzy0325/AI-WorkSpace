<script setup lang="ts">
withDefaults(
  defineProps<{ size?: number }>(),
  { size: 20 },
)
</script>

<template>
  <svg :width="size" :height="size" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
    <path d="M5 5v14h14" />
    <path d="M11 11l7-7" />
    <path d="M14 4h4v4" />
    <circle cx="11" cy="11" r="1.5" fill="currentColor" stroke="none" />
  </svg>
</template>

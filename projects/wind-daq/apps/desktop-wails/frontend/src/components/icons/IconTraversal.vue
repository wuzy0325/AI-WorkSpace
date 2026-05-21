<script setup lang="ts">
withDefaults(
  defineProps<{ size?: number }>(),
  { size: 20 },
)
</script>

<template>
  <svg :width="size" :height="size" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
    <path d="M3 21h18" />
    <path d="M5 14l4-3 5 4 5-9" />
    <path d="M19 6V2" />
    <circle cx="5" cy="14" r="1.5" fill="currentColor" stroke="none" />
    <circle cx="9" cy="11" r="1.5" fill="currentColor" stroke="none" />
    <circle cx="14" cy="15" r="1.5" fill="currentColor" stroke="none" />
    <circle cx="19" cy="6" r="1.5" fill="currentColor" stroke="none" />
  </svg>
</template>

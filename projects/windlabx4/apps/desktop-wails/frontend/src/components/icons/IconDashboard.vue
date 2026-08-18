<script setup lang="ts">
withDefaults(
  defineProps<{ size?: number }>(),
  { size: 20 },
)
</script>

<template>
  <svg :width="size" :height="size" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
    <rect x="3" y="3" width="18" height="18" rx="2" />
    <path d="M11 3v18" />
    <path d="M7 7h1" />
    <path d="M7 11h1" />
    <path d="M15 7h2" />
    <path d="M15 11h2" />
    <path d="M15 15h2" />
  </svg>
</template>

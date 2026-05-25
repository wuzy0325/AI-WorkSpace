<script setup lang="ts">
withDefaults(defineProps<{ size?: number }>(), { size: 24 })
</script>

<template>
  <svg :width="size" :height="size" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
    <circle cx="12" cy="12" r="3" />
    <circle cx="12" cy="5" r="2" />
    <circle cx="5" cy="12" r="2" />
    <circle cx="19" cy="12" r="2" />
    <circle cx="12" cy="19" r="2" />
    <line x1="12" y1="7" x2="12" y2="9" />
    <line x1="7" y1="12" x2="9" y2="12" />
    <line x1="15" y1="12" x2="17" y2="12" />
    <line x1="12" y1="15" x2="12" y2="17" />
  </svg>
</template>

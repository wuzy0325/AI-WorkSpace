<script setup lang="ts">
withDefaults(defineProps<{ size?: number }>(), { size: 24 })
</script>

<template>
  <svg :width="size" :height="size" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
    <circle cx="12" cy="5" r="2" />
    <circle cx="5" cy="19" :r="2" />
    <circle cx="19" cy="19" :r="2" />
    <line x1="12" y1="7" x2="6.5" y2="17" />
    <line x1="12" y1="7" x2="17.5" y2="17" />
  </svg>
</template>

<script setup lang="ts">
withDefaults(defineProps<{ size?: number }>(), { size: 24 })
</script>

<template>
  <svg :width="size" :height="size" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
    <path d="M12 2v6" />
    <path d="M8 8h8" />
    <path d="M10 8v10a2 2 0 0 0 4 0V8" />
    <circle cx="12" cy="20" r="2" />
  </svg>
</template>

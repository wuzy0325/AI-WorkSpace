<script setup lang="ts">
import { NText } from 'naive-ui'

defineProps<{
  title: string
  subtitle?: string
}>()
</script>

<template>
  <div>
    <NText v-if="subtitle" depth="3" style="display:block;font-size:0.65rem;font-weight:700;text-transform:uppercase;letter-spacing:0.08em;margin-bottom:0.25rem">
      {{ subtitle }}
    </NText>
    <NText depth="1" tag="h3" style="margin:0;font-size:0.95rem;font-weight:800;letter-spacing:-0.02em">
      {{ title }}
    </NText>
  </div>
</template>

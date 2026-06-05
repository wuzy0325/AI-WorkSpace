<script setup lang="ts">
import AppShell from '@components/layout/AppShell.vue'
</script>

<template>
  <AppShell canvas-class="dashboard-ceramic-shell">
    <template v-if="$slots.header" #header>
      <slot name="header" />
    </template>
    <template v-if="$slots.rail" #rail>
      <slot name="rail" />
    </template>
    <template v-if="$slots.sidebar" #sidebar>
      <slot name="sidebar" />
    </template>
    <template v-if="$slots.toolbar" #toolbar>
      <slot name="toolbar" />
    </template>
    <slot />
    <template v-if="$slots.statusbar" #statusbar>
      <slot name="statusbar" />
    </template>
  </AppShell>
</template>

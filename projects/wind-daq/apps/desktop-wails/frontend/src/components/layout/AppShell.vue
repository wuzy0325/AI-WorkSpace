<script setup lang="ts">
withDefaults(
  defineProps<{
    contentClass?: string
    canvasClass?: string
  }>(),
  {
    contentClass: '',
    canvasClass: '',
  },
)
</script>

<template>
  <div class="app-shell">
    <slot name="header" />

    <div class="app-shell__workspace">
      <slot name="rail" />
      <slot name="sidebar" />

      <div :class="['app-canvas', canvasClass]">
        <slot name="toolbar" />
        <main :class="['app-shell__main', contentClass]">
          <slot />
        </main>
      </div>
    </div>

    <slot name="statusbar" />
  </div>
</template>

<style scoped>
.app-shell {
  display: flex;
  flex-direction: column;
  height: 100vh;
  overflow: hidden;
  color: var(--text-primary);
}

.app-shell__workspace {
  display: flex;
  flex: 1;
  min-height: 0;
  flex-wrap: nowrap;
  overflow: hidden;
  background: color-mix(in srgb, var(--bg-app) 92%, var(--bg-panel));
}

.app-canvas {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-width: 320px;
  min-height: 0;
  overflow: hidden;
  border-left: 1px solid color-mix(in srgb, var(--border-default) 80%, var(--border-strong));
}

.app-shell__main {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
  overflow: hidden;
}
</style>

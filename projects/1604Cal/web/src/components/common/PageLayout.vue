<template>
  <div
    class="page-layout"
    :class="{ 'sidebar-collapsed': sidebarCollapsed }"
  >
    <Sidebar @collapse-change="sidebarCollapsed = $event" />
    <main class="main-content">
      <slot />
    </main>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import Sidebar from './Sidebar.vue'

const sidebarCollapsed = ref(false)
</script>

<style scoped lang="scss">
.page-layout {
  display: grid;
  grid-template-columns: 180px 1fr;
  width: 100%;
  height: 100%;
  background: #f6f7f6;
  overflow: hidden;
  font-family: 'DM Sans', 'SF Pro Display', -apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Microsoft YaHei', sans-serif;
  transition: grid-template-columns 250ms cubic-bezier(0.4, 0, 0.2, 1);
}

.page-layout.sidebar-collapsed {
  grid-template-columns: 52px 1fr;
}

.main-content {
  min-width: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

@media (max-width: 768px) {
  .page-layout {
    grid-template-columns: 1fr;
    grid-template-rows: auto 1fr;
  }
}
</style>

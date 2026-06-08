<script setup lang="ts">
import MotionControlPanel from '@components/motion/MotionControlPanel.vue'
import { NButton } from 'naive-ui'

withDefaults(
  defineProps<{
    embedded?: boolean
  }>(),
  { embedded: false },
)

function closeCurrentWindow(): void {
  window.close()
}
</script>

<template>
  <div data-test="motion-shell" class="bg-[color:var(--bg-canvas)] text-[color:var(--text-primary)] font-sans flex flex-col overflow-hidden" :class="embedded ? 'h-full min-h-0' : 'h-screen'">
    <header class="glass-header flex items-center justify-between px-5 py-3 shrink-0">
      <div>
        <h1 class="text-sm font-bold tracking-tight text-[color:var(--text-primary)]">运动控制器</h1>
        <p class="text-[10px] font-semibold uppercase tracking-wider text-[color:var(--text-muted)]">{{ embedded ? '轴控制与监视' : '独立窗口 · 用于轴控制与监视' }}</p>
      </div>
      <NButton
        v-if="!embedded"
        quaternary
        size="small"
        @click="closeCurrentWindow"
      >
        关闭
      </NButton>
    </header>
    <main class="flex-1 min-h-0 p-4 bg-[color:var(--bg-canvas)]">
      <MotionControlPanel />
    </main>
  </div>
</template>
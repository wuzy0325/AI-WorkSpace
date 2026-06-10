<script setup lang="ts">
import MotionControlPanel from '@components/motion/MotionControlPanel.vue'
import UiButton from '@components/ui/UiButton.vue'

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
  <div data-test="motion-shell" class="flex h-full min-h-0 flex-col overflow-hidden font-sans" style="background:var(--color-bg-canvas);color:var(--color-text-primary)">
    <header class="glass-header flex shrink-0 items-center justify-between px-5 py-3">
      <div>
        <h1 class="text-sm font-bold tracking-tight" style="color:var(--color-text-primary)">运动控制器</h1>
        <p class="text-[10px] font-semibold uppercase tracking-wider" style="color:var(--color-text-muted)">{{ embedded ? '轴控制与监视' : '独立窗口 · 用于轴控制与监视' }}</p>
      </div>
      <UiButton v-if="!embedded" variant="ghost" size="sm" @click="closeCurrentWindow">关闭</UiButton>
    </header>
    <main class="flex-1 min-h-0 p-4" style="background:var(--color-bg-canvas)">
      <MotionControlPanel />
    </main>
  </div>
</template>

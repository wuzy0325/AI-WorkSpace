<script setup lang="ts">
import MotionControlPanel from '@components/motion/MotionControlPanel.vue'
import UiButton from '@components/ui/UiButton.vue'
import UiTooltip from '@components/ui/UiTooltip.vue'

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
  <div data-test="motion-shell" class="flex h-full min-h-0 flex-col overflow-hidden font-sans" style="background:var(--bg-canvas);color:var(--text-primary)">
    <header class="glass-header flex shrink-0 items-center justify-between px-5 py-3">
      <div class="flex items-center gap-2">
        <div>
          <h1 class="text-sm font-bold tracking-tight" style="color:var(--text-primary)">运动控制器</h1>
          <p class="text-[10px] font-semibold uppercase tracking-wider" style="color:var(--text-muted)">{{ embedded ? '轴控制与监视' : '独立窗口 · 用于轴控制与监视' }}</p>
        </div>
        <UiTooltip content="控制风洞测试中的运动轴（如迎角、侧滑角等）的位置和速度" position="right">
          <span class="help-icon">?</span>
        </UiTooltip>
      </div>
      <UiButton v-if="!embedded" variant="ghost" size="sm" @click="closeCurrentWindow">关闭</UiButton>
    </header>
    <main class="flex-1 min-h-0 p-4" style="background:var(--bg-canvas)">
      <MotionControlPanel />
    </main>
  </div>
</template>

<style scoped>
.help-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;
  border-radius: 50%;
  background: color-mix(in srgb, var(--accent-primary) 15%, transparent);
  color: var(--accent-primary);
  font-size: 10px;
  font-weight: 800;
  cursor: help;
  transition: background 0.2s;
}
.help-icon:hover {
  background: color-mix(in srgb, var(--accent-primary) 25%, transparent);
}
</style>

<script setup lang="ts">
import { Settings } from '@lucide/vue'
import IconDashboard from '@components/icons/IconDashboard.vue'
import IconMotion from '@components/icons/IconMotion.vue'
import IconCalibrationFiveHole from '@components/icons/IconCalibrationFiveHole.vue'
import IconTraversal from '@components/icons/IconTraversal.vue'
import IconLog from '@components/icons/IconLog.vue'
import IconStorage from '@components/icons/IconStorage.vue'

function iconComponent(type?: string) {
  if (type === 'IO') return IconDashboard
  if (type === 'AX') return IconMotion
  if (type === 'CP') return IconCalibrationFiveHole
  if (type === 'TR') return IconTraversal
  if (type === 'LG') return IconLog
  if (type === 'ST') return IconStorage
  return IconDashboard
}

export interface RailItem {
  id: string
  label: string
  icon?: string
  active?: boolean
  disabled?: boolean
}

withDefaults(
  defineProps<{
    items?: RailItem[]
  }>(),
  { items: () => [] },
)

const emit = defineEmits<{
  (e: 'select', id: string): void
  (e: 'openSettings'): void
}>()
</script>

<template>
  <aside class="app-rail-nav">
    <nav class="app-rail-nav__menu">
      <button
        v-for="item in items"
        :key="item.id"
        type="button"
        :aria-label="item.label"
        :class="[
          'app-rail-nav__btn',
          { 'active': item.active, 'disabled': item.disabled },
        ]"
        :title="item.label"
        :disabled="item.disabled"
        @click="emit('select', item.id)"
      >
        <component :is="iconComponent(item.icon)" class="w-5 h-5" />
      </button>
    </nav>

    <div class="app-rail-nav__footer">
      <button
        type="button"
        class="app-rail-nav__btn"
        title="Settings"
        @click="emit('openSettings')"
      >
        <Settings class="w-5 h-5" />
      </button>
      <slot />
    </div>
  </aside>
</template>

<style scoped>
.app-rail-nav {
  width: clamp(56px, 6vw, var(--layout-rail-width, 72px));
  height: 100%;
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  background: color-mix(in srgb, var(--bg-panel) 94%, transparent);
  backdrop-filter: blur(10px);
  -webkit-backdrop-filter: blur(10px);
  border-right: 1px solid rgba(255, 255, 255, 0.05);
}

:root[data-theme='light'] .app-rail-nav {
  border-right: 1px solid rgba(0, 0, 0, 0.05);
}

.app-rail-nav__menu {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.75rem;
  padding: 1.5rem 0;
  flex: 1;
}

.app-rail-nav__btn {
  width: 40px;
  height: 40px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 0.75rem;
  background: transparent;
  color: #64748b;
  font: 900 0.7rem/1 var(--font-family-mono, monospace);
  transition: all 0.2s ease;
}

.app-rail-nav__btn:hover {
  color: #10b981;
}

.app-rail-nav__btn.active {
  background: rgba(16, 185, 129, 0.15);
  color: #10b981;
  border: 1px solid rgba(16, 185, 129, 0.25);
}

.app-rail-nav__btn.disabled {
  opacity: 0.35;
  cursor: not-allowed;
  pointer-events: none;
}

.app-rail-nav__footer {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 1rem;
  padding: 1rem 0;
  margin-top: auto;
}
</style>

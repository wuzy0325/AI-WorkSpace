<script setup lang="ts">
import { Settings } from '@lucide/vue'
import IconDashboard from '@components/icons/IconDashboard.vue'
import IconMotion from '@components/icons/IconMotion.vue'
import IconCalibrationFiveHole from '@components/icons/IconCalibrationFiveHole.vue'
import IconTraversal from '@components/icons/IconTraversal.vue'
import IconLog from '@components/icons/IconLog.vue'

export interface AppRailNavItem {
  id: string
  label: string
  icon?: string
  active?: boolean
  disabled?: boolean
}

withDefaults(
  defineProps<{
    items?: AppRailNavItem[]
  }>(),
  { items: () => [] },
)

const emit = defineEmits<{
  (e: 'select', id: string): void
  (e: 'open-settings'): void
}>()

function getIconComponent(iconType: string | undefined) {
  if (iconType === 'IO') return IconDashboard
  if (iconType === 'AX') return IconMotion
  if (iconType === 'CP') return IconCalibrationFiveHole
  if (iconType === 'TR') return IconTraversal
  if (iconType === 'LG') return IconLog
  return IconDashboard
}
</script>

<template>
  <aside class="app-rail-nav">
    <nav class="app-rail-nav__menu">
      <button
        v-for="item in items"
        :key="item.id"
        type="button"
        :aria-label="item.label"
        class="app-rail-nav__button"
        :class="{
          'app-rail-nav__button--active': item.active,
          'app-rail-nav__button--disabled': item.disabled
        }"
        :title="item.label"
        :disabled="item.disabled"
        @click="emit('select', item.id)"
      >
        <component :is="getIconComponent(item.icon)" class="w-5 h-5" />
      </button>
    </nav>

    <div class="app-rail-nav__footer">
      <button
        type="button"
        class="app-rail-nav__button app-rail-nav__button--settings"
        aria-label="设置"
        title="设置"
        @click="emit('open-settings')"
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

.app-rail-nav__button {
  width: 40px;
  height: 40px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 0.75rem;
  color: #64748b;
  transition: all 0.2s ease;
}

.app-rail-nav__button:hover {
  color: #10b981;
}

.app-rail-nav__button--active {
  background: rgba(16, 185, 129, 0.15);
  color: #10b981;
  border: 1px solid rgba(16, 185, 129, 0.25);
}

.app-rail-nav__button--disabled {
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

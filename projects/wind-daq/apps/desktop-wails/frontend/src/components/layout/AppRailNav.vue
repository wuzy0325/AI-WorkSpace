<script setup lang="ts">
import { ref } from 'vue'
import { Settings } from '@lucide/vue'
import UiButton from '@components/ui/UiButton.vue'
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

/* 导航栏展开状态：使用点击切换代替 hover，避免误触和布局跳动 */
const isExpanded = ref(false)

function toggleExpand(): void {
  isExpanded.value = !isExpanded.value
}

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
  <aside
    class="app-rail-nav"
    :class="{ 'app-rail-nav--expanded': isExpanded }"
  >
    <nav class="app-rail-nav__menu">
      <UiButton
        v-for="item in items"
        :key="item.id"
        quaternary
        size="sm"
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
        <template #icon>
          <component :is="getIconComponent(item.icon)" class="w-5 h-5" />
        </template>
        <span v-if="isExpanded" class="app-rail-nav__label">{{ item.label }}</span>
      </UiButton>
    </nav>

    <div class="app-rail-nav__footer">
      <!-- 展开/收起切换按钮 -->
      <UiButton
        quaternary
        size="sm"
        class="app-rail-nav__button app-rail-nav__button--toggle"
        :aria-label="isExpanded ? '收起导航' : '展开导航'"
        :title="isExpanded ? '收起导航' : '展开导航'"
        @click="toggleExpand"
      >
        <template #icon>
          <svg v-if="isExpanded" class="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M11 17l-5-5 5-5M18 17l-5-5 5-5"/></svg>
          <svg v-else class="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M13 17l5-5-5-5M6 17l5-5-5-5"/></svg>
        </template>
        <span v-if="isExpanded" class="app-rail-nav__label">收起</span>
      </UiButton>
      <UiButton
        quaternary
        size="sm"
        class="app-rail-nav__button app-rail-nav__button--settings"
        aria-label="设置"
        title="设置"
        @click="emit('open-settings')"
      >
        <template #icon>
          <Settings class="w-5 h-5" />
        </template>
        <span v-if="isExpanded" class="app-rail-nav__label">设置</span>
      </UiButton>
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
  background: var(--bg-panel);
  border-right: 1px solid var(--border-default);
  transition: width 0.2s ease;
  overflow: hidden;
}

.app-rail-nav--expanded {
  width: 160px;
}

.app-rail-nav__menu {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 0.5rem;
  padding: 1.5rem 0.75rem;
  flex: 1;
}

.app-rail-nav__button {
  width: 100%;
  height: 40px;
  display: flex;
  align-items: center;
  justify-content: flex-start;
  gap: 0.75rem;
  padding: 0 0.5rem;
}

.app-rail-nav__label {
  font-size: var(--font-size-xs);
  font-weight: 600;
  color: var(--text-secondary);
  white-space: nowrap;
  opacity: 0;
  transition: opacity 0.15s ease;
}

.app-rail-nav--expanded .app-rail-nav__label {
  opacity: 1;
}

:deep(.app-rail-nav__button--active) {
  color: var(--accent-primary);
}

:deep(.app-rail-nav__button--active) .app-rail-nav__label {
  color: var(--accent-primary);
}

:deep(.app-rail-nav__button--active:hover) {
  color: var(--accent-primary);
}

.app-rail-nav__button--disabled {
  opacity: 0.35;
  cursor: not-allowed;
  pointer-events: none;
}

/* 展开/收起按钮使用更明显的视觉区分 */
.app-rail-nav__button--toggle {
  color: var(--text-muted);
}

.app-rail-nav__button--toggle:hover {
  color: var(--text-primary);
}

.app-rail-nav__footer {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 0.5rem;
  padding: 1rem 0.75rem;
  margin-top: auto;
}
</style>

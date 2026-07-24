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
  // external 标记：该项不切换页面，而是触发外部动作（如弹出独立窗口）
  external?: boolean
  // locked 标记：该项为付费模块且当前未解锁，在图标右下角显示小锁角标。
  // 解锁后由父组件置为 false，角标消失。仅影响视觉提示，不影响点击行为。
  locked?: boolean
}

withDefaults(
  defineProps<{
    items?: AppRailNavItem[]
    // footerItems 渲染在底部区（设置按钮上方），用于独立窗口等特殊入口
    footerItems?: AppRailNavItem[]
    // 国际化翻译表
    t?: Record<string, string>
  }>(),
  { items: () => [], footerItems: () => [], t: () => ({}) },
)

const emit = defineEmits<{
  (e: 'select', id: string): void
  (e: 'open-external', id: string): void
  (e: 'open-settings'): void
}>()

/* 导航栏展开状态：使用点击切换代替 hover，避免误触和布局跳动。
   默认展开——启动时露出导航项文字标签，便于新用户识别各入口用途；
   用户点击底部切换按钮收起后仅保留图标列，节省横向空间。 */
const isExpanded = ref(true)

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

// 点击导航项：external 项触发 open-external 事件，普通项触发 select 事件
function handleClick(item: AppRailNavItem): void {
  if (item.disabled) return
  if (item.external) {
    emit('open-external', item.id)
  } else {
    emit('select', item.id)
  }
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
          'app-rail-nav__button--disabled': item.disabled,
          'app-rail-nav__button--external': item.external
        }"
        :title="item.label"
        :disabled="item.disabled"
        @click="handleClick(item)"
      >
        <template #icon>
          <span class="app-rail-nav__icon-wrap">
            <component :is="getIconComponent(item.icon)" class="w-5 h-5" />
            <!-- 付费模块未解锁角标：在图标右下角叠加小锁，提示需解锁 -->
            <span
              v-if="item.locked"
              class="app-rail-nav__lock-badge"
              :aria-label="t.locked || '付费模块，点击解锁'"
              :title="t.locked || '付费模块，点击解锁'"
            >
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round">
                <rect x="5" y="11" width="14" height="10" rx="2" />
                <path d="M8 11V7a4 4 0 0 1 8 0v4" />
              </svg>
            </span>
          </span>
        </template>
        <span v-if="isExpanded" class="app-rail-nav__label">{{ item.label }}</span>
        <!-- external 项显示弹出小图标，提示会打开独立窗口 -->
        <svg v-if="item.external && isExpanded" class="w-3 h-3 ml-auto opacity-60" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/>
          <polyline points="15 3 21 3 21 9"/>
          <line x1="10" y1="14" x2="21" y2="3"/>
        </svg>
      </UiButton>
    </nav>

    <div class="app-rail-nav__footer">
      <!-- 底部 external 项（如运动控制器独立窗口入口） -->
      <UiButton
        v-for="item in footerItems"
        :key="item.id"
        quaternary
        size="sm"
        :aria-label="item.label"
        class="app-rail-nav__button app-rail-nav__button--external"
        :title="item.label"
        :disabled="item.disabled"
        @click="handleClick(item)"
      >
        <template #icon>
          <component :is="getIconComponent(item.icon)" class="w-5 h-5" />
        </template>
        <span v-if="isExpanded" class="app-rail-nav__label">{{ item.label }}</span>
        <!-- external 项显示弹出小图标，提示会打开独立窗口 -->
        <svg v-if="isExpanded" class="w-3 h-3 ml-auto opacity-60" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/>
          <polyline points="15 3 21 3 21 9"/>
          <line x1="10" y1="14" x2="21" y2="3"/>
        </svg>
      </UiButton>
      <!-- 展开/收起切换按钮 -->
      <UiButton
        quaternary
        size="sm"
        class="app-rail-nav__button app-rail-nav__button--toggle"
        :aria-label="isExpanded ? (t.collapseNav || '收起导航') : (t.expandNav || '展开导航')"
        :title="isExpanded ? (t.collapseNav || '收起导航') : (t.expandNav || '展开导航')"
        @click="toggleExpand"
      >
        <template #icon>
          <svg v-if="isExpanded" class="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M11 17l-5-5 5-5M18 17l-5-5 5-5"/></svg>
          <svg v-else class="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M13 17l5-5-5-5M6 17l5-5-5-5"/></svg>
        </template>
        <span v-if="isExpanded" class="app-rail-nav__label">{{ t.collapse || '收起' }}</span>
      </UiButton>
      <UiButton
        quaternary
        size="sm"
        class="app-rail-nav__button app-rail-nav__button--settings"
        :aria-label="t.settings || '设置'"
        :title="t.settings || '设置'"
        @click="emit('open-settings')"
      >
        <template #icon>
          <Settings class="w-5 h-5" />
        </template>
        <span v-if="isExpanded" class="app-rail-nav__label">{{ t.settings || '设置' }}</span>
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

/* 图标容器：相对定位，作为锁角标的定位锚点 */
.app-rail-nav__icon-wrap {
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}

/* 付费模块未解锁锁角标：绝对定位到图标右下角，尺寸约为图标的 60%。
   使用 accent-warning 色族传达「受限」语义，避免与 active 态的 accent-primary 混淆。 */
.app-rail-nav__lock-badge {
  position: absolute;
  right: -3px;
  bottom: -3px;
  width: 12px;
  height: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  background: var(--accent-warning);
  color: var(--color-brand-foreground);
  /* 描边色与导航栏背景同色，形成「挖空」效果，让角标在任意图标上都清晰可见 */
  box-shadow: 0 0 0 1.5px var(--bg-panel);
}

.app-rail-nav__lock-badge svg {
  width: 8px;
  height: 8px;
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

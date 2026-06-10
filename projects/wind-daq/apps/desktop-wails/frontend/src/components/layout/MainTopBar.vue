<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { useThemeStore } from '@stores/themeStore'
import { Sun, Moon, Activity } from '@lucide/vue'
import UiButton from '@components/ui/UiButton.vue'

type MainShellPage = 'dashboard' | 'motion' | 'calibration' | 'traversal' | 'log'
type MainViewMode = 'chart' | 'table' | 'both' | 'overview'

const props = withDefaults(
  defineProps<{
    locale: 'zh' | 'en'
    version: string
    isAcquiring: boolean
    activePage: MainShellPage
    viewMode: MainViewMode
    t: Record<string, string>
  }>(),
  {}
)

const emit = defineEmits<{
  (e: 'set-locale', locale: 'zh' | 'en'): void
  (e: 'set-view-mode', mode: MainViewMode): void
}>()

const themeStore = useThemeStore()
const { theme } = storeToRefs(themeStore)

const dashboardModes: MainViewMode[] = ['chart', 'table', 'both', 'overview']

function themeToggleLabel(): string {
  return theme.value === 'dark'
    ? (props.t.toggleToLightTheme || '切换为浅色模式')
    : (props.t.toggleToDarkTheme || '切换为深色模式')
}

function modeLabel(mode: MainViewMode): string {
  if (mode === 'chart') return props.t.chartMode || '图表'
  if (mode === 'table') return props.t.tableMode || '表格'
  if (mode === 'both') return props.t.bothMode || '混合'
  return props.t.overviewMode || '总览'
}

function activePageLabel(): string {
  if (props.activePage === 'motion') return props.t.motionControl || '运动控制'
  if (props.activePage === 'calibration') return props.t.probeCalibration || '探针校准'
  if (props.activePage === 'traversal') return props.t.traversalTest || '遍历测试'
  return ''
}
</script>

<template>
  <header class="main-topbar">
    <div data-test="topbar-main-row" class="main-topbar__content">
      <!-- Logo -->
      <div class="main-topbar__brand">
        <div class="main-topbar__logo">
          <Activity class="w-5 h-5 text-white" />
        </div>
        <div>
          <h1 data-test="topbar-brand-title" class="main-topbar__title">
            Wind<span class="main-topbar__title-accent">DAQ</span>
          </h1>
          <p data-test="topbar-brand-subtitle" class="main-topbar__subtitle">DATA ACQUISITION</p>
        </div>
      </div>

      <!-- Navigation -->
      <nav v-if="activePage === 'dashboard'" class="main-topbar__nav">
        <UiButton
          v-for="mode in dashboardModes"
          :key="mode"
          size="sm"
          class="main-topbar__nav-btn"
          :class="{ 'main-topbar__nav-btn--active': viewMode === mode }"
          @click="emit('set-view-mode', mode)"
        >
          {{ modeLabel(mode) }}
        </UiButton>
      </nav>
      <div v-else class="main-topbar__nav">
        <div class="main-topbar__nav-btn main-topbar__nav-btn--active">
          {{ activePageLabel() }}
        </div>
      </div>

      <!-- Right Section -->
      <div data-test="topbar-utility-group" class="main-topbar__actions">
        <!-- Status Pill -->
        <div
          data-test="system-status-pill"
          class="main-topbar__status"
          :class="isAcquiring ? 'main-topbar__status--active' : 'main-topbar__status--idle'"
        >
          <span class="main-topbar__status-dot" :class="{ 'status-pulse': isAcquiring }"></span>
          <span data-test="system-status-label" class="main-topbar__status-text">{{ isAcquiring ? (t.acquiring || '采集开启') : (t.idle || '就绪') }}</span>
        </div>

        <!-- Theme Toggle -->
        <UiButton
          quaternary
          size="md"
          class="main-topbar__icon-btn"
          @click="themeStore.toggleTheme()"
          :aria-label="themeToggleLabel()"
          :title="themeToggleLabel()"
        >
          <template #icon>
            <Sun v-if="theme === 'dark'" class="w-4 h-4" />
            <Moon v-else class="w-4 h-4" />
          </template>
        </UiButton>

        <!-- Locale Switch -->
        <div class="main-topbar__locale">
          <UiButton
            size="sm"
            class="main-topbar__locale-btn"
            :class="{ 'main-topbar__locale-btn--active': locale === 'zh' }"
            @click="emit('set-locale', 'zh')"
          >
            中文
          </UiButton>
          <UiButton
            size="sm"
            class="main-topbar__locale-btn"
            :class="{ 'main-topbar__locale-btn--active': locale === 'en' }"
            @click="emit('set-locale', 'en')"
          >
            EN
          </UiButton>
        </div>

        <!-- Version -->
        <span data-test="topbar-version-text" class="main-topbar__version">v{{ version }}</span>
      </div>
    </div>
  </header>
</template>

<style scoped>
.main-topbar {
  height: 56px;
  flex-shrink: 0;
  background: rgba(30, 41, 59, 0.75);
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
  position: relative;
  z-index: 50;
}

:root[data-theme='light'] .main-topbar {
  background: rgba(255, 255, 255, 0.75);
  border-bottom: 1px solid rgba(0, 0, 0, 0.05);
}

.main-topbar__content {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 100%;
  padding: 0 1.5rem;
  gap: 1rem;
}

.main-topbar__brand {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  flex-shrink: 0;
}

.main-topbar__logo {
  width: 32px;
  height: 32px;
  background: var(--accent-primary);
  border-radius: 0.5rem;
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: 0 4px 12px color-mix(in srgb, var(--accent-primary) 30%, transparent);
}

.main-topbar__title {
  font-size: 1.125rem;
  font-weight: 800;
  letter-spacing: -0.025em;
  color: var(--text-primary);
  line-height: 1;
}

:root[data-theme='light'] .main-topbar__title {
  color: var(--bg-app);
}

.main-topbar__title-accent {
  color: var(--accent-primary);
}

.main-topbar__subtitle {
  font-size: 0.5rem;
  font-weight: 600;
  color: #64748b;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  margin-top: 0.125rem;
}

@media (max-width: 1280px) {
  .main-topbar__subtitle {
    display: none;
  }
}

@media (min-width: 1280px) {
  .main-topbar__subtitle {
    display: block;
  }
}

.main-topbar__nav {
  display: flex;
  align-items: center;
  gap: 0.25rem;
  padding: 0.25rem;
  background: rgba(0, 0, 0, 0.2);
  border-radius: 0.5rem;
  flex-shrink: 0;
}

:root[data-theme='light'] .main-topbar__nav {
  background: rgba(0, 0, 0, 0.05);
}

.main-topbar__nav-btn {
  font-size: 0.75rem;
  font-weight: 600;
  white-space: nowrap;
}

:deep(.main-topbar__nav-btn--active) {
  background: color-mix(in srgb, var(--accent-primary) 10%, transparent) !important;
  color: var(--accent-primary) !important;
}

:deep(.main-topbar__nav-btn--active:hover) {
  color: var(--accent-primary) !important;
}

.main-topbar__actions {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  flex-shrink: 0;
}

.main-topbar__status {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.25rem 0.75rem;
  border-radius: 9999px;
  font-size: 0.625rem;
  font-weight: 700;
  letter-spacing: 0.05em;
  text-transform: uppercase;
}

.main-topbar__status--active {
  background: color-mix(in srgb, var(--accent-primary) 10%, transparent);
  border: 1px solid color-mix(in srgb, var(--accent-primary) 20%, transparent);
  color: var(--accent-primary);
}

.main-topbar__status--idle {
  background: rgba(148, 163, 184, 0.1);
  border: 1px solid rgba(148, 163, 184, 0.2);
  color: #64748b;
}

.main-topbar__status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: currentColor;
}

.status-pulse {
  animation: status-pulse 2s ease-in-out infinite;
}

@keyframes status-pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.4; }
}

.main-topbar__icon-btn {
  width: 32px;
  height: 32px;
}

.main-topbar__locale {
  display: flex;
  align-items: center;
  padding: 0.125rem;
  background: rgba(0, 0, 0, 0.2);
  border-radius: 0.5rem;
  border: 1px solid rgba(255, 255, 255, 0.08);
}

:root[data-theme='light'] .main-topbar__locale {
  background: rgba(0, 0, 0, 0.05);
  border: 1px solid rgba(0, 0, 0, 0.08);
}

.main-topbar__locale-btn {
  padding: 0.25rem 0.625rem;
  font-size: 0.625rem;
  font-weight: 700;
  color: #64748b;
  border-radius: 0.375rem;
  transition: all 0.2s ease;
}

:deep(.main-topbar__locale-btn--active) {
  background: #10b981 !important;
  color: white !important;
}

.main-topbar__version {
  font-size: 0.625rem;
  font-weight: 500;
  color: #64748b;
  font-family: ui-monospace, monospace;
}

@media (max-width: 1536px) {
  .main-topbar__version {
    display: none;
  }
}

@media (min-width: 1536px) {
  .main-topbar__version {
    display: block;
  }
}

@media (max-width: 1024px) {
  .main-topbar__nav {
    display: none;
  }
}
</style>

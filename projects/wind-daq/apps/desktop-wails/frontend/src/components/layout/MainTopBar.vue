<script setup lang="ts">
import { Activity, Sun, Moon } from '@lucide/vue'

type MainShellPage = 'dashboard' | 'motion' | 'calibration' | 'traversal' | 'log' | 'storage'
type DashboardMode = 'overview' | 'chart' | 'table' | 'both'

defineProps<{
  version?: string
  isAcquiring?: boolean
  activePage?: MainShellPage
  viewMode?: DashboardMode
  locale?: 'zh' | 'en'
  theme?: 'dark' | 'light'
  labels?: Record<string, string>
}>()

const emit = defineEmits<{
  (e: 'toggleTheme'): void
  (e: 'setLocale', locale: 'zh' | 'en'): void
  (e: 'setViewMode', mode: DashboardMode): void
}>()

const dashboardModes: DashboardMode[] = ['chart', 'table', 'both', 'overview']

function modeLabel(mode: DashboardMode, labels?: Record<string, string>) {
  if (mode === 'chart') return labels?.chartMode ?? '图表'
  if (mode === 'table') return labels?.tableMode ?? '表格'
  if (mode === 'both') return labels?.bothMode ?? '混合'
  return labels?.overviewMode ?? '总览'
}

function pageLabel(page: MainShellPage | undefined, labels?: Record<string, string>) {
  if (page === 'motion') return labels?.motionControl ?? '运动控制'
  if (page === 'calibration') return labels?.probeCalibration ?? '探针校准'
  if (page === 'traversal') return labels?.traversalTest ?? '遍历测试'
  if (page === 'log') return labels?.logViewer ?? '运行日志'
  if (page === 'storage') return labels?.storage ?? '存储'
  return ''
}
</script>

<template>
  <header class="main-topbar">
    <div class="main-topbar__content">
      <div class="main-topbar__brand">
        <div class="main-topbar__logo">
          <Activity class="w-5 h-5 text-white" />
        </div>
        <div>
          <h1 class="main-topbar__title">Wind<span class="main-topbar__title-accent">DAQ</span></h1>
          <p class="main-topbar__subtitle">DATA ACQUISITION</p>
        </div>
      </div>

      <div class="main-topbar__right">
        <nav v-if="activePage === 'dashboard'" class="main-topbar__nav">
          <button
            v-for="mode in dashboardModes"
            :key="mode"
            class="main-topbar__nav-btn"
            :class="{ 'main-topbar__nav-btn--active': viewMode === mode }"
            @click="emit('setViewMode', mode)"
          >
            {{ modeLabel(mode, labels) }}
          </button>
        </nav>
        <div v-else-if="pageLabel(activePage, labels)" class="main-topbar__nav">
          <span class="main-topbar__nav-btn main-topbar__nav-btn--active">
            {{ pageLabel(activePage, labels) }}
          </span>
        </div>

        <div class="main-topbar__status" :class="isAcquiring ? 'main-topbar__status--live' : ''">
          <span class="main-topbar__status-dot" :class="{ 'status-pulse': isAcquiring }" />
          <span>{{ isAcquiring ? (labels?.acquiring ?? '采集中') : (labels?.idle ?? '就绪') }}</span>
        </div>

        <div class="main-topbar__actions">
          <div class="main-topbar__locale">
            <button class="main-topbar__locale-btn" :class="{ active: locale === 'zh' }" @click="emit('setLocale', 'zh')">中文</button>
            <button class="main-topbar__locale-btn" :class="{ active: locale === 'en' }" @click="emit('setLocale', 'en')">EN</button>
          </div>
          <button class="main-topbar__theme-btn" @click="emit('toggleTheme')" title="Toggle theme">
            <Sun v-if="theme === 'dark'" class="w-4 h-4" />
            <Moon v-else class="w-4 h-4" />
          </button>
          <span v-if="version" class="main-topbar__version">v{{ version }}</span>
        </div>
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
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 0.5rem;
  background: #10b981;
  color: #f8fbff;
  box-shadow: 0 4px 12px rgba(16, 185, 129, 0.3);
}

.main-topbar__title {
  margin: 0;
  font-size: 1.125rem;
  font-weight: 800;
  letter-spacing: -0.025em;
  color: #e2e8f0;
  line-height: 1;
}

:root[data-theme='light'] .main-topbar__title {
  color: #0f172a;
}

.main-topbar__title-accent {
  color: #10b981;
}

.main-topbar__subtitle {
  margin: 0.125rem 0 0;
  font-size: 0.5rem;
  font-weight: 600;
  color: #64748b;
  letter-spacing: 0.1em;
  text-transform: uppercase;
}

.main-topbar__right {
  display: flex;
  align-items: center;
  gap: 1rem;
}

.main-topbar__actions {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.main-topbar__nav {
  display: flex;
  align-items: center;
  gap: 0.25rem;
  padding: 0.25rem;
  border-radius: 0.5rem;
  background: rgba(0, 0, 0, 0.2);
  flex-shrink: 0;
}

:root[data-theme='light'] .main-topbar__nav {
  background: rgba(0, 0, 0, 0.05);
}

.main-topbar__nav-btn {
  padding: 0.25rem 1rem;
  border-radius: 0.375rem;
  color: var(--text-muted);
  font-size: 0.75rem;
  font-weight: 600;
  white-space: nowrap;
  transition: all 0.2s ease;
}

.main-topbar__nav-btn:hover,
.main-topbar__nav-btn--active {
  color: #10b981;
}

.main-topbar__nav-btn--active {
  background: rgba(16, 185, 129, 0.1);
  box-shadow: inset 0 -2px 0 #10b981;
}

.main-topbar__locale {
  display: flex;
  padding: 0.125rem;
  border-radius: 0.5rem;
  background: rgba(0, 0, 0, 0.2);
  border: 1px solid rgba(255, 255, 255, 0.08);
}

.main-topbar__locale-btn {
  padding: 0.25rem 0.625rem;
  font-size: 0.625rem;
  font-weight: 700;
  color: var(--text-muted);
  border-radius: 0.375rem;
  transition: all 0.2s ease;
}

.main-topbar__locale-btn.active {
  background: #10b981;
  color: white;
  box-shadow: 0 2px 8px rgba(16, 185, 129, 0.3);
}

.main-topbar__theme-btn {
  width: 32px;
  height: 32px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 0.5rem;
  color: var(--text-muted);
  background: rgba(255, 255, 255, 0.05);
  border: 1px solid rgba(255, 255, 255, 0.08);
  transition: all 0.2s ease;
}

.main-topbar__theme-btn:hover {
  color: #10b981;
  background: rgba(16, 185, 129, 0.1);
  border-color: rgba(16, 185, 129, 0.2);
}

.main-topbar__status {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.25rem 0.75rem;
  border-radius: 999px;
  font-size: 0.625rem;
  font-weight: 800;
  letter-spacing: 0.05em;
  background: rgba(148, 163, 184, 0.1);
  border: 1px solid rgba(148, 163, 184, 0.2);
  color: #64748b;
}

.main-topbar__status--live {
  background: rgba(16, 185, 129, 0.1);
  border-color: rgba(16, 185, 129, 0.3);
  color: #10b981;
}

.main-topbar__status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: currentColor;
}

.main-topbar__version {
  color: var(--text-muted);
  font-family: var(--font-family-mono, monospace);
  font-size: 0.625rem;
  font-weight: 600;
}
</style>

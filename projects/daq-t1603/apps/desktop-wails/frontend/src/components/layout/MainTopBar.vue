<script setup lang="ts">
import { computed } from 'vue'
import { Sun, Moon, Activity, Settings2, Plus, CircleDot } from '@lucide/vue'
import { useDeviceStore } from '@stores/deviceStore'
import { useTheme } from '@composables/useTheme'

defineProps<{
  version: string
}>()

const emit = defineEmits<{
  (e: 'add-device'): void
  (e: 'open-config'): void
}>()

const deviceStore = useDeviceStore()
const { theme, toggle: toggleTheme } = useTheme()

const totalDevices = computed(() => deviceStore.profiles.length)
const connectedDevices = computed(
  () => deviceStore.profiles.filter((p) => deviceStore.statusFor(p.id) === 'Connected' || deviceStore.statusFor(p.id) === 'Acquiring').length
)
const acquiringDevices = computed(
  () => deviceStore.profiles.filter((p) => deviceStore.acquiringFor(p.id)).length
)

const isAcquiring = computed(() => acquiringDevices.value > 0)

function themeToggleLabel(): string {
  return theme.value === 'dark' ? '切换为浅色模式' : '切换为深色模式'
}
</script>

<template>
  <header class="topbar">
    <div class="topbar__content">
      <div class="topbar__brand">
        <div class="topbar__logo">
          <Activity class="topbar__logo-icon" />
        </div>
        <div class="topbar__title-wrap">
          <h1 class="topbar__title">
            DAQ-T<span class="topbar__title-accent">1603</span>
          </h1>
          <p class="topbar__subtitle">Temperature Acquisition</p>
        </div>
      </div>

      <div class="topbar__nav">
        <div class="topbar__nav-btn topbar__nav-btn--active">
          <CircleDot class="topbar__nav-icon" />
          实时监控
        </div>
      </div>

      <div class="topbar__actions">
        <div class="topbar__stat">
          <span class="topbar__stat-label">设备</span>
          <span class="topbar__stat-value mono">{{ totalDevices }}</span>
        </div>
        <div class="topbar__divider"></div>
        <div class="topbar__stat">
          <span class="topbar__stat-label">在线</span>
          <span class="topbar__stat-value mono topbar__stat-value--online">{{ connectedDevices }}</span>
        </div>

        <div
          class="topbar__status"
          :class="isAcquiring ? 'topbar__status--active' : 'topbar__status--idle'"
        >
          <span class="topbar__status-dot" :class="{ 'pulse-dot': isAcquiring }"></span>
          <span class="topbar__status-text">{{ isAcquiring ? '采集中' : '就绪' }}</span>
        </div>

        <button
          class="topbar__icon-btn"
          :title="'添加设备'"
          @click="emit('add-device')"
        >
          <Plus class="topbar__icon" />
        </button>

        <button
          class="topbar__icon-btn"
          :title="'打开配置'"
          @click="emit('open-config')"
        >
          <Settings2 class="topbar__icon" />
        </button>

        <button
          class="topbar__icon-btn"
          @click="toggleTheme"
          :aria-label="themeToggleLabel()"
          :title="themeToggleLabel()"
        >
          <Sun v-if="theme === 'dark'" class="topbar__icon" />
          <Moon v-else class="topbar__icon" />
        </button>

        <span class="topbar__version">v{{ version }}</span>
      </div>
    </div>
  </header>
</template>

<style scoped>
.topbar {
  height: var(--layout-header-height);
  flex-shrink: 0;
  background: var(--topbar-bg);
  backdrop-filter: blur(14px) saturate(140%);
  -webkit-backdrop-filter: blur(14px) saturate(140%);
  border-bottom: 1px solid var(--border-default);
  position: relative;
  z-index: 50;
}

.topbar__content {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 100%;
  padding: 0 1.5rem;
  gap: 1.5rem;
}

.topbar__brand {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  flex-shrink: 0;
}

.topbar__logo {
  width: 36px;
  height: 36px;
  background: linear-gradient(135deg, var(--accent) 0%, var(--accent-active) 100%);
  border-radius: var(--radius-md);
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: 0 6px 16px var(--accent-glow);
}

.topbar__logo-icon {
  width: 20px;
  height: 20px;
  color: #ffffff;
}

.topbar__title-wrap {
  display: flex;
  flex-direction: column;
  line-height: 1;
}

.topbar__title {
  font-size: var(--font-size-lg);
  font-weight: 800;
  letter-spacing: -0.02em;
  color: var(--text-primary);
}

.topbar__title-accent {
  color: var(--accent);
}

.topbar__subtitle {
  font-size: 0.5rem;
  font-weight: 600;
  color: var(--text-muted);
  letter-spacing: 0.18em;
  text-transform: uppercase;
  margin-top: 0.25rem;
}

@media (max-width: 1280px) {
  .topbar__subtitle {
    display: none;
  }
}

.topbar__nav {
  display: flex;
  align-items: center;
  gap: 0.25rem;
  padding: 0.25rem;
  background: var(--btn-bg);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  flex-shrink: 0;
}

.topbar__nav-btn {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.4rem 0.875rem;
  font-size: var(--font-size-sm);
  font-weight: 600;
  color: var(--text-secondary);
  border-radius: var(--radius-sm);
  transition: all 0.2s ease;
  white-space: nowrap;
}

.topbar__nav-btn:hover {
  color: var(--text-primary);
}

.topbar__nav-btn--active {
  background: var(--accent-muted);
  color: var(--accent);
  box-shadow: inset 0 -2px 0 var(--accent);
}

.topbar__nav-icon {
  width: 14px;
  height: 14px;
}

.topbar__actions {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  flex-shrink: 0;
}

.topbar__stat {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  line-height: 1.1;
}

.topbar__stat-label {
  font-size: 0.55rem;
  font-weight: 700;
  color: var(--text-muted);
  letter-spacing: 0.12em;
  text-transform: uppercase;
}

.topbar__stat-value {
  font-size: var(--font-size-base);
  font-weight: 800;
  color: var(--text-primary);
  margin-top: 0.15rem;
}

.topbar__stat-value--online {
  color: var(--accent);
}

.topbar__divider {
  width: 1px;
  height: 24px;
  background: var(--border-default);
}

.topbar__status {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.35rem 0.85rem;
  border-radius: var(--radius-pill);
  font-size: var(--font-size-xs);
  font-weight: 700;
  letter-spacing: 0.05em;
  text-transform: uppercase;
}

.topbar__status--active {
  background: var(--success-muted);
  border: 1px solid var(--accent-border);
  color: var(--success);
}

.topbar__status--idle {
  background: var(--btn-bg);
  border: 1px solid var(--border-default);
  color: var(--text-muted);
}

.topbar__status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: currentColor;
}

.topbar__icon-btn {
  width: 36px;
  height: 36px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: var(--radius-md);
  color: var(--text-secondary);
  background: var(--btn-bg);
  border: 1px solid var(--border-default);
  transition: all 0.2s ease;
}

.topbar__icon-btn:hover {
  color: var(--accent);
  background: var(--accent-soft);
  border-color: var(--accent-border);
}

.topbar__icon {
  width: 16px;
  height: 16px;
}

.topbar__version {
  font-size: 0.6rem;
  font-weight: 600;
  color: var(--text-muted);
  font-family: var(--font-family-mono);
  padding-left: 0.25rem;
}

@media (max-width: 1536px) {
  .topbar__version {
    display: none;
  }
}

@media (max-width: 1024px) {
  .topbar__nav {
    display: none;
  }
}
</style>

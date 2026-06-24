<script setup lang="ts">
import { Sun, Moon } from '@lucide/vue'
import MotionControlPanel from '@components/motion/MotionControlPanel.vue'
import { useI18nStore } from '@stores/i18nStore'
import { useThemeStore } from '@stores/themeStore'

withDefaults(
  defineProps<{
    embedded?: boolean
  }>(),
  { embedded: false },
)

const i18n = useI18nStore()
const theme = useThemeStore()

function toggleLanguage(): void {
  i18n.setLocale(i18n.locale === 'zh' ? 'en' : 'zh')
}
</script>

<template>
  <div data-test="motion-shell" class="motion-view" :class="embedded ? 'h-full min-h-0' : 'h-screen'">
    <!-- 顶部标题栏 -->
    <header class="motion-view-header">
      <div class="motion-view-header-left">
        <h1 class="motion-view-title">运动控制器</h1>
        <p class="motion-view-subtitle">{{ embedded ? '轴控制与监控' : '独立窗口 · 轴控制与监控' }}</p>
      </div>
      <div class="motion-view-header-actions">
        <button
          class="header-action-btn"
          :title="theme.mode === 'dark' ? i18n.t.switchToLightTheme : i18n.t.switchToDarkTheme"
          @click="theme.toggleTheme"
        >
          <Sun v-if="theme.mode === 'dark'" class="w-4 h-4" />
          <Moon v-else class="w-4 h-4" />
        </button>
        <button
          class="header-action-btn header-action-btn--text"
          :title="i18n.locale === 'zh' ? i18n.t.switchToEnglish : i18n.t.switchToChinese"
          @click="toggleLanguage"
        >
          {{ i18n.locale === 'zh' ? 'EN' : '中文' }}
        </button>
      </div>
    </header>

    <!-- 主内容区 -->
    <main class="motion-view-main">
      <MotionControlPanel />
    </main>
  </div>
</template>

<style scoped>
/* ============================================================
   主视图容器
   ============================================================ */
.motion-view {
  display: flex;
  flex-direction: column;
  overflow: hidden;
  background: var(--bg-canvas);
  color: var(--text-primary);
  font-family: var(--font-family-sans, 'Microsoft YaHei UI', sans-serif);
}

/* ============================================================
   顶部标题栏
   ============================================================ */
.motion-view-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-shrink: 0;
  padding: var(--space-3) var(--space-5);
  background: var(--bg-panel);
  border-bottom: 1px solid var(--border-default);
}

.motion-view-header-left {
  display: flex;
  flex-direction: column;
  gap: var(--space-0-5);
}

.motion-view-title {
  font-size: 0.875rem;
  font-weight: 700;
  letter-spacing: -0.01em;
  color: var(--text-primary);
  line-height: 1.3;
}

.motion-view-subtitle {
  font-size: 0.625rem;
  font-weight: 600;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--text-muted);
}

.motion-view-header-actions {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

/* 头部操作按钮 */
.header-action-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  height: 2rem;
  min-width: 2rem;
  padding: 0 var(--space-2);
  border-radius: var(--radius-md);
  font-size: 0.75rem;
  font-weight: 600;
  border: 1px solid var(--border-default);
  color: var(--text-muted);
  background: transparent;
  cursor: pointer;
  transition: all var(--motion-fast) var(--easing-standard);
}

.header-action-btn:hover {
  color: var(--text-primary);
  background: var(--bg-panel-strong);
  border-color: var(--border-strong);
}

.header-action-btn:active {
  transform: scale(0.95);
}

.header-action-btn--text {
  padding: 0 var(--space-3);
}

/* ============================================================
   主内容区
   ============================================================ */
.motion-view-main {
  flex: 1;
  min-height: 0;
  background: var(--bg-canvas);
}
</style>

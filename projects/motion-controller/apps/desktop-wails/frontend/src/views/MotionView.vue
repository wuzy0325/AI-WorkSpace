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
  <div data-test="motion-shell" class="motion-win" :class="embedded ? 'h-full min-h-0' : 'h-screen'">
    <!-- 顶部标题栏（仪器窗口式 titlebar） -->
    <header class="motion-titlebar">
      <div class="motion-lights" aria-hidden="true">
        <i class="r"></i><i class="y"></i><i class="g"></i>
      </div>
      <div class="motion-name">{{ i18n.t.motionController }}</div>
      <div class="motion-spacer"></div>
      <button
        class="motion-tb-btn"
        :title="theme.mode === 'dark' ? i18n.t.switchToLightTheme : i18n.t.switchToDarkTheme"
        @click="theme.toggleTheme"
      >
        <Sun v-if="theme.mode === 'dark'" class="w-3.5 h-3.5" />
        <Moon v-else class="w-3.5 h-3.5" />
        <span>{{ i18n.t.theme }}</span>
      </button>
      <button
        class="motion-tb-btn"
        :title="i18n.locale === 'zh' ? i18n.t.switchToEnglish : i18n.t.switchToChinese"
        @click="toggleLanguage"
      >
        {{ i18n.locale === 'zh' ? '中 / EN' : 'EN / 中' }}
      </button>
    </header>

    <!-- 主内容区 -->
    <main class="motion-body">
      <MotionControlPanel />
    </main>
  </div>
</template>

<style scoped>
/* ============================================================
   仪器窗口容器
   ============================================================ */
.motion-win {
  display: flex;
  flex-direction: column;
  overflow: hidden;
  background: var(--bg-canvas);
  color: var(--text-primary);
  font-family: var(--font-family-sans, 'Microsoft YaHei UI', sans-serif);
  border: 1px solid var(--border-default);
  /* 仪器质感：紧凑圆角 */
  border-radius: 8px;
}

/* ============================================================
   顶部标题栏（macOS 风格 titlebar）
   ============================================================ */
.motion-titlebar {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 9px 14px;
  background: var(--bg-panel-strong);
  border-bottom: 1px solid var(--border-default);
  flex-shrink: 0;
}

.motion-lights {
  display: flex;
  gap: 7px;
  flex-shrink: 0;
}

.motion-lights i {
  width: 11px;
  height: 11px;
  border-radius: 50%;
  display: block;
}

.motion-lights .r { background: #ff5f57; }
.motion-lights .y { background: #febc2e; }
.motion-lights .g { background: #28c840; }

.motion-name {
  font-size: 12.5px;
  font-weight: 600;
  letter-spacing: 0.3px;
  color: var(--text-muted);
}

.motion-spacer {
  flex: 1;
}

.motion-tb-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  background: var(--bg-panel);
  border: 1px solid var(--border-default);
  color: var(--text-muted);
  border-radius: 3px;
  padding: 4px 10px;
  font-size: 12px;
  cursor: pointer;
  transition: all 0.15s ease;
  white-space: nowrap;
}

.motion-tb-btn:hover {
  color: var(--text-primary);
  border-color: var(--accent-primary);
}

.motion-tb-btn:active {
  transform: scale(0.96);
}

/* ============================================================
   主内容区
   ============================================================ */
.motion-body {
  flex: 1;
  min-height: 0;
  display: flex;
  background: var(--bg-canvas);
}
</style>

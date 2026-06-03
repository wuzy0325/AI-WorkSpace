<script setup lang="ts">
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
  <div data-test="motion-shell" class="bg-[color:var(--bg-canvas)] text-[color:var(--text-primary)] font-sans flex flex-col overflow-hidden" :class="embedded ? 'h-full min-h-0' : 'h-screen'">
    <header class="glass-header flex items-center justify-between px-5 py-3 shrink-0">
      <div>
        <h1 class="text-sm font-bold tracking-tight text-[color:var(--text-primary)]">运动控制器</h1>
        <p class="text-[10px] font-semibold tracking-wider text-[color:var(--text-muted)]">{{ embedded ? '轴控制与监控' : '独立窗口 · 轴控制与监控' }}</p>
      </div>
      <div class="flex items-center gap-2">
        <button
          class="h-8 w-8 rounded-md text-sm transition-all border border-[color:var(--border-default)] text-[color:var(--text-muted)] hover:text-[color:var(--text-primary)] hover:bg-[color:var(--bg-panel-strong)] active:scale-95 flex items-center justify-center"
          :title="theme.mode === 'dark' ? i18n.t.switchToLightTheme : i18n.t.switchToDarkTheme"
          @click="theme.toggleTheme"
        >
          <span v-if="theme.mode === 'dark'">☀</span>
          <span v-else>☾</span>
        </button>
        <button
          class="h-8 px-3 rounded-md text-xs font-semibold transition-all border border-[color:var(--border-default)] text-[color:var(--text-muted)] hover:text-[color:var(--text-primary)] hover:bg-[color:var(--bg-panel-strong)] active:scale-95"
          :title="i18n.locale === 'zh' ? i18n.t.switchToEnglish : i18n.t.switchToChinese"
          @click="toggleLanguage"
        >
          {{ i18n.locale === 'zh' ? 'EN' : '中文' }}
        </button>
      </div>
    </header>
    <main class="flex-1 min-h-0 p-4 bg-[color:var(--bg-canvas)]">
      <MotionControlPanel />
    </main>
  </div>
</template>

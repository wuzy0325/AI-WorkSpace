<script setup lang="ts">
import MotionControlPanel from '@components/motion/MotionControlPanel.vue'
import { useThemeStore } from '@stores/themeStore'
import { useI18nStore } from '@stores/i18nStore'

withDefaults(
  defineProps<{
    embedded?: boolean
  }>(),
  { embedded: false },
)

const theme = useThemeStore()
const i18n = useI18nStore()

function closeCurrentWindow(): void {
  window.close()
}

function toggleLocale(): void {
  i18n.setLocale(i18n.locale === 'zh' ? 'en' : 'zh')
}
</script>

<template>
  <div data-test="motion-shell" class="bg-[color:var(--bg-canvas)] text-[color:var(--text-primary)] font-sans flex flex-col overflow-hidden" :class="embedded ? 'h-full min-h-0' : 'h-screen'">
    <header class="glass-header flex items-center justify-between px-5 py-3 shrink-0">
      <div>
        <h1 class="text-sm font-bold tracking-tight text-[color:var(--text-primary)]">{{ i18n.t.motionController }}</h1>
        <p class="text-[10px] font-semibold tracking-wider text-[color:var(--text-muted)]">{{ embedded ? i18n.t.axisControlAndMonitor : i18n.t.standaloneWindowAxisControlAndMonitor }}</p>
      </div>
      <div class="flex items-center gap-2">
        <button
          class="h-8 px-2.5 rounded-md text-[10px] font-bold tracking-wide transition-all border border-[color:var(--border-default)] text-[color:var(--text-muted)] hover:text-[color:var(--text-primary)] hover:bg-[color:var(--bg-panel-strong)] active:scale-95"
          @click="toggleLocale"
          :title="i18n.locale === 'zh' ? i18n.t.switchToEnglish : i18n.t.switchToChinese"
        >
          {{ i18n.locale === 'zh' ? 'EN' : '中文' }}
        </button>
        <button
          class="h-8 w-8 rounded-md flex items-center justify-center transition-all border border-[color:var(--border-default)] text-[color:var(--text-muted)] hover:text-[color:var(--text-primary)] hover:bg-[color:var(--bg-panel-strong)] active:scale-95"
          @click="theme.toggleTheme()"
          :title="theme.mode === 'dark' ? i18n.t.switchToLightTheme : i18n.t.switchToDarkTheme"
        >
          <svg v-if="theme.mode === 'dark'" class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <circle cx="12" cy="12" r="5"/><line x1="12" y1="1" x2="12" y2="3"/><line x1="12" y1="21" x2="12" y2="23"/><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"/><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"/><line x1="1" y1="12" x2="3" y2="12"/><line x1="21" y1="12" x2="23" y2="12"/><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"/><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"/>
          </svg>
          <svg v-else class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/>
          </svg>
        </button>
        <button
          v-if="!embedded"
          class="h-8 px-3 rounded-md text-xs font-semibold transition-all border border-[color:var(--border-default)] text-[color:var(--text-muted)] hover:text-[color:var(--text-primary)] hover:bg-[color:var(--bg-panel-strong)] active:scale-95"
          @click="closeCurrentWindow"
        >
          {{ i18n.t.close }}
        </button>
      </div>
    </header>
    <main class="flex-1 min-h-0 p-4 bg-[color:var(--bg-canvas)]">
      <MotionControlPanel />
    </main>
  </div>
</template>

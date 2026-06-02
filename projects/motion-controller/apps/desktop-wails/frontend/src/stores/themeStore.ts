import { defineStore } from 'pinia'
import { ref, watch } from 'vue'

export type ThemeMode = 'dark' | 'light'

const STORAGE_KEY = 'motion-controller.theme'

function getInitialTheme(): ThemeMode {
  try {
    const stored = localStorage.getItem(STORAGE_KEY)
    if (stored === 'light' || stored === 'dark') return stored
  } catch { /* 忽略 */ }
  if (typeof window !== 'undefined' && window.matchMedia('(prefers-color-scheme: light)').matches) {
    return 'light'
  }
  return 'dark'
}

function applyTheme(mode: ThemeMode): void {
  if (typeof document !== 'undefined') {
    document.documentElement.setAttribute('data-theme', mode)
  }
}

export const useThemeStore = defineStore('theme', () => {
  const mode = ref<ThemeMode>(getInitialTheme())

  function setTheme(newMode: ThemeMode): void {
    mode.value = newMode
    applyTheme(newMode)
    try {
      localStorage.setItem(STORAGE_KEY, newMode)
    } catch { /* 忽略 */ }
  }

  function toggleTheme(): void {
    setTheme(mode.value === 'dark' ? 'light' : 'dark')
  }

  watch(mode, (m) => applyTheme(m), { immediate: true })

  return { mode, setTheme, toggleTheme }
})

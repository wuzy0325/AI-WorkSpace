import { computed, ref } from 'vue'
import { defineStore } from 'pinia'

export type ThemeMode = 'dark' | 'light'

function resolveInitialTheme(): ThemeMode {
  try {
    const stored = localStorage.getItem('wind-daq.theme')
    if (stored === 'light' || stored === 'dark') return stored
  } catch {
    // localStorage not available
  }
  // 检测系统主题偏好
  if (typeof window !== 'undefined' && window.matchMedia) {
    const prefersLight = window.matchMedia('(prefers-color-scheme: light)').matches
    if (prefersLight) return 'light'
  }
  return 'dark'
}

function applyTheme(theme: ThemeMode) {
  document.documentElement.dataset.theme = theme
  if (theme === 'dark') {
    document.documentElement.classList.add('dark')
  } else {
    document.documentElement.classList.remove('dark')
  }
  try {
    localStorage.setItem('wind-daq.theme', theme)
  } catch {
    // localStorage not available
  }
}

export const useThemeStore = defineStore('theme', () => {
  const theme = ref<ThemeMode>(resolveInitialTheme())
  const nextTheme = computed<ThemeMode>(() => (theme.value === 'dark' ? 'light' : 'dark'))

  function initializeTheme() {
    applyTheme(theme.value)
  }

  function setTheme(value: ThemeMode) {
    theme.value = value
    applyTheme(value)
  }

  function toggleTheme() {
    setTheme(nextTheme.value)
  }

  return { theme, nextTheme, initializeTheme, setTheme, toggleTheme }
})

import { computed, ref } from 'vue'
import { defineStore } from 'pinia'

export type ThemeMode = 'dark' | 'light'

function resolveInitialTheme(): ThemeMode {
  try {
    const stored = localStorage.getItem('windlabx4.theme')
    if (stored === 'light' || stored === 'dark') return stored
  } catch {
    // localStorage not available
  }
  // 系统偏好深色时跟随深色，否则使用浅色（默认主题，见 DESIGN.md "Theme"）
  if (typeof window !== 'undefined' && window.matchMedia) {
    const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches
    if (prefersDark) return 'dark'
  }
  return 'light'
}

function applyTheme(theme: ThemeMode) {
  document.documentElement.dataset.theme = theme
  if (theme === 'dark') {
    document.documentElement.classList.add('dark')
  } else {
    document.documentElement.classList.remove('dark')
  }
  try {
    localStorage.setItem('windlabx4.theme', theme)
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

import { ref, watch } from 'vue'

type Theme = 'dark' | 'light'

const STORAGE_KEY = 'wista-theme'

function loadInitial(): Theme {
  if (typeof document === 'undefined') return 'dark'
  const attr = document.documentElement.getAttribute('data-theme')
  if (attr === 'light' || attr === 'dark') return attr
  try {
    const stored = localStorage.getItem(STORAGE_KEY)
    if (stored === 'light' || stored === 'dark') return stored
  } catch {
    // localStorage not available
  }
  return 'dark'
}

const current = ref<Theme>(loadInitial())

function applyTheme(theme: Theme) {
  if (typeof document === 'undefined') return
  document.documentElement.setAttribute('data-theme', theme)
  try {
    localStorage.setItem(STORAGE_KEY, theme)
  } catch {
    // ignore
  }
}

applyTheme(current.value)

watch(current, applyTheme)

export function useTheme() {
  function toggle() {
    current.value = current.value === 'dark' ? 'light' : 'dark'
  }

  function setTheme(value: Theme) {
    current.value = value
  }

  return { theme: current, toggle, setTheme }
}

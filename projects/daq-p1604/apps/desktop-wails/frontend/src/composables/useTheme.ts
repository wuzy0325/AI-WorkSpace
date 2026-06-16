import { ref } from 'vue'

type Theme = 'light' | 'dark'

const theme = ref<Theme>((localStorage.getItem('daq-p1604-theme') as Theme) || 'dark')

export function useTheme() {
  function toggleTheme() {
    theme.value = theme.value === 'dark' ? 'light' : 'dark'
    localStorage.setItem('daq-p1604-theme', theme.value)
  }

  return { theme, toggleTheme }
}

import { ref } from 'vue'

type Theme = 'light' | 'dark'

const theme = ref<Theme>((localStorage.getItem('daq-p1604-theme') as Theme) || 'light')

// 将主题同步到 <html data-theme="..."> 属性
function applyThemeToDom(value: Theme): void {
  document.documentElement.setAttribute('data-theme', value)
}

applyThemeToDom(theme.value)

export function useTheme() {
  function toggleTheme() {
    const next: Theme = theme.value === 'dark' ? 'light' : 'dark'
    theme.value = next
    applyThemeToDom(next)
    localStorage.setItem('daq-p1604-theme', next)
  }

  return { theme, toggleTheme }
}

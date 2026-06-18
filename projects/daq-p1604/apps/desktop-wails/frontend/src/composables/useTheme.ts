import { ref, watch } from 'vue'

type Theme = 'light' | 'dark'

// 全局主题状态：从 localStorage 读取，默认深色
const theme = ref<Theme>((localStorage.getItem('daq-p1604-theme') as Theme) || 'dark')

/**
 * 将主题同步到 <html data-theme="..."> 属性
 * CSS 变量通过 [data-theme='light'] / [data-theme='dark'] 选择器区分
 */
function applyThemeToDom(value: Theme): void {
  document.documentElement.setAttribute('data-theme', value)
}

// 初始化时立即应用一次，确保首屏 CSS 变量正确
applyThemeToDom(theme.value)

// 监听主题变化，同步到 DOM 和 localStorage
watch(
  theme,
  (value) => {
    applyThemeToDom(value)
    localStorage.setItem('daq-p1604-theme', value)
  },
)

export function useTheme() {
  function toggleTheme() {
    theme.value = theme.value === 'dark' ? 'light' : 'dark'
  }

  return { theme, toggleTheme }
}

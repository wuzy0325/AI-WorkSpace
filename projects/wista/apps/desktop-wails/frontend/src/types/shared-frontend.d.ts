declare module '@shared-frontend/index' {
  import type { DefineComponent } from 'vue'
  export const NaiveThemeProvider: DefineComponent<{
    theme: 'dark' | 'light'
    themeOverrides?: Record<string, any>
  }>
}

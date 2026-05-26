import { computed } from 'vue'
import { useThemeStore } from '@stores/themeStore'
import type { ChartTheme } from '../types'

export function useTraversalChartTheme() {
  const themeStore = useThemeStore()

  const chartTheme = computed<ChartTheme>(() => {
    const isDark = themeStore.theme === 'dark'

    return {
      textColor: isDark ? '#cbd5e1' : '#475569',
      axisColor: isDark ? '#64748b' : '#94a3b8',
      gridColor: isDark ? '#334155' : '#e2e8f0',
      panelColor: 'transparent',
      tooltipBackground: isDark ? 'rgba(15, 23, 42, 0.96)' : 'rgba(255, 255, 255, 0.96)',
      tooltipBorder: isDark ? '#334155' : '#cbd5e1',
      heatmapColors: isDark
        ? ['#172554', '#2563eb', '#0891b2', '#10b981', '#facc15', '#ef4444']
        : ['#dbeafe', '#60a5fa', '#22d3ee', '#34d399', '#facc15', '#f87171']
    }
  })

  return { chartTheme }
}


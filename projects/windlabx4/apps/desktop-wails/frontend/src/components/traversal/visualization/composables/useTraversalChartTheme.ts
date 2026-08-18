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
        : ['#dbeafe', '#60a5fa', '#22d3ee', '#34d399', '#facc15', '#f87171'],
      // 主系列颜色：dark 用天蓝、light 用靛蓝，与 heatmap 主色保持视觉一致
      seriesPrimary: isDark ? '#38bdf8' : '#3b82f6',
      // 热力图 hover 高亮边框：dark 用浅灰、light 用白
      emphasisBorder: isDark ? '#e2e8f0' : '#ffffff',
      // 雷达图填充色：主色带 28% 透明度
      radarAreaFill: isDark ? 'rgba(56, 189, 248, 0.28)' : 'rgba(59, 130, 246, 0.28)',
      // 雷达图背景分区色：奇偶交替，营造分层感
      radarSplitArea: isDark
        ? ['rgba(56, 189, 248, 0.06)', 'rgba(56, 189, 248, 0.12)']
        : ['rgba(59, 130, 246, 0.06)', 'rgba(59, 130, 246, 0.12)']
    }
  })

  return { chartTheme }
}


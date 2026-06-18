<script setup lang="ts">
import { computed, ref, watch, onMounted } from 'vue'
import VChart from 'vue-echarts'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { LineChart } from 'echarts/charts'
import { GridComponent, TooltipComponent, LegendComponent } from 'echarts/components'
import { useDeviceStore } from '@stores/deviceStore'
import { useTheme } from '@composables/useTheme'

use([CanvasRenderer, LineChart, GridComponent, TooltipComponent, LegendComponent])

const props = withDefaults(defineProps<{
  deviceId?: string
  maxPoints?: number
}>(), { deviceId: '', maxPoints: 120 })

const deviceStore = useDeviceStore()
const { theme } = useTheme()

const COLORS = [
  '#3b82f6', '#10b981', '#f59e0b', '#a855f7',
  '#f43f5e', '#06b6d4', '#f97316', '#6366f1',
  '#84cc16', '#14b8a6', '#d946ef', '#0ea5e9',
  '#eab308', '#22c55e', '#ef4444', '#8b5cf6',
  '#ec4899', '#64748b',
]

interface ThemeColors {
  text: string
  muted: string
  grid: string
  tooltipBg: string
  tooltipBorder: string
}

function readThemeColors(): ThemeColors {
  if (typeof document === 'undefined') {
    return { text: '#94a3b8', muted: '#64748b', grid: 'rgba(148,163,184,0.08)', tooltipBg: '#0f172a', tooltipBorder: 'rgba(148,163,184,0.18)' }
  }
  const styles = getComputedStyle(document.documentElement)
  const read = (name: string, fallback: string) => {
    const value = styles.getPropertyValue(name).trim()
    return value || fallback
  }
  return {
    text: read('--text-secondary', '#94a3b8'),
    muted: read('--text-muted', '#64748b'),
    grid: read('--chart-grid', 'rgba(148,163,184,0.08)'),
    tooltipBg: read('--chart-tooltip-bg', 'rgba(15,23,42,0.95)'),
    tooltipBorder: read('--chart-tooltip-border', 'rgba(148,163,184,0.18)'),
  }
}

const palette = ref<ThemeColors>(readThemeColors())
const chartReady = ref(false)

// 延迟标记图表就绪，避免初始渲染闪烁
onMounted(() => {
  requestAnimationFrame(() => {
    chartReady.value = true
  })
})

watch(theme, () => {
  // 延迟读取以确保 CSS 变量已更新
  requestAnimationFrame(() => {
    palette.value = readThemeColors()
  })
})

const option = computed(() => {
  const history = deviceStore.historyFor(props.deviceId).slice(-props.maxPoints)
  const times = history.map((d) => {
    const date = new Date(d.timestamp)
    return date.toLocaleTimeString('zh-CN', { hour12: false })
  })

  const channels = deviceStore.selectedProfile?.channels ?? []
  const selectedChannels = channels
    .filter((ch) => ch.enabled && deviceStore.isChartSelected(props.deviceId, ch.index))

  const c = palette.value

  const series = selectedChannels.map((ch, i) => {
    const color = ch.color || COLORS[i % COLORS.length]
    return {
      name: ch.name || `CH${ch.index + 1}`,
      type: 'line' as const,
      data: history.map((d) => {
        const v = d.values[ch.index]
        return typeof v === 'number' && !Number.isNaN(v) ? v : null
      }),
      smooth: true,
      symbol: 'none',
      showSymbol: false,
      lineStyle: { width: 1.75, color },
      itemStyle: { color },
      emphasis: { focus: 'series' as const, lineStyle: { width: 2.5 } },
      areaStyle: {
        color: {
          type: 'linear', x: 0, y: 0, x2: 0, y2: 1,
          colorStops: [
            { offset: 0, color: color + '32' },
            { offset: 1, color: color + '00' },
          ],
        },
      },
    }
  })

  return {
    backgroundColor: 'transparent',
    animation: false,
    tooltip: {
      trigger: 'axis',
      backgroundColor: c.tooltipBg,
      borderColor: c.tooltipBorder,
      borderWidth: 1,
      padding: [8, 12],
      textStyle: { color: c.text, fontSize: 12 },
      axisPointer: {
        type: 'line',
        lineStyle: { color: c.text, opacity: 0.4, type: 'dashed' as const },
      },
      // 按通道精度格式化 tooltip 数值（dataIndex 对应 series 顺序）
      valueFormatter: (value: unknown, dataIndex: number) => {
        if (value === null || value === undefined || typeof value !== 'number') return '-'
        const ch = selectedChannels[dataIndex]
        const p = ch ? (ch.precision ?? 3) : 3
        return value.toFixed(p)
      },
    },
    legend: {
      data: selectedChannels.map((ch) => ch.name || `CH${ch.index + 1}`),
      textStyle: { color: c.muted, fontSize: 10 },
      icon: 'roundRect',
      itemWidth: 8,
      itemHeight: 8,
      top: 4,
      right: 8,
    },
    grid: { left: 48, right: 16, top: 32, bottom: 28 },
    xAxis: {
      type: 'category' as const,
      data: times,
      axisLine: { show: false },
      axisTick: { show: false },
      splitLine: { show: false },
      axisLabel: {
        fontSize: 10,
        color: c.muted,
        fontFamily: 'ui-monospace, monospace',
        interval: Math.max(0, Math.floor(times.length / 6)),
      },
    },
    yAxis: {
      type: 'value' as const,
      splitLine: { lineStyle: { color: c.grid, type: 'dashed' as const } },
      axisLine: { show: false },
      axisTick: { show: false },
      axisLabel: { fontSize: 10, color: c.muted },
    },
    series,
  }
})

const hasData = computed(() => deviceStore.historyFor(props.deviceId).length > 0)
const selectedChannelCount = computed(() => {
  const channels = deviceStore.selectedProfile?.channels ?? []
  return channels.filter((ch) => ch.enabled && deviceStore.isChartSelected(props.deviceId, ch.index)).length
})
</script>

<template>
  <div class="chart">
    <VChart
      v-if="hasData && chartReady && selectedChannelCount > 0"
      :key="theme"
      :option="option"
      autoresize
      class="chart__canvas"
    />
    <div v-else class="chart__empty">
      <div class="chart__empty-pulse"></div>
      <p class="chart__empty-text">
        {{ !hasData ? '等待实时数据...' : '未选择通道' }}
      </p>
      <p class="chart__empty-hint">
        {{ !hasData ? '设备开始采集后将自动显示波形' : '请在上方通道选择中勾选需要显示的通道' }}
      </p>
    </div>
  </div>
</template>

<style scoped>
.chart {
  width: 100%;
  height: 100%;
  min-height: 0;
  position: relative;
  background-image:
    radial-gradient(circle at 1px 1px, var(--chart-grid) 1px, transparent 0);
  background-size: 24px 24px;
  background-position: 0 0;
  border-radius: var(--radius-md);
}

.chart__canvas {
  width: 100%;
  height: 100%;
}

.chart__empty {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 0.5rem;
  color: var(--text-muted);
}

.chart__empty-pulse {
  width: 56px;
  height: 56px;
  border-radius: 50%;
  border: 2px solid var(--accent-border);
  position: relative;
  margin-bottom: 0.5rem;
}

.chart__empty-pulse::after {
  content: '';
  position: absolute;
  inset: -2px;
  border-radius: 50%;
  border: 2px solid var(--accent);
  animation: empty-breathe 1.8s ease-in-out infinite;
}

.chart__empty-text {
  font-size: var(--font-size-sm);
  font-weight: 700;
  letter-spacing: 0.04em;
}

.chart__empty-hint {
  font-size: var(--font-size-xs);
  color: var(--text-muted);
  opacity: 0.8;
  max-width: 28rem;
  text-align: center;
  line-height: 1.5;
}

/* 减少动画偏好 */
@media (prefers-reduced-motion: reduce) {
  .chart__empty-pulse::after {
    animation: none;
    opacity: 0.5;
  }
}
</style>

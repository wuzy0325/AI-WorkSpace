<script setup lang="ts">
import { computed, ref, watch, onMounted } from 'vue'
import VChart from 'vue-echarts'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { LineChart } from 'echarts/charts'
import { GridComponent, TooltipComponent, LegendComponent } from 'echarts/components'
import { useDeviceStore } from '@stores/deviceStore'
import { useDisplayStore } from '@stores/displayStore'
import { useI18nStore } from '@stores/i18nStore'
import { useTheme } from '@composables/useTheme'
import { channelDisplayName } from '../../utils/channelDisplayName'

use([CanvasRenderer, LineChart, GridComponent, TooltipComponent, LegendComponent])

const props = withDefaults(defineProps<{
  deviceId?: string
}>(), { deviceId: '' })

const deviceStore = useDeviceStore()
const displayStore = useDisplayStore()
const i18n = useI18nStore()
const { theme } = useTheme()

// 18 通道波形配色：剔除容易与「警告/异常」混淆的橙黄（amber/orange/yellow），
// 改用蓝青绿紫粉等冷色/中性色，降低视觉噪音并避免误读。
const COLORS = [
  '#3b82f6', '#10b981', '#8b5cf6', '#06b6d4',
  '#f43f5e', '#14b8a6', '#6366f1', '#22c55e',
  '#a855f7', '#0ea5e9', '#ec4899', '#84cc16',
  '#64748b', '#d946ef', '#ef4444', '#4f46e5',
  '#0891b2', '#be185d',
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
  // 按"时间窗口(秒)"截取历史：cutoff = 当前时间 - windowMs
  // 与刷新率解耦：无论用户选 2Hz 还是 30Hz，图表横轴都保留相同的秒数
  const fullHistory = deviceStore.historyFor(props.deviceId)
  const windowMs = displayStore.historyWindowSec * 1000
  const nowMs = fullHistory.length > 0
    ? fullHistory[fullHistory.length - 1]!.timestamp
    : Date.now()
  const cutoff = nowMs - windowMs
  // 从右向左找到第一个 timestamp < cutoff 的位置作为起点，避免每帧全表遍历
  let startIdx = 0
  for (let i = fullHistory.length - 1; i >= 0; i--) {
    if (fullHistory[i]!.timestamp < cutoff) {
      startIdx = i + 1
      break
    }
  }
  const history = fullHistory.slice(startIdx)
  const times = history.map((d) => {
    const date = new Date(d.timestamp)
    return date.toLocaleTimeString(i18n.timeLocale, { hour12: false })
  })

  const channels = deviceStore.selectedProfile?.channels ?? []
  const selectedChannels = channels
    .filter((ch) => ch.enabled && deviceStore.isChartSelected(props.deviceId, ch.index))

  const c = palette.value

  const series = selectedChannels.map((ch, i) => {
    const color = ch.color || COLORS[i % COLORS.length]
    const name = channelDisplayName(ch.index, ch.name, i18n.t)
    return {
      name,
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
      // 按通道精度格式化 tooltip 数值：axis trigger 下 params 为数组，
      // 通过 seriesName 匹配通道，避免误用 dataIndex 导致精度错位
      formatter: (params: unknown) => {
        if (!Array.isArray(params) || params.length === 0) return ''
        const first = params[0] as { name?: string }
        const time = first.name ?? ''
        const rows = params.map((p: any) => {
          const seriesName = (p.seriesName as string) ?? ''
          const ch = selectedChannels.find(
            (c) => channelDisplayName(c.index, c.name, i18n.t) === seriesName,
          )
          const precision = ch ? (ch.precision ?? 3) : 3
          const rawValue = Array.isArray(p.value) ? p.value[1] : p.value
          const valueText =
            typeof rawValue === 'number' && !Number.isNaN(rawValue)
              ? rawValue.toFixed(precision)
              : '-'
          const marker = (p.marker as string) ?? ''
          return `${marker}${seriesName}: <strong>${valueText}</strong>`
        })
        return `<div style="font-weight:600;margin-bottom:4px">${time}</div>${rows.join('<br/>')}`
      },
    },
    legend: {
      data: selectedChannels.map((ch) => channelDisplayName(ch.index, ch.name, i18n.t)),
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
        {{ !hasData ? i18n.t('chart.waitingData') : i18n.t('chart.noChannelSelected') }}
      </p>
      <p class="chart__empty-hint">
        {{ !hasData ? i18n.t('chart.willShowWhenAcquiring') : i18n.t('chart.pleaseSelectChannel') }}
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

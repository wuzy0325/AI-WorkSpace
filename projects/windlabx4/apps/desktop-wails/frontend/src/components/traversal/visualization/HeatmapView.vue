<script setup lang="ts">
import { computed, ref, type ComputedRef } from 'vue'
import type { EChartsOption } from 'echarts'
import type { TraversalDataPoint } from '@shared/types/traversal'
import { useI18nStore } from '@stores/i18nStore'
import { VISUALIZATION_PARAM_CONFIG, type HeatmapCell, type VisualizationParam } from './types'
import { useECharts } from './composables/useECharts'
import { useHeatmapData } from './composables/useHeatmapData'
import { useScreenshotExport } from './composables/useScreenshotExport'
import { useTraversalChartTheme } from './composables/useTraversalChartTheme'
import { useThrottledChartUpdate } from './composables/useThrottledChartUpdate'

const props = defineProps<{
  dataPoints: TraversalDataPoint[]
  param: VisualizationParam
}>()

const i18n = useI18nStore()
const t = computed(() => i18n.t)
const chartRef = ref<HTMLElement | null>(null)
const { chart } = useECharts(chartRef)
const { chartTheme } = useTraversalChartTheme()
const { exportScreenshot } = useScreenshotExport(chart)

const dataPointsRef = computed(() => props.dataPoints)
const paramRef = computed(() => props.param)
// 仅取 heatmapData/valueRange——xAxis/yAxis 改 value 类型后不再需要 alphaValues/betaValues
const { heatmapData, valueRange } = useHeatmapData(dataPointsRef, paramRef)

const paramConfig = computed(() => VISUALIZATION_PARAM_CONFIG[props.param])
const paramLabel = computed(() => t.value[paramConfig.value.labelKey] ?? paramConfig.value.fallbackLabel)
const hasData = computed(() => heatmapData.value.length > 0)

interface HeatmapTooltipParam {
  data?: HeatmapCell
}

function isHeatmapTooltipParam(value: unknown): value is HeatmapTooltipParam {
  return typeof value === 'object' && value !== null && 'data' in value
}

function updateChart(): void {
  if (!chart.value) return

  const theme = chartTheme.value
  const option: EChartsOption = {
    backgroundColor: theme.panelColor,
    title: {
      text: `${paramLabel.value} ${t.value.heatmap}`,
      left: 'center',
      textStyle: { color: theme.textColor, fontSize: 14, fontWeight: 600 }
    },
    tooltip: {
      trigger: 'item',
      backgroundColor: theme.tooltipBackground,
      borderColor: theme.tooltipBorder,
      textStyle: { color: theme.textColor },
      formatter: (params: unknown) => {
        if (!isHeatmapTooltipParam(params) || !params.data) return ''
        const value = params.data.value[2]
        return `${t.value.pointAlpha}: ${params.data.alpha.toFixed(2)} deg<br/>${t.value.pointBeta}: ${params.data.beta.toFixed(2)} deg<br/>${paramLabel.value}: ${value.toFixed(4)} ${paramConfig.value.unit}`
      }
    },
    grid: { left: 56, right: 88, top: 56, bottom: 48 },
    // 改用 value 类型：原 category 模式按索引均匀排列，非均匀布点（密集小角度+稀疏大角度）
    // 会被等距显示，单元格大小不能反映真实 α 间距，造成视觉失真。
    xAxis: {
      type: 'value',
      name: t.value.alphaAxis,
      nameLocation: 'middle',
      nameGap: 30,
      axisLine: { lineStyle: { color: theme.axisColor } },
      axisLabel: { color: theme.textColor },
      nameTextStyle: { color: theme.textColor },
      splitLine: { show: true, lineStyle: { color: theme.gridColor } }
    },
    yAxis: {
      type: 'value',
      name: t.value.betaAxis,
      nameLocation: 'middle',
      nameGap: 42,
      axisLine: { lineStyle: { color: theme.axisColor } },
      axisLabel: { color: theme.textColor },
      nameTextStyle: { color: theme.textColor },
      splitLine: { show: true, lineStyle: { color: theme.gridColor } }
    },
    visualMap: {
      min: valueRange.value[0],
      max: valueRange.value[1],
      calculable: true,
      right: 8,
      top: 'middle',
      inRange: { color: theme.heatmapColors },
      textStyle: { color: theme.textColor }
    },
    series: [{
      type: 'heatmap',
      data: heatmapData.value,
      // 高亮边框颜色走 theme，dark/light 切换时同步
      emphasis: { itemStyle: { borderColor: theme.emphasisBorder, borderWidth: 1 } }
    }]
  }

  chart.value.setOption(option, true)
}

// rAF 节流：1000+ 点高频推送下避免每帧多次 setOption 全量重绘。
// 注意：xAxis/yAxis 改 value 类型后 updateChart 不再读取 alphaValues/betaValues，
// 因此 watch 列表只保留 heatmapData/valueRange 等实际影响渲染的源。
useThrottledChartUpdate([chart, heatmapData, valueRange, chartTheme, paramLabel], updateChart, { immediate: true })
</script>

<template>
  <div class="relative h-full min-h-[360px] w-full">
    <div v-if="!hasData" class="absolute inset-0 z-10 flex items-center justify-center text-sm text-[color:var(--text-muted)]">
      {{ t.noVisualizationData }}
    </div>
    <div ref="chartRef" class="h-full w-full"></div>
    <!-- 截图导出按钮：鼠标悬停时显示，避免遮挡图表内容 -->
    <button
      v-if="hasData"
      class="heatmap-export-btn"
      :title="t.exportPng || 'Export PNG'"
      @click="exportScreenshot(`heatmap-${paramLabel.replace(/\s+/g, '-')}`)"
    >
      {{ t.exportPng || 'Export PNG' }}
    </button>
  </div>
</template>

<style scoped>
.heatmap-export-btn {
  position: absolute;
  right: 8px;
  top: 8px;
  z-index: 20;
  padding: 4px 8px;
  border-radius: var(--radius-md, 4px);
  border: 1px solid var(--border-default);
  background: var(--bg-panel);
  font-size: var(--text-xs, 12px);
  color: var(--text-secondary);
  cursor: pointer;
  opacity: 0;
  transition: opacity 200ms ease, background 120ms ease;
}

.heatmap-export-btn:hover {
  background: var(--bg-panel-strong);
}

.heatmap-export-btn:focus-visible {
  opacity: 1;
}

.relative:hover .heatmap-export-btn {
  opacity: 1;
}
</style>


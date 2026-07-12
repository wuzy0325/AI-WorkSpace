<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { EChartsOption } from 'echarts'
import type {
  CustomSeriesRenderItem,
  CustomSeriesRenderItemAPI,
  CustomSeriesRenderItemParams,
  CustomSeriesRenderItemReturn
} from 'echarts'
import type { CustomSeriesOption } from 'echarts'
import type { TraversalDataPoint } from '@shared/types/traversal'
import { useI18nStore } from '@stores/i18nStore'
import { useECharts } from './composables/useECharts'
import { useScreenshotExport } from './composables/useScreenshotExport'
import { useTraversalChartTheme } from './composables/useTraversalChartTheme'

const props = defineProps<{
  dataPoints: TraversalDataPoint[]
}>()

const i18n = useI18nStore()
const t = computed(() => i18n.t)
const chartRef = ref<HTMLElement | null>(null)
const { chart } = useECharts(chartRef)
const { chartTheme } = useTraversalChartTheme()
const { exportScreenshot } = useScreenshotExport(chart)

interface VectorPoint {
  value: [number, number, number]
  measuredAlpha: number
  measuredBeta: number
  directionRad: number
}

// 插值有效 + alpha/beta 均为有限数才参与矢量场（line 模式 beta=null 时跳过）
const vectorData = computed<VectorPoint[]>(() => props.dataPoints
  .filter((point) =>
    point.interpolationResult.isValid
    && typeof point.coordinates.alpha === 'number'
    && Number.isFinite(point.coordinates.alpha)
    && typeof point.coordinates.beta === 'number'
    && Number.isFinite(point.coordinates.beta)
  )
  .map((point) => ({
    value: [point.coordinates.alpha as number, point.coordinates.beta as number, point.interpolationResult.velocity],
    measuredAlpha: point.interpolationResult.alpha,
    measuredBeta: point.interpolationResult.beta,
    directionRad: Math.atan2(point.interpolationResult.beta, point.interpolationResult.alpha)
  })))

const hasData = computed(() => vectorData.value.length > 0)

function rangeFor(index: 0 | 1 | 2): [number, number] {
  const values = vectorData.value.map((point) => point.value[index])
  if (values.length === 0) return [0, 1]
  const min = Math.min(...values)
  const max = Math.max(...values)
  return min === max ? [min - 1, max + 1] : [min, max]
}

interface VectorTooltipParam {
  data?: VectorPoint
}

function isVectorTooltipParam(value: unknown): value is VectorTooltipParam {
  return typeof value === 'object' && value !== null && 'data' in value
}

const renderVector: CustomSeriesRenderItem = (
  params: CustomSeriesRenderItemParams,
  api: CustomSeriesRenderItemAPI
): CustomSeriesRenderItemReturn => {
  const data = vectorData.value[params.dataIndex]
  if (!data) return undefined

  const point = api.coord([data.value[0], data.value[1]])
  const velocityRange = rangeFor(2)
  const span = velocityRange[1] - velocityRange[0]
  const normalizedVelocity = span === 0 ? 0.5 : (data.value[2] - velocityRange[0]) / span
  const arrowLength = 10 + normalizedVelocity * 14
  const headLength = 5
  const rad = data.directionRad
  const x1 = point[0] - Math.cos(rad) * arrowLength * 0.5
  const y1 = point[1] - Math.sin(rad) * arrowLength * 0.5
  const x2 = point[0] + Math.cos(rad) * arrowLength * 0.5
  const y2 = point[1] + Math.sin(rad) * arrowLength * 0.5
  const left = rad + Math.PI * 0.78
  const right = rad - Math.PI * 0.78

  return {
    type: 'group',
    children: [
      {
        type: 'line',
        shape: { x1, y1, x2, y2 },
        style: { stroke: '#38bdf8', lineWidth: 2 }
      },
      {
        type: 'polygon',
        shape: {
          points: [
            [x2, y2],
            [x2 + Math.cos(left) * headLength, y2 + Math.sin(left) * headLength],
            [x2 + Math.cos(right) * headLength, y2 + Math.sin(right) * headLength]
          ]
        },
        style: { fill: '#38bdf8' }
      }
    ]
  }
}

function updateChart(): void {
  if (!chart.value) return

  const theme = chartTheme.value
  const alphaRange = rangeFor(0)
  const betaRange = rangeFor(1)
  const velocityRange = rangeFor(2)
  const option: EChartsOption = {
    backgroundColor: theme.panelColor,
    title: {
      text: t.value.vectorField,
      subtext: 'direction: interpolated alpha/beta, length: velocity',
      left: 'center',
      textStyle: { color: theme.textColor, fontSize: 14, fontWeight: 600 },
      subtextStyle: { color: theme.textColor }
    },
    tooltip: {
      trigger: 'item',
      backgroundColor: theme.tooltipBackground,
      borderColor: theme.tooltipBorder,
      textStyle: { color: theme.textColor },
      formatter: (params: unknown) => {
        if (!isVectorTooltipParam(params) || !params.data) return ''
        const [alpha, beta, velocity] = params.data.value
        return `point alpha: ${alpha.toFixed(2)} deg<br/>point beta: ${beta.toFixed(2)} deg<br/>flow alpha: ${params.data.measuredAlpha.toFixed(2)} deg<br/>flow beta: ${params.data.measuredBeta.toFixed(2)} deg<br/>velocity: ${velocity.toFixed(3)} m/s`
      }
    },
    grid: { left: 56, right: 88, top: 64, bottom: 48 },
    xAxis: {
      type: 'value',
      min: alphaRange[0],
      max: alphaRange[1],
      name: 'alpha (deg)',
      nameLocation: 'middle',
      nameGap: 30,
      axisLine: { lineStyle: { color: theme.axisColor } },
      axisLabel: { color: theme.textColor },
      nameTextStyle: { color: theme.textColor },
      splitLine: { lineStyle: { color: theme.gridColor } }
    },
    yAxis: {
      type: 'value',
      min: betaRange[0],
      max: betaRange[1],
      name: 'beta (deg)',
      nameLocation: 'middle',
      nameGap: 42,
      axisLine: { lineStyle: { color: theme.axisColor } },
      axisLabel: { color: theme.textColor },
      nameTextStyle: { color: theme.textColor },
      splitLine: { lineStyle: { color: theme.gridColor } }
    },
    visualMap: {
      min: velocityRange[0],
      max: velocityRange[1],
      dimension: 2,
      calculable: true,
      right: 8,
      top: 'middle',
      inRange: { color: theme.heatmapColors },
      textStyle: { color: theme.textColor }
    },
    series: [{
      type: 'custom',
      renderItem: renderVector,
      data: vectorData.value
    } satisfies CustomSeriesOption]
  }

  chart.value.setOption(option, true)
}

watch([chart, vectorData, chartTheme, t], updateChart, { immediate: true })
</script>

<template>
  <div class="relative h-full min-h-[360px] w-full">
    <div v-if="!hasData" class="absolute inset-0 z-10 flex items-center justify-center text-sm text-[color:var(--text-muted)]">
      {{ t.noVisualizationData }}
    </div>
    <div ref="chartRef" class="h-full w-full"></div>
    <!-- 截图导出按钮：鼠标悬停时显示 -->
    <button
      v-if="hasData"
      class="vector-field-export-btn"
      :title="t.exportPng || 'Export PNG'"
      @click="exportScreenshot('vector-field')"
    >
      {{ t.exportPng || 'Export PNG' }}
    </button>
  </div>
</template>

<style scoped>
.vector-field-export-btn {
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

.vector-field-export-btn:hover {
  background: var(--bg-panel-strong);
}

.vector-field-export-btn:focus-visible {
  opacity: 1;
}

.relative:hover .vector-field-export-btn {
  opacity: 1;
}
</style>


<script setup lang="ts">
import { computed, ref } from 'vue'
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
import { useThrottledChartUpdate } from './composables/useThrottledChartUpdate'

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
  // 方向用 α（攻角）作为偏航极角——遍历测试中 α/β 是探针姿态控制变量，
  // 来流方向固定为风洞轴向；箭头沿 α 方向投射、长度为速度大小，
  // 物理上反映"该姿态下来流相对探针的偏置方向与速度量级"。
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
    directionRad: (point.coordinates.alpha as number) * Math.PI / 180
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

// visualMap.dimension 对 custom series 不生效——原代码声明 dimension: 2 但 renderVector
// 内固定用 '#38bdf8'，所有箭头同色。这里手动根据 normalizedVelocity 在 theme.heatmapColors
// 之间插值取色，让箭头颜色随 velocity 渐变。
function pickColorByVelocity(normalized: number, colors: string[]): string {
  if (colors.length === 0) return '#38bdf8'
  if (colors.length === 1) return colors[0]
  const clamped = Math.min(1, Math.max(0, normalized))
  const idx = clamped * (colors.length - 1)
  const lo = Math.floor(idx)
  const hi = Math.min(colors.length - 1, lo + 1)
  // 简化处理：取就近的颜色（避免在 hex 字符串上做 RGB 插值的复杂度）
  return idx - lo < 0.5 ? colors[lo] : colors[hi]
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
  // 箭头颜色按 velocity 从 theme.heatmapColors 取色
  const arrowColor = pickColorByVelocity(normalizedVelocity, chartTheme.value.heatmapColors)

  return {
    type: 'group',
    children: [
      {
        type: 'line',
        shape: { x1, y1, x2, y2 },
        style: { stroke: arrowColor, lineWidth: 2 }
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
        style: { fill: arrowColor }
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
      subtext: t.value.vectorFieldSubtext,
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
        return `${t.value.pointAlpha}: ${alpha.toFixed(2)} deg<br/>${t.value.pointBeta}: ${beta.toFixed(2)} deg<br/>${t.value.flowAlpha}: ${params.data.measuredAlpha.toFixed(2)} deg<br/>${t.value.flowBeta}: ${params.data.measuredBeta.toFixed(2)} deg<br/>${t.value.velocityLabel}: ${velocity.toFixed(3)} m/s`
      }
    },
    grid: { left: 56, right: 88, top: 64, bottom: 48 },
    xAxis: {
      type: 'value',
      min: alphaRange[0],
      max: alphaRange[1],
      name: t.value.alphaAxis,
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
      name: t.value.betaAxis,
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

// rAF 节流：高频推送下避免每帧多次 setOption 全量重绘
useThrottledChartUpdate([chart, vectorData, chartTheme, t], updateChart, { immediate: true })
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


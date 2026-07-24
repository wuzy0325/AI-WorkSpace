<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { EChartsOption } from 'echarts'
import type { TraversalDataPoint } from '@shared/types/traversal'
import { useI18nStore } from '@stores/i18nStore'
import { getParamValue, VISUALIZATION_PARAM_CONFIG, type VisualizationParam } from './types'
import { useECharts } from './composables/useECharts'
import { useScreenshotExport } from './composables/useScreenshotExport'
import { useTraversalChartTheme } from './composables/useTraversalChartTheme'
import { useThrottledChartUpdate } from './composables/useThrottledChartUpdate'
import UiSelect from '@components/ui/UiSelect.vue'

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

const sectionType = ref<'beta' | 'alpha'>('beta')
const sectionValue = ref<number | null>(null)

// 浮点比较 epsilon：后端按 0.1 步长生成时累积误差会让 0.3 !== 0.30000000000000004，
// 严格相等会导致剖面图"明明有点位却显示空数据"。1e-6 容差覆盖典型浮点误差。
const FLOAT_EPSILON = 1e-6

// 插值有效 + alpha/beta 均为有限数才参与截面图（line 模式 beta=null 时跳过）
const validPoints = computed(() => props.dataPoints.filter((point) =>
  point.interpolationResult.isValid
  && typeof point.coordinates.alpha === 'number'
  && Number.isFinite(point.coordinates.alpha)
  && typeof point.coordinates.beta === 'number'
  && Number.isFinite(point.coordinates.beta)
))
const paramConfig = computed(() => VISUALIZATION_PARAM_CONFIG[props.param])
const paramLabel = computed(() => t.value[paramConfig.value.labelKey] ?? paramConfig.value.fallbackLabel)

function uniqueSortedEpsilon(values: number[]): number[] {
  // 按 epsilon 聚类去重：先排序，相邻差值 < epsilon 视为同一个值
  if (values.length === 0) return []
  const sorted = [...values].sort((a, b) => a - b)
  const result: number[] = [sorted[0]]
  for (let i = 1; i < sorted.length; i++) {
    if (Math.abs(sorted[i] - result[result.length - 1]) >= FLOAT_EPSILON) {
      result.push(sorted[i])
    }
  }
  return result
}

const sectionOptions = computed(() => {
  const key = sectionType.value
  return uniqueSortedEpsilon(validPoints.value.map((point) => point.coordinates[key] as number))
})

const chartData = computed<[number, number][]>(() => {
  if (sectionValue.value === null) return []

  const fixedKey = sectionType.value
  const xKey = fixedKey === 'beta' ? 'alpha' : 'beta'

  return validPoints.value
    // 浮点严格相等改为 epsilon 比较，避免累积误差漏点
    .filter((point) => Math.abs((point.coordinates[fixedKey] as number) - sectionValue.value!) < FLOAT_EPSILON)
    .map((point): [number, number] | null => {
      const value = getParamValue(point.interpolationResult, props.param)
      const x = point.coordinates[xKey]
      if (value === null || typeof x !== 'number' || !Number.isFinite(x)) return null
      return [x, value]
    })
    .filter((point): point is [number, number] => point !== null)
    .sort((a, b) => a[0] - b[0])
})

const hasData = computed(() => chartData.value.length > 0)

watch(sectionOptions, (values) => {
  if (values.length === 0) {
    sectionValue.value = null
    return
  }

  // 选中值失效判定也走 epsilon，避免严格不等导致重置到 values[0]
  const matched = sectionValue.value === null
    ? false
    : values.some((v) => Math.abs(v - sectionValue.value!) < FLOAT_EPSILON)
  if (!matched) {
    sectionValue.value = values[0]
  }
}, { immediate: true })

function updateChart(): void {
  if (!chart.value) return

  const theme = chartTheme.value
  const fixedLabel = sectionType.value === 'beta' ? t.value.fixedBeta : t.value.fixedAlpha
  const xLabel = sectionType.value === 'beta' ? t.value.alphaAxis : t.value.betaAxis
  const option: EChartsOption = {
    backgroundColor: theme.panelColor,
    title: {
      text: `${paramLabel.value} ${t.value.crossSection}`,
      subtext: sectionValue.value === null ? '' : `${fixedLabel} = ${sectionValue.value.toFixed(2)} deg`,
      left: 'center',
      textStyle: { color: theme.textColor, fontSize: 14, fontWeight: 600 },
      subtextStyle: { color: theme.textColor }
    },
    tooltip: {
      trigger: 'axis',
      backgroundColor: theme.tooltipBackground,
      borderColor: theme.tooltipBorder,
      textStyle: { color: theme.textColor }
    },
    grid: { left: 64, right: 28, top: 64, bottom: 52 },
    xAxis: {
      type: 'value',
      name: xLabel,
      nameLocation: 'middle',
      nameGap: 30,
      axisLine: { lineStyle: { color: theme.axisColor } },
      axisLabel: { color: theme.textColor },
      nameTextStyle: { color: theme.textColor },
      splitLine: { lineStyle: { color: theme.gridColor } }
    },
    yAxis: {
      type: 'value',
      name: paramConfig.value.unit ? `${paramLabel.value} (${paramConfig.value.unit})` : paramLabel.value,
      nameLocation: 'middle',
      nameGap: 48,
      axisLine: { lineStyle: { color: theme.axisColor } },
      axisLabel: { color: theme.textColor },
      nameTextStyle: { color: theme.textColor },
      splitLine: { lineStyle: { color: theme.gridColor } }
    },
    series: [{
      type: 'line',
      data: chartData.value,
      // §27 第 10 条：禁用 smooth（贝塞尔曲线）以降低约 30% 渲染开销，
      // 与 RealtimeChart.vue 的性能取舍保持一致。剖面图点数稀疏时折线足够清晰。
      smooth: false,
      symbolSize: 6,
      lineStyle: { color: theme.seriesPrimary, width: 2 },
      itemStyle: { color: theme.seriesPrimary }
    }]
  }

  chart.value.setOption(option, true)
}

// rAF 节流：高频推送下避免每帧多次 setOption 全量重绘
useThrottledChartUpdate([chart, chartData, chartTheme, paramLabel], updateChart, { immediate: true })
</script>

<template>
  <div class="flex h-full min-h-[360px] flex-col gap-3">
    <div class="flex flex-wrap items-center gap-3 text-xs text-[color:var(--text-secondary)]">
      <label class="flex items-center gap-2">
        {{ t.sectionType }}
        <UiSelect v-model="sectionType" :options="[{value:'beta',label:t.fixedBeta},{value:'alpha',label:t.fixedAlpha}]" style="min-width:100px" />
      </label>
      <label class="flex items-center gap-2">
        {{ t.sectionValue }}
        <UiSelect :model-value="sectionValue != null ? String(sectionValue) : ''" @update:model-value="sectionValue = $event ? Number($event) : null" :options="sectionOptions.map(v => ({value:String(v),label:v.toFixed(2)}))" style="min-width:100px" />
      </label>
    </div>
    <div class="relative min-h-0 flex-1">
      <div v-if="!hasData" class="absolute inset-0 z-10 flex items-center justify-center text-sm text-[color:var(--text-muted)]">
        {{ t.noVisualizationData }}
      </div>
      <div ref="chartRef" class="h-full w-full"></div>
      <!-- 截图导出按钮：鼠标悬停时显示 -->
      <button
        v-if="hasData"
        class="cross-section-export-btn"
        :title="t.exportPng || 'Export PNG'"
        @click="exportScreenshot(`cross-section-${paramLabel.replace(/\s+/g, '-')}`)"
      >
        {{ t.exportPng || 'Export PNG' }}
      </button>
    </div>
  </div>
</template>

<style scoped>
.cross-section-export-btn {
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

.cross-section-export-btn:hover {
  background: var(--bg-panel-strong);
}

.cross-section-export-btn:focus-visible {
  opacity: 1;
}

.relative:hover .cross-section-export-btn {
  opacity: 1;
}
</style>


<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { EChartsOption } from 'echarts'
import type { TraversalDataPoint } from '@shared/types/traversal'
import { useI18nStore } from '@stores/i18nStore'
import { getParamValue, VISUALIZATION_PARAM_CONFIG, type VisualizationParam } from './types'
import { useECharts } from './composables/useECharts'
import { useScreenshotExport } from './composables/useScreenshotExport'
import { useTraversalChartTheme } from './composables/useTraversalChartTheme'
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

const validPoints = computed(() => props.dataPoints.filter((point) => point.interpolationResult.isValid))
const paramConfig = computed(() => VISUALIZATION_PARAM_CONFIG[props.param])
const paramLabel = computed(() => t.value[paramConfig.value.labelKey] ?? paramConfig.value.fallbackLabel)

function uniqueSorted(values: number[]): number[] {
  return Array.from(new Set(values)).sort((a, b) => a - b)
}

const sectionOptions = computed(() => {
  const key = sectionType.value
  return uniqueSorted(validPoints.value.map((point) => point.coordinates[key]))
})

const chartData = computed<[number, number][]>(() => {
  if (sectionValue.value === null) return []

  const fixedKey = sectionType.value
  const xKey = fixedKey === 'beta' ? 'alpha' : 'beta'

  return validPoints.value
    .filter((point) => point.coordinates[fixedKey] === sectionValue.value)
    .map((point): [number, number] | null => {
      const value = getParamValue(point.interpolationResult, props.param)
      return value === null ? null : [point.coordinates[xKey], value]
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

  if (sectionValue.value === null || !values.includes(sectionValue.value)) {
    sectionValue.value = values[0]
  }
}, { immediate: true })

function updateChart(): void {
  if (!chart.value) return

  const theme = chartTheme.value
  const fixedLabel = sectionType.value === 'beta' ? 'beta' : 'alpha'
  const xLabel = sectionType.value === 'beta' ? 'alpha' : 'beta'
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
      name: `${xLabel} (deg)`,
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
      smooth: true,
      symbolSize: 6,
      lineStyle: { color: '#3b82f6', width: 2 },
      itemStyle: { color: '#3b82f6' }
    }]
  }

  chart.value.setOption(option, true)
}

watch([chart, chartData, chartTheme, paramLabel], updateChart, { immediate: true })
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


<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { EChartsOption } from 'echarts'
import type { TraversalDataPoint } from '@shared/types/traversal'
import { useI18nStore } from '@stores/i18nStore'
import { useECharts } from './composables/useECharts'
import { useScreenshotExport } from './composables/useScreenshotExport'
import { useTraversalChartTheme } from './composables/useTraversalChartTheme'
import UiButton from '@components/ui/UiButton.vue'

const props = defineProps<{
  dataPoints: TraversalDataPoint[]
}>()

const i18n = useI18nStore()
const t = computed(() => i18n.t)
const chartRef = ref<HTMLElement | null>(null)
const { chart } = useECharts(chartRef)
const { chartTheme } = useTraversalChartTheme()
const { exportScreenshot } = useScreenshotExport(chart)
const selectedIndex = ref(0)

const hasData = computed(() => props.dataPoints.length > 0)
const selectedPoint = computed(() => props.dataPoints[selectedIndex.value] ?? null)
const rawPressures = computed(() => {
  const point = selectedPoint.value
  if (!point) return []
  return [point.rawPressure.P1, point.rawPressure.P2, point.rawPressure.P3, point.rawPressure.P4, point.rawPressure.P5]
})

const radarValues = computed(() => {
  if (rawPressures.value.length === 0) return [0, 0, 0, 0, 0]
  const min = Math.min(...rawPressures.value)
  const max = Math.max(...rawPressures.value)
  const span = max - min || 1
  return rawPressures.value.map((value) => ((value - min) / span) * 100)
})

watch(() => props.dataPoints.length, (length) => {
  if (length === 0) {
    selectedIndex.value = 0
    return
  }

  if (selectedIndex.value >= length) {
    selectedIndex.value = length - 1
  }
}, { immediate: true })

function updateChart(): void {
  if (!chart.value) return

  const theme = chartTheme.value
  const point = selectedPoint.value
  const option: EChartsOption = {
    backgroundColor: theme.panelColor,
    title: {
      text: point ? `${t.value.pressureRadar} #${selectedIndex.value + 1}` : t.value.pressureRadar,
      subtext: point ? `alpha=${point.coordinates.alpha.toFixed(2)} deg beta=${point.coordinates.beta.toFixed(2)} deg` : '',
      left: 'center',
      textStyle: { color: theme.textColor, fontSize: 14, fontWeight: 600 },
      subtextStyle: { color: theme.textColor }
    },
    tooltip: {
      trigger: 'item',
      backgroundColor: theme.tooltipBackground,
      borderColor: theme.tooltipBorder,
      textStyle: { color: theme.textColor },
      formatter: () => rawPressures.value
        .map((value, index) => `P${index + 1}: ${value.toFixed(4)} kPa`)
        .join('<br/>')
    },
    radar: {
      indicator: ['P1', 'P2', 'P3', 'P4', 'P5'].map((name) => ({ name, max: 100 })),
      radius: '62%',
      axisName: { color: theme.textColor },
      splitLine: { lineStyle: { color: theme.gridColor } },
      splitArea: { areaStyle: { color: ['rgba(59,130,246,0.06)', 'rgba(59,130,246,0.12)'] } },
      axisLine: { lineStyle: { color: theme.axisColor } }
    },
    series: [{
      type: 'radar',
      data: [{
        value: radarValues.value,
        name: t.value.pressureRadar,
        areaStyle: { color: 'rgba(59, 130, 246, 0.28)' },
        lineStyle: { color: '#3b82f6', width: 2 },
        itemStyle: { color: '#3b82f6' }
      }]
    }]
  }

  chart.value.setOption(option, true)
}

watch([chart, selectedPoint, radarValues, chartTheme, t], updateChart, { immediate: true })
</script>

<template>
  <div class="grid h-full min-h-[360px] grid-cols-[1fr_13rem] gap-4 max-lg:grid-cols-1">
    <div class="relative min-h-0">
      <div v-if="!hasData" class="absolute inset-0 z-10 flex items-center justify-center text-sm text-[color:var(--text-muted)]">
        {{ t.noVisualizationData }}
      </div>
      <div ref="chartRef" class="h-full w-full"></div>
      <!-- 截图导出按钮：鼠标悬停时显示 -->
      <button
        v-if="hasData"
        class="pressure-radar-export-btn"
        :title="t.exportPng || 'Export PNG'"
        @click="exportScreenshot(`pressure-radar-${selectedIndex + 1}`)"
      >
        {{ t.exportPng || 'Export PNG' }}
      </button>
    </div>

    <aside class="min-h-0 overflow-hidden rounded-xl border border-[color:var(--border-default)] bg-[color:var(--bg-panel)] p-3">
      <h4 class="mb-2 text-xs font-semibold uppercase tracking-[0.14em] text-[color:var(--text-muted)]">{{ t.measurementPoints }}</h4>
      <div class="flex max-h-full flex-col gap-1 overflow-y-auto pr-1">
        <UiButton
          v-for="(point, index) in dataPoints"
          :key="point.pointId"
          size="sm"
          quaternary
          :class="selectedIndex === index ? 'bg-blue-500/15 text-blue-500' : ''"
          @click="selectedIndex = index"
        >
          #{{ point.pointId }} alpha={{ point.coordinates.alpha.toFixed(1) }} beta={{ point.coordinates.beta.toFixed(1) }}
        </UiButton>
      </div>
    </aside>
  </div>
</template>

<style scoped>
.pressure-radar-export-btn {
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

.pressure-radar-export-btn:hover {
  background: var(--bg-panel-strong);
}

.pressure-radar-export-btn:focus-visible {
  opacity: 1;
}

.relative:hover .pressure-radar-export-btn {
  opacity: 1;
}
</style>


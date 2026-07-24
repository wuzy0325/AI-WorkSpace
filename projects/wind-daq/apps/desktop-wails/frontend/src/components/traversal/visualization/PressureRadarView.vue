<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { EChartsOption } from 'echarts'
import type { TraversalDataPoint, TraversalRawPressure } from '@shared/types/traversal'
import { useI18nStore } from '@stores/i18nStore'
import { useECharts } from './composables/useECharts'
import { useScreenshotExport } from './composables/useScreenshotExport'
import { useTraversalChartTheme } from './composables/useTraversalChartTheme'
import { useThrottledChartUpdate } from './composables/useThrottledChartUpdate'
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

// 五孔探针有 P1-P5，三孔探针只有 P1-P3，后端缺失通道用 NaN 表示（非 0）。
// 原代码硬编码取 P1-P5，NaN 时 Math.min 返回 NaN 导致雷达图全图崩溃。
// 改为动态读取 rawPressure 中所有 Number.isFinite 的 P* 字段，
// 三孔/五孔探针都能正确显示。
//
// 注意：0 是合法压力值（静压参考点、传感器零位等），不能过滤。
// 全 0 时 span=0 → fallback 1，归一化结果全 0，雷达图退化为原点，
// 这是"全 0 压力"的正确视觉表现，不应人为剔除。
function extractValidPressures(raw: TraversalRawPressure): { name: string; value: number }[] {
  const entries: Array<[string, number]> = [
    ['P1', raw.P1], ['P2', raw.P2], ['P3', raw.P3],
    ['P4', raw.P4], ['P5', raw.P5]
  ]
  return entries
    .filter(([, value]) => typeof value === 'number' && Number.isFinite(value))
    .map(([name, value]) => ({ name, value }))
}

const pressureEntries = computed(() => {
  const point = selectedPoint.value
  if (!point) return []
  return extractValidPressures(point.rawPressure)
})

const radarValues = computed(() => {
  if (pressureEntries.value.length === 0) return []
  const values = pressureEntries.value.map((entry) => entry.value)
  const min = Math.min(...values)
  const max = Math.max(...values)
  const span = max - min || 1
  // 归一化到 [0, 100]，与 radar.indicator.max=100 对齐
  return values.map((value) => ((value - min) / span) * 100)
})

/** 安全格式化坐标：null/NaN 显示为 '--'，避免 toFixed 崩溃 */
function formatCoord(v: number | null | undefined, digits = 2): string {
  if (typeof v !== 'number' || Number.isNaN(v)) return '--'
  return v.toFixed(digits)
}

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
      // alpha/beta 允许 null（line 模式 Y 轴 NaN），toFixed 前必须做 number 守卫
      subtext: point
        ? `${t.value.alphaLabel}=${formatCoord(point.coordinates.alpha)} deg ${t.value.betaLabel}=${formatCoord(point.coordinates.beta)} deg`
        : '',
      left: 'center',
      textStyle: { color: theme.textColor, fontSize: 14, fontWeight: 600 },
      subtextStyle: { color: theme.textColor }
    },
    tooltip: {
      trigger: 'item',
      backgroundColor: theme.tooltipBackground,
      borderColor: theme.tooltipBorder,
      textStyle: { color: theme.textColor },
      formatter: () => pressureEntries.value
        .map((entry) => `${entry.name}: ${entry.value.toFixed(4)} ${t.value.pressureUnit}`)
        .join('<br/>')
    },
    radar: {
      // indicator 动态生成：三孔探针显示 3 顶点，五孔探针显示 5 顶点
      indicator: pressureEntries.value.map((entry) => ({ name: entry.name, max: 100 })),
      radius: '62%',
      axisName: { color: theme.textColor },
      splitLine: { lineStyle: { color: theme.gridColor } },
      // 雷达图背景分区色走 theme，dark/light 切换时同步
      splitArea: { areaStyle: { color: theme.radarSplitArea } },
      axisLine: { lineStyle: { color: theme.axisColor } }
    },
    series: [{
      type: 'radar',
      data: [{
        value: radarValues.value,
        name: t.value.pressureRadar,
        // 雷达图填充/线条/点颜色全部走 theme，dark/light 切换时同步
        areaStyle: { color: theme.radarAreaFill },
        lineStyle: { color: theme.seriesPrimary, width: 2 },
        itemStyle: { color: theme.seriesPrimary }
      }]
    }]
  }

  chart.value.setOption(option, true)
}

// rAF 节流：高频推送下避免每帧多次 setOption 全量重绘
useThrottledChartUpdate([chart, selectedPoint, radarValues, chartTheme, t], updateChart, { immediate: true })
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
          #{{ point.pointId }} {{ t.alphaLabel }}={{ formatCoord(point.coordinates.alpha, 1) }} {{ t.betaLabel }}={{ formatCoord(point.coordinates.beta, 1) }}
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


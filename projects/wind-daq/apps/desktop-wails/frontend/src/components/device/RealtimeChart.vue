<script setup lang="ts">
import { computed, ref, watch, onBeforeUnmount, nextTick } from 'vue'
import VChart from 'vue-echarts'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { LineChart } from 'echarts/charts'
import { GridComponent, TooltipComponent, DataZoomComponent } from 'echarts/components'
import { useDeviceStore } from '@stores/deviceStore'
import { buildChannelColorMap, CHANNEL_COLORS } from '@utils/channelColors'
import { useRafThrottle } from '@composables/useRafThrottle'

/** HTML 转义，防止用户输入的通道名嵌入 tooltip 时产生注入 */
function escapeHtml(s: string): string {
  const map: Record<string, string> = {
    '&': '&amp;', '<': '&lt;', '>': '&gt;',
    '"': '&quot;', "'": '&#x27;',
  }
  return s.replace(/[&<>"']/g, (c) => map[c])
}

use([CanvasRenderer, LineChart, GridComponent, TooltipComponent, DataZoomComponent])

const props = withDefaults(
  defineProps<{
    deviceId?: string
    channelIndices?: number[]
    maxPoints?: number
  }>(),
  { deviceId: '', channelIndices: () => [0, 1, 2, 3], maxPoints: 100 },
)

const deviceStore = useDeviceStore()

const profile = computed(() => deviceStore.profiles.find((p) => p.id === props.deviceId))

// 通道颜色映射
const channelColorMap = computed(() => {
  if (!profile.value) return new Map<number, string>()
  return buildChannelColorMap(profile.value.type, profile.value.channels ?? [])
})

// 通道精度/单位/名称元数据
const channelMeta = computed(() => {
  const map = new Map<number, { precision: number; unit: string; name: string }>()
  if (!profile.value) return map
  for (const ch of profile.value.channels ?? []) {
    map.set(ch.index, {
      precision: typeof ch.precision === 'number' && ch.precision >= 0 ? ch.precision : 3,
      unit: ch.unit ?? '',
      name: ch.name ?? `CH${ch.index + 1}`,
    })
  }
  return map
})

function formatTime(ts: number): string {
  const date = new Date(ts)
  const pad = (n: number) => n.toString().padStart(2, '0')
  return `${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`
}

// ===== 渲染层拆分 =====

// 获取 VChart 实例
const vchartRef = ref<InstanceType<typeof VChart> | null>(null)

// optionBase：静态结构，仅在 channelIndices/profile/theme 变化时重算
const optionBase = computed(() => {
  const metaMap = channelMeta.value
  const colorMap = channelColorMap.value

  // 收集当前可见通道的单位
  const visibleUnits = new Set<string>()
  for (const ch of props.channelIndices) {
    const u = metaMap.get(ch)?.unit ?? ''
    if (u) visibleUnits.add(u)
  }
  const yAxisUnit = visibleUnits.size === 1 ? Array.from(visibleUnits)[0] : ''

  // series 骨架（不含 data），ECharts 按 name 匹配增量数据
  const series = props.channelIndices.map((ch) => {
    const meta = metaMap.get(ch)
    const name = meta?.name || `CH${ch + 1}`
    const color = colorMap.get(ch) ?? CHANNEL_COLORS[0]
    return {
      name,
      _channelIndex: ch,
      type: 'line' as const,
      smooth: false,
      symbol: 'none',
      lineStyle: { width: 1.5 },
      itemStyle: { color },
      animation: false,
    }
  })

  return {
    tooltip: {
      trigger: 'axis' as const,
      axisPointer: { type: 'line' as const },
      formatter: (params: unknown): string => {
        const list = Array.isArray(params) ? params : [params]
        if (list.length === 0) return ''
        const first = list[0] as { axisValueLabel?: string; axisValue?: string | number }
        const rawTs = first.axisValue
        const header = typeof rawTs === 'number' ? formatTime(rawTs) : (first.axisValueLabel ?? '')
        const rows = list.map((item) => {
          const p = item as {
            seriesName?: string
            value?: number | null
            color?: string
            marker?: string
            seriesIndex?: number
          }
          const marker = typeof p.marker === 'string'
            ? p.marker
            : `<span style="display:inline-block;width:10px;height:10px;border-radius:50%;background:${p.color};margin-right:6px;"></span>`
          const raw = p.value
          const seriesDef = series[p.seriesIndex ?? -1] as { _channelIndex?: number } | undefined
          const meta = seriesDef && typeof seriesDef._channelIndex === 'number'
            ? metaMap.get(seriesDef._channelIndex)
            : undefined
          const precision = meta?.precision ?? 3
          const unit = meta?.unit ?? ''
          let text: string
          if (raw === null || raw === undefined || typeof raw !== 'number' || !Number.isFinite(raw)) {
            text = '—'
          } else {
            text = raw.toFixed(precision)
            if (unit) text += ` ${unit}`
          }
          return `<div style="display:flex;align-items:center;gap:6px;">${marker}<span style="flex:1;">${escapeHtml(p.seriesName ?? '')}</span><span style="font-weight:600;margin-left:12px;">${text}</span></div>`
        })
        return `<div style="font-size:12px;line-height:1.6;"><div style="font-weight:600;margin-bottom:4px;">${escapeHtml(header)}</div>${rows.join('')}</div>`
      },
    },
    grid: { left: 40, right: 16, top: yAxisUnit ? 22 : 8, bottom: 24 },
    xAxis: {
      type: 'time' as const,
      axisLine: { show: false },
      axisTick: { show: false },
      axisLabel: {
        fontSize: 10,
        color: '#64748b',
        formatter: (value: number) => formatTime(value),
      },
      splitLine: { show: false },
    },
    yAxis: {
      type: 'value' as const,
      name: yAxisUnit,
      nameLocation: 'end' as const,
      nameGap: 8,
      nameTextStyle: { fontSize: 10, color: '#64748b', fontWeight: 600 },
      splitLine: { lineStyle: { color: 'rgba(255,255,255,0.06)' } },
      axisLabel: { fontSize: 10, color: '#64748b' },
    },
    series,
  }
})

// seriesData：仅 historyVersion 变化时重算，返回带 name 的数据数组供增量 setOption
const seriesData = computed(() => {
  // 依赖 historyVersion 触发重新求值
  void deviceStore.historyVersion
  const data = deviceStore.historyFor(props.deviceId)
  const metaMap = channelMeta.value
  return props.channelIndices.map((ch) => {
    const meta = metaMap.get(ch)
    const name = meta?.name || `CH${ch + 1}`
    const points = data.map((d) => {
      const indices = Array.isArray(d.channelIndices) ? d.channelIndices : []
      const channels = Array.isArray(d.channels) ? d.channels : []
      const pos = indices.indexOf(ch)
      return pos >= 0 ? [d.timestamp, channels[pos]] : [d.timestamp, null]
    })
    return { name, data: points }
  })
})

// optionBase 变化时全量重建图表
watch(optionBase, (opt) => {
  const chart = vchartRef.value?.chart
  if (chart) {
    chart.setOption(opt, { notMerge: true })
  }
}, { immediate: false })

// rAF 合并层：seriesData 变化时增量更新
const rAF = useRafThrottle(() => {
  const chart = vchartRef.value?.chart
  if (!chart) return
  chart.setOption(
    { series: seriesData.value },
    { notMerge: false, lazyUpdate: true },
  )
})

watch(seriesData, () => {
  rAF.markDirty()
}, { flush: 'sync' })

onBeforeUnmount(() => {
  rAF.dispose()
})

// chartReady：VChart 挂载后首次全量初始化
const chartReady = ref(false)

function onChartReady() {
  chartReady.value = true
  nextTick(() => {
    const chart = vchartRef.value?.chart
    if (chart) {
      chart.setOption(optionBase.value, { notMerge: true })
    }
  })
}

const hasData = computed(() => deviceStore.historyFor(props.deviceId).length > 0)

// VChart 通过 v-if 卸载时重置 chartReady，确保重装后触发全量初始化
watch(hasData, (val) => {
  if (!val) chartReady.value = false
})
const selectedChannelCount = computed(() => {
  const channels = profile.value?.channels ?? []
  return channels.filter((ch) => ch.enabled && deviceStore.isChartSelected(props.deviceId, ch.index)).length
})
</script>

<template>
  <div class="realtime-chart">
    <VChart
      v-if="hasData && chartReady && selectedChannelCount > 0"
      ref="vchartRef"
      autoresize
      class="realtime-chart__canvas"
      @rendered="onChartReady"
    />
    <div v-else class="realtime-chart__empty">等待实时数据...</div>
  </div>
</template>

<style scoped>
.realtime-chart {
  width: 100%;
  height: 240px;
}

.realtime-chart__canvas {
  width: 100%;
  height: 100%;
}

.realtime-chart__empty {
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--text-muted);
  font-size: var(--font-size-sm);
}
</style>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import VChart from 'vue-echarts'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { LineChart } from 'echarts/charts'
import { GridComponent, TooltipComponent, DataZoomComponent } from 'echarts/components'
import { useDeviceStore } from '@stores/deviceStore'

/** HTML 转义，防止用户输入的通道名嵌入 tooltip 时产生注入；
 *  虽然当前是本地桌面应用，仍按最佳实践处理不可信输入 */
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
const CHANNEL_COLORS = ['#3b82f6', '#10b981', '#f59e0b', '#a855f7', '#f43f5e', '#06b6d4', '#f97316', '#6366f1']

const history = computed(() => deviceStore.historyFor(props.deviceId))

/**
 * 通道精度/单位/名称的元数据映射，供 tooltip formatter 使用。
 * key = 通道 index，value = { precision, unit, name }
 * 缺失通道 fallback 精度 3、单位 空、名称 CH{n}
 */
const channelMeta = computed(() => {
  const map = new Map<number, { precision: number; unit: string; name: string }>()
  const profile = deviceStore.profiles.find((p) => p.id === props.deviceId)
  if (!profile) return map
  for (const ch of profile.channels ?? []) {
    map.set(ch.index, {
      precision: typeof ch.precision === 'number' && ch.precision >= 0 ? ch.precision : 3,
      unit: ch.unit ?? '',
      name: ch.name ?? `CH${ch.index + 1}`,
    })
  }
  return map
})

const option = computed(() => {
  const data = history.value.slice(-props.maxPoints)
  const times = data.map((d) => {
    const date = new Date(d.timestamp)
    return date.toLocaleTimeString('zh-CN', { hour12: false })
  })
  const metaMap = channelMeta.value
  const series = props.channelIndices.map((ch, i) => {
    const meta = metaMap.get(ch)
    const name = meta?.name || `CH${ch + 1}`
    return {
      // series name 直接带上单位，方便用户识别；tooltip formatter 也基于此 name
      name,
      // 私有字段：把通道 index 携带下去，tooltip formatter 用它反查 precision/unit
      _channelIndex: ch,
      type: 'line' as const,
      data: data.map((d) => {
        const indices = Array.isArray(d.channelIndices) ? d.channelIndices : []
        const channels = Array.isArray(d.channels) ? d.channels : []
        const pos = indices.indexOf(ch)
        return pos >= 0 ? channels[pos] : null
      }),
      smooth: true,
      symbol: 'none',
      lineStyle: { width: 1.5 },
      itemStyle: { color: CHANNEL_COLORS[i % CHANNEL_COLORS.length] },
      areaStyle: {
        color: {
          type: 'linear',
          x: 0, y: 0, x2: 0, y2: 1,
          colorStops: [
            { offset: 0, color: CHANNEL_COLORS[i % CHANNEL_COLORS.length] + '40' },
            { offset: 1, color: CHANNEL_COLORS[i % CHANNEL_COLORS.length] + '05' },
          ],
        },
      },
    }
  })

  return {
    tooltip: {
      trigger: 'axis' as const,
      // axisPointer 使用十字线，方便对齐读数
      axisPointer: { type: 'line' as const },
      // 自定义 formatter：按 profile 中每个通道的 precision 格式化数值，并附单位。
      // 这样 hover 显示的精度与卡片、CSV 一致，避免出现"图上 6 位小数而设置只要 2 位"的落差。
      formatter: (params: unknown): string => {
        const list = Array.isArray(params) ? params : [params]
        if (list.length === 0) return ''
        const first = list[0] as { axisValueLabel?: string; axisValue?: string }
        const header = first.axisValueLabel ?? first.axisValue ?? ''
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
          // 反查通道 index → precision/unit
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
    grid: { left: 40, right: 16, top: 8, bottom: 24 },
    xAxis: {
      type: 'category' as const,
      data: times,
      axisLine: { show: false },
      axisTick: { show: false },
      axisLabel: { fontSize: 10, color: '#64748b' },
    },
    yAxis: {
      type: 'value' as const,
      splitLine: { lineStyle: { color: 'rgba(255,255,255,0.06)' } },
      axisLabel: { fontSize: 10, color: '#64748b' },
    },
    series,
  }
})

const loading = ref(false)
watch(history, () => { loading.value = false }, { deep: true })
</script>

<template>
  <div class="realtime-chart">
    <VChart
      v-if="history.length > 0"
      :option="option"
      autoresize
      class="realtime-chart__canvas"
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

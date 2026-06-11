<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import VChart from 'vue-echarts'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { LineChart } from 'echarts/charts'
import { GridComponent, TooltipComponent, DataZoomComponent } from 'echarts/components'
import { useDeviceStore } from '@stores/deviceStore'

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

const option = computed(() => {
  const data = history.value.slice(-props.maxPoints)
  const times = data.map((d) => {
    const date = new Date(d.timestamp)
    return date.toLocaleTimeString('zh-CN', { hour12: false })
  })
  const series = props.channelIndices.map((ch, i) => ({
    name: `CH${ch + 1}`,
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
  }))

  return {
    tooltip: { trigger: 'axis' },
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

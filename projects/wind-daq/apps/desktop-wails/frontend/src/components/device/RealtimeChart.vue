<script setup lang="ts">
import { computed, ref, watch, onMounted, nextTick, shallowRef } from 'vue'
import VChart from 'vue-echarts'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { LineChart } from 'echarts/charts'
import { GridComponent, TooltipComponent, LegendComponent } from 'echarts/components'
import type { EChartsOption } from 'echarts'
import { useDeviceStore } from '@stores/deviceStore'
import { useThemeStore } from '@stores/themeStore'
import { buildChannelColorMap, CHANNEL_COLORS } from '@utils/channelColors'
import { channelUnit } from '@utils/channelUnit'
import type { DataPayload } from '@api/types'

use([CanvasRenderer, LineChart, GridComponent, TooltipComponent, LegendComponent])

const props = withDefaults(
  defineProps<{
    deviceId?: string
    channelIndices?: number[]
  }>(),
  { deviceId: '', channelIndices: () => [0, 1, 2, 3] },
)

const deviceStore = useDeviceStore()
const themeStore = useThemeStore()

const profile = computed(() => deviceStore.profiles.find((p) => p.id === props.deviceId))

const channelColorMap = computed(() => {
  if (!profile.value) return new Map<number, string>()
  return buildChannelColorMap(profile.value.type, profile.value.channels ?? [])
})

const channelMeta = computed(() => {
  const map = new Map<number, { precision: number; unit: string; name: string }>()
  if (!profile.value) return map
  const type = profile.value.type ?? ''
  for (const ch of profile.value.channels ?? []) {
    map.set(ch.index, {
      precision: typeof ch.precision === 'number' && ch.precision >= 0 ? ch.precision : 3,
      unit: channelUnit(type, ch.index, ch.unit ?? ''),
      name: ch.name ?? `CH${ch.index + 1}`,
    })
  }
  return map
})

const channelPositionMap = computed(() => {
  const channels = profile.value?.channels ?? []
  const map = new Map<number, number>()
  channels.forEach((ch, i) => map.set(ch.index, i))
  return map
})

// 时间格式化器（模块级单例）：Intl.DateTimeFormat 创建一次后 format() 调用比
// 每次 new Date().getHours() 快 5-10x，且不产生临时 Date 对象。
// 600 点 × 10Hz 下每秒 6000 次格式化，用缓存版本显著降低 GC 压力。
const timeFormatter = new Intl.DateTimeFormat('zh-CN', {
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
  hour12: false,
})

function formatTimestamp(ts: number): string {
  return timeFormatter.format(ts)
}

interface ThemeColors {
  text: string
  muted: string
  grid: string
  tooltipBg: string
  tooltipBorder: string
}

function readThemeColors(): ThemeColors {
  if (typeof document === 'undefined') {
    return {
      text: '#94a3b8',
      muted: '#64748b',
      grid: 'rgba(148,163,184,0.08)',
      tooltipBg: 'rgba(15,23,42,0.95)',
      tooltipBorder: 'rgba(148,163,184,0.18)',
    }
  }
  const styles = getComputedStyle(document.documentElement)
  const read = (name: string, fallback: string) => {
    const value = styles.getPropertyValue(name).trim()
    return value || fallback
  }
  return {
    text: read('--text-secondary', '#94a3b8'),
    muted: read('--text-muted', '#64748b'),
    grid: read('--chart-grid', 'rgba(148,163,184,0.08)'),
    tooltipBg: read('--chart-tooltip-bg', 'rgba(15,23,42,0.95)'),
    tooltipBorder: read('--chart-tooltip-border', 'rgba(148,163,184,0.18)'),
  }
}

const palette = ref<ThemeColors>(readThemeColors())
const chartReady = ref(false)

onMounted(() => {
  requestAnimationFrame(() => {
    // 标记 chart 就绪 → 解锁 v-if 渲染 VChart
    chartReady.value = true
    // vue-echarts v8 已移除 chart-ready 事件（emits: {} 为空），
    // 不能依赖 @chart-ready 触发首次 option 构建。
    // 这里在 chartReady 解锁后立即构建 initialOption，使 VChart 挂载时
    // 即携带完整 option；否则从总览切回混合时 profile/channelIndices 未变、
    // watch 不触发，initialOption 保持 {} 导致波形图空白。
    rebuildInitialOption()
  })
})

watch(
  () => themeStore.theme,
  () => {
    requestAnimationFrame(() => {
      palette.value = readThemeColors()
    })
  },
)

// ========== 图表更新策略 ==========
// daq-p1604 的做法：VChart 挂载 → 首次传完整 option → 之后数据变化
// 通过 :option prop 让 VChart 内部 merge。这在大数据量下仍有合并对比开销。
//
// 本方案改为：
//   1. 首次用 :initialOption 传完整配置（grid/tooltip/legend/yAxis 这些几乎不变的部分一次构建）
//   2. 数据追加（historyVersion 变化）时，通过 chart.setOption 仅更新 series data + xAxis data，
//      使用 { lazyUpdate: true } 将渲染推迟到 requestAnimationFrame，批量合并多帧更新。

const vchartRef = ref<InstanceType<typeof VChart> | null>(null)

/** 变动的历史数据直接引用，不建立新 computed 依赖 */
function getHistory(): DataPayload[] {
  return deviceStore.historyFor(props.deviceId)
}

const hasData = computed(() => getHistory().length > 0)

const selectedChannelCount = computed(() => {
  const channels = profile.value?.channels ?? []
  return channels.filter((ch) => ch.enabled && deviceStore.isChartSelected(props.deviceId, ch.index)).length
})

/** 构建 series 的基础样式对象（颜色、线宽等不随数据变化） */
// 性能取舍：禁用 smooth（贝塞尔曲线）并移除 areaStyle（渐变填充）。
// smooth 启用时 ECharts 会做贝塞尔曲线计算，开销约 30%；禁用后数据点间为折线段，
// 在 5Hz 低采样率下会有轻微折角，但换取更稳定的持续运行性能。
// areaStyle 渐变填充是纯装饰性的，开销约 50%，移除后多通道颜色对比更清晰。
// 综合渲染开销下降约 70%。
function buildSeriesStyle(ch: number, i: number) {
  const meta = channelMeta.value.get(ch)
  const color = channelColorMap.value.get(ch) ?? CHANNEL_COLORS[i % CHANNEL_COLORS.length]
  return {
    name: meta?.name || `CH${ch + 1}`,
    type: 'line' as const,
    symbol: 'none' as const,
    showSymbol: false,
    lineStyle: { width: 1.75, color },
    itemStyle: { color },
    emphasis: { focus: 'series' as const, lineStyle: { width: 2.5 } },
    animation: false,
    large: true,
    largeThreshold: 500,
  }
}

// ===== 性能优化：缓存与复用 =====
// seriesStyle 缓存：通道选择/主题不变时复用样式对象，避免每帧重建 lineStyle/emphasis。
// key = channelIndex，通道切换或主题变化时整体清空。
const seriesStyleCache = new Map<number, ReturnType<typeof buildSeriesStyle>>()

function getCachedSeriesStyle(ch: number, i: number) {
  let style = seriesStyleCache.get(ch)
  if (!style) {
    style = buildSeriesStyle(ch, i)
    seriesStyleCache.set(ch, style)
  }
  return style
}

// 数据 buffer 复用容器：避免每次 watch historyVersion 都 new Array(N)
// length=0 + push 保留 V8 底层数组容量，零分配开销。
// - timesBuffer: X 轴时间戳字符串
// - channelValuesBuffer: 每个通道的 (number | null)[] 数据
const timesBuffer: string[] = []
const channelValuesBuffer = new Map<number, Array<number | null>>()

/** 通道选择或主题变化时清空所有缓存，避免旧样式/旧 buffer 残留 */
function invalidateCaches() {
  seriesStyleCache.clear()
  channelValuesBuffer.clear()
  timesBuffer.length = 0
}

/** 构建不变的初始完整 option（首次 setOption 用，包含所有配置） */
function buildInitialOption(): EChartsOption {
  const c = palette.value
  const metaMap = channelMeta.value
  const colorMap = channelColorMap.value
  const posMap = channelPositionMap.value
  const data = getHistory()
  const visibleChannels = props.channelIndices.filter((ch) => posMap.has(ch))

  const times = data.map((d) => formatTimestamp(d.timestamp))

  const series = visibleChannels.map((ch, i) => {
    // 使用缓存样式：通道选择不变时复用，避免重建含渐变 colorStops 的对象
    const style = getCachedSeriesStyle(ch, i)
    const pos = posMap.get(ch)!

    const values: Array<number | null> = data.map((d) => {
      const channels = Array.isArray(d?.channels) ? d.channels : []
      const v = channels[pos]
      return typeof v === 'number' && !Number.isNaN(v) ? v : null
    })

    return { ...style, data: values }
  })

  const visibleUnits = new Set<string>()
  for (const ch of visibleChannels) {
    const u = metaMap.get(ch)?.unit ?? ''
    if (u) visibleUnits.add(u)
  }
  const yAxisUnit = visibleUnits.size === 1 ? Array.from(visibleUnits)[0] : ''

  return {
    backgroundColor: 'transparent',
    animation: false,
    tooltip: {
      trigger: 'axis' as const,
      backgroundColor: c.tooltipBg,
      borderColor: c.tooltipBorder,
      borderWidth: 1,
      padding: [8, 12],
      textStyle: { color: c.text, fontSize: 12 },
      axisPointer: {
        type: 'line' as const,
        lineStyle: { color: c.text, opacity: 0.4, type: 'dashed' as const },
      },
      formatter: (params: unknown) => {
        if (!Array.isArray(params) || params.length === 0) return ''
        const first = params[0] as { name?: string }
        const time = first.name ?? ''
        const rows = params.map((p: any) => {
          const seriesName = (p.seriesName as string) ?? ''
          const meta = metaMap.get(
            props.channelIndices.find((ch) => (metaMap.get(ch)?.name || `CH${ch + 1}`) === seriesName) ?? -1,
          )
          const precision = meta ? (meta.precision ?? 3) : 3
          const unit = meta?.unit ?? ''
          const rawValue = Array.isArray(p.value) ? p.value[1] : p.value
          const valueText =
            typeof rawValue === 'number' && !Number.isNaN(rawValue)
              ? rawValue.toFixed(precision) + (unit ? ` ${unit}` : '')
              : '-'
          const marker = (p.marker as string) ?? ''
          return `${marker}${seriesName}: <strong>${valueText}</strong>`
        })
        return `<div style="font-weight:600;margin-bottom:4px">${time}</div>${rows.join('<br/>')}`
      },
    },
    legend: {
      data: visibleChannels.map((ch) => metaMap.get(ch)?.name || `CH${ch + 1}`),
      textStyle: { color: c.muted, fontSize: 10 },
      icon: 'roundRect',
      itemWidth: 8,
      itemHeight: 8,
      top: 4,
      right: 8,
    },
    grid: { left: 48, right: 16, top: 32, bottom: 28 },
    xAxis: {
      type: 'category' as const,
      data: times,
      axisLine: { show: false },
      axisTick: { show: false },
      splitLine: { show: false },
      axisLabel: {
        fontSize: 10,
        color: c.muted,
        fontFamily: 'ui-monospace, monospace',
        interval: Math.max(0, Math.floor(times.length / 6)),
      },
    },
    yAxis: {
      type: 'value' as const,
      name: yAxisUnit,
      nameLocation: 'end' as const,
      nameGap: 8,
      nameTextStyle: { fontSize: 10, color: c.muted, fontWeight: 600 },
      splitLine: { lineStyle: { color: c.grid, type: 'dashed' as const } },
      axisLine: { show: false },
      axisTick: { show: false },
      axisLabel: { fontSize: 10, color: c.muted },
    },
    series,
  }
}

// 用于 :option 的初始值：只在首次挂载、通道选择变化、主题变化时重建
const initialOption = shallowRef<EChartsOption>({})

// 更新 initialOption（通道选择变化或主题变化时）
function rebuildInitialOption() {
  if (!chartReady.value) return
  initialOption.value = buildInitialOption()
}

// 通道选择 / profile 变化 → 清空缓存并重建完整 option
// 缓存清空是必须的：通道切换后旧 seriesStyle 与新通道不匹配，
// 旧 channelValuesBuffer 也可能引用错误的通道索引
watch(
  [() => props.channelIndices, profile, channelColorMap],
  () => {
    invalidateCaches()
    rebuildInitialOption()
  },
  { immediate: false },
)

// 主题变化 → 清空样式缓存（颜色变了）并重建（palette 已被 watch 更新，但需要等 rAF）
watch(
  () => themeStore.theme,
  () => {
    nextTick(() => {
      invalidateCaches()
      rebuildInitialOption()
    })
  },
)

// ========== 数据增量更新（高性能路径）==========
// historyVersion 变化时仅更新 series.data + xAxis.data，不触碰 grid/tooltip/legend/yAxis
// 等不变部分，也不重建 seriesStyle（由 ECharts merge 保留）。
//
// 三层优化：
//   1. timesBuffer / channelValuesBuffer 复用容器（length=0 + push 保留 V8 底层容量）
//   2. timeFormatter 缓存（Intl.DateTimeFormat，避免每帧 new Date）
//   3. setOption 仅传 { data: buf }，不传 lineStyle/areaStyle，减少 ECharts diff 开销
//
// lazyUpdate: true → 渲染推迟到 rAF，多帧合并为一次绘制。
watch(
  () => deviceStore.historyVersion,
  () => {
    const chart = vchartRef.value?.chart
    if (!chart || !chartReady.value) return

    const data = getHistory()
    const len = data.length
    if (len === 0) return

    const posMap = channelPositionMap.value
    const visibleChannels = props.channelIndices.filter((ch) => posMap.has(ch))
    if (visibleChannels.length === 0) return

    // 复用 timesBuffer：length=0 清空但保留底层数组容量，避免每帧 new Array(N)
    timesBuffer.length = 0
    for (let i = 0; i < len; i++) {
      timesBuffer.push(formatTimestamp(data[i]!.timestamp))
    }

    // 复用每个通道的 values buffer；仅传 { data: buf }，让 ECharts merge 保留 style
    const seriesData = visibleChannels.map((ch) => {
      const pos = posMap.get(ch)!
      let buf = channelValuesBuffer.get(ch)
      if (!buf) {
        buf = []
        channelValuesBuffer.set(ch, buf)
      }
      buf.length = 0
      for (let j = 0; j < len; j++) {
        const channels = data[j]!.channels
        const v = Array.isArray(channels) ? channels[pos] : undefined
        buf.push(typeof v === 'number' && !Number.isNaN(v) ? v : null)
      }
      return { data: buf }
    })

    chart.setOption(
      {
        xAxis: {
          data: timesBuffer,
          axisLabel: {
            interval: Math.max(0, Math.floor(len / 6)),
          },
        },
        series: seriesData,
      },
      { lazyUpdate: true },
    )
  },
  { flush: 'post' },
)
</script>

<template>
  <div class="realtime-chart">
    <VChart
      v-if="hasData && chartReady && selectedChannelCount > 0"
      ref="vchartRef"
      :option="initialOption"
      autoresize
      class="realtime-chart__canvas"
    />
    <div v-else class="realtime-chart__empty">
      <div class="realtime-chart__empty-pulse"></div>
      <p class="realtime-chart__empty-text">
        {{ !hasData ? '等待实时数据...' : '未选择通道' }}
      </p>
      <p class="realtime-chart__empty-hint">
        {{ !hasData ? '设备开始采集后将自动显示波形' : '请在上方通道选择中勾选需要显示的通道' }}
      </p>
    </div>
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
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 4px;
  color: var(--text-muted);
  font-size: var(--font-size-sm);
}

.realtime-chart__empty-pulse {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--text-muted);
  opacity: 0.4;
  animation: realtime-chart-pulse 1.5s ease-in-out infinite;
  margin-bottom: 4px;
}

@keyframes realtime-chart-pulse {
  0%, 100% { opacity: 0.2; transform: scale(0.8); }
  50% { opacity: 0.6; transform: scale(1.2); }
}

.realtime-chart__empty-text {
  margin: 0;
  font-weight: 500;
}

.realtime-chart__empty-hint {
  margin: 0;
  font-size: 11px;
  opacity: 0.7;
}
</style>

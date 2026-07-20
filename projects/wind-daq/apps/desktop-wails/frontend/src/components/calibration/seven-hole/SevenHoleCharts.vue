<script setup lang="ts">
/**
 * 七孔探针校准 7 区域特性曲线图（spec Task 22 + 用户需求：7 个区域每个 3 类图表）
 *
 * 区域定义：1 个内区（P7 最大）+ 6 个外区扇区（P1~P6 最大）= 7 个区域
 *
 * 每个区域 3 类真实图表（布局完全一致，仅系数与坐标轴不同）：
 *   内区（α-β 系数）：
 *     1. Kα-Kβ 散点图：X 轴 Kα，Y 轴 Kβ，颜色按 α 角度渐变（visualMap）
 *     2. α-K0 曲线：X 轴 α，Y 轴 K0，多条曲线按 β 分组
 *     3. α-Ks 曲线：X 轴 α，Y 轴 Ks，多条曲线按 β 分组
 *   外区 N 区（θ-φ 系数，N=1..6）：
 *     1. Kθ-Kφ 散点图：X 轴 Kθ，Y 轴 Kφ，颜色按 θ 角度渐变（visualMap）
 *     2. φ-K0[n] 曲线：X 轴 φ，Y 轴 K0[n]，多条曲线按 θ 分组
 *     3. φ-Ks[n] 曲线：X 轴 φ，Y 轴 Ks[n]，多条曲线按 θ 分组
 *
 * 区域切换由父组件 SevenHoleMain.vue 通过 activeTab prop 控制（Tab 栏在父组件渲染），
 * 本组件复用 3 个 ECharts 实例，根据 activeTab 切换数据源与 option 配置。
 *
 * 严格遵循 spec Task 22 验收要求：
 *   - 使用 ECharts，配置 large: true + largeThreshold: 500 优化大数据量渲染
 *   - 颜色全部从 CSS 设计 token 读取（--text-* / --bg-* / --accent-* / --border-*）
 *   - i18n 完整：axis 名称 / tooltip / legend / 标题全部走 i18nStore
 *   - 数据更新通过 useThrottledChartUpdate 实现 rAF 节流，避免高频更新卡顿
 *   - unmount 时 useThrottledChartUpdate 自动取消挂起的 rAF + 主动 dispose ECharts 实例
 *   - 图表高度 160-180px，宽高比约 3:2（与既有图表组件一致）
 */
import { computed, ref, shallowRef, watch, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { storeToRefs } from 'pinia'
import { init, use, type ECharts } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { ScatterChart, LineChart } from 'echarts/charts'
import {
  GridComponent,
  TooltipComponent,
  LegendComponent,
  VisualMapComponent,
  TitleComponent,
} from 'echarts/components'
import type { EChartsOption } from 'echarts'
import { useI18nStore } from '@stores/i18nStore'
import { useThemeStore } from '@stores/themeStore'
import { useCalibrationStore } from '@stores/calibrationStore'
import { useThrottledChartUpdate } from '@components/traversal/visualization/composables/useThrottledChartUpdate'
import type { SevenHoleDataPoint, CalibrationAnyDataPoint } from '@shared/types/calibration'
import { Activity } from '@lucide/vue'

// ECharts 模块注册：仅在模块顶层执行一次，重复 use() 是 no-op
use([
  CanvasRenderer,
  ScatterChart,
  LineChart,
  GridComponent,
  TooltipComponent,
  LegendComponent,
  VisualMapComponent,
  TitleComponent,
])

// 七孔图表区域类型：内区 + 外区 1~6 区
// 父组件 SevenHoleMain.vue 通过 Tab 栏切换，状态由父组件管理
type SevenHoleChartTab = 'inner' | 'outer-1' | 'outer-2' | 'outer-3' | 'outer-4' | 'outer-5' | 'outer-6'

const props = defineProps<{
  activeTab: SevenHoleChartTab
}>()

const { t } = storeToRefs(useI18nStore())
const themeStore = useThemeStore()
const calibrationStore = useCalibrationStore()

// 三个内区图表 DOM 引用
const scatterEl = ref<HTMLDivElement | null>(null)
const k0CurveEl = ref<HTMLDivElement | null>(null)
const ksCurveEl = ref<HTMLDivElement | null>(null)

// ECharts 实例（shallowRef 避免 Vue 对 ECharts 实例做 deep reactive 包裹，
// ECharts 内部已自有状态管理，deep reactive 会导致 setOption 性能劣化）
const scatterChart = shallowRef<ECharts | null>(null)
const k0CurveChart = shallowRef<ECharts | null>(null)
const ksCurveChart = shallowRef<ECharts | null>(null)

let resizeObserver: ResizeObserver | null = null

// ===== 主题色读取 =====
// 与 RealtimeChart.vue / ThreeHoleChart.vue 一致：从 <html> 读取 CSS 变量，
// 主题切换时由 themeStore.theme watch 触发 palette 更新 → 触发图表重绘。
// fallback 取 dark 主题下的色值，确保 SSR / 样式未加载时也能在暗色背景上可见。
interface ChartThemeColors {
  text: string
  muted: string
  grid: string
  tooltipBg: string
  tooltipBorder: string
  visualMapColors: string[]
}

function readThemeColors(): ChartThemeColors {
  if (typeof document === 'undefined') {
    return {
      text: '#cbd5e1',
      muted: '#64748b',
      grid: 'rgba(148,163,184,0.08)',
      tooltipBg: 'rgba(15,23,42,0.95)',
      tooltipBorder: 'rgba(148,163,184,0.18)',
      // visualMap 渐变色：从冷蓝到暖红覆盖 α 范围
      visualMapColors: ['#1e40af', '#3b82f6', '#22d3ee', '#34d399', '#facc15', '#ef4444'],
    }
  }
  const styles = getComputedStyle(document.documentElement)
  const read = (name: string, fallback: string): string => {
    const value = styles.getPropertyValue(name).trim()
    return value || fallback
  }
  return {
    text: read('--text-primary', '#cbd5e1'),
    muted: read('--text-muted', '#64748b'),
    grid: read('--border-default', 'rgba(148,163,184,0.08)'),
    tooltipBg: read('--bg-canvas', 'rgba(15,23,42,0.95)'),
    tooltipBorder: read('--border-strong', 'rgba(148,163,184,0.18)'),
    // visualMap 用 accent 系列色：从冷到暖覆盖 α 范围
    visualMapColors: [
      read('--accent-info', '#3b82f6'),
      read('--accent-primary', '#38bdf8'),
      read('--accent-success', '#34d399'),
      read('--accent-warning', '#facc15'),
      read('--accent-danger', '#ef4444'),
    ],
  }
}

const palette = ref<ChartThemeColors>(readThemeColors())

// 主题切换：themeStore.theme 变化时刷新 palette，触发 useThrottledChartUpdate 节流重绘
watch(() => themeStore.theme, () => {
  palette.value = readThemeColors()
})

// ===== 数据派生 =====
// 类型谓词：从 CalibrationAnyDataPoint 联合类型中过滤出 SevenHoleDataPoint
// 七孔数据点的判别特征：region 字段为非空字符串 + coefficients 存在
function isSevenHoleDataPoint(p: CalibrationAnyDataPoint): p is SevenHoleDataPoint {
  const sh = p as Partial<SevenHoleDataPoint>
  return typeof sh.region === 'string' && sh.region.length > 0 && sh.coefficients !== undefined
}

// 内区数据点：仅 region === 'inner' 的点用于绘制
const innerDataPoints = computed<SevenHoleDataPoint[]>(() => {
  return calibrationStore.dataPoints
    .filter(isSevenHoleDataPoint)
    .filter((p) => p.region === 'inner')
})

// 内区采样点提取：从 coordinates 取 α/β，从 coefficients 取 Kalpha/Kbeta/K0/Ks
// 过滤掉 NaN/Infinity 防止 ECharts 绘制异常（数据点采集异常或部分字段缺失时）
interface InnerPoint {
  alpha: number
  beta: number
  Kalpha: number
  Kbeta: number
  K0: number
  Ks: number
}

const innerPoints = computed<InnerPoint[]>(() => {
  return innerDataPoints.value
    .map((p) => {
      const alpha = p.coordinates['α']
      const beta = p.coordinates['β']
      const { Kalpha, Kbeta, K0, Ks } = p.coefficients
      return { alpha, beta, Kalpha, Kbeta, K0, Ks }
    })
    .filter((p): p is InnerPoint =>
      Number.isFinite(p.alpha) && Number.isFinite(p.beta)
      && Number.isFinite(p.Kalpha) && Number.isFinite(p.Kbeta)
      && Number.isFinite(p.K0) && Number.isFinite(p.Ks)
    )
})

// 散点图数据：[Kalpha, Kbeta, alpha]，第三维（alpha）用于 visualMap 着色
const scatterData = computed<Array<[number, number, number]>>(() => {
  return innerPoints.value.map((p) => [p.Kalpha, p.Kbeta, p.alpha])
})

// α 范围：用于 visualMap 的 min/max
const alphaRange = computed<{ min: number; max: number }>(() => {
  if (scatterData.value.length === 0) return { min: -30, max: 30 }
  const alphas = scatterData.value.map((d) => d[2])
  const min = Math.min(...alphas)
  const max = Math.max(...alphas)
  // 范围为零时扩展 ±1，避免 visualMap 退化为单色
  return min === max ? { min: min - 1, max: max + 1 } : { min, max }
})

// 按 β 分组：返回按 β 升序排序的 [{ beta, data: [alpha, value][] }, ...]
// 每个 β 值对应一条曲线，data 内按 α 升序排列确保曲线连续不回环
interface BetaGroup {
  beta: number
  data: Array<[number, number]>
}

function groupByBeta(points: InnerPoint[], valueKey: 'K0' | 'Ks'): BetaGroup[] {
  const groups = new Map<number, Array<[number, number]>>()
  for (const p of points) {
    if (!groups.has(p.beta)) groups.set(p.beta, [])
    groups.get(p.beta)!.push([p.alpha, p[valueKey]])
  }
  return Array.from(groups.entries())
    .sort((a, b) => a[0] - b[0])
    .map(([beta, data]) => ({
      beta,
      data: data.sort((a, b) => a[0] - b[0]),
    }))
}

const k0GroupedData = computed<BetaGroup[]>(() => groupByBeta(innerPoints.value, 'K0'))
const ksGroupedData = computed<BetaGroup[]>(() => groupByBeta(innerPoints.value, 'Ks'))

// ===== 外区数据派生（按 activeTab 过滤扇区）=====
// activeTab='outer-N' 时取 region='outer' && sector=N 的点，
// 提取 θ/φ（从 coordinates）+ Ktheta/Kphi/K0Outer/KsOuter（从 coefficients）。
// 外区系数命名带扇区编号 n（spec §4.2/§4.3），但 SevenHoleCoefficients 类型已统一字段名（K0Outer/KsOuter），
// 扇区差异通过 sector 字段在数据点级别区分，无需在系数字段名上重复携带。
const activeSector = computed<number | null>(() => {
  if (props.activeTab === 'inner') return null
  // 'outer-1' → 1
  const match = props.activeTab.match(/^outer-(\d+)$/)
  return match ? parseInt(match[1], 10) : null
})

const outerDataPoints = computed<SevenHoleDataPoint[]>(() => {
  const sector = activeSector.value
  if (sector === null) return []
  return calibrationStore.dataPoints
    .filter(isSevenHoleDataPoint)
    .filter((p) => p.region === 'outer' && p.sector === sector)
})

interface OuterPoint {
  theta: number
  phi: number
  Ktheta: number
  Kphi: number
  K0Outer: number
  KsOuter: number
}

const outerPoints = computed<OuterPoint[]>(() => {
  return outerDataPoints.value
    .map((p) => {
      const theta = p.coordinates['θ']
      const phi = p.coordinates['φ']
      const { Ktheta, Kphi, K0Outer, KsOuter } = p.coefficients
      return { theta, phi, Ktheta, Kphi, K0Outer, KsOuter }
    })
    .filter((p): p is OuterPoint =>
      Number.isFinite(p.theta) && Number.isFinite(p.phi)
      && Number.isFinite(p.Ktheta) && Number.isFinite(p.Kphi)
      && Number.isFinite(p.K0Outer) && Number.isFinite(p.KsOuter)
    )
})

// 外区散点图数据：[Ktheta, Kphi, theta]，第三维（theta）用于 visualMap 着色
const outerScatterData = computed<Array<[number, number, number]>>(() => {
  return outerPoints.value.map((p) => [p.Ktheta, p.Kphi, p.theta])
})

// θ 范围：用于外区散点图 visualMap 的 min/max
const thetaRange = computed<{ min: number; max: number }>(() => {
  if (outerScatterData.value.length === 0) return { min: 0, max: 60 }
  const thetas = outerScatterData.value.map((d) => d[2])
  const min = Math.min(...thetas)
  const max = Math.max(...thetas)
  return min === max ? { min: min - 1, max: max + 1 } : { min, max }
})

// 按 θ 分组：返回按 θ 升序排序的 [{ theta, data: [phi, value][] }, ...]
// 每个 θ 值对应一条曲线，data 内按 φ 升序排列确保曲线连续不回环
interface ThetaGroup {
  theta: number
  data: Array<[number, number]>
}

function groupByTheta(points: OuterPoint[], valueKey: 'K0Outer' | 'KsOuter'): ThetaGroup[] {
  const groups = new Map<number, Array<[number, number]>>()
  for (const p of points) {
    if (!groups.has(p.theta)) groups.set(p.theta, [])
    groups.get(p.theta)!.push([p.phi, p[valueKey]])
  }
  return Array.from(groups.entries())
    .sort((a, b) => a[0] - b[0])
    .map(([theta, data]) => ({
      theta,
      data: data.sort((a, b) => a[0] - b[0]),
    }))
}

const outerK0GroupedData = computed<ThetaGroup[]>(() => groupByTheta(outerPoints.value, 'K0Outer'))
const outerKsGroupedData = computed<ThetaGroup[]>(() => groupByTheta(outerPoints.value, 'KsOuter'))

// ===== ECharts option 构建（内/外区共用，根据 activeTab 切换数据源与坐标轴名）=====

// 1. 散点图：内区 Kα-Kβ（按 α 渐变）/ 外区 Kθ-Kφ（按 θ 渐变）
function buildScatterOption(): EChartsOption {
  const c = palette.value
  const isInner = props.activeTab === 'inner'
  const data = isInner ? scatterData.value : outerScatterData.value
  const range = isInner ? alphaRange.value : thetaRange.value
  const xName = isInner ? t.value.shc_axisKa : t.value.shc_axisKtheta
  const yName = isInner ? t.value.shc_axisKb : t.value.shc_axisKphi
  const gradientLabel = isInner ? t.value.shc_alphaGradient : t.value.shc_thetaGradient
  // 公共 grid 配置：左侧 56px 容纳 Y 轴标题，右侧 48px 容纳 visualMap，底部 28px 容纳 X 轴刻度
  const grid = { left: 56, right: 48, top: 16, bottom: 28 }

  // 无数据时不再返回"暂无数据"占位，而是渲染完整空坐标轴 + 网格 + 颜色条
  // （散点 series 数据为空数组，visualMap 用 alphaRange/thetaRange 的 fallback min/max）
  // 这样切到外区 Tab 时操作员能看到完整的 Kθ-Kφ 坐标系框架，等采集数据到达后散点立即填入

  // tooltip formatter：内区反查 β，外区反查 φ（散点只携带 colorDim，第四维需从原始数据查找）
  const tooltipFormatter = (params: unknown): string => {
    const p = params as { value?: unknown }
    const val = p.value as [number, number, number] | undefined
    if (!val) return ''
    const [xVal, yVal, colorDim] = val
    if (isInner) {
      // 内区：xVal=Kα, yVal=Kβ, colorDim=α，反查 β
      const matched = innerPoints.value.find(
        (pt) => Math.abs(pt.Kalpha - xVal) < 1e-9
          && Math.abs(pt.Kbeta - yVal) < 1e-9
          && Math.abs(pt.alpha - colorDim) < 1e-9,
      )
      const beta = matched?.beta
      return t.value.shc_tooltipPoint
        .replace('{ka}', xVal.toFixed(3))
        .replace('{kb}', yVal.toFixed(3))
        .replace('{alpha}', colorDim.toFixed(1))
        .replace('{beta}', beta !== undefined ? beta.toFixed(1) : '--')
    }
    // 外区：xVal=Kθ, yVal=Kφ, colorDim=θ，反查 φ
    const matched = outerPoints.value.find(
      (pt) => Math.abs(pt.Ktheta - xVal) < 1e-9
        && Math.abs(pt.Kphi - yVal) < 1e-9
        && Math.abs(pt.theta - colorDim) < 1e-9,
    )
    const phi = matched?.phi
    return t.value.shc_tooltipOuterPoint
      .replace('{ktheta}', xVal.toFixed(3))
      .replace('{kphi}', yVal.toFixed(3))
      .replace('{theta}', colorDim.toFixed(1))
      .replace('{phi}', phi !== undefined ? phi.toFixed(1) : '--')
  }

  return {
    backgroundColor: 'transparent',
    animation: false,
    grid,
    tooltip: {
      trigger: 'item',
      backgroundColor: c.tooltipBg,
      borderColor: c.tooltipBorder,
      borderWidth: 1,
      padding: [8, 12],
      textStyle: { color: c.text, fontSize: 12 },
      formatter: tooltipFormatter,
    },
    xAxis: {
      type: 'value',
      name: xName,
      nameLocation: 'middle',
      nameGap: 18,
      nameTextStyle: { color: c.muted, fontSize: 11 },
      axisLine: { lineStyle: { color: c.muted } },
      axisTick: { lineStyle: { color: c.muted } },
      axisLabel: { color: c.muted, fontSize: 10 },
      splitLine: { lineStyle: { color: c.grid, type: 'dashed' } },
    },
    yAxis: {
      type: 'value',
      name: yName,
      nameLocation: 'middle',
      nameGap: 38,
      nameTextStyle: { color: c.muted, fontSize: 11 },
      axisLine: { lineStyle: { color: c.muted } },
      axisTick: { lineStyle: { color: c.muted } },
      axisLabel: { color: c.muted, fontSize: 10 },
      splitLine: { lineStyle: { color: c.grid, type: 'dashed' } },
    },
    visualMap: {
      min: range.min,
      max: range.max,
      dimension: 2, // 散点第三维（内区 α / 外区 θ）
      calculable: false,
      show: true,
      orient: 'vertical',
      right: 0,
      top: 'middle',
      itemWidth: 8,
      itemHeight: 70,
      text: [gradientLabel],
      textStyle: { color: c.muted, fontSize: 9 },
      inRange: { color: c.visualMapColors },
    },
    series: [
      {
        type: 'scatter',
        symbolSize: 6,
        data,
        // spec §27 第 5 条：大数据量下启用 large 模式优化渲染
        large: true,
        largeThreshold: 500,
        emphasis: { focus: 'series' },
      },
    ],
  }
}

// 2 / 3. 曲线图：内区 α-K0/Ks（按 β 分组）/ 外区 φ-K0[n]/Ks[n]（按 θ 分组）
// 通用构建函数：grouped 已是按分组维度（β 或 θ）聚合的数据，xName/yName/legendNameFormatter 由调用方传入
function buildCurveOption(
  grouped: Array<{ key: number; data: Array<[number, number]> }>,
  yName: string,
  legendNameFormatter: (key: number) => string,
  xAxisName: string,
  xAxisLabelPrefix: string,
): EChartsOption {
  const c = palette.value
  const grid = { left: 56, right: 16, top: 32, bottom: 28 }

  // 无数据时不再返回"暂无数据"占位，而是渲染完整空坐标轴 + 网格 + legend
  // （series 为空数组，xAxis/yAxis/legend 正常显示）
  // 这样切到外区 Tab 时操作员能看到完整的 φ-K0[n] / φ-Ks[n] 坐标系框架，等采集数据到达后曲线立即填入

  // 颜色循环：分组较多时按调色板循环，与 visualMap 配色同源保持视觉一致
  const colors = c.visualMapColors

  const series = grouped.map((group, i) => ({
    name: legendNameFormatter(group.key),
    type: 'line' as const,
    showSymbol: false,
    symbolSize: 4,
    data: group.data,
    lineStyle: { width: 1.5, color: colors[i % colors.length] },
    itemStyle: { color: colors[i % colors.length] },
    emphasis: { focus: 'series' as const, lineStyle: { width: 2.5 } },
    animation: false,
    // spec §27 第 5 条：大数据量下启用 large 模式优化渲染
    large: true,
    largeThreshold: 500,
  }))

  return {
    backgroundColor: 'transparent',
    animation: false,
    grid,
    tooltip: {
      trigger: 'axis',
      backgroundColor: c.tooltipBg,
      borderColor: c.tooltipBorder,
      borderWidth: 1,
      padding: [8, 12],
      textStyle: { color: c.text, fontSize: 12 },
      axisPointer: {
        type: 'line',
        lineStyle: { color: c.muted, opacity: 0.4, type: 'dashed' },
      },
      // formatter 参数为 axis 触发时的所有 series 数据数组
      formatter: (params: unknown) => {
        if (!Array.isArray(params) || params.length === 0) return ''
        const first = params[0] as { value?: unknown; axisValue?: unknown }
        const val = first.value
        // xAxis type='value' 时 val 是 [x, yValue] 元组
        const xVal = Array.isArray(val) ? val[0] : first.axisValue
        const xStr = typeof xVal === 'number' ? xVal.toFixed(1) : String(xVal ?? '')
        const rows = params.map((p: unknown) => {
          const item = p as { value?: unknown; seriesName?: string; marker?: string }
          const v = Array.isArray(item.value) ? item.value[1] : item.value
          const value = typeof v === 'number' && !Number.isNaN(v) ? v.toFixed(3) : '-'
          const marker = item.marker ?? ''
          const seriesName = item.seriesName ?? ''
          return `${marker}${seriesName}: <strong>${value}</strong>`
        })
        return `<div style="font-weight:600;margin-bottom:4px">${xAxisLabelPrefix}: ${xStr}°</div>${rows.join('<br/>')}`
      },
    },
    legend: {
      type: 'scroll',
      textStyle: { color: c.muted, fontSize: 9 },
      icon: 'roundRect',
      itemWidth: 8,
      itemHeight: 8,
      top: 0,
      right: 0,
    },
    xAxis: {
      type: 'value',
      name: xAxisName,
      nameLocation: 'middle',
      nameGap: 18,
      nameTextStyle: { color: c.muted, fontSize: 11 },
      axisLine: { lineStyle: { color: c.muted } },
      axisTick: { lineStyle: { color: c.muted } },
      axisLabel: { color: c.muted, fontSize: 10 },
      splitLine: { lineStyle: { color: c.grid, type: 'dashed' } },
    },
    yAxis: {
      type: 'value',
      name: yName,
      nameLocation: 'middle',
      nameGap: 38,
      nameTextStyle: { color: c.muted, fontSize: 11 },
      axisLine: { lineStyle: { color: c.muted } },
      axisTick: { lineStyle: { color: c.muted } },
      axisLabel: { color: c.muted, fontSize: 10 },
      splitLine: { lineStyle: { color: c.grid, type: 'dashed' } },
    },
    series,
  }
}

// 内区曲线分组适配：BetaGroup → 通用 { key, data } 形式
const innerK0Grouped = computed(() => k0GroupedData.value.map((g) => ({ key: g.beta, data: g.data })))
const innerKsGrouped = computed(() => ksGroupedData.value.map((g) => ({ key: g.beta, data: g.data })))
// 外区曲线分组适配：ThetaGroup → 通用 { key, data } 形式
const outerK0Grouped = computed(() => outerK0GroupedData.value.map((g) => ({ key: g.theta, data: g.data })))
const outerKsGrouped = computed(() => outerKsGroupedData.value.map((g) => ({ key: g.theta, data: g.data })))

// ===== 更新所有图表（根据 activeTab 切换内/外区数据源）=====
function updateCharts(): void {
  scatterChart.value?.setOption(buildScatterOption(), { notMerge: true })
  if (props.activeTab === 'inner') {
    k0CurveChart.value?.setOption(
      buildCurveOption(
        innerK0Grouped.value,
        t.value.shc_axisK0,
        (beta) => t.value.shc_legendBetaN.replace('{beta}', beta.toFixed(1)),
        t.value.shc_axisAlpha,
        'α',
      ),
      { notMerge: true },
    )
    ksCurveChart.value?.setOption(
      buildCurveOption(
        innerKsGrouped.value,
        t.value.shc_axisKs,
        (beta) => t.value.shc_legendBetaN.replace('{beta}', beta.toFixed(1)),
        t.value.shc_axisAlpha,
        'α',
      ),
      { notMerge: true },
    )
  } else {
    k0CurveChart.value?.setOption(
      buildCurveOption(
        outerK0Grouped.value,
        t.value.shc_axisK0Outer,
        (theta) => t.value.shc_legendThetaN.replace('{theta}', theta.toFixed(1)),
        t.value.shc_axisPhi,
        'φ',
      ),
      { notMerge: true },
    )
    ksCurveChart.value?.setOption(
      buildCurveOption(
        outerKsGrouped.value,
        t.value.shc_axisKsOuter,
        (theta) => t.value.shc_legendThetaN.replace('{theta}', theta.toFixed(1)),
        t.value.shc_axisPhi,
        'φ',
      ),
      { notMerge: true },
    )
  }
}

// 节流更新：dataPoints 变化或主题/语言切换时通过 rAF 合并到下一帧
// useThrottledChartUpdate 在 unmount 时自动取消挂起的 rAF（防内存泄漏 + 防 setOption 已 dispose 实例）
// 监听内/外区数据 + activeTab 切换 + palette/t，任意变化都触发节流重绘
useThrottledChartUpdate(
  [innerPoints, outerPoints, () => props.activeTab, palette, t],
  updateCharts,
  // 不立即触发：onMounted 路径会主动调 updateCharts 完成首次绘制
  { immediate: false },
)

// activeTab 切换：父组件 Tab 栏点击触发，下一帧 resize + 重绘
// resize 必要性：父组件 Tab 切换可能伴随布局变化（如未来加区域切换动画），提前 resize 防画面错位
watch(() => props.activeTab, () => {
  void nextTick(() => {
    scatterChart.value?.resize()
    k0CurveChart.value?.resize()
    ksCurveChart.value?.resize()
    updateCharts()
  })
})

// ===== 生命周期 =====
function handleResize(): void {
  scatterChart.value?.resize()
  k0CurveChart.value?.resize()
  ksCurveChart.value?.resize()
}

onMounted(() => {
  void nextTick(() => {
    // 初始化 ECharts 实例（容器需在 DOM 中且尺寸非零）
    if (scatterEl.value) scatterChart.value = init(scatterEl.value)
    if (k0CurveEl.value) k0CurveChart.value = init(k0CurveEl.value)
    if (ksCurveEl.value) ksCurveChart.value = init(ksCurveEl.value)

    // 首次绘制
    updateCharts()

    // 监听容器尺寸变化：父级 flex 变化（侧栏折叠 / 窗口 resize）时触发 resize
    // 三个图表共享同一父容器，监听一个即可触发三个一起 resize
    const observeTarget = scatterEl.value?.parentElement
    if (observeTarget && typeof ResizeObserver !== 'undefined') {
      resizeObserver = new ResizeObserver(() => {
        handleResize()
      })
      resizeObserver.observe(observeTarget)
    }
    window.addEventListener('resize', handleResize)
  })
})

onBeforeUnmount(() => {
  if (resizeObserver) {
    resizeObserver.disconnect()
    resizeObserver = null
  }
  window.removeEventListener('resize', handleResize)
  // 主动 dispose ECharts 实例：useThrottledChartUpdate 已取消挂起 rAF，
  // 但 ECharts 实例自身的内部定时器 / canvas 上下文需 dispose 才能彻底释放
  scatterChart.value?.dispose()
  k0CurveChart.value?.dispose()
  ksCurveChart.value?.dispose()
  scatterChart.value = null
  k0CurveChart.value = null
  ksCurveChart.value = null
})

// ===== 图表标题（根据 activeTab 动态切换内/外区文案）=====
// 7 个区域共用 3 个 ECharts 实例，仅标题与坐标轴名不同。
// 内区标题不带扇区编号；外区标题带当前扇区编号 n（spec §4.2/§4.3 系数命名约定）。
const scatterTitle = computed(() => {
  if (props.activeTab === 'inner') return t.value.shc_innerKaKbTitle
  const n = activeSector.value ?? 0
  return t.value.shc_outerKthetaKphiTitleN.replace('{n}', String(n))
})
const k0CurveTitle = computed(() => {
  if (props.activeTab === 'inner') return t.value.shc_innerAlphaK0Title
  const n = activeSector.value ?? 0
  return t.value.shc_outerPhiK0TitleN.replace('{n}', String(n))
})
const ksCurveTitle = computed(() => {
  if (props.activeTab === 'inner') return t.value.shc_innerAlphaKsTitle
  const n = activeSector.value ?? 0
  return t.value.shc_outerPhiKsTitleN.replace('{n}', String(n))
})
</script>

<template>
  <div data-test="seven-hole-charts" class="seven-hole-charts">
    <!-- 3 类图表布局（与原内区布局一致）：上半区主散点图（跨两列），下半区两条曲线并列
         activeTab 由父组件 SevenHoleMain.vue 的 Tab 栏控制，本组件仅渲染图表区域 -->
    <div class="seven-hole-charts__inner">
      <!-- 1. 散点图：内区 Kα-Kβ / 外区 Kθ-Kφ（主图，跨两列） -->
      <div class="chart-card chart-card--main">
        <div class="chart-card__title">
          <Activity class="chart-card__icon" />
          <span>{{ scatterTitle }}</span>
        </div>
        <div ref="scatterEl" class="chart-canvas"></div>
      </div>
      <!-- 2. K0 曲线：内区 α-K0 / 外区 φ-K0[n] -->
      <div class="chart-card chart-card--sub">
        <div class="chart-card__title">
          <Activity class="chart-card__icon" />
          <span>{{ k0CurveTitle }}</span>
        </div>
        <div ref="k0CurveEl" class="chart-canvas"></div>
      </div>
      <!-- 3. Ks 曲线：内区 α-Ks / 外区 φ-Ks[n] -->
      <div class="chart-card chart-card--sub">
        <div class="chart-card__title">
          <Activity class="chart-card__icon" />
          <span>{{ ksCurveTitle }}</span>
        </div>
        <div ref="ksCurveEl" class="chart-canvas"></div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.seven-hole-charts {
  display: flex;
  flex-direction: column;
  height: 100%;
  width: 100%;
  min-height: 0;
}

/* 图表网格：上半主图跨两列，下半两副图并列
   内/外区共用此布局（用户需求：图形布局都类似内区，7 个区域每个 3 类图表）
   行高比 1.2:1 确保主图比副图略大，视觉层次清晰 */
.seven-hole-charts__inner {
  flex: 1;
  display: grid;
  grid-template-columns: 1fr 1fr;
  grid-template-rows: 1.2fr 1fr;
  gap: 8px;
  padding: 8px;
  min-height: 0;
}

.chart-card--main {
  grid-column: 1 / -1;
}

/* 图表卡片：圆角面板 + 阴影 */
.chart-card {
  display: flex;
  flex-direction: column;
  background: var(--bg-panel);
  border: 1px solid var(--border-default);
  border-radius: 8px;
  padding: 6px 8px;
  box-shadow: var(--shadow-panel);
  min-height: 0;
  overflow: hidden;
}

.chart-card__title {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  font-weight: 600;
  color: var(--text-muted);
  flex-shrink: 0;
  margin-bottom: 2px;
}

.chart-card__icon {
  width: 14px;
  height: 14px;
  color: var(--accent-primary);
  flex-shrink: 0;
}

/* 图表画布：flex:1 填充剩余空间，min-height 160px 兜底保证可读性
   （spec Task 22：图表高度 160-180px，宽高比约 3:2） */
.chart-canvas {
  flex: 1;
  width: 100%;
  min-height: 160px;
}
</style>

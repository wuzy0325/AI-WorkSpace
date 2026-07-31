<script setup lang="ts">
import { ref, watch, onMounted, onUnmounted, nextTick, computed } from 'vue'
import { useI18nStore } from '@stores/i18nStore'
import type { TotalPressureDataPoint, TotalPressureCoefficients } from '@shared/types/calibration'

// i18n：Canvas 内的"暂无数据"等文本随语言切换重绘
const i18n = useI18nStore()
const t = computed(() => i18n.t)

// 图表 X 轴数据源：
// - 'alpha'：从 point.alpha 取攻角（总压探针的旋转角，单轴）
// - CPT/error/machNumber/velocity：从 point.coefficients 取对应系数（用于系数-系数散点图）
type ChartXKey = 'alpha' | keyof TotalPressureCoefficients

// Y 轴范围手动覆盖：用户在图表 Tab 工具条输入 min/max 后传入，
// 跳过基于数据点的自动 [min-10%, max+10%] 计算逻辑。
// min 必须 < max，否则视为无效回退到自动模式（draw 内做校验）。
interface YRangeOverride {
  min: number
  max: number
}

const props = defineProps<{
  dataPoints: TotalPressureDataPoint[]
  xKey: ChartXKey
  yKey: keyof TotalPressureCoefficients
  xLabel: string
  // Y 轴标签可选：概览页空间紧凑时传空字符串隐藏，图表 Tab 传完整标签
  yLabel?: string
  // X 轴刻度标签开关：概览页空间紧凑时传 false 隐藏，图表 Tab 传 true
  showXAxisLabels?: boolean
  // Y 轴刻度小数位数：默认 3，与原硬编码一致；用户可调高以适配小量程数据（如 error 量级 0.0001）
  yPrecision?: number
  // Y 轴范围手动覆盖：null 表示自动模式（基于数据点 + 10% padding）
  yRangeOverride?: YRangeOverride | null
}>()

const canvasRef = ref<HTMLCanvasElement | null>(null)
let resizeObserver: ResizeObserver | null = null
// 主题切换观察者：themeStore.setTheme 会修改 <html> 的 data-theme 属性和 dark class，
// 监听到变化时触发 Canvas 重绘，确保暗色/亮色主题下图表颜色同步切换。
let themeObserver: MutationObserver | null = null

// 图表颜色集合：所有颜色从 CSS 设计 token 实时解析，确保主题切换时 Canvas 同步重绘。
// 满足 §23 约束："所有 UI 元素颜色必须使用设计 token，Canvas 图表通过 getComputedStyle 读取 token"。
interface ChartColors {
  accent: string       // 主曲线 + 普通散点
  last: string         // 最新散点高亮色
  textMuted: string    // 刻度文字
  textPrimary: string  // 轴标题
  border: string       // 坐标轴线（用 border-strong 保证清晰）
  grid: string         // 网格线（用 border-default 弱于轴线）
  pointStroke: string  // 散点描边（用卡片背景色形成"内描边"效果，让散点突出于背景）
}

// readToken 从 <html> 读取 CSS 变量并 trim。
// getComputedStyle 对 var() 引用会递归解析为最终值，对直接 hex/rgb token 会返回字符串；
// 但对 color-mix() 表达式会原样返回（Canvas 不识别），因此遇到此类表达式时回退到 fallback。
// 这样设计可兼容 wind-daq 的 color.css 中 --accent-primary: var(--accent-primary-core) 这类间接定义。
function readToken(name: string, fallback: string): string {
  if (typeof document === 'undefined') return fallback
  const value = getComputedStyle(document.documentElement).getPropertyValue(name).trim()
  if (!value || value.startsWith('var(') || value.startsWith('color-mix(')) return fallback
  return value
}

// resolveChartColors 在每次 draw() 调用时读取当前主题颜色，避免缓存导致主题切换后颜色不更新。
// fallback 取 dark.css 中的值，确保即使 token 未定义（如 SSR 或样式未加载）也能在暗色背景上可见。
function resolveChartColors(): ChartColors {
  return {
    accent: readToken('--accent-primary', '#10b981'),
    last: readToken('--accent-success', '#22c55e'),
    textMuted: readToken('--text-muted', '#94a3b8'),
    textPrimary: readToken('--text-primary', '#e2e8f0'),
    border: readToken('--border-strong', '#475569'),
    grid: readToken('--border-default', '#334155'),
    pointStroke: readToken('--bg-panel', '#172338'),
  }
}

// Canvas font 最保险用系统通用字体名 sans-serif。
// 之前读 CSS 变量 / computed style 拿到多字体栈或带引号名字，导致 fillText 静默失败。
// 刻度文字可读性优先于字体美观一致性，直接硬编码 sans-serif。
const CANVAS_FONT = 'sans-serif'

// 从 dataPoints 提取 (x, y) 散点：xKey='alpha' 时取 point.alpha，否则取 coefficients
// 类型谓词 filter 确保 x/y 收窄为 number（TotalPressureCoefficients 含可选字段，索引访问返回 number | undefined）
function extractPoints(): { x: number; y: number }[] {
  return props.dataPoints
    .map((p) => {
      const x = props.xKey === 'alpha' ? p.alpha : p.coefficients[props.xKey as keyof TotalPressureCoefficients]
      const y = p.coefficients[props.yKey as keyof TotalPressureCoefficients]
      return { x, y }
    })
    .filter((p): p is { x: number; y: number } => typeof p.x === 'number' && isFinite(p.x) && typeof p.y === 'number' && isFinite(p.y))
}

function draw(): void {
  const canvas = canvasRef.value
  if (!canvas) return
  const ctx = canvas.getContext('2d')
  if (!ctx) return

  // 每次重绘时实时解析主题颜色：主题切换 MutationObserver 会触发 draw()，
  // 此处取到的就是新主题下的颜色，无需缓存失效逻辑。
  const colors = resolveChartColors()

  // 高 DPI 屏适配：canvas 实际像素 = CSS 像素 × dpr，再 scale 回 1:1 绘图
  const dpr = window.devicePixelRatio || 1
  const rect = canvas.getBoundingClientRect()
  if (rect.width === 0 || rect.height === 0) return
  canvas.width = rect.width * dpr
  canvas.height = rect.height * dpr
  ctx.setTransform(1, 0, 0, 1, 0, 0)
  ctx.scale(dpr, dpr)

  const width = rect.width
  const height = rect.height
  // X 轴刻度画在绘图区底部内侧（textBaseline=bottom），padBottom 只需小余量避免散点贴底
  const showXLabels = props.showXAxisLabels !== false
  const padLeft = 48
  const padRight = 12
  const padTop = 12
  const padBottom = showXLabels ? 22 : 8

  ctx.clearRect(0, 0, width, height)

  const points = extractPoints()

  if (points.length === 0) {
    ctx.fillStyle = colors.textMuted
    ctx.font = `13px ${CANVAS_FONT}`
    ctx.textAlign = 'center'
    ctx.textBaseline = 'middle'
    ctx.fillText(t.value.tp_noData, width / 2, height / 2)
    return
  }

  const xValues = points.map((p) => p.x)
  const yValues = points.map((p) => p.y)
  let xMin = Math.min(...xValues)
  let xMax = Math.max(...xValues)
  let yMin = Math.min(...yValues)
  let yMax = Math.max(...yValues)

  // 范围为零时扩展，避免除零
  if (xMin === xMax) { xMin -= 1; xMax += 1 }
  if (yMin === yMax) { yMin -= 1; yMax += 1 }

  // 留 10% 边距，散点不贴边
  const xPad = (xMax - xMin) * 0.1
  const yPad = (yMax - yMin) * 0.1
  xMin -= xPad; xMax += xPad
  // Y 轴范围手动覆盖：用户在图表 Tab 输入 min/max 后传入，
  // 直接采用用户值，跳过 10% padding（用户已自行决定边界）。
  // 无效输入（min >= max 或非有限数）静默回退到自动模式，避免 UI 报错打断操作。
  const override = props.yRangeOverride
  if (override && Number.isFinite(override.min) && Number.isFinite(override.max) && override.min < override.max) {
    yMin = override.min
    yMax = override.max
  } else {
    yMin -= yPad; yMax += yPad
  }

  const plotW = width - padLeft - padRight
  const plotH = height - padTop - padBottom

  const xScale = (x: number) => padLeft + ((x - xMin) / (xMax - xMin)) * plotW
  const yScale = (y: number) => padTop + plotH - ((y - yMin) / (yMax - yMin)) * plotH

  // Y 轴网格线（水平）
  ctx.strokeStyle = colors.grid
  ctx.lineWidth = 1
  const yTicks = 4
  for (let i = 0; i <= yTicks; i++) {
    const y = padTop + (plotH * i) / yTicks
    ctx.beginPath()
    ctx.moveTo(padLeft, y)
    ctx.lineTo(padLeft + plotW, y)
    ctx.stroke()
  }

  // 画坐标轴（左竖线 + 底横线）
  ctx.strokeStyle = colors.border
  ctx.lineWidth = 1.5
  ctx.beginPath()
  ctx.moveTo(padLeft, padTop)
  ctx.lineTo(padLeft, padTop + plotH)
  ctx.lineTo(padLeft + plotW, padTop + plotH)
  ctx.stroke()

  // X 轴刻度：每个数据点位置显示其 α 值，去重按升序排列
  if (showXLabels) {
    const tickValues = [...new Set(points.map((p) => p.x))].sort((a, b) => a - b)
    ctx.fillStyle = colors.textMuted
    ctx.font = `11px ${CANVAS_FONT}`
    ctx.textAlign = 'center'
    ctx.textBaseline = 'bottom'
    for (const val of tickValues) {
      const x = xScale(val)
      ctx.fillText(val.toFixed(1), x, padTop + plotH - 2)
    }
  }

  // Y 轴刻度标签
  // 精度可由父组件通过 yPrecision 配置（默认 3）：CPT 通常 3 位足够，
  // 但 error 系数量级可能到 0.0001，需要更高精度才能看出刻度差异。
  // 非法值（负数或非整数）回退到 3，避免 toFixed 抛错。
  const yPrecisionRaw = props.yPrecision
  const yPrecision = (typeof yPrecisionRaw === 'number' && Number.isFinite(yPrecisionRaw) && yPrecisionRaw >= 0 && Number.isInteger(yPrecisionRaw))
    ? yPrecisionRaw
    : 3
  ctx.fillStyle = colors.textMuted
  ctx.font = `11px ${CANVAS_FONT}`
  ctx.textAlign = 'right'
  ctx.textBaseline = 'middle'
  for (let i = 0; i <= yTicks; i++) {
    const val = yMin + ((yMax - yMin) * i) / yTicks
    const y = padTop + plotH - (plotH * i) / yTicks
    ctx.fillText(val.toFixed(yPrecision), padLeft - 6, y)
  }

  // Y 轴标题仅在传入非空字符串时绘制（图表 Tab 可选）
  if (props.yLabel) {
    ctx.fillStyle = colors.textPrimary
    ctx.font = `12px ${CANVAS_FONT}`
    ctx.save()
    ctx.translate(10, padTop + plotH / 2)
    ctx.rotate(-Math.PI / 2)
    ctx.textAlign = 'center'
    ctx.textBaseline = 'top'
    ctx.fillText(props.yLabel, 0, 0)
    ctx.restore()
  }

  // 按 x 升序排序后绘制折线，确保 α 递增时曲线连续不回环
  // 校准数据点本身就是按 α 顺序采集的，排序仅防御外部传入乱序的情况
  const sortedPoints = [...points].sort((a, b) => a.x - b.x)

  // 画折线：主题色细线，连接所有采样点，让趋势一目了然
  if (sortedPoints.length >= 2) {
    ctx.strokeStyle = colors.accent
    ctx.lineWidth = 1.5
    ctx.beginPath()
    sortedPoints.forEach((p, i) => {
      const x = xScale(p.x)
      const y = yScale(p.y)
      if (i === 0) ctx.moveTo(x, y)
      else ctx.lineTo(x, y)
    })
    ctx.stroke()
  }

  // 画散点：折线上叠加圆点，最后一个点用高亮色标识"最新采集位置"
  sortedPoints.forEach((point, index) => {
    const x = xScale(point.x)
    const y = yScale(point.y)
    ctx.beginPath()
    ctx.arc(x, y, 4, 0, Math.PI * 2)
    ctx.fillStyle = index === sortedPoints.length - 1 ? colors.last : colors.accent
    ctx.fill()
    ctx.strokeStyle = colors.pointStroke
    ctx.lineWidth = 1.5
    ctx.stroke()
  })
}

// dataPoints 变化时自动重绘（deep 监听数组内容变化）
watch(() => props.dataPoints, () => {
  void nextTick(draw)
}, { deep: true })

// Y 轴精度或手动范围变化时立即重绘：用户在工具条调整参数后需即时看到效果。
// yRangeOverride 是对象引用，deep 监听确保用户改 min 或 max 都能触发。
watch(() => props.yPrecision, () => {
  void nextTick(draw)
})
watch(() => props.yRangeOverride, () => {
  void nextTick(draw)
}, { deep: true })

// 语言切换时重绘：Canvas 内的"暂无数据"等文本依赖 i18n.t，需主动触发重绘
watch(t, () => {
  void nextTick(draw)
})

onMounted(() => {
  void nextTick(draw)
  // 监听容器尺寸变化：窗口 resize、侧栏折叠、父容器 flex 变化时触发重绘
  const canvas = canvasRef.value
  if (canvas?.parentElement) {
    resizeObserver = new ResizeObserver(() => {
      void nextTick(draw)
    })
    resizeObserver.observe(canvas.parentElement)
  }
  // 监听 <html> 主题属性变化：themeStore.setTheme 会同时修改 data-theme 和 dark class，
  // 任一变化都说明主题已切换，触发重绘让 Canvas 颜色同步更新。
  // 用 MutationObserver 而非直接 watch themeStore，是为了让组件不依赖具体 store 实例，
  // 将来复用到其他项目时只要该项目也通过 <html> 属性切换主题即可正常工作。
  themeObserver = new MutationObserver(() => {
    void nextTick(draw)
  })
  themeObserver.observe(document.documentElement, {
    attributes: true,
    attributeFilter: ['data-theme', 'class'],
  })
})

onUnmounted(() => {
  if (resizeObserver) {
    resizeObserver.disconnect()
    resizeObserver = null
  }
  if (themeObserver) {
    themeObserver.disconnect()
    themeObserver = null
  }
})

// 暴露 draw 方法：父组件可主动触发重绘（如切换 Tab 后 canvas 尺寸变化）
defineExpose({ draw })
</script>

<template>
  <canvas ref="canvasRef" class="h-full w-full"></canvas>
</template>

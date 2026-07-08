<script setup lang="ts">
import { ref, watch, onMounted, onUnmounted, nextTick } from 'vue'
import type { ThreeHoleDataPoint, ThreeHoleCoefficients } from '@shared/types/calibration'

// 图表 X 轴数据源：
// - 'theta'：从 point.coordinates['θ'] 取角度（三孔探针的旋转角）
// - Kb/Kt/Sb：从 point.coefficients 取对应系数（用于系数-系数散点图）
type ChartXKey = 'theta' | keyof ThreeHoleCoefficients

const props = defineProps<{
  dataPoints: ThreeHoleDataPoint[]
  xKey: ChartXKey
  yKey: keyof ThreeHoleCoefficients
  xLabel: string
  // Y 轴标签可选：概览页空间紧凑时传空字符串隐藏，图表 Tab 传完整标签
  yLabel?: string
  // X 轴刻度标签和轴标题开关：概览页空间紧凑时传 false 隐藏，图表 Tab 传 true
  showXAxisLabels?: boolean
}>()

const canvasRef = ref<HTMLCanvasElement | null>(null)
let resizeObserver: ResizeObserver | null = null

// Canvas 绘图直接用硬编码颜色，不读 CSS 变量。
// 之前 readColor('--text-muted') 拿到的是 "var(--gray-11)" 这种未解析引用，
// Canvas 不认 var() 语法 → fillStyle 静默无效 → 刻度文字不渲染。
// 改用临时元素读 computed color 可行，但主题切换时 canvas 不会自动重绘，
// 且实现复杂。硬编码颜色 + 主题感知 class 更简单可靠。
const COLOR_ACCENT = '#3b82f6'
const COLOR_LAST = '#10b981'
const COLOR_TEXT_MUTED = '#64748b'
const COLOR_TEXT_PRIMARY = '#1e293b'
const COLOR_BORDER = '#cbd5e1'
const COLOR_GRID = '#e2e8f0'
const COLOR_POINT_STROKE = '#ffffff'

// Canvas font 最保险用系统通用字体名 sans-serif。
// 之前读 CSS 变量 / computed style 拿到多字体栈或带引号名字，导致 fillText 静默失败。
// 刻度文字可读性优先于字体美观一致性，直接硬编码 sans-serif。
const CANVAS_FONT = 'sans-serif'

// 从 dataPoints 提取 (x, y) 散点：xKey='theta' 时取 coordinates['θ']，否则取 coefficients
function extractPoints(): { x: number; y: number }[] {
  return props.dataPoints
    .map((p) => {
      const x = props.xKey === 'theta' ? p.coordinates['θ'] : p.coefficients[props.xKey as keyof ThreeHoleCoefficients]
      const y = p.coefficients[props.yKey]
      return { x, y }
    })
    .filter((p) => typeof p.x === 'number' && isFinite(p.x) && typeof p.y === 'number' && isFinite(p.y))
}

function draw(): void {
  const canvas = canvasRef.value
  if (!canvas) return
  const ctx = canvas.getContext('2d')
  if (!ctx) return

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
  const padBottom = 8

  ctx.clearRect(0, 0, width, height)

  const points = extractPoints()

  if (points.length === 0) {
    ctx.fillStyle = COLOR_TEXT_MUTED
    ctx.font = `13px ${CANVAS_FONT}`
    ctx.textAlign = 'center'
    ctx.textBaseline = 'middle'
    ctx.fillText('暂无数据', width / 2, height / 2)
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
  yMin -= yPad; yMax += yPad

  const plotW = width - padLeft - padRight
  const plotH = height - padTop - padBottom

  const xScale = (x: number) => padLeft + ((x - xMin) / (xMax - xMin)) * plotW
  const yScale = (y: number) => padTop + plotH - ((y - yMin) / (yMax - yMin)) * plotH

  // Y 轴网格线（水平）
  ctx.strokeStyle = COLOR_GRID
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
  ctx.strokeStyle = COLOR_BORDER
  ctx.lineWidth = 1.5
  ctx.beginPath()
  ctx.moveTo(padLeft, padTop)
  ctx.lineTo(padLeft, padTop + plotH)
  ctx.lineTo(padLeft + plotW, padTop + plotH)
  ctx.stroke()

  // X 轴刻度：每个数据点位置显示其 θ 值，去重按升序排列
  {
    const tickValues = [...new Set(points.map((p) => p.x))].sort((a, b) => a - b)
    ctx.fillStyle = COLOR_TEXT_MUTED
    ctx.font = `11px ${CANVAS_FONT}`
    ctx.textAlign = 'center'
    ctx.textBaseline = 'bottom'
    for (const val of tickValues) {
      const x = xScale(val)
      ctx.fillText(val.toFixed(1), x, padTop + plotH - 2)
    }
  }

  // Y 轴刻度标签
  ctx.fillStyle = COLOR_TEXT_MUTED
  ctx.font = `11px ${CANVAS_FONT}`
  ctx.textAlign = 'right'
  ctx.textBaseline = 'middle'
  for (let i = 0; i <= yTicks; i++) {
    const val = yMin + ((yMax - yMin) * i) / yTicks
    const y = padTop + plotH - (plotH * i) / yTicks
    ctx.fillText(val.toFixed(3), padLeft - 6, y)
  }

  // 轴标题：X 轴标题省略（卡片 h3 已标明"Kb - θ 曲线"，且 canvas 底部空间不足以再画标题）
  // Y 轴标题仅在传入非空字符串时绘制（图表 Tab 可选）
  if (props.yLabel) {
    ctx.fillStyle = COLOR_TEXT_PRIMARY
    ctx.font = `12px ${CANVAS_FONT}`
    ctx.save()
    ctx.translate(10, padTop + plotH / 2)
    ctx.rotate(-Math.PI / 2)
    ctx.textAlign = 'center'
    ctx.textBaseline = 'top'
    ctx.fillText(props.yLabel, 0, 0)
    ctx.restore()
  }

  // 按_x 升序排序后绘制折线，确保 θ 递增时曲线连续不回环
  // 校准数据点本身就是按 θ 顺序采集的，排序仅防御外部传入乱序的情况
  const sortedPoints = [...points].sort((a, b) => a.x - b.x)

  // 画折线：主题色细线，连接所有采样点，让趋势一目了然
  if (sortedPoints.length >= 2) {
    ctx.strokeStyle = COLOR_ACCENT
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
    ctx.fillStyle = index === sortedPoints.length - 1 ? COLOR_LAST : COLOR_ACCENT
    ctx.fill()
    ctx.strokeStyle = COLOR_POINT_STROKE
    ctx.lineWidth = 1.5
    ctx.stroke()
  })
}

// dataPoints 变化时自动重绘（deep 监听数组内容变化）
watch(() => props.dataPoints, () => {
  void nextTick(draw)
}, { deep: true })

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
})

onUnmounted(() => {
  if (resizeObserver) {
    resizeObserver.disconnect()
    resizeObserver = null
  }
})

// 暴露 draw 方法：父组件可主动触发重绘（如切换 Tab 后 canvas 尺寸变化）
defineExpose({ draw })
</script>

<template>
  <canvas ref="canvasRef" class="h-full w-full"></canvas>
</template>

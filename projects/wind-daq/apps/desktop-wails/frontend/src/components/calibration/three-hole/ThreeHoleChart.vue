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

// 从 CSS 变量读取颜色，组件挂载后 document.documentElement 才有完整 token
// 提供 fallback 避免首次渲染时 getComputedStyle 返回空值导致绘图异常
function readColor(varName: string, fallback: string): string {
  if (typeof window === 'undefined') return fallback
  const style = getComputedStyle(document.documentElement)
  return style.getPropertyValue(varName).trim() || fallback
}

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
  // 概览页隐藏 X 轴标签时底部只留 8px，图表 Tab 显示标签时留 28px
  const showXLabels = props.showXAxisLabels !== false
  const padLeft = 44
  const padRight = 12
  const padTop = 12
  const padBottom = showXLabels ? 28 : 8

  ctx.clearRect(0, 0, width, height)

  const points = extractPoints()

  // 读取设计 token 颜色
  const accentColor = readColor('--accent-primary', '#3b82f6')
  const textMuted = readColor('--text-muted', '#64748b')
  const textPrimary = readColor('--text-primary', '#1e293b')
  const borderColor = readColor('--border-default', '#e2e8f0')
  const gridColor = readColor('--bg-panel-strong', '#f1f5f9')
	// 最新点高亮色：通过 CSS 变量读取，与"运动中"指示器保持视觉一致
	const lastPointColor = readColor('--accent-success', '#10b981')
	// Canvas 字体：跟随应用主题字体设定，避免与 UI 其他部分字体不一致
	const themeFont = readColor('--font-family-base', '') || getComputedStyle(document.documentElement).fontFamily || 'sans-serif'

  if (points.length === 0) {
    ctx.fillStyle = textMuted
    ctx.font = `13px ${themeFont}`
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

  // 画网格线（浅色，在数据点下方）
  ctx.strokeStyle = gridColor
  ctx.lineWidth = 1
  const xTicks = 5
  const yTicks = 4
  for (let i = 0; i <= xTicks; i++) {
    const x = padLeft + (plotW * i) / xTicks
    ctx.beginPath()
    ctx.moveTo(x, padTop)
    ctx.lineTo(x, padTop + plotH)
    ctx.stroke()
  }
  for (let i = 0; i <= yTicks; i++) {
    const y = padTop + (plotH * i) / yTicks
    ctx.beginPath()
    ctx.moveTo(padLeft, y)
    ctx.lineTo(padLeft + plotW, y)
    ctx.stroke()
  }

  // 画坐标轴（左竖线 + 底横线）
  ctx.strokeStyle = borderColor
  ctx.lineWidth = 1.5
  ctx.beginPath()
  ctx.moveTo(padLeft, padTop)
  ctx.lineTo(padLeft, padTop + plotH)
  ctx.lineTo(padLeft + plotW, padTop + plotH)
  ctx.stroke()

  // X 轴刻度标签：概览页隐藏时跳过（showXAxisLabels=false）
  if (showXLabels) {
    ctx.fillStyle = textMuted
    ctx.font = `11px ${themeFont}`
    ctx.textAlign = 'center'
    ctx.textBaseline = 'top'
    for (let i = 0; i <= xTicks; i++) {
      const val = xMin + ((xMax - xMin) * i) / xTicks
      const x = padLeft + (plotW * i) / xTicks
      ctx.fillText(val.toFixed(1), x, padTop + plotH + 4)
    }
  }

  // Y 轴刻度标签
  ctx.textAlign = 'right'
  ctx.textBaseline = 'middle'
  for (let i = 0; i <= yTicks; i++) {
    const val = yMin + ((yMax - yMin) * i) / yTicks
    const y = padTop + plotH - (plotH * i) / yTicks
    ctx.fillText(val.toFixed(3), padLeft - 6, y)
  }

  // 轴标签：X 轴标题仅在显示刻度标签时绘制，Y 轴标题仅在传入非空字符串时绘制
  ctx.fillStyle = textPrimary
  ctx.font = `12px ${themeFont}`
  if (showXLabels) {
    ctx.textAlign = 'center'
    ctx.textBaseline = 'bottom'
    ctx.fillText(props.xLabel, padLeft + plotW / 2, height - 2)
  }
  if (props.yLabel) {
    ctx.save()
    ctx.translate(10, padTop + plotH / 2)
    ctx.rotate(-Math.PI / 2)
    ctx.textAlign = 'center'
    ctx.textBaseline = 'top'
    ctx.fillText(props.yLabel, 0, 0)
    ctx.restore()
  }

  // 画散点：最后一个点用高亮色，其余用主题色
  points.forEach((point, index) => {
    const x = xScale(point.x)
    const y = yScale(point.y)
    ctx.beginPath()
    ctx.arc(x, y, 4, 0, Math.PI * 2)
    ctx.fillStyle = index === points.length - 1 ? lastPointColor : accentColor
    ctx.fill()
    ctx.strokeStyle = readColor('--bg-panel', '#ffffff')
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

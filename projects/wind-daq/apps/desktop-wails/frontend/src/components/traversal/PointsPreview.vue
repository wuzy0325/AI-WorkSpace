<script setup lang="ts">
import { ref, computed, watch, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { getTraversalLayoutPoints } from '@shared/types/traversal'
import type { TraversalLayout, TraversalPoint, TraversalPointPhase } from '@shared/types/traversal'
import { useThemeStore } from '@stores/themeStore'

const props = defineProps<{
  layout?: TraversalLayout
  currentPoint?: { alpha: number; beta: number }
  completedPoints?: number
  currentPointPhase?: TraversalPointPhase
  /** 父组件传入的可见性：当前 Tab 非 preview 时应暂停动画以节省资源 */
  visible?: boolean
}>()

const themeStore = useThemeStore()

const canvasRef = ref<HTMLCanvasElement | null>(null)
const containerRef = ref<HTMLElement | null>(null)

const points = computed<TraversalPoint[]>(() => getTraversalLayoutPoints(props.layout))

const bounds = computed(() => {
  if (points.value.length === 0) {
    return { minX: -100, maxX: 100, minY: -100, maxY: 100 }
  }

  let minX = Infinity, maxX = -Infinity, minY = Infinity, maxY = -Infinity
  for (const p of points.value) {
    minX = Math.min(minX, p.x)
    maxX = Math.max(maxX, p.x)
    minY = Math.min(minY, p.y)
    maxY = Math.max(maxY, p.y)
  }

  const paddingX = (maxX - minX) * 0.1 || 10
  const paddingY = (maxY - minY) * 0.1 || 10

  return {
    minX: minX - paddingX,
    maxX: maxX + paddingX,
    minY: minY - paddingY,
    maxY: maxY + paddingY
  }
})

interface ViewTransform {
  scale: number
  offsetX: number
  offsetY: number
  plotWidth: number
  plotHeight: number
}

function createViewTransform(width: number, height: number): ViewTransform {
  const spanX = bounds.value.maxX - bounds.value.minX || 1
  const spanY = bounds.value.maxY - bounds.value.minY || 1
  const dataAspect = spanX / spanY
  const containerAspect = width / height
  const maxAspectAdjustment = 1.35
  const displayAspect = Math.min(
    dataAspect * maxAspectAdjustment,
    Math.max(dataAspect / maxAspectAdjustment, containerAspect)
  )
  const uniformPlotWidth = displayAspect > containerAspect ? width : height * displayAspect
  const uniformPlotHeight = displayAspect > containerAspect ? width / displayAspect : height
  const hasSingleRow = points.value.length > 1 && points.value.every((point) => Math.abs(point.y - points.value[0].y) < 0.01)
  const shouldImproveLineReadability = hasSingleRow && uniformPlotHeight < height * 0.45
  const plotWidth = shouldImproveLineReadability ? width : uniformPlotWidth
  const plotHeight = shouldImproveLineReadability ? height * 0.56 : uniformPlotHeight
  const scale = plotWidth / spanX

  return {
    scale,
    plotWidth,
    plotHeight,
    offsetX: (width - plotWidth) / 2,
    offsetY: (height - plotHeight) / 2
  }
}

function transformX(x: number, transform: ViewTransform): number {
  const { minX, maxX } = bounds.value
  return transform.offsetX + ((x - minX) / (maxX - minX || 1)) * transform.plotWidth
}

function transformY(y: number, transform: ViewTransform): number {
  const { minY, maxY } = bounds.value
  return transform.offsetY + transform.plotHeight - ((y - minY) / (maxY - minY || 1)) * transform.plotHeight
}

// 主题颜色配置
const themeColors = computed(() => {
  const isDark = themeStore.theme === 'dark'
  return {
    // 背景色
    background: isDark ? '#0f172a' : '#f8fafc',
    // 网格线
    grid: isDark ? '#334155' : '#e2e8f0',
    // 坐标轴
    axis: isDark ? '#475569' : '#94a3b8',
    // 已完成的点
    point: '#8b5cf6',
    // 当前点
    currentPoint: '#ef4444',
    // 当前点描边
    currentPointStroke: isDark ? '#fca5a5' : '#fecaca',
    // 文字
    text: isDark ? '#94a3b8' : '#64748b'
  }
})

function draw() {
  const canvas = canvasRef.value
  if (!canvas) return

  const ctx = canvas.getContext('2d')
  if (!ctx) return

  const rect = containerRef.value?.getBoundingClientRect()
  if (!rect || rect.width === 0 || rect.height === 0) return

  // 避免无限小尺寸绘制
  if (rect.width < 10 || rect.height < 10) return

  const dpr = window.devicePixelRatio || 1
  canvas.width = rect.width * dpr
  canvas.height = rect.height * dpr
  canvas.style.width = `${rect.width}px`
  canvas.style.height = `${rect.height}px`
  ctx.scale(dpr, dpr)

  const width = rect.width
  const height = rect.height
  const viewTransform = createViewTransform(width, height)

  const colors = themeColors.value

  // 背景
  ctx.fillStyle = colors.background
  ctx.fillRect(0, 0, width, height)

  // 网格线
  ctx.strokeStyle = colors.grid
  ctx.lineWidth = 1

  const gridSpacing = Math.max(
    (bounds.value.maxX - bounds.value.minX) / 10,
    (bounds.value.maxY - bounds.value.minY) / 10
  )

  ctx.beginPath()
  for (let x = Math.ceil(bounds.value.minX / gridSpacing) * gridSpacing; x <= bounds.value.maxX; x += gridSpacing) {
    const screenX = transformX(x, viewTransform)
    ctx.moveTo(screenX, viewTransform.offsetY)
    ctx.lineTo(screenX, viewTransform.offsetY + viewTransform.plotHeight)
  }
  for (let y = Math.ceil(bounds.value.minY / gridSpacing) * gridSpacing; y <= bounds.value.maxY; y += gridSpacing) {
    const screenY = transformY(y, viewTransform)
    ctx.moveTo(viewTransform.offsetX, screenY)
    ctx.lineTo(viewTransform.offsetX + viewTransform.plotWidth, screenY)
  }
  ctx.stroke()

  // 坐标轴
  ctx.strokeStyle = colors.axis
  ctx.lineWidth = 1.5
  ctx.beginPath()
  const centerX = transformX(0, viewTransform)
  ctx.moveTo(centerX, viewTransform.offsetY)
  ctx.lineTo(centerX, viewTransform.offsetY + viewTransform.plotHeight)
  const centerY = transformY(0, viewTransform)
  ctx.moveTo(viewTransform.offsetX, centerY)
  ctx.lineTo(viewTransform.offsetX + viewTransform.plotWidth, centerY)
  ctx.stroke()

  // 点位
  const totalPoints = points.value.length
  const completedCount = props.completedPoints ?? 0

  for (let i = 0; i < points.value.length; i++) {
    const point = points.value[i]
    const screenX = transformX(point.x, viewTransform)
    const screenY = transformY(point.y, viewTransform)

    // 使用索引匹配当前点，避免坐标容差匹配导致的错位问题
    // 后端 CurrentPoint 既是已完成点数，也是当前正在处理的点的索引
    const isCurrentPoint = i === completedCount && props.currentPointPhase !== undefined

    const isCompleted = i < completedCount
    const isPending = i >= completedCount && !isCurrentPoint

    // 如果测试已完成，当前点也应该显示为已完成状态
    const shouldShowAsCurrent = isCurrentPoint && !isCompleted && props.currentPointPhase

    ctx.beginPath()
    const radius = shouldShowAsCurrent ? 8 : 4
    ctx.arc(screenX, screenY, radius, 0, Math.PI * 2)

    if (shouldShowAsCurrent) {
      // 当前正在处理的点，根据阶段显示不同颜色
      let color: string
      let glowColor: string

      if (props.currentPointPhase === 'moving') {
        // 移动中：蓝色
        color = '#3b82f6'
        glowColor = '#3b82f6'
      } else if (props.currentPointPhase === 'stabilizing') {
        // 等待稳定：黄色
        color = '#fbbf24'
        glowColor = '#fbbf24'
      } else if (props.currentPointPhase === 'acquiring' || props.currentPointPhase === 'saving') {
        // 采集中 / 保存中：绿色
        // saving 是采集完成后的数据保存阶段，语义上属于采集的延伸，沿用绿色以与图例保持一致
        color = '#10b981'
        glowColor = '#10b981'
      } else {
        // 兜底默认：绿色（避免出现图例外的橙色）
        color = '#10b981'
        glowColor = '#10b981'
      }

      // 应用闪烁效果
      const blinkOpacity = blinkState.value.opacity
      const blinkRadius = blinkState.value.radiusOffset

      ctx.fillStyle = color
      ctx.globalAlpha = blinkOpacity
      ctx.fill()

      // 发光效果随闪烁变化
      ctx.shadowColor = glowColor
      ctx.shadowBlur = 8 + blinkRadius * 2
      ctx.fill()
      ctx.shadowBlur = 0
      ctx.globalAlpha = 1

      // 绘制闪烁的外圈
      ctx.strokeStyle = glowColor
      ctx.lineWidth = 2
      ctx.globalAlpha = blinkOpacity * 0.6
      ctx.beginPath()
      ctx.arc(screenX, screenY, radius + 4 + blinkRadius, 0, Math.PI * 2)
      ctx.stroke()
      ctx.globalAlpha = 1

      // 绘制第二个外圈（更淡）
      ctx.strokeStyle = glowColor
      ctx.lineWidth = 1
      ctx.globalAlpha = blinkOpacity * 0.3
      ctx.beginPath()
      ctx.arc(screenX, screenY, radius + 8 + blinkRadius, 0, Math.PI * 2)
      ctx.stroke()
      ctx.globalAlpha = 1
    } else if (isCompleted) {
      // 已完成的点：统一使用稳定的紫色，避免渐变色误导数据趋势
      ctx.fillStyle = colors.point
      ctx.fill()
    } else {
      // 未完成的点：半透明灰色
      ctx.fillStyle = 'rgba(148, 163, 184, 0.3)'
      ctx.fill()
    }
  }

  // 文字
  ctx.fillStyle = colors.text
  ctx.font = '10px sans-serif'
  ctx.fillText(`X: ${bounds.value.minX.toFixed(0)} ~ ${bounds.value.maxX.toFixed(0)}`, 5, height - 5)
  ctx.fillText(`Y: ${bounds.value.minY.toFixed(0)} ~ ${bounds.value.maxY.toFixed(0)}`, width - 100, height - 5)
}

// 闪烁动画状态
const blinkState = ref({
  opacity: 1,
  glowing: false,
  growing: true,
  radiusOffset: 0
})

let blinkAnimationId: number | null = null
let lastBlinkTime = 0
const BLINK_DURATION = 600 // 闪烁周期（毫秒）

function startBlinkAnimation(): void {
  if (blinkAnimationId !== null) return

  function animate(timestamp: number): void {
    if (!lastBlinkTime) lastBlinkTime = timestamp
    const elapsed = timestamp - lastBlinkTime

    // 计算闪烁进度：0-1
    const progress = (elapsed % BLINK_DURATION) / BLINK_DURATION

    // 使用正弦波创建平滑的闪烁效果
    const sineValue = Math.sin(progress * Math.PI * 2)

    // 透明度在 0.4-1.0 之间变化
    blinkState.value.opacity = 0.4 + (sineValue + 1) * 0.3

    // 半径偏移在 0-3 之间变化
    blinkState.value.radiusOffset = (sineValue + 1) * 1.5

    // 发光强度变化
    blinkState.value.glowing = sineValue > 0

    draw()

    blinkAnimationId = requestAnimationFrame(animate)
  }

  blinkAnimationId = requestAnimationFrame(animate)
}

function stopBlinkAnimation(): void {
  if (blinkAnimationId !== null) {
    cancelAnimationFrame(blinkAnimationId)
    blinkAnimationId = null
    lastBlinkTime = 0
    blinkState.value.opacity = 1
    blinkState.value.glowing = false
    blinkState.value.radiusOffset = 0
  }
}

// 监听是否需要闪烁：可见且当前点正在处理（有阶段状态）
watch(() => props.visible !== false && props.currentPointPhase !== undefined, (shouldBlink) => {
  if (shouldBlink) {
    startBlinkAnimation()
  } else {
    stopBlinkAnimation()
  }
}, { immediate: true })

watch([() => props.layout, () => props.currentPoint, () => props.completedPoints, () => props.currentPointPhase, () => themeStore.theme], () => {
  nextTick(draw)
}, { deep: true })

let resizeObserver: ResizeObserver | null = null

onMounted(() => {
  // 使用 ResizeObserver 监听容器尺寸变化
  if (containerRef.value && typeof ResizeObserver !== 'undefined') {
    resizeObserver = new ResizeObserver(() => {
      nextTick(draw)
    })
    resizeObserver.observe(containerRef.value)
  }
  window.addEventListener('resize', draw)

  // 延迟执行绘制，确保容器已正确渲染
  setTimeout(() => {
    nextTick(draw)
  }, 100)

  // 启动闪烁动画（如果有当前点）
  if (props.currentPoint && props.currentPointPhase) {
    startBlinkAnimation()
  }
})

onBeforeUnmount(() => {
  stopBlinkAnimation()
  if (resizeObserver && containerRef.value) {
    resizeObserver.unobserve(containerRef.value)
    resizeObserver.disconnect()
  }
  window.removeEventListener('resize', draw)
})
</script>

<template>
  <div ref="containerRef" class="w-full h-full relative">
    <canvas ref="canvasRef" class="w-full h-full"></canvas>
  </div>
</template>


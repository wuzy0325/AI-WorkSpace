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

// 涓婚棰滆壊閰嶇疆
const themeColors = computed(() => {
  const isDark = themeStore.theme === 'dark'
  return {
    // 鑳屾櫙鑹?
    background: isDark ? '#0f172a' : '#f8fafc',
    // 缃戞牸绾?
    grid: isDark ? '#334155' : '#e2e8f0',
    // 鍧愭爣杞?
    axis: isDark ? '#475569' : '#94a3b8',
    // 鏅€氱偣
    point: isDark ? '#3b82f6' : '#2563eb',
    // 褰撳墠鐐?
    currentPoint: '#ef4444',
    // 褰撳墠鐐规弿杈?
    currentPointStroke: isDark ? '#fca5a5' : '#fecaca',
    // 鏂囧瓧
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

  // 閬垮厤鏃犻檺灏忓昂瀵哥粯鍒?
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

  // 鑳屾櫙
  ctx.fillStyle = colors.background
  ctx.fillRect(0, 0, width, height)

  // 缃戞牸绾?
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

  // 鍧愭爣杞?
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

  // 鐐逛綅
  const totalPoints = points.value.length
  const completedCount = props.completedPoints ?? 0

  for (let i = 0; i < points.value.length; i++) {
    const point = points.value[i]
    const screenX = transformX(point.x, viewTransform)
    const screenY = transformY(point.y, viewTransform)

    const isCurrentPoint = props.currentPoint &&
      Math.abs(props.currentPoint.alpha - point.x) < 0.01 &&
      Math.abs(props.currentPoint.beta - point.y) < 0.01

    const isCompleted = i < completedCount
    const isPending = i >= completedCount && !isCurrentPoint

    // 濡傛灉娴嬭瘯宸插畬鎴愶紝褰撳墠鐐逛篃搴旇鏄剧ず涓哄凡瀹屾垚鐘舵€?
    const shouldShowAsCurrent = isCurrentPoint && !isCompleted && props.currentPointPhase

    ctx.beginPath()
    const radius = shouldShowAsCurrent ? 8 : 4
    ctx.arc(screenX, screenY, radius, 0, Math.PI * 2)

    if (shouldShowAsCurrent) {
      // 褰撳墠姝ｅ湪澶勭悊鐨勭偣锛屾牴鎹樁娈垫樉绀轰笉鍚岄鑹?
      let color: string
      let glowColor: string

      if (props.currentPointPhase === 'moving') {
        // 绉诲姩涓細钃濊壊
        color = '#3b82f6'
        glowColor = '#3b82f6'
      } else if (props.currentPointPhase === 'stabilizing') {
        // 绛夊緟绋冲畾锛氶粍鑹?
        color = '#fbbf24'
        glowColor = '#fbbf24'
      } else if (props.currentPointPhase === 'acquiring') {
        // 閲囬泦涓細缁胯壊
        color = '#10b981'
        glowColor = '#10b981'
      } else {
        // 榛樿锛氭鑹?
        color = '#f97316'
        glowColor = '#f97316'
      }

      // 搴旂敤闂儊鏁堟灉
      const blinkOpacity = blinkState.value.opacity
      const blinkRadius = blinkState.value.radiusOffset

      ctx.fillStyle = color
      ctx.globalAlpha = blinkOpacity
      ctx.fill()

      // 鍙戝厜鏁堟灉闅忛棯鐑佸彉鍖?
      ctx.shadowColor = glowColor
      ctx.shadowBlur = 8 + blinkRadius * 2
      ctx.fill()
      ctx.shadowBlur = 0
      ctx.globalAlpha = 1

      // 缁樺埗闂儊鐨勫鍦?
      ctx.strokeStyle = glowColor
      ctx.lineWidth = 2
      ctx.globalAlpha = blinkOpacity * 0.6
      ctx.beginPath()
      ctx.arc(screenX, screenY, radius + 4 + blinkRadius, 0, Math.PI * 2)
      ctx.stroke()
      ctx.globalAlpha = 1

      // 缁樺埗绗簩涓鍦堬紙鏇存贰锛?
      ctx.strokeStyle = glowColor
      ctx.lineWidth = 1
      ctx.globalAlpha = blinkOpacity * 0.3
      ctx.beginPath()
      ctx.arc(screenX, screenY, radius + 8 + blinkRadius, 0, Math.PI * 2)
      ctx.stroke()
      ctx.globalAlpha = 1
    } else if (isCompleted) {
      // 宸插畬鎴愮殑鐐癸細鏍规嵁杩涘害浠庣传鑹叉笎鍙樺埌绮夎壊
      const progress = totalPoints > 1 ? i / (totalPoints - 1) : 0
      // 浠庣传鑹?#a855f7 娓愬彉鍒扮矇鑹?#ec4899
      const r = Math.round(168 + (236 - 168) * progress)
      const g = Math.round(85 + (72 - 85) * progress)
      const b = Math.round(247 + (153 - 247) * progress)
      ctx.fillStyle = `rgb(${r}, ${g}, ${b})`
      ctx.fill()
    } else {
      // 鏈畬鎴愮殑鐐癸細鍗婇€忔槑鐏拌壊
      ctx.fillStyle = 'rgba(148, 163, 184, 0.3)'
      ctx.fill()
    }
  }

  // 鏂囧瓧
  ctx.fillStyle = colors.text
  ctx.font = '10px sans-serif'
  ctx.fillText(`X: ${bounds.value.minX.toFixed(0)} ~ ${bounds.value.maxX.toFixed(0)}`, 5, height - 5)
  ctx.fillText(`Y: ${bounds.value.minY.toFixed(0)} ~ ${bounds.value.maxY.toFixed(0)}`, width - 100, height - 5)
}

// 闂儊鍔ㄧ敾鐘舵€?
const blinkState = ref({
  opacity: 1,
  glowing: false,
  growing: true,
  radiusOffset: 0
})

let blinkAnimationId: number | null = null
let lastBlinkTime = 0
const BLINK_DURATION = 600 // 闂儊鍛ㄦ湡锛堟绉掞級

function startBlinkAnimation(): void {
  if (blinkAnimationId !== null) return

  function animate(timestamp: number): void {
    if (!lastBlinkTime) lastBlinkTime = timestamp
    const elapsed = timestamp - lastBlinkTime

    // 璁＄畻闂儊杩涘害锛?-1锛?
    const progress = (elapsed % BLINK_DURATION) / BLINK_DURATION

    // 浣跨敤姝ｅ鸡娉㈠垱寤哄钩婊戠殑闂儊鏁堟灉
    const sineValue = Math.sin(progress * Math.PI * 2)

    // 閫忔槑搴﹀湪 0.4-1.0 涔嬮棿鍙樺寲
    blinkState.value.opacity = 0.4 + (sineValue + 1) * 0.3

    // 鍗婂緞鍋忕Щ鍦?0-3 涔嬮棿鍙樺寲
    blinkState.value.radiusOffset = (sineValue + 1) * 1.5

    // 鍙戝厜寮哄害鍙樺寲
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

// 鐩戝惉鏄惁闇€瑕侀棯鐑侊紙鏈夊綋鍓嶇偣涓旀鍦ㄨ繍琛岋級
watch(() => props.currentPoint && props.currentPointPhase, (hasCurrentPoint) => {
  if (hasCurrentPoint) {
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
  // 浣跨敤 ResizeObserver 鐩戝惉瀹瑰櫒灏哄鍙樺寲
  if (containerRef.value && typeof ResizeObserver !== 'undefined') {
    resizeObserver = new ResizeObserver(() => {
      nextTick(draw)
    })
    resizeObserver.observe(containerRef.value)
  }
  window.addEventListener('resize', draw)

  // 寤惰繜鎵ц缁樺埗锛岀‘淇濆鍣ㄥ凡姝ｇ‘娓叉煋
  setTimeout(() => {
    nextTick(draw)
  }, 100)

  // 鍚姩闂儊鍔ㄧ敾锛堝鏋滄湁褰撳墠鐐癸級
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


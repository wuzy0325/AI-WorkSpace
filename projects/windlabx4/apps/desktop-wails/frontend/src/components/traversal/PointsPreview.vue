<script setup lang="ts">
import { ref, computed, watch, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { getTraversalLayoutPoints } from '@shared/types/traversal'
import type { TraversalLayout, TraversalPoint, TraversalPointPhase, TraversalCoordPoint } from '@shared/types/traversal'
import { useThemeStore } from '@stores/themeStore'
import { useI18nStore } from '@stores/i18nStore'
import UiSelect from '@components/ui/UiSelect.vue'

/** 画布支持的 4 轴。AXIS_KEYS 用于下拉选项构造和 string → key 类型守卫。 */
type AxisKey = 'x' | 'y' | 'z' | 'u'
const AXIS_KEYS: readonly AxisKey[] = ['x', 'y', 'z', 'u'] as const

const props = defineProps<{
  layout?: TraversalLayout
  // alpha/beta 允许 null：line 模式 Y 轴 NaN 序列化为 null
  currentPoint?: TraversalCoordPoint
  completedPoints?: number
  currentPointPhase?: TraversalPointPhase
  /** 父组件传入的可见性：当前 Tab 非 preview 时应暂停动画以节省资源 */
  visible?: boolean
}>()

// 横/纵轴选择：父组件持有状态，子组件通过 v-model 双向同步。
// 默认 X-Y 与历史行为一致；切换其他轴对时画布按新轴对数据重绘。
const hAxis = defineModel<AxisKey>('hAxis', { default: 'x' })
const vAxis = defineModel<AxisKey>('vAxis', { default: 'y' })

const themeStore = useThemeStore()
const i18n = useI18nStore()
const t = computed(() => i18n.t)

const canvasRef = ref<HTMLCanvasElement | null>(null)
const containerRef = ref<HTMLElement | null>(null)

const points = computed<TraversalPoint[]>(() => getTraversalLayoutPoints(props.layout))

// 统一通过 key 索引读取轴坐标值，避免硬编码 x/y/z/u。
// 与 plan §Task 7 改造点 #6 (点绘制) 配合：transformX(point[hAxis]) / transformY(point[vAxis])。
function coord(p: TraversalPoint, axis: AxisKey): number {
  return p[axis]
}

// bounds 切换为 h/v 语义字段：原 minX/maxX/minY/maxY 改为 minH/maxH/minV/maxV，
// 这样 transformX/transformY 与 draw 网格/十字线/底部标签全链路同语义。
const bounds = computed(() => {
  if (points.value.length === 0) {
    return { minH: -100, maxH: 100, minV: -100, maxV: 100 }
  }

  let minH = Infinity, maxH = -Infinity, minV = Infinity, maxV = -Infinity
  for (const p of points.value) {
    const h = coord(p, hAxis.value)
    const v = coord(p, vAxis.value)
    minH = Math.min(minH, h)
    maxH = Math.max(maxH, h)
    minV = Math.min(minV, v)
    maxV = Math.max(maxV, v)
  }

  const paddingH = (maxH - minH) * 0.1 || 10
  const paddingV = (maxV - minV) * 0.1 || 10

  return {
    minH: minH - paddingH,
    maxH: maxH + paddingH,
    minV: minV - paddingV,
    maxV: maxV + paddingV
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
  const spanH = bounds.value.maxH - bounds.value.minH || 1
  const spanV = bounds.value.maxV - bounds.value.minV || 1
  const dataAspect = spanH / spanV
  const containerAspect = width / height
  const maxAspectAdjustment = 1.35
  const displayAspect = Math.min(
    dataAspect * maxAspectAdjustment,
    Math.max(dataAspect / maxAspectAdjustment, containerAspect)
  )
  const uniformPlotWidth = displayAspect > containerAspect ? width : height * displayAspect
  const uniformPlotHeight = displayAspect > containerAspect ? width / displayAspect : height
  // 单行检测：横纵轴选定后，所有点的纵轴坐标是否几乎相同。
  // 切换到其他轴对时本判定仍基于 vAxis.value，自动适配（如 X-Z 轴对在 line 模式下 Z 全 0，会被识别为单行）。
  const hasSingleRow = points.value.length > 1 && points.value.every((point) => Math.abs(coord(point, vAxis.value) - coord(points.value[0], vAxis.value)) < 0.01)
  const shouldImproveLineReadability = hasSingleRow && uniformPlotHeight < height * 0.45
  const plotWidth = shouldImproveLineReadability ? width : uniformPlotWidth
  const plotHeight = shouldImproveLineReadability ? height * 0.56 : uniformPlotHeight
  const scale = plotWidth / spanH

  return {
    scale,
    plotWidth,
    plotHeight,
    offsetX: (width - plotWidth) / 2,
    offsetY: (height - plotHeight) / 2
  }
}

// transformX/transformY 改名为 transformH/transformV，内部读取 bounds.value.minH/maxH/minV/maxV，
// 与 bounds 字段语义保持一致；外部调用点同步更新。
function transformH(h: number, transform: ViewTransform): number {
  const { minH, maxH } = bounds.value
  return transform.offsetX + ((h - minH) / (maxH - minH || 1)) * transform.plotWidth
}

function transformV(v: number, transform: ViewTransform): number {
  const { minV, maxV } = bounds.value
  return transform.offsetY + transform.plotHeight - ((v - minV) / (maxV - minV || 1)) * transform.plotHeight
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

// 横轴选项排除当前纵轴值，纵轴选项排除当前横轴值（互斥校验，避免塌缩成一条线）。
// 选项 label 直接显示大写轴名（X/Y/Z/U），简洁直观。
const hAxisOptions = computed(() =>
  AXIS_KEYS
    .filter(k => k !== vAxis.value)
    .map(k => ({ value: k, label: k.toUpperCase() }))
)
const vAxisOptions = computed(() =>
  AXIS_KEYS
    .filter(k => k !== hAxis.value)
    .map(k => ({ value: k, label: k.toUpperCase() }))
)

// UiSelect 的 modelValue 是 string，需要类型守卫转换回 AxisKey 联合类型。
function isAxisKey(v: string): v is AxisKey {
  return v === 'x' || v === 'y' || v === 'z' || v === 'u'
}
function onHAxisChange(v: string) {
  if (isAxisKey(v)) hAxis.value = v
}
function onVAxisChange(v: string) {
  if (isAxisKey(v)) vAxis.value = v
}

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
  // 不设置 canvas.style.width/height：让 CSS w-full h-full 控制显示尺寸。
  // 原实现把 style.width/height 设为固定 px，会反向撑大 containerRef
  // （当 containerRef 的 h-full 在某些 flex 布局下不严格生效时），
  // 进而在双探针模式下撑大 .dual-row__points，导致窗口缩小后画布不缩小。
  // backing store 由 canvas.width/height 属性控制，与 CSS 显示尺寸独立，
  // 配合 ctx.scale(dpr, dpr) 在高 DPI 屏幕下保持清晰渲染。
  ctx.scale(dpr, dpr)

  const width = rect.width
  const height = rect.height
  const viewTransform = createViewTransform(width, height)

  const colors = themeColors.value

  // 背景
  ctx.fillStyle = colors.background
  ctx.fillRect(0, 0, width, height)

  // 网格线：横纵方向均基于 h/v 边界计算，与原 X/Y 网格行为对齐
  ctx.strokeStyle = colors.grid
  ctx.lineWidth = 1

  const gridSpacing = Math.max(
    (bounds.value.maxH - bounds.value.minH) / 10,
    (bounds.value.maxV - bounds.value.minV) / 10
  )

  ctx.beginPath()
  for (let h = Math.ceil(bounds.value.minH / gridSpacing) * gridSpacing; h <= bounds.value.maxH; h += gridSpacing) {
    const screenX = transformH(h, viewTransform)
    ctx.moveTo(screenX, viewTransform.offsetY)
    ctx.lineTo(screenX, viewTransform.offsetY + viewTransform.plotHeight)
  }
  for (let v = Math.ceil(bounds.value.minV / gridSpacing) * gridSpacing; v <= bounds.value.maxV; v += gridSpacing) {
    const screenY = transformV(v, viewTransform)
    ctx.moveTo(viewTransform.offsetX, screenY)
    ctx.lineTo(viewTransform.offsetX + viewTransform.plotWidth, screenY)
  }
  ctx.stroke()

  // 坐标轴十字线（0 点对齐）：底层 transform 已切换，逻辑保持不变
  ctx.strokeStyle = colors.axis
  ctx.lineWidth = 1.5
  ctx.beginPath()
  const centerX = transformH(0, viewTransform)
  ctx.moveTo(centerX, viewTransform.offsetY)
  ctx.lineTo(centerX, viewTransform.offsetY + viewTransform.plotHeight)
  const centerY = transformV(0, viewTransform)
  ctx.moveTo(viewTransform.offsetX, centerY)
  ctx.lineTo(viewTransform.offsetX + viewTransform.plotWidth, centerY)
  ctx.stroke()

  // 点位：横纵坐标读取从硬编码 point.x/point.y 切换为 point[hAxis]/point[vAxis]
  const completedCount = props.completedPoints ?? 0

  for (let i = 0; i < points.value.length; i++) {
    const point = points.value[i]
    const screenX = transformH(coord(point, hAxis.value), viewTransform)
    const screenY = transformV(coord(point, vAxis.value), viewTransform)

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

  // 底部标签：显示选中轴名 + 范围，替代原硬编码 X:/Y:
  ctx.fillStyle = colors.text
  ctx.font = '10px sans-serif'
  const hLabel = `${hAxis.value.toUpperCase()}: ${bounds.value.minH.toFixed(0)} ~ ${bounds.value.maxH.toFixed(0)}`
  const vLabel = `${vAxis.value.toUpperCase()}: ${bounds.value.minV.toFixed(0)} ~ ${bounds.value.maxV.toFixed(0)}`
  ctx.fillText(hLabel, 5, height - 5)
  ctx.fillText(vLabel, width - 100, height - 5)
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

// I-26 修复：原 watch 对 props.layout 用 deep:true，但 layout 含数百点位时深比较开销大。
// draw() 只读取 points（由 layout 派生）与 hAxis/vAxis/bounds，不依赖 currentPoint 字段细节；
// 改为浅监听 + 显式列出真正影响绘制的依赖（completedPoints/currentPointPhase 是标量）。
// layout 引用变化（父组件重新构造对象）即可触发重绘，无需深比较内部点位数组。
watch(
  [() => props.layout, () => props.completedPoints, () => props.currentPointPhase, () => themeStore.theme, hAxis, vAxis],
  () => { nextTick(draw) },
)

let resizeObserver: ResizeObserver | null = null
// I-25 修复：保存 setTimeout 句柄，组件卸载时清理，避免回调在卸载后仍触发 draw。
let initialDrawTimer: ReturnType<typeof setTimeout> | null = null

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
  initialDrawTimer = setTimeout(() => {
    initialDrawTimer = null
    nextTick(draw)
  }, 100)

  // 启动闪烁动画（如果有当前点）
  if (props.currentPoint && props.currentPointPhase) {
    startBlinkAnimation()
  }
})

onBeforeUnmount(() => {
  stopBlinkAnimation()
  // I-25 修复：清理未触发的初始绘制 timer，防止卸载后回调访问已失效的 canvasRef。
  if (initialDrawTimer !== null) {
    clearTimeout(initialDrawTimer)
    initialDrawTimer = null
  }
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
    <!-- 横/纵轴选择器：仅自定义布点（custom）涉及 Z/U 轴，其余模式（line/rectangle/sector）只生成 X/Y，切换轴对无意义故隐藏 -->
    <div v-if="layout?.pattern === 'custom'" class="axis-selector">
      <UiSelect
        :model-value="hAxis"
        :options="hAxisOptions"
        size="sm"
        :aria-label="t.travHAxis || 'Horizontal axis'"
        @update:model-value="onHAxisChange"
      />
      <UiSelect
        :model-value="vAxis"
        :options="vAxisOptions"
        size="sm"
        :aria-label="t.travVAxis || 'Vertical axis'"
        @update:model-value="onVAxisChange"
      />
    </div>
  </div>
</template>

<style scoped>
/* 轴选择器：绝对定位左上角，覆盖在 canvas 上，避免占用画布空间 */
.axis-selector {
  position: absolute;
  top: 4px;
  left: 4px;
  display: flex;
  gap: 4px;
  z-index: 2;
  width: 140px;
}
.axis-selector :deep(.n-base-selection) {
  min-width: 60px;
  flex: 1;
}
</style>

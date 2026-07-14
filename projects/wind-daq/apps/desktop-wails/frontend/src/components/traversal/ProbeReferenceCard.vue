<script setup lang="ts">
/**
 * 五孔探针三维参考图。探针轴为 Z，端面前方为 -Z；Y 竖直向上，
 * X 位于偏转面。α、β 分别显示在 X-Z、Y-Z 平面中。
 */
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import UiSlider from '@components/ui/UiSlider.vue'
import { useI18nStore } from '@stores/i18nStore'

type Point3 = { x: number; y: number; z: number }
type ViewName = 'iso' | 'face' | 'top' | 'side'

const i18n = useI18nStore()
const t = computed(() => i18n.t)
const canvasRef = ref<HTMLCanvasElement | null>(null)
const stageRef = ref<HTMLElement | null>(null)
const alpha = ref(25)
const beta = ref(20)
const activeView = ref<ViewName>('iso')

const camera = { yaw: -0.62, pitch: 0.34, zoom: 1 }
const views: Record<ViewName, typeof camera> = {
  iso: { yaw: -0.62, pitch: 0.34, zoom: 1 },
  face: { yaw: -Math.PI / 2, pitch: 0, zoom: 1.1 },
  top: { yaw: 0, pitch: Math.PI / 2, zoom: 1.04 },
  side: { yaw: 0, pitch: 0, zoom: 1.04 },
}

let resizeObserver: ResizeObserver | null = null
let themeObserver: MutationObserver | null = null
let dragging = false
let lastPointerX = 0
let lastPointerY = 0
let frameId = 0

const point = (x: number, y: number, z: number): Point3 => ({ x, y, z })
const multiply = (p: Point3, factor: number): Point3 => point(p.x * factor, p.y * factor, p.z * factor)
const normalize = (p: Point3): Point3 => {
  const length = Math.hypot(p.x, p.y, p.z) || 1
  return multiply(p, 1 / length)
}
const radians = (degrees: number) => degrees * Math.PI / 180
const signedAngle = (value: number) => `${value > 0 ? '+' : ''}${value}°`

function cssColor(name: string, fallback: string) {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim() || fallback
}

function colors() {
  return {
    flow: cssColor('--accent-info', '#38bdf8'),
    alpha: cssColor('--accent-warning', '#f59e0b'),
    beta: cssColor('--accent-danger', '#ef5b47'),
    x: cssColor('--accent-danger', '#ef5b47'),
    y: cssColor('--accent-info', '#38bdf8'),
    z: cssColor('--accent-success', '#22c55e'),
    text: cssColor('--text-primary', '#e2e8f0'),
    muted: cssColor('--text-muted', '#94a3b8'),
    border: cssColor('--border-strong', '#475569'),
    panel: cssColor('--bg-panel', '#172338'),
    canvas: cssColor('--bg-canvas', '#111c31'),
  }
}

function scheduleDraw() {
  cancelAnimationFrame(frameId)
  frameId = requestAnimationFrame(draw)
}

function resizeCanvas() {
  const canvas = canvasRef.value
  const stage = stageRef.value
  if (!canvas || !stage) return
  const rect = stage.getBoundingClientRect()
  const dpr = Math.min(window.devicePixelRatio || 1, 2)
  canvas.width = Math.round(rect.width * dpr)
  canvas.height = Math.round(rect.height * dpr)
  canvas.style.width = `${rect.width}px`
  canvas.style.height = `${rect.height}px`
  scheduleDraw()
}

function rotate(p: Point3): Point3 {
  const cosYaw = Math.cos(camera.yaw)
  const sinYaw = Math.sin(camera.yaw)
  const cosPitch = Math.cos(camera.pitch)
  const sinPitch = Math.sin(camera.pitch)
  const x = p.x * cosYaw + p.z * sinYaw
  const z = -p.x * sinYaw + p.z * cosYaw
  return point(x, p.y * cosPitch - z * sinPitch, p.y * sinPitch + z * cosPitch)
}

function draw() {
  const canvas = canvasRef.value
  const stage = stageRef.value
  if (!canvas || !stage) return
  const canvasContext = canvas.getContext('2d')
  if (!canvasContext) return
  const context: CanvasRenderingContext2D = canvasContext

  const rect = stage.getBoundingClientRect()
  const dpr = Math.min(window.devicePixelRatio || 1, 2)
  context.setTransform(dpr, 0, 0, dpr, 0, 0)
  context.clearRect(0, 0, rect.width, rect.height)
  const palette = colors()

  function project(p: Point3) {
    const rotated = rotate(p)
    const scale = Math.min(rect.width, rect.height) * 0.105 * camera.zoom
    const perspective = 1 / Math.max(0.64, 1 - rotated.z / 18)
    return {
      x: rect.width * 0.48 + rotated.x * scale * perspective,
      y: rect.height * 0.53 - rotated.y * scale * perspective,
      depth: rotated.z,
    }
  }

  function line(start: Point3, end: Point3, color: string, width = 1, dash: number[] = []) {
    const a = project(start)
    const b = project(end)
    context.save()
    context.beginPath()
    context.moveTo(a.x, a.y)
    context.lineTo(b.x, b.y)
    context.strokeStyle = color
    context.lineWidth = width
    context.setLineDash(dash)
    context.stroke()
    context.restore()
  }

  function polygon(points: Point3[], fill: string, stroke: string, width = 1) {
    const projected = points.map(project)
    context.beginPath()
    context.moveTo(projected[0].x, projected[0].y)
    for (let index = 1; index < projected.length; index++) {
      context.lineTo(projected[index].x, projected[index].y)
    }
    context.closePath()
    context.fillStyle = fill
    context.fill()
    context.strokeStyle = stroke
    context.lineWidth = width
    context.stroke()
  }

  function label(text: string, position: Point3, color = palette.text, font = '700 12px monospace') {
    const p = project(position)
    context.save()
    context.font = font
    context.textAlign = 'center'
    context.textBaseline = 'middle'
    context.lineWidth = 4
    context.strokeStyle = palette.canvas
    context.strokeText(text, p.x, p.y)
    context.fillStyle = color
    context.fillText(text, p.x, p.y)
    context.restore()
  }

  function arrow(start: Point3, end: Point3, color: string, width: number) {
    const a = project(start)
    const b = project(end)
    const angle = Math.atan2(b.y - a.y, b.x - a.x)
    const head = 10 + width
    context.save()
    context.beginPath()
    context.moveTo(a.x, a.y)
    context.lineTo(b.x, b.y)
    context.strokeStyle = color
    context.lineWidth = width
    context.lineCap = 'round'
    context.stroke()
    context.beginPath()
    context.moveTo(b.x, b.y)
    context.lineTo(b.x - head * Math.cos(angle - 0.42), b.y - head * Math.sin(angle - 0.42))
    context.lineTo(b.x - head * Math.cos(angle + 0.42), b.y - head * Math.sin(angle + 0.42))
    context.closePath()
    context.fillStyle = color
    context.fill()
    context.restore()
  }

  function polyline(points: Point3[], color: string, width: number) {
    const projected = points.map(project)
    context.beginPath()
    context.moveTo(projected[0].x, projected[0].y)
    for (let index = 1; index < projected.length; index++) {
      context.lineTo(projected[index].x, projected[index].y)
    }
    context.strokeStyle = color
    context.lineWidth = width
    context.lineCap = 'round'
    context.stroke()
  }

  function planeGrid(kind: 'xz' | 'yz', color: string) {
    context.save()
    context.globalAlpha = 0.34
    const span = 5.3
    if (kind === 'xz') {
      polygon([point(-5.2, 0, -span), point(1.1, 0, -span), point(1.1, 0, span), point(-5.2, 0, span)], palette.canvas, color)
      for (let z = -5; z <= 5; z++) line(point(-5.2, 0, z), point(1.1, 0, z), color, 0.6)
    } else {
      polygon([point(-5.2, -span, 0), point(1.1, -span, 0), point(1.1, span, 0), point(-5.2, span, 0)], palette.canvas, color)
      for (let y = -5; y <= 5; y++) line(point(-5.2, y, 0), point(1.1, y, 0), color, 0.6)
    }
    context.restore()
  }

  function drawProbe() {
    const segments = 24
    const radius = 0.62
    const faces: Array<{ points: Point3[]; depth: number; shade: number }> = []
    for (let index = 0; index < segments; index++) {
      const a = index / segments * Math.PI * 2
      const b = (index + 1) / segments * Math.PI * 2
      const points = [
        point(0.08, Math.cos(a) * radius, Math.sin(a) * radius),
        point(4.7, Math.cos(a) * radius, Math.sin(a) * radius),
        point(4.7, Math.cos(b) * radius, Math.sin(b) * radius),
        point(0.08, Math.cos(b) * radius, Math.sin(b) * radius),
      ]
      faces.push({
        points,
        depth: points.reduce((sum, p) => sum + rotate(p).z, 0) / points.length,
        shade: 38 + Math.round(Math.max(0, Math.cos(a - 0.7)) * 24),
      })
    }
    faces.sort((a, b) => a.depth - b.depth)
    faces.forEach(face => {
      context.save()
      context.globalAlpha = face.shade / 100
      polygon(face.points, palette.text, palette.border, 0.6)
      context.restore()
    })

    const face = Array.from({ length: segments }, (_, index) => {
      const angle = index / segments * Math.PI * 2
      return point(0, Math.cos(angle) * radius, Math.sin(angle) * radius)
    })
    polygon(face, palette.muted, palette.text, 1.2)

    const ports = [
      { id: 'P2', y: 0, z: 0, labelY: 0, labelZ: 0 },
      { id: 'P3', y: 0.34, z: 0, labelY: 0.53, labelZ: 0 },
      { id: 'P1', y: -0.34, z: 0, labelY: -0.53, labelZ: 0 },
      { id: 'P4', y: 0, z: -0.34, labelY: 0, labelZ: -0.53 },
      { id: 'P5', y: 0, z: 0.34, labelY: 0, labelZ: 0.53 },
    ]
    ports.forEach(port => {
      const hole = Array.from({ length: 16 }, (_, index) => {
        const angle = index / 16 * Math.PI * 2
        return point(-0.03, port.y + Math.cos(angle) * 0.09, port.z + Math.sin(angle) * 0.09)
      })
      polygon(hole, palette.canvas, palette.text, 0.7)
      label(port.id, point(-0.08, port.labelY, port.labelZ), palette.text, '700 10px monospace')
    })
  }

  function arc(plane: 'xz' | 'yz', angle: number, radius: number) {
    if (angle === 0) return
    const steps = Math.max(2, Math.ceil(Math.abs(angle) / 3))
    const points = Array.from({ length: steps + 1 }, (_, index) => {
      const value = radians(angle * index / steps)
      return plane === 'xz'
        ? point(-Math.cos(value) * radius, 0, -Math.sin(value) * radius)
        : point(-Math.cos(value) * radius, Math.sin(value) * radius, 0)
    })
    polyline(points, plane === 'xz' ? palette.alpha : palette.beta, 3)
  }

  planeGrid('xz', palette.alpha)
  planeGrid('yz', palette.beta)

  // 内部几何 x 对应探针 Z 轴，z 对应 X 轴。
  arrow(point(0, 0, 0), point(-5.25, 0, 0), palette.z, 2)
  arrow(point(0, 0, 0), point(0, 2.35, 0), palette.y, 2)
  arrow(point(0, 0, 0), point(0, 0, 2.35), palette.x, 2)
  label('-Z', point(-5.58, 0, 0), palette.z, '700 15px monospace')
  label('Y', point(0, 2.67, 0), palette.y, '700 15px monospace')
  label('X', point(0, 0, 2.67), palette.x, '700 15px monospace')

  const alphaTangent = Math.tan(radians(alpha.value))
  const betaTangent = Math.tan(radians(beta.value))
  const flow = normalize(point(1, -betaTangent, alphaTangent))
  const flowStart = multiply(flow, -6.2)
  const facePoint = point(-0.08, 0, 0)
  const alphaProjection = normalize(point(1, 0, alphaTangent))
  const betaProjection = normalize(point(1, -betaTangent, 0))
  line(multiply(alphaProjection, -5.55), facePoint, palette.alpha, 1.5, [7, 5])
  line(multiply(betaProjection, -5.55), facePoint, palette.beta, 1.5, [7, 5])
  line(flowStart, multiply(alphaProjection, -5.55), palette.alpha, 1, [3, 5])
  line(flowStart, multiply(betaProjection, -5.55), palette.beta, 1, [3, 5])
  arc('xz', alpha.value, 1.42)
  arc('yz', beta.value, 1.05)

  drawProbe()
  arrow(flowStart, facePoint, palette.flow, 5)
  label(t.value.travProbeFlowDir, multiply(flow, -5.85), palette.flow, '700 11px sans-serif')
  label(`α ${signedAngle(alpha.value)}`, point(-1.7, 0.08, -0.45), palette.alpha)
  label(`β ${signedAngle(beta.value)}`, point(-1.35, beta.value >= 0 ? 0.52 : -0.52, 0.08), palette.beta)
}

function setView(view: ViewName) {
  Object.assign(camera, views[view])
  activeView.value = view
  scheduleDraw()
}

function onPointerDown(event: PointerEvent) {
  const canvas = canvasRef.value
  if (!canvas) return
  dragging = true
  lastPointerX = event.clientX
  lastPointerY = event.clientY
  canvas.setPointerCapture(event.pointerId)
}

function onPointerMove(event: PointerEvent) {
  if (!dragging) return
  camera.yaw += (event.clientX - lastPointerX) * 0.008
  camera.pitch = Math.max(-1.45, Math.min(1.45, camera.pitch + (event.clientY - lastPointerY) * 0.008))
  lastPointerX = event.clientX
  lastPointerY = event.clientY
  activeView.value = 'iso'
  scheduleDraw()
}

function onPointerUp(event: PointerEvent) {
  dragging = false
  const canvas = canvasRef.value
  if (canvas?.hasPointerCapture(event.pointerId)) canvas.releasePointerCapture(event.pointerId)
}

function onWheel(event: WheelEvent) {
  event.preventDefault()
  camera.zoom = Math.max(0.7, Math.min(1.5, camera.zoom * Math.exp(-event.deltaY * 0.001)))
  scheduleDraw()
}

watch([alpha, beta], scheduleDraw)

onMounted(async () => {
  await nextTick()
  if (stageRef.value) {
    resizeObserver = new ResizeObserver(resizeCanvas)
    resizeObserver.observe(stageRef.value)
  }
  themeObserver = new MutationObserver(scheduleDraw)
  themeObserver.observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme'] })
  resizeCanvas()
})

onBeforeUnmount(() => {
  resizeObserver?.disconnect()
  themeObserver?.disconnect()
  cancelAnimationFrame(frameId)
})
</script>

<template>
  <section class="probe-reference">
    <div ref="stageRef" class="probe-stage">
      <canvas
        ref="canvasRef"
        :aria-label="t.travProbeDiagramTitle"
        @pointerdown="onPointerDown"
        @pointermove="onPointerMove"
        @pointerup="onPointerUp"
        @pointercancel="onPointerUp"
        @dblclick="setView('iso')"
        @wheel="onWheel"
      />
      <div class="stage-help">{{ t.travProbe3dHelp }}</div>
      <div class="stage-legend" aria-hidden="true">
        <span class="legend-item legend-flow">{{ t.travProbeFlowDir }}</span>
        <span class="legend-item legend-alpha">α · X-Z</span>
        <span class="legend-item legend-beta">β · Y-Z</span>
      </div>
    </div>

    <aside class="probe-controls">
      <div>
        <h3 class="control-title">{{ t.travProbeDiagramTitle }}</h3>
        <p class="control-hint">{{ t.travProbe3dHint }}</p>
      </div>

      <div class="control-group alpha-control">
        <label class="control-label">
          <span>{{ t.travProbeAlphaYaw }}</span>
          <strong>{{ signedAngle(alpha) }}</strong>
        </label>
        <UiSlider v-model="alpha" :min="-60" :max="60" :aria-label="t.travProbeAlphaYaw" />
        <div class="scale-labels"><span>-60°</span><span>0°</span><span>+60°</span></div>
      </div>

      <div class="control-group beta-control">
        <label class="control-label">
          <span>{{ t.travProbeBetaPitch }}</span>
          <strong>{{ signedAngle(beta) }}</strong>
        </label>
        <UiSlider v-model="beta" :min="-60" :max="60" :aria-label="t.travProbeBetaPitch" />
        <div class="scale-labels"><span>-60°</span><span>0°</span><span>+60°</span></div>
      </div>

      <div class="control-group">
        <div class="view-grid">
          <button
            v-for="view in (['iso', 'face', 'top', 'side'] as ViewName[])"
            :key="view"
            type="button"
            class="view-button"
            :class="{ 'view-button--active': activeView === view }"
            @click="setView(view)"
          >
            {{ t[`travProbeView_${view}`] }}
          </button>
        </div>
      </div>

      <div class="reference-notes">
        <p><strong class="alpha-text">α</strong>{{ t.travProbeAlpha3dHint }}</p>
        <p><strong class="beta-text">β</strong>{{ t.travProbeBeta3dHint }}</p>
        <p>{{ t.travProbeAxis3dHint }}</p>
        <p>{{ t.travProbePortsHint }}</p>
      </div>
    </aside>
  </section>
</template>

<style scoped>
.probe-reference {
  --probe-alpha: var(--accent-warning);
  --probe-beta: var(--accent-danger);
  --probe-flow: var(--accent-info);
  display: flex;
  min-width: 0;
  min-height: 480px;
  height: 100%;
  overflow: hidden;
  border: 1px solid var(--border-default);
  border-radius: var(--radius-lg);
  background: var(--bg-canvas);
}

.probe-stage {
  position: relative;
  min-width: 0;
  flex: 1;
  overflow: hidden;
  background: var(--bg-canvas);
}

.probe-stage canvas {
  display: block;
  width: 100%;
  height: 100%;
  cursor: grab;
  touch-action: none;
}

.probe-stage canvas:active { cursor: grabbing; }

.stage-help,
.stage-legend {
  position: absolute;
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  background: color-mix(in srgb, var(--bg-panel) 88%, transparent);
  color: var(--text-muted);
  font-size: var(--font-size-xs);
  pointer-events: none;
}

.stage-help {
  top: var(--space-3);
  left: var(--space-3);
  padding: var(--space-2) var(--space-3);
}

.stage-legend {
  bottom: var(--space-3);
  left: var(--space-3);
  display: flex;
  gap: var(--space-4);
  padding: var(--space-2) var(--space-3);
}

.legend-item::before {
  content: '';
  display: inline-block;
  width: 1rem;
  height: 2px;
  margin-right: var(--space-1);
  vertical-align: middle;
  background: currentColor;
}

.legend-flow { color: var(--probe-flow); }
.legend-alpha { color: var(--probe-alpha); }
.legend-beta { color: var(--probe-beta); }

.probe-controls {
  width: 280px;
  flex: 0 0 280px;
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  padding: var(--space-5);
  overflow-y: auto;
  border-left: 1px solid var(--border-default);
  background: var(--bg-panel);
  color: var(--text-primary);
}

.control-title {
  margin: 0;
  color: var(--text-primary);
  font-size: var(--font-size-base);
  font-weight: 650;
}

.control-hint {
  margin: var(--space-1) 0 0;
  color: var(--text-secondary);
  font-size: var(--font-size-xs);
  line-height: 1.55;
}

.control-group {
  padding-top: var(--space-3);
  border-top: 1px solid var(--border-default);
}

.control-label {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2);
  margin-bottom: var(--space-2);
  color: var(--text-secondary);
  font-size: var(--font-size-xs);
}

.control-label strong {
  font-family: var(--font-family-mono);
  font-size: var(--font-size-sm);
  font-variant-numeric: tabular-nums;
}

.alpha-control .control-label strong,
.alpha-text { color: var(--probe-alpha); }
.beta-control .control-label strong,
.beta-text { color: var(--probe-beta); }

.scale-labels {
  display: flex;
  justify-content: space-between;
  margin-top: var(--space-1);
  color: var(--text-muted);
  font-family: var(--font-family-mono);
  font-size: 10px;
}

.view-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--space-2);
}

.view-button {
  min-height: 30px;
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  background: var(--bg-panel-strong);
  color: var(--text-secondary);
  font-size: var(--font-size-xs);
  cursor: pointer;
}

.view-button:hover,
.view-button:focus-visible,
.view-button--active {
  border-color: var(--accent-primary);
  color: var(--accent-primary);
  outline: none;
}

.reference-notes {
  display: grid;
  gap: var(--space-2);
  padding: var(--space-3);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  background: var(--bg-panel-strong);
  color: var(--text-secondary);
  font-size: var(--font-size-xs);
  line-height: 1.5;
}

.reference-notes p { margin: 0; }
.reference-notes strong {
  display: inline-block;
  width: 1.25rem;
}

@media (max-width: 1100px) {
  .probe-reference { min-width: 900px; }
}
</style>

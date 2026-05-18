<script setup lang="ts">
import { ref, watch, onMounted, onUnmounted, computed } from 'vue'
import type { UiSampleFrame } from '../api/wails'

const props = defineProps<{ frame: UiSampleFrame; channelCount: number }>()

const canvas = ref<HTMLCanvasElement | null>(null)
const container = ref<HTMLDivElement | null>(null)

const MAX_POINTS = 1000
const channelColors = ['#3B82F6', '#22C55E', '#F59E0B', '#A855F7']

const buffers = computed(() => {
  const bufs: Float32Array[] = []
  for (let i = 0; i < props.channelCount; i++) {
    bufs.push(new Float32Array(MAX_POINTS))
  }
  return bufs
})

let writeIdx = 0
let ctx: CanvasRenderingContext2D | null = null
let animId = 0

function pushFrame() {
  const f = props.frame
  if (!f || f.sampleCount === 0) return
  for (let ch = 0; ch < props.channelCount; ch++) {
    const val = f.latestValues[ch] ?? 0
    const buf = buffers.value[ch]
    if (buf) {
      buf[writeIdx % MAX_POINTS] = val
    }
  }
  writeIdx++
  scheduleDraw()
}

let drawScheduled = false

function scheduleDraw() {
  if (drawScheduled) return
  drawScheduled = true
  animId = requestAnimationFrame(() => {
    drawScheduled = false
    draw()
  })
}

function draw() {
  if (!ctx || !canvas.value) return
  const w = canvas.value.width
  const h = canvas.value.height
  const nc = props.channelCount
  if (nc === 0) return

  ctx.clearRect(0, 0, w, h)

  // Grid
  ctx.strokeStyle = 'rgba(255,255,255,0.04)'
  ctx.lineWidth = 1
  for (let gy = 0; gy <= 6; gy++) {
    const y = (h / 6) * gy
    ctx.beginPath()
    ctx.moveTo(0, y)
    ctx.lineTo(w, y)
    ctx.stroke()
  }

  const count = Math.min(writeIdx, MAX_POINTS)
  if (count < 2) return

  const start = writeIdx > MAX_POINTS ? writeIdx % MAX_POINTS : 0

  for (let ch = 0; ch < nc; ch++) {
    const buf = buffers.value[ch]
    if (!buf) continue
    const chH = h / nc
    const yOff = chH * ch + chH / 2
    const amp = chH * 0.4

    ctx.strokeStyle = channelColors[ch]
    ctx.lineWidth = 1.5
    ctx.beginPath()

    let first = true
    for (let i = 0; i < count; i++) {
      const idx = (start + i) % MAX_POINTS
      const x = (i / (count - 1)) * w
      const y = yOff - buf[idx] * amp
      if (first) {
        ctx.moveTo(x, y)
        first = false
      } else {
        ctx.lineTo(x, y)
      }
    }
    ctx.stroke()
  }
}

function resize() {
  if (!canvas.value || !container.value) return
  const rect = container.value.getBoundingClientRect()
  const dpr = window.devicePixelRatio || 1
  canvas.value.width = rect.width * dpr
  canvas.value.height = rect.height * dpr
  canvas.value.style.width = rect.width + 'px'
  canvas.value.style.height = rect.height + 'px'
  if (ctx) {
    ctx.scale(dpr, dpr)
  }
  draw()
}

watch(() => props.frame.sequenceStart, () => {
  pushFrame()
})

onMounted(() => {
  ctx = canvas.value?.getContext('2d') ?? null
  resize()
  window.addEventListener('resize', resize)
})

onUnmounted(() => {
  window.removeEventListener('resize', resize)
  if (animId) cancelAnimationFrame(animId)
})
</script>

<template>
  <div ref="container" class="waveform-container">
    <canvas ref="canvas"></canvas>
    <div class="channel-labels">
      <span v-for="ch in channelCount" :key="ch" :style="{ color: channelColors[ch - 1] }">
        CH{{ ch - 1 }}
      </span>
    </div>
  </div>
</template>

<style scoped>
.waveform-container {
  flex: 1;
  position: relative;
  background: var(--bg-canvas);
  border: 1px solid var(--border-default);
  border-radius: 3px;
  overflow: hidden;
  min-height: 0;
}

canvas {
  display: block;
  width: 100%;
  height: 100%;
}

.channel-labels {
  position: absolute;
  top: 6px;
  right: 10px;
  display: flex;
  gap: 12px;
  font-family: var(--font-mono);
  font-size: 10px;
  font-weight: 600;
  opacity: 0.7;
}
</style>

<script setup lang="ts">
import { computed } from 'vue'
import type { StepSegment, TraversalPattern } from '@shared/types/traversal'
import UiButton from '@components/ui/UiButton.vue'
import { useTraversalSegmentValidation, getSegmentError, hasSegmentError } from '@composables/useTraversalValidation'

const testName = defineModel<string>('testName', { required: true })
const dwellTimeMs = defineModel<number>('dwellTimeMs', { required: true })
const samplesPerPoint = defineModel<number>('samplesPerPoint', { required: true })
const pattern = defineModel<TraversalPattern>('pattern', { required: true })
const lineConfig = defineModel<{
  startX: number; startY: number; endX: number; endY: number
  xStepSegments: StepSegment[]; yStepSegments: StepSegment[]
}>('lineConfig', { required: true })
const rectangleConfig = defineModel<{
  xMin: number; xMax: number; xStepSegments: StepSegment[]
  yMin: number; yMax: number; yStepSegments: StepSegment[]
}>('rectangleConfig', { required: true })
const sectorConfig = defineModel<{
  centerX: number; centerY: number; radiusMin: number; radiusMax: number
  radialStepSegments: StepSegment[]; angleStart: number; angleEnd: number
  angularStepSegments: StepSegment[]
}>('sectorConfig', { required: true })
const customPoints = defineModel<Array<{ x: number; y: number }>>('customPoints', { required: true })
const customPointInput = defineModel<{ x: number; y: number }>('customPointInput', { required: true })

const props = defineProps<{
  estimatedPointCount: number
  t: Record<string, string>
}>()

const computedRectangleRange = computed(() => {
  const xs = rectangleConfig.value.xStepSegments
  const ys = rectangleConfig.value.yStepSegments
  const xMin = xs.length > 0 ? Math.min(...xs.map(s => s.start)) : 0
  const xMax = xs.length > 0 ? Math.max(...xs.map(s => s.end)) : 0
  const yMin = ys.length > 0 ? Math.min(...ys.map(s => s.start)) : 0
  const yMax = ys.length > 0 ? Math.max(...ys.map(s => s.end)) : 0
  rectangleConfig.value.xMin = xMin
  rectangleConfig.value.xMax = xMax
  rectangleConfig.value.yMin = yMin
  rectangleConfig.value.yMax = yMax
  return { xMin, xMax, yMin, yMax }
})

const computedSectorRange = computed(() => {
  const rs = sectorConfig.value.radialStepSegments
  const as = sectorConfig.value.angularStepSegments
  const radiusMin = rs.length > 0 ? Math.min(...rs.map(s => s.start)) : 0
  const radiusMax = rs.length > 0 ? Math.max(...rs.map(s => s.end)) : 0
  const angleStart = as.length > 0 ? Math.min(...as.map(s => s.start)) : 0
  const angleEnd = as.length > 0 ? Math.max(...as.map(s => s.end)) : 0
  sectorConfig.value.radiusMin = radiusMin
  sectorConfig.value.radiusMax = radiusMax
  sectorConfig.value.angleStart = angleStart
  sectorConfig.value.angleEnd = angleEnd
  return { radiusMin, radiusMax, angleStart, angleEnd }
})

const tRef = computed(() => props.t)

const { errors: rectangleXSegmentErrors, countError: rectangleXSegmentCountError } =
  useTraversalSegmentValidation(computed(() => rectangleConfig.value.xStepSegments), computed(() => rectangleConfig.value.xMin), computed(() => rectangleConfig.value.xMax), tRef)
const { errors: rectangleYSegmentErrors, countError: rectangleYSegmentCountError } =
  useTraversalSegmentValidation(computed(() => rectangleConfig.value.yStepSegments), computed(() => rectangleConfig.value.yMin), computed(() => rectangleConfig.value.yMax), tRef)
const { errors: sectorRadialSegmentErrors, countError: sectorRadialSegmentCountError } =
  useTraversalSegmentValidation(computed(() => sectorConfig.value.radialStepSegments), computed(() => sectorConfig.value.radiusMin), computed(() => sectorConfig.value.radiusMax), tRef)
const { errors: sectorAngularSegmentErrors, countError: sectorAngularSegmentCountError } =
  useTraversalSegmentValidation(computed(() => sectorConfig.value.angularStepSegments), computed(() => sectorConfig.value.angleStart), computed(() => sectorConfig.value.angleEnd), tRef)
const { errors: lineXSegmentErrors, countError: lineXSegmentCountError } =
  useTraversalSegmentValidation(computed(() => lineConfig.value.xStepSegments), computed(() => lineConfig.value.startX), computed(() => lineConfig.value.endX), tRef)
const { errors: lineYSegmentErrors, countError: lineYSegmentCountError } =
  useTraversalSegmentValidation(computed(() => lineConfig.value.yStepSegments), computed(() => lineConfig.value.startY), computed(() => lineConfig.value.endY), tRef)

function addSegment() { lineConfig.value.xStepSegments.push({ start: -30, end: 30, step: 5 }) }
function removeSegment(index: number) { lineConfig.value.xStepSegments.splice(index, 1) }
function addRectangleXSegment() {
  const segs = rectangleConfig.value.xStepSegments
  if (segs.length === 0) segs.push({ start: 0, end: 10, step: 5 })
  else { const last = segs[segs.length - 1]; segs.push({ start: last.end, end: last.end + 10, step: 5 }) }
}
function removeRectangleXSegment(index: number) { rectangleConfig.value.xStepSegments.splice(index, 1) }
function addRectangleYSegment() {
  const segs = rectangleConfig.value.yStepSegments
  if (segs.length === 0) segs.push({ start: 0, end: 10, step: 5 })
  else { const last = segs[segs.length - 1]; segs.push({ start: last.end, end: last.end + 10, step: 5 }) }
}
function removeRectangleYSegment(index: number) { rectangleConfig.value.yStepSegments.splice(index, 1) }
function addSectorRadialSegment() {
  const segs = sectorConfig.value.radialStepSegments
  if (segs.length === 0) segs.push({ start: 100, end: 200, step: 50 })
  else { const last = segs[segs.length - 1]; segs.push({ start: last.end, end: last.end + 100, step: 50 }) }
}
function removeSectorRadialSegment(index: number) { sectorConfig.value.radialStepSegments.splice(index, 1) }
function addSectorAngularSegment() {
  const segs = sectorConfig.value.angularStepSegments
  if (segs.length === 0) segs.push({ start: 0, end: 10, step: 5 })
  else { const last = segs[segs.length - 1]; segs.push({ start: last.end, end: last.end + 10, step: 5 }) }
}
function removeSectorAngularSegment(index: number) { sectorConfig.value.angularStepSegments.splice(index, 1) }
function addCustomPoint() { customPoints.value.push({ x: customPointInput.value.x, y: customPointInput.value.y }); customPointInput.value = { x: 0, y: 0 } }
function removeCustomPoint(index: number) { customPoints.value.splice(index, 1) }
</script>

<template>
  <div class="space-y-4">
    <section class="p-3 rounded-[var(--radius-md)] border border-[color:var(--border-default)] bg-[color:var(--bg-panel)]">
      <div class="grid gap-3 grid-cols-3">
        <div>
          <label class="block text-[10px] text-[color:var(--text-muted)] mb-1">{{ t.testNameLabel }}</label>
          <input v-model="testName" type="text" class="w-full rounded-[var(--radius-sm)] border border-[color:var(--border-default)] bg-[color:var(--bg-panel-strong)] px-2 py-1.5 text-sm text-[color:var(--text-primary)]" :placeholder="t.testNameLabel" />
        </div>
        <div>
          <label class="block text-[10px] text-[color:var(--text-muted)] mb-1">{{ t.dwellMsLabel }}</label>
          <input v-model.number="dwellTimeMs" type="number" min="100" max="60000" class="w-full rounded-[var(--radius-sm)] border border-[color:var(--border-default)] bg-[color:var(--bg-panel-strong)] px-2 py-1.5 text-sm text-[color:var(--text-primary)]" :placeholder="t.dwellMsLabel" />
        </div>
        <div>
          <label class="block text-[10px] text-[color:var(--text-muted)] mb-1">{{ t.samplesLabel }}</label>
          <input v-model.number="samplesPerPoint" type="number" min="1" max="1000" class="w-full rounded-[var(--radius-sm)] border border-[color:var(--border-default)] bg-[color:var(--bg-panel-strong)] px-2 py-1.5 text-sm text-[color:var(--text-primary)]" :placeholder="t.samplesLabel" />
        </div>
      </div>
    </section>

    <section class="flex gap-1.5">
      <button v-for="p in (['line', 'rectangle', 'sector', 'custom'] as const)" :key="p"
        @click="pattern = p"
        class="px-3 py-1.5 text-sm font-medium rounded-[var(--radius-sm)] transition-colors"
        :class="pattern === p ? 'bg-[color:var(--accent-primary)] text-white' : 'bg-[color:var(--bg-panel)] text-[color:var(--text-secondary)] border border-[color:var(--border-default)] hover:bg-[color:var(--bg-panel-strong)]'">
        {{ p === 'line' ? t.patternLine : p === 'rectangle' ? t.patternRectangle : p === 'sector' ? t.patternSector : t.patternCustom }}
      </button>
    </section>

    <!-- Line pattern -->
    <section v-if="pattern === 'line'" class="p-3 rounded-[var(--radius-md)] border border-[color:var(--border-default)] bg-[color:var(--bg-panel)]">
      <div class="grid gap-3 lg:grid-cols-[180px_1fr]">
        <div class="p-2 rounded-[var(--radius-sm)] border border-[color:var(--border-default)] bg-[color:var(--bg-panel-strong)]">
          <div class="text-[10px] font-medium text-[color:var(--text-muted)] uppercase mb-2">{{ t.pointLayout }}</div>
          <div class="grid grid-cols-2 gap-1.5">
            <div><label class="block text-[9px] text-[color:var(--text-muted)] mb-0.5">{{ t.startX }}</label><input v-model.number="lineConfig.startX" type="number" class="w-full rounded-[var(--radius-sm)] border border-[color:var(--border-default)] bg-[color:var(--bg-panel)] px-2 py-1 text-sm text-[color:var(--text-primary)]" /></div>
            <div><label class="block text-[9px] text-[color:var(--text-muted)] mb-0.5">{{ t.startY }}</label><input v-model.number="lineConfig.startY" type="number" class="w-full rounded-[var(--radius-sm)] border border-[color:var(--border-default)] bg-[color:var(--bg-panel)] px-2 py-1 text-sm text-[color:var(--text-primary)]" /></div>
            <div><label class="block text-[9px] text-[color:var(--text-muted)] mb-0.5">{{ t.endX }}</label><input v-model.number="lineConfig.endX" type="number" class="w-full rounded-[var(--radius-sm)] border border-[color:var(--border-default)] bg-[color:var(--bg-panel)] px-2 py-1 text-sm text-[color:var(--text-primary)]" /></div>
            <div><label class="block text-[9px] text-[color:var(--text-muted)] mb-0.5">{{ t.endY }}</label><input v-model.number="lineConfig.endY" type="number" class="w-full rounded-[var(--radius-sm)] border border-[color:var(--border-default)] bg-[color:var(--bg-panel)] px-2 py-1 text-sm text-[color:var(--text-primary)]" /></div>
          </div>
        </div>
        <div class="p-2 rounded-[var(--radius-sm)] border border-[color:var(--border-default)] bg-[color:var(--bg-panel-strong)]">
          <div class="flex items-center justify-between mb-1.5">
            <div class="text-[10px] font-medium text-[color:var(--text-muted)] uppercase">{{ t.xSegments }}</div>
            <UiButton size="sm" variant="secondary" @click="addSegment">{{ t.addSegment }}</UiButton>
          </div>
          <div class="flex items-center gap-1.5 pb-1 text-[9px] text-[color:var(--text-muted)]">
            <div class="flex-1">{{ t.start }}</div><div class="flex-1">{{ t.end }}</div><div class="flex-1">{{ t.step }}</div><div class="w-12"></div>
          </div>
          <div v-for="(segment, index) in lineConfig.xStepSegments" :key="index" class="mb-1.5">
            <div class="grid grid-cols-[1fr_1fr_1fr_auto] gap-1.5 items-center">
              <input v-model.number="segment.start" type="number" class="w-full rounded-[var(--radius-sm)] border px-2 py-1 text-sm text-[color:var(--text-primary)]" :class="hasSegmentError(lineXSegmentErrors, index, 'start') ? 'border-red-500 bg-red-50' : 'border-[color:var(--border-default)] bg-[color:var(--bg-panel)]'" />
              <input v-model.number="segment.end" type="number" class="w-full rounded-[var(--radius-sm)] border px-2 py-1 text-sm text-[color:var(--text-primary)]" :class="hasSegmentError(lineXSegmentErrors, index, 'end') ? 'border-red-500 bg-red-50' : 'border-[color:var(--border-default)] bg-[color:var(--bg-panel)]'" />
              <input v-model.number="segment.step" type="number" class="w-full rounded-[var(--radius-sm)] border px-2 py-1 text-sm text-[color:var(--text-primary)]" :class="hasSegmentError(lineXSegmentErrors, index, 'step') ? 'border-red-500 bg-red-50' : 'border-[color:var(--border-default)] bg-[color:var(--bg-panel)]'" />
              <UiButton size="sm" variant="danger" :disabled="lineConfig.xStepSegments.length === 1" @click="removeSegment(index)">{{ t.del }}</UiButton>
            </div>
            <div v-if="getSegmentError(lineXSegmentErrors, index, 'start') || getSegmentError(lineXSegmentErrors, index, 'end') || getSegmentError(lineXSegmentErrors, index, 'step')" class="text-[8px] text-red-500 mt-0.5 pl-1">
              {{ getSegmentError(lineXSegmentErrors, index, 'start') || getSegmentError(lineXSegmentErrors, index, 'end') || getSegmentError(lineXSegmentErrors, index, 'step') }}
            </div>
          </div>
        </div>
      </div>
    </section>

    <!-- Rectangle pattern -->
    <section v-else-if="pattern === 'rectangle'" class="p-3 rounded-[var(--radius-md)] border border-[color:var(--border-default)] bg-[color:var(--bg-panel)]">
      <div class="space-y-3">
        <div class="p-2 rounded-[var(--radius-sm)] border border-[color:var(--border-default)] bg-[color:var(--bg-panel-strong)]">
          <div class="text-[10px] font-medium text-[color:var(--text-muted)] uppercase mb-2">{{ t.pointLayout }} ({{ t.xMin }}: {{ computedRectangleRange.xMin }}, {{ t.xMax }}: {{ computedRectangleRange.xMax }}, {{ t.yMin }}: {{ computedRectangleRange.yMin }}, {{ t.yMax }}: {{ computedRectangleRange.yMax }})</div>
        </div>
        <div class="p-2 rounded-[var(--radius-sm)] border border-[color:var(--border-default)] bg-[color:var(--bg-panel-strong)]">
          <div class="flex items-center justify-between mb-1.5">
            <div class="text-[10px] font-medium text-[color:var(--text-muted)] uppercase">{{ t.xSegments }}</div>
            <UiButton size="sm" variant="secondary" @click="addRectangleXSegment">{{ t.addSegment }}</UiButton>
          </div>
          <div class="flex items-center gap-1.5 pb-1 text-[9px] text-[color:var(--text-muted)]"><div class="flex-1">{{ t.start }}</div><div class="flex-1">{{ t.end }}</div><div class="flex-1">{{ t.step }}</div><div class="w-12"></div></div>
          <div v-for="(segment, index) in rectangleConfig.xStepSegments" :key="index" class="mb-1.5">
            <div class="grid grid-cols-[1fr_1fr_1fr_auto] gap-1.5 items-center">
              <input v-model.number="segment.start" type="number" class="w-full rounded-[var(--radius-sm)] border px-2 py-1 text-sm text-[color:var(--text-primary)]" :class="hasSegmentError(rectangleXSegmentErrors, index, 'start') ? 'border-red-500 bg-red-50' : 'border-[color:var(--border-default)] bg-[color:var(--bg-panel)]'" />
              <input v-model.number="segment.end" type="number" class="w-full rounded-[var(--radius-sm)] border px-2 py-1 text-sm text-[color:var(--text-primary)]" :class="hasSegmentError(rectangleXSegmentErrors, index, 'end') ? 'border-red-500 bg-red-50' : 'border-[color:var(--border-default)] bg-[color:var(--bg-panel)]'" />
              <input v-model.number="segment.step" type="number" class="w-full rounded-[var(--radius-sm)] border px-2 py-1 text-sm text-[color:var(--text-primary)]" :class="hasSegmentError(rectangleXSegmentErrors, index, 'step') ? 'border-red-500 bg-red-50' : 'border-[color:var(--border-default)] bg-[color:var(--bg-panel)]'" />
              <UiButton size="sm" variant="danger" :disabled="rectangleConfig.xStepSegments.length === 1" @click="removeRectangleXSegment(index)">{{ t.del }}</UiButton>
            </div>
            <div v-if="getSegmentError(rectangleXSegmentErrors, index, 'start') || getSegmentError(rectangleXSegmentErrors, index, 'end') || getSegmentError(rectangleXSegmentErrors, index, 'step')" class="text-[8px] text-red-500 mt-0.5 pl-1">
              {{ getSegmentError(rectangleXSegmentErrors, index, 'start') || getSegmentError(rectangleXSegmentErrors, index, 'end') || getSegmentError(rectangleXSegmentErrors, index, 'step') }}
            </div>
          </div>
        </div>
        <div class="p-2 rounded-[var(--radius-sm)] border border-[color:var(--border-default)] bg-[color:var(--bg-panel-strong)]">
          <div class="flex items-center justify-between mb-1.5">
            <div class="text-[10px] font-medium text-[color:var(--text-muted)] uppercase">{{ t.ySegments }}</div>
            <UiButton size="sm" variant="secondary" @click="addRectangleYSegment">{{ t.addSegment }}</UiButton>
          </div>
          <div class="flex items-center gap-1.5 pb-1 text-[9px] text-[color:var(--text-muted)]"><div class="flex-1">{{ t.start }}</div><div class="flex-1">{{ t.end }}</div><div class="flex-1">{{ t.step }}</div><div class="w-12"></div></div>
          <div v-for="(segment, index) in rectangleConfig.yStepSegments" :key="index" class="mb-1.5">
            <div class="grid grid-cols-[1fr_1fr_1fr_auto] gap-1.5 items-center">
              <input v-model.number="segment.start" type="number" class="w-full rounded-[var(--radius-sm)] border px-2 py-1 text-sm text-[color:var(--text-primary)]" :class="hasSegmentError(rectangleYSegmentErrors, index, 'start') ? 'border-red-500 bg-red-50' : 'border-[color:var(--border-default)] bg-[color:var(--bg-panel)]'" />
              <input v-model.number="segment.end" type="number" class="w-full rounded-[var(--radius-sm)] border px-2 py-1 text-sm text-[color:var(--text-primary)]" :class="hasSegmentError(rectangleYSegmentErrors, index, 'end') ? 'border-red-500 bg-red-50' : 'border-[color:var(--border-default)] bg-[color:var(--bg-panel)]'" />
              <input v-model.number="segment.step" type="number" class="w-full rounded-[var(--radius-sm)] border px-2 py-1 text-sm text-[color:var(--text-primary)]" :class="hasSegmentError(rectangleYSegmentErrors, index, 'step') ? 'border-red-500 bg-red-50' : 'border-[color:var(--border-default)] bg-[color:var(--bg-panel)]'" />
              <UiButton size="sm" variant="danger" :disabled="rectangleConfig.yStepSegments.length === 1" @click="removeRectangleYSegment(index)">{{ t.del }}</UiButton>
            </div>
            <div v-if="getSegmentError(rectangleYSegmentErrors, index, 'start') || getSegmentError(rectangleYSegmentErrors, index, 'end') || getSegmentError(rectangleYSegmentErrors, index, 'step')" class="text-[8px] text-red-500 mt-0.5 pl-1">
              {{ getSegmentError(rectangleYSegmentErrors, index, 'start') || getSegmentError(rectangleYSegmentErrors, index, 'end') || getSegmentError(rectangleYSegmentErrors, index, 'step') }}
            </div>
          </div>
        </div>
      </div>
    </section>

    <!-- Sector pattern -->
    <section v-else-if="pattern === 'sector'" class="p-3 rounded-[var(--radius-md)] border border-[color:var(--border-default)] bg-[color:var(--bg-panel)]">
      <div class="grid gap-3 lg:grid-cols-[180px_1fr]">
        <div class="p-2 rounded-[var(--radius-sm)] border border-[color:var(--border-default)] bg-[color:var(--bg-panel-strong)]">
          <div class="text-[10px] font-medium text-[color:var(--text-muted)] uppercase mb-2">{{ t.pointLayout }}</div>
          <div class="grid grid-cols-2 gap-1.5">
            <div><label class="block text-[9px] text-[color:var(--text-muted)] mb-0.5">{{ t.centerX }}</label><input v-model.number="sectorConfig.centerX" type="number" class="w-full rounded-[var(--radius-sm)] border border-[color:var(--border-default)] bg-[color:var(--bg-panel)] px-2 py-1 text-sm text-[color:var(--text-primary)]" /></div>
            <div><label class="block text-[9px] text-[color:var(--text-muted)] mb-0.5">{{ t.centerY }}</label><input v-model.number="sectorConfig.centerY" type="number" class="w-full rounded-[var(--radius-sm)] border border-[color:var(--border-default)] bg-[color:var(--bg-panel)] px-2 py-1 text-sm text-[color:var(--text-primary)]" /></div>
            <div><label class="block text-[9px] text-[color:var(--text-muted)] mb-0.5">{{ t.radiusMin }}</label><input v-model.number="sectorConfig.radiusMin" type="number" class="w-full rounded-[var(--radius-sm)] border border-[color:var(--border-default)] bg-[color:var(--bg-panel)] px-2 py-1 text-sm text-[color:var(--text-primary)]" /></div>
            <div><label class="block text-[9px] text-[color:var(--text-muted)] mb-0.5">{{ t.radiusMax }}</label><input v-model.number="sectorConfig.radiusMax" type="number" class="w-full rounded-[var(--radius-sm)] border border-[color:var(--border-default)] bg-[color:var(--bg-panel)] px-2 py-1 text-sm text-[color:var(--text-primary)]" /></div>
            <div><label class="block text-[9px] text-[color:var(--text-muted)] mb-0.5">{{ t.angleStart }}</label><input v-model.number="sectorConfig.angleStart" type="number" class="w-full rounded-[var(--radius-sm)] border border-[color:var(--border-default)] bg-[color:var(--bg-panel)] px-2 py-1 text-sm text-[color:var(--text-primary)]" /></div>
            <div><label class="block text-[9px] text-[color:var(--text-muted)] mb-0.5">{{ t.angleEnd }}</label><input v-model.number="sectorConfig.angleEnd" type="number" class="w-full rounded-[var(--radius-sm)] border border-[color:var(--border-default)] bg-[color:var(--bg-panel)] px-2 py-1 text-sm text-[color:var(--text-primary)]" /></div>
          </div>
        </div>
        <div class="space-y-3">
          <div class="p-2 rounded-[var(--radius-sm)] border border-[color:var(--border-default)] bg-[color:var(--bg-panel-strong)]">
            <div class="flex items-center justify-between mb-1.5">
              <div class="text-[10px] font-medium text-[color:var(--text-muted)] uppercase">{{ t.radiusSegments }}</div>
              <UiButton size="sm" variant="secondary" @click="addSectorRadialSegment">{{ t.addSegment }}</UiButton>
            </div>
            <div class="flex items-center gap-1.5 pb-1 text-[9px] text-[color:var(--text-muted)]"><div class="flex-1">{{ t.start }}</div><div class="flex-1">{{ t.end }}</div><div class="flex-1">{{ t.step }}</div><div class="w-12"></div></div>
            <div v-for="(segment, index) in sectorConfig.radialStepSegments" :key="index" class="mb-1.5">
              <div class="grid grid-cols-[1fr_1fr_1fr_auto] gap-1.5 items-center">
                <input v-model.number="segment.start" type="number" class="w-full rounded-[var(--radius-sm)] border px-2 py-1 text-sm text-[color:var(--text-primary)]" :class="hasSegmentError(sectorRadialSegmentErrors, index, 'start') ? 'border-red-500 bg-red-50' : 'border-[color:var(--border-default)] bg-[color:var(--bg-panel)]'" />
                <input v-model.number="segment.end" type="number" class="w-full rounded-[var(--radius-sm)] border px-2 py-1 text-sm text-[color:var(--text-primary)]" :class="hasSegmentError(sectorRadialSegmentErrors, index, 'end') ? 'border-red-500 bg-red-50' : 'border-[color:var(--border-default)] bg-[color:var(--bg-panel)]'" />
                <input v-model.number="segment.step" type="number" class="w-full rounded-[var(--radius-sm)] border px-2 py-1 text-sm text-[color:var(--text-primary)]" :class="hasSegmentError(sectorRadialSegmentErrors, index, 'step') ? 'border-red-500 bg-red-50' : 'border-[color:var(--border-default)] bg-[color:var(--bg-panel)]'" />
                <UiButton size="sm" variant="danger" :disabled="sectorConfig.radialStepSegments.length === 1" @click="removeSectorRadialSegment(index)">{{ t.del }}</UiButton>
              </div>
              <div v-if="getSegmentError(sectorRadialSegmentErrors, index, 'start') || getSegmentError(sectorRadialSegmentErrors, index, 'end') || getSegmentError(sectorRadialSegmentErrors, index, 'step')" class="text-[8px] text-red-500 mt-0.5 pl-1">
                {{ getSegmentError(sectorRadialSegmentErrors, index, 'start') || getSegmentError(sectorRadialSegmentErrors, index, 'end') || getSegmentError(sectorRadialSegmentErrors, index, 'step') }}
              </div>
            </div>
          </div>
          <div class="p-2 rounded-[var(--radius-sm)] border border-[color:var(--border-default)] bg-[color:var(--bg-panel-strong)]">
            <div class="flex items-center justify-between mb-1.5">
              <div class="text-[10px] font-medium text-[color:var(--text-muted)] uppercase">{{ t.angleSegments }}</div>
              <UiButton size="sm" variant="secondary" @click="addSectorAngularSegment">{{ t.addSegment }}</UiButton>
            </div>
            <div class="flex items-center gap-1.5 pb-1 text-[9px] text-[color:var(--text-muted)]"><div class="flex-1">{{ t.start }}</div><div class="flex-1">{{ t.end }}</div><div class="flex-1">{{ t.step }}</div><div class="w-12"></div></div>
            <div v-for="(segment, index) in sectorConfig.angularStepSegments" :key="index" class="mb-1.5">
              <div class="grid grid-cols-[1fr_1fr_1fr_auto] gap-1.5 items-center">
                <input v-model.number="segment.start" type="number" class="w-full rounded-[var(--radius-sm)] border px-2 py-1 text-sm text-[color:var(--text-primary)]" :class="hasSegmentError(sectorAngularSegmentErrors, index, 'start') ? 'border-red-500 bg-red-50' : 'border-[color:var(--border-default)] bg-[color:var(--bg-panel)]'" />
                <input v-model.number="segment.end" type="number" class="w-full rounded-[var(--radius-sm)] border px-2 py-1 text-sm text-[color:var(--text-primary)]" :class="hasSegmentError(sectorAngularSegmentErrors, index, 'end') ? 'border-red-500 bg-red-50' : 'border-[color:var(--border-default)] bg-[color:var(--bg-panel)]'" />
                <input v-model.number="segment.step" type="number" class="w-full rounded-[var(--radius-sm)] border px-2 py-1 text-sm text-[color:var(--text-primary)]" :class="hasSegmentError(sectorAngularSegmentErrors, index, 'step') ? 'border-red-500 bg-red-50' : 'border-[color:var(--border-default)] bg-[color:var(--bg-panel)]'" />
                <UiButton size="sm" variant="danger" :disabled="sectorConfig.angularStepSegments.length === 1" @click="removeSectorAngularSegment(index)">{{ t.del }}</UiButton>
              </div>
              <div v-if="getSegmentError(sectorAngularSegmentErrors, index, 'start') || getSegmentError(sectorAngularSegmentErrors, index, 'end') || getSegmentError(sectorAngularSegmentErrors, index, 'step')" class="text-[8px] text-red-500 mt-0.5 pl-1">
                {{ getSegmentError(sectorAngularSegmentErrors, index, 'start') || getSegmentError(sectorAngularSegmentErrors, index, 'end') || getSegmentError(sectorAngularSegmentErrors, index, 'step') }}
              </div>
            </div>
          </div>
        </div>
      </div>
    </section>

    <!-- Custom pattern -->
    <section v-else class="p-3 rounded-[var(--radius-md)] border border-[color:var(--border-default)] bg-[color:var(--bg-panel)]">
      <div class="text-[10px] font-medium text-[color:var(--text-muted)] uppercase mb-2">{{ t.pointLayout }}</div>
      <div class="flex flex-wrap items-end gap-2">
        <div><label class="block text-[9px] text-[color:var(--text-muted)] mb-0.5">X</label><input v-model.number="customPointInput.x" type="number" class="w-20 rounded-[var(--radius-sm)] border border-[color:var(--border-default)] bg-[color:var(--bg-panel-strong)] px-2 py-1 text-sm text-[color:var(--text-primary)]" /></div>
        <div><label class="block text-[9px] text-[color:var(--text-muted)] mb-0.5">Y</label><input v-model.number="customPointInput.y" type="number" class="w-20 rounded-[var(--radius-sm)] border border-[color:var(--border-default)] bg-[color:var(--bg-panel-strong)] px-2 py-1 text-sm text-[color:var(--text-primary)]" /></div>
        <UiButton size="sm" variant="primary" @click="addCustomPoint">{{ t.addPoint }}</UiButton>
      </div>
      <div class="mt-3 space-y-1.5">
        <div v-for="(point, index) in customPoints" :key="index" class="p-2 rounded-[var(--radius-sm)] border border-[color:var(--border-default)] bg-[color:var(--bg-panel-strong)] flex items-center justify-between">
          <span class="font-mono text-xs text-[color:var(--text-primary)]">{{ t.point }} {{ index + 1 }}: {{ point.x }}, {{ point.y }}</span>
          <UiButton size="sm" variant="danger" @click="removeCustomPoint(index)">{{ t.remove }}</UiButton>
        </div>
      </div>
    </section>
  </div>
</template>


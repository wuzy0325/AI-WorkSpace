<script setup lang="ts">
import { computed } from 'vue'
import type { StepSegment, TraversalPattern } from '@shared/types/traversal'
import { useTraversalSegmentValidation, getSegmentError, hasSegmentError } from '@composables/useTraversalValidation'
import { NButton, NCard, NInput, NInputNumber, NSpace, NText } from 'naive-ui'

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
  rectangleConfig.value.xMin = xMin; rectangleConfig.value.xMax = xMax
  rectangleConfig.value.yMin = yMin; rectangleConfig.value.yMax = yMax
  return { xMin, xMax, yMin, yMax }
})

const computedSectorRange = computed(() => {
  const rs = sectorConfig.value.radialStepSegments
  const as = sectorConfig.value.angularStepSegments
  const radiusMin = rs.length > 0 ? Math.min(...rs.map(s => s.start)) : 0
  const radiusMax = rs.length > 0 ? Math.max(...rs.map(s => s.end)) : 0
  const angleStart = as.length > 0 ? Math.min(...as.map(s => s.start)) : 0
  const angleEnd = as.length > 0 ? Math.max(...as.map(s => s.end)) : 0
  sectorConfig.value.radiusMin = radiusMin; sectorConfig.value.radiusMax = radiusMax
  sectorConfig.value.angleStart = angleStart; sectorConfig.value.angleEnd = angleEnd
  return { radiusMin, radiusMax, angleStart, angleEnd }
})

const tRef = computed(() => props.t)

const patternLabel = computed(() => {
  switch (pattern.value) {
    case 'line': return tRef.value.patternLine
    case 'rectangle': return tRef.value.patternRectangle
    case 'sector': return tRef.value.patternSector
    default: return tRef.value.patternCustom
  }
})

const { errors: rxSegErrs, countError: rxSegCntErr } = useTraversalSegmentValidation(computed(() => rectangleConfig.value.xStepSegments), computed(() => rectangleConfig.value.xMin), computed(() => rectangleConfig.value.xMax), tRef)
const { errors: rySegErrs } = useTraversalSegmentValidation(computed(() => rectangleConfig.value.yStepSegments), computed(() => rectangleConfig.value.yMin), computed(() => rectangleConfig.value.yMax), tRef)
const { errors: srSegErrs } = useTraversalSegmentValidation(computed(() => sectorConfig.value.radialStepSegments), computed(() => sectorConfig.value.radiusMin), computed(() => sectorConfig.value.radiusMax), tRef)
const { errors: saSegErrs } = useTraversalSegmentValidation(computed(() => sectorConfig.value.angularStepSegments), computed(() => sectorConfig.value.angleStart), computed(() => sectorConfig.value.angleEnd), tRef)
const { errors: lxSegErrs } = useTraversalSegmentValidation(computed(() => lineConfig.value.xStepSegments), computed(() => lineConfig.value.startX), computed(() => lineConfig.value.endX), tRef)
const { errors: lySegErrs } = useTraversalSegmentValidation(computed(() => lineConfig.value.yStepSegments), computed(() => lineConfig.value.startY), computed(() => lineConfig.value.endY), tRef)

function addSegment() { lineConfig.value.xStepSegments.push({ start: -30, end: 30, step: 5 }) }
function removeSegment(i: number) { lineConfig.value.xStepSegments.splice(i, 1) }
function addRectangleXSegment() { const s = rectangleConfig.value.xStepSegments; if (s.length === 0) s.push({ start: 0, end: 10, step: 5 }); else { const l = s[s.length - 1]; s.push({ start: l.end, end: l.end + 10, step: 5 }) } }
function removeRectangleXSegment(i: number) { rectangleConfig.value.xStepSegments.splice(i, 1) }
function addRectangleYSegment() { const s = rectangleConfig.value.yStepSegments; if (s.length === 0) s.push({ start: 0, end: 10, step: 5 }); else { const l = s[s.length - 1]; s.push({ start: l.end, end: l.end + 10, step: 5 }) } }
function removeRectangleYSegment(i: number) { rectangleConfig.value.yStepSegments.splice(i, 1) }
function addSectorRadialSegment() { const s = sectorConfig.value.radialStepSegments; if (s.length === 0) s.push({ start: 100, end: 200, step: 50 }); else { const l = s[s.length - 1]; s.push({ start: l.end, end: l.end + 100, step: 50 }) } }
function removeSectorRadialSegment(i: number) { sectorConfig.value.radialStepSegments.splice(i, 1) }
function addSectorAngularSegment() { const s = sectorConfig.value.angularStepSegments; if (s.length === 0) s.push({ start: 0, end: 10, step: 5 }); else { const l = s[s.length - 1]; s.push({ start: l.end, end: l.end + 10, step: 5 }) } }
function removeSectorAngularSegment(i: number) { sectorConfig.value.angularStepSegments.splice(i, 1) }
function addCustomPoint() { customPoints.value.push({ x: customPointInput.value.x, y: customPointInput.value.y }); customPointInput.value = { x: 0, y: 0 } }
function removeCustomPoint(i: number) { customPoints.value.splice(i, 1) }
</script>

<template>
  <div class="step-content">
    <NCard size="small" :bordered="true" class="section-card">
      <div class="layout-basics">
        <div><NText depth="3" style="font-size:10px">{{ t.testNameLabel }}</NText><NInput v-model:value="testName" size="small" :placeholder="t.testNameLabel" /></div>
        <div><NText depth="3" style="font-size:10px">{{ t.dwellMsLabel }}</NText><NInputNumber v-model:value="dwellTimeMs" :min="100" :max="60000" size="small" style="width:100%" /></div>
        <div><NText depth="3" style="font-size:10px">{{ t.samplesLabel }}</NText><NInputNumber v-model:value="samplesPerPoint" :min="1" :max="1000" size="small" style="width:100%" /></div>
      </div>
    </NCard>

    <NSpace size="small">
      <NButton v-for="p in (['line', 'rectangle', 'sector', 'custom'] as const)" :key="p" size="tiny" :type="pattern === p ? 'primary' : 'default'" secondary @click="pattern = p">{{ patternLabel }}</NButton>
    </NSpace>

    <NCard v-if="pattern === 'line'" size="small" :bordered="true" class="section-card">
      <div class="seg-grid">
        <div class="seg-side">
          <NText depth="3" style="font-size:10px;font-weight:500">{{ t.pointLayout }}</NText>
          <div class="seg-pts">
            <div><NText depth="3" style="font-size:9px">{{ t.startX }}</NText><NInputNumber v-model:value="lineConfig.startX" size="tiny" style="width:100%" /></div>
            <div><NText depth="3" style="font-size:9px">{{ t.startY }}</NText><NInputNumber v-model:value="lineConfig.startY" size="tiny" style="width:100%" /></div>
            <div><NText depth="3" style="font-size:9px">{{ t.endX }}</NText><NInputNumber v-model:value="lineConfig.endX" size="tiny" style="width:100%" /></div>
            <div><NText depth="3" style="font-size:9px">{{ t.endY }}</NText><NInputNumber v-model:value="lineConfig.endY" size="tiny" style="width:100%" /></div>
          </div>
        </div>
        <div class="seg-list">
          <div class="seg-header"><NText depth="3" style="font-size:10px;text-transform:uppercase">{{ t.xSegments }}</NText><NButton size="tiny" secondary @click="addSegment">{{ t.addSegment }}</NButton></div>
          <div class="seg-labels"><NText depth="3" style="font-size:9px;flex:1">{{ t.start }}</NText><NText depth="3" style="font-size:9px;flex:1">{{ t.end }}</NText><NText depth="3" style="font-size:9px;flex:1">{{ t.step }}</NText><div style="width:40px"></div></div>
          <div v-for="(s, i) in lineConfig.xStepSegments" :key="i" class="seg-row">
            <NInputNumber v-model:value="s.start" size="tiny" style="flex:1" :status="hasSegmentError(lxSegErrs, i, 'start') ? 'error' : undefined" />
            <NInputNumber v-model:value="s.end" size="tiny" style="flex:1" :status="hasSegmentError(lxSegErrs, i, 'end') ? 'error' : undefined" />
            <NInputNumber v-model:value="s.step" size="tiny" style="flex:1" :status="hasSegmentError(lxSegErrs, i, 'step') ? 'error' : undefined" />
            <NButton size="tiny" secondary :disabled="lineConfig.xStepSegments.length === 1" @click="removeSegment(i)">{{ t.del }}</NButton>
          </div>
        </div>
      </div>
    </NCard>

    <NCard v-else-if="pattern === 'rectangle'" size="small" :bordered="true" class="section-card">
      <NText depth="3" style="font-size:10px;font-weight:500;display:block;margin-bottom:8px">{{ t.pointLayout }} (X: {{ computedRectangleRange.xMin }}..{{ computedRectangleRange.xMax }}, Y: {{ computedRectangleRange.yMin }}..{{ computedRectangleRange.yMax }})</NText>
      <div class="seg-col-list">
        <div class="seg-list"><div class="seg-header"><NText depth="3" style="font-size:10px;text-transform:uppercase">X {{ t.xSegments }}</NText><NButton size="tiny" secondary @click="addRectangleXSegment">{{ t.addSegment }}</NButton></div>
          <div class="seg-labels"><NText depth="3" style="font-size:9px;flex:1">{{ t.start }}</NText><NText depth="3" style="font-size:9px;flex:1">{{ t.end }}</NText><NText depth="3" style="font-size:9px;flex:1">{{ t.step }}</NText><div style="width:40px"></div></div>
          <div v-for="(s, i) in rectangleConfig.xStepSegments" :key="i" class="seg-row"><NInputNumber v-model:value="s.start" size="tiny" style="flex:1" :status="hasSegmentError(rxSegErrs, i, 'start') ? 'error' : undefined" /><NInputNumber v-model:value="s.end" size="tiny" style="flex:1" :status="hasSegmentError(rxSegErrs, i, 'end') ? 'error' : undefined" /><NInputNumber v-model:value="s.step" size="tiny" style="flex:1" :status="hasSegmentError(rxSegErrs, i, 'step') ? 'error' : undefined" /><NButton size="tiny" secondary :disabled="rectangleConfig.xStepSegments.length === 1" @click="removeRectangleXSegment(i)">{{ t.del }}</NButton></div>
        </div>
        <div class="seg-list"><div class="seg-header"><NText depth="3" style="font-size:10px;text-transform:uppercase">Y {{ t.ySegments }}</NText><NButton size="tiny" secondary @click="addRectangleYSegment">{{ t.addSegment }}</NButton></div>
          <div class="seg-labels"><NText depth="3" style="font-size:9px;flex:1">{{ t.start }}</NText><NText depth="3" style="font-size:9px;flex:1">{{ t.end }}</NText><NText depth="3" style="font-size:9px;flex:1">{{ t.step }}</NText><div style="width:40px"></div></div>
          <div v-for="(s, i) in rectangleConfig.yStepSegments" :key="i" class="seg-row"><NInputNumber v-model:value="s.start" size="tiny" style="flex:1" :status="hasSegmentError(rySegErrs, i, 'start') ? 'error' : undefined" /><NInputNumber v-model:value="s.end" size="tiny" style="flex:1" :status="hasSegmentError(rySegErrs, i, 'end') ? 'error' : undefined" /><NInputNumber v-model:value="s.step" size="tiny" style="flex:1" :status="hasSegmentError(rySegErrs, i, 'step') ? 'error' : undefined" /><NButton size="tiny" secondary :disabled="rectangleConfig.yStepSegments.length === 1" @click="removeRectangleYSegment(i)">{{ t.del }}</NButton></div>
        </div>
      </div>
    </NCard>

    <NCard v-else-if="pattern === 'sector'" size="small" :bordered="true" class="section-card">
      <div class="seg-grid">
        <div class="seg-side">
          <NText depth="3" style="font-size:10px;font-weight:500">{{ t.pointLayout }}</NText>
          <div class="seg-pts">
            <div><NText depth="3" style="font-size:9px">{{ t.centerX }}</NText><NInputNumber v-model:value="sectorConfig.centerX" size="tiny" style="width:100%" /></div>
            <div><NText depth="3" style="font-size:9px">{{ t.centerY }}</NText><NInputNumber v-model:value="sectorConfig.centerY" size="tiny" style="width:100%" /></div>
            <div><NText depth="3" style="font-size:9px">{{ t.radiusMin }}</NText><NInputNumber v-model:value="sectorConfig.radiusMin" size="tiny" style="width:100%" /></div>
            <div><NText depth="3" style="font-size:9px">{{ t.radiusMax }}</NText><NInputNumber v-model:value="sectorConfig.radiusMax" size="tiny" style="width:100%" /></div>
            <div><NText depth="3" style="font-size:9px">{{ t.angleStart }}</NText><NInputNumber v-model:value="sectorConfig.angleStart" size="tiny" style="width:100%" /></div>
            <div><NText depth="3" style="font-size:9px">{{ t.angleEnd }}</NText><NInputNumber v-model:value="sectorConfig.angleEnd" size="tiny" style="width:100%" /></div>
          </div>
        </div>
        <div class="seg-col-list">
          <div class="seg-list"><div class="seg-header"><NText depth="3" style="font-size:10px;text-transform:uppercase">{{ t.radiusSegments }}</NText><NButton size="tiny" secondary @click="addSectorRadialSegment">{{ t.addSegment }}</NButton></div>
            <div class="seg-labels"><NText depth="3" style="font-size:9px;flex:1">{{ t.start }}</NText><NText depth="3" style="font-size:9px;flex:1">{{ t.end }}</NText><NText depth="3" style="font-size:9px;flex:1">{{ t.step }}</NText><div style="width:40px"></div></div>
            <div v-for="(s, i) in sectorConfig.radialStepSegments" :key="i" class="seg-row"><NInputNumber v-model:value="s.start" size="tiny" style="flex:1" :status="hasSegmentError(srSegErrs, i, 'start') ? 'error' : undefined" /><NInputNumber v-model:value="s.end" size="tiny" style="flex:1" :status="hasSegmentError(srSegErrs, i, 'end') ? 'error' : undefined" /><NInputNumber v-model:value="s.step" size="tiny" style="flex:1" :status="hasSegmentError(srSegErrs, i, 'step') ? 'error' : undefined" /><NButton size="tiny" secondary :disabled="sectorConfig.radialStepSegments.length === 1" @click="removeSectorRadialSegment(i)">{{ t.del }}</NButton></div>
          </div>
          <div class="seg-list"><div class="seg-header"><NText depth="3" style="font-size:10px;text-transform:uppercase">{{ t.angleSegments }}</NText><NButton size="tiny" secondary @click="addSectorAngularSegment">{{ t.addSegment }}</NButton></div>
            <div class="seg-labels"><NText depth="3" style="font-size:9px;flex:1">{{ t.start }}</NText><NText depth="3" style="font-size:9px;flex:1">{{ t.end }}</NText><NText depth="3" style="font-size:9px;flex:1">{{ t.step }}</NText><div style="width:40px"></div></div>
            <div v-for="(s, i) in sectorConfig.angularStepSegments" :key="i" class="seg-row"><NInputNumber v-model:value="s.start" size="tiny" style="flex:1" :status="hasSegmentError(saSegErrs, i, 'start') ? 'error' : undefined" /><NInputNumber v-model:value="s.end" size="tiny" style="flex:1" :status="hasSegmentError(saSegErrs, i, 'end') ? 'error' : undefined" /><NInputNumber v-model:value="s.step" size="tiny" style="flex:1" :status="hasSegmentError(saSegErrs, i, 'step') ? 'error' : undefined" /><NButton size="tiny" secondary :disabled="sectorConfig.angularStepSegments.length === 1" @click="removeSectorAngularSegment(i)">{{ t.del }}</NButton></div>
          </div>
        </div>
      </div>
    </NCard>

    <NCard v-else size="small" :bordered="true" class="section-card">
      <NText depth="3" style="font-size:10px;font-weight:500;display:block;margin-bottom:8px">{{ t.pointLayout }}</NText>
      <NSpace size="small" align="flex-end">
        <div><NText depth="3" style="font-size:9px">X</NText><NInputNumber v-model:value="customPointInput.x" size="tiny" style="width:80px" /></div>
        <div><NText depth="3" style="font-size:9px">Y</NText><NInputNumber v-model:value="customPointInput.y" size="tiny" style="width:80px" /></div>
        <NButton size="tiny" type="primary" @click="addCustomPoint">{{ t.addPoint }}</NButton>
      </NSpace>
      <div v-if="customPoints.length > 0" class="pt-list">
        <div v-for="(pt, i) in customPoints" :key="i" class="pt-row">
          <NText depth="1" style="font-size:11px">{{ t.point }} {{ i + 1 }}: {{ pt.x }}, {{ pt.y }}</NText>
          <NButton size="tiny" secondary @click="removeCustomPoint(i)">{{ t.remove }}</NButton>
        </div>
      </div>
    </NCard>
  </div>
</template>

<style scoped>
.step-content { display:flex; flex-direction:column; gap:12px; }
.section-card { font-size:12px; }
.layout-basics { display:grid; grid-template-columns:1fr 1fr 1fr; gap:8px; }
.seg-grid { display:grid; grid-template-columns:180px 1fr; gap:10px; }
.seg-side { padding:8px; border-radius:4px; border:1px solid var(--border-default); background:var(--bg-panel-strong); }
.seg-pts { display:grid; grid-template-columns:1fr 1fr; gap:6px; margin-top:6px; }
.seg-list { padding:8px; border-radius:4px; border:1px solid var(--border-default); background:var(--bg-panel-strong); }
.seg-col-list { display:flex; flex-direction:column; gap:8px; }
.seg-header { display:flex; align-items:center; justify-content:space-between; margin-bottom:6px; }
.seg-labels { display:flex; gap:6px; padding:0 2px 4px; }
.seg-row { display:flex; gap:6px; align-items:center; margin-bottom:6px; }
.pt-list { display:flex; flex-direction:column; gap:6px; margin-top:8px; }
.pt-row { display:flex; align-items:center; justify-content:space-between; padding:6px 8px; border-radius:4px; border:1px solid var(--border-default); background:var(--bg-panel-strong); }
</style>
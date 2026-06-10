<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import UiButton from '@components/ui/UiButton.vue'
import { useDeviceStore } from '@stores/deviceStore'
import { useFeedbackStore } from '@stores/feedbackStore'
import { useI18nStore } from '@stores/i18nStore'
import { useMotionStore } from '@stores/motionStore'
import { useStorageStore } from '@stores/storageStore'
import { useTraversalStore } from '@stores/traversalStore'
import {
  createDefaultTraversalProbeChannels,
  getTraversalLayoutPointCount,
  isTraversalConfigurableProbeChannel,
  isTraversalRequiredProbeChannel
} from '@shared/types/traversal'
import type {
  MultiPrbInterpolationMode,
  PrbFileInfo,
  ProbeChannelConfig,
  StepSegment,
  TraversalLayout,
  TraversalMotionAxisConfig,
  TraversalPattern,
  TraversalTestConfig,
  CalibrationCsvFileInfo,
  InterpolationAlgorithm
} from '@shared/types/traversal'
import UiPanel from '@components/ui/UiPanel.vue'
import UiCheckbox from '@components/ui/UiCheckbox.vue'
import UiInput from '@components/ui/UiInput.vue'
import UiInputNumber from '@components/ui/UiInputNumber.vue'
import UiDialog from '@components/ui/UiDialog.vue'
import UiStep from '@components/ui/UiStep.vue'
import UiSteps from '@components/ui/UiSteps.vue'
import PointsPreview from './PointsPreview.vue'
import TraversalLayoutStep from './TraversalLayoutStep.vue'
import TraversalHardwareStep from './TraversalHardwareStep.vue'
import TraversalPrbStep from './TraversalPrbStep.vue'

const emit = defineEmits<{
  close: []
  saved: [config: TraversalTestConfig]
}>()

const deviceStore = useDeviceStore()
const i18n = useI18nStore()
const motionStore = useMotionStore()
const storageStore = useStorageStore()
const traversalStore = useTraversalStore()
const feedbackStore = useFeedbackStore()

const t = computed(() => i18n.t)

const INVALID_FILE_NAME_CHARS = /[<>:"/\\|?*\u0000-\u001F]+/g
const WINDOWS_RESERVED_FILE_NAMES = new Set([
  'CON', 'PRN', 'AUX', 'NUL',
  'COM1','COM2','COM3','COM4','COM5','COM6','COM7','COM8','COM9',
  'LPT1','LPT2','LPT3','LPT4','LPT5','LPT6','LPT7','LPT8','LPT9'
])

const isLoading = ref(true)
const isSaving = ref(false)
const currentStep = ref(0)
const steps = computed(() => [t.value.stepLayout, t.value.stepHardware, t.value.stepPrb, t.value.stepReview])

const testName = ref(`Traversal-${new Date().toLocaleDateString()}`)
const dwellTimeMs = ref(2000)
const samplesPerPoint = ref(10)
const pattern = ref<TraversalPattern>('line')
const prbMode = ref<'single' | 'multi'>('single')
const interpolationAlgorithm = ref<InterpolationAlgorithm>('old')
const calibrationCsvFile = ref<CalibrationCsvFileInfo | null>(null)
const prbFile = ref<PrbFileInfo | null>(null)
const multiPrbFiles = ref<PrbFileInfo[]>([])
const multiPrbMachNumbers = ref<number[]>([])
const multiPrbInterpolationMode = ref<MultiPrbInterpolationMode>('linear')
const savePath = ref('')
const saveFileName = ref(buildDefaultSaveFileName(testName.value))

const lineConfig = ref({
  startX: -30, startY: 0, endX: 30, endY: 0,
  xStepSegments: [{ start: -30, end: 30, step: 5 }],
  yStepSegments: [] as StepSegment[]
})

const rectangleConfig = ref({
  xMin: -30, xMax: 30,
  xStepSegments: [{ start: -30, end: 30, step: 5 }],
  yMin: -30, yMax: 30,
  yStepSegments: [{ start: -30, end: 30, step: 5 }]
})

const sectorConfig = ref({
  centerX: 0, centerY: 0,
  radiusMin: 100, radiusMax: 300,
  radialStepSegments: [{ start: 100, end: 300, step: 50 }],
  angleStart: -30, angleEnd: 30,
  angularStepSegments: [{ start: -30, end: 30, step: 5 }]
})

const customPoints = ref<Array<{ x: number; y: number }>>([])
const customPointInput = ref({ x: 0, y: 0 })
const probeChannels = ref<ProbeChannelConfig[]>(createDefaultTraversalProbeChannels())
const motionAxes = ref<TraversalMotionAxisConfig[]>([
  { name: 'X', controllerId: '', axis: 'X', angleMapping: { type: 'alpha' } },
  { name: 'Y', controllerId: '', axis: 'Y', angleMapping: { type: 'beta' } }
])
const saveOptions = ref<TraversalTestConfig['saveOptions']>({
  savePointId: true, saveTimestamp: true, saveRawPressure: true, saveCalculatedResult: true
})

const currentLayout = computed<TraversalLayout>(() => {
  switch (pattern.value) {
    case 'line': return { pattern: 'line', line: lineConfig.value }
    case 'rectangle': return { pattern: 'rectangle', rectangle: rectangleConfig.value }
    case 'sector': return { pattern: 'sector', sector: sectorConfig.value }
    case 'custom': return { pattern: 'custom', custom: { points: customPoints.value } }
  }
})

const estimatedPointCount = computed(() => getTraversalLayoutPointCount(currentLayout.value))

const isStepValid = computed(() => {
  if (currentStep.value === 0) return testName.value.trim() !== '' && estimatedPointCount.value > 0
  if (currentStep.value === 1) {
    return probeChannels.value.filter((c) => c.enabled).every((c) => c.channel.deviceId !== '' && c.channel.channelIndex >= 0) &&
      motionAxes.value.every((a) => a.controllerId !== '')
  }
  if (currentStep.value === 2) {
    if (interpolationAlgorithm.value === 'new') return calibrationCsvFile.value !== null
    if (prbMode.value === 'multi') return multiPrbFiles.value.length > 0
    return prbFile.value !== null
  }
  if (currentStep.value === steps.value.length - 1) return savePath.value.trim() !== '' && saveFileName.value.trim() !== ''
  return true
})

watch(testName, (next, prev) => {
  const prevDefault = buildDefaultSaveFileName(prev)
  if (saveFileName.value === '' || saveFileName.value === prevDefault)
    saveFileName.value = buildDefaultSaveFileName(next)
})

function buildDefaultSaveFileName(name: string) { return sanitizeCsvFileName(name, 'Traversal') }
function sanitizeFileNameStem(value: string, fallback: string): string {
  const normFallback = fallback.trim().replace(INVALID_FILE_NAME_CHARS, '-').replace(/\s+/g, ' ').replace(/-+/g, '-').trim().replace(/^[.\s-]+/, '').replace(/[.\s-]+$/, '') || 'Traversal'
  const stem = value.trim().replace(/\.csv$/i, '').replace(INVALID_FILE_NAME_CHARS, '-').replace(/\s+/g, ' ').replace(/-+/g, '-').trim().replace(/^[.\s-]+/, '').replace(/[.\s-]+$/, '') || normFallback
  return WINDOWS_RESERVED_FILE_NAMES.has(stem.toUpperCase()) ? `${stem}-file` : stem
}
function sanitizeCsvFileName(fileName: string, fallback: string) { return `${sanitizeFileNameStem(fileName, fallback)}.csv` }
function normalizeSaveFileName(fileName: string) { return sanitizeCsvFileName(fileName, testName.value) }
function isRequiredProbeChannel(c: ProbeChannelConfig) { return isTraversalRequiredProbeChannel(c.role, c.name) }
function normalizeProbeChannel(c: ProbeChannelConfig) { return { ...c, channel: { ...c.channel }, enabled: isRequiredProbeChannel(c) ? true : c.enabled } }
function clonePrbFileInfo(f: PrbFileInfo) { return { ...f, validRange: { ...f.validRange } } }
function normalizeMultiPrbMachNumbers(files: PrbFileInfo[], nums: number[] = []) { return files.map((f, i) => { const v = nums[i] ?? f.machNumber ?? f.validRange.machMin; return Number.isFinite(v) ? v : 0 }) }
function isConfigurableProbeChannel(c: ProbeChannelConfig) { return isTraversalConfigurableProbeChannel(c.role, c.name) }
function nextStep() { if (currentStep.value < steps.value.length - 1 && isStepValid.value) currentStep.value++ }
function prevStep() { if (currentStep.value > 0) currentStep.value-- }

function applySavedLayout(layout: TraversalLayout) {
  pattern.value = layout.pattern
  if (layout.line) lineConfig.value = { ...layout.line, xStepSegments: layout.line.xStepSegments.map(s => ({ ...s })), yStepSegments: layout.line.yStepSegments.map(s => ({ ...s })) }
  if (layout.rectangle) rectangleConfig.value = { ...layout.rectangle, xStepSegments: layout.rectangle.xStepSegments.map(s => ({ ...s })), yStepSegments: layout.rectangle.yStepSegments.map(s => ({ ...s })) }
  if (layout.sector) sectorConfig.value = { ...layout.sector, radialStepSegments: layout.sector.radialStepSegments.map(s => ({ ...s })), angularStepSegments: layout.sector.angularStepSegments.map(s => ({ ...s })) }
  customPoints.value = layout.custom?.points.map(p => ({ ...p })) ?? []
}

function applySavedConfig(config: TraversalTestConfig) {
  const c = JSON.parse(JSON.stringify(config)) as TraversalTestConfig
  testName.value = c.name; dwellTimeMs.value = c.dwellTimeMs; samplesPerPoint.value = c.samplesPerPoint
  savePath.value = c.savePath; saveFileName.value = sanitizeCsvFileName(c.saveFileName, c.name)
  const useMulti = Boolean((c.useMultiPrb ?? false) && c.multiPrb?.files.length)
  prbMode.value = useMulti ? 'multi' : 'single'
  prbFile.value = useMulti ? null : c.prbFile ? clonePrbFileInfo(c.prbFile) : null
  multiPrbFiles.value = useMulti ? (c.multiPrb?.files ?? []).map(f => clonePrbFileInfo(f)) : []
  multiPrbMachNumbers.value = useMulti ? normalizeMultiPrbMachNumbers(multiPrbFiles.value, c.multiPrb?.machNumbers) : []
  multiPrbInterpolationMode.value = c.multiPrb?.interpolationMode ?? 'linear'
  saveOptions.value = { ...saveOptions.value, ...c.saveOptions, customFields: c.saveOptions.customFields ? { ...c.saveOptions.customFields } : undefined }
  applySavedLayout(c.layout)
  if (c.channels.probeChannels.length > 0) {
    const next = [...probeChannels.value]
    for (const sc of c.channels.probeChannels) {
      if (!isConfigurableProbeChannel(sc)) continue
      const idx = next.findIndex(x => x.role ? x.role === sc.role : x.name === sc.name)
      if (idx >= 0) next[idx] = normalizeProbeChannel({ ...next[idx], ...sc, channel: { ...sc.channel } })
      else next.push(normalizeProbeChannel({ ...sc, channel: { ...sc.channel } }))
    }
    probeChannels.value = next
  }
  if (c.channels.motionAxes.length > 0) {
    const next = [...motionAxes.value]
    for (const sa of c.channels.motionAxes) {
      const idx = next.findIndex(x => x.name === sa.name)
      const normalized = { ...sa, angleMapping: sa.angleMapping ? { ...sa.angleMapping } : undefined }
      if (idx >= 0) next[idx] = normalized; else next.push(normalized)
    }
    motionAxes.value = next
  }
  interpolationAlgorithm.value = c.interpolationAlgorithm ?? 'old'
  calibrationCsvFile.value = c.calibrationCsvFile ? { ...c.calibrationCsvFile } : null
}

async function loadSavedConfig() {
  await traversalStore.loadConfig()
  if (traversalStore.config) applySavedConfig(traversalStore.config)
}

async function pickSavePath() {
  try {
    const p = await storageStore.pickDirectory()
    if (p) savePath.value = p
  } catch (e) {
    feedbackStore.pushToast('Failed to choose directory: ' + (e instanceof Error ? e.message : String(e)), 'error')
  }
}

async function saveConfig() {
  if (traversalStore.isRunning) {
    const ok = await feedbackStore.confirm(t.value.saveConfigWhileRunning, { title: t.value.statusRunning, confirmText: t.value.save, cancelText: t.value.cancel })
    if (!ok) return
  }
  isSaving.value = true
  try {
    const normName = normalizeSaveFileName(saveFileName.value)
    saveFileName.value = normName
    const useMulti = prbMode.value === 'multi' && multiPrbFiles.value.length > 0
    const raw = {
      name: testName.value, layout: currentLayout.value,
      channels: { probeChannels: probeChannels.value.filter(c => c.enabled && isConfigurableProbeChannel(c)), motionAxes: motionAxes.value },
      prbFile: useMulti ? null : prbFile.value,
      multiPrb: useMulti ? { files: multiPrbFiles.value.map(f => clonePrbFileInfo(f)), machNumbers: multiPrbMachNumbers.value.map(n => Number(n)), interpolationMode: multiPrbInterpolationMode.value } : undefined,
      useMultiPrb: useMulti, interpolationAlgorithm: interpolationAlgorithm.value,
      calibrationCsvFile: interpolationAlgorithm.value === 'new' ? calibrationCsvFile.value : null,
      dwellTimeMs: dwellTimeMs.value, samplesPerPoint: samplesPerPoint.value,
      savePath: savePath.value.trim(), saveFileName: normName, saveOptions: saveOptions.value
    }
    const config: TraversalTestConfig = JSON.parse(JSON.stringify(raw))
    const ok = await traversalStore.saveConfig(config)
    if (!ok) throw new Error(traversalStore.error || t.value.failedSaveConfig)
    emit('saved', config); emit('close')
  } catch (e) {
    feedbackStore.pushToast(t.value.failedSaveConfig + ': ' + (e instanceof Error ? e.message : String(e)), 'error')
  } finally { isSaving.value = false }
}

onMounted(async () => {
  try {
    await Promise.all([deviceStore.refreshProfiles(), motionStore.refreshProfiles(), storageStore.loadSettings(), loadSavedConfig()])
    if (!savePath.value.trim()) savePath.value = storageStore.settings?.baseDirectory?.trim() ?? ''
    if (!saveFileName.value.trim()) saveFileName.value = buildDefaultSaveFileName(testName.value)
  } finally { isLoading.value = false }
})
</script>

<template>
  <UiDialog :show="true" width="960px" closable @close="emit('close')">
    <template #header>
      <div>
          <span class="setup-overline">{{ t.traversalSetup }}</span>
          <span class="setup-title">{{ t.traversalWorkbenchConfig }}</span>
      </div>
    </template>

    <UiSteps :current="currentStep" class="steps-nav">
      <UiStep v-for="(step, idx) in steps" :key="idx" :title="step" :disabled="idx > currentStep" />
    </UiSteps>

    <div class="traversal-body">
      <div class="traversal-main">
        <TraversalLayoutStep
          v-if="currentStep === 0"
          v-model:test-name="testName"
          v-model:dwell-time-ms="dwellTimeMs"
          v-model:samples-per-point="samplesPerPoint"
          v-model:pattern="pattern"
          v-model:line-config="lineConfig"
          v-model:rectangle-config="rectangleConfig"
          v-model:sector-config="sectorConfig"
          v-model:custom-points="customPoints"
          v-model:custom-point-input="customPointInput"
          :estimated-point-count="estimatedPointCount"
          :t="(t as unknown as Record<string, string>)"
        />
        <TraversalHardwareStep
          v-else-if="currentStep === 1"
          v-model:probe-channels="probeChannels"
          v-model:motion-axes="motionAxes"
          :t="(t as unknown as Record<string, string>)"
          :is-loading="isLoading"
        />
        <TraversalPrbStep
          v-else-if="currentStep === 2"
          v-model:prb-mode="prbMode"
          v-model:interpolation-algorithm="interpolationAlgorithm"
          v-model:prb-file="prbFile"
          v-model:multi-prb-files="multiPrbFiles"
          v-model:multi-prb-mach-numbers="multiPrbMachNumbers"
          v-model:multi-prb-interpolation-mode="multiPrbInterpolationMode"
          v-model:calibration-csv-file="calibrationCsvFile"
          :t="(t as unknown as Record<string, string>)"
        />
        <div v-else class="step-content">
          <UiPanel class="section-card">
            <template #header><span class="summary-section-title">配置摘要</span></template>
            <div class="summary-grid">
              <div class="summary-row"><span style="color:var(--text-tertiary)">{{ t.name }}</span><span>{{ testName }}</span></div>
              <div class="summary-row"><span style="color:var(--text-tertiary)">{{ t.pattern }}</span><span>{{ pattern }}</span></div>
              <div class="summary-row"><span style="color:var(--text-tertiary)">{{ t.estimatedPoints }}</span><span class="summary-accent">{{ estimatedPointCount }}</span></div>
              <div class="summary-row"><span style="color:var(--text-tertiary)">{{ (t as Record<string, string>).interpolationAlgorithm || 'Algorithm' }}</span><span>{{ interpolationAlgorithm === 'new' ? ((t as Record<string, string>).algorithmNew || 'New') : ((t as Record<string, string>).algorithmOld || 'Old') }}</span></div>
              <div class="summary-row"><span style="color:var(--text-tertiary)">{{ t.prb }}</span><span class="text-ellipsis">{{ interpolationAlgorithm === 'new' ? (calibrationCsvFile ? calibrationCsvFile.fileName : t.none) : (prbFile ? prbFile.fileName : t.none) }}</span></div>
            </div>
          </UiPanel>

          <UiPanel class="section-card">
            <div class="save-row">
              <UiInput v-model="savePath" placeholder="Output directory" size="small" class="flex-input" />
              <UiInput v-model="saveFileName" placeholder="CSV file name" size="small" class="flex-input" />
              <UiButton size="sm" @click="pickSavePath">Browse</UiButton>
            </div>
            <span class="save-hint">Choose a real output directory for traversal CSV exports.</span>
          </UiPanel>

          <UiPanel class="section-card">
            <div class="save-options">
              <label class="option-label"><UiCheckbox v-model:checked="saveOptions.savePointId" size="small" />{{ t.savePointId }}</label>
              <label class="option-label"><UiCheckbox v-model:checked="saveOptions.saveTimestamp" size="small" />{{ t.saveTimestamp }}</label>
              <label class="option-label"><UiCheckbox v-model:checked="saveOptions.saveRawPressure" size="small" />{{ t.saveRawPressure }}</label>
              <label class="option-label"><UiCheckbox v-model:checked="saveOptions.saveCalculatedResult" size="small" />{{ t.saveCalculatedResult }}</label>
            </div>
          </UiPanel>
        </div>
      </div>

      <aside class="traversal-sidebar">
        <div class="sidebar-stats">
          <div class="sidebar-stat"><span class="stat-label">{{ t.points }}</span><span class="stat-number">{{ estimatedPointCount }}</span></div>
          <div class="sidebar-stat"><span class="stat-label">{{ t.dwell }}</span><span class="stat-value">{{ dwellTimeMs }} <span class="stat-label">ms</span></span></div>
          <div class="sidebar-stat"><span class="stat-label">{{ t.samples }}</span><span class="stat-value">{{ samplesPerPoint }}</span></div>
        </div>
        <div class="sidebar-preview">
          <PointsPreview :layout="currentLayout" />
        </div>
      </aside>
    </div>

    <template #footer>
      <div class="footer-actions">
        <div>
          <UiButton v-if="currentStep > 0" variant="secondary" size="sm" @click="prevStep">{{ t.previous }}</UiButton>
        </div>
        <div class="footer-right">
          <UiButton variant="secondary" size="sm" @click="emit('close')">{{ t.cancel }}</UiButton>
          <UiButton v-if="currentStep < steps.length - 1" variant="primary" size="sm" :disabled="!isStepValid" @click="nextStep">{{ t.next }}</UiButton>
          <UiButton v-else size="sm" variant="primary" :loading="isSaving" :disabled="!isStepValid" @click="saveConfig">{{ isSaving ? t.saving : t.saveConfig }}</UiButton>
        </div>
      </div>
    </template>
  </UiDialog>
</template>

<style scoped>
.traversal-body {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 340px;
  gap: 0;
  min-height: 0;
  flex: 1
}

.traversal-main {
  min-height: 0;
  overflow-y: auto;
  padding-right: var(--space-4)
}

.step-content {
  display: flex;
  flex-direction: column;
  gap: var(--space-3)
}

.section-card {
  font-size: var(--text-sm)
}

.summary-grid {
  display: flex;
  flex-direction: column
}

.summary-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 7px 0;
  border-bottom: 1px solid var(--border-default);
  gap: var(--space-3);
  font-size: var(--text-sm)
}

.summary-row:last-child {
  border-bottom: none
}

.save-row {
  display: flex;
  gap: 6px
}

.save-options {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--space-1)
}

.option-label {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 5px var(--space-2);
  border-radius: var(--radius-md);
  font-size: var(--text-sm);
  color: var(--text-primary);
  cursor: pointer;
  transition: background 120ms ease
}

.option-label:hover {
  background: var(--bg-panel-strong)
}

.traversal-sidebar {
  border-left: 1px solid var(--border-default);
  background: var(--bg-panel-strong);
  padding-left: var(--space-4);
  display: flex;
  flex-direction: column;
  gap: var(--space-3)
}

.sidebar-stats {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: var(--space-2)
}

.sidebar-stat {
  padding: 10px;
  border-radius: var(--radius-md);
  border: 1px solid var(--border-default);
  background: var(--bg-panel);
  text-align: center
}

.sidebar-preview {
  flex: 1;
  min-height: 280px;
  border-radius: var(--radius-md);
  border: 1px solid var(--border-default);
  background: var(--bg-canvas);
  overflow: hidden
}

.footer-actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%
}

.footer-right {
  display: flex;
  align-items: center;
  gap: var(--space-2)
}

.setup-overline { font-size:var(--text-xs);font-weight:600;text-transform:uppercase;letter-spacing:0.2em;color:var(--text-tertiary) }
.setup-title { font-size:18px;font-weight:600;margin-top:2px;display:block }
.steps-nav { margin-bottom:var(--space-4) }
.summary-section-title { font-size:var(--text-sm);font-weight:600 }
.summary-accent { color:var(--color-accent);font-weight:700 }
.text-ellipsis { max-width:180px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap }
.flex-input { flex:1 }
.save-hint { font-size:var(--text-xs);display:block;margin-top:6px;color:var(--text-tertiary) }
.stat-label { font-size:var(--text-xs);color:var(--text-tertiary) }
.stat-number { font-size:20px;font-weight:700;color:var(--color-accent) }
.stat-value { font-size:13px;font-weight:600 }
.stat-unit { font-size:var(--text-xs) }
</style>
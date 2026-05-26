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
  'CON',
  'PRN',
  'AUX',
  'NUL',
  'COM1',
  'COM2',
  'COM3',
  'COM4',
  'COM5',
  'COM6',
  'COM7',
  'COM8',
  'COM9',
  'LPT1',
  'LPT2',
  'LPT3',
  'LPT4',
  'LPT5',
  'LPT6',
  'LPT7',
  'LPT8',
  'LPT9'
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
  startX: -30,
  startY: 0,
  endX: 30,
  endY: 0,
  xStepSegments: [{ start: -30, end: 30, step: 5 }],
  yStepSegments: [] as StepSegment[]
})

const rectangleConfig = ref({
  xMin: -30,
  xMax: 30,
  xStepSegments: [{ start: -30, end: 30, step: 5 }],
  yMin: -30,
  yMax: 30,
  yStepSegments: [{ start: -30, end: 30, step: 5 }]
})

const sectorConfig = ref({
  centerX: 0,
  centerY: 0,
  radiusMin: 100,
  radiusMax: 300,
  radialStepSegments: [{ start: 100, end: 300, step: 50 }],
  angleStart: -30,
  angleEnd: 30,
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
  savePointId: true,
  saveTimestamp: true,
  saveRawPressure: true,
  saveCalculatedResult: true
})

const fieldClass =
  'w-full rounded-[var(--radius-md)] border border-[color:var(--border-default)] bg-[color:var(--bg-panel)] px-3 py-2 text-sm text-[color:var(--text-primary)] transition-colors hover:border-[color:var(--border-strong)] focus:border-[color:var(--accent-primary)] focus:outline-none focus:ring-2 focus:ring-[color:var(--focus-ring-soft)] disabled:cursor-not-allowed disabled:opacity-50'

const cardClass = 'ui-panel-surface p-4'
const subCardClass = 'rounded-[var(--radius-sm)] border border-[color:var(--border-default)] bg-[color:var(--bg-panel-strong)] p-3'
const checkboxClass = 'h-4 w-4 rounded border-[color:var(--border-default)] text-[color:var(--accent-primary)] focus:ring-[color:var(--focus-ring-soft)]'

const currentLayout = computed<TraversalLayout>(() => {
  switch (pattern.value) {
    case 'line':
      return { pattern: 'line', line: lineConfig.value }
    case 'rectangle':
      return { pattern: 'rectangle', rectangle: rectangleConfig.value }
    case 'sector':
      return { pattern: 'sector', sector: sectorConfig.value }
    case 'custom':
      return { pattern: 'custom', custom: { points: customPoints.value } }
  }
})

const estimatedPointCount = computed(() => getTraversalLayoutPointCount(currentLayout.value))

const isStepValid = computed(() => {
  if (currentStep.value === 0) return testName.value.trim() !== '' && estimatedPointCount.value > 0
  if (currentStep.value === 1) {
    return probeChannels.value.filter((channel) => channel.enabled).every((channel) => channel.channel.deviceId !== '' && channel.channel.channelIndex >= 0) &&
      motionAxes.value.every((axis) => axis.controllerId !== '')
  }
  if (currentStep.value === 2) {
    // PRB/CSV 鏁版嵁瀵煎叆楠岃瘉
    if (interpolationAlgorithm.value === 'new') {
      return calibrationCsvFile.value !== null
    }
    // 鏃х畻娉曟垨榛樿
    if (prbMode.value === 'multi') {
      return multiPrbFiles.value.length > 0
    }
    return prbFile.value !== null
  }
  if (currentStep.value === steps.value.length - 1) {
    return savePath.value.trim() !== '' && saveFileName.value.trim() !== ''
  }
  return true
})

watch(testName, (nextName, previousName) => {
  const previousDefault = buildDefaultSaveFileName(previousName)
  if (saveFileName.value === '' || saveFileName.value === previousDefault) {
    saveFileName.value = buildDefaultSaveFileName(nextName)
  }
})

function buildDefaultSaveFileName(name: string): string {
  return sanitizeCsvFileName(name, 'Traversal')
}

function sanitizeFileNameStem(value: string, fallbackStem: string): string {
  const normalizedFallback = fallbackStem
    .trim()
    .replace(INVALID_FILE_NAME_CHARS, '-')
    .replace(/\s+/g, ' ')
    .replace(/-+/g, '-')
    .trim()
    .replace(/^[.\s-]+/, '')
    .replace(/[.\s-]+$/, '') || 'Traversal'

  const withoutCsvExtension = value.trim().replace(/\.csv$/i, '')
  const collapsed = withoutCsvExtension
    .replace(INVALID_FILE_NAME_CHARS, '-')
    .replace(/\s+/g, ' ')
    .replace(/-+/g, '-')
    .trim()
    .replace(/^[.\s-]+/, '')
    .replace(/[.\s-]+$/, '')

  const safeStem = collapsed || normalizedFallback

  return WINDOWS_RESERVED_FILE_NAMES.has(safeStem.toUpperCase())
    ? `${safeStem}-file`
    : safeStem
}

function sanitizeCsvFileName(fileName: string, fallbackStem: string): string {
  return `${sanitizeFileNameStem(fileName, fallbackStem)}.csv`
}

function normalizeSaveFileName(fileName: string): string {
  return sanitizeCsvFileName(fileName, testName.value)
}

function isRequiredProbeChannel(channel: ProbeChannelConfig): boolean {
  return isTraversalRequiredProbeChannel(channel.role, channel.name)
}

function normalizeProbeChannel(channel: ProbeChannelConfig): ProbeChannelConfig {
  return {
    ...channel,
    channel: { ...channel.channel },
    enabled: isRequiredProbeChannel(channel) ? true : channel.enabled
  }
}

function clonePrbFileInfo(file: PrbFileInfo): PrbFileInfo {
  return {
    ...file,
    validRange: { ...file.validRange }
  }
}

function normalizeMultiPrbMachNumbers(files: PrbFileInfo[], machNumbers: number[] = []): number[] {
  return files.map((file, index) => {
    const value = machNumbers[index] ?? file.machNumber ?? file.validRange.machMin
    return Number.isFinite(value) ? value : 0
  })
}

function isConfigurableProbeChannel(channel: ProbeChannelConfig): boolean {
  return isTraversalConfigurableProbeChannel(channel.role, channel.name)
}

function nextStep(): void {
  if (currentStep.value < steps.value.length - 1 && isStepValid.value) currentStep.value += 1
}

function prevStep(): void {
  if (currentStep.value > 0) currentStep.value -= 1
}

function applySavedLayout(layout: TraversalLayout): void {
  pattern.value = layout.pattern

  if (layout.line) {
    lineConfig.value = {
      ...layout.line,
      xStepSegments: layout.line.xStepSegments.map((segment) => ({ ...segment })),
      yStepSegments: layout.line.yStepSegments.map((segment) => ({ ...segment }))
    }
  }

  if (layout.rectangle) {
    rectangleConfig.value = {
      ...layout.rectangle,
      xStepSegments: layout.rectangle.xStepSegments.map((segment) => ({ ...segment })),
      yStepSegments: layout.rectangle.yStepSegments.map((segment) => ({ ...segment }))
    }
  }

  if (layout.sector) {
    sectorConfig.value = {
      ...layout.sector,
      radialStepSegments: layout.sector.radialStepSegments.map((segment) => ({ ...segment })),
      angularStepSegments: layout.sector.angularStepSegments.map((segment) => ({ ...segment }))
    }
  }

  customPoints.value = layout.custom?.points.map((point) => ({ ...point })) ?? []
}

function applySavedConfig(config: TraversalTestConfig): void {
  const savedConfig = JSON.parse(JSON.stringify(config)) as TraversalTestConfig

  testName.value = savedConfig.name
  dwellTimeMs.value = savedConfig.dwellTimeMs
  samplesPerPoint.value = savedConfig.samplesPerPoint
  savePath.value = savedConfig.savePath
  saveFileName.value = sanitizeCsvFileName(savedConfig.saveFileName, savedConfig.name)
  const useSavedMultiPrb = Boolean((savedConfig.useMultiPrb ?? false) && savedConfig.multiPrb?.files.length)
  prbMode.value = useSavedMultiPrb ? 'multi' : 'single'
  prbFile.value = useSavedMultiPrb
    ? null
    : savedConfig.prbFile
      ? clonePrbFileInfo(savedConfig.prbFile)
      : null
  multiPrbFiles.value = useSavedMultiPrb
    ? (savedConfig.multiPrb?.files ?? []).map((file) => clonePrbFileInfo(file))
    : []
  multiPrbMachNumbers.value = useSavedMultiPrb
    ? normalizeMultiPrbMachNumbers(multiPrbFiles.value, savedConfig.multiPrb?.machNumbers)
    : []
  multiPrbInterpolationMode.value = savedConfig.multiPrb?.interpolationMode ?? 'linear'
  saveOptions.value = {
    ...saveOptions.value,
    ...savedConfig.saveOptions,
    customFields: savedConfig.saveOptions.customFields
      ? { ...savedConfig.saveOptions.customFields }
      : undefined
  }

  applySavedLayout(savedConfig.layout)

  if (savedConfig.channels.probeChannels.length > 0) {
    const nextProbeChannels = [...probeChannels.value]
    for (const savedChannel of savedConfig.channels.probeChannels) {
      if (!isConfigurableProbeChannel(savedChannel)) {
        continue
      }

      const index = nextProbeChannels.findIndex((channel) =>
        channel.role ? channel.role === savedChannel.role : channel.name === savedChannel.name
      )

      if (index >= 0) {
        nextProbeChannels[index] = normalizeProbeChannel({
          ...nextProbeChannels[index],
          ...savedChannel,
          channel: { ...savedChannel.channel }
        })
      } else {
        nextProbeChannels.push(normalizeProbeChannel({
          ...savedChannel,
          channel: { ...savedChannel.channel }
        }))
      }
    }
    probeChannels.value = nextProbeChannels
  }

  if (savedConfig.channels.motionAxes.length > 0) {
    const nextMotionAxes = [...motionAxes.value]
    for (const savedAxis of savedConfig.channels.motionAxes) {
      const index = nextMotionAxes.findIndex((axis) => axis.name === savedAxis.name)

      const normalizedAxis: TraversalMotionAxisConfig = {
        ...savedAxis,
        angleMapping: savedAxis.angleMapping ? { ...savedAxis.angleMapping } : undefined
      }

      if (index >= 0) {
        nextMotionAxes[index] = normalizedAxis
      } else {
        nextMotionAxes.push(normalizedAxis)
      }
    }
    motionAxes.value = nextMotionAxes
  }

  interpolationAlgorithm.value = savedConfig.interpolationAlgorithm ?? 'old'
  if (savedConfig.calibrationCsvFile) {
    calibrationCsvFile.value = { ...savedConfig.calibrationCsvFile }
  } else {
    calibrationCsvFile.value = null
  }
}

async function loadSavedConfig(): Promise<void> {
  await traversalStore.loadConfig()
  if (traversalStore.config) {
    applySavedConfig(traversalStore.config)
  }
}

async function pickSavePath(): Promise<void> {
  try {
    const selectedPath = await storageStore.pickDirectory()
    if (selectedPath) {
      savePath.value = selectedPath
    }
  } catch (err) {
    feedbackStore.pushToast('Failed to choose output directory: ' + (err instanceof Error ? err.message : String(err)), 'error')
  }
}

async function saveConfig(): Promise<void> {
  // 妫€鏌ユ祴璇曟槸鍚︽鍦ㄨ繍琛?
  if (traversalStore.isRunning) {
    const confirmed = await feedbackStore.confirm(
      t.value.saveConfigWhileRunning,
      {
        title: t.value.statusRunning,
        confirmText: t.value.save,
        cancelText: t.value.cancel
      }
    )
    if (!confirmed) {
      return
    }
  }

  isSaving.value = true
  try {
    const normalizedSaveFileName = normalizeSaveFileName(saveFileName.value)
    saveFileName.value = normalizedSaveFileName
    const useMultiPrb = prbMode.value === 'multi' && multiPrbFiles.value.length > 0
    const multiPrb = useMultiPrb
      ? {
          files: multiPrbFiles.value.map((file) => clonePrbFileInfo(file)),
          machNumbers: multiPrbMachNumbers.value.map((machNumber) => Number(machNumber)),
          interpolationMode: multiPrbInterpolationMode.value
        }
      : undefined

    // 鏋勫缓閰嶇疆瀵硅薄锛屼娇鐢?JSON 娣辨嫹璐濆交搴曡В闄?Vue 鍝嶅簲寮忎唬鐞?
    // 杩欐槸蹇呰鐨勶紝鍥犱负 Electron IPC 浣跨敤缁撴瀯鍖栧厠闅嗙畻娉曪紝鏃犳硶搴忓垪鍖?Proxy 瀵硅薄
    const rawConfig = {
      name: testName.value,
      layout: currentLayout.value,
      channels: {
        probeChannels: probeChannels.value.filter((channel) => channel.enabled && isConfigurableProbeChannel(channel)),
        motionAxes: motionAxes.value
      },
      prbFile: useMultiPrb ? null : prbFile.value,
      multiPrb,
      useMultiPrb,
      interpolationAlgorithm: interpolationAlgorithm.value,
      calibrationCsvFile: interpolationAlgorithm.value === 'new' ? calibrationCsvFile.value : null,
      dwellTimeMs: dwellTimeMs.value,
      samplesPerPoint: samplesPerPoint.value,
      savePath: savePath.value.trim(),
      saveFileName: normalizedSaveFileName,
      saveOptions: saveOptions.value
    }
    const config: TraversalTestConfig = JSON.parse(JSON.stringify(rawConfig))
    const ok = await traversalStore.saveConfig(config)
    if (!ok) throw new Error(traversalStore.error || t.value.failedSaveConfig)
    emit('saved', config)
    emit('close')
  } catch (err) {
    feedbackStore.pushToast(t.value.failedSaveConfig + ': ' + (err instanceof Error ? err.message : String(err)), 'error')
  } finally {
    isSaving.value = false
  }
}

onMounted(async () => {
  try {
    await Promise.all([
      deviceStore.refreshProfiles(),
      motionStore.refreshProfiles(),
      storageStore.loadSettings(),
      loadSavedConfig()
    ])

    if (savePath.value.trim() === '') {
      savePath.value = storageStore.settings?.baseDirectory?.trim() ?? ''
    }

    if (saveFileName.value.trim() === '') {
      saveFileName.value = buildDefaultSaveFileName(testName.value)
    }
  } finally {
    isLoading.value = false
  }
})
</script>

<template>
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-[rgba(7,12,20,0.58)] px-4 py-6 backdrop-blur-md">
    <div data-test="traversal-settings-shell" class="flex max-h-[92vh] w-[960px] flex-col overflow-hidden rounded-[var(--radius-md)] border border-[color:var(--border-default)] bg-[color:var(--bg-panel)] text-[color:var(--text-primary)] shadow-[0_28px_80px_rgba(2,6,23,0.48)]">
      <div class="border-b border-[color:var(--border-default)] bg-[color:var(--bg-panel)] px-6 py-5">
        <div class="flex items-start justify-between gap-4">
          <div>
            <p class="text-[11px] font-semibold uppercase tracking-[0.2em] text-[color:var(--text-muted)]">{{ t.traversalSetup }}</p>
            <h2 class="mt-2 text-xl font-semibold text-[color:var(--text-primary)]">{{ t.traversalWorkbenchConfig }}</h2>
          </div>
          <UiButton variant="secondary" @click="emit('close')">{{ t.close }}</UiButton>
        </div>
      </div>

      <div data-test="traversal-settings-steps" class="border-b border-[color:var(--border-default)] bg-[color:var(--bg-panel-strong)] px-6 py-4">
        <div class="flex items-center gap-6">
          <div v-for="(step, index) in steps" :key="index" class="flex items-center gap-2">
            <div class="flex h-7 w-7 items-center justify-center rounded-[var(--radius-sm)] text-xs font-semibold"
              :class="index === currentStep ? 'bg-[color:var(--accent-primary)] text-white' : index < currentStep ? 'bg-[color:var(--accent-success)] text-white' : 'bg-[color:var(--bg-panel)] text-[color:var(--text-muted)] border border-[color:var(--border-default)]'">
              {{ index + 1 }}
            </div>
            <span class="text-sm" :class="index === currentStep ? 'text-[color:var(--text-primary)] font-medium' : 'text-[color:var(--text-muted)]'">{{ step }}</span>
          </div>
        </div>
      </div>

      <div class="grid min-h-0 flex-1 grid-cols-[minmax(0,1fr)_360px] overflow-hidden">
        <div class="min-h-0 overflow-auto px-6 py-5">
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

          <div v-else class="space-y-4">
            <section :class="cardClass" class="p-4">
              <div class="grid grid-cols-2 gap-x-8 gap-y-3 text-sm">
                <div class="flex items-center justify-between py-2 border-b border-[color:var(--border-default)]">
                  <span class="text-[color:var(--text-secondary)]">{{ t.name }}</span>
                  <span class="font-medium text-[color:var(--text-primary)]">{{ testName }}</span>
                </div>
                <div class="flex items-center justify-between py-2 border-b border-[color:var(--border-default)]">
                  <span class="text-[color:var(--text-secondary)]">{{ t.pattern }}</span>
                  <span class="font-medium text-[color:var(--text-primary)]">{{ pattern }}</span>
                </div>
                <div class="flex items-center justify-between py-2 border-b border-[color:var(--border-default)]">
                  <span class="text-[color:var(--text-secondary)]">{{ t.estimatedPoints }}</span>
                  <span data-test="traversal-estimated-points" class="font-mono font-semibold text-[color:var(--accent-primary)]">{{ estimatedPointCount }}</span>
                </div>
                <div class="flex items-center justify-between py-2 border-b border-[color:var(--border-default)]">
                  <span class="text-[color:var(--text-secondary)]">{{ (t as Record<string, string>).interpolationAlgorithm || 'Algorithm' }}</span>
                  <span class="font-medium text-[color:var(--text-primary)]">{{ interpolationAlgorithm === 'new' ? ((t as Record<string, string>).algorithmNew || 'New') : ((t as Record<string, string>).algorithmOld || 'Old') }}</span>
                </div>
                <div class="flex items-center justify-between py-2 border-b border-[color:var(--border-default)]">
                  <span class="text-[color:var(--text-secondary)]">{{ t.prb }}</span>
                  <span class="font-medium text-[color:var(--text-primary)] truncate max-w-[150px]">{{ interpolationAlgorithm === 'new' ? (calibrationCsvFile ? calibrationCsvFile.fileName : t.none) : (prbFile ? prbFile.fileName : t.none) }}</span>
                </div>
              </div>
            </section>

            <section :class="cardClass" class="space-y-3">
              <div class="grid gap-3 lg:grid-cols-[minmax(0,1fr)_220px_auto]">
                <input v-model="savePath" data-test="traversal-save-path-input" type="text" :class="fieldClass" placeholder="Output directory" />
                <input v-model="saveFileName" data-test="traversal-save-file-name-input" type="text" :class="fieldClass" placeholder="CSV file name" />
                <UiButton variant="secondary" @click="pickSavePath">Browse</UiButton>
              </div>
              <p class="text-xs text-[color:var(--text-muted)]">Choose a real output directory for traversal CSV exports.</p>
            </section>

            <section :class="cardClass" class="p-3">
              <div class="grid grid-cols-2 gap-2">
                <label class="flex items-center gap-2 p-2 rounded-[var(--radius-sm)] hover:bg-[color:var(--bg-panel-strong)] cursor-pointer">
                  <input v-model="saveOptions.savePointId" type="checkbox" :class="checkboxClass" />
                  <span class="text-sm text-[color:var(--text-primary)]">{{ t.savePointId }}</span>
                </label>
                <label class="flex items-center gap-2 p-2 rounded-[var(--radius-sm)] hover:bg-[color:var(--bg-panel-strong)] cursor-pointer">
                  <input v-model="saveOptions.saveTimestamp" type="checkbox" :class="checkboxClass" />
                  <span class="text-sm text-[color:var(--text-primary)]">{{ t.saveTimestamp }}</span>
                </label>
                <label class="flex items-center gap-2 p-2 rounded-[var(--radius-sm)] hover:bg-[color:var(--bg-panel-strong)] cursor-pointer">
                  <input v-model="saveOptions.saveRawPressure" type="checkbox" :class="checkboxClass" />
                  <span class="text-sm text-[color:var(--text-primary)]">{{ t.saveRawPressure }}</span>
                </label>
                <label class="flex items-center gap-2 p-2 rounded-[var(--radius-sm)] hover:bg-[color:var(--bg-panel-strong)] cursor-pointer">
                  <input v-model="saveOptions.saveCalculatedResult" type="checkbox" :class="checkboxClass" />
                  <span class="text-sm text-[color:var(--text-primary)]">{{ t.saveCalculatedResult }}</span>
                </label>
              </div>
            </section>
          </div>
        </div>

        <aside class="border-l border-[color:var(--border-default)] bg-[color:var(--bg-panel-strong)] px-5 py-5 flex flex-col">
          <section :class="cardClass" class="flex flex-col flex-1">
            <div class="grid grid-cols-3 gap-3 mb-4">
              <div :class="subCardClass" class="text-center py-3">
                <div class="text-xs text-[color:var(--text-muted)] mb-1">{{ t.points }}</div>
                <div class="font-mono text-xl font-bold text-[color:var(--accent-primary)]">{{ estimatedPointCount }}</div>
              </div>
              <div :class="subCardClass" class="text-center py-3">
                <div class="text-xs text-[color:var(--text-muted)] mb-1">{{ t.dwell }}</div>
                <div class="font-mono text-sm font-semibold text-[color:var(--text-primary)]">{{ dwellTimeMs }} <span class="text-xs text-[color:var(--text-muted)]">ms</span></div>
              </div>
              <div :class="subCardClass" class="text-center py-3">
                <div class="text-xs text-[color:var(--text-muted)] mb-1">{{ t.samples }}</div>
                <div class="font-mono text-sm font-semibold text-[color:var(--text-primary)]">{{ samplesPerPoint }}</div>
              </div>
            </div>
            <div class="flex-1 min-h-[320px] rounded-[var(--radius-md)] border border-[color:var(--border-default)] bg-[color:var(--bg-canvas)] overflow-hidden">
              <PointsPreview :layout="currentLayout" />
            </div>
          </section>
        </aside>
      </div>

      <div class="border-t border-[color:var(--border-default)] bg-[color:var(--bg-panel-strong)] px-6 py-4">
        <div class="flex items-center justify-between">
          <UiButton v-if="currentStep > 0" variant="secondary" @click="prevStep">{{ t.previous }}</UiButton>
          <div v-else></div>
          <div class="flex items-center gap-2">
            <UiButton variant="secondary" @click="emit('close')">{{ t.cancel }}</UiButton>
            <UiButton v-if="currentStep < steps.length - 1" variant="primary" :disabled="!isStepValid" @click="nextStep">{{ t.next }}</UiButton>
            <UiButton v-else variant="primary" :disabled="isSaving || !isStepValid" @click="saveConfig">{{ isSaving ? t.saving : t.saveConfig }}</UiButton>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>


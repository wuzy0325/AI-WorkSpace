<script setup lang="ts">
import { computed, ref, watch } from 'vue'
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
  getTraversalStepValues,
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
  TraversalPrimaryAxis,
  TraversalTestConfig,
  CalibrationCsvFileInfo,
  InterpolationAlgorithm,
  StabilizationConfig,
  DataValidationConfig
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
import { reportAllSettledFailures } from '@utils/allSettledReport'

const props = withDefaults(
  defineProps<{
    show?: boolean
  }>(),
  { show: false }
)

const emit = defineEmits<{
  close: []
  saved: [config: TraversalTestConfig]
  'update:show': [value: boolean]
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

// 记录用户已访问过的步骤索引，用于支持步骤导航点击跳转
const visitedSteps = ref<Set<number>>(new Set([0]))

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

// 蛇形扫描顺序：偶数行正向，奇数行反向，减少回程时间
const snakeOrder = ref(false)

// 走线主轴：新建 profile 默认 'x'（先沿 X 走完一条线再切换 Y），用户可切换为 'y'。
// 仅对 line / rectangle 布局生效；扇形布局不消费此字段。
// 注意：applySavedLayout 加载旧 profile（无 primaryAxis 字段）时显式落 'y' 保旧行为，
// 避免静默反转升级前已优化的物理走线方向。
const primaryAxis = ref<TraversalPrimaryAxis>('x')

// 稳定化配置：fixed 模式使用固定等待时间，adaptive 模式持续监测压力变化
const stabilizationMode = ref<'fixed' | 'adaptive'>('fixed')
const stabilizationConfig = ref<StabilizationConfig>({
  mode: 'fixed',
  fixedTimeMs: 2000
})

// 数据验证配置：可选，用于校验压力范围和异常尖峰
const validationEnabled = ref(false)
const validationConfig = ref<DataValidationConfig>({
  enabled: false,
  pressureRange: {},
  onInvalid: 'continue'
})

// 稳定化模式切换时，重建 stabilizationConfig 以保留/初始化子配置
watch(stabilizationMode, (mode) => {
  stabilizationConfig.value = {
    mode,
    fixedTimeMs: stabilizationConfig.value.fixedTimeMs ?? 2000,
    adaptive: mode === 'adaptive'
      ? (stabilizationConfig.value.adaptive ?? {
          maxWaitMs: 10000,
          minWaitMs: 2000,
          stabilityThreshold: 1,
          checkIntervalMs: 200,
          consecutiveChecks: 3
        })
      : undefined
  }
})

// 保持 dwellTimeMs（布局步骤中的输入）与 stabilizationConfig.fixedTimeMs（review 步骤中的输入）同步
// 二者代表同一物理量（固定模式下的等待时间），需双向同步以避免保存时数据不一致
watch(dwellTimeMs, (v) => {
  if (stabilizationConfig.value.mode === 'fixed' && stabilizationConfig.value.fixedTimeMs !== v) {
    stabilizationConfig.value = { ...stabilizationConfig.value, fixedTimeMs: v }
  }
})
watch(() => stabilizationConfig.value.fixedTimeMs, (v) => {
  if (stabilizationConfig.value.mode === 'fixed' && v !== undefined && v !== dwellTimeMs.value) {
    dwellTimeMs.value = v
  }
})

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
  // 蛇形扫描顺序 + 走线主轴透传到 layout，供后端按行交替反向遍历并选择主轴方向
  switch (pattern.value) {
    case 'line': return { pattern: 'line', snakeOrder: snakeOrder.value, primaryAxis: primaryAxis.value, line: lineConfig.value }
    case 'rectangle': return { pattern: 'rectangle', snakeOrder: snakeOrder.value, primaryAxis: primaryAxis.value, rectangle: rectangleConfig.value }
    case 'sector': {
      // 扇形圆心不输入，默认“第一个测点 = 当前位置 = 坐标原点 (0,0)”。
      // 由第一个测点 (r₁, θ₁) 反推圆心，使第一点坐标 = (0,0)，
      // 后续点位相对第一点计算。装配时用户手动把探针定位到第一个测点。
      const radii = getTraversalStepValues(sectorConfig.value.radiusMin, sectorConfig.value.radiusMax, sectorConfig.value.radialStepSegments)
      const angles = getTraversalStepValues(sectorConfig.value.angleStart, sectorConfig.value.angleEnd, sectorConfig.value.angularStepSegments)
      const r1 = radii[0] ?? 0
      const t1 = ((angles[0] ?? 0) * Math.PI) / 180
      const hasData = radii.length > 0 && angles.length > 0
      const centerX = hasData ? -r1 * Math.cos(t1) : 0
      const centerY = hasData ? -r1 * Math.sin(t1) : 0
      return { pattern: 'sector', snakeOrder: snakeOrder.value, sector: { ...sectorConfig.value, centerX, centerY } }
    }
    case 'custom': return { pattern: 'custom', snakeOrder: snakeOrder.value, custom: { points: customPoints.value } }
  }
})

const estimatedPointCount = computed(() => getTraversalLayoutPointCount(currentLayout.value))

// 仅 line / rectangle 布局消费走线主轴；review 摘要只在支持时显示该行，避免重复条件字面量。
const supportsPrimaryAxis = computed(() => pattern.value === 'line' || pattern.value === 'rectangle')

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
function nextStep() {
  if (currentStep.value < steps.value.length - 1 && isStepValid.value) {
    currentStep.value++
    visitedSteps.value.add(currentStep.value)
  }
}
function prevStep() {
  if (currentStep.value > 0) {
    currentStep.value--
    visitedSteps.value.add(currentStep.value)
  }
}

// 点击步骤导航跳转：仅允许跳转到已访问过的步骤
function goToStep(stepIndex: number) {
  if (visitedSteps.value.has(stepIndex)) {
    currentStep.value = stepIndex
  }
}

function applySavedLayout(layout: TraversalLayout) {
  pattern.value = layout.pattern
  // 恢复蛇形扫描顺序，缺省为 false
  snakeOrder.value = layout.snakeOrder ?? false
  // 恢复走线主轴：旧 profile 无此字段时落 'y'（保旧行为，避免静默反转物理走线方向）；
  // 新 profile 保存时已显式存 'x' 或 'y'，加载时按持久化值恢复。
  primaryAxis.value = layout.primaryAxis ?? 'y'
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

  // 恢复数据验证配置：未保存时使用默认禁用状态
  if (c.validation) {
    validationEnabled.value = c.validation.enabled
    validationConfig.value = {
      enabled: c.validation.enabled,
      pressureRange: c.validation.pressureRange ? { ...c.validation.pressureRange } : {},
      onInvalid: c.validation.onInvalid ?? 'continue'
    }
  } else {
    validationEnabled.value = false
    validationConfig.value = { enabled: false, pressureRange: {}, onInvalid: 'continue' }
  }

  // 恢复稳定化配置：未保存时使用默认 fixed 模式
  if (c.stabilization) {
    stabilizationMode.value = c.stabilization.mode
    stabilizationConfig.value = {
      mode: c.stabilization.mode,
      fixedTimeMs: c.stabilization.fixedTimeMs ?? 2000,
      adaptive: c.stabilization.adaptive ? { ...c.stabilization.adaptive } : undefined
    }
  } else {
    stabilizationMode.value = 'fixed'
    stabilizationConfig.value = { mode: 'fixed', fixedTimeMs: 2000 }
  }
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
    feedbackStore.pushToast(t.value.failedChooseDirectory + '：' + (e instanceof Error ? e.message : String(e)), 'error')
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
    // dwellTimeMs 与 stabilizationConfig.fixedTimeMs 保持同步：固定模式下二者一致
    const raw = {
      name: testName.value, layout: currentLayout.value,
      channels: { probeChannels: probeChannels.value.filter(c => c.enabled && isConfigurableProbeChannel(c)), motionAxes: motionAxes.value },
      prbFile: useMulti ? null : prbFile.value,
      multiPrb: useMulti ? { files: multiPrbFiles.value.map(f => clonePrbFileInfo(f)), machNumbers: multiPrbMachNumbers.value.map(n => Number(n)), interpolationMode: multiPrbInterpolationMode.value } : undefined,
      useMultiPrb: useMulti, interpolationAlgorithm: interpolationAlgorithm.value,
      calibrationCsvFile: interpolationAlgorithm.value === 'new' ? calibrationCsvFile.value : null,
      dwellTimeMs: stabilizationConfig.value.fixedTimeMs ?? dwellTimeMs.value,
      samplesPerPoint: samplesPerPoint.value,
      savePath: savePath.value.trim(), saveFileName: normName, saveOptions: saveOptions.value,
      // 仅在启用时保存验证配置，与 Cursor DAQ 行为一致
      validation: validationEnabled.value ? validationConfig.value : undefined,
      // 稳定化配置始终保存，后端依据 mode 决定使用固定时间或自适应逻辑
      stabilization: stabilizationConfig.value
    }
    const config: TraversalTestConfig = JSON.parse(JSON.stringify(raw))
    const ok = await traversalStore.saveConfig(config)
    if (!ok) throw new Error(traversalStore.error || t.value.failedSaveConfig)
    emit('saved', config); emit('close')
  } catch (e) {
    feedbackStore.pushToast(t.value.failedSaveConfig + ': ' + (e instanceof Error ? e.message : String(e)), 'error')
  } finally { isSaving.value = false }
}

// 对话框打开时重新加载已保存的配置，确保 UI 与后端状态同步
watch(() => props.show, async (isVisible) => {
  if (!isVisible) return
  isLoading.value = true
  try {
    const results = await Promise.allSettled([
      deviceStore.refreshProfiles(),
      motionStore.refreshProfiles(),
      storageStore.loadSettings(),
      loadSavedConfig()
    ])
    reportAllSettledFailures(
      results,
      ['设备列表', '运动控制器列表', '存储设置', '遍历配置'],
      feedbackStore.pushToast,
    )
    if (!savePath.value.trim()) savePath.value = storageStore.settings?.baseDirectory?.trim() ?? ''
    if (!saveFileName.value.trim()) saveFileName.value = buildDefaultSaveFileName(testName.value)
    // 重置步骤导航到第一步
    currentStep.value = 0
    visitedSteps.value = new Set([0])
  } finally { isLoading.value = false }
}, { immediate: true })
</script>

<template>
  <!-- 遍历测试配置对话框：限制最大高度，使用 flex 布局确保内容不溢出 -->
  <UiDialog :show="props.show" width="min(92vw, 960px)" closable @close="emit('close')">
    <template #header>
      <div>
          <span class="setup-overline">{{ t.traversalSetup }}</span>
          <span class="setup-title">{{ t.traversalWorkbenchConfig }}</span>
      </div>
    </template>

    <UiSteps :current="currentStep" class="steps-nav">
      <UiStep
        v-for="(step, idx) in steps"
        :key="idx"
        :title="step"
        :status="idx < currentStep ? 'finish' : idx === currentStep ? 'process' : 'wait'"
        :disabled="!visitedSteps.has(idx)"
        @click="goToStep(idx)"
      />
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
          v-model:snake-order="snakeOrder"
          v-model:primary-axis="primaryAxis"
          :estimated-point-count="estimatedPointCount"
          :t="(t as unknown as Record<string, string>)"
        />
        <TraversalHardwareStep
          v-else-if="currentStep === 1"
          v-model:probe-channels="probeChannels"
          v-model:motion-axes="motionAxes"
          v-model:validation-enabled="validationEnabled"
          v-model:validation-config="validationConfig"
          v-model:stabilization-mode="stabilizationMode"
          v-model:stabilization-config="stabilizationConfig"
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
          <!-- 配置摘要：只读展示，不含配置决策 -->
          <UiPanel class="section-card">
            <template #header><span class="summary-section-title">{{ t.summaryTitle }}</span></template>
            <div class="summary-grid">
              <div class="summary-row"><span style="color:var(--text-tertiary)">{{ t.name }}</span><span>{{ testName }}</span></div>
              <div class="summary-row"><span style="color:var(--text-tertiary)">{{ t.pattern }}</span><span>{{ pattern }}</span></div>
              <div class="summary-row"><span style="color:var(--text-tertiary)">{{ t.estimatedPoints }}</span><span class="summary-accent">{{ estimatedPointCount }}</span></div>
              <div class="summary-row"><span style="color:var(--text-tertiary)">{{ t.interpolationAlgorithm }}</span><span>{{ interpolationAlgorithm === 'new' ? t.algorithmNew : t.algorithmOld }}</span></div>
              <div class="summary-row"><span style="color:var(--text-tertiary)">{{ t.prb }}</span><span class="text-ellipsis">{{ interpolationAlgorithm === 'new' ? (calibrationCsvFile ? calibrationCsvFile.fileName : t.none) : (prbFile ? prbFile.fileName : t.none) }}</span></div>
              <div class="summary-row"><span style="color:var(--text-tertiary)">{{ t.travSnakeOrder }}</span><span>{{ snakeOrder ? t.enabled : t.disabled }}</span></div>
              <div v-if="supportsPrimaryAxis" class="summary-row"><span style="color:var(--text-tertiary)">{{ t.travPrimaryAxis || 'Primary axis' }}</span><span>{{ primaryAxis === 'x' ? (t.travPrimaryAxisX || 'X first') : (t.travPrimaryAxisY || 'Y first') }}</span></div>
              <div class="summary-row"><span style="color:var(--text-tertiary)">{{ t.travStableMode }}</span><span>{{ stabilizationMode === 'fixed' ? t.travFixedTime : t.travAdaptive }}</span></div>
              <div class="summary-row"><span style="color:var(--text-tertiary)">{{ t.travEnableValidation }}</span><span>{{ validationEnabled ? t.enabled : t.disabled }}</span></div>
            </div>
          </UiPanel>

          <UiPanel class="section-card">
            <div class="save-row">
              <UiInput v-model="savePath" :placeholder="t.outputDirectory" size="small" class="flex-input" :title="savePath" />
              <UiInput v-model="saveFileName" :placeholder="t.outputFileName" size="small" class="flex-input" />
              <UiButton size="sm" @click="pickSavePath">{{ t.browse }}</UiButton>
            </div>
            <span class="save-hint">{{ t.outputDirectoryHint }}</span>
          </UiPanel>

          <UiPanel class="section-card">
            <template #header>
              <div class="save-options-header">
                <span class="summary-section-title">{{ t.saveOptionsTitle }}</span>
                <div class="save-options-actions">
                  <UiButton size="sm" quaternary @click="saveOptions = { savePointId: true, saveTimestamp: true, saveRawPressure: true, saveCalculatedResult: true }">{{ t.selectAll }}</UiButton>
                  <UiButton size="sm" quaternary @click="saveOptions = { savePointId: false, saveTimestamp: false, saveRawPressure: false, saveCalculatedResult: false }">{{ t.deselectAll }}</UiButton>
                </div>
              </div>
            </template>
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
          <div class="sidebar-stat sidebar-stat--highlight">
            <span class="stat-label">{{ t.points }}</span>
            <span class="stat-number">{{ estimatedPointCount }}</span>
          </div>
          <div class="sidebar-stat">
            <span class="stat-label">{{ t.dwell }}</span>
            <span class="stat-value">{{ dwellTimeMs }}<span class="stat-unit">ms</span></span>
          </div>
          <div class="sidebar-stat">
            <span class="stat-label">{{ t.samples }}</span>
            <span class="stat-value">{{ samplesPerPoint }}</span>
          </div>
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
/* 遍历测试配置画面主体布局：左右分栏，限制最大高度防止被拉长 */
.traversal-body {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 300px;
  gap: 0;
  min-height: 0;
  max-height: 65vh;
  flex: 1;
  overflow: hidden
}

.traversal-main {
  min-height: 0;
  max-height: 65vh;
  overflow-y: auto;
  padding-right: var(--space-3);
  scrollbar-width: thin
}

.step-content {
  display: flex;
  flex-direction: column;
  gap: var(--space-2)
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
  padding: 6px 0;
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
  padding: 4px var(--space-2);
  border-radius: var(--radius-md);
  font-size: var(--text-sm);
  color: var(--text-primary);
  cursor: pointer;
  transition: background 120ms ease
}

.option-label:hover {
  background: var(--bg-panel-strong)
}

/* 右侧边栏：固定宽度，限制高度，内部可滚动 */
.traversal-sidebar {
  border-left: 1px solid var(--border-default);
  background: var(--bg-panel-strong);
  padding-left: var(--space-3);
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  max-height: 65vh;
  overflow-y: auto;
  scrollbar-width: thin
}

.sidebar-stats {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: var(--space-2)
}

.sidebar-stat {
  padding: 10px 6px;
  border-radius: var(--radius-md);
  border: 1px solid var(--border-default);
  background: var(--bg-panel);
  text-align: center;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px
}

.sidebar-stat--highlight {
  border-color: var(--color-primary, #3b82f6);
  background: linear-gradient(180deg, var(--bg-panel) 0%, rgba(59,130,246,0.06) 100%)
}

.sidebar-stat--highlight .stat-number {
  color: var(--color-primary, #3b82f6)
}

/* 点阵预览区域：使用自适应高度替代固定最小高度，防止撑大对话框 */
.sidebar-preview {
  flex: 1 1 auto;
  min-height: 160px;
  max-height: 360px;
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
/* 步骤导航栏：减小下方间距，与内容区域更紧凑 */
.steps-nav { margin-bottom:var(--space-2) }
.summary-section-title { font-size:var(--text-sm);font-weight:600 }
.summary-accent { color:var(--color-accent);font-weight:700 }
.text-ellipsis { max-width:180px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap }
.flex-input { flex:1 }
.save-hint { font-size:var(--text-xs);display:block;margin-top:6px;color:var(--text-tertiary) }
.stat-label { font-size:var(--text-xs);color:var(--text-tertiary) }
.stat-number { font-size:18px;font-weight:700;color:var(--color-accent) }
.stat-value { font-size:12px;font-weight:600 }
.stat-unit { font-size:var(--text-xs) }

.save-options-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%
}

.save-options-actions {
  display: flex;
  align-items: center;
  gap: var(--space-1)
}

/* 子配置区块：数据验证策略、稳定化模式等嵌套配置 */
.sub-config-block {
  margin-top: var(--space-2);
  padding-left: var(--space-4);
  display: flex;
  flex-direction: column;
  gap: 4px
}

.sub-config-label {
  font-size: var(--text-xs);
  color: var(--text-tertiary)
}

.sub-config-hint {
  margin-top: var(--space-2);
  font-size: var(--text-xs);
  color: var(--text-tertiary);
  line-height: 1.5
}

/* 单选按钮组：稳定化模式、错误处理策略 */
.radio-group {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-3);
  margin-top: 4px
}

.radio-label {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: var(--text-sm);
  color: var(--text-primary);
  cursor: pointer
}

.radio-label input[type="radio"] {
  margin: 0
}
</style>

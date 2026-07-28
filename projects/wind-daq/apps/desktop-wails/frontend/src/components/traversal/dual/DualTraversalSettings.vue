/**
 * ============================================================================
 * DualTraversalSettings — 双探针并行遍历配置对话框（spec FR2 / Task 21）
 * ============================================================================
 *
 * 【功能定位】
 * 双探针模式下的统一配置对话框：
 *   - 顶部 tab 切换 probe1 / probe2，每 probe 独立编辑配置
 *   - 复用既有步骤组件（TraversalHardwareStep / TraversalPrbStep /
 *     TraversalLayoutStep / PointsPreview / MotionSafetyPanel）
 *   - 每 probe 独立保存到后端 probe-scoped 配置键（traversal.probe1 /
 *     traversal.probe2）
 *   - 保存前原子校验：当前 probe 的 controller ID 非空且与另一 probe 不同
 *
 * 【隔离设计】
 * - 通过 dualTraversalStore 的 keyed session 实现 probe1/probe2 配置完全隔离
 * - 每 probe 维护独立的草稿对象（drafts[probeId]），tab 切换不丢失未保存编辑
 * - 草稿通过 computed v-model 暴露给步骤组件，活动 tab 切换自动重定向
 *
 * 【不修改既有 TraversalSettings.vue】
 * 本组件独立实现配置编辑流程，不复用 TraversalSettings.vue 容器，
 * 但复用其子步骤组件，确保 single 模式与 dual 模式步骤体验一致。
 *
 * @module DualTraversalSettings
 * @see TraversalHardwareStep / TraversalPrbStep / TraversalLayoutStep — 步骤组件
 * ============================================================================
 */
<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'

import { useDualTraversalStore } from '@stores/dualTraversalStore'
import { useI18nStore } from '@stores/i18nStore'
import { useFeedbackStore } from '@stores/feedbackStore'
import { useDeviceStore } from '@stores/deviceStore'
import { useMotionStore } from '@stores/motionStore'
import { useStorageStore } from '@stores/storageStore'

import TraversalHardwareStep from '@components/traversal/TraversalHardwareStep.vue'
import TraversalPrbStep from '@components/traversal/TraversalPrbStep.vue'
import TraversalLayoutStep from '@components/traversal/TraversalLayoutStep.vue'
import PointsPreview from '@components/traversal/PointsPreview.vue'
import MotionSafetyPanel from '@components/shared/MotionSafetyPanel.vue'

import UiDialog from '@components/ui/UiDialog.vue'
import UiSteps from '@components/ui/UiSteps.vue'
import UiStep from '@components/ui/UiStep.vue'
import UiButton from '@components/ui/UiButton.vue'
import UiPanel from '@components/ui/UiPanel.vue'
import UiInput from '@components/ui/UiInput.vue'
import UiCheckbox from '@components/ui/UiCheckbox.vue'
import UiSelect from '@components/ui/UiSelect.vue'

import { reportAllSettledFailures } from '@utils/allSettledReport'
import { buildCalibrationCsvName } from '@shared/calibrationCsvPath'
import {
  createDefaultTraversalProbeChannels,
  createSevenHoleTraversalProbeChannels,
  deriveRangeFromSegments,
  findTraversalAxisKindIssues,
  getTraversalLayoutPointCount,
  getTraversalStepValues,
  hasDuplicateChannel,
  hasDuplicateMotionAxis,
  isTraversalConfigurableProbeChannel,
  isTraversalRequiredProbeChannel,
  normalizeTraversalLayoutRanges,
  TRAVERSAL_PROBE_PRESENTATION,
} from '@shared/types/traversal'
import type {
  CalibrationCsvFileInfo,
  InterpolationAlgorithm,
  MotionSafetyConfig,
  MultiPrbInterpolationMode,
  PrbFileInfo,
  ProbeChannelConfig,
  ProbeId,
  StepSegment,
  SevenHolePrbDraft,
  SevenHolePrbFileInfo,
  TraversalLayout,
  TraversalMotionAxisConfig,
  TraversalPattern,
  TraversalPrimaryAxis,
  TraversalProbeType,
  TraversalTestConfig,
} from '@shared/types/traversal'
import type { TraversalPrbOperations } from '@components/traversal/traversalPrbOperations'

const props = defineProps<{
  show: boolean
  /** 初始激活的 probe tab（由 DualTraversalMain 的 openSettings 事件传入） */
  probeId: ProbeId
}>()

const emit = defineEmits<{
  close: []
  saved: [probeId: ProbeId]
}>()

const dualStore = useDualTraversalStore()
const i18n = useI18nStore()
const feedbackStore = useFeedbackStore()
const deviceStore = useDeviceStore()
const motionStore = useMotionStore()
const storageStore = useStorageStore()
const t = computed(() => i18n.t)

// ---------------------------------------------------------------------------
// 活动tab + 草稿管理
// ---------------------------------------------------------------------------
// activeTab：当前编辑的 probe；probeId prop 变化时同步（用户从某 probe 行点
// "设置"按钮进入时直达对应 tab）。
const activeTab = ref<ProbeId>(props.probeId)
watch(() => props.probeId, (v) => { activeTab.value = v })

// 每 probe 一份独立草稿；tab 切换不丢失未保存编辑。
// drafts 直接持有可变对象，computed v-model 读写都指向当前活动 tab 的草稿属性。
function createEmptyDraft(): TraversalTestConfig {
  return {
    name: `Traversal-${new Date().toISOString().slice(0, 10)}`,
    layout: { pattern: 'line', primaryAxis: 'x', line: { startX: -30, startY: 0, endX: 30, endY: 0, xStepSegments: [{ start: -30, end: 30, step: 5 }], yStepSegments: [] } },
    channels: { probeChannels: createDefaultTraversalProbeChannels(), motionAxes: [
      { name: 'X', controllerId: '', axis: 'X' },
      { name: 'Y', controllerId: '', axis: 'Y' },
      { name: 'Z', controllerId: '', axis: 'Z' },
      { name: 'U', controllerId: '', axis: 'U' },
    ] },
    probeType: 'five-hole',
    dwellTimeMs: 2000,
    samplesPerPoint: 10,
    savePath: '',
    saveFileName: buildCalibrationCsvName(`Traversal-${new Date().toISOString().slice(0, 10)}`, 'Traversal'),
    saveOptions: { savePointId: true, saveTimestamp: true, saveRawPressure: true, saveCalculatedResult: true },
    pProbePressureType: 'gauge',
    motionSafety: undefined,
    prbFile: null,
    multiPrb: undefined,
    useMultiPrb: false,
    interpolationAlgorithm: 'old',
    calibrationCsvFile: null,
    sevenHolePrb: undefined,
    inactiveProbeChannels: [],
  }
}

const drafts = reactive<Record<ProbeId, TraversalTestConfig>>({
  probe1: createEmptyDraft(),
  probe2: createEmptyDraft(),
})

// 七孔 PRB 编辑态（与 TraversalSettings 七孔 draft 结构一致）
const sevenHolePrbDrafts = reactive<Record<ProbeId, SevenHolePrbDraft>>({
  probe1: { source: 'prb', innerFile: null, outerFiles: [null, null, null, null, null, null] },
  probe2: { source: 'prb', innerFile: null, outerFiles: [null, null, null, null, null, null] },
})

// 当前活动 probe 的草稿（shortcut）
const draft = computed<TraversalTestConfig>(() => drafts[activeTab.value])
const sevenDraft = computed<SevenHolePrbDraft>(() => sevenHolePrbDrafts[activeTab.value])

// 加载状态：对话框打开时从后端恢复两路配置
const isLoading = ref(true)
const isSaving = ref(false)
const pendingOperationCount = ref(0)
const isOperationPending = computed(() => pendingOperationCount.value > 0)

// ---------------------------------------------------------------------------
// 步骤导航
// ---------------------------------------------------------------------------
const currentStep = ref(0)
const visitedSteps = ref<Set<number>>(new Set([0]))
const steps = computed(() => [t.value.stepHardware, t.value.stepPrb, t.value.stepLayout, t.value.stepReview])

function goToStep(idx: number): void {
  if (visitedSteps.value.has(idx)) currentStep.value = idx
}
function nextStep(): void {
  if (currentStep.value < steps.value.length - 1 && isStepValid.value) {
    currentStep.value++
    visitedSteps.value.add(currentStep.value)
  }
}
function prevStep(): void {
  if (currentStep.value > 0) {
    currentStep.value--
    visitedSteps.value.add(currentStep.value)
  }
}

// ---------------------------------------------------------------------------
// 步骤组件 v-model 适配层
// ---------------------------------------------------------------------------
// 所有 v-model 都通过 computed 读写 draft.value（即 drafts[activeTab]）。
// tab 切换后，draft.value 自动指向新 probe 的草稿，UI 自动重定向。

function bindDraft<K extends keyof TraversalTestConfig>(key: K) {
  return computed<TraversalTestConfig[K]>({
    get: () => draft.value[key],
    set: (v: TraversalTestConfig[K]) => { draft.value[key] = v },
  })
}

const testName = bindDraft('name')
const dwellTimeMs = bindDraft('dwellTimeMs')
const samplesPerPoint = bindDraft('samplesPerPoint')
// probeType 等字段为 optional，子组件 v-model 要求 required 类型，因此用具体 computed 提供 fallback
const probeType = computed<TraversalProbeType>({
  get: () => draft.value.probeType ?? 'five-hole',
  set: (v: TraversalProbeType) => { draft.value.probeType = v },
})
const pProbePressureType = computed<'gauge' | 'absolute'>({
  get: () => draft.value.pProbePressureType ?? 'gauge',
  set: (v: 'gauge' | 'absolute') => { draft.value.pProbePressureType = v },
})
const prbFile = bindDraft('prbFile')
const multiPrbFiles = computed<PrbFileInfo[]>({
  get: () => draft.value.multiPrb?.files ?? [],
  set: (v: PrbFileInfo[]) => {
    draft.value.multiPrb = { ...(draft.value.multiPrb ?? { machNumbers: [], interpolationMode: 'linear' }), files: v }
  },
})
const multiPrbMachNumbers = computed<number[]>({
  get: () => draft.value.multiPrb?.machNumbers ?? [],
  set: (v: number[]) => {
    draft.value.multiPrb = { ...(draft.value.multiPrb ?? { files: [], interpolationMode: 'linear' }), machNumbers: v }
  },
})
const multiPrbInterpolationMode = computed<MultiPrbInterpolationMode>({
  get: () => draft.value.multiPrb?.interpolationMode ?? 'linear',
  set: (v: MultiPrbInterpolationMode) => {
    draft.value.multiPrb = { ...(draft.value.multiPrb ?? { files: [], machNumbers: [] }), interpolationMode: v }
  },
})
const interpolationAlgorithm = computed<InterpolationAlgorithm>({
  get: () => draft.value.interpolationAlgorithm ?? 'old',
  set: (v: InterpolationAlgorithm) => { draft.value.interpolationAlgorithm = v },
})
const calibrationCsvFile = computed<CalibrationCsvFileInfo | null>({
  get: () => draft.value.calibrationCsvFile ?? null,
  set: (v: CalibrationCsvFileInfo | null) => { draft.value.calibrationCsvFile = v },
})
const savePath = bindDraft('savePath')
const saveFileName = bindDraft('saveFileName')
const saveOptions = bindDraft('saveOptions')
const motionSafety = bindDraft('motionSafety')

// prbMode：派生自 useMultiPrb，保存时同步回 draft
const prbMode = computed<'single' | 'multi'>({
  get: () => draft.value.useMultiPrb ? 'multi' : 'single',
  set: (v: 'single' | 'multi') => { draft.value.useMultiPrb = v === 'multi' },
})

// 探针通道与运动轴：直接读写 draft.channels 子字段
const probeChannels = computed<ProbeChannelConfig[]>({
  get: () => draft.value.channels.probeChannels,
  set: (v: ProbeChannelConfig[]) => { draft.value.channels.probeChannels = v },
})
const motionAxes = computed<TraversalMotionAxisConfig[]>({
  get: () => draft.value.channels.motionAxes,
  set: (v: TraversalMotionAxisConfig[]) => { draft.value.channels.motionAxes = v },
})

// 布局子字段：lineConfig / rectangleConfig / sectorConfig / customPoints / pattern / snakeOrder / primaryAxis
const pattern = computed<TraversalPattern>({
  get: () => draft.value.layout.pattern,
  set: (v: TraversalPattern) => { draft.value.layout.pattern = v },
})
const snakeOrder = computed<boolean>({
  get: () => draft.value.layout.snakeOrder ?? false,
  set: (v: boolean) => { draft.value.layout.snakeOrder = v },
})
const primaryAxis = computed<TraversalPrimaryAxis>({
  get: () => draft.value.layout.primaryAxis ?? 'x',
  set: (v: TraversalPrimaryAxis) => { draft.value.layout.primaryAxis = v },
})

// 布局子对象直接读写 draft.layout.xxx（v-model 需要 mutable 对象引用）
const lineConfig = computed({
  get: () => draft.value.layout.line!,
  set: (v) => { draft.value.layout.line = v },
})
const rectangleConfig = computed({
  get: () => draft.value.layout.rectangle!,
  set: (v) => { draft.value.layout.rectangle = v },
})
const sectorConfig = computed({
  get: () => draft.value.layout.sector!,
  set: (v) => { draft.value.layout.sector = v },
})
const customPoints = computed({
  get: () => draft.value.layout.custom?.points ?? [],
  set: (v) => { draft.value.layout.custom = { points: v } },
})
const customPointInput = ref({ x: 0, y: 0, z: 0, u: 0 })

// 七孔 PRB 编辑态（每 probe 独立，tab 切换自动重定向）
const sevenHolePrbDraft = computed<SevenHolePrbDraft>({
  get: () => sevenDraft.value,
  set: (v: SevenHolePrbDraft) => { sevenHolePrbDrafts[activeTab.value] = v },
})

async function runPrbOperation<T>(operation: () => Promise<T>): Promise<T> {
  pendingOperationCount.value++
  try {
    return await operation()
  } finally {
    pendingOperationCount.value--
  }
}

const prbOperations = computed<TraversalPrbOperations>(() => {
  const probeId = activeTab.value
  return {
    getError: () => dualStore.sessions[probeId].error,
    importPrbFile: (filePath) => runPrbOperation(() => dualStore.importPrbFile(probeId, filePath)),
    importMultiPrbFiles: (filePaths, machNumbers, mode) => runPrbOperation(
      () => dualStore.importMultiPrbFiles(probeId, filePaths, machNumbers, mode),
    ),
    importCalibrationCsvFile: (filePath) => runPrbOperation(
      () => dualStore.importCalibrationCsvFile(probeId, filePath),
    ),
    importSevenHolePrbFiles: (inner, outer) => runPrbOperation(
      () => dualStore.importSevenHolePrbFiles(probeId, inner, outer),
    ),
    importSevenHoleCalibrationCsvFiles: (inner, outer) => runPrbOperation(
      () => dualStore.importSevenHoleCalibrationCsvFiles(probeId, inner, outer),
    ),
    clearInterpolator: (type) => runPrbOperation(() => dualStore.clearInterpolator(probeId, type)),
  }
})

// 预览画布横/纵轴选择（UI 临时状态，不进入持久化）
const previewHAxis = ref<'x' | 'y' | 'z' | 'u'>('x')
const previewVAxis = ref<'x' | 'y' | 'z' | 'u'>('y')

// 探针类型选项
const probeTypeOptions = computed(() => [
  { value: 'five-hole' as TraversalProbeType, label: t.value.travProbeTypeFiveHole },
  { value: 'seven-hole' as TraversalProbeType, label: t.value.travProbeTypeSevenHole },
])
const pProbePressureTypeOptions = computed(() => [
  { value: 'gauge', label: t.value.travPressureTypeGauge },
  { value: 'absolute', label: t.value.travPressureTypeAbsolute },
])

// 探针类型切换：双探针模式不做双变体暂存，直接重置通道/PRB（与 single 模式差异点）
function onProbeTypeChange(next: TraversalProbeType): void {
  if (next === probeType.value) return
  probeType.value = next
  // 切换探针类型时重置通道到对应预设，避免遗留不兼容绑定
  probeChannels.value = next === 'seven-hole'
    ? createSevenHoleTraversalProbeChannels()
    : createDefaultTraversalProbeChannels()
  // 七孔 PRB draft 同步重置
  sevenHolePrbDrafts[activeTab.value] = { source: 'prb', innerFile: null, outerFiles: [null, null, null, null, null, null] }
}

// 标题：根据活动 probe 的探针类型显示
const wizardTitleText = computed(() => {
  // probeType 为 optional 字段；UI 展示用 fallback 'five-hole' 兜底，避免 undefined 索引
  const presentation = TRAVERSAL_PROBE_PRESENTATION[probeType.value ?? 'five-hole']
  const key = presentation.titleKey as keyof typeof t.value
  return (t.value[key] as string | undefined) ?? t.value.dualSettingsTitle
})

// ---------------------------------------------------------------------------
// 估算点位数（摘要步骤展示）
// ---------------------------------------------------------------------------
const estimatedPointCount = computed(() => getTraversalLayoutPointCount(draft.value.layout))

const rectangleHasArea = computed(() => {
  if (pattern.value !== 'rectangle') return true
  const r = draft.value.layout.rectangle
  if (!r) return true
  const xRange = deriveRangeFromSegments(r.xStepSegments)
  const yRange = deriveRangeFromSegments(r.yStepSegments)
  return xRange.max > xRange.min && yRange.max > yRange.min
})

// ---------------------------------------------------------------------------
// 步骤校验（与 TraversalSettings 对齐，但仅当前 probe 自身校验）
// ---------------------------------------------------------------------------
const hasDuplicateChannelFlag = computed(() => hasDuplicateChannel(probeChannels.value))
const axisKindIssues = computed(() => findTraversalAxisKindIssues(motionAxes.value, motionStore.profiles, pattern.value))

const isStepValid = computed(() => {
  if (axisKindIssues.value.length > 0 && currentStep.value >= 2) return false
  if (currentStep.value === 0) {
    if (hasDuplicateChannelFlag.value) return false
    return probeChannels.value.filter((c) => c.enabled).every((c) => c.channel.deviceId !== '' && c.channel.channelIndex >= 0)
  }
  if (currentStep.value === 1) {
    if (probeType.value === 'seven-hole') {
      const d = sevenDraft.value
      return d.innerFile !== null && d.outerFiles.every((f) => f !== null)
    }
    if (interpolationAlgorithm.value === 'new') return calibrationCsvFile.value !== null
    if (prbMode.value === 'multi') return multiPrbFiles.value.length > 0
    return prbFile.value !== null
  }
  if (currentStep.value === 2) {
    const axesToValidate = pattern.value === 'line'
      ? motionAxes.value.filter((a) => a.name === 'X')
      : pattern.value === 'custom'
        ? motionAxes.value
        : motionAxes.value.filter((a) => a.name === 'X' || a.name === 'Y')
    const noDuplicateBinding = !hasDuplicateMotionAxis(axesToValidate)
    return testName.value.trim() !== '' && estimatedPointCount.value > 0 && rectangleHasArea.value &&
      noDuplicateBinding &&
      axesToValidate.every((a) => a.controllerId !== '')
  }
  if (currentStep.value === steps.value.length - 1) return savePath.value.trim() !== '' && saveFileName.value.trim() !== ''
  return true
})

// 步骤校验失败的具体原因（FR2：disabled 按钮必须显式提示，避免用户困惑为何无法点下一步）。
// 与 isStepValid 同源检查，按"用户最先能修复的项"排序返回首条原因。
const stepInvalidReason = computed<string>(() => {
  if (currentStep.value === 0) {
    if (hasDuplicateChannelFlag.value) return t.value.dualStepInvalidProbeChannel
    const unconfigured = probeChannels.value.filter((c) => c.enabled && (c.channel.deviceId === '' || c.channel.channelIndex < 0))
    if (unconfigured.length > 0) return t.value.dualStepInvalidProbeChannel
    return ''
  }
  if (currentStep.value === 1) {
    if (probeType.value === 'seven-hole') {
      const d = sevenDraft.value
      if (d.innerFile === null || d.outerFiles.some((f) => f === null)) return t.value.dualStepInvalidPrbSevenHole
      return ''
    }
    if (interpolationAlgorithm.value === 'new') {
      return calibrationCsvFile.value === null ? t.value.dualStepInvalidCalibrationCsv : ''
    }
    if (prbMode.value === 'multi') {
      return multiPrbFiles.value.length === 0 ? t.value.dualStepInvalidMultiPrb : ''
    }
    return prbFile.value === null ? t.value.dualStepInvalidPrbFile : ''
  }
  if (currentStep.value === 2) {
    if (axisKindIssues.value.length > 0) return t.value.dualStepInvalidAxisKind
    if (testName.value.trim() === '') return t.value.dualStepInvalidName
    if (estimatedPointCount.value <= 0) return t.value.dualStepInvalidNoPoints
    if (!rectangleHasArea.value) return t.value.dualStepInvalidRectangleArea
    const axesToValidate = pattern.value === 'line'
      ? motionAxes.value.filter((a) => a.name === 'X')
      : pattern.value === 'custom'
        ? motionAxes.value
        : motionAxes.value.filter((a) => a.name === 'X' || a.name === 'Y')
    if (hasDuplicateMotionAxis(axesToValidate)) return t.value.dualStepInvalidDuplicateAxis
    const emptyAxes = axesToValidate.filter((a) => a.controllerId === '').map((a) => a.name).join('/')
    if (emptyAxes) return t.value.dualStepInvalidControllerEmpty.replace('{axes}', emptyAxes)
    return ''
  }
  if (currentStep.value === steps.value.length - 1) {
    if (savePath.value.trim() === '' || saveFileName.value.trim() === '') return t.value.dualStepInvalidSavePath
    return ''
  }
  return ''
})

// ---------------------------------------------------------------------------
// 跨 probe 控制器冲突检测（spec FR2：保存前原子校验）
// ---------------------------------------------------------------------------
// 当前 probe 启用的运动控制器 ID 集合（line 模式仅 X，rectangle/sector 仅 X+Y，
// custom 全部 4 轴；与 isStepValid 第 2 步校验范围对齐，避免误判 Z/U 冲突）
function activeControllerIds(cfg: TraversalTestConfig): Set<string> {
  const ids = new Set<string>()
  const pat = cfg.layout.pattern
  const axes = cfg.channels.motionAxes
  const filtered = pat === 'line'
    ? axes.filter((a) => a.name === 'X')
    : pat === 'custom'
      ? axes
      : axes.filter((a) => a.name === 'X' || a.name === 'Y')
  for (const a of filtered) {
    if (a.controllerId) ids.add(a.controllerId)
  }
  return ids
}

// 当前 probe 的控制器是否为空（保存前阻断）
const currentControllerEmpty = computed(() => {
  const ids = activeControllerIds(draft.value)
  return ids.size === 0
})

// 跨 probe 冲突：当前 probe 与另一 probe 的控制器 ID 有交集
const controllerConflict = computed(() => {
  const currentIds = activeControllerIds(draft.value)
  if (currentIds.size === 0) return false
  const otherIds = activeControllerIds(draft.value === drafts.probe1 ? drafts.probe2 : drafts.probe1)
  for (const id of currentIds) {
    if (otherIds.has(id)) return true
  }
  return false
})

// 冲突原因（用于 toast 与 UI 提示）
const conflictReason = computed(() => {
  if (currentControllerEmpty.value) {
    return t.value.dualControllerEmpty.replace('{probeId}', activeTab.value === 'probe1' ? t.value.probe1Label : t.value.probe2Label)
  }
  if (controllerConflict.value) {
    return t.value.dualControllerConflict
  }
  return ''
})

// ---------------------------------------------------------------------------
// 加载与保存
// ---------------------------------------------------------------------------
async function loadDraft(probeId: ProbeId): Promise<void> {
  await dualStore.loadConfig(probeId)
  const loaded = dualStore.sessions[probeId].config
  if (loaded) {
    // 深拷贝避免修改 store 内对象，保证编辑隔离
    drafts[probeId] = JSON.parse(JSON.stringify(loaded)) as TraversalTestConfig
    // 七孔 PRB draft 从 sevenHolePrb 恢复
    if (loaded.sevenHolePrb) {
      sevenHolePrbDrafts[probeId] = {
        source: loaded.sevenHolePrb.kind === 'seven-hole-calibration-csv' ? 'calibration-csv' : 'prb',
        innerFile: { ...loaded.sevenHolePrb.innerFile },
        outerFiles: loaded.sevenHolePrb.outerFiles.map((f) => ({ ...f })),
      }
    }
    // 规范化 layout 范围（与 single 模式 loadConfig 一致）
    drafts[probeId].layout = normalizeTraversalLayoutRanges(drafts[probeId].layout)
  }
}

function buildSavePayload(probeId: ProbeId): TraversalTestConfig {
  const d = drafts[probeId]
  // 深拷贝避免持久化对象与编辑态共享引用
  const payload: TraversalTestConfig = JSON.parse(JSON.stringify(d))
  // 七孔 PRB draft → sevenHolePrb 持久化字段
  if (probeType.value === 'seven-hole' && sevenHolePrbDrafts[probeId].innerFile && sevenHolePrbDrafts[probeId].outerFiles.every((f) => f !== null)) {
    const draft = sevenHolePrbDrafts[probeId]
    // 6 个扇区文件已通过 every 校验为非空，类型断言为 SevenHolePrbFileInfo[] 以匹配持久化结构
    const outerFiles = draft.outerFiles.map((f) => ({ ...(f as SevenHolePrbFileInfo) })) as [
      SevenHolePrbFileInfo, SevenHolePrbFileInfo, SevenHolePrbFileInfo,
      SevenHolePrbFileInfo, SevenHolePrbFileInfo, SevenHolePrbFileInfo
    ]
    payload.sevenHolePrb = {
      kind: draft.source === 'calibration-csv' ? 'seven-hole-calibration-csv' : 'seven-hole-prb-set',
      innerFile: { ...draft.innerFile! },
      outerFiles,
    }
  } else {
    payload.sevenHolePrb = undefined
  }
  // 清洗 saveFileName
  payload.saveFileName = buildCalibrationCsvName(payload.saveFileName.replace(/\.csv$/i, ''), payload.name)
  return payload
}

async function pickSavePath(): Promise<void> {
  try {
    const p = await storageStore.pickDirectory()
    if (p) savePath.value = p
  } catch (e) {
    feedbackStore.pushToast(t.value.failedChooseDirectory + '：' + (e instanceof Error ? e.message : String(e)), 'error')
  }
}

// MotionSafetyPanel 实例引用（保存前调用 isValid 阻断非法配置）
const motionSafetyPanelRef = ref<InstanceType<typeof MotionSafetyPanel> | null>(null)

async function saveConfig(): Promise<void> {
  // 保存前原子校验：当前 probe 控制器非空 + 不与另一 probe 冲突
  if (currentControllerEmpty.value || controllerConflict.value) {
    feedbackStore.pushToast(conflictReason.value, 'error')
    return
  }
  // 运动安全配置校验（与 single 模式一致）
  const safetyPanel = motionSafetyPanelRef.value
  if (safetyPanel && !safetyPanel.isValid) {
    const firstErr = safetyPanel.blockingErrors[0] ?? t.value.travMotionSafetyErrCriticalGreaterThanArrival
    feedbackStore.pushToast(firstErr, 'error')
    return
  }
  isSaving.value = true
  try {
    const payload = buildSavePayload(activeTab.value)
    const ok = await dualStore.saveConfig(activeTab.value, payload)
    if (!ok) {
      feedbackStore.pushToast(
        `${activeTab.value === 'probe1' ? t.value.probe1Label : t.value.probe2Label} ${t.value.dualSaveFailed}：${dualStore.sessions[activeTab.value].error ?? ''}`,
        'error',
      )
      return
    }
    feedbackStore.pushToast(t.value.dualSaveSuccess, 'success')
    emit('saved', activeTab.value)
    emit('close')
  } finally {
    isSaving.value = false
  }
}

// ---------------------------------------------------------------------------
// 对话框打开时加载两路配置 + 设备/运动/存储 profile
// ---------------------------------------------------------------------------
watch(() => props.show, async (isVisible) => {
  if (!isVisible) return
  isLoading.value = true
  currentStep.value = 0
  visitedSteps.value = new Set([0])
  activeTab.value = props.probeId
  try {
    const results = await Promise.allSettled([
      deviceStore.refreshProfiles(),
      motionStore.refreshProfiles(),
      storageStore.loadSettings(),
      loadDraft('probe1'),
      loadDraft('probe2'),
    ])
    reportAllSettledFailures(
      results,
      [t.value.travErrDeviceList, t.value.travErrMotionList, t.value.travErrStorage, t.value.travErrTraversalConfig, t.value.travErrTraversalConfig],
      feedbackStore.pushToast,
    )
    // 兜底默认保存路径
    if (!drafts.probe1.savePath.trim()) drafts.probe1.savePath = storageStore.settings?.baseDirectory?.trim() ?? ''
    if (!drafts.probe2.savePath.trim()) drafts.probe2.savePath = storageStore.settings?.baseDirectory?.trim() ?? ''
  } finally {
    isLoading.value = false
  }
}, { immediate: true })

// ---------------------------------------------------------------------------
// Tab 切换：重置步骤导航到 0（让用户从开头查看新 probe 配置；
// 已访问步骤记录清空，避免跳转到未在新 probe 验证过的步骤）
// ---------------------------------------------------------------------------
function onTabSwitch(next: ProbeId): void {
  if (next === activeTab.value || isOperationPending.value) return
  activeTab.value = next
  currentStep.value = 0
  visitedSteps.value = new Set([0])
}

const tabOptions = computed<{ value: ProbeId; label: string }[]>(() => [
  { value: 'probe1', label: t.value.probe1Label },
  { value: 'probe2', label: t.value.probe2Label },
])
</script>

<template>
  <UiDialog :show="props.show" width="min(92vw, 960px)" closable @close="emit('close')">
    <template #header>
      <div class="dual-settings-header">
        <span class="setup-overline">{{ t.dualSettingsTitle }}</span>
        <span class="setup-title">{{ wizardTitleText }}</span>
        <span class="setup-hint">{{ t.dualSettingsTabHint }}</span>
      </div>
    </template>

    <!-- Tab 切换：probe1 / probe2 -->
    <div class="dual-settings-tabs" role="tablist" :aria-label="t.dualSettingsTitle">
      <button
        v-for="tab in tabOptions"
        :key="tab.value"
        type="button"
        role="tab"
        :disabled="isOperationPending"
        :aria-selected="activeTab === tab.value"
        :class="['dual-settings-tab', { 'dual-settings-tab--active': activeTab === tab.value }]"
        @click="onTabSwitch(tab.value)"
      >
        {{ tab.label }}
      </button>
    </div>

    <!-- 跨 probe 控制器冲突提示（保存前阻断） -->
    <div
      v-if="currentControllerEmpty || controllerConflict"
      class="dual-settings-conflict"
      role="alert"
    >
      <span class="dual-settings-conflict-icon" aria-hidden="true">⚠</span>
      <span class="dual-settings-conflict-text">{{ conflictReason }}</span>
    </div>

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

    <div class="dual-settings-body">
      <div class="dual-settings-main">
        <!-- 步骤 0：硬件（探针类型 + 压力类型 + 通道 + 运动安全） -->
        <div v-if="currentStep === 0" class="step-content">
          <div class="pressure-type-bar">
            <div class="pressure-type-label">
              <span class="pressure-type-title">{{ t.travProbeType }}</span>
              <span class="pressure-type-hint">{{ t.travProbeTypeHint }}</span>
            </div>
            <UiSelect
              :model-value="probeType"
              :options="probeTypeOptions"
              size="sm"
              class="pressure-type-select"
              :aria-label="t.travProbeType"
              @update:model-value="(v: string) => onProbeTypeChange(v === 'seven-hole' ? 'seven-hole' : 'five-hole')"
            />
          </div>
          <div class="pressure-type-bar">
            <div class="pressure-type-label">
              <span class="pressure-type-title">{{ t.travPressureType }}</span>
              <span class="pressure-type-hint">{{ t.travPressureTypeHint }}</span>
            </div>
            <UiSelect
              :model-value="pProbePressureType"
              :options="pProbePressureTypeOptions"
              size="sm"
              class="pressure-type-select"
              :aria-label="t.travPressureType"
              @update:model-value="(v: string) => pProbePressureType = (v === 'absolute' ? 'absolute' : 'gauge')"
            />
          </div>
          <TraversalHardwareStep
            v-model:probe-channels="probeChannels"
            :t="(t as unknown as Record<string, string>)"
            :is-loading="isLoading"
          />
          <MotionSafetyPanel
            ref="motionSafetyPanelRef"
            v-model:motion-safety="motionSafety"
            :motion-axes="motionAxes"
            :t="(t as unknown as Record<string, string>)"
          />
        </div>

        <!-- 步骤 1：PRB -->
        <TraversalPrbStep
          v-else-if="currentStep === 1"
          v-model:probe-type="probeType"
          v-model:seven-hole-prb-draft="sevenHolePrbDraft"
          v-model:prb-mode="prbMode"
          v-model:interpolation-algorithm="interpolationAlgorithm"
          v-model:prb-file="prbFile"
          v-model:multi-prb-files="multiPrbFiles"
          v-model:multi-prb-mach-numbers="multiPrbMachNumbers"
          v-model:multi-prb-interpolation-mode="multiPrbInterpolationMode"
          v-model:calibration-csv-file="calibrationCsvFile"
          :t="(t as unknown as Record<string, string>)"
          :operations="prbOperations"
        />

        <!-- 步骤 2：布点 -->
        <TraversalLayoutStep
          v-else-if="currentStep === 2"
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
          v-model:motion-axes="motionAxes"
          :estimated-point-count="estimatedPointCount"
          :t="(t as unknown as Record<string, string>)"
        />

        <!-- 步骤 3：摘要 + 保存选项 -->
        <div v-else class="step-content">
          <UiPanel class="section-card">
            <template #header><span class="summary-section-title">{{ t.summaryTitle }}</span></template>
            <div class="summary-grid">
              <div class="summary-row"><span class="summary-label">{{ t.name }}</span><span>{{ testName }}</span></div>
              <div class="summary-row"><span class="summary-label">{{ t.travProbeType }}</span><span>{{ probeType === 'seven-hole' ? t.travProbeTypeSevenHole : t.travProbeTypeFiveHole }}</span></div>
              <div class="summary-row"><span class="summary-label">{{ t.pattern }}</span><span>{{ pattern }}</span></div>
              <div class="summary-row"><span class="summary-label">{{ t.estimatedPoints }}</span><span class="summary-accent">{{ estimatedPointCount }}</span></div>
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
            <template #header><span class="summary-section-title">{{ t.saveOptionsTitle }}</span></template>
            <div class="save-options">
              <label class="option-label"><UiCheckbox v-model:checked="saveOptions.savePointId" size="small" />{{ t.savePointId }}</label>
              <label class="option-label"><UiCheckbox v-model:checked="saveOptions.saveTimestamp" size="small" />{{ t.saveTimestamp }}</label>
              <label class="option-label"><UiCheckbox v-model:checked="saveOptions.saveRawPressure" size="small" />{{ t.saveRawPressure }}</label>
              <label class="option-label"><UiCheckbox v-model:checked="saveOptions.saveCalculatedResult" size="small" />{{ t.saveCalculatedResult }}</label>
            </div>
          </UiPanel>
        </div>
      </div>

      <!-- 侧栏：统计 + 点位预览 -->
      <aside class="dual-settings-sidebar">
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
          <PointsPreview v-model:h-axis="previewHAxis" v-model:v-axis="previewVAxis" :layout="draft.layout" />
        </div>
      </aside>
    </div>

    <template #footer>
      <div class="footer-actions">
        <div class="footer-left">
          <UiButton v-if="currentStep > 0" variant="secondary" size="sm" @click="prevStep">{{ t.previous }}</UiButton>
          <!-- 步骤校验失败原因：FR2 要求 disabled 按钮必须显式提示，避免用户困惑为何无法继续 -->
          <span v-if="stepInvalidReason" class="footer-invalid-reason" role="alert">
            <span class="footer-invalid-reason-icon" aria-hidden="true">⚠</span>
            <span class="footer-invalid-reason-text">{{ stepInvalidReason }}</span>
          </span>
        </div>
        <div class="footer-right">
          <UiButton variant="secondary" size="sm" @click="emit('close')">{{ t.cancel }}</UiButton>
          <UiButton v-if="currentStep < steps.length - 1" variant="primary" size="sm" :disabled="!isStepValid" @click="nextStep">{{ t.next }}</UiButton>
          <UiButton
            v-else
            size="sm"
            variant="primary"
            :loading="isSaving"
            :disabled="!isStepValid || currentControllerEmpty || controllerConflict"
            @click="saveConfig"
          >
            {{ isSaving ? t.saving : t.saveConfig }}
          </UiButton>
        </div>
      </div>
    </template>
  </UiDialog>
</template>

<style scoped>
.dual-settings-header {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.setup-overline {
  font-size: var(--text-xs);
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.2em;
  color: var(--text-tertiary);
}

.setup-title {
  font-size: 16px;
  font-weight: 600;
}

.setup-hint {
  font-size: var(--text-xs);
  color: var(--text-tertiary);
  margin-top: 2px;
}

.dual-settings-tabs {
  display: flex;
  gap: 4px;
  border-bottom: 1px solid var(--border-default, #e0e0e0);
  margin-bottom: var(--space-2);
  flex: 0 0 auto;
}

.dual-settings-tab {
  padding: 6px 16px;
  background: transparent;
  border: none;
  border-bottom: 2px solid transparent;
  color: var(--text-secondary, #5a5a5a);
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  transition: color 0.15s, border-color 0.15s;
}

.dual-settings-tab:hover {
  color: var(--text-primary, #1f1f1f);
}

.dual-settings-tab--active {
  color: var(--color-primary, #2080f0);
  border-bottom-color: var(--color-primary, #2080f0);
  font-weight: 600;
}

.dual-settings-conflict {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  background: var(--color-warning-bg, #fff8e1);
  color: var(--color-warning, #f0a020);
  border: 1px solid var(--color-warning, #f0a020);
  border-radius: 6px;
  font-size: 13px;
  margin-bottom: var(--space-2);
}

.dual-settings-conflict-icon {
  font-weight: 700;
  flex-shrink: 0;
}

.dual-settings-conflict-text {
  overflow: hidden;
  text-overflow: ellipsis;
}

.dual-settings-body {
  display: grid;
  grid-template-columns: minmax(420px, 1fr) 280px;
  gap: 0;
  min-height: 0;
  height: 60vh;
  flex: 1;
  overflow: hidden;
}

.dual-settings-main {
  min-height: 0;
  height: 60vh;
  overflow-y: auto;
  padding-right: var(--space-2);
  scrollbar-width: thin;
}

.step-content {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.section-card {
  font-size: var(--text-sm);
}

.section-card :deep(.n-card__content) {
  padding: 4px 8px;
}

.section-card :deep(.n-card-header) {
  padding: 4px 8px;
}

.pressure-type-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2);
  flex-wrap: wrap;
  padding: 4px 8px;
  border-radius: var(--radius-md);
  border: 1px solid var(--border-default);
  background: var(--bg-panel);
}

.pressure-type-label {
  display: flex;
  align-items: baseline;
  gap: var(--space-2);
  flex: 1;
  min-width: 0;
}

.pressure-type-title {
  font-size: var(--text-sm);
  font-weight: 500;
  color: var(--text-primary);
  white-space: nowrap;
}

.pressure-type-hint {
  font-size: var(--text-xs);
  color: var(--text-tertiary);
  line-height: 1.3;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.pressure-type-select {
  width: 120px;
  flex-shrink: 0;
}

.summary-grid {
  display: flex;
  flex-direction: column;
}

.summary-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 4px 0;
  border-bottom: 1px solid var(--border-default);
  gap: var(--space-3);
  font-size: var(--text-sm);
}

.summary-row:last-child {
  border-bottom: none;
}

.summary-label {
  color: var(--text-tertiary);
}

.summary-accent {
  color: var(--color-accent);
  font-weight: 700;
}

.save-row {
  display: flex;
  gap: 6px;
}

.save-options {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--space-1);
}

.option-label {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 2px var(--space-2);
  border-radius: var(--radius-md);
  font-size: var(--text-sm);
  color: var(--text-primary);
  cursor: pointer;
  transition: background 120ms ease;
}

.option-label:hover {
  background: var(--bg-panel-strong);
}

.dual-settings-sidebar {
  border-left: 1px solid var(--border-default);
  background: var(--bg-panel-strong);
  padding-left: var(--space-2);
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  height: 60vh;
  overflow-y: auto;
  scrollbar-width: thin;
}

.sidebar-stats {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: var(--space-2);
}

.sidebar-stat {
  padding: 6px 4px;
  border-radius: var(--radius-md);
  border: 1px solid var(--border-default);
  background: var(--bg-panel);
  text-align: center;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
}

.sidebar-stat--highlight {
  border-color: var(--color-primary, #3b82f6);
  background: linear-gradient(180deg, var(--bg-panel) 0%, rgba(59, 130, 246, 0.06) 100%);
}

.sidebar-stat--highlight .stat-number {
  color: var(--color-primary, #3b82f6);
}

.sidebar-preview {
  flex: 1 1 auto;
  min-height: 140px;
  max-height: 320px;
  border-radius: var(--radius-md);
  border: 1px solid var(--border-default);
  background: var(--bg-canvas);
  overflow: hidden;
}

.footer-actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  gap: var(--space-2);
}

.footer-left {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  flex: 1 1 auto;
  min-width: 0;
}

.footer-right {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  margin-left: auto;
  flex: 0 0 auto;
}

/* 步骤校验失败原因：紧贴 Previous 按钮右侧显示，红字 + 警告图标；
   用户立刻能看到为何 Next/Save 被 disabled，无需 hover tooltip（FR2 显式提示） */
.footer-invalid-reason {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: var(--color-error, #d03030);
  font-size: var(--text-xs, 12px);
  line-height: 1.3;
  min-width: 0;
}

.footer-invalid-reason-icon {
  flex-shrink: 0;
  font-weight: 700;
}

.footer-invalid-reason-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.steps-nav {
  margin-bottom: var(--space-2);
}

.summary-section-title {
  font-size: var(--text-sm);
  font-weight: 600;
}

.flex-input {
  flex: 1;
}

.save-hint {
  font-size: var(--text-xs);
  display: block;
  margin-top: 4px;
  color: var(--text-tertiary);
}

.stat-label {
  font-size: var(--text-xs);
  color: var(--text-tertiary);
}

.stat-number {
  font-size: 16px;
  font-weight: 700;
  color: var(--color-accent);
}

.stat-value {
  font-size: 12px;
  font-weight: 600;
}

.stat-unit {
  font-size: var(--text-xs);
}
</style>

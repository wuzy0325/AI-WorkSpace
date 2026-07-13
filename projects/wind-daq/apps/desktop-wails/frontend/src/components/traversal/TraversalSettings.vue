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
  deriveRangeFromSegments,
  getTraversalLayoutPointCount,
  getTraversalStepValues,
  hasDuplicateChannel,
  isTraversalConfigurableProbeChannel,
  isTraversalRequiredProbeChannel,
  normalizeTraversalLayoutRanges
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
  InterpolationAlgorithm
} from '@shared/types/traversal'
import UiPanel from '@components/ui/UiPanel.vue'
import UiCheckbox from '@components/ui/UiCheckbox.vue'
import UiInput from '@components/ui/UiInput.vue'
import UiInputNumber from '@components/ui/UiInputNumber.vue'
import UiSelect from '@components/ui/UiSelect.vue'
import UiDialog from '@components/ui/UiDialog.vue'
import UiStep from '@components/ui/UiStep.vue'
import UiSteps from '@components/ui/UiSteps.vue'
import PointsPreview from './PointsPreview.vue'
import TraversalLayoutStep from './TraversalLayoutStep.vue'
import TraversalHardwareStep from './TraversalHardwareStep.vue'
import TraversalPrbStep from './TraversalPrbStep.vue'
import { reportAllSettledFailures } from '@utils/allSettledReport'
// 复用校准模块共享的 CSV 文件名清洗工具：与三孔/五孔/总压/总温校准画面统一行为，
// 避免在移位测试画面重复维护一份"非法字符过滤 + 日期去重 + Windows 保留名兜底"逻辑。
import { buildCalibrationCsvName } from '@shared/calibrationCsvPath'

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

const isLoading = ref(true)
const isSaving = ref(false)
const currentStep = ref(0)
// 步骤顺序：通道 → prb → 布点 → 摘要。
// 调整原因：多数用户每次都用默认布点，但通道和探针配置每次必做。
// 把布点挪到第3步后，用户可在前两步完成核心配置后直接 Next 通过默认布点，
// 避免每次打开配置画面先看到不关心的布点界面。
// 注意：isStepValid 和模板 v-if 分支的 currentStep 索引与此处顺序强绑定，改动需同步。
const steps = computed(() => [t.value.stepHardware, t.value.stepPrb, t.value.stepLayout, t.value.stepReview])

// 记录用户已访问过的步骤索引，用于支持步骤导航点击跳转
const visitedSteps = ref<Set<number>>(new Set([0]))

// 测试名默认带 ISO 日期（YYYY-MM-DD），与三孔/五孔校准画面一致。
// 早期用 toLocaleDateString() 在中文环境返回含斜杠的 "2026/7/10"，会被 buildCalibrationCsvName
// 当作路径分隔符清洗掉，导致文件名缺失日期段。直接用 ISO 切片避免本地化差异。
const testName = ref(`Traversal-${new Date().toISOString().slice(0, 10)}`)
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
// 默认文件名复用校准模块共享工具：buildCalibrationCsvName 会自动检测 testName 末尾
// 已含 ISO 日期，不再追加，最终得到 "Traversal-2026-07-10.csv"。
const saveFileName = ref(buildCalibrationCsvName(testName.value, 'Traversal'))

// 蛇形扫描顺序：偶数行正向，奇数行反向，减少回程时间
const snakeOrder = ref(false)

// 走线主轴：新建 profile 默认 'x'（先沿 X 走完一条线再切换 Y），用户可切换为 'y'。
// 仅对 line / rectangle 布局生效；扇形布局不消费此字段。
// 注意：applySavedLayout 加载旧 profile（无 primaryAxis 字段）时显式落 'y' 保旧行为，
// 避免静默反转升级前已优化的物理走线方向。
const primaryAxis = ref<TraversalPrimaryAxis>('x')

// 预览画布横/纵轴选择：4 轴扩展后允许用户在 X/Y/Z/U 任意两轴组合间切换预览。
// 默认 'x'/'y' 与历史行为一致；custom 模式下若点位含非零 z/u，用户可切换到 X-Z / X-U 等组合查看。
// 这两个 ref 仅影响预览渲染，不进入 TraversalLayout 持久化（轴对选择是 UI 临时状态）。
const previewHAxis = ref<'x' | 'y' | 'z' | 'u'>('x')
const previewVAxis = ref<'x' | 'y' | 'z' | 'u'>('y')

// 五孔探针压力类型：'gauge' 表压（默认）/ 'absolute' 绝压。
// 后端 BuildRawPressure 归一化时按此字段决定 P1-P5 是否减去 Patm。
// 旧配置无此字段时 applySavedConfig 兜底为 'gauge'，与历史行为一致。
const pProbePressureType = ref<'gauge' | 'absolute'>('gauge')

// 压力类型下拉选项：label 随 i18n 切换刷新，故用 computed 派生
const pProbePressureTypeOptions = computed(() => [
  { value: 'gauge', label: t.value.travPressureTypeGauge },
  { value: 'absolute', label: t.value.travPressureTypeAbsolute }
])

const lineConfig = ref({
  startX: -30, startY: 0,
  endX: 30, endY: 0,
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

const customPoints = ref<Array<{ x: number; y: number; z: number; u: number }>>([])
const customPointInput = ref({ x: 0, y: 0, z: 0, u: 0 })
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
    case 'rectangle': {
      return normalizeTraversalLayoutRanges({
        pattern: 'rectangle',
        snakeOrder: snakeOrder.value,
        primaryAxis: primaryAxis.value,
        rectangle: rectangleConfig.value
      })
    }
    case 'sector': {
      // 扇形圆心不输入，默认“第一个测点 = 当前位置 = 坐标原点 (0,0)”。
      // 由第一个测点 (r₁, θ₁) 反推圆心，使第一点坐标 = (0,0)，
      // 后续点位相对第一点计算。装配时用户手动把探针定位到第一个测点。
      // radiusMin/radiusMax/angleStart/angleEnd 保留用户输入值（sector 模板有输入框），
      // 仅对 undefined/NaN（加载旧 profile 缺字段）走 segments 派生兜底。
      const rs = sectorConfig.value.radialStepSegments
      const as = sectorConfig.value.angularStepSegments
      const rRange = deriveRangeFromSegments(rs)
      const aRange = deriveRangeFromSegments(as)
      const radiusMin = Number.isFinite(sectorConfig.value.radiusMin) ? sectorConfig.value.radiusMin : rRange.min
      const radiusMax = Number.isFinite(sectorConfig.value.radiusMax) ? sectorConfig.value.radiusMax : rRange.max
      const angleStart = Number.isFinite(sectorConfig.value.angleStart) ? sectorConfig.value.angleStart : aRange.min
      const angleEnd = Number.isFinite(sectorConfig.value.angleEnd) ? sectorConfig.value.angleEnd : aRange.max
      const radii = getTraversalStepValues(radiusMin, radiusMax, rs)
      const angles = getTraversalStepValues(angleStart, angleEnd, as)
      const r1 = radii[0] ?? 0
      const t1 = ((angles[0] ?? 0) * Math.PI) / 180
      const hasData = radii.length > 0 && angles.length > 0
      const centerX = hasData ? -r1 * Math.cos(t1) : 0
      const centerY = hasData ? -r1 * Math.sin(t1) : 0
      return { pattern: 'sector', snakeOrder: snakeOrder.value, sector: { ...sectorConfig.value, radiusMin, radiusMax, angleStart, angleEnd, centerX, centerY } }
    }
    case 'custom': return { pattern: 'custom', snakeOrder: snakeOrder.value, custom: { points: customPoints.value } }
  }
})

const estimatedPointCount = computed(() => getTraversalLayoutPointCount(currentLayout.value))

const rectangleHasArea = computed(() => {
  if (pattern.value !== 'rectangle') return true
  const xRange = deriveRangeFromSegments(rectangleConfig.value.xStepSegments)
  const yRange = deriveRangeFromSegments(rectangleConfig.value.yStepSegments)
  return xRange.max > xRange.min && yRange.max > yRange.min
})

// 仅 line / rectangle 布局消费走线主轴；review 摘要只在支持时显示该行，避免重复条件字面量。
const supportsPrimaryAxis = computed(() => pattern.value === 'line' || pattern.value === 'rectangle')

// 步骤校验：索引与新顺序 [Hardware(0), Prb(1), Layout(2), Review(3)] 对齐。
// - 第0步 Hardware：探针通道必须绑定设备+通道号，无重复索引，运动轴必须绑定控制器
// - 第1步 Prb：新算法要求 CSV 文件；多 prb 模式要求至少1个 prb；单 prb 模式要求 prbFile
// - 第2步 Layout：测试名非空 + 估算点数>0 + rectangle 模式下区域面积>0
// - 最后一步 Review：保存路径和文件名非空
//
// 通道号重复检测：多个启用通道选了同一 channelIndex 时阻断保存。
// 后端 ParseConfig 对重复索引直接返回错误不启动测试（见 traversal_config.go channels 收集）。
// 前端在 isStepValid 中提前阻止进入下一步，避免"配置保存成功但测试启动报错"的体验断裂。
// 检测算法共享 shared/types/traversal.ts 的 hasDuplicateChannel，与 TraversalHardwareStep.vue 视觉提示共用同一真相源。
const hasDuplicateChannelFlag = computed(() => hasDuplicateChannel(probeChannels.value))

const isStepValid = computed(() => {
  if (currentStep.value === 0) {
    if (hasDuplicateChannelFlag.value) return false
    return probeChannels.value.filter((c) => c.enabled).every((c) => c.channel.deviceId !== '' && c.channel.channelIndex >= 0) &&
      motionAxes.value.every((a) => a.controllerId !== '')
  }
  if (currentStep.value === 1) {
    if (interpolationAlgorithm.value === 'new') return calibrationCsvFile.value !== null
    if (prbMode.value === 'multi') return multiPrbFiles.value.length > 0
    return prbFile.value !== null
  }
  if (currentStep.value === 2) return testName.value.trim() !== '' && estimatedPointCount.value > 0 && rectangleHasArea.value
  if (currentStep.value === steps.value.length - 1) return savePath.value.trim() !== '' && saveFileName.value.trim() !== ''
  return true
})

// testName 变化时同步刷新默认 saveFileName：仅当用户未手动修改（仍为空或等于上一默认值）时覆盖，
// 避免覆盖用户手动输入的文件名。复用共享工具保证清洗规则与校准画面一致。
watch(testName, (next, prev) => {
  const prevDefault = buildCalibrationCsvName(prev, 'Traversal')
  if (saveFileName.value === '' || saveFileName.value === prevDefault)
    saveFileName.value = buildCalibrationCsvName(next, 'Traversal')
})

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
  // primaryAxis 仅 rectangle 消费（UI 已对 line/sector/custom 隐藏主轴 radio），
  // 这里按持久化值恢复，用户切回 rectangle 时能找回原选择，不强制清空。
  primaryAxis.value = layout.primaryAxis ?? 'y'
  if (layout.line) {
    const legacyUsesY = layout.line.startX === layout.line.endX && layout.line.startY !== layout.line.endY
    const xSegments = legacyUsesY ? layout.line.yStepSegments : layout.line.xStepSegments
    lineConfig.value = {
      ...layout.line,
      startX: legacyUsesY ? layout.line.startY : layout.line.startX,
      endX: legacyUsesY ? layout.line.endY : layout.line.endX,
      startY: 0,
      endY: 0,
      xStepSegments: xSegments.map(s => ({ ...s })),
      yStepSegments: []
    }
  }
  if (layout.rectangle) {
    const normalized = normalizeTraversalLayoutRanges(layout).rectangle!
    rectangleConfig.value = {
      ...normalized,
      xStepSegments: normalized.xStepSegments.map(s => ({ ...s })),
      yStepSegments: normalized.yStepSegments.map(s => ({ ...s }))
    }
  }
  if (layout.sector) {
    // radiusMin/radiusMax/angleStart/angleEnd 同样兜底，避免旧 profile 缺字段导致校验失效
    const rRange = deriveRangeFromSegments(layout.sector.radialStepSegments)
    const aRange = deriveRangeFromSegments(layout.sector.angularStepSegments)
    sectorConfig.value = {
      ...layout.sector,
      radiusMin: Number.isFinite(layout.sector.radiusMin) ? layout.sector.radiusMin : rRange.min,
      radiusMax: Number.isFinite(layout.sector.radiusMax) ? layout.sector.radiusMax : rRange.max,
      angleStart: Number.isFinite(layout.sector.angleStart) ? layout.sector.angleStart : aRange.min,
      angleEnd: Number.isFinite(layout.sector.angleEnd) ? layout.sector.angleEnd : aRange.max,
      radialStepSegments: layout.sector.radialStepSegments.map(s => ({ ...s })),
      angularStepSegments: layout.sector.angularStepSegments.map(s => ({ ...s }))
    }
  }
  // 4 轴扩展后 customPoints 必须包含 z/u：从持久化 layout 读取时直接传 z/u，
  // 后端 traversal_config.go 已在反序列化时映射 Z/U，故 layout.custom.points 元素必含这两字段。
  // 不再用 z:0/u:0 兜底，否则 4 轴自定义点配置加载后会丢失 Z/U 数据。
  customPoints.value = (layout.custom?.points ?? []).map(p => ({ x: p.x, y: p.y, z: p.z, u: p.u }))
}

function applySavedConfig(config: TraversalTestConfig) {
  const c = JSON.parse(JSON.stringify(config)) as TraversalTestConfig
  testName.value = c.name; dwellTimeMs.value = c.dwellTimeMs; samplesPerPoint.value = c.samplesPerPoint
  // 持久化的 saveFileName 已带 .csv 后缀，buildCalibrationCsvName 不剥离后缀会
  // 把 ".csv" 当作普通字符串清洗后再次追加 .csv，得到 "xxx.csv-2026-07-10.csv"。
  // 先剥离 .csv 后缀再交给共享工具，与原 sanitizeFileNameStem 行为对齐。
  savePath.value = c.savePath; saveFileName.value = buildCalibrationCsvName(c.saveFileName.replace(/\.csv$/i, ''), c.name)
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

  // 恢复五孔压力类型：旧配置无此字段时兜底为 'gauge'（与历史行为一致）。
  // 显式校验值域，避免后端 ParseAndStartTraversal 兜底前 UI 显示脏值。
  pProbePressureType.value = c.pProbePressureType === 'absolute' ? 'absolute' : 'gauge'
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
    // 保存前再次清洗 saveFileName：用户可能手动输入了非法字符或未带 .csv 后缀。
    // 剥离 .csv 后缀后交给共享工具，fallback 用 testName 保证空文件名也能落到有意义的默认值。
    const normName = buildCalibrationCsvName(saveFileName.value.replace(/\.csv$/i, ''), testName.value)
    saveFileName.value = normName
    const useMulti = prbMode.value === 'multi' && multiPrbFiles.value.length > 0
    // 稳定化使用驻留时间（dwellTimeMs）：后端 waitForStabilization 在 stab==nil 或 mode=="fixed"
    // 时回退到 dwellTimeMs，因此前端不再单独保存 stabilization/validation 配置，
    // 旧 profile 持久化的字段会被后端忽略，无需迁移。
    const raw = {
      name: testName.value, layout: currentLayout.value,
      channels: { probeChannels: probeChannels.value.filter(c => c.enabled && isConfigurableProbeChannel(c)), motionAxes: motionAxes.value },
      prbFile: useMulti ? null : prbFile.value,
      multiPrb: useMulti ? { files: multiPrbFiles.value.map(f => clonePrbFileInfo(f)), machNumbers: multiPrbMachNumbers.value.map(n => Number(n)), interpolationMode: multiPrbInterpolationMode.value } : undefined,
      useMultiPrb: useMulti, interpolationAlgorithm: interpolationAlgorithm.value,
      calibrationCsvFile: interpolationAlgorithm.value === 'new' ? calibrationCsvFile.value : null,
      dwellTimeMs: dwellTimeMs.value,
      samplesPerPoint: samplesPerPoint.value,
      savePath: savePath.value.trim(), saveFileName: normName, saveOptions: saveOptions.value,
      // 五孔压力类型始终保存，后端 BuildRawPressure 归一化依据此字段
      pProbePressureType: pProbePressureType.value
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
      [t.value.travErrDeviceList, t.value.travErrMotionList, t.value.travErrStorage, t.value.travErrTraversalConfig],
      feedbackStore.pushToast,
    )
    if (!savePath.value.trim()) savePath.value = storageStore.settings?.baseDirectory?.trim() ?? ''
    if (!saveFileName.value.trim()) saveFileName.value = buildCalibrationCsvName(testName.value, 'Traversal')
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
        <div v-if="currentStep === 0" class="step-content">
          <!-- 五孔压力类型开关：与批量工具栏同处一行，避免额外卡片占用垂直空间 -->
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
            v-model:motion-axes="motionAxes"
            :t="(t as unknown as Record<string, string>)"
            :is-loading="isLoading"
          />
        </div>
        <TraversalPrbStep
          v-else-if="currentStep === 1"
          v-model:prb-mode="prbMode"
          v-model:interpolation-algorithm="interpolationAlgorithm"
          v-model:prb-file="prbFile"
          v-model:multi-prb-files="multiPrbFiles"
          v-model:multi-prb-mach-numbers="multiPrbMachNumbers"
          v-model:multi-prb-interpolation-mode="multiPrbInterpolationMode"
          v-model:calibration-csv-file="calibrationCsvFile"
          :t="(t as unknown as Record<string, string>)"
        />
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
          :estimated-point-count="estimatedPointCount"
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
          <PointsPreview v-model:h-axis="previewHAxis" v-model:v-axis="previewVAxis" :layout="currentLayout" />
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
/* 遍历测试配置画面主体布局：左右分栏，固定高度确保步骤切换时画面尺寸稳定。
   左侧主内容区设置最小宽度 420px，避免在窄视口或对话框缩小时文字被压成竖排。
   关键：使用 height: 60vh（而非 max-height），让内容少时主体保持固定高度、
   内容多时内部滚动，避免步骤切换时对话框整体高度跳动破坏视觉锚点。 */
.traversal-body {
  display: grid;
  grid-template-columns: minmax(420px, 1fr) 280px;
  gap: 0;
  min-height: 0;
  height: 60vh;
  flex: 1;
  overflow: hidden
}

.traversal-main {
  min-height: 0;
  height: 60vh;
  overflow-y: auto;
  padding-right: var(--space-2);
  scrollbar-width: thin
}

.step-content {
  display: flex;
  flex-direction: column;
  gap: 6px
}

.section-card {
  font-size: var(--text-sm)
}

/* 紧凑化 UiPanel 内边距：默认 var(--space-3) var(--space-4) 偏大，
   覆盖为 4px 8px 让摘要/保存/选项卡片更紧凑、与硬件步骤视觉对齐 */
.section-card :deep(.n-card__content) {
  padding: 4px 8px
}
.section-card :deep(.n-card-header) {
  padding: 4px 8px
}

/* 五孔压力类型开关：不再使用独立卡片，改为扁平工具条，与批量工具栏视觉同级。
   标题和提示水平排列，下拉固定右侧，整行高度与下方工具栏一致。 */
.pressure-type-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2);
  flex-wrap: wrap;
  padding: 4px 8px;
  border-radius: var(--radius-md);
  border: 1px solid var(--border-default);
  background: var(--bg-panel)
}

.pressure-type-label {
  display: flex;
  align-items: baseline;
  gap: var(--space-2);
  flex: 1;
  min-width: 0
}

.pressure-type-title {
  font-size: var(--text-sm);
  font-weight: 500;
  color: var(--text-primary);
  white-space: nowrap
}

.pressure-type-hint {
  font-size: var(--text-xs);
  color: var(--text-tertiary);
  line-height: 1.3;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap
}

.pressure-type-select {
  width: 120px;
  flex-shrink: 0
}

.summary-grid {
  display: flex;
  flex-direction: column
}

/* 摘要行：收紧垂直内边距，避免视觉松散 */
.summary-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 4px 0;
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

/* 保存选项标签：紧凑高度，与其他卡片行高对齐 */
.option-label {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 2px var(--space-2);
  border-radius: var(--radius-md);
  font-size: var(--text-sm);
  color: var(--text-primary);
  cursor: pointer;
  transition: background 120ms ease
}

.option-label:hover {
  background: var(--bg-panel-strong)
}

/* 右侧边栏：固定宽度，固定高度（与主体 60vh 对齐），内部可滚动；
   收紧宽度至 280px 减少视觉占用。
   使用 height 而非 max-height，确保不同步骤下边栏与主体等高、
   点位预览图尺寸稳定，避免横向对比时预览大小漂移。 */
.traversal-sidebar {
  border-left: 1px solid var(--border-default);
  background: var(--bg-panel-strong);
  padding-left: var(--space-2);
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  height: 60vh;
  overflow-y: auto;
  scrollbar-width: thin
}

.sidebar-stats {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: var(--space-2)
}

/* 侧栏统计卡片：收紧内边距，三卡同高对齐 */
.sidebar-stat {
  padding: 6px 4px;
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
  min-height: 140px;
  max-height: 320px;
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
.setup-title { font-size:16px;font-weight:600;margin-top:2px;display:block }
/* 步骤导航栏：减小下方间距，与内容区域更紧凑 */
.steps-nav { margin-bottom:var(--space-2) }
.summary-section-title { font-size:var(--text-sm);font-weight:600 }
.summary-accent { color:var(--color-accent);font-weight:700 }
.text-ellipsis { max-width:180px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap }
.flex-input { flex:1 }
.save-hint { font-size:var(--text-xs);display:block;margin-top:4px;color:var(--text-tertiary) }
.stat-label { font-size:var(--text-xs);color:var(--text-tertiary) }
.stat-number { font-size:16px;font-weight:700;color:var(--color-accent) }
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
</style>

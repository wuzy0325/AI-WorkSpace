<script setup lang="ts">
import { ref, computed, onMounted, watch, type Component } from 'vue'
import { storeToRefs } from 'pinia'
import { useCalibrationStore } from '@stores/calibrationStore'
import { useDeviceStore } from '@stores/deviceStore'
import { useMotionStore } from '@stores/motionStore'
import { useFeedbackStore } from '@stores/feedbackStore'
import { useI18nStore } from '@stores/i18nStore'
import { useStorageStore } from '@stores/storageStore'
import { buildCalibrationCsvName, joinCalibrationPath, splitCalibrationSavePath } from '@shared/calibrationCsvPath'
import { calibrationApi } from '@api/calibrationApi'
import type {
  CalibrationConfig,
  ProbeChannelConfig,
  MotionAxisConfig,
  ChannelRef,
} from '@shared/types/calibration'
import type { MotionSafetyConfig } from '@shared/types/traversal'
import {
  applyCalibrationPrecisionDefaults,
  DEFAULT_CALIBRATION_MACH_PRECISION,
  DEFAULT_CALIBRATION_PROBE_PRECISION,
  DEFAULT_CALIBRATION_VELOCITY_PRECISION,
} from '@shared/calibrationPrecision'
import { getProbeChannelDisplayName } from '@shared/calibrationChannelI18n'
import { generateFiveHoleSnakePoints } from './motionCalibrationUtils'
import MotionSafetyPanel from '@components/shared/MotionSafetyPanel.vue'
import UiAlert from '@components/ui/UiAlert.vue'
import UiCheckbox from '@components/ui/UiCheckbox.vue'
import UiDialog from '@components/ui/UiDialog.vue'
import UiInput from '@components/ui/UiInput.vue'
import UiInputNumber from '@components/ui/UiInputNumber.vue'
import UiPanel from '@components/ui/UiPanel.vue'
import UiSelect from '@components/ui/UiSelect.vue'
import UiSpin from '@components/ui/UiSpin.vue'
import UiStatusBadge from '@components/ui/UiStatusBadge.vue'
import UiStep from '@components/ui/UiStep.vue'
import UiSteps from '@components/ui/UiSteps.vue'
import {
  ChevronLeft,
  ChevronRight,
  LayoutGrid,
  Move3D,
  Save,
  ShieldCheck,
  FileText,
  Timer,
  Target,
  Activity,
  Wind,
  Gauge,
  Zap,
} from '@lucide/vue'
import UiButton from '@components/ui/UiButton.vue'
import { reportAllSettledFailures } from '@utils/allSettledReport'
import { useChannelBatchOperations } from '@composables/useChannelBatchOperations'

const emit = defineEmits<{
  close: []
  saved: [config: CalibrationConfig]
}>()

const deviceStore = useDeviceStore()
const motionStore = useMotionStore()
const calibrationStore = useCalibrationStore()
const feedbackStore = useFeedbackStore()
const storageStore = useStorageStore()
const { t } = storeToRefs(useI18nStore())

const isLoading = ref(true)
const isSaving = ref(false)
const currentStep = ref(0)
const steps = computed(() => [
  { label: t.value.stepBasic || '基本设置' },
  { label: t.value.stepHardware || '硬件配置' },
  { label: t.value.stepConfirm || '确认保存' },
])

const pointLayout = ref({
  alphaMin: -30,
  alphaMax: 30,
  alphaStep: 5,
  betaMin: -30,
  betaMax: 30,
  betaStep: 5,
  serpentine: false,
})

// 浮点容差：alphaMax-alphaMin 与整数倍 alphaStep 比较时容忍 1e-9 的累加误差
const FLOAT_EPSILON = 1e-9
function isRangeDivisible(range: number, step: number): boolean {
  return Math.abs(range / step - Math.round(range / step)) < FLOAT_EPSILON
}

/**
 * 点位布局校验单一真源
 *
 * 统一负责两件事：
 *   1) 给 pointCount / previewDots / currentStepErrors 提供一致的"有效性 + 点数"判定，
 *      避免出现"UI 显示 0 个点但又没有任何错误"这种反直觉状态。
 *   2) 把所有校验消息集中在一处，便于本地化与维护。
 *
 * 返回值：
 *   - valid 仅在所有规则通过时为 true；
 *   - count 始终给出"按当前参数渲染时应得的点数"，无效时为 0；
 *   - errors 收集本次校验失败的提示，按显示顺序排列。
 */
function validatePointLayout(): { valid: boolean; count: number; errors: string[] } {
  const { alphaMin, alphaMax, alphaStep, betaMin, betaMax, betaStep } = pointLayout.value
  const errors: string[] = []

  if (alphaStep <= 0 || betaStep <= 0) errors.push(t.value.stepMustPositive || '步长必须为正数')
  if (alphaMax <= alphaMin || betaMax <= betaMin) errors.push(t.value.maxGreaterThanMin || '最大值必须大于最小值')

  const alphaRange = alphaMax - alphaMin
  const betaRange = betaMax - betaMin
  const divisible = isRangeDivisible(alphaRange, alphaStep) && isRangeDivisible(betaRange, betaStep)
  if (!divisible) errors.push(t.value.rangeDivisible || '范围必须能被步长整除')

  let count = 0
  if (errors.length === 0) {
    // 整数迭代：保持 pointCount 与 previewDots 完全一致，避免浮点累加误差
    const alphaPoints = Math.round(alphaRange / alphaStep) + 1
    const betaPoints = Math.round(betaRange / betaStep) + 1
    count = alphaPoints * betaPoints
  }

  return { valid: errors.length === 0, count, errors }
}

const pointLayoutValidation = computed(() => validatePointLayout())

const pointCount = computed(() => pointLayoutValidation.value.count)

const dwellTimeMs = ref(2000)
const samplesPerPoint = ref(10)
const calibrationName = ref('')
// CSV 保存路径：自动校准启动时后端按此路径覆盖初始化 csvWriter，逐点实时写入，
// 崩溃/断电不丢已采集点。空字符串将导致后端跳过实时写入（仅靠校准结束全量导出）。
//
// 拆分为目录 (savePath) + 文件名 (saveFileName) 两个独立字段（与遍历测试一致）：
//   - savePath 仅保存目录，pickSavePath 只选目录不拼接文件名，避免用户改文件名时
//     必须重新点选目录的繁琐交互；
//   - saveFileName 用户可见可手动编辑，watch calibrationName 自动同步默认值，
//     保存前再次清洗（剥离 .csv 后缀 + 非法字符过滤），加载时同样剥离后缀再清洗，
//     防止持久化的 ".csv" 被 buildCalibrationCsvName 当作普通字符再追加一次。
//
// 注意：初始值为空字符串而非 buildCalibrationCsvName(...)，因为 calibrationName 初始也是空，
// onMounted 加载配置完成后若仍为空会兜底为"五孔探针-<日期>"，watch 会同步刷新默认文件名。
const savePath = ref('')
const saveFileName = ref('')
const sphereTankGateEnabled = ref(false)
const sphereTankWaitTimeSec = ref(3)
const sphereTankStableChannel = ref<ChannelRef>({ deviceId: '', channelIndex: 0 })
const machNumberPrecision = ref<number>(DEFAULT_CALIBRATION_MACH_PRECISION)
const velocityPrecision = ref<number>(DEFAULT_CALIBRATION_VELOCITY_PRECISION)

const probeChannels = ref<ProbeChannelConfig[]>([
  { name: t.value['fiveHoleP1'], role: 'fiveHole.p1', channel: { deviceId: '', channelIndex: 0 }, enabled: true, precision: DEFAULT_CALIBRATION_PROBE_PRECISION },
  { name: t.value['fiveHoleP2'], role: 'fiveHole.p2', channel: { deviceId: '', channelIndex: 1 }, enabled: true, precision: DEFAULT_CALIBRATION_PROBE_PRECISION },
  { name: t.value['fiveHoleP3'], role: 'fiveHole.p3', channel: { deviceId: '', channelIndex: 2 }, enabled: true, precision: DEFAULT_CALIBRATION_PROBE_PRECISION },
  { name: t.value['fiveHoleP4'], role: 'fiveHole.p4', channel: { deviceId: '', channelIndex: 3 }, enabled: true, precision: DEFAULT_CALIBRATION_PROBE_PRECISION },
  { name: t.value['fiveHoleP5'], role: 'fiveHole.p5', channel: { deviceId: '', channelIndex: 4 }, enabled: true, precision: DEFAULT_CALIBRATION_PROBE_PRECISION },
  { name: t.value['fiveHolePAtm'], role: 'fiveHole.pAtm', channel: { deviceId: '', channelIndex: 16 }, enabled: true, precision: DEFAULT_CALIBRATION_PROBE_PRECISION },
  { name: t.value['fiveHoleTAtm'], role: 'fiveHole.tAtm', channel: { deviceId: '', channelIndex: 17 }, enabled: true, precision: DEFAULT_CALIBRATION_PROBE_PRECISION },
  { name: t.value['fiveHolePTotal'], role: 'fiveHole.pTotal', channel: { deviceId: '', channelIndex: -1 }, enabled: true, precision: DEFAULT_CALIBRATION_PROBE_PRECISION },
  { name: t.value['fiveHolePTunnelStatic'], role: 'fiveHole.pTunnelStatic', channel: { deviceId: '', channelIndex: -1 }, enabled: true, precision: DEFAULT_CALIBRATION_PROBE_PRECISION },
  { name: t.value['fiveHoleTTunnel'], role: 'fiveHole.tTunnel', channel: { deviceId: '', channelIndex: -1 }, enabled: true, precision: DEFAULT_CALIBRATION_PROBE_PRECISION },
])

const motionAxes = ref<MotionAxisConfig[]>([
  { name: 'Alpha', controllerId: '', axis: 'X' },
  { name: 'Beta', controllerId: '', axis: 'Y' },
])

// 运动安全配置：4 个全局阈值 + 按轴覆盖，留空字段等价于"使用后端默认值"。
// 与遍历测试模块共享同一份 MotionSafetyConfig 类型与 MotionSafetyPanel 组件，
// 保证校准与遍历的运动安全语义完全一致。
const motionSafety = ref<MotionSafetyConfig | undefined>(undefined)

const deviceList = computed(() => deviceStore.profiles)
const motionControllerList = computed(() => motionStore.profiles)

// 批量操作：统一选择设备 + 通道号自动递增填充（仿照遍历测试模块）
const {
  batchDeviceId,
  autoFillStartIndex,
  applyDeviceToAll,
  autoFillChannelIndices,
} = useChannelBatchOperations(probeChannels)

// 设备下拉选项（批量工具栏与单通道选择共用）
const deviceOptions = computed(() =>
  deviceList.value.map((d) => ({ label: `${d.name} (${d.type})`, value: d.id })),
)

const REQUIRED_CHANNEL_ROLES = [
  'fiveHole.p1', 'fiveHole.p2', 'fiveHole.p3', 'fiveHole.p4', 'fiveHole.p5',
  'fiveHole.pAtm', 'fiveHole.tAtm', 'fiveHole.pTotal', 'fiveHole.pTunnelStatic', 'fiveHole.tTunnel',
] as const

// 通道分组定义（三个分组共用同一套表格模板）
interface ChannelGroup {
  key: string
  label: string
  icon: Component
  roles: string[]
  channels: ProbeChannelConfig[]
}

const channelGroups = computed<ChannelGroup[]>(() => [
  { key: 'probe', label: t.value.fiveHoleProbeGroup || '探针五孔', icon: Activity, roles: ['fiveHole.p1', 'fiveHole.p2', 'fiveHole.p3', 'fiveHole.p4', 'fiveHole.p5'], channels: probeChannels.value.filter((ch) => ['fiveHole.p1', 'fiveHole.p2', 'fiveHole.p3', 'fiveHole.p4', 'fiveHole.p5'].includes(ch.role || '')) },
  { key: 'atmosphere', label: t.value.atmosphereGroup || '大气环境', icon: Wind, roles: ['fiveHole.pAtm', 'fiveHole.tAtm'], channels: probeChannels.value.filter((ch) => ['fiveHole.pAtm', 'fiveHole.tAtm'].includes(ch.role || "")) },
  { key: 'windTunnel', label: t.value.windTunnelGroup || '风洞参数', icon: Gauge, roles: ['fiveHole.pTotal', 'fiveHole.pTunnelStatic', 'fiveHole.tTunnel'], channels: probeChannels.value.filter((ch) => ['fiveHole.pTotal', 'fiveHole.pTunnelStatic', 'fiveHole.tTunnel'].includes(ch.role || "")) },
])

// 通道映射进度：已正确映射的通道数 / 必需通道总数
const mappedChannelCount = computed(
  () =>
    probeChannels.value.filter(
      (ch) => ch.enabled && ch.channel.deviceId && ch.channel.channelIndex >= 0,
    ).length,
)
const totalRequiredChannelCount = REQUIRED_CHANNEL_ROLES.length

// 整数迭代生成预览点，避免浮点累加误差；与 pointLayoutValidation.count 共享同一真源
const previewDots = computed<{ cx: number; cy: number }[]>(() => {
  if (!pointLayoutValidation.value.valid) return []
  const { alphaMin, alphaMax, alphaStep, betaMin, betaMax, betaStep } = pointLayout.value
  const xRange = alphaMax - alphaMin
  const yRange = betaMax - betaMin
  const alphaCount = Math.round(xRange / alphaStep)
  const betaCount = Math.round(yRange / betaStep)
  const dots: { cx: number; cy: number }[] = []
  for (let i = 0; i <= alphaCount; i++) {
    for (let j = 0; j <= betaCount; j++) {
      const a = alphaMin + i * alphaStep
      const b = betaMin + j * betaStep
      const cx = 20 + ((a - alphaMin) / xRange) * 160
      const cy = 180 - ((b - betaMin) / yRange) * 160
      dots.push({ cx: Math.round(cx * 10) / 10, cy: Math.round(cy * 10) / 10 })
    }
  }
  return dots
})

const currentStepErrors = computed<string[]>(() => {
  if (currentStep.value === 0) {
    const errors: string[] = []
    if (calibrationName.value.trim() === '') errors.push(t.value.enterConfigName || '请输入配置名称')
    if (savePath.value.trim() === '') errors.push(t.value.fh_pleaseSelectCsvPath)
    if (saveFileName.value.trim() === '') errors.push(t.value.fh_pleaseInputCsvFileName)
    // 点位布局相关错误统一由 validatePointLayout 提供
    errors.push(...pointLayoutValidation.value.errors)
    if (dwellTimeMs.value < 100) errors.push(t.value.dwellTimeMin || '驻留时间至少100ms')
    if (samplesPerPoint.value < 1) errors.push(t.value.samplesMin || '采样次数至少为1')
    return errors
  }
  if (currentStep.value === 1) {
    const errors: string[] = []
    const enabledRoles = new Set(probeChannels.value.filter((ch) => ch.enabled).map((ch) => ch.role))
    const missingRoles = REQUIRED_CHANNEL_ROLES.filter((role) => !enabledRoles.has(role))
    if (missingRoles.length > 0) errors.push(t.value.requiredChannelsMissing || '缺少必需的通道')
    const invalidChannel = probeChannels.value.find((ch) => ch.enabled && (!ch.channel.deviceId || ch.channel.channelIndex < 0))
    if (invalidChannel) errors.push(`${t.value.channelMapIncomplete || '通道映射不完整'}: ${getProbeChannelDisplayName(invalidChannel.role, invalidChannel.name, t.value)}`)
    const missingControllerAxis = motionAxes.value.find((axis) => !axis.controllerId)
    if (missingControllerAxis) errors.push(`${t.value.axisNotBound || '轴未绑定'}: ${missingControllerAxis.name}`)
    if (sphereTankGateEnabled.value && !sphereTankStableChannel.value.deviceId) errors.push(t.value.sphereTankRequiresDevice || '球罐门控需要选择设备')
    return errors
  }
  return []
})

const isStepValid = computed(() => {
  if (currentStep.value === 2) return true
  return currentStepErrors.value.length === 0
})

function nextStep() { if (currentStep.value < steps.value.length - 1) currentStep.value++ }

function prevStep() { if (currentStep.value > 0) currentStep.value-- }

// generateFiveHoleSnakePoints 由 saveConfig 在归一化 layout 后直接调用，
// 不再在这里包一层同步包装函数，避免误用未归一化的 pointLayout.value。

// 选择 CSV 保存目录：与遍历测试一致，只选目录赋给 savePath，
// 文件名由独立的 saveFileName 字段管理（用户可见可编辑），避免每次改文件名都要重选目录。
async function pickSavePath() {
  try {
    const picked = await storageStore.pickDirectory()
    if (picked) savePath.value = picked
  } catch (e) {
    feedbackStore.pushToast('选择保存路径失败: ' + (e instanceof Error ? e.message : String(e)), 'error')
  }
}

// calibrationName 变化时同步刷新默认 saveFileName：仅当用户未手动修改（仍为空或等于上一默认值）时覆盖，
// 避免覆盖用户手动输入的文件名。复用共享工具保证清洗规则与遍历测试画面一致。
// 注意：Vue 3 watch 默认 pre 模式异步触发，onMounted 中 calibrationName 兜底设值后
// 并不依赖本 watch 同步刷新 saveFileName——onMounted 显式兜底语句会先把它填好，
// 本 watch 仅在用户后续手动改 calibrationName 时接力同步默认文件名。
watch(calibrationName, (next, prev) => {
  const prevDefault = prev.trim() ? buildCalibrationCsvName(prev.trim(), 'five-hole') : ''
  if (saveFileName.value === '' || saveFileName.value === prevDefault)
    saveFileName.value = buildCalibrationCsvName(next.trim(), 'five-hole')
})

// 把 UiInputNumber 在输入中间态 emit 的 null 归一化为 number，
// 避免 pointLayout 字段残留 null/NaN 导致生成点位错乱、保存到磁盘的 fiveHoleLayout 与 points 不一致。
// 规则：null/NaN/Infinity 一律回退为 0；同时返回全新对象避免污染 ref。
function sanitizePointLayout(layout: typeof pointLayout.value): typeof pointLayout.value {
  const fix = (v: number | null | undefined): number => {
    if (v === null || v === undefined || !Number.isFinite(v)) return 0
    return v
  }
  return {
    alphaMin: fix(layout.alphaMin),
    alphaMax: fix(layout.alphaMax),
    alphaStep: fix(layout.alphaStep),
    betaMin: fix(layout.betaMin),
    betaMax: fix(layout.betaMax),
    betaStep: fix(layout.betaStep),
    serpentine: layout.serpentine === true,
  }
}

async function saveConfig() {
  isSaving.value = true
  try {
    // 保存前显式校验点位布局：UiInputNumber 在清空-输入中间态会 emit null，
    // 此时 isStepValid 在 step 2 仍为 true（见 line 252），保存按钮可点，
    // 必须在此拦截避免脏值落盘。
    if (!pointLayoutValidation.value.valid) {
      feedbackStore.pushToast('点位布局参数无效: ' + pointLayoutValidation.value.errors.join('；'), 'warning')
      return
    }
    // 保存前再次清洗 saveFileName：用户可能手动输入了非法字符或未带 .csv 后缀。
    // 剥离 .csv 后缀后交给共享工具，fallback 用 calibrationName 保证空文件名也能落到有意义的默认值。
    const normName = buildCalibrationCsvName(saveFileName.value.replace(/\.csv$/i, ''), calibrationName.value.trim())
    saveFileName.value = normName
    // 后端 csv_writer 约定 SavePath 必须是含 .csv 扩展名的完整文件路径，
    // 故前端在保存时把目录与文件名拼接为完整路径再传给后端，同时持久化 saveFileName
    // 便于下次加载时分离展示。
    const fullSavePath = savePath.value.trim() ? joinCalibrationPath(savePath.value.trim(), normName) : ''
    // 归一化后的 layout 作为本次保存的真值，同时用于生成 points 与 fiveHoleLayout 字段，
    // 保证磁盘上两个字段永远一致，消除双真值源。
    const sanitizedLayout = sanitizePointLayout(pointLayout.value)
    const config: CalibrationConfig = {
      type: 'five-hole',
      name: calibrationName.value,
      probeChannels: probeChannels.value.filter((ch) => ch.enabled),
      motionAxes: motionAxes.value,
      // 运动安全配置透传：未配置字段为 undefined，后端 Resolve() 时合并默认值
      motionSafety: motionSafety.value,
      points: await generateFiveHoleSnakePoints(sanitizedLayout),
      dwellTimeMs: dwellTimeMs.value,
      samplesPerPoint: samplesPerPoint.value,
      savePath: fullSavePath,
      saveFileName: normName,
      fiveHoleLayout: sanitizedLayout,
      derivedValuePrecision: { machNumber: machNumberPrecision.value, velocity: velocityPrecision.value },
      uiRefreshHz: calibrationStore.uiRefreshHz,
      sphereTankGate: {
        enabled: sphereTankGateEnabled.value,
        waitTimeSec: Math.max(0, sphereTankWaitTimeSec.value),
        stableTimeChannel: { ...sphereTankStableChannel.value },
      },
    }
    const normalizedConfig = applyCalibrationPrecisionDefaults(config)
    const res = await calibrationApi.saveConfig('five-hole', normalizedConfig)
    if (!res.success) throw new Error(res.error || t.value.fh_saveFailed)
    emit('saved', normalizedConfig)
    emit('close')
  } catch (err) {
    feedbackStore.pushToast(t.value.fh_saveFailedColon + (err instanceof Error ? err.message : String(err)), 'error')
  } finally { isSaving.value = false }
}

// 加载已保存配置：非 404 错误时给用户 toast 提示，避免静默失败
async function loadSavedConfig() {
  try {
    const res = await calibrationApi.getConfig('five-hole')
    const config = res.success && res.data ? applyCalibrationPrecisionDefaults(res.data) : null
    if (!config) return
    calibrationName.value = config.name
    if (config.fiveHoleLayout) pointLayout.value = { ...pointLayout.value, ...config.fiveHoleLayout, serpentine: config.fiveHoleLayout.serpentine === true }
    if (config.probeChannels) {
      config.probeChannels.forEach((savedCh) => {
        const existingCh = probeChannels.value.find((ch) => ch.role ? ch.role === savedCh.role : ch.name === savedCh.name)
        if (!existingCh) return
        existingCh.channel = { ...savedCh.channel }
        existingCh.enabled = savedCh.enabled
        existingCh.role = savedCh.role ?? existingCh.role
        existingCh.precision = savedCh.precision
      })
    }
    if (config.motionAxes) {
      config.motionAxes.forEach((savedAxis, index) => {
        if (motionAxes.value[index]) motionAxes.value[index] = { ...savedAxis }
      })
    }
    // 还原运动安全配置：浅拷贝避免修改持久化对象，未配置字段保持 undefined
    if (config.motionSafety) {
      motionSafety.value = { ...config.motionSafety }
    } else {
      motionSafety.value = undefined
    }
    dwellTimeMs.value = config.dwellTimeMs
    samplesPerPoint.value = config.samplesPerPoint
    // 还原 savePath 与 saveFileName：
    //   - 优先使用持久化的 saveFileName（新配置），剥离 .csv 后缀再交给共享工具重新清洗，
    //     防止持久化的 ".csv" 被 buildCalibrationCsvName 当作普通字符再追加一次。
    //   - 旧配置无 saveFileName 字段时，用 splitCalibrationSavePath 从完整 savePath
    //     反拆 basename 作为兜底，兼容升级前已落盘的配置；dir 还原为纯目录。
    if (config.saveFileName) {
      saveFileName.value = buildCalibrationCsvName(config.saveFileName.replace(/\.csv$/i, ''), config.name)
    } else if (config.savePath) {
      const { baseName } = splitCalibrationSavePath(config.savePath)
      saveFileName.value = buildCalibrationCsvName(baseName.replace(/\.csv$/i, ''), config.name)
    }
    const { dir: restoredDir } = splitCalibrationSavePath(config.savePath || '')
    savePath.value = restoredDir
    machNumberPrecision.value = config.derivedValuePrecision?.machNumber ?? DEFAULT_CALIBRATION_MACH_PRECISION
    velocityPrecision.value = config.derivedValuePrecision?.velocity ?? DEFAULT_CALIBRATION_VELOCITY_PRECISION
    if (config.sphereTankGate) {
      sphereTankGateEnabled.value = config.sphereTankGate.enabled
      sphereTankWaitTimeSec.value = config.sphereTankGate.waitTimeSec
      sphereTankStableChannel.value = { ...config.sphereTankGate.stableTimeChannel }
    }
  } catch (err) {
    // 首次打开无配置（404）属正常，其他异常需提示
    const msg = err instanceof Error ? err.message : String(err)
    if (!msg.includes('404') && !msg.includes('not found')) {
      feedbackStore.pushToast(t.value.wf_loadConfigFailed + msg, 'warning')
    }
  }
}

onMounted(async () => {
  try {
    const results = await Promise.allSettled([deviceStore.refreshProfiles(), motionStore.refreshProfiles(), loadSavedConfig()])
    reportAllSettledFailures(
      results,
      ['设备列表', '运动控制器列表', '五孔校准配置'],
      feedbackStore.pushToast,
    )
    // calibrationName 兜底：用 ISO 日期（YYYY-MM-DD），避免 toLocaleDateString 在中文环境返回含斜杠的日期
    // 被下游 buildCalibrationCsvName 当作路径分隔符清洗掉。
    if (!calibrationName.value) calibrationName.value = `五孔探针-${new Date().toISOString().slice(0, 10)}`
    // watch calibrationName 兜底设值时会同步刷新 saveFileName，避免首次进入对话框文件名输入框空白。
    if (!saveFileName.value.trim()) saveFileName.value = buildCalibrationCsvName(calibrationName.value.trim(), 'five-hole')
    // 加载后若 savePath 仍为空，回退到全局基础目录，与遍历测试体验一致：
    // 用户首次打开对话框不必先点选目录就能完成保存。
    if (!savePath.value.trim()) savePath.value = storageStore.settings?.baseDirectory?.trim() ?? ''
  } finally { isLoading.value = false }
})

const axisOptions = [
  { label: 'X 轴', value: 'X' },
  { label: 'Y 轴', value: 'Y' },
  { label: 'Z 轴', value: 'Z' },
  { label: 'U 轴', value: 'U' },
]

// 通道索引枚举选项：UI 显示 CH1~CH18（1-based），内部 value 仍为数组索引 0~17
// 通道序号从 1 开始更符合操作员直觉，对应底层数组的 0-based 索引
const channelIndexOptions = Array.from({ length: 18 }, (_, i) => ({ label: `CH${i + 1}`, value: String(i) }))

// 通道表格列定义（所有通道直接罗列，通过分组列标识归属）
const channelColumns = [
  { key: 'enabled', label: '启用' },
  { key: 'group', label: '分组' },
  { key: 'name', label: '名称' },
  { key: 'device', label: '数据源' },
  { key: 'channel', label: '通道' },
  { key: 'precision', label: '精度' },
] as const

// 根据通道角色反查分组 key，用于在统一表格中显示分组 tag
function groupKeyOfRole(role: string | undefined): string {
  if (!role) return ''
  for (const g of channelGroups.value) {
    if (g.roles.includes(role)) return g.key
  }
  return ''
}

// 通道分组简称：用于在统一表格中以 tag 形式标识每个通道的归属
// 改为函数以响应全局语言切换（computed ref 在模板中无法自动解包 [key] 索引访问）
function getChannelGroupLabel(groupKey: string): string {
  const labels: Record<string, string> = {
    probe: t.value.fiveHoleProbeGroupShort || '五孔',
    atmosphere: t.value.atmosphereGroupShort || '大气',
    windTunnel: t.value.windTunnelGroupShort || '风洞',
  }
  return labels[groupKey] || '—'
}
</script>

<template>
  <UiDialog :show="true" width="min(92vw, 960px)" title="五孔探针校准配置" closable @update:show="emit('close')">
  <UiSteps :current="currentStep" class="steps-mb">
    <UiStep v-for="(step, idx) in steps" :key="idx" :title="step.label" :disabled="idx > currentStep" />
  </UiSteps>

  <UiSpin v-if="isLoading" class="spinner" />

    <template v-else>
      <!-- 紧凑布局：左侧主体内容（限制高度内部滚动）+ 右侧 sidebar（统计 + 点阵预览） -->
      <div class="calib-body">
      <div class="calib-main">
      <!-- 错误提示：列出全部错误，便于一次性修正 -->
      <UiAlert
        v-if="currentStepErrors.length > 0"
        type="warning"
        :title="`请修正以下问题 (${currentStepErrors.length})`"
        class="alert-error"
      >
        <ul class="error-list">
          <li v-for="(err, i) in currentStepErrors" :key="i">{{ err }}</li>
        </ul>
      </UiAlert>

      <!-- 步骤 1：基本设置（紧凑布局，预览移至右侧 sidebar） -->
      <div v-if="currentStep === 0" class="step-content">
        <!-- 基础信息：配置名称 + 刷新频率 横向合并 -->
        <UiPanel class="section-card">
          <template #header>
            <div class="section-header">
              <FileText :size="14" />
              <span>基础信息</span>
            </div>
          </template>
          <div class="basic-grid">
            <div class="field">
              <span class="field-label">配置名称</span>
              <UiInput v-model="calibrationName" placeholder="例如：五孔探针-2026-001" />
            </div>
            <div class="field field--fixed">
              <span class="field-label">界面刷新频率</span>
              <UiSelect
                :model-value="String(calibrationStore.uiRefreshHz)"
                :options="Array.from({ length: 20 }, (_, i) => ({ label: `${i + 1} Hz`, value: String(i + 1) }))"
                @update:model-value="calibrationStore.setUiRefreshHz(Number($event))"
              />
            </div>
          </div>
        </UiPanel>

        <!-- 点位布局：α / β 参数横向并排，预览移至 sidebar -->
        <UiPanel class="section-card">
          <template #header>
            <div class="section-header">
              <Move3D :size="14" />
              <span>点位布局</span>
            </div>
          </template>
          <div class="layout-params">
            <div class="angle-block">
              <div class="angle-block-header">
                <span class="angle-tag alpha-tag">α</span>
                <span class="angle-block-title">攻角范围</span>
              </div>
              <div class="angle-fields">
                <div class="field">
                  <span class="field-label">最小 (°)</span>
                  <UiInputNumber v-model="pointLayout.alphaMin" />
                </div>
                <div class="field">
                  <span class="field-label">最大 (°)</span>
                  <UiInputNumber v-model="pointLayout.alphaMax" />
                </div>
                <div class="field">
                  <span class="field-label">步长 (°)</span>
                  <UiInputNumber v-model="pointLayout.alphaStep" :min="1" />
                </div>
              </div>
            </div>
            <div class="angle-block">
              <div class="angle-block-header">
                <span class="angle-tag beta-tag">β</span>
                <span class="angle-block-title">侧滑角范围</span>
              </div>
              <div class="angle-fields">
                <div class="field">
                  <span class="field-label">最小 (°)</span>
                  <UiInputNumber v-model="pointLayout.betaMin" />
                </div>
                <div class="field">
                  <span class="field-label">最大 (°)</span>
                  <UiInputNumber v-model="pointLayout.betaMax" />
                </div>
                <div class="field">
                  <span class="field-label">步长 (°)</span>
                  <UiInputNumber v-model="pointLayout.betaStep" :min="1" />
                </div>
              </div>
            </div>
          </div>
          <div class="layout-options">
            <UiCheckbox v-model:checked="pointLayout.serpentine">
              蛇形走位（奇数行反向遍历 α，减少空行程）
            </UiCheckbox>
          </div>
        </UiPanel>

        <!-- 采集与输出：采集参数 + 输出精度 合并为 4 字段横排 -->
        <UiPanel class="section-card">
          <template #header>
            <div class="section-header">
              <Timer :size="14" />
              <span>采集与输出</span>
            </div>
          </template>
          <div class="param-grid-4">
            <div class="field">
              <span class="field-label">驻留时间 (ms)</span>
              <UiInputNumber v-model="dwellTimeMs" :min="100" :step="100" />
            </div>
            <div class="field">
              <span class="field-label">每点采样数</span>
              <UiInputNumber v-model="samplesPerPoint" :min="1" :max="1000" />
            </div>
            <div class="field">
              <span class="field-label">马赫数精度</span>
              <UiInputNumber v-model="machNumberPrecision" :min="0" :max="8" />
            </div>
            <div class="field">
              <span class="field-label">流速精度 (m/s)</span>
              <UiInputNumber v-model="velocityPrecision" :min="0" :max="8" />
            </div>
          </div>
        </UiPanel>

        <UiPanel class="section-card">
          <template #header>
            <div class="section-header">
              <Save :size="14" />
              <span>CSV 保存</span>
            </div>
          </template>
          <!-- 目录与文件名分离展示（与遍历测试一致）：目录用浏览按钮选，文件名用户可见可手动编辑 -->
          <div class="field">
            <span class="field-label">保存目录</span>
            <div class="flex items-center gap-2">
              <UiInput v-model="savePath" placeholder="点击右侧按钮选择保存目录" class="flex-1" :title="savePath" />
              <UiButton size="sm" variant="secondary" @click="pickSavePath">选择目录…</UiButton>
            </div>
          </div>
          <div class="field">
            <span class="field-label">CSV 文件名</span>
            <UiInput v-model="saveFileName" :placeholder="t.fh_pleaseInputCsvFileName" class="flex-1" />
          </div>
        </UiPanel>
      </div>

      <!-- 步骤 2：硬件配置 -->
      <div v-if="currentStep === 1" class="step-content hardware-step">
        <!-- 通道映射进度条：直观展示配置完成度 -->
        <div class="mapping-progress">
          <span class="mapping-progress-label">通道映射</span>
          <div class="mapping-progress-bar">
            <div
              class="mapping-progress-fill"
              :style="{ width: `${(mappedChannelCount / totalRequiredChannelCount) * 100}%` }"
            />
          </div>
          <span class="mapping-progress-text">{{ mappedChannelCount }} / {{ totalRequiredChannelCount }}</span>
        </div>

        <!-- 批量操作工具栏：统一选择设备 + 通道号自动递增填充 -->
        <div class="batch-toolbar">
          <div class="batch-toolbar-row">
            <div class="batch-cell">
              <span class="batch-label">统一设备</span>
              <UiSelect
                v-model="batchDeviceId"
                :options="deviceOptions"
                placeholder="选择设备"
                class="batch-select"
              />
            </div>
            <div class="batch-cell">
              <UiButton size="sm" variant="primary" :disabled="!batchDeviceId" @click="applyDeviceToAll">应用到全部通道</UiButton>
            </div>
          </div>
          <div class="batch-toolbar-row">
            <div class="batch-cell">
              <span class="batch-label">起始通道</span>
              <UiSelect
                :model-value="autoFillStartIndex !== null ? String(autoFillStartIndex) : ''"
                @update:model-value="autoFillStartIndex = $event !== '' ? Number($event) : null"
                :options="channelIndexOptions"
                placeholder="选择起始通道号"
                class="batch-select"
              />
            </div>
            <div class="batch-cell">
              <UiButton size="sm" variant="primary" :disabled="autoFillStartIndex === null" @click="autoFillChannelIndices">自动递增填充</UiButton>
            </div>
          </div>
        </div>

        <!-- 通道映射：所有通道直接罗列在单个表格中，通过分组列标识归属 -->
        <UiPanel class="section-card">
          <template #header>
            <div class="section-header">
              <Activity :size="14" />
              <span>通道映射</span>
              <span class="section-count">{{ probeChannels.length }} 通道</span>
            </div>
          </template>
          <div class="table-wrap">
            <table class="ntable">
              <thead>
                <tr>
                  <th v-for="col in channelColumns" :key="col.key">{{ col.label }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="ch in probeChannels" :key="ch.name">
                  <td class="cell-center"><UiCheckbox v-model:checked="ch.enabled" /></td>
                  <td><span class="group-tag" :data-group="groupKeyOfRole(ch.role)">{{ getChannelGroupLabel(groupKeyOfRole(ch.role)) }}</span></td>
                  <td><span class="cell-name">{{ getProbeChannelDisplayName(ch.role, ch.name, t) }}</span></td>
                  <td class="cell-device">
                    <UiSelect
                      v-model="ch.channel.deviceId"
                      :options="deviceList.map(d => ({ label: `${d.name} (${d.type})`, value: d.id }))"
                      placeholder="选择设备"
                      :disabled="!ch.enabled"
                      :fallback="false"
                    />
                  </td>
                  <td class="cell-channel"><UiSelect
                    :model-value="ch.channel.channelIndex >= 0 ? String(ch.channel.channelIndex) : ''"
                    @update:model-value="ch.channel.channelIndex = $event !== '' ? Number($event) : -1"
                    :options="channelIndexOptions"
                    placeholder="未分配"
                    :disabled="!ch.enabled"
                  /></td>
                  <td><UiInputNumber v-model="ch.precision" :min="0" :max="8" :disabled="!ch.enabled" /></td>
                </tr>
              </tbody>
            </table>
          </div>
        </UiPanel>

        <!-- 运动轴配置 -->
        <UiPanel class="section-card">
          <template #header>
            <div class="section-header">
              <Move3D :size="14" />
              <span>运动轴配置</span>
            </div>
          </template>
          <div class="table-wrap">
            <table class="ntable">
              <thead><tr><th>坐标轴</th><th class="col-controller">运动控制器</th><th class="col-axis">物理轴</th></tr></thead>
              <tbody>
                <tr v-for="axis in motionAxes" :key="axis.name">
                  <td><UiStatusBadge status="connected">{{ axis.name }}</UiStatusBadge></td>
                  <td>
                    <UiSelect
                      v-model="axis.controllerId"
                      :options="motionControllerList.map(c => ({ label: `${c.name} (${c.type})`, value: c.id }))"
                      placeholder="选择控制器"
                    />
                  </td>
                  <td><UiSelect v-model="axis.axis" :options="axisOptions" /></td>
                </tr>
              </tbody>
            </table>
          </div>
        </UiPanel>

        <!-- 运动安全配置：紧贴运动轴配置下方，让操作员在绑定轴后立即调整到位容差与异常停机阈值。
             留空字段等价于"使用后端默认值"，避免强制用户填全 4 个字段才能保存。
             与遍历测试模块共享同一份 MotionSafetyPanel 组件，保证语义一致。 -->
        <MotionSafetyPanel
          v-model:motion-safety="motionSafety"
          :motion-axes="motionAxes"
          :t="(t as unknown as Record<string, string>)"
        />

        <!-- 球罐判定门控 -->
        <UiPanel class="section-card">
          <template #header>
            <div class="section-header">
              <ShieldCheck :size="14" />
              <span>球罐判定门控</span>
              <UiCheckbox v-model:checked="sphereTankGateEnabled" class="header-toggle">启用</UiCheckbox>
            </div>
          </template>
          <div v-if="sphereTankGateEnabled" class="sphere-grid">
            <div class="field">
              <span class="field-label">等待时间 (秒)</span>
              <UiInputNumber v-model="sphereTankWaitTimeSec" :min="0" :step="0.1" />
            </div>
            <div class="field">
              <span class="field-label">稳定通道设备</span>
              <UiSelect v-model="sphereTankStableChannel.deviceId" :options="deviceList.map(d => ({ label: d.name, value: d.id }))" placeholder="选择设备" :fallback="false" />
            </div>
            <div class="field">
              <span class="field-label">稳定通道</span>
              <UiSelect
                :model-value="sphereTankStableChannel.channelIndex >= 0 ? String(sphereTankStableChannel.channelIndex) : ''"
                @update:model-value="sphereTankStableChannel.channelIndex = $event !== '' ? Number($event) : 0"
                :options="channelIndexOptions"
                placeholder="选择通道"
              />
            </div>
            <p class="sphere-hint">球罐压力稳定后才开始采集，避免启动瞬态影响数据质量</p>
          </div>
          <p v-else class="empty-hint">未启用球罐判定，校准将按驻留时间直接采集</p>
        </UiPanel>
      </div>

      <!-- 步骤 3：确认保存 -->
      <div v-if="currentStep === 2" class="step-content">
        <UiPanel class="section-card">
          <template #header>
            <div class="section-header">
              <ShieldCheck :size="14" />
              <span>配置摘要</span>
            </div>
          </template>
          <div class="summary-grid-2">
            <div class="summary-row">
              <span class="summary-label">配置名称</span>
              <span class="summary-value">{{ calibrationName }}</span>
            </div>
            <div class="summary-row">
              <span class="summary-label">校准类型</span>
              <span class="summary-value">五孔探针</span>
            </div>
            <div class="summary-row">
              <span class="summary-label">α 范围</span>
              <span class="summary-value">{{ pointLayout.alphaMin }}° ~ {{ pointLayout.alphaMax }}° (步长 {{ pointLayout.alphaStep }}°)</span>
            </div>
            <div class="summary-row">
              <span class="summary-label">β 范围</span>
              <span class="summary-value">{{ pointLayout.betaMin }}° ~ {{ pointLayout.betaMax }}° (步长 {{ pointLayout.betaStep }}°)</span>
            </div>
            <div class="summary-row">
              <span class="summary-label">走位方式</span>
              <span class="summary-value" :class="pointLayout.serpentine ? 'accent-bold' : 'muted-text'">
                {{ pointLayout.serpentine ? '蛇形（奇数行反向）' : '逐行 raster' }}
              </span>
            </div>
            <div class="summary-row">
              <span class="summary-label">总点数</span>
              <span class="summary-value accent-bold">{{ pointCount }} 点</span>
            </div>
            <div class="summary-row">
              <span class="summary-label">启用通道</span>
              <span class="summary-value">{{ probeChannels.filter(ch => ch.enabled).length }} / {{ probeChannels.length }} 个</span>
            </div>
            <div class="summary-row">
              <span class="summary-label">驻留时间</span>
              <span class="summary-value">{{ dwellTimeMs }} ms</span>
            </div>
            <div class="summary-row">
              <span class="summary-label">每点采样数</span>
              <span class="summary-value">{{ samplesPerPoint }}</span>
            </div>
            <div class="summary-row">
              <span class="summary-label">运动轴</span>
              <span class="summary-value">{{ motionAxes.map(a => `${a.name}→${a.axis}`).join('，') }}</span>
            </div>
            <div class="summary-row">
              <span class="summary-label">球罐判定</span>
              <span class="summary-value" :class="sphereTankGateEnabled ? 'accent-bold' : 'muted-text'">
                {{ sphereTankGateEnabled ? `启用（等待 ${sphereTankWaitTimeSec}s）` : '未启用' }}
              </span>
            </div>
            <div class="summary-row">
              <span class="summary-label">CSV 保存</span>
              <span class="summary-value">{{ savePath && saveFileName ? joinCalibrationPath(savePath, saveFileName) : savePath }}</span>
            </div>
          </div>
        </UiPanel>
      </div>
      </div>
      <!-- 右侧 sidebar：始终显示统计与点阵预览，跨步骤提供上下文 -->
      <aside class="calib-sidebar">
        <div class="sidebar-stats">
          <div class="sidebar-stat sidebar-stat--highlight">
            <span class="stat-label">总点数</span>
            <span class="stat-number">{{ pointCount }}</span>
          </div>
          <div class="sidebar-stat">
            <span class="stat-label">驻留时间</span>
            <span class="stat-value">{{ dwellTimeMs }}<span class="stat-unit">ms</span></span>
          </div>
          <div class="sidebar-stat">
            <span class="stat-label">采样数</span>
            <span class="stat-value">{{ samplesPerPoint }}</span>
          </div>
        </div>
        <div class="sidebar-preview">
          <div class="preview-header">
            <LayoutGrid :size="14" />
            <span class="preview-title">点阵预览</span>
          </div>
          <div class="preview-canvas">
            <svg viewBox="0 0 200 200" class="preview-svg" aria-hidden="true">
              <line x1="20" y1="180" x2="180" y2="180" class="axis-line" />
              <line x1="20" y1="20" x2="20" y2="180" class="axis-line" />
              <circle
                v-for="(dot, i) in previewDots"
                :key="i"
                :cx="dot.cx"
                :cy="dot.cy"
                r="2.5"
                class="preview-dot"
              />
              <text x="100" y="196" class="axis-label">α</text>
              <text x="12" y="100" class="axis-label" transform="rotate(-90 12 100)">β</text>
            </svg>
          </div>
          <div class="preview-summary">
            <span class="preview-count-label">总点数</span>
            <span class="preview-count">{{ pointCount }}</span>
          </div>
        </div>
      </aside>
    </div>
    </template>

    <template #footer>
      <div class="footer-bar">
        <div>
          <UiButton v-if="currentStep > 0" variant="secondary" size="sm" @click="prevStep">
            <ChevronLeft :size="14" class="icon-left" />上一步
          </UiButton>
        </div>
        <div class="footer-actions">
          <span class="step-indicator">步骤 {{ currentStep + 1 }} / {{ steps.length }}</span>
          <UiButton v-if="currentStep < steps.length - 1" variant="primary" size="sm" :disabled="!isStepValid" @click="nextStep">
            下一步<ChevronRight :size="14" class="icon-right" />
          </UiButton>
          <UiButton v-else size="sm" variant="primary" :loading="isSaving" :disabled="!isStepValid" @click="saveConfig">
            <Save :size="14" />保存配置
          </UiButton>
        </div>
      </div>
    </template>
  </UiDialog>
</template>

<style scoped>
/* ============================================================
   枚举控件（NSelect）宽度稳定化
   所有 UiSelect 统一撑满父容器，防止选项文字长短导致宽度跳动
   ============================================================ */
.field :deep(.n-select),
.batch-cell :deep(.n-select),
.ntable td :deep(.n-select),
.sphere-grid :deep(.n-select) {
  width: 100% !important;
  min-width: 0;
}

/* UiInputNumber 同样撑满字段容器，避免移除内联 style 后宽度回退 */
.field :deep(.n-input-number),
.ntable td :deep(.n-input-number) {
  width: 100% !important;
  min-width: 0;
}

/* 紧凑布局主体：左右分栏，使用固定 height（而非 max-height）确保步骤切换时画面尺寸稳定。
   关键：用 height: 60vh 让内容少时主体保持固定高度、内容多时内部滚动，
   避免步骤切换时对话框整体高度跳动破坏视觉锚点（与遍历测试一致）。 */
.calib-body {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 240px;
  gap: 0;
  min-height: 0;
  height: 60vh;
  flex: 1;
  overflow: hidden;
}

.calib-main {
  min-height: 0;
  height: 60vh;
  overflow-y: auto;
  padding-right: var(--space-3);
  scrollbar-width: thin;
}

/* 右侧 sidebar：固定宽度，与主体等高（60vh），内部可滚动；
   使用 height 而非 max-height 确保不同步骤下边栏与主体等高、
   点阵预览图尺寸稳定（与遍历测试一致）。 */
.calib-sidebar {
  border-left: 1px solid var(--border-default);
  background: var(--bg-panel-strong);
  padding-left: var(--space-3);
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  height: 60vh;
  overflow-y: auto;
  scrollbar-width: thin;
}

/* 紧凑化 UiPanel 内边距：默认 var(--space-3) var(--space-4) 偏大，
   覆盖为 4px 8px 让所有步骤的卡片视觉对齐、与遍历测试一致。
   仅对非 hardware-step 生效，hardware-step 已有专属覆盖保持原行为。 */
.step-content:not(.hardware-step) .section-card :deep(.n-card__content) {
  padding: 4px 8px;
}
.step-content:not(.hardware-step) .section-card :deep(.n-card-header) {
  padding: 4px 8px;
}

.sidebar-stats {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: var(--space-2);
}

.sidebar-stat {
  padding: 8px 6px;
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
  border-color: var(--accent-primary);
  background: linear-gradient(180deg, var(--bg-panel) 0%, color-mix(in srgb, var(--accent-primary) 6%, transparent) 100%);
}

.sidebar-stat--highlight .stat-number {
  color: var(--accent-primary);
}

.sidebar-preview {
  flex: 1 1 auto;
  min-height: 140px;
  max-height: 320px;
  border-radius: var(--radius-md);
  border: 1px solid var(--border-default);
  background: var(--bg-canvas);
  padding: var(--space-2);
  display: flex;
  flex-direction: column;
  gap: var(--space-1-5);
  overflow: hidden;
}

.stat-label { font-size: var(--text-xs); color: var(--text-tertiary); }
.stat-number { font-size: 16px; font-weight: 700; color: var(--accent-primary); font-variant-numeric: tabular-nums; }
.stat-value { font-size: 12px; font-weight: 600; color: var(--text-primary); font-variant-numeric: tabular-nums; }
.stat-unit { font-size: var(--text-xs); color: var(--text-muted); margin-left: 2px; }

/* 步骤内容容器：滚动由主体区域统一承载，避免底部按钮被挤出视口 */
.step-content {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding-right: var(--space-1);
}

.hardware-step {
  gap: var(--space-1-5);
}

.hardware-step :deep(.n-card-header) {
  padding: var(--space-2) var(--space-3) var(--space-1-5);
}

.hardware-step :deep(.n-card__content) {
  padding: var(--space-1-5) var(--space-3) var(--space-2) !important;
}

.hardware-step .mapping-progress {
  padding: var(--space-1) var(--space-2);
}

.hardware-step .ntable th {
  padding: var(--space-1) var(--space-2);
}

.hardware-step .ntable td {
  padding: var(--space-0-5) var(--space-2);
}

/* 批量操作工具栏：扁平化工具条 */
.batch-toolbar {
  padding: 8px 12px;
  border-radius: var(--radius-md);
  border: 1px solid var(--border-default);
  background: var(--bg-panel);
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.batch-toolbar-row {
  display: grid;
  grid-template-columns: 180px 1fr;
  align-items: end;
  gap: 10px;
}

.batch-cell {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.batch-label {
  font-size: var(--text-xs);
  font-weight: 500;
  color: var(--text-secondary);
  white-space: nowrap;
}

.batch-select {
  width: 100%;
}

.section-card {
  font-size: var(--text-sm);
}

/* 区块标题：图标 + 文字 + 计数 */
.section-header {
  display: flex;
  align-items: center;
  flex-wrap: nowrap;
  gap: var(--space-1-5);
  font-size: var(--text-sm);
  font-weight: 600;
  color: var(--text-primary);
  /* 主体变窄后防止分组标题（如“探针五孔”）内部换行 */
  white-space: nowrap;
}

.section-count {
  margin-left: auto;
  font-size: var(--text-xs);
  font-weight: 400;
  color: var(--text-muted);
}

/* 字段通用结构 */
.field {
  display: flex;
  flex-direction: column;
  gap: var(--space-0-5);
  /* 确保内部 NSelect / NInputNumber 撑满字段宽度 */
  min-width: 0;
}

/* 固定宽度字段修饰：用于不需要弹性伸缩的短控件（如刷新频率下拉） */
.field--fixed {
  flex-shrink: 0;
}

.field-label {
  font-size: var(--text-xs);
  color: var(--text-tertiary);
}

/* 点位布局：α / β 参数横向并排（预览移至 sidebar） */
.layout-params {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--space-3);
}

.angle-block {
  display: flex;
  flex-direction: column;
  gap: var(--space-1-5);
}

.angle-block-header {
  display: flex;
  align-items: center;
  gap: var(--space-1-5);
}

.angle-tag {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: var(--space-4);
  height: var(--space-4);
  border-radius: var(--radius-sm);
  font-weight: 700;
  font-size: var(--text-xs);
}

.alpha-tag {
  background: var(--axis-x-soft);
  color: var(--axis-x);
}

.beta-tag {
  background: var(--axis-y-soft);
  color: var(--axis-y);
}

.angle-block-title {
  font-size: var(--text-sm);
  color: var(--text-secondary);
}

.angle-fields {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: var(--space-1);
}

.layout-options {
  margin-top: var(--space-2);
  padding-top: var(--space-2);
  border-top: 1px solid var(--border-default);
  font-size: var(--text-xs);
  color: var(--text-secondary);
}

.preview-header {
  display: flex;
  align-items: center;
  gap: var(--space-1);
  color: var(--text-muted);
}

.preview-title {
  font-size: var(--text-xs);
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

.preview-canvas {
  display: flex;
  justify-content: center;
}

.preview-svg {
  width: 100%;
  max-width: 200px;
  height: auto;
  aspect-ratio: 1 / 1;
}

.preview-dot {
  fill: var(--accent-primary);
}

.axis-line {
  stroke: var(--border-strong);
  stroke-width: 1;
}

.axis-label {
  font-size: 10px;
  fill: var(--text-muted);
  text-anchor: middle;
}

.preview-summary {
  display: flex;
  align-items: baseline;
  justify-content: center;
  gap: var(--space-1);
  padding-top: var(--space-1-5);
  border-top: 1px solid var(--border-default);
}

.preview-count-label {
  font-size: var(--text-xs);
  color: var(--text-muted);
}

.preview-count {
  font-size: var(--text-base);
  font-weight: 700;
  color: var(--accent-primary);
  font-variant-numeric: tabular-nums;
}

/* 基础信息：配置名称弹性 + 刷新频率固定宽度 */
.basic-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 160px;
  gap: var(--space-2);
  align-items: end;
}

/* 采集与输出：4 字段横排 */
.param-grid-4 {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: var(--space-1-5);
}

/* 通道映射进度条 */
.mapping-progress {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-1-5) var(--space-2);
  border-radius: var(--radius-md);
  background: var(--bg-panel-strong);
  border: 1px solid var(--border-default);
}

.mapping-progress-label {
  font-size: var(--text-xs);
  font-weight: 600;
  color: var(--text-secondary);
  white-space: nowrap;
}

.mapping-progress-bar {
  flex: 1;
  height: var(--space-1);
  border-radius: var(--radius-sm);
  background: var(--border-default);
  overflow: hidden;
}

.mapping-progress-fill {
  height: 100%;
  background: var(--accent-primary);
  border-radius: var(--radius-sm);
  transition: width 0.2s ease;
}

.mapping-progress-text {
  font-size: var(--text-xs);
  color: var(--text-secondary);
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

/* 表格 */
.table-wrap {
  overflow-x: auto;
}

.ntable {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--text-sm);
  table-layout: fixed;
}

.ntable th {
  text-align: left;
  padding: var(--space-1-5) 8px;
  font-weight: 600;
  font-size: var(--text-xs);
  color: var(--text-muted);
  background: var(--bg-panel-strong);
  border-bottom: 1px solid var(--border-default);
}

/* 通道映射表格固定列宽：启用 | 分组 | 名称(弹性) | 数据源(弹性) | 通道 | 精度 */
.ntable th:nth-child(1),
.ntable td:nth-child(1) { width: 48px; }

.ntable th:nth-child(2),
.ntable td:nth-child(2) { width: 56px; }

.ntable th:nth-child(5),
.ntable td:nth-child(5) { width: 96px; }

.ntable th:nth-child(6),
.ntable td:nth-child(6) { width: 80px; }

.ntable td {
  padding: var(--space-1) 8px;
  border-bottom: 1px solid var(--border-default);
  overflow: hidden;
}

.ntable tbody tr:hover {
  background: color-mix(in srgb, var(--accent-primary) 3%, transparent);
}

.cell-center {
  text-align: center;
}

.cell-name {
  font-size: var(--text-sm);
  color: var(--text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* 设备下拉所在单元格：允许下拉弹出层溢出 */
.cell-device,
.cell-channel {
  overflow: visible;
}

/* 运动轴表格列宽比例 */
.col-controller { width: 40%; }
.col-axis { width: 25%; }

/* 分组 tag：用颜色区分五孔/大气/风洞三类通道，紧凑不占横向空间 */
.group-tag {
  display: inline-block;
  padding: 1px 8px;
  border-radius: var(--radius-sm);
  font-size: var(--text-xs);
  font-weight: 600;
  line-height: 1.6;
  color: var(--text-secondary);
  background: var(--bg-panel-strong);
  border: 1px solid var(--border-default);
  white-space: nowrap;
}

.group-tag[data-group='probe'] {
  color: var(--accent-primary);
  background: color-mix(in srgb, var(--accent-primary) 8%, transparent);
  border-color: color-mix(in srgb, var(--accent-primary) 24%, transparent);
}

.group-tag[data-group='atmosphere'] {
  color: var(--success, #16a34a);
  background: color-mix(in srgb, var(--success, #16a34a) 10%, transparent);
  border-color: color-mix(in srgb, var(--success, #16a34a) 26%, transparent);
}

.group-tag[data-group='windTunnel'] {
  color: var(--warn, #d97706);
  background: color-mix(in srgb, var(--warn, #d97706) 10%, transparent);
  border-color: color-mix(in srgb, var(--warn, #d97706) 26%, transparent);
}

/* 球罐判定门控 */
.header-toggle {
  margin-left: auto;
  font-size: var(--text-xs);
}

/* 3列：等待时间(窄) | 设备(宽弹性) | 通道(窄) */
.sphere-grid {
  display: grid;
  grid-template-columns: 120px 1fr 120px;
  gap: var(--space-2);
  align-items: start;
}

.sphere-hint {
  grid-column: 1 / -1;
  margin: 0;
  font-size: var(--text-xs);
  color: var(--text-muted);
  line-height: 1.5;
}

.empty-hint {
  margin: 0;
  padding: var(--space-1) 0;
  font-size: var(--text-xs);
  color: var(--text-muted);
}

/* 摘要：双列网格 */
.summary-grid-2 {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0 var(--space-4);
}

.summary-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: var(--space-1-5) 0;
  border-bottom: 1px solid var(--border-default);
  gap: var(--space-2);
}

.summary-row:nth-last-child(-n + 2) {
  border-bottom: none;
}

.summary-label {
  font-size: var(--text-sm);
  color: var(--text-muted);
  white-space: nowrap;
}

.summary-value {
  font-size: var(--text-sm);
  color: var(--text-primary);
  text-align: right;
  word-break: break-word;
}

.accent-bold {
  color: var(--accent-primary);
  font-weight: 700;
}

.muted-text {
  color: var(--text-muted);
}

/* 错误列表 */
.error-list {
  margin: var(--space-1) 0 0;
  padding-left: var(--space-4);
  font-size: var(--text-sm);
  line-height: 1.5;
}

/* 页脚 */
.footer-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
}

.footer-actions {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

.step-indicator {
  font-size: var(--text-xs);
  color: var(--text-tertiary);
}

.icon-left {
  margin-right: var(--space-1);
}

.icon-right {
  margin-left: var(--space-1);
}

.steps-mb {
  margin-bottom: var(--space-3);
}

.spinner {
  display: flex;
  justify-content: center;
  padding: var(--space-8) 0;
}

.alert-error {
  margin-bottom: var(--space-2);
  font-size: var(--text-sm);
}
</style>

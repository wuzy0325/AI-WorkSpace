<script setup lang="ts">
/**
 * 七孔探针校准 3 步配置向导（spec Task 20）
 *
 * 设计参考 FiveHoleSettings.vue 的 3 步向导范式：
 *   - 步骤 0 基本设置：配置名 + 刷新率 + CSV 保存 + 校准模式 + 内区 α/β + 外区 θ/φ + 不确定度参数
 *   - 步骤 1 硬件配置：11 通道映射表 + 运动轴配置 + MotionSafetyPanel + 球罐判定门控
 *   - 步骤 2 确认保存：配置摘要双列网格
 *
 * 点位预览严格约束（spec Task 20 Acceptance）：
 *   - 调用 calibrationApi.previewSevenHolePoints(config) 获取真实点位
 *   - 禁止本地实现点位生成算法
 *   - 离线场景显示"请先连接后端"提示，不 fallback
 *   - 配置变更触发预览防抖（500ms）
 */
import { ref, computed, onMounted, onBeforeUnmount, watch, type Component } from 'vue'
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
  SevenHoleConfig,
  SevenHoleMode,
  SevenHolePreviewResult,
  CalibrationPoint,
} from '@shared/types/calibration'
import type { MotionSafetyConfig } from '@shared/types/traversal'
import {
  applyCalibrationPrecisionDefaults,
  DEFAULT_CALIBRATION_MACH_PRECISION,
  DEFAULT_CALIBRATION_PROBE_PRECISION,
  DEFAULT_CALIBRATION_VELOCITY_PRECISION,
} from '@shared/calibrationPrecision'
import { getProbeChannelDisplayName } from '@shared/calibrationChannelI18n'
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
  Activity,
  Wind,
  Gauge,
  Target,
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
  t.value.sh_step0BasicSettings || t.value.stepBasic || '基本设置',
  t.value.sh_step1HardwareConfig || t.value.stepHardware || '硬件配置',
  t.value.sh_step2ConfirmSave || t.value.stepConfirm || '确认保存',
])

// ==================== 七孔点位布局配置 ====================
// 与后端 calibration.SevenHoleConfig 严格对齐（spec Task 18）
// 默认值参考 spec §2.2：内区 α/β ±30° 步长 5°；外区 θ 推荐 [30°, 60°] 步长 5°（与 spec 一致），
// φ 0~360° 步长 30°（UI 简化步长，用户可按需调到 5° 以获得更密点位）
const sevenHoleLayout = ref<SevenHoleConfig>({
  mode: 'full',
  innerAlphaMin: -30,
  innerAlphaMax: 30,
  innerAlphaStep: 5,
  innerBetaMin: -30,
  innerBetaMax: 30,
  innerBetaStep: 5,
  outerThetaMin: 30,
  outerThetaMax: 60,
  outerThetaStep: 5,
  outerPhiMin: 0,
  outerPhiMax: 330,
  outerPhiStep: 30,
  serpentine: false,
})

// ==================== 不确定度参数 ====================
// spec Task 20 要求 UI 暴露 k / TIE_BREAK_TOLERANCE / 采样次数 / 驻留时间 四个字段。
// 采样次数与驻留时间持久化到 CalibrationConfig.samplesPerPoint / dwellTimeMs；
// k 与 TIE_BREAK_TOLERANCE 当前为算法常量（后端硬编码），UI 仅展示默认值，
// 待后续 Task 扩展 SevenHoleConfig 类型后再持久化。
const coverageFactorK = ref(2)
const tieBreakTolerance = ref(0.001)
const dwellTimeMs = ref(2000)
const samplesPerPoint = ref(10)
const calibrationName = ref('')
const machNumberPrecision = ref<number>(DEFAULT_CALIBRATION_MACH_PRECISION)
const velocityPrecision = ref<number>(DEFAULT_CALIBRATION_VELOCITY_PRECISION)

// CSV 保存路径：拆分为目录 (savePath) + 文件名 (saveFileName) 两个独立字段，
// 与五孔/三孔/总压/总温保持一致的交互体验。
const savePath = ref('')
const saveFileName = ref('')

// ==================== 球罐判定门控 ====================
const sphereTankGateEnabled = ref(false)
const sphereTankWaitTimeSec = ref(3)
const sphereTankStableChannel = ref<ChannelRef>({ deviceId: '', channelIndex: 0 })
// 球罐压力通道：仅用于前端实时显示压力值，不参与闸门判定；未配置 deviceId 时 UI 显示"暂无数据"
const sphereTankPressureChannel = ref<ChannelRef>({ deviceId: '', channelIndex: 0 })

// ==================== 11 通道映射 ====================
// 与后端 seven_hole.go requiredRoles 严格一致（spec §6.1）：
//   - sevenHole.p1~p7：7 个压力孔（外围 6 孔 P1~P6 + 中心孔 P7）
//   - sevenHole.pTotal：风洞参考总压
//   - sevenHole.pTunnelStatic：风洞参考静压
//   - sevenHole.pAtm：大气压力
//   - sevenHole.tAtm：大气温度
const probeChannels = ref<ProbeChannelConfig[]>([
  { name: t.value.sh_sevenHoleP1, role: 'sevenHole.p1', channel: { deviceId: '', channelIndex: 0 }, enabled: true, precision: DEFAULT_CALIBRATION_PROBE_PRECISION },
  { name: t.value.sh_sevenHoleP2, role: 'sevenHole.p2', channel: { deviceId: '', channelIndex: 1 }, enabled: true, precision: DEFAULT_CALIBRATION_PROBE_PRECISION },
  { name: t.value.sh_sevenHoleP3, role: 'sevenHole.p3', channel: { deviceId: '', channelIndex: 2 }, enabled: true, precision: DEFAULT_CALIBRATION_PROBE_PRECISION },
  { name: t.value.sh_sevenHoleP4, role: 'sevenHole.p4', channel: { deviceId: '', channelIndex: 3 }, enabled: true, precision: DEFAULT_CALIBRATION_PROBE_PRECISION },
  { name: t.value.sh_sevenHoleP5, role: 'sevenHole.p5', channel: { deviceId: '', channelIndex: 4 }, enabled: true, precision: DEFAULT_CALIBRATION_PROBE_PRECISION },
  { name: t.value.sh_sevenHoleP6, role: 'sevenHole.p6', channel: { deviceId: '', channelIndex: 5 }, enabled: true, precision: DEFAULT_CALIBRATION_PROBE_PRECISION },
  { name: t.value.sh_sevenHoleP7, role: 'sevenHole.p7', channel: { deviceId: '', channelIndex: 6 }, enabled: true, precision: DEFAULT_CALIBRATION_PROBE_PRECISION },
  { name: t.value.sh_sevenHolePTotal, role: 'sevenHole.pTotal', channel: { deviceId: '', channelIndex: -1 }, enabled: true, precision: DEFAULT_CALIBRATION_PROBE_PRECISION },
  { name: t.value.sh_sevenHolePTunnelStatic, role: 'sevenHole.pTunnelStatic', channel: { deviceId: '', channelIndex: -1 }, enabled: true, precision: DEFAULT_CALIBRATION_PROBE_PRECISION },
  { name: t.value.sh_sevenHolePAtm, role: 'sevenHole.pAtm', channel: { deviceId: '', channelIndex: 16 }, enabled: true, precision: DEFAULT_CALIBRATION_PROBE_PRECISION },
  { name: t.value.sh_sevenHoleTAtm, role: 'sevenHole.tAtm', channel: { deviceId: '', channelIndex: 17 }, enabled: true, precision: DEFAULT_CALIBRATION_PROBE_PRECISION },
])

// ==================== 运动轴配置 ====================
// 七孔探针需 α/β 双轴联动（与五孔一致），内区 α-β 网格扫描 + 外区 θ-φ 通过 α-β 运动学映射
const motionAxes = ref<MotionAxisConfig[]>([
  { name: 'Alpha', controllerId: '', axis: 'X' },
  { name: 'Beta', controllerId: '', axis: 'Y' },
])

// 运动安全配置：复用遍历测试模块的 MotionSafetyPanel 共享组件
const motionSafety = ref<MotionSafetyConfig | undefined>(undefined)

const deviceList = computed(() => deviceStore.profiles)
const motionControllerList = computed(() => motionStore.profiles)

// 通道批量操作：统一选择设备 + 通道号自动递增填充
const {
  batchDeviceId,
  autoFillStartIndex,
  applyDeviceToAll,
  autoFillChannelIndices,
} = useChannelBatchOperations(probeChannels)

const deviceOptions = computed(() =>
  deviceList.value.map((d) => ({ label: `${d.name} (${d.type})`, value: d.id })),
)

// 11 个必需通道角色（与后端 requiredRoles 严格一致）
const REQUIRED_CHANNEL_ROLES = [
  'sevenHole.p1', 'sevenHole.p2', 'sevenHole.p3', 'sevenHole.p4',
  'sevenHole.p5', 'sevenHole.p6', 'sevenHole.p7',
  'sevenHole.pTotal', 'sevenHole.pTunnelStatic',
  'sevenHole.pAtm', 'sevenHole.tAtm',
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
  {
    key: 'probe',
    label: t.value.sh_sevenHoleProbeGroup,
    icon: Activity,
    roles: ['sevenHole.p1', 'sevenHole.p2', 'sevenHole.p3', 'sevenHole.p4', 'sevenHole.p5', 'sevenHole.p6', 'sevenHole.p7'],
    channels: probeChannels.value.filter((ch) => [
      'sevenHole.p1', 'sevenHole.p2', 'sevenHole.p3', 'sevenHole.p4',
      'sevenHole.p5', 'sevenHole.p6', 'sevenHole.p7',
    ].includes(ch.role || '')),
  },
  {
    key: 'atmosphere',
    label: t.value.atmosphereGroup,
    icon: Wind,
    roles: ['sevenHole.pAtm', 'sevenHole.tAtm'],
    channels: probeChannels.value.filter((ch) => ['sevenHole.pAtm', 'sevenHole.tAtm'].includes(ch.role || '')),
  },
  {
    key: 'windTunnel',
    label: t.value.windTunnelGroup,
    icon: Gauge,
    roles: ['sevenHole.pTotal', 'sevenHole.pTunnelStatic'],
    channels: probeChannels.value.filter((ch) => ['sevenHole.pTotal', 'sevenHole.pTunnelStatic'].includes(ch.role || '')),
  },
])

const mappedChannelCount = computed(
  () =>
    probeChannels.value.filter(
      (ch) => ch.enabled && ch.channel.deviceId && ch.channel.channelIndex >= 0,
    ).length,
)
const totalRequiredChannelCount = REQUIRED_CHANNEL_ROLES.length

// ==================== 点位预览（API 驱动，500ms 防抖） ====================
// 严格约束：禁止本地生成点位，必须调用 calibrationApi.previewSevenHolePoints
const previewResult = ref<SevenHolePreviewResult | null>(null)
const previewLoading = ref(false)
const previewError = ref<string>('')

// 浮点容差：用于范围整除性判定，与 FiveHoleSettings 保持一致
const FLOAT_EPSILON = 1e-9
function isRangeDivisible(range: number, step: number): boolean {
  if (step <= 0) return false
  return Math.abs(range / step - Math.round(range / step)) < FLOAT_EPSILON
}

/**
 * 七孔点位布局本地校验
 *
 * 调用 previewSevenHolePoints 之前先做基础校验，避免无效参数发给后端。
 * 校验规则与 FiveHoleSettings.validatePointLayout 同构：
 *   - 步长必须 > 0
 *   - max 必须 > min
 *   - 范围必须能被步长整除（防止浮点累加误差导致点数不一致）
 *
 * 数据集模式下外区 θ/φ 不使用用户配置，跳过外区校验。
 */
function validateSevenHoleLayout(): { valid: boolean; errors: string[] } {
  const errors: string[] = []
  const layout = sevenHoleLayout.value

  // 内区 α/β 校验（两种模式都使用）
  if (layout.innerAlphaStep <= 0 || layout.innerBetaStep <= 0) {
    errors.push(t.value.stepMustPositive)
  }
  if (layout.innerAlphaMax <= layout.innerAlphaMin || layout.innerBetaMax <= layout.innerBetaMin) {
    errors.push(t.value.maxGreaterThanMin)
  }
  if (
    !isRangeDivisible(layout.innerAlphaMax - layout.innerAlphaMin, layout.innerAlphaStep) ||
    !isRangeDivisible(layout.innerBetaMax - layout.innerBetaMin, layout.innerBetaStep)
  ) {
    errors.push(t.value.rangeDivisible)
  }

  // 外区 θ/φ 校验（仅完整模式生效）
  if (layout.mode === 'full') {
    if (layout.outerThetaStep <= 0 || layout.outerPhiStep <= 0) {
      errors.push(t.value.stepMustPositive)
    }
    if (layout.outerThetaMax <= layout.outerThetaMin || layout.outerPhiMax <= layout.outerPhiMin) {
      errors.push(t.value.maxGreaterThanMin)
    }
    if (
      !isRangeDivisible(layout.outerThetaMax - layout.outerThetaMin, layout.outerThetaStep) ||
      !isRangeDivisible(layout.outerPhiMax - layout.outerPhiMin, layout.outerPhiStep)
    ) {
      errors.push(t.value.rangeDivisible)
    }
  }

  return { valid: errors.length === 0, errors }
}

const layoutValidation = computed(() => validateSevenHoleLayout())

const pointCount = computed(() => previewResult.value?.totalCount ?? 0)
const innerCount = computed(() => previewResult.value?.innerCount ?? 0)
const outerCount = computed(() => previewResult.value?.outerCount ?? 0)

/**
 * 点位预览防抖触发器
 *
 * 监听 sevenHoleLayout 变化，500ms 防抖后调用后端 API：
 *   - 本地校验通过：调 previewSevenHolePoints 获取真实点位
 *   - 本地校验失败：清空预览，不调用 API
 *   - API 调用失败：清空预览，记录错误信息（用于"请先连接后端"提示）
 */
let previewTimer: ReturnType<typeof setTimeout> | null = null
async function refreshPreview() {
  if (previewTimer) {
    clearTimeout(previewTimer)
    previewTimer = null
  }
  if (!layoutValidation.value.valid) {
    previewResult.value = null
    previewError.value = layoutValidation.value.errors.join('；')
    return
  }
  previewLoading.value = true
  previewError.value = ''
  try {
    const result = await calibrationApi.previewSevenHolePoints({ ...sevenHoleLayout.value })
    previewResult.value = result
  } catch (err) {
    // 离线场景：禁止 fallback 到本地生成，仅显示"请先连接后端"提示
    previewResult.value = null
    previewError.value = t.value.sh_offlineHint
    console.warn('previewSevenHolePoints failed:', err)
  } finally {
    previewLoading.value = false
  }
}

function schedulePreviewRefresh() {
  if (previewTimer) clearTimeout(previewTimer)
  previewTimer = setTimeout(() => {
    previewTimer = null
    void refreshPreview()
  }, 500)
}

// 监听布局变化触发防抖预览（deep：SevenHoleConfig 字段级变更）
watch(sevenHoleLayout, () => schedulePreviewRefresh(), { deep: true })

onBeforeUnmount(() => {
  if (previewTimer) {
    clearTimeout(previewTimer)
    previewTimer = null
  }
})

// ==================== SVG 点阵预览投影 ====================
// 七孔点位含双坐标系（内区 α-β / 外区 θ-φ），统一投影到 SVG [0,200]×[0,200] 画布：
//   - 提取每个点的两个坐标值（顺序：first→x, second→y）
//   - 全局 min/max 归一化到 [20,180]×[180,20]（Y 翻转适配 SVG 坐标系）
//   - 内区点用 accent-primary 色，外区点用 axis-y 色（视觉区分两套坐标系）
interface PreviewDot {
  cx: number
  cy: number
  region: 'inner' | 'outer'
}

const previewDots = computed<PreviewDot[]>(() => {
  const points = previewResult.value?.points
  if (!points || points.length === 0) return []

  // 第一轮：收集所有 (x, y) 与 region 标记
  const raw = points.map((p: CalibrationPoint) => {
    const keys = Object.keys(p.coordinates)
    const x = p.coordinates[keys[0]] ?? 0
    const y = p.coordinates[keys[1]] ?? 0
    // 通过是否有 θ 键判定外区点（与后端 region 字段语义一致）
    const region: 'inner' | 'outer' = keys.includes('θ') ? 'outer' : 'inner'
    return { x, y, region }
  })

  // 第二轮：全局 min/max 归一化
  const xs = raw.map((r) => r.x)
  const ys = raw.map((r) => r.y)
  const xMin = Math.min(...xs)
  const xMax = Math.max(...xs)
  const yMin = Math.min(...ys)
  const yMax = Math.max(...ys)
  const xRange = xMax - xMin || 1
  const yRange = yMax - yMin || 1

  return raw.map((r) => ({
    cx: Math.round((20 + ((r.x - xMin) / xRange) * 160) * 10) / 10,
    cy: Math.round((180 - ((r.y - yMin) / yRange) * 160) * 10) / 10,
    region: r.region,
  }))
})

// ==================== 步骤校验 ====================
const currentStepErrors = computed<string[]>(() => {
  if (currentStep.value === 0) {
    const errors: string[] = []
    if (calibrationName.value.trim() === '') errors.push(t.value.enterConfigName)
    if (savePath.value.trim() === '') errors.push(t.value.fh_pleaseSelectCsvPath)
    if (saveFileName.value.trim() === '') errors.push(t.value.fh_pleaseInputCsvFileName)
    // 点位布局相关错误统一由 validateSevenHoleLayout 提供
    errors.push(...layoutValidation.value.errors)
    if (dwellTimeMs.value < 100) errors.push(t.value.dwellTimeMin)
    if (samplesPerPoint.value < 1) errors.push(t.value.samplesMin)
    return errors
  }
  if (currentStep.value === 1) {
    const errors: string[] = []
    // 11 通道必填校验：每个角色都必须映射到具体通道
    const enabledRoles = new Set(probeChannels.value.filter((ch) => ch.enabled).map((ch) => ch.role))
    const missingRoles = REQUIRED_CHANNEL_ROLES.filter((role) => !enabledRoles.has(role))
    if (missingRoles.length > 0) errors.push(t.value.requiredChannelsMissing)
    const invalidChannel = probeChannels.value.find((ch) => ch.enabled && (!ch.channel.deviceId || ch.channel.channelIndex < 0))
    if (invalidChannel) errors.push(`${t.value.channelMapIncomplete}: ${getProbeChannelDisplayName(invalidChannel.role, invalidChannel.name, t.value)}`)
    const missingControllerAxis = motionAxes.value.find((axis) => !axis.controllerId)
    if (missingControllerAxis) errors.push(`${t.value.axisNotBound}: ${missingControllerAxis.name}`)
    if (sphereTankGateEnabled.value && !sphereTankStableChannel.value.deviceId) errors.push(t.value.sphereTankRequiresDevice)
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

// ==================== CSV 保存路径操作 ====================
async function pickSavePath() {
  try {
    const picked = await storageStore.pickDirectory()
    if (picked) savePath.value = picked
  } catch (e) {
    feedbackStore.pushToast(t.value.sh_previewFailed + ': ' + (e instanceof Error ? e.message : String(e)), 'error')
  }
}

// calibrationName 变化时同步刷新默认 saveFileName（与五孔一致）
watch(calibrationName, (next, prev) => {
  const prevDefault = prev.trim() ? buildCalibrationCsvName(prev.trim(), 'seven-hole') : ''
  if (saveFileName.value === '' || saveFileName.value === prevDefault) {
    saveFileName.value = buildCalibrationCsvName(next.trim(), 'seven-hole')
  }
})

/**
 * 归一化 SevenHoleConfig 字段
 *
 * UiInputNumber 在清空-输入中间态会 emit null，归一化为 number 避免脏值落盘。
 * 与 FiveHoleSettings.sanitizePointLayout 同构。
 */
function sanitizeSevenHoleLayout(layout: SevenHoleConfig): SevenHoleConfig {
  const fix = (v: number | null | undefined): number => {
    if (v === null || v === undefined || !Number.isFinite(v)) return 0
    return v
  }
  return {
    mode: layout.mode,
    innerAlphaMin: fix(layout.innerAlphaMin),
    innerAlphaMax: fix(layout.innerAlphaMax),
    innerAlphaStep: fix(layout.innerAlphaStep),
    innerBetaMin: fix(layout.innerBetaMin),
    innerBetaMax: fix(layout.innerBetaMax),
    innerBetaStep: fix(layout.innerBetaStep),
    outerThetaMin: fix(layout.outerThetaMin),
    outerThetaMax: fix(layout.outerThetaMax),
    outerThetaStep: fix(layout.outerThetaStep),
    outerPhiMin: fix(layout.outerPhiMin),
    outerPhiMax: fix(layout.outerPhiMax),
    outerPhiStep: fix(layout.outerPhiStep),
    serpentine: layout.serpentine === true,
  }
}

async function saveConfig() {
  isSaving.value = true
  try {
    // 保存前显式校验点位布局，拦截 UiInputNumber 中间态脏值
    if (!layoutValidation.value.valid) {
      feedbackStore.pushToast(t.value.sh_previewFailed + ': ' + layoutValidation.value.errors.join('；'), 'warning')
      return
    }
    // 保存前再次清洗 saveFileName：剥离 .csv 后缀 + 非法字符过滤
    const normName = buildCalibrationCsvName(saveFileName.value.replace(/\.csv$/i, ''), calibrationName.value.trim())
    saveFileName.value = normName
    const fullSavePath = savePath.value.trim() ? joinCalibrationPath(savePath.value.trim(), normName) : ''
    const sanitizedLayout = sanitizeSevenHoleLayout(sevenHoleLayout.value)

    // 点位列表：优先使用预览结果（已含完整点位），否则空数组由后端按布局重新生成
    // 注意：previewResult 在离线场景下为 null，此时 points 落空，
    // 后端 Start 阶段会根据 sevenHoleLayout 重新生成（与五孔 generateFiveHoleSnakePoints 同语义）
    const points = previewResult.value?.points ?? []

    const config: CalibrationConfig = {
      type: 'seven-hole',
      name: calibrationName.value,
      probeChannels: probeChannels.value.filter((ch) => ch.enabled),
      motionAxes: motionAxes.value,
      motionSafety: motionSafety.value,
      points,
      dwellTimeMs: dwellTimeMs.value,
      samplesPerPoint: samplesPerPoint.value,
      savePath: fullSavePath,
      saveFileName: normName,
      sevenHoleLayout: sanitizedLayout,
      derivedValuePrecision: { machNumber: machNumberPrecision.value, velocity: velocityPrecision.value },
      uiRefreshHz: calibrationStore.uiRefreshHz,
      sphereTankGate: {
        enabled: sphereTankGateEnabled.value,
        waitTimeSec: Math.max(0, sphereTankWaitTimeSec.value),
        stableTimeChannel: { ...sphereTankStableChannel.value },
        // 仅当配置了压力通道 deviceId 时才落盘，避免写入空 ChannelRef 造成后端误订阅
        ...(sphereTankPressureChannel.value.deviceId
          ? { pressureChannel: { ...sphereTankPressureChannel.value } }
          : {}),
      },
    }
    const normalizedConfig = applyCalibrationPrecisionDefaults(config)
    const res = await calibrationApi.saveConfig('seven-hole', normalizedConfig)
    if (!res.success) throw new Error(res.error || t.value.fh_saveFailed)
    emit('saved', normalizedConfig)
    emit('close')
  } catch (err) {
    feedbackStore.pushToast(t.value.fh_saveFailedColon + (err instanceof Error ? err.message : String(err)), 'error')
  } finally { isSaving.value = false }
}

// 加载已保存配置：与五孔 loadSavedConfig 同构
async function loadSavedConfig() {
  try {
    const res = await calibrationApi.getConfig('seven-hole')
    const config = res.success && res.data ? applyCalibrationPrecisionDefaults(res.data) : null
    if (!config) return
    calibrationName.value = config.name
    if (config.sevenHoleLayout) {
      sevenHoleLayout.value = {
        ...sevenHoleLayout.value,
        ...config.sevenHoleLayout,
        serpentine: config.sevenHoleLayout.serpentine === true,
      }
    }
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
    if (config.motionSafety) {
      motionSafety.value = { ...config.motionSafety }
    } else {
      motionSafety.value = undefined
    }
    dwellTimeMs.value = config.dwellTimeMs
    samplesPerPoint.value = config.samplesPerPoint
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
      // 压力通道为可选字段，未配置时回退为空 deviceId（UI 显示"暂无数据"）
      sphereTankPressureChannel.value = config.sphereTankGate.pressureChannel
        ? { ...config.sphereTankGate.pressureChannel }
        : { deviceId: '', channelIndex: 0 }
    }
  } catch (err) {
    const msg = err instanceof Error ? err.message : String(err)
    if (!msg.includes('404') && !msg.includes('not found')) {
      feedbackStore.pushToast(t.value.wf_loadConfigFailed + msg, 'warning')
    }
  }
}

onMounted(async () => {
  try {
    const results = await Promise.allSettled([
      deviceStore.refreshProfiles(),
      motionStore.refreshProfiles(),
      loadSavedConfig(),
    ])
    reportAllSettledFailures(
      results,
      [t.value.deviceList, t.value.sh_motionControllerList, t.value.sh_sevenHoleCalibrationConfig],
      feedbackStore.pushToast,
    )
    // calibrationName 兜底：使用 ISO 日期避免 toLocaleDateString 在中文环境返回含斜杠的日期
    if (!calibrationName.value) calibrationName.value = `${t.value.sh_defaultCalibNamePrefix}-${new Date().toISOString().slice(0, 10)}`
    if (!saveFileName.value.trim()) saveFileName.value = buildCalibrationCsvName(calibrationName.value.trim(), 'seven-hole')
    // savePath 兜底：回退到全局基础目录
    if (!savePath.value.trim()) savePath.value = storageStore.settings?.baseDirectory?.trim() ?? ''
    // 首次加载触发一次预览（不防抖）
    void refreshPreview()
  } finally { isLoading.value = false }
})

// ==================== UI 选项配置 ====================
const modeOptions = computed(() => [
  { label: t.value.sh_modeFull, value: 'full' as SevenHoleMode },
  { label: t.value.sh_modeDataset, value: 'dataset' as SevenHoleMode },
])

const axisOptions = computed(() => [
  { label: t.value.sh_axisX, value: 'X' },
  { label: t.value.sh_axisY, value: 'Y' },
  { label: t.value.sh_axisZ, value: 'Z' },
  { label: t.value.sh_axisU, value: 'U' },
])

// 通道索引枚举选项：UI 显示 CH1~CH18（1-based），内部 value 为 0-based 索引
const channelIndexOptions = Array.from({ length: 18 }, (_, i) => ({ label: `CH${i + 1}`, value: String(i) }))

const channelColumns = [
  { key: 'enabled', label: t.value.sh_enable },
  { key: 'group', label: t.value.batchConfig },
  { key: 'name', label: t.value.sh_configName },
  { key: 'device', label: t.value.sh_selectDevice },
  { key: 'channel', label: t.value.sh_stableChannel },
  { key: 'precision', label: t.value.sh_machNumberPrecision },
] as const

function groupKeyOfRole(role: string | undefined): string {
  if (!role) return ''
  for (const g of channelGroups.value) {
    if (g.roles.includes(role)) return g.key
  }
  return ''
}

function getChannelGroupLabel(groupKey: string): string {
  const labels: Record<string, string> = {
    probe: t.value.sh_sevenHoleProbeGroupShort,
    atmosphere: t.value.atmosphereGroupShort,
    windTunnel: t.value.windTunnelGroupShort,
  }
  return labels[groupKey] || '—'
}

// 步骤指示器文案
const stepIndicatorText = computed(() =>
  t.value.sh_stepIndicator
    .replace('{current}', String(currentStep.value + 1))
    .replace('{total}', String(steps.value.length)),
)

// 球罐启用摘要文案
const sphereTankSummaryText = computed(() =>
  sphereTankGateEnabled.value
    ? t.value.sh_sphereTankEnabled.replace('{sec}', String(sphereTankWaitTimeSec.value))
    : t.value.sh_sphereTankDisabled,
)
</script>

<template>
  <UiDialog :show="true" width="min(92vw, 960px)" :title="t.sh_dialogTitle" closable @update:show="emit('close')">
    <UiSteps :current="currentStep" class="steps-mb">
      <UiStep v-for="(step, idx) in steps" :key="idx" :title="step" :disabled="idx > currentStep" />
    </UiSteps>

    <UiSpin v-if="isLoading" class="spinner" />

    <template v-else>
      <div class="calib-body">
        <div class="calib-main">
          <!-- 错误提示：列出全部错误，便于一次性修正 -->
          <UiAlert
            v-if="currentStepErrors.length > 0"
            type="warning"
            :title="`${t.sh_pleaseFixIssues} (${currentStepErrors.length})`"
            class="alert-error"
          >
            <ul class="error-list">
              <li v-for="(err, i) in currentStepErrors" :key="i">{{ err }}</li>
            </ul>
          </UiAlert>

          <!-- 步骤 0：基本设置 -->
          <div v-if="currentStep === 0" class="step-content">
            <!-- 基础信息：配置名称 + 刷新频率 -->
            <UiPanel class="section-card">
              <template #header>
                <div class="section-header">
                  <FileText :size="14" />
                  <span>{{ t.sh_basicInfo }}</span>
                </div>
              </template>
              <div class="basic-grid">
                <div class="field">
                  <span class="field-label">{{ t.sh_configName }}</span>
                  <UiInput v-model="calibrationName" :placeholder="t.sh_configNamePlaceholder" />
                </div>
                <div class="field field--fixed">
                  <span class="field-label">{{ t.sh_uiRefreshHz }}</span>
                  <UiSelect
                    :model-value="String(calibrationStore.uiRefreshHz)"
                    :options="Array.from({ length: 20 }, (_, i) => ({ label: `${i + 1} Hz`, value: String(i + 1) }))"
                    @update:model-value="calibrationStore.setUiRefreshHz(Number($event))"
                  />
                </div>
              </div>
            </UiPanel>

            <!-- 校准模式：完整 / 数据集 -->
            <UiPanel class="section-card">
              <template #header>
                <div class="section-header">
                  <Target :size="14" />
                  <span>{{ t.sh_calibrationMode }}</span>
                </div>
              </template>
              <div class="mode-grid">
                <UiSelect
                  :model-value="sevenHoleLayout.mode"
                  :options="modeOptions"
                  @update:model-value="sevenHoleLayout.mode = $event as SevenHoleMode"
                />
                <p class="mode-desc">
                  {{ sevenHoleLayout.mode === 'full' ? t.sh_modeFullDesc : t.sh_modeDatasetDesc }}
                </p>
              </div>
            </UiPanel>

            <!-- 点位布局：内区 α/β + 外区 θ/φ -->
            <UiPanel class="section-card">
              <template #header>
                <div class="section-header">
                  <Move3D :size="14" />
                  <span>{{ t.sh_pointLayout }}</span>
                </div>
              </template>
              <div class="layout-params">
                <!-- 内区 α -->
                <div class="angle-block">
                  <div class="angle-block-header">
                    <span class="angle-tag alpha-tag">α</span>
                    <span class="angle-block-title">{{ t.sh_innerAlpha }}</span>
                  </div>
                  <div class="angle-fields">
                    <div class="field">
                      <span class="field-label">{{ t.sh_minValueDeg }}</span>
                      <UiInputNumber v-model="sevenHoleLayout.innerAlphaMin" />
                    </div>
                    <div class="field">
                      <span class="field-label">{{ t.sh_maxValueDeg }}</span>
                      <UiInputNumber v-model="sevenHoleLayout.innerAlphaMax" />
                    </div>
                    <div class="field">
                      <span class="field-label">{{ t.sh_stepDeg }}</span>
                      <UiInputNumber v-model="sevenHoleLayout.innerAlphaStep" :min="1" />
                    </div>
                  </div>
                </div>
                <!-- 内区 β -->
                <div class="angle-block">
                  <div class="angle-block-header">
                    <span class="angle-tag beta-tag">β</span>
                    <span class="angle-block-title">{{ t.sh_innerBeta }}</span>
                  </div>
                  <div class="angle-fields">
                    <div class="field">
                      <span class="field-label">{{ t.sh_minValueDeg }}</span>
                      <UiInputNumber v-model="sevenHoleLayout.innerBetaMin" />
                    </div>
                    <div class="field">
                      <span class="field-label">{{ t.sh_maxValueDeg }}</span>
                      <UiInputNumber v-model="sevenHoleLayout.innerBetaMax" />
                    </div>
                    <div class="field">
                      <span class="field-label">{{ t.sh_stepDeg }}</span>
                      <UiInputNumber v-model="sevenHoleLayout.innerBetaStep" :min="1" />
                    </div>
                  </div>
                </div>
                <!-- 外区 θ -->
                <div class="angle-block" :class="{ 'angle-block--disabled': sevenHoleLayout.mode === 'dataset' }">
                  <div class="angle-block-header">
                    <span class="angle-tag theta-tag">θ</span>
                    <span class="angle-block-title">{{ t.sh_outerTheta }}</span>
                  </div>
                  <div class="angle-fields">
                    <div class="field">
                      <span class="field-label">{{ t.sh_minValueDeg }}</span>
                      <UiInputNumber v-model="sevenHoleLayout.outerThetaMin" :disabled="sevenHoleLayout.mode === 'dataset'" />
                    </div>
                    <div class="field">
                      <span class="field-label">{{ t.sh_maxValueDeg }}</span>
                      <UiInputNumber v-model="sevenHoleLayout.outerThetaMax" :disabled="sevenHoleLayout.mode === 'dataset'" />
                    </div>
                    <div class="field">
                      <span class="field-label">{{ t.sh_stepDeg }}</span>
                      <UiInputNumber v-model="sevenHoleLayout.outerThetaStep" :min="1" :disabled="sevenHoleLayout.mode === 'dataset'" />
                    </div>
                  </div>
                </div>
                <!-- 外区 φ -->
                <div class="angle-block" :class="{ 'angle-block--disabled': sevenHoleLayout.mode === 'dataset' }">
                  <div class="angle-block-header">
                    <span class="angle-tag phi-tag">φ</span>
                    <span class="angle-block-title">{{ t.sh_outerPhi }}</span>
                  </div>
                  <div class="angle-fields">
                    <div class="field">
                      <span class="field-label">{{ t.sh_minValueDeg }}</span>
                      <UiInputNumber v-model="sevenHoleLayout.outerPhiMin" :disabled="sevenHoleLayout.mode === 'dataset'" />
                    </div>
                    <div class="field">
                      <span class="field-label">{{ t.sh_maxValueDeg }}</span>
                      <UiInputNumber v-model="sevenHoleLayout.outerPhiMax" :disabled="sevenHoleLayout.mode === 'dataset'" />
                    </div>
                    <div class="field">
                      <span class="field-label">{{ t.sh_stepDeg }}</span>
                      <UiInputNumber v-model="sevenHoleLayout.outerPhiStep" :min="1" :disabled="sevenHoleLayout.mode === 'dataset'" />
                    </div>
                  </div>
                </div>
              </div>
              <div class="layout-options">
                <UiCheckbox v-model:checked="sevenHoleLayout.serpentine">
                  {{ t.sh_serpentine }}
                </UiCheckbox>
              </div>
            </UiPanel>

            <!-- 不确定度参数：k / TIE_BREAK_TOLERANCE / 采样次数 / 驻留时间 -->
            <UiPanel class="section-card">
              <template #header>
                <div class="section-header">
                  <ShieldCheck :size="14" />
                  <span>{{ t.sh_uncertaintyParams }}</span>
                </div>
              </template>
              <div class="param-grid-4">
                <div class="field">
                  <span class="field-label">{{ t.sh_coverageFactorK }}</span>
                  <UiInputNumber v-model="coverageFactorK" :min="1" :max="3" />
                </div>
                <div class="field">
                  <span class="field-label">{{ t.sh_tieBreakTolerance }}</span>
                  <UiInputNumber v-model="tieBreakTolerance" :min="0" :step="0.0001" />
                </div>
                <div class="field">
                  <span class="field-label">{{ t.sh_samplesPerPoint }}</span>
                  <UiInputNumber v-model="samplesPerPoint" :min="1" :max="1000" />
                </div>
                <div class="field">
                  <span class="field-label">{{ t.sh_dwellTimeMs }}</span>
                  <UiInputNumber v-model="dwellTimeMs" :min="100" :step="100" />
                </div>
              </div>
            </UiPanel>

            <!-- 采集与输出：精度参数 -->
            <UiPanel class="section-card">
              <template #header>
                <div class="section-header">
                  <Timer :size="14" />
                  <span>{{ t.sh_acquisitionOutput }}</span>
                </div>
              </template>
              <div class="param-grid-2">
                <div class="field">
                  <span class="field-label">{{ t.sh_machNumberPrecision }}</span>
                  <UiInputNumber v-model="machNumberPrecision" :min="0" :max="8" />
                </div>
                <div class="field">
                  <span class="field-label">{{ t.sh_velocityPrecision }}</span>
                  <UiInputNumber v-model="velocityPrecision" :min="0" :max="8" />
                </div>
              </div>
            </UiPanel>

            <!-- CSV 保存 -->
            <UiPanel class="section-card">
              <template #header>
                <div class="section-header">
                  <Save :size="14" />
                  <span>{{ t.sh_csvSave }}</span>
                </div>
              </template>
              <div class="field">
                <span class="field-label">{{ t.sh_saveDirectory }}</span>
                <div class="flex items-center gap-2">
                  <UiInput v-model="savePath" :placeholder="t.sh_selectSaveDirectoryHint" class="flex-1" :title="savePath" />
                  <UiButton size="sm" variant="secondary" @click="pickSavePath">{{ t.sh_selectDirectory }}</UiButton>
                </div>
              </div>
              <div class="field">
                <span class="field-label">{{ t.sh_csvFileName }}</span>
                <UiInput v-model="saveFileName" :placeholder="t.fh_pleaseInputCsvFileName" class="flex-1" />
              </div>
            </UiPanel>
          </div>

          <!-- 步骤 1：硬件配置 -->
          <div v-if="currentStep === 1" class="step-content hardware-step">
            <!-- 通道映射进度条 -->
            <div class="mapping-progress">
              <span class="mapping-progress-label">{{ t.sh_channelMapping }}</span>
              <div class="mapping-progress-bar">
                <div
                  class="mapping-progress-fill"
                  :style="{ width: `${(mappedChannelCount / totalRequiredChannelCount) * 100}%` }"
                />
              </div>
              <span class="mapping-progress-text">{{ mappedChannelCount }} / {{ totalRequiredChannelCount }}</span>
            </div>

            <!-- 批量操作工具栏 -->
            <div class="batch-toolbar">
              <div class="batch-toolbar-row">
                <div class="batch-cell">
                  <span class="batch-label">{{ t.unifiedDevice }}</span>
                  <UiSelect
                    v-model="batchDeviceId"
                    :options="deviceOptions"
                    :placeholder="t.sh_selectDevice"
                    class="batch-select"
                  />
                </div>
                <div class="batch-cell">
                  <UiButton size="sm" variant="primary" :disabled="!batchDeviceId" @click="applyDeviceToAll">{{ t.applyToAllChannels }}</UiButton>
                </div>
              </div>
              <div class="batch-toolbar-row">
                <div class="batch-cell">
                  <span class="batch-label">{{ t.startChannel }}</span>
                  <UiSelect
                    :model-value="autoFillStartIndex !== null ? String(autoFillStartIndex) : ''"
                    :options="channelIndexOptions"
                    :placeholder="t.selectStartChannel"
                    class="batch-select"
                    @update:model-value="autoFillStartIndex = $event !== '' ? Number($event) : null"
                  />
                </div>
                <div class="batch-cell">
                  <UiButton size="sm" variant="primary" :disabled="autoFillStartIndex === null" @click="autoFillChannelIndices">{{ t.autoIncrementFill }}</UiButton>
                </div>
              </div>
            </div>

            <!-- 通道映射表 -->
            <UiPanel class="section-card">
              <template #header>
                <div class="section-header">
                  <Activity :size="14" />
                  <span>{{ t.sh_channelMapping }}</span>
                  <span class="section-count">{{ probeChannels.length }} {{ t.sh_channelsUnit }}</span>
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
                          :placeholder="t.sh_selectDevice"
                          :disabled="!ch.enabled"
                          :fallback="false"
                        />
                      </td>
                      <td class="cell-channel">
                        <UiSelect
                          :model-value="ch.channel.channelIndex >= 0 ? String(ch.channel.channelIndex) : ''"
                          :options="channelIndexOptions"
                          :placeholder="t.sh_unassigned"
                          :disabled="!ch.enabled"
                          @update:model-value="ch.channel.channelIndex = $event !== '' ? Number($event) : -1"
                        />
                      </td>
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
                  <span>{{ t.sh_motionAxisConfig }}</span>
                </div>
              </template>
              <div class="table-wrap">
                <table class="ntable">
                  <thead><tr><th>{{ t.sh_motionAxes }}</th><th class="col-controller">{{ t.sh_selectController }}</th><th class="col-axis">{{ t.sh_motionAxisConfig }}</th></tr></thead>
                  <tbody>
                    <tr v-for="axis in motionAxes" :key="axis.name">
                      <td><UiStatusBadge status="connected">{{ axis.name }}</UiStatusBadge></td>
                      <td>
                        <UiSelect
                          v-model="axis.controllerId"
                          :options="motionControllerList.map(c => ({ label: `${c.name} (${c.type})`, value: c.id }))"
                          :placeholder="t.sh_selectController"
                        />
                      </td>
                      <td><UiSelect v-model="axis.axis" :options="axisOptions" /></td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </UiPanel>

            <!-- 球罐判定门控：放在运动安全面板上方，便于操作员优先确认球罐压力条件 -->
            <UiPanel class="section-card">
              <template #header>
                <div class="section-header">
                  <ShieldCheck :size="14" />
                  <span>{{ t.sh_sphereTankGate }}</span>
                  <UiCheckbox v-model:checked="sphereTankGateEnabled" class="header-toggle">{{ t.sh_enable }}</UiCheckbox>
                </div>
              </template>
              <div v-if="sphereTankGateEnabled" class="sphere-grid">
                <div class="field">
                  <span class="field-label">{{ t.sh_waitTimeSec }}</span>
                  <UiInputNumber v-model="sphereTankWaitTimeSec" :min="0" :step="0.1" />
                </div>
                <div class="field">
                  <span class="field-label">{{ t.sh_stableChannelDevice }}</span>
                  <UiSelect v-model="sphereTankStableChannel.deviceId" :options="deviceList.map(d => ({ label: d.name, value: d.id }))" :placeholder="t.sh_selectDevice" :fallback="false" />
                </div>
                <div class="field">
                  <span class="field-label">{{ t.sh_stableChannel }}</span>
                  <UiSelect
                    :model-value="sphereTankStableChannel.channelIndex >= 0 ? String(sphereTankStableChannel.channelIndex) : ''"
                    :options="channelIndexOptions"
                    :placeholder="t.sh_selectDevice"
                    @update:model-value="sphereTankStableChannel.channelIndex = $event !== '' ? Number($event) : 0"
                  />
                </div>
                <!-- 球罐压力通道：仅用于实时显示压力值，不参与闸门判定 -->
                <div class="field">
                  <span class="field-label">{{ t.wf_spherePressureDevice }}</span>
                  <UiSelect v-model="sphereTankPressureChannel.deviceId" :options="deviceList.map(d => ({ label: d.name, value: d.id }))" :placeholder="t.sh_selectDevice" :fallback="false" />
                </div>
                <div class="field">
                  <span class="field-label">{{ t.wf_spherePressureChannel }}</span>
                  <UiSelect
                    :model-value="sphereTankPressureChannel.channelIndex >= 0 ? String(sphereTankPressureChannel.channelIndex) : ''"
                    :options="channelIndexOptions"
                    :placeholder="t.sh_selectDevice"
                    @update:model-value="sphereTankPressureChannel.channelIndex = $event !== '' ? Number($event) : 0"
                  />
                </div>
                <p class="sphere-hint">{{ t.sh_sphereTankHint }}</p>
                <p class="sphere-hint muted">{{ t.wf_spherePressureHint }}</p>
              </div>
              <p v-else class="empty-hint">{{ t.sh_sphereTankDisabledHint }}</p>
            </UiPanel>

            <!-- 运动安全配置：复用共享组件 -->
            <MotionSafetyPanel
              v-model:motion-safety="motionSafety"
              :motion-axes="motionAxes"
              :t="(t as unknown as Record<string, string>)"
            />
          </div>

          <!-- 步骤 2：确认保存 -->
          <div v-if="currentStep === 2" class="step-content">
            <UiPanel class="section-card">
              <template #header>
                <div class="section-header">
                  <ShieldCheck :size="14" />
                  <span>{{ t.sh_configSummary }}</span>
                </div>
              </template>
              <div class="summary-grid-2">
                <div class="summary-row">
                  <span class="summary-label">{{ t.sh_configName }}</span>
                  <span class="summary-value">{{ calibrationName }}</span>
                </div>
                <div class="summary-row">
                  <span class="summary-label">{{ t.sh_calibrationType }}</span>
                  <span class="summary-value">{{ t.sh_sevenHoleCalibration }}</span>
                </div>
                <div class="summary-row">
                  <span class="summary-label">{{ t.sh_calibrationMode }}</span>
                  <span class="summary-value">{{ sevenHoleLayout.mode === 'full' ? t.sh_modeFull : t.sh_modeDataset }}</span>
                </div>
                <div class="summary-row">
                  <span class="summary-label">{{ t.sh_innerAlpha }}</span>
                  <span class="summary-value">{{ sevenHoleLayout.innerAlphaMin }}° ~ {{ sevenHoleLayout.innerAlphaMax }}° ({{ sevenHoleLayout.innerAlphaStep }}°)</span>
                </div>
                <div class="summary-row">
                  <span class="summary-label">{{ t.sh_innerBeta }}</span>
                  <span class="summary-value">{{ sevenHoleLayout.innerBetaMin }}° ~ {{ sevenHoleLayout.innerBetaMax }}° ({{ sevenHoleLayout.innerBetaStep }}°)</span>
                </div>
                <div class="summary-row" v-if="sevenHoleLayout.mode === 'full'">
                  <span class="summary-label">{{ t.sh_outerTheta }}</span>
                  <span class="summary-value">{{ sevenHoleLayout.outerThetaMin }}° ~ {{ sevenHoleLayout.outerThetaMax }}° ({{ sevenHoleLayout.outerThetaStep }}°)</span>
                </div>
                <div class="summary-row" v-if="sevenHoleLayout.mode === 'full'">
                  <span class="summary-label">{{ t.sh_outerPhi }}</span>
                  <span class="summary-value">{{ sevenHoleLayout.outerPhiMin }}° ~ {{ sevenHoleLayout.outerPhiMax }}° ({{ sevenHoleLayout.outerPhiStep }}°)</span>
                </div>
                <div class="summary-row">
                  <span class="summary-label">{{ t.sh_walkMode }}</span>
                  <span class="summary-value" :class="sevenHoleLayout.serpentine ? 'accent-bold' : 'muted-text'">
                    {{ sevenHoleLayout.serpentine ? t.sh_serpentineMode : t.sh_rasterMode }}
                  </span>
                </div>
                <div class="summary-row">
                  <span class="summary-label">{{ t.sh_totalCount }}</span>
                  <span class="summary-value accent-bold">{{ pointCount }} {{ t.sh_pointsUnit }}</span>
                </div>
                <div class="summary-row">
                  <span class="summary-label">{{ t.sh_innerCount }}</span>
                  <span class="summary-value">{{ innerCount }} {{ t.sh_pointsUnit }}</span>
                </div>
                <div class="summary-row">
                  <span class="summary-label">{{ t.sh_outerCount }}</span>
                  <span class="summary-value">{{ outerCount }} {{ t.sh_pointsUnit }}</span>
                </div>
                <div class="summary-row">
                  <span class="summary-label">{{ t.sh_enabledChannels }}</span>
                  <span class="summary-value">{{ probeChannels.filter(ch => ch.enabled).length }} / {{ probeChannels.length }} {{ t.sh_countUnit }}</span>
                </div>
                <div class="summary-row">
                  <span class="summary-label">{{ t.sh_dwellTimeMs }}</span>
                  <span class="summary-value">{{ dwellTimeMs }} ms</span>
                </div>
                <div class="summary-row">
                  <span class="summary-label">{{ t.sh_samplesPerPoint }}</span>
                  <span class="summary-value">{{ samplesPerPoint }}</span>
                </div>
                <div class="summary-row">
                  <span class="summary-label">{{ t.sh_motionAxes }}</span>
                  <span class="summary-value">{{ motionAxes.map(a => `${a.name}→${a.axis}`).join('，') }}</span>
                </div>
                <div class="summary-row">
                  <span class="summary-label">{{ t.sh_sphereTankGate }}</span>
                  <span class="summary-value" :class="sphereTankGateEnabled ? 'accent-bold' : 'muted-text'">{{ sphereTankSummaryText }}</span>
                </div>
                <div class="summary-row">
                  <span class="summary-label">{{ t.sh_csvSave }}</span>
                  <span class="summary-value">{{ savePath && saveFileName ? joinCalibrationPath(savePath, saveFileName) : savePath }}</span>
                </div>
              </div>
            </UiPanel>
          </div>
        </div>

        <!-- 右侧 sidebar：跨步骤常驻统计 + SVG 点阵预览 -->
        <aside class="calib-sidebar">
          <div class="sidebar-stats">
            <div class="sidebar-stat sidebar-stat--highlight">
              <span class="stat-label">{{ t.sh_totalCount }}</span>
              <span class="stat-number">{{ pointCount }}</span>
            </div>
            <div class="sidebar-stat">
              <span class="stat-label">{{ t.sh_dwellTime }}</span>
              <span class="stat-value">{{ dwellTimeMs }}<span class="stat-unit">ms</span></span>
            </div>
            <div class="sidebar-stat">
              <span class="stat-label">{{ t.sh_sampleCount }}</span>
              <span class="stat-value">{{ samplesPerPoint }}</span>
            </div>
          </div>

          <!-- 内/外区点数细分 -->
          <div class="sidebar-region-stats">
            <div class="region-stat region-stat--inner">
              <span class="region-label">{{ t.sh_innerCount }}</span>
              <span class="region-value">{{ innerCount }}</span>
            </div>
            <div class="region-stat region-stat--outer">
              <span class="region-label">{{ t.sh_outerCount }}</span>
              <span class="region-value">{{ outerCount }}</span>
            </div>
          </div>

          <div class="sidebar-preview">
            <div class="preview-header">
              <LayoutGrid :size="14" />
              <span class="preview-title">{{ t.sh_pointPreview }}</span>
              <UiSpin v-if="previewLoading" :show="true" class="preview-spin" />
            </div>
            <div class="preview-canvas">
              <!-- 离线/错误提示：不 fallback 到本地生成 -->
              <div v-if="previewError" class="preview-error">
                <p>{{ previewError }}</p>
              </div>
              <svg v-else viewBox="0 0 200 200" class="preview-svg" aria-hidden="true">
                <line x1="20" y1="180" x2="180" y2="180" class="axis-line" />
                <line x1="20" y1="20" x2="20" y2="180" class="axis-line" />
                <circle
                  v-for="(dot, i) in previewDots"
                  :key="i"
                  :cx="dot.cx"
                  :cy="dot.cy"
                  r="2"
                  :class="dot.region === 'inner' ? 'preview-dot-inner' : 'preview-dot-outer'"
                />
                <text x="100" y="196" class="axis-label">α/θ</text>
                <text x="12" y="100" class="axis-label" transform="rotate(-90 12 100)">β/φ</text>
              </svg>
            </div>
            <div class="preview-summary">
              <span class="preview-count-label">{{ t.sh_totalCount }}</span>
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
            <ChevronLeft :size="14" class="icon-left" />{{ t.sh_prevStep }}
          </UiButton>
        </div>
        <div class="footer-actions">
          <span class="step-indicator">{{ stepIndicatorText }}</span>
          <UiButton v-if="currentStep < steps.length - 1" variant="primary" size="sm" :disabled="!isStepValid" @click="nextStep">
            {{ t.sh_nextStep }}<ChevronRight :size="14" class="icon-right" />
          </UiButton>
          <UiButton v-else size="sm" variant="primary" :loading="isSaving" :disabled="!isStepValid" @click="saveConfig">
            <Save :size="14" />{{ t.sh_saveConfig }}
          </UiButton>
        </div>
      </div>
    </template>
  </UiDialog>
</template>

<style scoped>
/* ============================================================
   枚举控件宽度稳定化：所有 UiSelect / UiInputNumber 撑满父容器
   ============================================================ */
.field :deep(.n-select),
.batch-cell :deep(.n-select),
.ntable td :deep(.n-select),
.sphere-grid :deep(.n-select) {
  width: 100% !important;
  min-width: 0;
}

.field :deep(.n-input-number),
.ntable td :deep(.n-input-number) {
  width: 100% !important;
  min-width: 0;
}

/* 紧凑布局主体：左右分栏，使用固定 height 确保步骤切换时画面尺寸稳定 */
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

/* 紧凑化 UiPanel 内边距 */
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

.sidebar-region-stats {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--space-2);
}

.region-stat {
  padding: 6px 8px;
  border-radius: var(--radius-md);
  border: 1px solid var(--border-default);
  background: var(--bg-panel);
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
}

.region-stat--inner {
  border-color: var(--accent-primary);
  background: color-mix(in srgb, var(--accent-primary) 6%, transparent);
}

.region-stat--outer {
  border-color: var(--axis-y, #d97706);
  background: color-mix(in srgb, var(--axis-y, #d97706) 6%, transparent);
}

.region-stat--inner .region-value {
  color: var(--accent-primary);
  font-weight: 700;
  font-variant-numeric: tabular-nums;
}

.region-stat--outer .region-value {
  color: var(--axis-y, #d97706);
  font-weight: 700;
  font-variant-numeric: tabular-nums;
}

.region-label {
  font-size: var(--text-xs);
  color: var(--text-tertiary);
}

.region-value {
  font-size: 14px;
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

.section-header {
  display: flex;
  align-items: center;
  flex-wrap: nowrap;
  gap: var(--space-1-5);
  font-size: var(--text-sm);
  font-weight: 600;
  color: var(--text-primary);
  white-space: nowrap;
}

.section-count {
  margin-left: auto;
  font-size: var(--text-xs);
  font-weight: 400;
  color: var(--text-muted);
}

.field {
  display: flex;
  flex-direction: column;
  gap: var(--space-0-5);
  min-width: 0;
}

.field--fixed {
  flex-shrink: 0;
}

.field-label {
  font-size: var(--text-xs);
  color: var(--text-tertiary);
}

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

.angle-block--disabled {
  opacity: 0.5;
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

.theta-tag {
  background: var(--axis-z-soft, color-mix(in srgb, var(--accent-primary) 12%, transparent));
  color: var(--axis-z, var(--accent-primary));
}

.phi-tag {
  background: var(--axis-u-soft, color-mix(in srgb, var(--axis-y, #d97706) 12%, transparent));
  color: var(--axis-u, var(--axis-y, #d97706));
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

.mode-grid {
  display: flex;
  flex-direction: column;
  gap: var(--space-1-5);
}

.mode-desc {
  margin: 0;
  font-size: var(--text-xs);
  color: var(--text-muted);
  line-height: 1.5;
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

.preview-spin {
  margin-left: auto;
  transform: scale(0.7);
}

.preview-canvas {
  display: flex;
  justify-content: center;
  align-items: center;
  min-height: 140px;
}

.preview-svg {
  width: 100%;
  max-width: 200px;
  height: auto;
  aspect-ratio: 1 / 1;
}

.preview-error {
  display: flex;
  align-items: center;
  justify-content: center;
  text-align: center;
  padding: var(--space-3);
  font-size: var(--text-xs);
  color: var(--accent-danger, var(--danger, #dc2626));
  background: color-mix(in srgb, var(--accent-danger, var(--danger, #dc2626)) 6%, transparent);
  border-radius: var(--radius-md);
  min-height: 100px;
}

.preview-error p {
  margin: 0;
  line-height: 1.5;
}

.preview-dot-inner {
  fill: var(--accent-primary);
}

.preview-dot-outer {
  fill: var(--axis-y, #d97706);
  opacity: 0.7;
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

.basic-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 160px;
  gap: var(--space-2);
  align-items: end;
}

.param-grid-2 {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--space-1-5);
}

.param-grid-4 {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: var(--space-1-5);
}

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

.cell-device,
.cell-channel {
  overflow: visible;
}

.col-controller { width: 40%; }
.col-axis { width: 25%; }

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

.header-toggle {
  margin-left: auto;
  font-size: var(--text-xs);
}

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

.error-list {
  margin: var(--space-1) 0 0;
  padding-left: var(--space-4);
  font-size: var(--text-sm);
  line-height: 1.5;
}

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

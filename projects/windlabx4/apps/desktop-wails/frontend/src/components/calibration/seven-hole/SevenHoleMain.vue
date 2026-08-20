<script setup lang="ts">
/**
 * 七孔探针校准主画面（spec Task 21）
 *
 * 设计参考 FiveHoleMain.vue 的顶部状态栏 + 左侧栏 + 主区域范式，
 * 在其基础上增加七孔独有特性：
 *   - 顶部状态栏增加"当前区域徽章"（内区 / 外区 N 区 + 边界点标记，spec §12 第 2 条）
 *   - 左侧栏 P1~P7 七通道大字号显示（P7 中心孔特殊色突出，是分区判定关键）
 *   - 主区域预留 3 类图表占位（Task 22 实现，本任务先留占位）
 *   - 控制按钮集中在 Header 右上角（开始/暂停/恢复/停止/配置/保存）
 *
 * 严格遵循 spec Task 21 验收要求：
 *   - cleanupSubscriptions() 在 onBeforeUnmount 调用
 *   - 禁止 calibrationStore.reset()（spec Decision #3 / I1：保留会话状态供切回 / 导出）
 *   - 颜色全部用设计 token（--accent-* / --text-* / --bg-* / --border-* / --shadow-*）
 *   - 所有 label 用 i18nStore 翻译（shm_ 前缀）
 *   - 角度定义明确标注：UI 区分"内区 α/β"和"外区 θ/φ"（spec §12 第 2 条）
 *   - MotionSafetyAlertCard 独立卡片承载 6 字段结构化故障现场
 */
import { ref, computed, watch, onBeforeUnmount } from 'vue'
import { storeToRefs } from 'pinia'
import { useCalibrationWorkflow } from '@composables/useCalibrationWorkflow'
import { deviceApi } from '@api/deviceApi'
import { useI18nStore } from '@stores/i18nStore'
import type { CalibrationConfig, ProbeChannelRole } from '@shared/types/calibration'
import type { DataPayload } from '@api/types'
import { getProbeChannelPrecision } from '@shared/calibrationPrecision'
import { findChannelValue } from '@shared/calibrationSnapshotValue'
import {
  ArrowLeft,
  Settings,
  Play,
  Pause,
  Square,
  Save,
  ChevronDown,
  ChevronUp,
  Gauge,
  Target,
  Wind,
} from '@lucide/vue'
import UiButton from '@components/ui/UiButton.vue'
import MotionSafetyAlertCard from '@components/shared/MotionSafetyAlertCard.vue'
import SevenHoleCharts from '@components/calibration/seven-hole/SevenHoleCharts.vue'

const emit = defineEmits<{
  openSettings: []
  back: []
}>()

// 复用 useCalibrationWorkflow 暴露的派生状态：与五孔保持一致，
// 七孔特化部分（startCalibration、风洞通道校验、P1~P7 实时压力映射）保留在本组件内。
const {
  calibrationStore,
  deviceStore,
  motionStore,
  feedbackStore,
  isLoading,
  hasConfig,
  currentConfig,
  loadSavedConfig,
  pauseCalibration,
  resumeCalibration,
  stopCalibration,
  saveCsv,
  progressInfo,
  formattedTimeInfo,
  isAcquisitionDeviceConnected,
  isMotionControllerConnected,
  sphereTankGate,
} = useCalibrationWorkflow('seven-hole')

const { t } = storeToRefs(useI18nStore())

// 暴露 reloadSavedConfig 给父组件 CalibrationWindow（与 FiveHoleMain 保持一致）：
// Settings 保存配置后父组件调 currentMainRef.reloadSavedConfig() 触发重新加载，
// 否则 currentConfig 仍是挂载时的旧值，canStartCalibration 不刷新，会一直提示"未配置"。
defineExpose({
  reloadSavedConfig: loadSavedConfig,
})

// 配置摘要折叠状态：校准中几乎不看，压缩到角落
const showConfigSummary = ref(false)
// 次要通道（PAtm/TAtm/PTotal/PStatic）折叠状态：默认展开，校准中可折叠让核心通道更突出
const showSecondaryChannels = ref(true)

// 七孔图表区域切换：内区 + 外区 1~6 区（共 7 个区域）
// 7 个区域对应 1 个内区（P7 最大）+ 6 个外区扇区（P1~P6 最大）。
// Tab 栏放在内容区顶部，替代原"角度提示+图表标题"行——区域切换是七孔校准主画面的核心导航，
// 放在内容区顶部更符合操作员视觉动线（顶部状态栏看实时状态 → 内容区顶部选区域 → 看该区域图表）。
type SevenHoleChartTab = 'inner' | 'outer-1' | 'outer-2' | 'outer-3' | 'outer-4' | 'outer-5' | 'outer-6'
const activeChartTab = ref<SevenHoleChartTab>('inner')

const chartTabs = computed<Array<{ key: SevenHoleChartTab; label: string }>>(() => [
  { key: 'inner', label: t.value.shm_innerRegion },
  { key: 'outer-1', label: t.value.shm_sectorN.replace('{n}', '1') },
  { key: 'outer-2', label: t.value.shm_sectorN.replace('{n}', '2') },
  { key: 'outer-3', label: t.value.shm_sectorN.replace('{n}', '3') },
  { key: 'outer-4', label: t.value.shm_sectorN.replace('{n}', '4') },
  { key: 'outer-5', label: t.value.shm_sectorN.replace('{n}', '5') },
  { key: 'outer-6', label: t.value.shm_sectorN.replace('{n}', '6') },
])

// 实时采集快照订阅：deviceApi.onSnapshot 推送设备通道原始数据，
// 由 buildRealtimePressuresFromSnapshots 映射到 RealtimePressures 喂给 store，
// 驱动"实时通道数据"面板更新。与 FiveHoleMain.vue 的模式一致。
let unsubscribeDaqSnapshot: (() => void) | null = null
let unsubscribeDeviceStatus: (() => void) | null = null
let unsubscribeMotionStatus: (() => void) | null = null
// 运动控制器状态轮询定时器：后端 Wails 事件推送频率不足以支撑"当前位置"实时显示，
// 需 300ms 主动拉取 status，驱动 actualAngles 更新。
let motionStatusPollTimer: ReturnType<typeof setInterval> | null = null

const latestSnapshots = ref<Map<string, DataPayload>>(new Map())

function cleanupSubscriptions(): void {
  unsubscribeDaqSnapshot?.()
  unsubscribeDaqSnapshot = null
  unsubscribeDeviceStatus?.()
  unsubscribeDeviceStatus = null
  unsubscribeMotionStatus?.()
  unsubscribeMotionStatus = null
  if (motionStatusPollTimer) {
    clearInterval(motionStatusPollTimer)
    motionStatusPollTimer = null
  }
}

// 挂载后订阅采集快照（与 FiveHoleMain 一致）
watch(isLoading, (loading) => {
  if (loading) return
  cleanupSubscriptions()
  unsubscribeDaqSnapshot = deviceApi.onSnapshot((payload: DataPayload) => {
    latestSnapshots.value.set(payload.deviceId, payload)
    if (!currentConfig.value || currentConfig.value.type !== 'seven-hole') return
    const snapshots = Array.from(latestSnapshots.value.values())
    const pressures = buildRealtimePressuresFromSnapshots(currentConfig.value, snapshots)
    if (pressures) {
      calibrationStore.updateRealtimePressures(pressures)
    }
  })
  unsubscribeDeviceStatus = deviceStore.attachStatusListener()
  unsubscribeMotionStatus = motionStore.attachStatusListener()
  // 首次拉取一次状态，避免订阅事件到达前 UI 空白
  void motionStore.refreshStatus()
  // 300ms 轮询：驱动"实际 α/β"实时显示运动控制器轴位置
  motionStatusPollTimer = setInterval(() => {
    void motionStore.refreshStatus()
  }, 300)
})

// 七孔特化校验：风洞总压/静压/温度三个通道必须配置齐全，否则马赫数/速度无法计算。
// 与五孔一致；七孔配置中无独立风洞温度通道角色（仅 tAtm），暂用 tAtm 兜底。
function hasConfiguredProbeChannel(roles: ProbeChannelRole[], namePatterns: RegExp[]): boolean {
  if (!currentConfig.value || !currentConfig.value.probeChannels) {
    return false
  }
  return currentConfig.value.probeChannels.some((channel) => {
    if (!channel.enabled || !channel.channel.deviceId || channel.channel.channelIndex < 0) {
      return false
    }
    if (channel.role && roles.includes(channel.role)) {
      return true
    }
    const normalizedName = channel.name.trim().toLowerCase()
    return namePatterns.some((pattern) => pattern.test(normalizedName))
  })
}

const hasWindTunnelTotalPressureChannel = computed(() => hasConfiguredProbeChannel(
  ['sevenHole.pTotal'],
  [/总压/, /total pressure/, /(?:^|[^a-z0-9])p0(?:[^a-z0-9]|$)/, /(?:^|[^a-z0-9])pt(?:[^a-z0-9]|$)/],
))

const hasWindTunnelStaticPressureChannel = computed(() => hasConfiguredProbeChannel(
  ['sevenHole.pTunnelStatic'],
  [/静压/, /static pressure/, /(?:^|[^a-z0-9])ps(?:[^a-z0-9]|$)/],
))

const hasWindTunnelTemperatureChannel = computed(() => hasConfiguredProbeChannel(
  // 七孔配置中无独立风洞温度通道角色（仅 tAtm），暂用 tAtm 兜底
  ['sevenHole.tAtm'],
  [/风洞.*温/, /tunnel temp/, /tunnel temperature/, /(?:^|[^a-z0-9])ttunnel(?:[^a-z0-9]|$)/, /大气.*温/, /(?:^|[^a-z0-9])tatm(?:[^a-z0-9]|$)/],
))

const hasRequiredWindTunnelChannels = computed(() => {
  return hasWindTunnelTotalPressureChannel.value
    && hasWindTunnelStaticPressureChannel.value
    && hasWindTunnelTemperatureChannel.value
})

const canStartCalibration = computed(() => {
  return hasConfig.value
    && !isLoading.value
    && hasRequiredWindTunnelChannels.value
    && isAcquisitionDeviceConnected.value
    && isMotionControllerConnected.value
})

const startDisabledReason = computed(() => {
  if (isLoading.value) return t.value.shm_loadingConfig
  if (!hasConfig.value) return t.value.shm_noConfig
  if (!hasRequiredWindTunnelChannels.value) return t.value.shm_noWindTunnelChannels
  if (!isAcquisitionDeviceConnected.value) return t.value.shm_deviceNotReady
  if (!isMotionControllerConnected.value) return t.value.shm_motionNotConnected
  return ''
})

// 开始校准（七孔特化：直接使用保存时已生成的 points，避免 layout 与 points 双真值源不一致）
async function startCalibration() {
  if (!canStartCalibration.value || !currentConfig.value) {
    feedbackStore.pushToast(startDisabledReason.value || t.value.shm_preCheckNotDone, 'warning')
    return
  }
  try {
    const configToStart: CalibrationConfig = {
      ...currentConfig.value,
      points: currentConfig.value.points,
    }
    await calibrationStore.startCalibration(configToStart)
  } catch (err) {
    console.error('Failed to start calibration:', err)
    feedbackStore.pushToast(t.value.shm_startFailed + ': ' + (err instanceof Error ? err.message : String(err)), 'error')
  }
}

// 实际 α/β：从位移机构当前实际位置解析（与 FiveHoleMain 一致）
// 七孔探针校准的 α/β 本质是旋转轴角度，motionStore.statusList 每 300ms 刷新一次。
// 识别规则：优先按 motionAxes[].name 匹配（兼容 α/alpha/攻角 与 β/beta/侧滑），
// 命名无法识别时回退按索引——motionAxes[0] 视为 α，motionAxes[1] 视为 β
const actualAngles = computed<{ alpha?: number; beta?: number }>(() => {
  const axes = currentConfig.value?.motionAxes
  if (!axes?.length) return {}
  const statusList = motionStore.statusList
  const result: { alpha?: number; beta?: number } = {}
  axes.forEach((axis, index) => {
    if (!axis.controllerId) return
    const status = statusList.find((s) => s.id === axis.controllerId)
    if (!status) return
    const axisStatus = status.axes.find((a) => a.name === axis.axis)
    if (!axisStatus) return
    const pos = axisStatus.position
    const name = axis.name.toLowerCase()
    if (/α|alpha|攻角/.test(name)) {
      result.alpha = pos
    } else if (/β|beta|侧滑/.test(name)) {
      result.beta = pos
    } else if (index === 0) {
      result.alpha = pos
    } else if (index === 1) {
      result.beta = pos
    }
  })
  return result
})

const isMoving = computed(() => {
  // 任一轴 moving=true 时视为运动中，用于顶部状态栏"运动中"标识
  const axes = currentConfig.value?.motionAxes ?? []
  const statusList = motionStore.statusList
  return axes.some((axis) => {
    if (!axis.controllerId) return false
    const status = statusList.find((s) => s.id === axis.controllerId)
    if (!status) return false
    const axisStatus = status.axes.find((a) => a.name === axis.axis)
    return axisStatus?.moving ?? false
  })
})

// 目标 α/β：从 progressInfo.currentPoint.coordinates 取
// 七孔点的 coordinates 在内区时为 α/β，外区时为 θ/φ（spec §3.4 双坐标系）。
// 外区时 α/β 不可用（显示 "--"），由区域徽章 + 角度标注（shm_angleNote）澄清。
// progressInfo 优先用 currentPointIndex（循环顶部推进，早于 moveToPoint）作索引从 config.points 查出当前点，
// 让目标角度先于实际角度变化。
const targetAngles = computed<{ alpha: number | null; beta: number | null }>(() => {
  const coords = progressInfo.value?.currentPoint
  const alpha = coords?.['α']
  const beta = coords?.['β']
  return {
    alpha: typeof alpha === 'number' ? alpha : null,
    beta: typeof beta === 'number' ? beta : null,
  }
})

// ===== 顶部状态栏派生状态（参考 FiveHoleMain.vue）=====

const completedPoints = computed(() => calibrationStore.dataPoints.length)
const totalPoints = computed(() => currentConfig.value?.points.length ?? 0)
const progressPercent = computed(() => {
  if (!totalPoints.value) return 0
  return Math.round((completedPoints.value / totalPoints.value) * 100)
})
const formattedProgress = computed(() => {
  return `${completedPoints.value} / ${totalPoints.value} (${progressPercent.value}%)`
})

const statusText = computed(() => {
  // spec Decision #15 / I7：按 status.status 精确映射，避免 stop 后退化为「空闲」
  const s = calibrationStore.status?.status
  if (s === 'running') return t.value.shm_statusRunning
  if (s === 'paused') return t.value.shm_statusPaused
  if (s === 'stopped') return t.value.shm_statusStopped
  if (s === 'completed') return t.value.shm_statusCompleted
  if (s === 'error') return t.value.shm_statusError
  return t.value.shm_statusIdle
})

// 状态色 CSS 变量标识：将 statusColor 转为设计 token，替代 Tailwind 调色板硬编码
const statusColorToken = computed(() => {
  const s = calibrationStore.status?.status
  if (s === 'running') return '--accent-success'
  if (s === 'paused') return '--accent-warning'
  // 已停止：黄色警示色，与 idle（normal）区分，提示「保留数据可导出」
  if (s === 'stopped') return '--accent-warning'
  if (s === 'completed') return '--accent-info'
  if (s === 'error') return '--accent-danger'
  return '--text-muted'
})

const canPause = computed(() => calibrationStore.isRunning && !calibrationStore.isPaused)
const canResume = computed(() => calibrationStore.isPaused)
const canStop = computed(() => calibrationStore.isRunning || calibrationStore.isPaused)
const canSave = computed(() => calibrationStore.completeEvent !== null || calibrationStore.dataPoints.length > 0)

// 实时气动参数（马赫数/速度）：calibrationStore.calculatedPhysics 基于实时压力计算
const physics = computed(() => calibrationStore.calculatedPhysics)

// 当前点采样子进度：calibrationStore.status 透传后端 autoEngine.GetSampleProgress()
const sampleProgress = computed(() => {
  const s = calibrationStore.status
  if (!s) return null
  const current = s.currentSample ?? 0
  const total = s.samplesPerPoint ?? 0
  if (total <= 0 || current <= 0) return null
  return { current, total, percent: Math.round((current / total) * 100) }
})

// 错误详情：后端 StateError 时 lastError 非空，顶部栏展示供操作员排查
const lastError = computed(() => calibrationStore.status?.lastError ?? '')

// 运动安全故障现场快照：从 calibrationStore.status.motionSafetyFailure 取，
// 后端在故障发生时写入、恢复时清空。告警卡片据此渲染/隐藏。
const motionSafetyFailure = computed(() => calibrationStore.status?.motionSafetyFailure ?? null)

// ===== 七孔独有：当前区域徽章 =====
// currentRegion: 'inner' / 'outer' / ''（首点之前）
// currentSector: 内区固定 7，外区 1..6；首点之前为 0
// boundaryFlag: 空串=非边界，"P7-Pn"或"Pn-Pm"=并列边界
const currentRegion = computed(() => calibrationStore.currentRegion)
const currentSector = computed(() => calibrationStore.currentSector)
const boundaryFlag = computed(() => calibrationStore.boundaryFlag)

// 区域徽章显示文本：
//   - inner → "内区"
//   - outer → "外区 N 区"（N=currentSector，未初始化时显示 "--"）
//   - 首点之前（currentRegion === ''）→ "--"
const regionBadgeText = computed(() => {
  const r = currentRegion.value
  if (r === 'inner') return t.value.shm_innerRegion
  if (r === 'outer') {
    const sector = currentSector.value
    const sectorText = sector > 0 ? String(sector) : '--'
    return t.value.shm_sectorN.replace('{n}', sectorText)
  }
  return t.value.shm_noRegion
})

// 区域徽章颜色 token：inner=蓝(--accent-info) / outer=橙(--accent-warning) / 未初始化=灰
const regionBadgeToken = computed(() => {
  const r = currentRegion.value
  if (r === 'inner') return '--accent-info'
  if (r === 'outer') return '--accent-warning'
  return '--text-muted'
})

const hasBoundary = computed(() => boundaryFlag.value !== '')

// 配置摘要：七孔特有 sevenHoleLayout（α/β/θ/φ 范围与步长）
const sevenHoleLayout = computed(() => currentConfig.value?.sevenHoleLayout)

// 通道分组：核心通道 P1-P7 / 次要通道 Patm/Tatm/Pt/Ps/Tt
const probeChannels = computed(() => currentConfig.value?.probeChannels ?? [])

const coreRoles = [
  'sevenHole.p1', 'sevenHole.p2', 'sevenHole.p3', 'sevenHole.p4',
  'sevenHole.p5', 'sevenHole.p6', 'sevenHole.p7',
]

const coreChannels = computed(() => {
  return probeChannels.value.filter((c) => coreRoles.includes(c.role || ''))
})

const secondaryChannels = computed(() => {
  return probeChannels.value.filter((c) => !coreRoles.includes(c.role || ''))
})

function formatSevenHoleRealtimeValue(
  role: ProbeChannelRole,
  value: number | undefined,
  unit = '',
): string {
  if (typeof value !== 'number' || !Number.isFinite(value)) {
    return '--'
  }
  const precision = getProbeChannelPrecision(currentConfig.value, role)
  const formatted = value.toFixed(precision)
  return unit ? `${formatted} ${unit}` : formatted
}

function getChannelValue(role: string): string {
  const pressures = calibrationStore.realtimePressures
  if (!pressures) return '--'
  switch (role) {
    case 'sevenHole.p1': return formatSevenHoleRealtimeValue('sevenHole.p1', pressures.P1)
    case 'sevenHole.p2': return formatSevenHoleRealtimeValue('sevenHole.p2', pressures.P2)
    case 'sevenHole.p3': return formatSevenHoleRealtimeValue('sevenHole.p3', pressures.P3)
    case 'sevenHole.p4': return formatSevenHoleRealtimeValue('sevenHole.p4', pressures.P4)
    case 'sevenHole.p5': return formatSevenHoleRealtimeValue('sevenHole.p5', pressures.P5)
    case 'sevenHole.p6': return formatSevenHoleRealtimeValue('sevenHole.p6', pressures.P6)
    case 'sevenHole.p7': return formatSevenHoleRealtimeValue('sevenHole.p7', pressures.P7)
    case 'sevenHole.pAtm': return formatSevenHoleRealtimeValue('sevenHole.pAtm', pressures.Patm)
    case 'sevenHole.tAtm': return formatSevenHoleRealtimeValue('sevenHole.tAtm', pressures.Tatm)
    case 'sevenHole.pTotal': return formatSevenHoleRealtimeValue('sevenHole.pTotal', pressures.P0)
    case 'sevenHole.pTunnelStatic': return formatSevenHoleRealtimeValue('sevenHole.pTunnelStatic', pressures.Ps)
    default: return '--'
  }
}

function getChannelUnit(role: string): string {
  const ch = probeChannels.value.find((c: { role?: string }) => c.role === role)
  if (!ch) {
    // 默认单位兜底：温度类角色给 °C，其他给 Pa
    if (role === 'sevenHole.tAtm') return '°C'
    return 'Pa'
  }
  const device = deviceStore.profiles?.find((d) => d.id === ch.channel.deviceId)
  const channelConfig = device?.channels[ch.channel.channelIndex]
  return channelConfig?.unit ?? 'Pa'
}

// P7 中心孔特殊样式判定：P7 是七孔探针分区判定的关键（spec §4.1），
// UI 用 --accent-info（蓝色）突出显示，与外圈 P1~P6（--accent-primary 蓝色）区分
function isP7Center(role: string | undefined): boolean {
  return role === 'sevenHole.p7'
}

// 从采集快照构建 RealtimePressures（七孔版本，含 P1~P7）
// 与 FiveHoleMain.vue 的 buildRealtimePressuresFromSnapshots 同构，扩展 P6/P7 通道
function buildRealtimePressuresFromSnapshots(
  config: CalibrationConfig,
  snapshots: DataPayload[],
): import('@stores/calibrationStore').RealtimePressures | null {
  const result = {
    P1: 0,
    P2: 0,
    P3: 0,
    P4: 0,
    P5: 0,
    P6: 0,
    P7: 0,
    Patm: 0,
    Tatm: 0,
    P0: undefined as number | undefined,
    Ps: undefined as number | undefined,
    Ttunnel: undefined as number | undefined,
  }
  let matchedChannelCount = 0

  const assignByRole = (role: ProbeChannelRole | undefined, value: number): void => {
    switch (role) {
      case 'sevenHole.p1': result.P1 = value; matchedChannelCount += 1; break
      case 'sevenHole.p2': result.P2 = value; matchedChannelCount += 1; break
      case 'sevenHole.p3': result.P3 = value; matchedChannelCount += 1; break
      case 'sevenHole.p4': result.P4 = value; matchedChannelCount += 1; break
      case 'sevenHole.p5': result.P5 = value; matchedChannelCount += 1; break
      case 'sevenHole.p6': result.P6 = value; matchedChannelCount += 1; break
      case 'sevenHole.p7': result.P7 = value; matchedChannelCount += 1; break
      case 'sevenHole.pAtm': result.Patm = value; matchedChannelCount += 1; break
      // 七孔校准无独立风洞温度通道角色（七孔配置仅 tAtm，无 tTunnel），
      // 用大气温度近似风洞总温（TAT），低速风洞下误差可接受。
      // 与 ThreeHoleMain 保持一致：tAtm 同时填充 Tatm 和 Ttunnel。
      case 'sevenHole.tAtm': result.Tatm = value; result.Ttunnel = value; matchedChannelCount += 1; break
      case 'sevenHole.pTotal': result.P0 = value; matchedChannelCount += 1; break
      case 'sevenHole.pTunnelStatic': result.Ps = value; matchedChannelCount += 1; break
      default: break
    }
  }

  for (const ch of config.probeChannels) {
    if (!ch.enabled) continue
    const rawValue = findChannelValue(snapshots, ch.channel.deviceId, ch.channel.channelIndex)
    if (rawValue === null) continue
    if (ch.role) {
      assignByRole(ch.role as ProbeChannelRole, rawValue)
      continue
    }
    // 名称兜底匹配（与 FiveHoleMain 一致的模式，扩展到 P1~P7）
    const name = ch.name.trim()
    const normalizedName = name.toLowerCase()
    if (/(?:^|[^a-z0-9])p1(?:[^a-z0-9]|$)/.test(normalizedName) || /孔\s*1/.test(name)) {
      result.P1 = rawValue; matchedChannelCount += 1
    } else if (/(?:^|[^a-z0-9])p2(?:[^a-z0-9]|$)/.test(normalizedName) || /孔\s*2/.test(name)) {
      result.P2 = rawValue; matchedChannelCount += 1
    } else if (/(?:^|[^a-z0-9])p3(?:[^a-z0-9]|$)/.test(normalizedName) || /孔\s*3/.test(name)) {
      result.P3 = rawValue; matchedChannelCount += 1
    } else if (/(?:^|[^a-z0-9])p4(?:[^a-z0-9]|$)/.test(normalizedName) || /孔\s*4/.test(name)) {
      result.P4 = rawValue; matchedChannelCount += 1
    } else if (/(?:^|[^a-z0-9])p5(?:[^a-z0-9]|$)/.test(normalizedName) || /孔\s*5/.test(name)) {
      result.P5 = rawValue; matchedChannelCount += 1
    } else if (/(?:^|[^a-z0-9])p6(?:[^a-z0-9]|$)/.test(normalizedName) || /孔\s*6/.test(name)) {
      result.P6 = rawValue; matchedChannelCount += 1
    } else if (/(?:^|[^a-z0-9])p7(?:[^a-z0-9]|$)/.test(normalizedName) || /孔\s*7/.test(name)) {
      result.P7 = rawValue; matchedChannelCount += 1
    } else if (/大气.*压/.test(name) || normalizedName.includes('atmospheric pressure') || /(?:^|[^a-z0-9])patm(?:[^a-z0-9]|$)/.test(normalizedName)) {
      result.Patm = rawValue; matchedChannelCount += 1
    } else if (/大气.*温/.test(name) || normalizedName.includes('atmospheric temp') || normalizedName.includes('atmospheric temperature') || /(?:^|[^a-z0-9])tatm(?:[^a-z0-9]|$)/.test(normalizedName)) {
      // 七孔无独立 tTunnel 角色，大气温度兜底填充 Ttunnel，与 role 分支保持一致
      result.Tatm = rawValue
      result.Ttunnel = rawValue
      matchedChannelCount += 1
    } else if (/总压/.test(name) || normalizedName.includes('total pressure') || /(?:^|[^a-z0-9])p0(?:[^a-z0-9]|$)/.test(normalizedName) || /(?:^|[^a-z0-9])pt(?:[^a-z0-9]|$)/.test(normalizedName)) {
      result.P0 = rawValue; matchedChannelCount += 1
    } else if (/静压/.test(name) || normalizedName.includes('static pressure') || /(?:^|[^a-z0-9])ps(?:[^a-z0-9]|$)/.test(normalizedName)) {
      result.Ps = rawValue; matchedChannelCount += 1
    } else if (/风洞.*温/.test(name) || normalizedName.includes('tunnel temp') || normalizedName.includes('tunnel temperature') || /(?:^|[^a-z0-9])ttunnel(?:[^a-z0-9]|$)/.test(normalizedName)) {
      result.Ttunnel = rawValue; matchedChannelCount += 1
    }
  }

  if (matchedChannelCount === 0) {
    return null
  }

  return result
}

onBeforeUnmount(() => {
  cleanupSubscriptions()
  // spec Decision #3 / I1：unmount 不调 calibrationStore.reset()，保留会话状态供切回 / 导出。
  // releaseView（引用计数-1 + 降频 polling）已由 useCalibrationWorkflow.onBeforeUnmount 统一处理。
})
</script>

<template>
  <div data-test="seven-hole-main-shell" class="flex h-full flex-col bg-[var(--bg-canvas)] text-[var(--text-primary)]">
    <!-- Header -->
    <div class="flex items-center justify-between border-b border-[var(--border-default)] bg-[var(--bg-panel)] px-5 py-2.5">
      <div class="flex items-center gap-3">
        <UiButton variant="secondary" size="sm" @click="emit('back')">
          <ArrowLeft class="h-4 w-4" />
        </UiButton>
        <div>
          <h1 class="text-base font-bold text-[var(--text-primary)]">{{ t.shm_sevenHoleCalibration }}</h1>
        </div>
      </div>
      <div class="flex items-center gap-2">
        <!-- 控制按钮：开始/暂停/恢复/停止 全部集中在 Header 右上角，与"配置/保存"并列 -->
        <UiButton v-if="!calibrationStore.isRunning && !calibrationStore.isPaused" variant="primary" size="sm" :disabled="!canStartCalibration" :title="startDisabledReason || undefined" @click="startCalibration">
          <Play class="h-4 w-4" />
          <span class="ml-1">{{ t.shm_start }}</span>
        </UiButton>
        <UiButton v-if="canPause" variant="warning" size="sm" @click="pauseCalibration">
          <Pause class="h-4 w-4" />
          <span class="ml-1">{{ t.shm_pause }}</span>
        </UiButton>
        <UiButton v-if="canResume" variant="primary" size="sm" @click="resumeCalibration">
          <Play class="h-4 w-4" />
          <span class="ml-1">{{ t.shm_resume }}</span>
        </UiButton>
        <UiButton v-if="canStop" variant="danger" size="sm" @click="stopCalibration">
          <Square class="h-4 w-4" />
          <span class="ml-1">{{ t.shm_stop }}</span>
        </UiButton>
        <UiButton variant="secondary" size="sm" @click="emit('openSettings')">
          <Settings class="h-4 w-4" />
          <span class="ml-1">{{ t.configBtn }}</span>
        </UiButton>
        <UiButton variant="secondary" size="sm" :disabled="!canSave" @click="saveCsv">
          <Save class="h-4 w-4" />
          <span class="ml-1">{{ t.save }}</span>
        </UiButton>
      </div>
    </div>

    <!-- 加载状态 -->
    <div v-if="isLoading" class="flex flex-1 items-center justify-center">
      <div class="text-center">
        <div class="mx-auto mb-4 h-8 w-8 animate-spin rounded-full border-2 border-[var(--accent-primary)] border-t-transparent"></div>
        <p class="text-[var(--text-muted)]">{{ t.shm_loading }}</p>
      </div>
    </div>

    <template v-else>
      <!-- 顶部状态栏：跨全宽 sticky 定位，校准员最频繁看的信息
           包含：状态徽章 / 进度条 / 时间 / 区域徽章(七孔独有) / 目标α-β+实际α-β / 采样子进度 / 错误 / 配置按钮
           Ma/V 已移至左侧栏大字显示，避免顶部状态栏信息冗余与宽度抖动
           sticky 定位确保运动控制器运行时内容区滚动不会导致状态栏位置跳动 -->
      <div class="sticky top-0 z-10 flex flex-wrap items-center gap-3 border-b border-[var(--border-default)] bg-[var(--bg-panel)] px-5 py-2.5">
        <!-- 状态徽章 -->
        <span
          class="rounded-full px-2 py-0.5 text-xs font-medium"
          :style="{
            backgroundColor: `color-mix(in srgb, var(${statusColorToken}) 15%, transparent)`,
            color: `var(${statusColorToken})`,
          }"
        >{{ statusText }}</span>

        <!-- 进度条 -->
        <div class="flex min-w-[180px] max-w-[280px] flex-1 items-center gap-2">
          <span class="whitespace-nowrap text-xs text-[var(--text-muted)]">{{ t.travProgress }}</span>
          <div class="h-2 flex-1 overflow-hidden rounded-full bg-[var(--bg-panel-strong)]">
            <div class="h-full rounded-full bg-[var(--accent-primary)] transition-all duration-300" :style="{ width: progressPercent + '%' }"></div>
          </div>
          <span class="whitespace-nowrap font-mono text-xs font-bold text-[var(--text-primary)]">{{ formattedProgress }}</span>
        </div>

        <!-- 时间信息 -->
        <div v-if="formattedTimeInfo" class="flex items-center gap-3 text-xs">
          <div class="flex items-center gap-1">
            <span class="text-[var(--text-muted)]">{{ t.travElapsed }}</span>
            <span class="font-mono font-bold text-[var(--text-primary)]">{{ formattedTimeInfo.elapsed }}</span>
          </div>
          <div class="flex items-center gap-1">
            <span class="text-[var(--text-muted)]">{{ t.travRemaining }}</span>
            <span class="font-mono font-bold text-[var(--text-primary)]">{{ formattedTimeInfo.remaining }}</span>
          </div>
        </div>

        <!-- 当前区域徽章（七孔独有）：内区=蓝 / 外区 N 区=橙 + 边界点小标记（黄）
             spec §12 第 2 条：UI 区分内区 α/β 与外区 θ/φ，区域徽章是核心视觉锚点 -->
        <div class="flex items-center gap-1.5 border-l border-[var(--border-default)] pl-3">
          <span class="whitespace-nowrap text-xs text-[var(--text-muted)]">{{ t.shm_regionLabel }}</span>
          <span
            class="rounded-full px-2 py-0.5 text-xs font-bold"
            :style="{
              backgroundColor: `color-mix(in srgb, var(${regionBadgeToken}) 15%, transparent)`,
              color: `var(${regionBadgeToken})`,
            }"
            :title="t.shm_angleNote"
          >{{ regionBadgeText }}</span>
          <span
            v-if="hasBoundary"
            class="rounded-full px-1.5 py-0.5 text-[10px] font-bold"
            :style="{
              backgroundColor: `color-mix(in srgb, var(--accent-warning) 20%, transparent)`,
              color: `var(--accent-warning)`,
            }"
            :title="boundaryFlag"
          >{{ t.shm_boundaryPoint }}</span>
        </div>

        <!-- 目标 α/β + 实际 α/β：校准员持续盯的核心信息
             七孔独有说明：内区时目标从 coordinates['α'/'β'] 取，外区时为 θ/φ（显示 "--"），
             由区域徽章 + 角度标注（shm_angleNote）澄清 -->
        <div class="flex items-center gap-4 border-l border-[var(--border-default)] pl-3">
          <div class="flex items-center gap-1.5">
            <Target class="h-4 w-4 text-[var(--text-muted)]" />
            <span class="text-xs text-[var(--text-muted)]">{{ t.shm_targetAngles }}</span>
            <span class="font-mono text-base font-bold text-[var(--text-primary)]">
              α{{ targetAngles.alpha !== null ? targetAngles.alpha.toFixed(1) + '°' : '--' }}
              <span class="mx-1 text-[var(--text-muted)]">/</span>
              β{{ targetAngles.beta !== null ? targetAngles.beta.toFixed(1) + '°' : '--' }}
            </span>
          </div>
          <div class="flex items-center gap-1.5">
            <span class="text-xs text-[var(--text-muted)]">{{ t.shm_actualAngles }}</span>
            <span class="font-mono text-base font-bold" :style="{ color: isMoving ? `var(--accent-success)` : `var(--accent-primary)` }">
              α{{ actualAngles.alpha != null ? actualAngles.alpha.toFixed(2) + '°' : '--' }}
              <span class="mx-1 text-[var(--text-muted)]">/</span>
              β{{ actualAngles.beta != null ? actualAngles.beta.toFixed(2) + '°' : '--' }}
            </span>
            <span v-if="isMoving" class="flex items-center gap-1 text-xs" :style="{ color: `var(--accent-success)` }">
              <span class="h-1.5 w-1.5 animate-pulse rounded-full" :style="{ backgroundColor: `var(--accent-success)` }"></span>
              {{ t.shm_moving }}
            </span>
          </div>
        </div>

        <!-- 当前点采样子进度 -->
        <div v-if="sampleProgress" class="flex items-center gap-2 border-l border-[var(--border-default)] pl-3">
          <span class="text-xs text-[var(--text-muted)]">{{ t.samples }}</span>
          <span class="font-mono text-sm font-bold text-[var(--accent-primary)]">{{ sampleProgress.current }}/{{ sampleProgress.total }}</span>
          <div class="h-1.5 w-16 overflow-hidden rounded-full bg-[var(--bg-panel-strong)]">
            <div class="h-full rounded-full bg-[var(--accent-primary)] transition-all duration-200" :style="{ width: sampleProgress.percent + '%' }"></div>
          </div>
        </div>

        <!-- 错误详情（截断 30 字符） -->
        <div v-if="lastError" class="flex items-center gap-1 border-l border-[var(--border-default)] pl-3" :title="lastError">
          <span class="text-xs font-medium" :style="{ color: `var(--accent-danger)` }">⚠ {{ lastError.length > 30 ? lastError.slice(0, 30) + '...' : lastError }}</span>
        </div>

        <!-- 配置摘要折叠按钮：校准中几乎不看，压缩到角落 -->
        <button class="ml-auto flex items-center gap-1 text-xs text-[var(--text-muted)] hover:text-[var(--text-primary)]" @click="showConfigSummary = !showConfigSummary">
          <Settings class="h-3.5 w-3.5" />
          {{ t.configBtn }}
          <ChevronDown v-if="showConfigSummary" class="h-3 w-3" />
          <ChevronUp v-else class="h-3 w-3" />
        </button>
      </div>

      <!-- 运动安全故障告警卡片：仅在 motionSafetyFailure 存在时渲染。
           独立卡片承载 6 字段结构化信息（控制器/轴/目标/实际/偏差/点号），
           单行状态栏无法承载这些信息。与遍历测试模块共用同一告警卡片组件。 -->
      <MotionSafetyAlertCard
        :failure="motionSafetyFailure"
        :t="(t as unknown as Record<string, string>)"
      />

      <!-- 配置摘要展开面板（默认收起） -->
      <div v-if="showConfigSummary" class="flex flex-wrap items-center gap-6 border-b border-[var(--border-default)] bg-[var(--bg-panel-strong)] px-5 py-2 text-xs">
        <div><span class="text-[var(--text-muted)]">{{ t.shm_configName }}：</span><span class="font-medium text-[var(--text-primary)]">{{ currentConfig?.name || t.shm_unconfigured }}</span></div>
        <div v-if="sevenHoleLayout">
          <span class="text-[var(--text-muted)]">{{ t.shm_calibrationMode }}：</span>
          <span class="font-medium text-[var(--text-primary)]">{{ sevenHoleLayout.mode === 'full' ? t.shm_modeFull : t.shm_modeDataset }}</span>
        </div>
        <div v-if="sevenHoleLayout">
          <span class="text-[var(--text-muted)]">{{ t.shm_innerAlphaRange }}：</span>
          <span class="font-medium text-[var(--text-primary)]">{{ sevenHoleLayout.innerAlphaMin }}° ~ {{ sevenHoleLayout.innerAlphaMax }}° ({{ sevenHoleLayout.innerAlphaStep }}°)</span>
        </div>
        <div v-if="sevenHoleLayout">
          <span class="text-[var(--text-muted)]">{{ t.shm_innerBetaRange }}：</span>
          <span class="font-medium text-[var(--text-primary)]">{{ sevenHoleLayout.innerBetaMin }}° ~ {{ sevenHoleLayout.innerBetaMax }}° ({{ sevenHoleLayout.innerBetaStep }}°)</span>
        </div>
        <div v-if="sevenHoleLayout">
          <span class="text-[var(--text-muted)]">{{ t.shm_outerThetaRange }}：</span>
          <span class="font-medium text-[var(--text-primary)]">{{ sevenHoleLayout.outerThetaMin }}° ~ {{ sevenHoleLayout.outerThetaMax }}° ({{ sevenHoleLayout.outerThetaStep }}°)</span>
        </div>
        <div v-if="sevenHoleLayout">
          <span class="text-[var(--text-muted)]">{{ t.shm_outerPhiRange }}：</span>
          <span class="font-medium text-[var(--text-primary)]">{{ sevenHoleLayout.outerPhiMin }}° ~ {{ sevenHoleLayout.outerPhiMax }}° ({{ sevenHoleLayout.outerPhiStep }}°)</span>
        </div>
        <div><span class="text-[var(--text-muted)]">{{ t.shm_totalPoints }}：</span><span class="font-medium text-[var(--text-primary)]">{{ totalPoints }}</span></div>
        <div><span class="text-[var(--text-muted)]">{{ t.shm_dwellTime }}：</span><span class="font-medium text-[var(--text-primary)]">{{ currentConfig?.dwellTimeMs || 0 }}ms</span></div>
        <div><span class="text-[var(--text-muted)]">{{ t.shm_samples }}：</span><span class="font-medium text-[var(--text-primary)]">{{ currentConfig?.samplesPerPoint || 0 }}</span></div>
      </div>

      <div class="flex flex-1 overflow-hidden">
        <!-- 左侧栏：384px 固定宽度，通道/气动参数(可滚动) + 球罐状态条(固定底部)
             控制按钮已移至 Header 右上角，与配置/保存并列 -->
        <div class="flex flex-shrink-0 flex-col overflow-hidden border-r border-[var(--border-default)] bg-[var(--bg-panel)]" style="width: 384px;">
          <!-- 中间内容区（可滚动）：P1~P7 核心通道大字（20px+）+ Ma/V + 次要通道可折叠 -->
          <div class="flex-1 overflow-y-auto">
            <!-- 关键数据：核心压力 P1~P7 + 实时气动参数 Ma/V -->
            <div class="border-b border-[var(--border-default)] p-3">
              <div class="mb-2 flex items-center gap-2 text-sm font-medium text-[var(--text-primary)]">
                <Gauge class="h-4 w-4 text-[var(--accent-primary)]" />
                {{ t.shm_keyData }}
              </div>
              <div class="space-y-1.5">
                <!-- P1~P7 核心压力：大字突出（20px+）；P7 中心孔用 --accent-info 蓝色突出（分区判定关键） -->
                <div
                  v-for="channel in coreChannels"
                  :key="channel.name"
                  class="flex items-baseline justify-between rounded-lg px-3 py-1"
                  :style="{
                    backgroundColor: isP7Center(channel.role)
                      ? `color-mix(in srgb, var(--accent-info) 8%, var(--bg-panel-strong))`
                      : `var(--bg-panel-strong)`,
                    borderLeft: isP7Center(channel.role) ? `3px solid var(--accent-info)` : `3px solid transparent`,
                  }"
                >
                  <span class="text-xs font-medium text-[var(--text-muted)]">
                    {{ channel.name }}
                    <span v-if="isP7Center(channel.role)" class="ml-1 text-[10px]" :style="{ color: `var(--accent-info)` }">●</span>
                  </span>
                  <div class="text-right">
                    <span
                      class="font-mono text-xl font-bold"
                      :style="{ color: isP7Center(channel.role) ? `var(--accent-info)` : `var(--accent-primary)` }"
                    >{{ getChannelValue(channel.role || '') }}</span>
                    <span class="ml-1 text-xs text-[var(--text-muted)]">{{ getChannelUnit(channel.role || '') }}</span>
                  </div>
                </div>
                <!-- 马赫数 Ma：绿色强调，区别于压力通道 -->
                <div class="flex items-baseline justify-between rounded-lg bg-[var(--bg-panel-strong)] px-3 py-2">
                  <span class="text-xs text-[var(--text-muted)]">{{ t.shm_machMa }}</span>
                  <span class="font-mono text-2xl font-bold text-[var(--accent-success)]">{{ physics?.machNumber !== undefined ? physics.machNumber.toFixed(3) : '--' }}</span>
                </div>
                <!-- 速度 V：绿色强调。侧边栏统一保留 3 位小数，与马赫数精度对齐 -->
                <div class="flex items-baseline justify-between rounded-lg bg-[var(--bg-panel-strong)] px-3 py-2">
                  <span class="text-xs text-[var(--text-muted)]">{{ t.shm_velocityV }}</span>
                  <div class="text-right">
                    <span class="font-mono text-2xl font-bold text-[var(--accent-success)]">{{ physics?.velocity !== undefined ? physics.velocity.toFixed(3) : '--' }}</span>
                    <span class="ml-1 text-xs text-[var(--text-muted)]">m/s</span>
                  </div>
                </div>
              </div>
            </div>

            <!-- 其他通道：PAtm/TAtm/PTotal/PStatic/TunnelTemp，可折叠 -->
            <div class="p-3">
              <button
                class="mb-2 flex w-full items-center gap-2 text-sm font-medium text-[var(--text-primary)] hover:text-[var(--accent-primary)]"
                @click="showSecondaryChannels = !showSecondaryChannels"
              >
                <Wind class="h-4 w-4 text-[var(--accent-primary)]" />
                {{ t.shm_otherChannels }}
                <ChevronUp v-if="showSecondaryChannels" class="ml-auto h-3 w-3 text-[var(--text-muted)]" />
                <ChevronDown v-else class="ml-auto h-3 w-3 text-[var(--text-muted)]" />
              </button>
              <div v-if="showSecondaryChannels" class="space-y-1.5">
                <div v-for="channel in secondaryChannels" :key="channel.name" class="flex items-center justify-between rounded bg-[var(--bg-panel-strong)] px-2 py-1.5">
                  <span class="text-xs text-[var(--text-muted)]">{{ channel.name }}</span>
                  <div class="text-right">
                    <span class="font-mono text-sm font-bold text-[var(--text-primary)]">{{ getChannelValue(channel.role || '') }}</span>
                    <span class="ml-1 text-xs text-[var(--text-muted)]">{{ getChannelUnit(channel.role || '') }}</span>
                  </div>
                </div>
                <!-- 兜底：若未配置任何次要通道，提示操作员去配置 -->
                <div v-if="secondaryChannels.length === 0" class="px-2 py-2 text-xs text-[var(--text-muted)]">
                  {{ t.shm_noSecondaryChannels }}
                </div>
              </div>
            </div>
          </div>

          <!-- 球罐门控状态条（固定底部）：一行显示，附"编辑"入口跳配置界面 -->
          <div class="flex-shrink-0 border-t border-[var(--border-default)] p-3">
            <div class="flex items-center gap-2 whitespace-nowrap rounded-lg bg-[var(--bg-panel-strong)] px-3 py-2 text-xs">
              <!-- 左侧：状态块（圆点 + 标题 · 状态词） -->
              <div class="flex items-center gap-2 whitespace-nowrap">
                <span class="h-2 w-2 rounded-full" :style="{ backgroundColor: sphereTankGate.isActive.value ? `var(--accent-success)` : `var(--text-muted)` }"></span>
                <span class="text-[var(--text-muted)]">{{ t.shm_sphereTankGate }}</span>
                <span class="text-[var(--text-muted)]">·</span>
                <span class="font-medium" :style="{ color: sphereTankGate.isActive.value ? `var(--accent-success)` : `var(--text-muted)` }">
                  {{ sphereTankGate.isActive.value ? t.shm_activated : sphereTankGate.statusText.value }}
                </span>
              </div>
              <!-- 左右视觉分隔 -->
              <span class="text-[var(--text-muted)]">|</span>
              <!-- 右侧：实时读数块（稳定时间 | 压力） -->
              <div class="flex items-center gap-2 whitespace-nowrap">
                <!-- 球罐当前稳定时间实时显示：仅展示，不参与判定；
                     无数据时显示固定宽度占位"--.--"避免布局抖动，单位 s 紧随数字避免"没配置"错觉 -->
                <span class="text-[var(--text-muted)]">{{ t.wf_sphereStableTimeLabel }}</span>
                <span class="font-mono font-bold text-[var(--text-primary)] tabular-nums">
                  {{ sphereTankGate.stableTimeSec.value !== null ? sphereTankGate.stableTimeSec.value.toFixed(2) : '--.--' }}{{ t.wf_sphereStableTimeUnit }}
                </span>
                <!-- 球罐压力实时显示：仅展示，不参与判定；
                     无数据时显示固定宽度占位"--.--"避免布局抖动，单位 kPa 紧随数字避免"没配置"错觉 -->
                <span class="text-[var(--text-muted)]">|</span>
                <span class="text-[var(--text-muted)]">{{ t.wf_spherePressureLabel }}</span>
                <span class="font-mono font-bold text-[var(--text-primary)] tabular-nums">
                  {{ sphereTankGate.pressureValue.value !== null ? sphereTankGate.pressureValue.value.toFixed(2) : '--.--' }}{{ t.wf_spherePressureUnit }}
                </span>
              </div>
            </div>
          </div>
        </div>

        <!-- 右侧主内容区 -->
        <div class="flex min-w-0 flex-1 flex-col overflow-hidden">
          <!-- 七孔图表区域切换 Tab 栏：7 个区域（内区 + 外区 1~6 区）
               替代原"角度提示+图表标题"行——区域切换是七孔校准主画面的核心导航，
               放在内容区顶部更符合操作员视觉动线。
               7 个区域对应 1 个内区（P7 最大）+ 6 个外区扇区（P1~P6 最大）。
               Tab 状态通过 v-model 传给 SevenHoleCharts 子组件控制图表数据源。 -->
          <div class="flex flex-shrink-0 items-center gap-1 border-b border-[var(--border-default)] bg-[var(--bg-panel)] px-3 py-1.5">
            <button
              v-for="tab in chartTabs"
              :key="tab.key"
              type="button"
              class="rounded-md px-3 py-1 text-xs font-medium transition-colors"
              :style="activeChartTab === tab.key ? {
                backgroundColor: `color-mix(in srgb, var(--accent-primary) 12%, transparent)`,
                color: `var(--accent-primary)`,
              } : {
                color: `var(--text-muted)`,
              }"
              @click="activeChartTab = tab.key"
            >{{ tab.label }}</button>
          </div>

          <!-- 主区域：七孔校准特性曲线图（spec Task 22 实现）
               - 内区 3 类真实图表：Kα-Kβ 网格折线图（α-β 双向等值线 + 当前点高亮） + α-K0 / α-Ks 曲线（按 β 分组）
               - 外区每个扇区 3 类真实图表：Kθ-Kφ 网格折线图（θ-φ 双向等值线 + 当前点高亮） + φ-K0[n] / φ-Ks[n] 曲线（按 θ 分组）
               数据来源：calibrationStore.dataPoints，按 activeChartTab 过滤区域与扇区 -->
          <div class="flex-1 overflow-hidden p-4">
            <SevenHoleCharts :active-tab="activeChartTab" :current-point-id="progressInfo?.currentPointId ?? null" />
          </div>
        </div>
      </div>
    </template>
  </div>
</template>

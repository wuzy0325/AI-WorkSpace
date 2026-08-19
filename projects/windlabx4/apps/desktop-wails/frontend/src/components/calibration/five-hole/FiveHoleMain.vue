<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount, watch, nextTick, type ComponentPublicInstance } from 'vue'
import { storeToRefs } from 'pinia'
import { useCalibrationWorkflow } from '@composables/useCalibrationWorkflow'
import { deviceApi } from '@api/deviceApi'
import { useI18nStore } from '@stores/i18nStore'
import type { CalibrationConfig, CalibrationDataPoint, ProbeChannelRole } from '@shared/types/calibration'
import type { DataPayload } from '@api/types'
import { isFiveHoleDataPoint } from '@shared/calibrationDataGuards'
import { getProbeChannelPrecision } from '@shared/calibrationPrecision'
import { findChannelValue } from '@shared/calibrationSnapshotValue'
import { drawFiveHoleChartScaffold, resolveKAlphaKbetaBounds, drawNoDataHint, drawAxisTicks, setupCanvas, CHART_COLORS } from './fiveHoleChartUtils'
import {
  ArrowLeft,
  Settings,
  Play,
  Pause,
  Square,
  Save,
  FileText,
  ChevronDown,
  ChevronUp,
  Gauge,
  TrendingUp,
  Target,
  Wind,
  Navigation2,
} from '@lucide/vue'
import UiButton from '@components/ui/UiButton.vue'
import MotionSafetyAlertCard from '@components/shared/MotionSafetyAlertCard.vue'
import { getImportedFiveHolePoints } from './importedFiveHolePoints'

const emit = defineEmits<{
  openSettings: []
  back: []
}>()

// 复用 useCalibrationWorkflow 暴露的派生状态：与三孔保持一致，
// 五孔特化部分（startCalibration、风洞通道校验）仍保留在本组件内。
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
} = useCalibrationWorkflow('five-hole')

const { t } = storeToRefs(useI18nStore())

// 暴露 reloadSavedConfig 给父组件 CalibrationWindow：
// Settings 保存配置后父组件调 currentMainRef.reloadSavedConfig() 触发重新加载，
// 否则 currentConfig 仍是挂载时的旧值，canStartCalibration 不刷新，会一直提示"未配置"。
// 与 ThreeHoleMain / TotalPressureMain / TotalTemperatureMain 保持一致，
// 由 calibrationMainExpose.contract.test.ts 在编译期断言本暴露存在。
async function reloadSavedConfig(): Promise<void> {
  await loadSavedConfig()
  applyImportedPointOverride()
}

function applyImportedPointOverride(): void {
  const imported = getImportedFiveHolePoints()
  if (imported && currentConfig.value) currentConfig.value.points = imported.points
}

defineExpose({ reloadSavedConfig })

// Tab 切换：与三孔一致提供 概览/图表/数据 三个视图，
// 让校准员既能总览系数与曲线，也能放大查看单图表，也能查阅完整数据表。
const activeTab = ref<'overview' | 'chart' | 'data'>('overview')
const showConfigSummary = ref(false)

// 实时采集快照订阅：deviceApi.onSnapshot 推送设备通道原始数据，
// 由 buildRealtimePressuresFromSnapshots 映射到 RealtimePressures 喂给 store，
// 驱动"实时通道数据"面板更新。与 ThreeHoleMain.vue 的模式一致。
let unsubscribeDaqSnapshot: (() => void) | null = null
let unsubscribeDeviceStatus: (() => void) | null = null
let unsubscribeMotionStatus: (() => void) | null = null
// 运动控制器状态轮询定时器：后端 Wails 事件推送频率不足以支撑"当前位置"实时显示，
// 需 300ms 主动拉取 status（与 ThreeHoleMain.vue 保持一致），驱动 actualAngles 更新。
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

// 挂载后订阅采集快照
watch(isLoading, (loading) => {
  if (loading) return
  applyImportedPointOverride()
  cleanupSubscriptions()
  unsubscribeDaqSnapshot = deviceApi.onSnapshot((payload: DataPayload) => {
    latestSnapshots.value.set(payload.deviceId, payload)
    if (!currentConfig.value || currentConfig.value.type !== 'five-hole') return
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

// 开始校准 (五孔特化：直接使用保存时已生成的 points，避免 fiveHoleLayout 与 points 双真值源不一致)
//
// 链路说明：
//   保存配置时 FiveHoleSettings.saveConfig 已用 pointLayout 生成 points 并一起落盘；
//   后端 CalibrationConfigDTO 也只接收 Points 字段（无 FiveHoleLayout）。
//   因此启动校准时直接用 currentConfig.points 即可，不再用 fiveHoleLayout 重新生成，
//   避免重生成路径（Wails 模式走本地兜底）与 layout 字段脏值（null/NaN）导致起始角度错乱。
async function startCalibration() {
  if (!canStartCalibration.value || !currentConfig.value) {
    feedbackStore.pushToast(startDisabledReason.value || t.value.fh_preCheckNotDone, 'warning')
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
    feedbackStore.pushToast(t.value.fh_startFailed + ': ' + (err instanceof Error ? err.message : String(err)), 'error')
  }
}

// 五孔特化校验：风洞总压/静压/温度三个通道必须配置齐全，否则马赫数/速度无法计算。
// 与三孔不同：三孔可用大气温度近似风洞总温，五孔要求独立风洞温度通道。
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
  ['fiveHole.pTotal'],
  [/总压/, /total pressure/, /(?:^|[^a-z0-9])p0(?:[^a-z0-9]|$)/, /(?:^|[^a-z0-9])pt(?:[^a-z0-9]|$)/],
))

const hasWindTunnelStaticPressureChannel = computed(() => hasConfiguredProbeChannel(
  ['fiveHole.pTunnelStatic'],
  [/静压/, /static pressure/, /(?:^|[^a-z0-9])ps(?:[^a-z0-9]|$)/],
))

const hasWindTunnelTemperatureChannel = computed(() => hasConfiguredProbeChannel(
  ['fiveHole.tTunnel'],
  [/风洞.*温/, /tunnel temp/, /tunnel temperature/, /(?:^|[^a-z0-9])ttunnel(?:[^a-z0-9]|$)/],
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
  if (isLoading.value) return t.value.fh_loadingConfig
  if (!hasConfig.value) return t.value.fh_noConfig
  if (!hasRequiredWindTunnelChannels.value) return t.value.fh_noWindTunnelChannels
  if (!isAcquisitionDeviceConnected.value) return t.value.fh_deviceNotReady
  if (!isMotionControllerConnected.value) return t.value.fh_motionNotConnected
  return ''
})

// 实时攻角/侧滑角：从位移机构当前实际位置解析
// 五孔探针校准的 α/β 本质是旋转轴角度，motionStore.statusList 每 300ms 刷新一次
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

// 目标 α/β：从 progressInfo.currentPoint.coordinates 取（与三孔读 θ 一致）
// progressInfo 优先用 currentPointIndex（循环顶部推进，早于 moveToPoint）作索引从 config.points 查出当前点，
// 让目标角度先于实际角度变化；后端 autoEngine 为 nil 时回退到 completedPoints。
const targetAngles = computed<{ alpha: number | null; beta: number | null }>(() => {
  const coords = progressInfo.value?.currentPoint
  const alpha = coords?.['α']
  const beta = coords?.['β']
  return {
    alpha: typeof alpha === 'number' ? alpha : null,
    beta: typeof beta === 'number' ? beta : null,
  }
})

function formatFiveHoleRealtimeValue(
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
    Patm: 0,
    Tatm: 0,
    P0: undefined as number | undefined,
    Ps: undefined as number | undefined,
    Ttunnel: undefined as number | undefined,
  }
  let matchedChannelCount = 0
  // 风洞温度优先级标记：tTunnel 角色优先于 tAtm 兜底，与总压对齐
  let hasTunnelTemp = false

  const assignByRole = (role: ProbeChannelRole | undefined, value: number): void => {
    switch (role) {
      case 'fiveHole.p1':
        result.P1 = value
        matchedChannelCount += 1
        break
      case 'fiveHole.p2':
        result.P2 = value
        matchedChannelCount += 1
        break
      case 'fiveHole.p3':
        result.P3 = value
        matchedChannelCount += 1
        break
      case 'fiveHole.p4':
        result.P4 = value
        matchedChannelCount += 1
        break
      case 'fiveHole.p5':
        result.P5 = value
        matchedChannelCount += 1
        break
      case 'fiveHole.pAtm':
        result.Patm = value
        matchedChannelCount += 1
        break
      // 五孔校准若无独立风洞温度传感器（tTunnel），用大气温度近似风洞总温（TAT），
      // 低速风洞下误差可接受。与 TotalPressureMain 的 hasTunnelTemp 模式一致。
      case 'fiveHole.tAtm':
        result.Tatm = value
        if (!hasTunnelTemp) result.Ttunnel = value
        matchedChannelCount += 1
        break
      case 'fiveHole.pTotal':
        result.P0 = value
        matchedChannelCount += 1
        break
      case 'fiveHole.pTunnelStatic':
        result.Ps = value
        matchedChannelCount += 1
        break
      case 'fiveHole.tTunnel':
        result.Ttunnel = value
        hasTunnelTemp = true
        matchedChannelCount += 1
        break
      default:
        break
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

    const name = ch.name.trim()
    const normalizedName = name.toLowerCase()
    if (/(?:^|[^a-z0-9])p1(?:[^a-z0-9]|$)/.test(normalizedName) || /孔\s*1/.test(name) || name.includes('孔1压力')) {
      result.P1 = rawValue
      matchedChannelCount += 1
    } else if (/(?:^|[^a-z0-9])p2(?:[^a-z0-9]|$)/.test(normalizedName) || /孔\s*2/.test(name) || name.includes('孔2压力')) {
      result.P2 = rawValue
      matchedChannelCount += 1
    } else if (/(?:^|[^a-z0-9])p3(?:[^a-z0-9]|$)/.test(normalizedName) || /孔\s*3/.test(name) || name.includes('孔3压力')) {
      result.P3 = rawValue
      matchedChannelCount += 1
    } else if (/(?:^|[^a-z0-9])p4(?:[^a-z0-9]|$)/.test(normalizedName) || /孔\s*4/.test(name) || name.includes('孔4压力')) {
      result.P4 = rawValue
      matchedChannelCount += 1
    } else if (/(?:^|[^a-z0-9])p5(?:[^a-z0-9]|$)/.test(normalizedName) || /孔\s*5/.test(name) || name.includes('孔5压力')) {
      result.P5 = rawValue
      matchedChannelCount += 1
    } else if (/大气.*压/.test(name) || normalizedName.includes('atmospheric pressure') || /(?:^|[^a-z0-9])patm(?:[^a-z0-9]|$)/.test(normalizedName) || normalizedName.includes('p∞')) {
      result.Patm = rawValue
      matchedChannelCount += 1
    } else if (/大气.*温/.test(name) || normalizedName.includes('atmospheric temp') || normalizedName.includes('atmospheric temperature') || /(?:^|[^a-z0-9])tatm(?:[^a-z0-9]|$)/.test(normalizedName) || normalizedName.includes('t∞')) {
      result.Tatm = rawValue
      // 大气温度在无独立风洞温度通道时兜底为 Ttunnel，与 role 分支保持一致
      if (!hasTunnelTemp) result.Ttunnel = rawValue
      matchedChannelCount += 1
    } else if (/总压/.test(name) || normalizedName.includes('total pressure') || /(?:^|[^a-z0-9])p0(?:[^a-z0-9]|$)/.test(normalizedName) || /(?:^|[^a-z0-9])pt(?:[^a-z0-9]|$)/.test(normalizedName)) {
      result.P0 = rawValue
      matchedChannelCount += 1
    } else if (/静压/.test(name) || normalizedName.includes('static pressure') || /(?:^|[^a-z0-9])ps(?:[^a-z0-9]|$)/.test(normalizedName)) {
      result.Ps = rawValue
      matchedChannelCount += 1
    } else if (/风洞.*温/.test(name) || normalizedName.includes('tunnel temp') || normalizedName.includes('tunnel temperature') || /(?:^|[^a-z0-9])ttunnel(?:[^a-z0-9]|$)/.test(normalizedName)) {
      result.Ttunnel = rawValue
      // 标记已有独立风洞温度，阻止后续 tAtm 兜底覆盖，与 role 分支保持一致
      hasTunnelTemp = true
      matchedChannelCount += 1
    }
  }

  if (matchedChannelCount === 0) {
    return null
  }

  return result
}

// ===== 图表绘制：保留五孔原有 Kα-Kβ / CPT-α / CPS-α 三图表逻辑 =====

function groupByBeta(
  points: CalibrationDataPoint[],
  coefficient: 'CPT' | 'CPS',
) {
  const groups = new Map<number, { alpha: number; value: number }[]>()
  points.forEach((p) => {
    const beta = p.coordinates['β'] as number
    const alpha = p.coordinates['α'] as number
    const value = coefficient === 'CPT' ? p.coefficients.CPT : p.coefficients.CPS
    if (!groups.has(beta)) groups.set(beta, [])
    groups.get(beta)!.push({ alpha, value })
  })
  groups.forEach((group) => group.sort((a, b) => a.alpha - b.alpha))
  return Array.from(groups.entries()).map(([beta, data]) => ({ beta, data }))
}

function groupByAlpha(
  points: CalibrationDataPoint[],
) {
  const groups = new Map<number, { beta: number; Kalpha: number; Kbeta: number; pointId: number }[]>()
  points.forEach((p) => {
    const alpha = p.coordinates['α'] as number
    const beta = p.coordinates['β'] as number
    if (!groups.has(alpha)) groups.set(alpha, [])
    groups.get(alpha)!.push({ beta, Kalpha: p.coefficients.Kalpha, Kbeta: p.coefficients.Kbeta, pointId: p.pointId })
  })
  groups.forEach((group) => group.sort((a, b) => a.beta - b.beta))
  return Array.from(groups.entries()).map(([alpha, data]) => ({ alpha, data }))
}

function groupByBetaKb(
  points: CalibrationDataPoint[],
) {
  const groups = new Map<number, { alpha: number; Kalpha: number; Kbeta: number }[]>()
  points.forEach((p) => {
    const beta = p.coordinates['β'] as number
    const alpha = p.coordinates['α'] as number
    if (!groups.has(beta)) groups.set(beta, [])
    groups.get(beta)!.push({ alpha, Kalpha: p.coefficients.Kalpha, Kbeta: p.coefficients.Kbeta })
  })
  groups.forEach((group) => group.sort((a, b) => a.alpha - b.alpha))
  return Array.from(groups.entries()).map(([beta, data]) => ({ beta, data }))
}

// Kα-Kβ 图表 Canvas
const kAlphaKbetaCanvas = ref<HTMLCanvasElement | null>(null)
const cptAlphaCanvas = ref<HTMLCanvasElement | null>(null)
const cpsAlphaCanvas = ref<HTMLCanvasElement | null>(null)

function setCptAlphaCanvasRef(el: Element | ComponentPublicInstance | null): void {
  cptAlphaCanvas.value = toCanvasElement(el)
}

function setCpsAlphaCanvasRef(el: Element | ComponentPublicInstance | null): void {
  cpsAlphaCanvas.value = toCanvasElement(el)
}

function toCanvasElement(el: Element | ComponentPublicInstance | null): HTMLCanvasElement | null {
  if (el instanceof HTMLCanvasElement) {
    return el
  }
  if (el && typeof el === 'object' && '$el' in el) {
    const root = (el as ComponentPublicInstance).$el
    return root instanceof HTMLCanvasElement ? root : null
  }
  return null
}

// 绘制 Kα-Kβ 网格折线图
function drawKAlphaKbetaChart() {
  const canvas = kAlphaKbetaCanvas.value
  if (!canvas) return
  const ctx = setupCanvas(canvas)
  if (!ctx) return

  const rect = canvas.getBoundingClientRect()
  const width = rect.width
  const height = rect.height
  const padding = 42

  // 计算坐标范围（使用对称边界，确保原点居中）
  const points = calibrationStore.dataPoints.filter(isFiveHoleDataPoint)
  let xMin = -1.0, xMax = 1.0, yMin = -1.0, yMax = 1.0, tickCount = 4
  if (points.length > 0) {
    const scatterData = points.map((p) => ({ x: p.coefficients.Kalpha, y: p.coefficients.Kbeta }))
    const bounds = resolveKAlphaKbetaBounds(scatterData)
    xMin = bounds.xMin; xMax = bounds.xMax
    yMin = bounds.yMin; yMax = bounds.yMax
    tickCount = bounds.tickCount
  }

  // 等比例：图表区域不变，扩展较宽方向的数据范围，使横纵像素/数据单位一致
  const plotWidth = width - 2 * padding
  const plotHeight = height - 2 * padding
  const aspectRatio = plotWidth / plotHeight
  const xRange0 = xMax - xMin || 1
  const yRange0 = yMax - yMin || 1
  const xCenter = (xMin + xMax) / 2
  const yCenter = (yMin + yMax) / 2
  if (aspectRatio > xRange0 / yRange0) {
    const newXRange = yRange0 * aspectRatio
    xMin = xCenter - newXRange / 2; xMax = xCenter + newXRange / 2
  } else {
    const newYRange = xRange0 / aspectRatio
    yMin = yCenter - newYRange / 2; yMax = yCenter + newYRange / 2
  }
  const xRange = xMax - xMin
  const yRange = yMax - yMin

  drawFiveHoleChartScaffold(ctx, width, height, padding, 'Kα', 'Kβ', true, true)
  drawAxisTicks(ctx, width, height, padding, xMin, xMax, yMin, yMax, tickCount)

  if (points.length === 0) {
    drawNoDataHint(ctx, width, height)
    return
  }

  const toX = (v: number) => padding + ((v - xMin) / xRange) * plotWidth
  const toY = (v: number) => height - padding - ((v - yMin) / yRange) * plotHeight

  // 先绘制 beta 等值线（次要，灰色细线）
  const betaGroups = groupByBetaKb(points)
  betaGroups.forEach((group) => {
    if (group.data.length < 2) return
    ctx.strokeStyle = '#94a3b8'
    ctx.lineWidth = 1
    ctx.beginPath()
    group.data.forEach((pt, i) => {
      const x = toX(pt.Kalpha)
      const y = toY(pt.Kbeta)
      if (i === 0) ctx.moveTo(x, y)
      else ctx.lineTo(x, y)
    })
    ctx.stroke()

    // 小圆点
    ctx.fillStyle = '#94a3b8'
    group.data.forEach((pt) => {
      ctx.beginPath()
      ctx.arc(toX(pt.Kalpha), toY(pt.Kbeta), 1.8, 0, 2 * Math.PI)
      ctx.fill()
    })
  })

  // 再绘制 alpha 等值线（主要，彩色粗线）
  const alphaGroups = groupByAlpha(points)
  alphaGroups.forEach((group, index) => {
    const color = CHART_COLORS[index % CHART_COLORS.length]
    ctx.strokeStyle = color
    ctx.lineWidth = 2
    ctx.beginPath()
    group.data.forEach((pt, i) => {
      const x = toX(pt.Kalpha)
      const y = toY(pt.Kbeta)
      if (i === 0) ctx.moveTo(x, y)
      else ctx.lineTo(x, y)
    })
    ctx.stroke()

    // 数据点
    ctx.fillStyle = color
    group.data.forEach((pt) => {
      ctx.beginPath()
      ctx.arc(toX(pt.Kalpha), toY(pt.Kbeta), 2.5, 0, 2 * Math.PI)
      ctx.fill()
    })
  })

  // 当前采集点高亮：从 progressInfo.currentPointId 取目标点 id（useCalibrationWorkflow 单独暴露）。
  // 不再读 calibrationStore.status.currentPoint.id——后端该字段类型是 number（点索引）而非 CalPoint 对象，
  // 旧实现下 currentPointId 永远 undefined，高亮逻辑从不触发。
  const currentPointId = progressInfo.value?.currentPointId
  if (currentPointId != null) {
    const cp = points.find((p) => p.pointId === currentPointId)
    if (cp) {
      const x = toX(cp.coefficients.Kalpha)
      const y = toY(cp.coefficients.Kbeta)
      ctx.shadowColor = '#f97316'
      ctx.shadowBlur = 10
      ctx.fillStyle = '#f97316'
      ctx.beginPath()
      ctx.arc(x, y, 5, 0, 2 * Math.PI)
      ctx.fill()
      ctx.shadowBlur = 0
    }
  }
}

// 绘制 CPT-α 曲线图
function drawCptAlphaChart() {
  const canvas = cptAlphaCanvas.value
  if (!canvas) return
  const ctx = setupCanvas(canvas)
  if (!ctx) return

  const rect = canvas.getBoundingClientRect()
  const width = rect.width
  const height = rect.height
  const padding = 34
  drawFiveHoleChartScaffold(ctx, width, height, padding, 'α (°)', 'CPT')

  const points = calibrationStore.dataPoints.filter(isFiveHoleDataPoint)
  if (points.length === 0) {
    drawNoDataHint(ctx, width, height)
    return
  }

  const data = groupByBeta(points, 'CPT')
  if (data.length === 0) {
    drawNoDataHint(ctx, width, height)
    return
  }

  const allValues = data.flatMap((g) => g.data.map((d) => d.value))
  const xMin = Math.min(...data.flatMap((g) => g.data.map((d) => d.alpha)))
  const xMax = Math.max(...data.flatMap((g) => g.data.map((d) => d.alpha)))
  const yMin = Math.min(...allValues)
  const yMax = Math.max(...allValues)
  const xRange = xMax - xMin || 1
  const yRange = yMax - yMin || 1

  drawAxisTicks(ctx, width, height, padding, xMin, xMax, yMin, yMax)

  data.forEach((group, index) => {
    const color = CHART_COLORS[index % CHART_COLORS.length]
    ctx.strokeStyle = color
    ctx.lineWidth = 2
    ctx.beginPath()
    group.data.forEach((point, i) => {
      const x = padding + ((point.alpha - xMin) / xRange) * (width - 2 * padding)
      const y = height - padding - ((point.value - yMin) / yRange) * (height - 2 * padding)
      if (i === 0) ctx.moveTo(x, y)
      else ctx.lineTo(x, y)
    })
    ctx.stroke()

    ctx.fillStyle = color
    group.data.forEach((point) => {
      const x = padding + ((point.alpha - xMin) / xRange) * (width - 2 * padding)
      const y = height - padding - ((point.value - yMin) / yRange) * (height - 2 * padding)
      ctx.beginPath()
      ctx.arc(x, y, 2.5, 0, 2 * Math.PI)
      ctx.fill()
    })
  })
}

// 绘制 CPS-α 曲线图
function drawCpsAlphaChart() {
  const canvas = cpsAlphaCanvas.value
  if (!canvas) return
  const ctx = setupCanvas(canvas)
  if (!ctx) return

  const rect = canvas.getBoundingClientRect()
  const width = rect.width
  const height = rect.height
  const padding = 34
  drawFiveHoleChartScaffold(ctx, width, height, padding, 'α (°)', 'CPS')

  const points = calibrationStore.dataPoints.filter(isFiveHoleDataPoint)
  if (points.length === 0) {
    drawNoDataHint(ctx, width, height)
    return
  }

  const data = groupByBeta(points, 'CPS')
  if (data.length === 0) {
    drawNoDataHint(ctx, width, height)
    return
  }

  const allValues = data.flatMap((g) => g.data.map((d) => d.value))
  const xMin = Math.min(...data.flatMap((g) => g.data.map((d) => d.alpha)))
  const xMax = Math.max(...data.flatMap((g) => g.data.map((d) => d.alpha)))
  const yMin = Math.min(...allValues)
  const yMax = Math.max(...allValues)
  const xRange = xMax - xMin || 1
  const yRange = yMax - yMin || 1

  drawAxisTicks(ctx, width, height, padding, xMin, xMax, yMin, yMax)

  data.forEach((group, index) => {
    const color = CHART_COLORS[index % CHART_COLORS.length]
    ctx.strokeStyle = color
    ctx.lineWidth = 2
    ctx.beginPath()
    group.data.forEach((point, i) => {
      const x = padding + ((point.alpha - xMin) / xRange) * (width - 2 * padding)
      const y = height - padding - ((point.value - yMin) / yRange) * (height - 2 * padding)
      if (i === 0) ctx.moveTo(x, y)
      else ctx.lineTo(x, y)
    })
    ctx.stroke()

    ctx.fillStyle = color
    group.data.forEach((point) => {
      const x = padding + ((point.alpha - xMin) / xRange) * (width - 2 * padding)
      const y = height - padding - ((point.value - yMin) / yRange) * (height - 2 * padding)
      ctx.beginPath()
      ctx.arc(x, y, 2.5, 0, 2 * Math.PI)
      ctx.fill()
    })
  })
}

// 更新图表：切换 Tab 后主动触发，确保 canvas 尺寸从 0 变为实际值后能正确绘制
function updateCharts() {
  drawKAlphaKbetaChart()
  drawCptAlphaChart()
  drawCpsAlphaChart()
}

// 图表定时刷新：让"当前采集点高亮"随 progressInfo.currentPointId 变化而更新，
// 同时防御 canvas 在隐藏 Tab 中尺寸为 0 时的绘制异常。
let chartTimer: ReturnType<typeof setInterval> | null = null
function startChartTimer(): void {
  if (chartTimer) clearInterval(chartTimer)
  chartTimer = setInterval(updateCharts, Math.round(calibrationStore.uiRefreshIntervalMs))
}

onMounted(() => {
  startChartTimer()
})

watch(
  () => calibrationStore.uiRefreshIntervalMs,
  () => {
    startChartTimer()
  },
)

// 切换到图表 Tab 时主动触发重绘（canvas 尺寸从 0 变为实际值）
watch(activeTab, (tab) => {
  if (tab === 'chart' || tab === 'overview') {
    nextTick(updateCharts)
  }
})

onBeforeUnmount(() => {
  if (chartTimer) clearInterval(chartTimer)
  cleanupSubscriptions()
  // spec Decision #3 / I1：unmount 不再调 calibrationStore.reset()，保留会话状态供切回 / 导出。
  // releaseView（引用计数-1 + 降频 polling）已由 useCalibrationWorkflow.onBeforeUnmount 统一处理。
})

// ===== 顶部状态栏派生状态（参考 ThreeHoleMain.vue）=====

const completedPoints = computed(() => calibrationStore.dataPoints.length)
const totalPoints = computed(() => currentConfig.value?.points.length ?? 0)
const progressPercent = computed(() => {
  if (!totalPoints.value) return 0
  return Math.round((completedPoints.value / totalPoints.value) * 100)
})
const formattedProgress = computed(() => {
  return `${completedPoints.value} / ${totalPoints.value} (${progressPercent.value}%)`
})

// 数据 Tab 顶部"共 X 条记录"文案：复用 {count} 占位符替换约定（与 ThreeHoleMain 一致）
const recordCountText = computed(() =>
  t.value.fh_recordCount.replace('{count}', String(calibrationStore.dataPoints.length)),
)

const statusText = computed(() => {
  // spec Decision #15 / I7：按 status.status 精确映射，避免 stop 后退化为「空闲」
  const s = calibrationStore.status?.status
  if (s === 'running') return t.value.running
  if (s === 'paused') return t.value.statusPaused
  if (s === 'stopped') return t.value.wf_statusStopped
  if (s === 'completed') return t.value.completed
  if (s === 'error') return t.value.error
  return t.value.idle
})

const statusColor = computed(() => {
  const s = calibrationStore.status?.status
  if (s === 'running') return 'success'
  if (s === 'paused') return 'warning'
  // 已停止：黄色警示色，与 idle（normal）区分，提示「保留数据可导出」
  if (s === 'stopped') return 'warning'
  if (s === 'completed') return 'info'
  if (s === 'error') return 'danger'
  return 'normal'
})

// 状态色 CSS 变量标识：将 statusColor 转为设计 token，替代 Tailwind 调色板硬编码
const statusColorToken = computed(() => {
  switch (statusColor.value) {
    case 'success': return '--accent-success'
    case 'warning': return '--accent-warning'
    case 'info': return '--accent-info'
    default: return '--text-muted'
  }
})

const canPause = computed(() => calibrationStore.isRunning && !calibrationStore.isPaused)
const canResume = computed(() => calibrationStore.isPaused)
const canStop = computed(() => calibrationStore.isRunning || calibrationStore.isPaused)
const canSave = computed(() => calibrationStore.completeEvent !== null || calibrationStore.dataPoints.length > 0)

// 实时气动参数（马赫数/速度）：spec Task 15 后由后端 CalibrationStatus.livePhysics 提供，
// store 在 updateStatusFromBackend 中映射，组件保持纯展示。
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

// 实时 CSV 路径：校准启动后操作员需知道数据写到哪
const csvSavePath = computed(() => currentConfig.value?.savePath ?? '')

// 配置摘要：五孔特有 α/β 双轴布局
const fiveHoleLayout = computed(() => currentConfig.value?.fiveHoleLayout)

// 最新系数：取最后一条五孔数据点
const latestCoefficients = computed(() => {
  const points = calibrationStore.dataPoints
  if (!points.length) return null
  const lastPoint = points[points.length - 1]
  if (isFiveHoleDataPoint(lastPoint)) return lastPoint.coefficients
  return null
})

// 最新原始数据：取最后一条五孔数据点
const latestRawData = computed(() => {
  const points = calibrationStore.dataPoints
  if (!points.length) return null
  const lastPoint = points[points.length - 1]
  if (isFiveHoleDataPoint(lastPoint)) return lastPoint.rawData
  return null
})

// 五孔数据点筛选结果缓存：避免 template 中多处重复 filter 创建新数组引用
const fiveHolePoints = computed(() => {
  return calibrationStore.dataPoints.filter(isFiveHoleDataPoint)
})

// 通道分组：核心通道 P1-P5 / 次要通道 Patm/Tatm/Pt/Ps/Tt
const probeChannels = computed(() => currentConfig.value?.probeChannels ?? [])

const coreChannels = computed(() => {
  const coreRoles = ['fiveHole.p1', 'fiveHole.p2', 'fiveHole.p3', 'fiveHole.p4', 'fiveHole.p5']
  return probeChannels.value.filter((c) => coreRoles.includes(c.role || ''))
})

const secondaryChannels = computed(() => {
  const coreRoles = ['fiveHole.p1', 'fiveHole.p2', 'fiveHole.p3', 'fiveHole.p4', 'fiveHole.p5']
  return probeChannels.value.filter((c) => !coreRoles.includes(c.role || ''))
})

function formatValue(value: number | undefined | null, precision?: number): string {
  if (value === undefined || value === null) return '--'
  return value.toFixed(precision ?? 3)
}

function getChannelValue(role: string): string {
  const pressures = calibrationStore.realtimePressures
  if (!pressures) return '--'
  switch (role) {
    case 'fiveHole.p1': return formatFiveHoleRealtimeValue('fiveHole.p1', pressures.P1)
    case 'fiveHole.p2': return formatFiveHoleRealtimeValue('fiveHole.p2', pressures.P2)
    case 'fiveHole.p3': return formatFiveHoleRealtimeValue('fiveHole.p3', pressures.P3)
    case 'fiveHole.p4': return formatFiveHoleRealtimeValue('fiveHole.p4', pressures.P4)
    case 'fiveHole.p5': return formatFiveHoleRealtimeValue('fiveHole.p5', pressures.P5)
    case 'fiveHole.pAtm': return formatFiveHoleRealtimeValue('fiveHole.pAtm', pressures.Patm)
    case 'fiveHole.tAtm': return formatFiveHoleRealtimeValue('fiveHole.tAtm', pressures.Tatm)
    case 'fiveHole.pTotal': return formatFiveHoleRealtimeValue('fiveHole.pTotal', pressures.P0)
    case 'fiveHole.pTunnelStatic': return formatFiveHoleRealtimeValue('fiveHole.pTunnelStatic', pressures.Ps)
    case 'fiveHole.tTunnel': return formatFiveHoleRealtimeValue('fiveHole.tTunnel', pressures.Ttunnel)
    default: return '--'
  }
}

function getChannelUnit(role: string): string {
  const ch = probeChannels.value.find((c: { role?: string }) => c.role === role)
  if (!ch) {
    // 默认单位兜底：温度类角色给 °C，其他给 Pa
    if (role === 'fiveHole.tAtm' || role === 'fiveHole.tTunnel') return '°C'
    return 'Pa'
  }
  const device = deviceStore.profiles?.find((d) => d.id === ch.channel.deviceId)
  const channelConfig = device?.channels[ch.channel.channelIndex]
  return channelConfig?.unit ?? 'Pa'
}
</script>

<template>
  <div data-test="five-hole-main-shell" class="flex h-full flex-col bg-[var(--bg-canvas)] text-[var(--text-primary)]">
    <!-- Header -->
    <div class="flex items-center justify-between border-b border-[var(--border-default)] bg-[var(--bg-panel)] px-5 py-2.5">
      <div class="flex items-center gap-3">
        <UiButton variant="secondary" size="sm" @click="emit('back')">
          <ArrowLeft class="h-4 w-4" />
        </UiButton>
        <div>
          <h1 class="text-base font-bold text-[var(--text-primary)]">{{ t.fh_fiveHoleCalibration }}</h1>
        </div>
      </div>
      <div class="flex items-center gap-2">
        <!-- 开始按钮：从左侧栏移到 Header，与"配置"并列便于一键启动；
             点击仍走本地 startCalibration（含风洞通道校验特化逻辑） -->
        <UiButton v-if="!calibrationStore.isRunning && !calibrationStore.isPaused" variant="primary" size="sm" :disabled="!canStartCalibration" :title="startDisabledReason || undefined" @click="startCalibration">
          <Play class="h-4 w-4" />
          <span class="ml-1">{{ t.startRun }}</span>
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
        <p class="text-[var(--text-muted)]">{{ t.fh_loading }}</p>
      </div>
    </div>

    <template v-else>
      <!-- 顶部状态栏：跨全宽，校准员最频繁看的信息（状态/进度/时间/目标α/β/实际α/β/Ma/V）
           sticky 定位确保运动控制器运行时内容区滚动不会导致状态栏位置跳动 -->
      <div class="sticky top-0 z-10 flex items-center gap-4 border-b border-[var(--border-default)] bg-[var(--bg-panel)] px-5 py-2.5">
        <span
          class="rounded-full px-2 py-0.5 text-xs font-medium"
          :style="{
            backgroundColor: `color-mix(in srgb, var(${statusColorToken}) 15%, transparent)`,
            color: `var(${statusColorToken})`,
          }"
        >{{ statusText }}</span>

        <div class="flex items-center gap-2 min-w-[180px] flex-1 max-w-[280px]">
          <span class="text-xs text-[var(--text-muted)] whitespace-nowrap">{{ t.travProgress }}</span>
          <div class="h-2 flex-1 overflow-hidden rounded-full bg-[var(--bg-panel-strong)]">
            <div class="h-full rounded-full bg-[var(--accent-primary)] transition-all duration-300" :style="{ width: progressPercent + '%' }"></div>
          </div>
          <span class="text-xs font-mono font-bold text-[var(--text-primary)] whitespace-nowrap">{{ formattedProgress }}</span>
        </div>

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

        <!-- 目标 α/β + 实际 α/β：校准员持续盯的核心信息，五孔双轴并列展示 -->
        <div class="flex items-center gap-4 border-l border-[var(--border-default)] pl-4">
          <div class="flex items-center gap-1.5">
            <Target class="h-4 w-4 text-[var(--text-muted)]" />
            <span class="text-xs text-[var(--text-muted)]">{{ t.travTarget }}</span>
            <span class="font-mono text-base font-bold text-[var(--text-primary)]">
              α{{ targetAngles.alpha !== null ? targetAngles.alpha.toFixed(1) + '°' : '--' }}
              <span class="mx-1 text-[var(--text-muted)]">/</span>
              β{{ targetAngles.beta !== null ? targetAngles.beta.toFixed(1) + '°' : '--' }}
            </span>
          </div>
          <div class="flex items-center gap-1.5">
            <span class="text-xs text-[var(--text-muted)]">{{ t.travActual }}</span>
            <span class="font-mono text-base font-bold" :style="{ color: isMoving ? `var(--accent-success)` : `var(--accent-primary)` }">
              α{{ actualAngles.alpha != null ? actualAngles.alpha.toFixed(2) + '°' : '--' }}
              <span class="mx-1 text-[var(--text-muted)]">/</span>
              β{{ actualAngles.beta != null ? actualAngles.beta.toFixed(2) + '°' : '--' }}
            </span>
            <span v-if="isMoving" class="flex items-center gap-1 text-xs" :style="{ color: `var(--accent-success)` }">
              <span class="h-1.5 w-1.5 animate-pulse rounded-full" :style="{ backgroundColor: `var(--accent-success)` }"></span>
              {{ t.moving }}
            </span>
          </div>
        </div>

        <!-- 当前点采样子进度 -->
        <div v-if="sampleProgress" class="flex items-center gap-2 border-l border-[var(--border-default)] pl-4">
          <span class="text-xs text-[var(--text-muted)]">{{ t.samples }}</span>
          <span class="font-mono text-sm font-bold text-[var(--accent-primary)]">{{ sampleProgress.current }}/{{ sampleProgress.total }}</span>
          <div class="h-1.5 w-16 overflow-hidden rounded-full bg-[var(--bg-panel-strong)]">
            <div class="h-full rounded-full bg-[var(--accent-primary)] transition-all duration-200" :style="{ width: sampleProgress.percent + '%' }"></div>
          </div>
        </div>

        <!-- 错误详情 -->
        <div v-if="lastError" class="flex items-center gap-1 border-l border-[var(--border-default)] pl-4" :title="lastError">
          <span class="text-xs font-medium" :style="{ color: `var(--accent-danger)` }">⚠ {{ lastError.length > 30 ? lastError.slice(0, 30) + '...' : lastError }}</span>
        </div>

        <!-- 配置摘要折叠：校准中几乎不看，压缩到角落 -->
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
        <div><span class="text-[var(--text-muted)]">{{ t.name }}：</span><span class="font-medium text-[var(--text-primary)]">{{ currentConfig?.name || t.unconfigured }}</span></div>
        <div v-if="fiveHoleLayout">
          <span class="text-[var(--text-muted)]">{{ t.fh_alphaRange }}：</span>
          <span class="font-medium text-[var(--text-primary)]">{{ fiveHoleLayout.alphaMin }}° ~ {{ fiveHoleLayout.alphaMax }}° ({{ fiveHoleLayout.alphaStep }}°)</span>
        </div>
        <div v-if="fiveHoleLayout">
          <span class="text-[var(--text-muted)]">{{ t.fh_betaRange }}：</span>
          <span class="font-medium text-[var(--text-primary)]">{{ fiveHoleLayout.betaMin }}° ~ {{ fiveHoleLayout.betaMax }}° ({{ fiveHoleLayout.betaStep }}°)</span>
        </div>
        <div><span class="text-[var(--text-muted)]">{{ t.totalPoints }}：</span><span class="font-medium text-[var(--text-primary)]">{{ totalPoints }}</span></div>
        <div><span class="text-[var(--text-muted)]">{{ t.dwell }}：</span><span class="font-medium text-[var(--text-primary)]">{{ currentConfig?.dwellTimeMs || 0 }}ms</span></div>
        <div><span class="text-[var(--text-muted)]">{{ t.samples }}：</span><span class="font-medium text-[var(--text-primary)]">{{ currentConfig?.samplesPerPoint || 0 }}</span></div>
      </div>

      <div class="flex flex-1 overflow-hidden">
        <!-- 左侧栏：384px，控制按钮(固定) + 通道/气动参数(可滚动) + 球罐状态条(固定底部) -->
        <div class="flex w-96 flex-col border-r border-[var(--border-default)] bg-[var(--bg-panel)] flex-shrink-0 overflow-hidden">
          <!-- 控制按钮（固定顶部） -->
          <div class="flex-shrink-0 border-b border-[var(--border-default)] p-3">
            <div class="grid grid-cols-2 gap-2">
              <UiButton v-if="canPause" variant="warning" @click="pauseCalibration">
                <Pause class="h-4 w-4" />
                <span class="ml-1">{{ t.fh_pause }}</span>
              </UiButton>
              <UiButton v-if="canResume" variant="primary" @click="resumeCalibration">
                <Play class="h-4 w-4" />
                <span class="ml-1">{{ t.fh_resume }}</span>
              </UiButton>
              <UiButton v-if="canStop" variant="danger" @click="stopCalibration">
                <Square class="h-4 w-4" />
                <span class="ml-1">{{ t.travStop }}</span>
              </UiButton>
            </div>
            <div v-if="startDisabledReason" class="mt-2 text-xs" :style="{ color: `var(--accent-warning)` }">{{ startDisabledReason }}</div>
          </div>

          <!-- 中间内容区（可滚动）：关键数据(P1-P5 + Ma/V) + 其他通道 -->
          <div class="flex-1 overflow-y-auto">
            <!-- 关键数据：核心压力 P1-P5 + 实时气动参数 Ma/V -->
            <div class="border-b border-[var(--border-default)] p-3">
              <div class="mb-2 flex items-center gap-2 text-sm font-medium text-[var(--text-primary)]">
                <Gauge class="h-4 w-4 text-[var(--accent-primary)]" />
                {{ t.fh_keyData }}
              </div>
              <div class="space-y-1.5">
                <!-- P1-P5 核心压力：大字突出 -->
                <div v-for="channel in coreChannels" :key="channel.name" class="flex items-baseline justify-between rounded-lg bg-[var(--bg-panel-strong)] px-3 py-1">
                  <span class="text-xs text-[var(--text-muted)]">{{ channel.name }}</span>
                  <div class="text-right">
                    <span class="font-mono text-xl font-bold text-[var(--accent-primary)]">{{ getChannelValue(channel.role || '') }}</span>
                    <span class="ml-1 text-xs text-[var(--text-muted)]">{{ getChannelUnit(channel.role || '') }}</span>
                  </div>
                </div>
                <!-- 马赫数 Ma：绿色强调，区别于压力通道 -->
                <div class="flex items-baseline justify-between rounded-lg bg-[var(--bg-panel-strong)] px-3 py-2">
                  <span class="text-xs text-[var(--text-muted)]">{{ t.fh_machMa }}</span>
                  <span class="font-mono text-2xl font-bold text-[var(--accent-success)]">{{ physics?.machNumber !== undefined ? physics.machNumber.toFixed(3) : '--' }}</span>
                </div>
                <!-- 速度 V：绿色强调。侧边栏统一保留 3 位小数，与马赫数精度对齐 -->
                <div class="flex items-baseline justify-between rounded-lg bg-[var(--bg-panel-strong)] px-3 py-2">
                  <span class="text-xs text-[var(--text-muted)]">{{ t.fh_velocityV }}</span>
                  <div class="text-right">
                    <span class="font-mono text-2xl font-bold text-[var(--accent-success)]">{{ physics?.velocity !== undefined ? physics.velocity.toFixed(3) : '--' }}</span>
                    <span class="ml-1 text-xs text-[var(--text-muted)]">m/s</span>
                  </div>
                </div>
              </div>
            </div>

            <!-- 其他通道：风洞总压/静压/温度、大气压/温度 -->
            <div class="p-3">
              <div class="mb-2 flex items-center gap-2 text-sm font-medium text-[var(--text-primary)]">
                <Wind class="h-4 w-4 text-[var(--accent-primary)]" />
                {{ t.fh_otherChannels }}
              </div>
              <div class="space-y-1.5">
                <div v-for="channel in secondaryChannels" :key="channel.name" class="flex items-center justify-between px-2 py-1.5 rounded bg-[var(--bg-panel-strong)]">
                  <span class="text-xs text-[var(--text-muted)]">{{ channel.name }}</span>
                  <div class="text-right">
                    <span class="font-mono text-sm font-bold text-[var(--text-primary)]">{{ getChannelValue(channel.role || '') }}</span>
                    <span class="ml-1 text-xs text-[var(--text-muted)]">{{ getChannelUnit(channel.role || '') }}</span>
                  </div>
                </div>
                <!-- 兜底：若未配置任何次要通道，提示操作员去配置 -->
                <div v-if="secondaryChannels.length === 0" class="px-2 py-2 text-xs text-[var(--text-muted)]">
                  {{ t.fh_noSecondaryChannels }}
                </div>
              </div>
            </div>
          </div>

          <!-- 球罐门控状态条（固定底部）：压缩为一行，附"编辑"入口跳配置界面 -->
          <div class="flex-shrink-0 border-t border-[var(--border-default)] p-3">
            <div class="flex items-center justify-between rounded-lg bg-[var(--bg-panel-strong)] px-3 py-2">
              <div class="flex items-center gap-2">
                <span class="h-2 w-2 rounded-full" :style="{ backgroundColor: sphereTankGate.isActive.value ? `var(--accent-success)` : `var(--text-muted)` }"></span>
                <span class="text-xs text-[var(--text-muted)]">{{ t.fh_sphereTankGate }}</span>
              </div>
              <div class="flex items-center gap-3 text-xs">
                <span class="font-medium" :style="{ color: sphereTankGate.isActive.value ? `var(--accent-success)` : `var(--text-muted)` }">
                  {{ sphereTankGate.isActive.value ? t.fh_activated : sphereTankGate.statusText.value }}
                </span>
                <span class="text-[var(--text-muted)]">|</span>
                <span class="font-mono font-bold text-[var(--text-primary)]">{{ sphereTankGate.waitTimeSec.value }}s</span>
                <!-- 球罐压力实时显示：仅展示，不参与判定；
                     无数据时显示固定宽度占位"--.--"避免布局抖动，单位 kPa 始终显示避免"没配置"错觉 -->
                <span class="text-[var(--text-muted)]">|</span>
                <span class="text-[var(--text-muted)]">{{ t.wf_spherePressureLabel }}</span>
                <span class="font-mono font-bold text-[var(--text-primary)] tabular-nums min-w-[56px] text-right">
                  {{ sphereTankGate.pressureValue.value !== null ? sphereTankGate.pressureValue.value.toFixed(2) : '--.--' }}
                </span>
                <span class="text-[var(--text-muted)]">{{ t.wf_spherePressureUnit }}</span>
              </div>
            </div>
          </div>
        </div>

        <!-- 右侧主内容区 -->
        <div class="flex flex-1 flex-col min-w-0 overflow-hidden">
          <!-- Tab 导航 -->
          <div class="flex border-b border-[var(--border-default)] bg-[var(--bg-panel)]">
            <UiButton quaternary size="sm" class="relative px-5 py-2.5 text-sm font-medium transition-colors" :class="activeTab === 'overview' ? 'text-[var(--accent-primary)]' : 'text-[var(--text-muted)] hover:text-[var(--text-primary)]'" @click="activeTab = 'overview'">
              <TrendingUp class="h-4 w-4" />
              {{ t.fh_overview }}
              <span v-if="activeTab === 'overview'" class="absolute bottom-0 left-0 right-0 h-0.5 bg-[var(--accent-primary)] rounded-t-full"></span>
            </UiButton>
            <UiButton quaternary size="sm" class="relative px-5 py-2.5 text-sm font-medium transition-colors" :class="activeTab === 'chart' ? 'text-[var(--accent-primary)]' : 'text-[var(--text-muted)] hover:text-[var(--text-primary)]'" @click="activeTab = 'chart'">
              <Navigation2 class="h-4 w-4" />
              {{ t.fh_chart }}
              <span v-if="activeTab === 'chart'" class="absolute bottom-0 left-0 right-0 h-0.5 bg-[var(--accent-primary)] rounded-t-full"></span>
            </UiButton>
            <UiButton quaternary size="sm" class="relative px-5 py-2.5 text-sm font-medium transition-colors" :class="activeTab === 'data' ? 'text-[var(--accent-primary)]' : 'text-[var(--text-muted)] hover:text-[var(--text-primary)]'" @click="activeTab = 'data'">
              <FileText class="h-4 w-4" />
              {{ t.fh_data }}
              <span v-if="activeTab === 'data'" class="absolute bottom-0 left-0 right-0 h-0.5 bg-[var(--accent-primary)] rounded-t-full"></span>
            </UiButton>
          </div>

          <!-- 概览页：左列（系数+原始数据）+ 右列（三图表） -->
          <div v-if="activeTab === 'overview'" class="flex-1 overflow-hidden p-4">
            <div class="flex h-full gap-3 min-h-0">
              <!-- 左列：系数 + 原始数据 -->
              <div class="flex w-[300px] flex-col gap-3 flex-shrink-0">
                <!-- 系数卡片（大字突出） -->
                <div class="rounded-xl border border-[var(--border-default)] bg-[var(--bg-panel)] p-4 shadow-[var(--shadow-panel)]">
                  <div class="mb-3 flex items-center gap-2">
                    <TrendingUp class="h-4 w-4 text-[var(--accent-primary)]" />
                    <h3 class="text-sm font-semibold text-[var(--text-primary)]">{{ t.fh_latestCoefficients }}</h3>
                  </div>
                  <div v-if="latestCoefficients" class="space-y-2">
                    <div class="flex items-baseline justify-between rounded-lg bg-[var(--bg-panel-strong)] px-3 py-2">
                      <span class="text-xs text-[var(--text-muted)]">Kα</span>
                      <span class="font-mono text-xl font-bold text-[var(--accent-primary)]">{{ formatValue(latestCoefficients.Kalpha, 4) }}</span>
                    </div>
                    <div class="flex items-baseline justify-between rounded-lg bg-[var(--bg-panel-strong)] px-3 py-2">
                      <span class="text-xs text-[var(--text-muted)]">Kβ</span>
                      <span class="font-mono text-xl font-bold text-[var(--accent-primary)]">{{ formatValue(latestCoefficients.Kbeta, 4) }}</span>
                    </div>
                    <div class="flex items-baseline justify-between rounded-lg bg-[var(--bg-panel-strong)] px-3 py-2">
                      <span class="text-xs text-[var(--text-muted)]">CPT</span>
                      <span class="font-mono text-xl font-bold text-[var(--accent-primary)]">{{ formatValue(latestCoefficients.CPT, 4) }}</span>
                    </div>
                    <div class="flex items-baseline justify-between rounded-lg bg-[var(--bg-panel-strong)] px-3 py-2">
                      <span class="text-xs text-[var(--text-muted)]">CPS</span>
                      <span class="font-mono text-xl font-bold text-[var(--accent-primary)]">{{ formatValue(latestCoefficients.CPS, 4) }}</span>
                    </div>
                    <!-- 实时马赫数/速度：与系数同卡展示，便于校准员关联系数与流场状态 -->
                    <div class="flex items-baseline justify-between rounded-lg bg-[var(--bg-panel-strong)] px-3 py-2 border-t border-[var(--border-default)] mt-2 pt-2">
                      <span class="text-xs text-[var(--text-muted)]">Ma</span>
                      <span class="font-mono text-xl font-bold text-[var(--accent-success)]">{{ physics?.machNumber !== undefined ? physics.machNumber.toFixed(3) : '--' }}</span>
                    </div>
                    <div class="flex items-baseline justify-between rounded-lg bg-[var(--bg-panel-strong)] px-3 py-2">
                      <span class="text-xs text-[var(--text-muted)]">V</span>
                      <div class="text-right">
                        <span class="font-mono text-xl font-bold text-[var(--accent-success)]">{{ physics?.velocity !== undefined ? physics.velocity.toFixed(3) : '--' }}</span>
                        <span class="ml-1 text-xs text-[var(--text-muted)]">m/s</span>
                      </div>
                    </div>
                  </div>
                  <div v-else class="flex h-24 items-center justify-center text-sm text-[var(--text-muted)]">{{ t.fh_noCoefficientsData }}</div>
                </div>

                <!-- 原始数据卡片：与系数卡片风格一致 -->
                <div class="flex-1 rounded-xl border border-[var(--border-default)] bg-[var(--bg-panel)] p-4 shadow-[var(--shadow-panel)] overflow-y-auto">
                  <div class="mb-3 flex items-center gap-2">
                    <Wind class="h-4 w-4 text-[var(--accent-primary)]" />
                    <h3 class="text-sm font-semibold text-[var(--text-primary)]">{{ t.fh_rawData }}</h3>
                  </div>
                  <div v-if="latestRawData" class="space-y-2">
                    <div class="flex items-baseline justify-between rounded-lg bg-[var(--bg-panel-strong)] px-3 py-2">
                      <span class="text-xs text-[var(--text-muted)]">P1</span>
                      <span class="font-mono text-lg font-bold text-[var(--accent-primary)]">{{ formatValue(latestRawData.p1, 1) }}</span>
                    </div>
                    <div class="flex items-baseline justify-between rounded-lg bg-[var(--bg-panel-strong)] px-3 py-2">
                      <span class="text-xs text-[var(--text-muted)]">P2</span>
                      <span class="font-mono text-lg font-bold text-[var(--accent-primary)]">{{ formatValue(latestRawData.p2, 1) }}</span>
                    </div>
                    <div class="flex items-baseline justify-between rounded-lg bg-[var(--bg-panel-strong)] px-3 py-2">
                      <span class="text-xs text-[var(--text-muted)]">P3</span>
                      <span class="font-mono text-lg font-bold text-[var(--accent-primary)]">{{ formatValue(latestRawData.p3, 1) }}</span>
                    </div>
                    <div class="flex items-baseline justify-between rounded-lg bg-[var(--bg-panel-strong)] px-3 py-2">
                      <span class="text-xs text-[var(--text-muted)]">P4</span>
                      <span class="font-mono text-lg font-bold text-[var(--accent-primary)]">{{ formatValue(latestRawData.p4, 1) }}</span>
                    </div>
                    <div class="flex items-baseline justify-between rounded-lg bg-[var(--bg-panel-strong)] px-3 py-2">
                      <span class="text-xs text-[var(--text-muted)]">P5</span>
                      <span class="font-mono text-lg font-bold text-[var(--accent-primary)]">{{ formatValue(latestRawData.p5, 1) }}</span>
                    </div>
                    <div class="flex items-baseline justify-between rounded-lg bg-[var(--bg-panel-strong)] px-3 py-2">
                      <span class="text-xs text-[var(--text-muted)]">{{ t.Patm }}</span>
                      <span class="font-mono text-lg font-bold text-[var(--accent-primary)]">{{ formatValue(latestRawData.pAtm, 1) }}</span>
                    </div>
                    <div class="flex items-baseline justify-between rounded-lg bg-[var(--bg-panel-strong)] px-3 py-2">
                      <span class="text-xs text-[var(--text-muted)]">{{ t.fh_windTunnelTotalPressure }}</span>
                      <span class="font-mono text-lg font-bold text-[var(--accent-primary)]">{{ formatValue(latestRawData.pTotal, 1) }}</span>
                    </div>
                    <div class="flex items-baseline justify-between rounded-lg bg-[var(--bg-panel-strong)] px-3 py-2">
                      <span class="text-xs text-[var(--text-muted)]">{{ t.fh_windTunnelStaticPressure }}</span>
                      <span class="font-mono text-lg font-bold text-[var(--accent-primary)]">{{ formatValue(latestRawData.pStatic, 1) }}</span>
                    </div>
                  </div>
                  <div v-else class="flex h-20 items-center justify-center text-sm text-[var(--text-muted)]">{{ t.fh_noRawData }}</div>
                </div>
              </div>

              <!-- 右列：三图表（Kα-Kβ 主图 + CPT-α / CPS-α 副图） -->
              <div class="flex flex-1 flex-col gap-3 min-w-0">
                <!-- Kα-Kβ 主图：占上半区，是五孔校准核心特征空间 -->
                <div class="flex-1 flex flex-col rounded-xl border border-[var(--border-default)] bg-[var(--bg-panel)] p-3 shadow-[var(--shadow-panel)] min-h-0">
                  <h3 class="mb-1 text-xs font-semibold text-[var(--text-muted)] flex-shrink-0">{{ t.fh_kAlphaKbetaSpace }}</h3>
                  <div class="flex-1 min-h-0">
                    <canvas ref="kAlphaKbetaCanvas" class="h-full w-full"></canvas>
                  </div>
                </div>
                <!-- CPT-α / CPS-α 副图：并列占下半区 -->
                <div class="flex flex-[0.85] gap-3 min-h-0">
                  <div class="flex-1 flex flex-col rounded-xl border border-[var(--border-default)] bg-[var(--bg-panel)] p-3 shadow-[var(--shadow-panel)] min-h-0">
                    <h3 class="mb-1 text-xs font-semibold text-[var(--text-muted)] flex-shrink-0">{{ t.fh_cptCurve }}</h3>
                    <div class="flex-1 min-h-0">
                      <canvas :ref="setCptAlphaCanvasRef" class="h-full w-full"></canvas>
                    </div>
                  </div>
                  <div class="flex-1 flex flex-col rounded-xl border border-[var(--border-default)] bg-[var(--bg-panel)] p-3 shadow-[var(--shadow-panel)] min-h-0">
                    <h3 class="mb-1 text-xs font-semibold text-[var(--text-muted)] flex-shrink-0">{{ t.fh_cpsCurve }}</h3>
                    <div class="flex-1 min-h-0">
                      <canvas :ref="setCpsAlphaCanvasRef" class="h-full w-full"></canvas>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <!-- 图表 Tab：放大查看三图表，Kα-Kβ 占上半，CPT-α / CPS-α 并列占下半 -->
          <div v-if="activeTab === 'chart'" class="flex-1 overflow-hidden p-4">
            <div class="flex h-full flex-col gap-3 min-h-0">
              <div class="flex-1 flex flex-col rounded-xl border border-[var(--border-default)] bg-[var(--bg-panel)] p-3 shadow-[var(--shadow-panel)] min-h-0">
                <h3 class="mb-2 text-sm font-semibold text-[var(--text-primary)] flex-shrink-0">{{ t.fh_kAlphaKbetaSpace }}</h3>
                <div class="flex-1 min-h-0">
                  <canvas ref="kAlphaKbetaCanvas" class="h-full w-full"></canvas>
                </div>
              </div>
              <div class="flex flex-[0.85] gap-3 min-h-0">
                <div class="flex-1 flex flex-col rounded-xl border border-[var(--border-default)] bg-[var(--bg-panel)] p-3 shadow-[var(--shadow-panel)] min-h-0">
                  <h3 class="mb-2 text-sm font-semibold text-[var(--text-primary)] flex-shrink-0">{{ t.fh_cptCurve }}</h3>
                  <div class="flex-1 min-h-0">
                    <canvas :ref="setCptAlphaCanvasRef" class="h-full w-full"></canvas>
                  </div>
                </div>
                <div class="flex-1 flex flex-col rounded-xl border border-[var(--border-default)] bg-[var(--bg-panel)] p-3 shadow-[var(--shadow-panel)] min-h-0">
                  <h3 class="mb-2 text-sm font-semibold text-[var(--text-primary)] flex-shrink-0">{{ t.fh_cpsCurve }}</h3>
                  <div class="flex-1 min-h-0">
                    <canvas :ref="setCpsAlphaCanvasRef" class="h-full w-full"></canvas>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <!-- 数据 Tab：完整数据表 -->
          <div v-if="activeTab === 'data'" class="flex-1 overflow-auto p-4">
            <div class="rounded-xl border border-[var(--border-default)] bg-[var(--bg-panel)] p-4 shadow-[var(--shadow-panel)]">
              <div class="mb-3 flex items-center gap-2">
                <FileText class="h-5 w-5 text-[var(--accent-primary)]" />
                <h3 class="text-base font-semibold text-[var(--text-primary)]">{{ t.fh_calibrationDataRecords }}</h3>
                <span class="ml-auto text-sm text-[var(--text-muted)]">{{ recordCountText }}</span>
              </div>
              <div class="overflow-auto">
                <table class="w-full text-sm">
                  <thead class="bg-[var(--bg-panel-strong)]">
                    <tr>
                      <th class="px-3 py-2 text-left text-xs font-medium text-[var(--text-muted)]">{{ t.fh_sequenceNumber }}</th>
                      <th class="px-3 py-2 text-left text-xs font-medium text-[var(--text-muted)]">α (°)</th>
                      <th class="px-3 py-2 text-left text-xs font-medium text-[var(--text-muted)]">β (°)</th>
                      <th class="px-3 py-2 text-left text-xs font-medium text-[var(--text-muted)]">Kα</th>
                      <th class="px-3 py-2 text-left text-xs font-medium text-[var(--text-muted)]">Kβ</th>
                      <th class="px-3 py-2 text-left text-xs font-medium text-[var(--text-muted)]">CPT</th>
                      <th class="px-3 py-2 text-left text-xs font-medium text-[var(--text-muted)]">CPS</th>
                      <th class="px-3 py-2 text-left text-xs font-medium text-[var(--text-muted)]">{{ t.fh_machMaHeader }}</th>
                      <th class="px-3 py-2 text-left text-xs font-medium text-[var(--text-muted)]">{{ t.samples }}</th>
                      <th class="px-3 py-2 text-left text-xs font-medium text-[var(--text-muted)]">{{ t.fh_stdDev }}</th>
                    </tr>
                  </thead>
                  <tbody class="divide-y divide-[var(--border-default)]">
                    <tr v-for="(point, index) in fiveHolePoints" :key="index" class="hover:bg-[var(--bg-panel-strong)]">
                      <td class="px-3 py-2 font-mono text-[var(--text-primary)]">{{ index + 1 }}</td>
                      <td class="px-3 py-2 font-mono text-[var(--text-primary)]">{{ formatValue(point.coordinates['α'], 1) }}</td>
                      <td class="px-3 py-2 font-mono text-[var(--text-primary)]">{{ formatValue(point.coordinates['β'], 1) }}</td>
                      <td class="px-3 py-2 font-mono text-[var(--text-primary)]">{{ formatValue(point.coefficients.Kalpha, 4) }}</td>
                      <td class="px-3 py-2 font-mono text-[var(--text-primary)]">{{ formatValue(point.coefficients.Kbeta, 4) }}</td>
                      <td class="px-3 py-2 font-mono text-[var(--text-primary)]">{{ formatValue(point.coefficients.CPT, 4) }}</td>
                      <td class="px-3 py-2 font-mono text-[var(--text-primary)]">{{ formatValue(point.coefficients.CPS, 4) }}</td>
                      <td class="px-3 py-2 font-mono text-[var(--text-primary)]">{{ point.coefficients.machNumber !== undefined ? point.coefficients.machNumber.toFixed(3) : '--' }}</td>
                      <td class="px-3 py-2 font-mono text-[var(--text-primary)]">{{ 'sampleCount' in point ? point.sampleCount : '--' }}</td>
                      <td class="px-3 py-2 font-mono text-[var(--text-primary)]">{{ formatValue(point.stdDev, 4) }}</td>
                    </tr>
                  </tbody>
                </table>
                <div v-if="fiveHolePoints.length === 0" class="py-8 text-center text-sm text-[var(--text-muted)]">{{ t.fh_noDataRecords }}</div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>

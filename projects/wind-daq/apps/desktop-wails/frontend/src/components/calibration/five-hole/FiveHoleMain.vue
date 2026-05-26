<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount, watch, nextTick, type ComponentPublicInstance } from 'vue'
import { useCalibrationWorkflow } from '@composables/useCalibrationWorkflow'
import { deviceApi } from '@api/deviceApi'
import type { CalibrationConfig, CalibrationDataPoint, ProbeChannelRole } from '@shared/types/calibration'
import type { DataPayload } from '@api/types'
import { isFiveHoleDataPoint } from '@shared/calibrationDataGuards'
import { getDerivedValuePrecision, getProbeChannelPrecision } from '@shared/calibrationPrecision'
import { formatFiveHoleActualPosition, generateFiveHoleSnakePoints } from './motionCalibrationUtils'
import { drawFiveHoleChartScaffold, resolveKAlphaKbetaBounds, drawNoDataHint, drawAxisTicks, setupCanvas, CHART_COLORS } from './fiveHoleChartUtils'
import { ArrowLeft, Settings, Play, Pause, StopCircle, RefreshCcw, Activity, Save, FileText, Clock, Navigation2, Maximize2, Minimize2 } from '@lucide/vue'
import IconCalibrationFiveHole from '@components/icons/IconCalibrationFiveHole.vue'

const emit = defineEmits<{
  openSettings: []
  back: []
}>()

const {
  calibrationStore,
  deviceStore,
  motionStore,
  feedbackStore,
  isLoading,
  hasConfig,
  currentConfig,
  sphereTankGate,
  loadSavedConfig,
  pauseCalibration,
  resumeCalibration,
  stopCalibration,
  saveCsv,
  exportReport,
  saveSphereTankGate,
  progressInfo,
  formattedTimeInfo,
  isAcquisitionDeviceConnected,
  isMotionControllerConnected,
  configuredDeviceNames,
  configuredControllerNames,
  statusText,
  statusColor,
} = useCalibrationWorkflow('five-hole')

defineExpose({
  reloadSavedConfig: loadSavedConfig
})

// 主图表展开状态
const isTopChartExpanded = ref(false)

// 组件挂载时加载数据
let unsubscribeDaqSnapshot: (() => void) | null = null
let unsubscribeDeviceStatus: (() => void) | null = null
let unsubscribeMotionStatus: (() => void) | null = null
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
  motionStatusPollTimer = setInterval(() => {
    void motionStore.refreshStatus()
  }, 300)
}, { immediate: true })

// 开始校准 (五孔特化：自动生成 snake 点)
async function startCalibration() {
  if (!canStartCalibration.value || !currentConfig.value) {
    feedbackStore.pushToast(startDisabledReason.value || '请先完成校准前检查', 'warning')
    return
  }
  try {
    const configToStart: CalibrationConfig = {
      ...currentConfig.value,
      points: currentConfig.value.fiveHoleLayout
        ? generateFiveHoleSnakePoints(currentConfig.value.fiveHoleLayout)
        : currentConfig.value.points,
    }
    await calibrationStore.startCalibration(configToStart)
  } catch (err) {
    console.error('Failed to start calibration:', err)
    feedbackStore.pushToast('启动校准失败: ' + (err instanceof Error ? err.message : String(err)), 'error')
  }
}

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
  [/总压/, /total pressure/, /(?:^|[^a-z0-9])p0(?:[^a-z0-9]|$)/, /(?:^|[^a-z0-9])pt(?:[^a-z0-9]|$)/]
))

const hasWindTunnelStaticPressureChannel = computed(() => hasConfiguredProbeChannel(
  ['fiveHole.pTunnelStatic'],
  [/静压/, /static pressure/, /(?:^|[^a-z0-9])ps(?:[^a-z0-9]|$)/]
))

const hasWindTunnelTemperatureChannel = computed(() => hasConfiguredProbeChannel(
  ['fiveHole.tTunnel'],
  [/风洞.*温/, /tunnel temp/, /tunnel temperature/, /(?:^|[^a-z0-9])ttunnel(?:[^a-z0-9]|$)/]
))

const hasRequiredWindTunnelChannels = computed(() => {
  return hasWindTunnelTotalPressureChannel.value
    && hasWindTunnelStaticPressureChannel.value
    && hasWindTunnelTemperatureChannel.value
})

const canStartCalibration = computed(() => {
  return hasConfig.value &&
    !isLoading.value &&
    hasRequiredWindTunnelChannels.value &&
    isAcquisitionDeviceConnected.value &&
    isMotionControllerConnected.value
})

const startDisabledReason = computed(() => {
  if (isLoading.value) return '正在加载配置，请稍候'
  if (!hasConfig.value) return '请先完成校准配置'
  if (!hasRequiredWindTunnelChannels.value) return '请配置风洞总压、风洞静压、风洞温度通道'
  if (!isAcquisitionDeviceConnected.value) return '采集设备未连接'
  if (!isMotionControllerConnected.value) return '运动控制器未连接'
  return ''
})

const actualMotionPosition = computed(() => {
  return formatFiveHoleActualPosition(currentConfig.value, motionStore.statusList)
})

const targetPointText = computed(() => {
  const coordinates = calibrationStore.status?.currentPoint?.coordinates
  if (coordinates) {
    const alpha = typeof coordinates['α'] === 'number' ? coordinates['α'] : 0
    const beta = typeof coordinates['β'] === 'number' ? coordinates['β'] : 0
    return `α=${alpha.toFixed(1)}°, β=${beta.toFixed(1)}°`
  }
  return 'α=0.0°, β=0.0°'
})

function getFiveHoleProbeRole(index: number): ProbeChannelRole {
  switch (index) {
    case 1: return 'fiveHole.p1'
    case 2: return 'fiveHole.p2'
    case 3: return 'fiveHole.p3'
    default: return 'fiveHole.p4'
  }
}

function getFiveHoleProbeValue(index: number): number | undefined {
  const pressures = calibrationStore.realtimePressures
  switch (index) {
    case 1: return pressures?.P1
    case 2: return pressures?.P2
    case 3: return pressures?.P3
    default: return pressures?.P4
  }
}

function formatFiveHoleRealtimeValue(
  role: ProbeChannelRole,
  value: number | undefined,
  unit = ''
): string {
  if (typeof value !== 'number' || !Number.isFinite(value)) {
    return '--'
  }

  const precision = getProbeChannelPrecision(currentConfig.value, role)
  const formatted = value.toFixed(precision)
  return unit ? `${formatted} ${unit}` : formatted
}

function formatFiveHoleDerivedValue(key: 'machNumber' | 'velocity', value: number | undefined): string {
  if (typeof value !== 'number' || !Number.isFinite(value)) {
    return '--'
  }

  return value.toFixed(getDerivedValuePrecision(currentConfig.value, key))
}

function buildRealtimePressuresFromSnapshots(
  config: CalibrationConfig,
  snapshots: DataPayload[]
): import('@stores/calibrationStore').RealtimePressures | null {
  const toValue = (deviceId: string, channelIndex: number): number | null => {
    const payload = snapshots.find((p) => p.deviceId === deviceId)
    if (!payload) return null
    const indices = Array.isArray(payload.channelIndices) ? payload.channelIndices : []
    const channels = Array.isArray(payload.channels) ? payload.channels : []
    const i = indices.indexOf(channelIndex)
    if (i === -1) return null
    const v = channels[i]
    return typeof v === 'number' ? v : null
  }

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
      case 'fiveHole.tAtm':
        result.Tatm = value
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
        matchedChannelCount += 1
        break
      default:
        break
    }
  }

  for (const ch of config.probeChannels) {
    if (!ch.enabled) continue
    const rawValue = toValue(ch.channel.deviceId, ch.channel.channelIndex)
    if (rawValue === null) continue
    const value = deviceStore.getDisplayValue(ch.channel.deviceId, ch.channel.channelIndex, rawValue)

    if (ch.role) {
      assignByRole(ch.role as ProbeChannelRole, value)
      continue
    }

    const name = ch.name.trim()
    const normalizedName = name.toLowerCase()
    if (/(?:^|[^a-z0-9])p1(?:[^a-z0-9]|$)/.test(normalizedName) || /孔\s*1/.test(name) || name.includes('孔1压力')) {
      result.P1 = value
      matchedChannelCount += 1
    } else if (/(?:^|[^a-z0-9])p2(?:[^a-z0-9]|$)/.test(normalizedName) || /孔\s*2/.test(name) || name.includes('孔2压力')) {
      result.P2 = value
      matchedChannelCount += 1
    } else if (/(?:^|[^a-z0-9])p3(?:[^a-z0-9]|$)/.test(normalizedName) || /孔\s*3/.test(name) || name.includes('孔3压力')) {
      result.P3 = value
      matchedChannelCount += 1
    } else if (/(?:^|[^a-z0-9])p4(?:[^a-z0-9]|$)/.test(normalizedName) || /孔\s*4/.test(name) || name.includes('孔4压力')) {
      result.P4 = value
      matchedChannelCount += 1
    } else if (/(?:^|[^a-z0-9])p5(?:[^a-z0-9]|$)/.test(normalizedName) || /孔\s*5/.test(name) || name.includes('孔5压力')) {
      result.P5 = value
      matchedChannelCount += 1
    } else if (/大气.*压/.test(name) || normalizedName.includes('atmospheric pressure') || /(?:^|[^a-z0-9])patm(?:[^a-z0-9]|$)/.test(normalizedName) || normalizedName.includes('p∞')) {
      result.Patm = value
      matchedChannelCount += 1
    } else if (/大气.*温/.test(name) || normalizedName.includes('atmospheric temp') || normalizedName.includes('atmospheric temperature') || /(?:^|[^a-z0-9])tatm(?:[^a-z0-9]|$)/.test(normalizedName) || normalizedName.includes('t∞')) {
      result.Tatm = value
      matchedChannelCount += 1
    } else if (/总压/.test(name) || normalizedName.includes('total pressure') || /(?:^|[^a-z0-9])p0(?:[^a-z0-9]|$)/.test(normalizedName) || /(?:^|[^a-z0-9])pt(?:[^a-z0-9]|$)/.test(normalizedName)) {
      result.P0 = value
      matchedChannelCount += 1
    } else if (/静压/.test(name) || normalizedName.includes('static pressure') || /(?:^|[^a-z0-9])ps(?:[^a-z0-9]|$)/.test(normalizedName)) {
      result.Ps = value
      matchedChannelCount += 1
    } else if (/风洞.*温/.test(name) || normalizedName.includes('tunnel temp') || normalizedName.includes('tunnel temperature') || /(?:^|[^a-z0-9])ttunnel(?:[^a-z0-9]|$)/.test(normalizedName)) {
      result.Ttunnel = value
      matchedChannelCount += 1
    }
  }

  if (matchedChannelCount === 0) {
    return null
  }

  return result
}

function groupByBeta(
  points: CalibrationDataPoint[],
  coefficient: 'CPT' | 'CPS'
) {
  const groups = new Map<number, { alpha: number; value: number }[]>()
  points.forEach(p => {
    const beta = p.coordinates['β'] as number
    const alpha = p.coordinates['α'] as number
    const value = coefficient === 'CPT' ? p.coefficients.CPT : p.coefficients.CPS
    if (!groups.has(beta)) groups.set(beta, [])
    groups.get(beta)!.push({ alpha, value })
  })
  groups.forEach(group => group.sort((a, b) => a.alpha - b.alpha))
  return Array.from(groups.entries()).map(([beta, data]) => ({ beta, data }))
}

function groupByAlpha(
  points: CalibrationDataPoint[]
) {
  const groups = new Map<number, { beta: number; Kalpha: number; Kbeta: number; pointId: number }[]>()
  points.forEach(p => {
    const alpha = p.coordinates['α'] as number
    const beta = p.coordinates['β'] as number
    if (!groups.has(alpha)) groups.set(alpha, [])
    groups.get(alpha)!.push({ beta, Kalpha: p.coefficients.Kalpha, Kbeta: p.coefficients.Kbeta, pointId: p.pointId })
  })
  groups.forEach(group => group.sort((a, b) => a.beta - b.beta))
  return Array.from(groups.entries()).map(([alpha, data]) => ({ alpha, data }))
}

function groupByBetaKb(
  points: CalibrationDataPoint[]
) {
  const groups = new Map<number, { alpha: number; Kalpha: number; Kbeta: number }[]>()
  points.forEach(p => {
    const beta = p.coordinates['β'] as number
    const alpha = p.coordinates['α'] as number
    if (!groups.has(beta)) groups.set(beta, [])
    groups.get(beta)!.push({ alpha, Kalpha: p.coefficients.Kalpha, Kbeta: p.coefficients.Kbeta })
  })
  groups.forEach(group => group.sort((a, b) => a.alpha - b.alpha))
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
    const scatterData = points.map(p => ({ x: p.coefficients.Kalpha, y: p.coefficients.Kbeta }))
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
  betaGroups.forEach(group => {
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
    group.data.forEach(pt => {
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
    group.data.forEach(pt => {
      ctx.beginPath()
      ctx.arc(toX(pt.Kalpha), toY(pt.Kbeta), 2.5, 0, 2 * Math.PI)
      ctx.fill()
    })
  })

  // 当前采集点高亮
  const currentPointId = calibrationStore.status?.currentPoint?.id
  if (currentPointId != null) {
    const cp = points.find(p => p.pointId === currentPointId)
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

  const allValues = data.flatMap(g => g.data.map(d => d.value))
  const xMin = Math.min(...data.flatMap(g => g.data.map(d => d.alpha)))
  const xMax = Math.max(...data.flatMap(g => g.data.map(d => d.alpha)))
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

  const allValues = data.flatMap(g => g.data.map(d => d.value))
  const xMin = Math.min(...data.flatMap(g => g.data.map(d => d.alpha)))
  const xMax = Math.max(...data.flatMap(g => g.data.map(d => d.alpha)))
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

// 更新图表
function updateCharts() {
  drawKAlphaKbetaChart()
  if (!isTopChartExpanded.value) {
    drawCptAlphaChart()
    drawCpsAlphaChart()
  }
}

let chartTimer: ReturnType<typeof setInterval> | null = null
function startChartTimer(): void {
  if (chartTimer) clearInterval(chartTimer)
  chartTimer = setInterval(updateCharts, calibrationStore.uiRefreshIntervalMs)
}

onMounted(() => {
  startChartTimer()
})

watch(
  () => calibrationStore.uiRefreshIntervalMs,
  () => {
    startChartTimer()
  }
)

watch(isTopChartExpanded, () => {
  nextTick(updateCharts)
})

onBeforeUnmount(() => {
  if (chartTimer) clearInterval(chartTimer)
  cleanupSubscriptions()
  calibrationStore.reset()
})
</script>

<template>
  <div class="h-full flex flex-col bg-[var(--bg-canvas)] text-[var(--text-primary)]">
    <!-- 顶部工具栏 -->
    <div data-test="five-hole-top-header" class="flex shrink-0 items-center justify-between border-b border-[var(--border-default)] bg-[var(--bg-panel)] px-4 py-3">
      <div class="flex items-center gap-4">
        <button
          @click="emit('back')"
          class="rounded-[var(--radius-sm)] p-2 text-[var(--text-secondary)] transition-colors hover:bg-[var(--bg-panel-strong)] hover:text-[var(--text-primary)]"
        >
          <ArrowLeft class="h-5 w-5" />
        </button>
        <div class="flex items-center gap-3">
          <div class="flex h-10 w-10 items-center justify-center rounded-xl bg-[var(--accent-primary)]/10 text-[var(--accent-primary)]">
            <IconCalibrationFiveHole :size="20" />
          </div>
          <div>
              <h1 class="text-lg font-bold text-[var(--text-primary)]">五孔探针校准</h1>
            <div class="flex items-center gap-3">
              <div class="flex items-center gap-1.5">
                <span class="h-2 w-2 rounded-full" :class="statusColor === 'success' ? 'bg-[var(--accent-success)] shadow-[0_0_6px_var(--accent-success)]' : 'bg-[var(--text-muted)]'"></span>
                <p class="text-xs text-[var(--text-muted)]">{{ statusText }}</p>
              </div>
              <div class="h-3 w-px bg-[var(--border-default)]"></div>
              <div class="flex items-center gap-1.5" :title="isAcquisitionDeviceConnected ? '采集设备已连接' : '采集设备未连接'">
                <span class="h-2 w-2 rounded-full" :class="isAcquisitionDeviceConnected ? 'bg-[var(--accent-success)] shadow-[0_0_6px_var(--accent-success)]' : 'bg-[var(--text-muted)]'"></span>
                <span class="text-xs text-[var(--text-muted)]">采集</span>
              </div>
              <div class="flex items-center gap-1.5" :title="isMotionControllerConnected ? '位移机构已连接' : '位移机构未连接'">
                <span class="h-2 w-2 rounded-full" :class="isMotionControllerConnected ? 'bg-[var(--accent-success)] shadow-[0_0_6px_var(--accent-success)]' : 'bg-[var(--text-muted)]'"></span>
                <span class="text-xs text-[var(--text-muted)]">位移</span>
              </div>
            </div>
          </div>
        </div>
      </div>

      <div class="flex items-center gap-3">
        <!-- 频率选择 -->
        <div class="flex items-center gap-2">
            <span class="text-xs text-[var(--text-muted)] whitespace-nowrap">刷新</span>
            <select
              :value="calibrationStore.uiRefreshHz"
              @change="calibrationStore.setUiRefreshHz(Number(($event.target as HTMLSelectElement).value))"
              class="rounded-[var(--radius-sm)] border border-[var(--border-default)] bg-[var(--bg-panel-strong)] px-2 py-1 text-xs text-[var(--text-primary)] outline-none"
          >
            <option v-for="hz in 20" :key="hz" :value="hz">{{ hz }} Hz</option>
          </select>
        </div>

        <!-- 球罐判定 -->
        <div class="flex items-center gap-2 rounded-[var(--radius-sm)] border border-[var(--border-default)] bg-[var(--bg-panel-strong)] px-3 py-1.5 text-xs">
          <label class="flex items-center gap-2 cursor-pointer text-[var(--text-primary)]">
              <input
                :checked="sphereTankGate.gateEnabled.value"
                type="checkbox"
                class="h-4 w-4 cursor-pointer"
                @change="saveSphereTankGate(($event.target as HTMLInputElement).checked, sphereTankGate.waitTimeSec.value)"
              >
              <span>球罐判定</span>
            </label>
            <span class="text-[var(--text-muted)]">稳定</span>
            <span class="font-mono font-bold text-[var(--accent-primary)]">{{ sphereTankGate.stableTimeSec.value === null ? '--' : `${sphereTankGate.stableTimeSec.value.toFixed(1)}s` }}</span>
            <span class="rounded-full px-2 py-0.5 text-[10px] font-semibold" :class="sphereTankGate.statusText.value === '稳定' ? 'bg-[var(--accent-success)]/10 text-[var(--accent-success)]' : 'bg-[var(--accent-warning)]/10 text-[var(--accent-warning)]'">{{ sphereTankGate.statusText.value }}</span>
        </div>

        <div class="h-6 w-px bg-[var(--border-default)]"></div>

        <!-- 配置按钮 -->
        <button
          @click="emit('openSettings')"
          class="flex items-center gap-1.5 rounded-[var(--radius-sm)] border border-[var(--border-default)] bg-[var(--bg-panel-strong)] px-3 py-1.5 text-xs text-[var(--text-primary)] transition-colors hover:bg-[var(--bg-canvas)]"
        >
          <Settings class="h-4 w-4" />
          配置
        </button>

        <!-- 主控制组 -->
        <template v-if="!calibrationStore.isRunning && !calibrationStore.isPaused">
          <button
            @click="startCalibration"
            :disabled="!canStartCalibration"
            class="flex items-center gap-1.5 rounded-[var(--radius-sm)] bg-[var(--accent-primary)] px-4 py-1.5 text-xs text-white transition-colors hover:bg-[color:color-mix(in_srgb,var(--accent-primary)_92%,black_8%)] disabled:cursor-not-allowed disabled:opacity-50"
          >
            <Play class="h-4 w-4" />
            开始校准
          </button>
        </template>

        <template v-else>
          <button
            v-if="!calibrationStore.isPaused"
            @click="pauseCalibration"
            class="rounded-[var(--radius-sm)] bg-[var(--accent-warning)] px-3 py-1.5 text-xs text-white transition-colors hover:bg-[color:color-mix(in_srgb,var(--accent-warning)_92%,black_8%)]"
          >
            暂停
          </button>
          <button
            v-else
            @click="resumeCalibration"
            class="rounded-[var(--radius-sm)] bg-[var(--accent-primary)] px-3 py-1.5 text-xs text-white transition-colors hover:bg-[color:color-mix(in_srgb,var(--accent-primary)_92%,black_8%)]"
          >
            继续
          </button>
          <div class="h-6 w-px bg-[color:color-mix(in_srgb,var(--accent-danger)_35%,var(--border-default))]"></div>
          <button
            @click="stopCalibration"
            class="rounded-[var(--radius-sm)] border border-[color:color-mix(in_srgb,var(--accent-danger)_35%,var(--border-default))] bg-[var(--accent-danger)] px-3 py-1.5 text-xs text-white transition-colors hover:bg-[color:color-mix(in_srgb,var(--accent-danger)_92%,black_8%)]"
          >
            停止
          </button>
        </template>
      </div>
    </div>

    <!-- 加载状态 -->
    <div v-if="isLoading" class="flex-1 flex items-center justify-center">
      <div class="text-center">
        <div class="animate-spin w-8 h-8 border-2 border-purple-500 border-t-transparent rounded-full mx-auto mb-4"></div>
        <p class="text-[var(--text-muted)]">正在加载...</p>
      </div>
    </div>

    <!-- 主内容区 -->
    <template v-else>
      <div class="flex-1 overflow-hidden p-4">
        <div class="grid h-full grid-cols-[280px_1fr] gap-4" data-test="five-hole-layout-root">
          <aside data-test="five-hole-left-sidebar" class="flex min-h-0 flex-col gap-4 overflow-hidden">
            <!-- 运行摘要卡片 -->
            <section class="rounded-[var(--radius-md)] border border-[var(--border-default)] bg-[var(--bg-panel)] p-4 shadow-[var(--shadow-panel)]">
              <div class="mb-3 flex items-center justify-between">
                <span class="text-xs font-semibold uppercase tracking-wider text-[var(--text-muted)]">运行摘要</span>
                <span class="rounded-full bg-[var(--accent-primary)]/10 px-2 py-0.5 text-[10px] font-semibold text-[var(--accent-primary)]">{{ statusText }}</span>
              </div>
              <div class="space-y-3">
                <div class="grid grid-cols-2 gap-2">
                  <div class="rounded-[var(--radius-sm)] border border-[var(--border-default)] bg-[var(--bg-panel-strong)] p-3 text-center">
                    <p class="mb-1 text-[10px] uppercase text-[var(--text-muted)]">完成度</p>
                    <p class="font-mono text-lg font-bold text-[var(--accent-primary)]">{{ progressInfo ? `${progressInfo.percent}%` : '0%' }}</p>
                  </div>
                  <div class="rounded-[var(--radius-sm)] border border-[var(--border-default)] bg-[var(--bg-panel-strong)] p-3 text-center">
                    <p class="mb-1 text-[10px] uppercase text-[var(--text-muted)]">已用时</p>
                    <p class="font-mono text-lg font-bold text-[var(--text-primary)]">{{ formattedTimeInfo?.elapsed || '00:00' }}</p>
                  </div>
                </div>
                <div class="flex items-center justify-between rounded-[var(--radius-sm)] bg-[var(--bg-panel-strong)] p-2">
                  <div class="flex flex-col">
                    <span class="text-[10px] text-[var(--text-muted)]">攻角 α</span>
                    <span class="font-mono text-sm font-semibold text-[var(--accent-primary)]">{{ calibrationStore.angleInfo?.alpha?.toFixed(2) ?? '0.00' }}°</span>
                  </div>
                  <div class="flex flex-col text-right">
                    <span class="text-[10px] text-[var(--text-muted)]">侧滑角 β</span>
                    <span class="font-mono text-sm font-semibold text-[var(--accent-primary)]">{{ calibrationStore.angleInfo?.beta?.toFixed(2) ?? '0.00' }}°</span>
                  </div>
                </div>
              </div>
            </section>

            <!-- 压力矩阵 -->
            <div class="rounded-[var(--radius-md)] border border-[var(--border-default)] bg-[var(--bg-panel)] p-4 shadow-[var(--shadow-panel)]">
              <div class="mb-3 flex items-center gap-2">
                <Activity class="h-4 w-4 text-[var(--accent-success)]" />
                <h3 class="text-xs font-semibold uppercase tracking-wider text-[var(--text-muted)]">五孔压力 (Pa)</h3>
              </div>
              <div class="grid grid-cols-2 gap-2">
                <div v-for="i in [1,2,3,4] as const" :key="i" class="flex flex-col rounded-[var(--radius-sm)] border border-[var(--border-default)] bg-[var(--bg-panel-strong)] p-2">
                  <span class="text-[10px] font-medium text-[var(--text-muted)]">P{{i}}</span>
                  <span class="font-mono text-sm font-semibold text-[var(--text-primary)]">
                    {{ formatFiveHoleRealtimeValue(getFiveHoleProbeRole(i), getFiveHoleProbeValue(i)) }}
                  </span>
                </div>
                <div class="flex flex-col rounded-[var(--radius-sm)] border border-[var(--accent-primary)]/20 bg-[var(--accent-primary)]/5 p-2">
                  <div class="flex items-center justify-between">
                    <span class="text-[10px] font-medium text-[var(--accent-primary)]">P5 (参考)</span>
                  </div>
                  <span class="font-mono text-base font-semibold text-[var(--accent-primary)]">
                    {{ formatFiveHoleRealtimeValue('fiveHole.p5', calibrationStore.realtimePressures?.P5) }}
                  </span>
                </div>
                <div class="flex flex-col rounded-[var(--radius-sm)] border border-[var(--accent-info)]/25 bg-[var(--accent-info)]/8 p-2">
                  <div class="flex items-center justify-between">
                    <span class="text-[10px] font-medium text-[var(--accent-info)]">风洞总压 Pt</span>
                  </div>
                  <span class="font-mono text-base font-semibold text-[var(--accent-info)]">
                    {{ formatFiveHoleRealtimeValue('fiveHole.pTotal', calibrationStore.realtimePressures?.P0) }}
                  </span>
                </div>
                <div class="flex flex-col rounded-[var(--radius-sm)] border border-[var(--accent-warning)]/25 bg-[var(--accent-warning)]/8 p-2">
                  <div class="flex items-center justify-between">
                    <span class="text-[10px] font-medium text-[var(--accent-warning)]">风洞静压 Ps</span>
                  </div>
                  <span class="font-mono text-base font-semibold text-[var(--accent-warning)]">
                    {{ formatFiveHoleRealtimeValue('fiveHole.pTunnelStatic', calibrationStore.realtimePressures?.Ps) }}
                  </span>
                </div>
                <div class="flex flex-col rounded-[var(--radius-sm)] border border-[var(--accent-success)]/25 bg-[var(--accent-success)]/8 p-2">
                  <span class="text-[10px] font-medium text-[var(--accent-success)]">风洞温度 Ttunnel</span>
                  <span class="font-mono text-base font-semibold text-[var(--accent-success)]">
                    {{ formatFiveHoleRealtimeValue('fiveHole.tTunnel', calibrationStore.realtimePressures?.Ttunnel, '°C') }}
                  </span>
                </div>
                <div class="flex flex-col rounded-[var(--radius-sm)] border border-[var(--border-default)] bg-[var(--bg-panel-strong)] p-2">
                  <span class="text-[10px] font-medium text-[var(--text-muted)]">大气压 Patm</span>
                  <span class="font-mono text-sm font-semibold text-[var(--text-primary)]">
                    {{ formatFiveHoleRealtimeValue('fiveHole.pAtm', calibrationStore.realtimePressures?.Patm) }}
                  </span>
                </div>
                <div class="flex flex-col rounded-[var(--radius-sm)] border border-[var(--border-default)] bg-[var(--bg-panel-strong)] p-2">
                  <span class="text-[10px] font-medium text-[var(--text-muted)]">大气温 Tatm</span>
                  <span class="font-mono text-sm font-semibold text-[var(--text-primary)]">
                    {{ formatFiveHoleRealtimeValue('fiveHole.tAtm', calibrationStore.realtimePressures?.Tatm, '°C') }}
                  </span>
                </div>
              </div>
            </div>

            <!-- 物理计算卡片 -->
            <div class="rounded-[var(--radius-md)] border border-[var(--border-default)] bg-[var(--bg-panel)] p-4 shadow-[var(--shadow-panel)]">
              <div class="mb-3 flex items-center gap-2">
                <Navigation2 class="h-4 w-4 text-[var(--accent-info)]" />
                <h3 class="text-xs font-semibold uppercase tracking-wider text-[var(--text-muted)]">气动参数</h3>
              </div>
              <div class="grid grid-cols-2 gap-2">
                <div class="flex flex-col rounded-[var(--radius-sm)] border border-[var(--border-default)] bg-[var(--bg-panel-strong)] p-2">
                  <span class="text-[10px] text-[var(--text-muted)]">马赫数 Ma</span>
                  <span class="font-mono text-sm font-semibold text-[var(--text-primary)]">{{ formatFiveHoleDerivedValue('machNumber', calibrationStore.calculatedPhysics?.machNumber) }}</span>
                </div>
                <div class="flex flex-col rounded-[var(--radius-sm)] border border-[var(--border-default)] bg-[var(--bg-panel-strong)] p-2">
                  <span class="text-[10px] text-[var(--text-muted)]">流速 V (m/s)</span>
                  <span class="font-mono text-sm font-semibold text-[var(--text-primary)]">{{ formatFiveHoleDerivedValue('velocity', calibrationStore.calculatedPhysics?.velocity) }}</span>
                </div>
              </div>
            </div>
          </aside>

          <main data-test="five-hole-right-workspace" class="flex min-h-0 flex-col gap-4">
            <!-- 主绘图区：K-Alpha/Beta -->
            <div class="flex-1 min-h-0 rounded-[var(--radius-md)] border border-[var(--border-default)] bg-[var(--bg-panel)] shadow-[var(--shadow-panel)] overflow-hidden flex flex-col">
              <div class="flex items-center justify-between border-b border-[var(--border-default)] bg-[var(--bg-panel-strong)]/50 px-4 py-2">
                <div class="flex items-center gap-2">
                  <span class="h-3 w-1.5 rounded-sm bg-[var(--accent-primary)]"></span>
                  <h4 class="text-xs font-semibold uppercase tracking-wider text-[var(--text-secondary)]">Kα - Kβ 系数特征空间</h4>
                </div>
                <button
                  @click="isTopChartExpanded = !isTopChartExpanded"
                  class="rounded-[var(--radius-sm)] p-1.5 text-[var(--text-secondary)] transition-colors hover:bg-[var(--bg-panel-strong)] hover:text-[var(--text-primary)]"
                  :title="isTopChartExpanded ? '恢复默认视图' : '展开图表'"
                >
                  <Maximize2 v-if="!isTopChartExpanded" class="h-4 w-4" />
                  <Minimize2 v-else class="h-4 w-4" />
                </button>
              </div>
              <div data-test="k-alpha-chart-canvas-wrap" class="relative flex-1 min-h-0">
                <canvas ref="kAlphaKbetaCanvas" class="h-full w-full"></canvas>
              </div>
            </div>

            <!-- 次级曲线区 -->
            <div v-show="!isTopChartExpanded" class="grid min-h-0 flex-[0.85] grid-cols-2 gap-4">
              <div v-for="chart in [
                { id: 'cpt', label: 'CPT - α 恢复系数', color: 'bg-[var(--accent-success)]' },
                { id: 'cps', label: 'CPS - α 静态系数', color: 'bg-[var(--accent-warning)]' }
              ]" :key="chart.id" class="rounded-[var(--radius-md)] border border-[var(--border-default)] bg-[var(--bg-panel)] shadow-[var(--shadow-panel)] overflow-hidden flex flex-col">
                <div class="border-b border-[var(--border-default)] bg-[var(--bg-panel-strong)]/50 px-4 py-2">
                  <h4 class="flex items-center gap-2 text-xs font-semibold uppercase tracking-wider text-[var(--text-secondary)]">
                    <span class="h-3 w-1.5 rounded-sm" :class="chart.color"></span>
                    {{ chart.label }}
                  </h4>
                </div>
                <div class="flex-1 min-h-0">
                  <canvas :ref="chart.id === 'cpt' ? setCptAlphaCanvasRef : setCpsAlphaCanvasRef" class="h-full w-full"></canvas>
                </div>
              </div>
            </div>
          </main>
        </div>
      </div>

      <!-- 完成提示 -->
      <div
        v-if="calibrationStore.completeEvent"
        class="px-4 py-3 border-t border-[var(--border-default)]"
        :class="calibrationStore.completeEvent.success ? 'bg-[var(--accent-success)]/10' : 'bg-[var(--accent-danger)]/10'"
      >
        <div class="flex items-center gap-3">
          <svg
            v-if="calibrationStore.completeEvent.success"
            class="w-5 h-5 text-[var(--accent-success)]"
            fill="currentColor"
            viewBox="0 0 20 20"
          >
            <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clip-rule="evenodd" />
          </svg>
          <svg v-else class="w-5 h-5 text-[var(--accent-danger)]" fill="currentColor" viewBox="0 0 20 20">
            <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 001.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clip-rule="evenodd" />
          </svg>
          <div>
            <p class="text-sm font-medium" :class="calibrationStore.completeEvent.success ? 'text-[var(--accent-success)]' : 'text-[var(--accent-danger)]'">
              {{ calibrationStore.completeEvent.success ? '校准完成！' : '校准失败' }}
            </p>
            <p class="text-xs text-[var(--text-secondary)]">
              {{ calibrationStore.completeEvent.success
                ? `共采集 ${calibrationStore.completeEvent.totalPoints} 个点，耗时 ${(calibrationStore.completeEvent.duration / 1000).toFixed(1)} 秒`
                : calibrationStore.completeEvent.error
              }}
            </p>
          </div>

          <div v-if="calibrationStore.completeEvent.success" class="ml-auto flex items-center gap-2">
            <button
              @click="saveCsv"
              class="rounded-[var(--radius-sm)] border border-[var(--border-default)] bg-[var(--bg-panel)] px-3 py-1.5 text-xs text-[var(--text-primary)] transition-colors hover:bg-[var(--bg-panel-strong)]"
            >
              保存CSV
            </button>
            <button
              @click="exportReport"
              class="rounded-[var(--radius-sm)] bg-[var(--accent-primary)] px-3 py-1.5 text-xs text-white transition-colors hover:bg-[color:color-mix(in_srgb,var(--accent-primary)_92%,black_8%)]"
            >
              导出报告
            </button>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { useCalibrationStore } from '@stores/calibrationStore'
import { useDeviceStore } from '@stores/deviceStore'
import { useMotionStore } from '@stores/motionStore'
import { useFeedbackStore } from '@stores/feedbackStore'
import { useI18nStore } from '@stores/i18nStore'
import { useCalibrationWorkflow } from '@composables/useCalibrationWorkflow'
import type { ProbeChannelRole } from '@shared/types/calibration'
import { getProbeChannelPrecision } from '@shared/calibrationPrecision'
import { findChannelValue } from '@shared/calibrationSnapshotValue'
import { isTotalPressureDataPoint } from '@shared/calibrationDataGuards'
import { deviceApi } from '@api/deviceApi'
import type { DataPayload } from '@api/types'
import UiButton from '@components/ui/UiButton.vue'
import UiInputNumber from '@components/ui/UiInputNumber.vue'
import TotalPressureChart from './TotalPressureChart.vue'
import MotionSafetyAlertCard from '@components/shared/MotionSafetyAlertCard.vue'
import {
  Play, Pause, Square, Settings, ArrowLeft, Save, FileText,
  ChevronDown, ChevronUp, Gauge, TrendingUp, Target, Wind, Download
} from '@lucide/vue'


const emit = defineEmits<{
  back: []
  openSettings: []
}>()

const calibrationStore = useCalibrationStore()
const deviceStore = useDeviceStore()
const motionStore = useMotionStore()
const feedbackStore = useFeedbackStore()
const { t } = storeToRefs(useI18nStore())

const workflow = useCalibrationWorkflow('total-pressure')
const sphereTankGate = workflow.sphereTankGate

// 暴露 reloadSavedConfig 给父组件 CalibrationWindow：
// Settings 保存配置后父组件调 currentMainRef.reloadSavedConfig() 触发重新加载，
// 否则 currentConfig 仍是挂载时的旧值，canStartCalibration 不刷新，会一直提示"未配置"。
// 与 FiveHoleMain / ThreeHoleMain / TotalTemperatureMain 保持一致，
// 由 calibrationMainExpose.contract.test.ts 在编译期断言本暴露存在。
defineExpose({
  reloadSavedConfig: workflow.loadSavedConfig,
})

const isLoading = ref(true)
const activeTab = ref<'overview' | 'chart' | 'data'>('overview')
const showConfigSummary = ref(false)
const latestSnapshots = ref<Map<string, DataPayload>>(new Map())

// K-α 图表 ref，用于切换 Tab 后主动触发重绘（canvas 尺寸从 0 变为实际值）
const chartRef = ref<InstanceType<typeof TotalPressureChart> | null>(null)

// 图表 Y 轴范围/精度控制：用户在图表 Tab 工具条调整后立即应用到 Canvas 重绘。
// - chartYRangeOverride 为 null 表示自动模式（基于数据点 + 10% padding）
// - 用户输入 min/max 后点击"应用"才写入 chartYRangeOverride，避免输入过程中频繁校验
// - chartYPrecision 立即生效（无应用按钮），用户改完即看到刻度精度变化
// - 概览页与图表 Tab 共用同一组设置，保证两 Tab 视觉一致
// - chartYPrecisionInput 与 UiInputNumber 的 number | null 双向绑定兼容；
//   chartYPrecision 用 computed 兜底 null → 3，确保传给图表的 prop 始终是合法 number
const chartYRangeOverride = ref<{ min: number; max: number } | null>(null)
const chartYMinInput = ref<number | null>(null)
const chartYMaxInput = ref<number | null>(null)
const chartYPrecisionInput = ref<number | null>(3)
const chartYPrecision = computed<number>(() => {
  const v = chartYPrecisionInput.value
  if (v === null || !Number.isFinite(v) || v < 0) return 3
  return Math.min(6, Math.max(0, Math.floor(v)))
})

// 应用 Y 轴范围：校验 min < max 后写入 chartYRangeOverride 触发图表重绘。
// 无效输入（任一为空、min >= max）toast 警告，不修改当前生效范围。
function applyYRange(): void {
  const min = chartYMinInput.value
  const max = chartYMaxInput.value
  if (min === null || max === null) {
    feedbackStore.pushToast(t.value.tp_yRangeInvalidHint, 'warning')
    return
  }
  if (min >= max) {
    feedbackStore.pushToast(t.value.tp_yRangeMinGeMax, 'warning')
    return
  }
  chartYRangeOverride.value = { min, max }
}

// 重置为自动模式：清空 override 与输入框，图表回退到基于数据点的自动范围。
function resetYRangeToAuto(): void {
  chartYRangeOverride.value = null
  chartYMinInput.value = null
  chartYMaxInput.value = null
}

// 实时采集快照订阅：deviceApi.onSnapshot 推送设备通道原始数据，
// 由 buildRealtimePressuresFromSnapshots 映射到 RealtimePressures 喂给 store，
// 驱动 calculatedPhysics（马赫数/速度）实时计算。与 ThreeHoleMain.vue 模式一致。
let unsubscribeDaqSnapshot: (() => void) | null = null
let unsubscribeDeviceStatus: (() => void) | null = null
let unsubscribeMotionStatus: (() => void) | null = null
// 运动控制器状态轮询定时器：后端 Wails 事件推送频率不足以支撑"实际α"实时显示，
// 需 300ms 主动拉取 status（与 ThreeHoleMain.vue 保持一致），驱动 liveAxisPositions 更新。
let motionStatusPollTimer: ReturnType<typeof setInterval> | null = null

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

// buildRealtimePressuresFromSnapshots 按总压通道角色映射将设备快照转为 RealtimePressures
// 映射规则（与后端 formulas.go::CalculateTotalPressureCoefficients 的绝压转换对齐）：
//   pTunnelTotal → P0（风洞总压表压，Ma/V 公式的 Pt 来源）
//   pTunnelStatic → Ps（风洞静压表压，Ma/V 公式的 Ps 来源）
//   pAtm → Patm（大气压，差压转绝压用）
//   tTunnel → Ttunnel（风洞温度，优先；缺失时用 tAtm 兜底，与后端 TAT 选取逻辑一致）
//   tAtm → Tatm + Ttunnel（兜底：低速风洞下用大气温度近似风洞总温）
// 探针总压 pProbeTotal 不参与 Ma/V 计算，不映射到 RealtimePressures。
function buildRealtimePressuresFromSnapshots(
  config: import('@shared/types/calibration').CalibrationConfig,
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
    PprobeTotal: undefined as number | undefined,
  }
  let matchedChannelCount = 0
  let hasTunnelTemp = false

  for (const ch of config.probeChannels) {
    if (!ch.enabled) continue
    const rawValue = findChannelValue(snapshots, ch.channel.deviceId, ch.channel.channelIndex)
    if (rawValue === null) continue
    switch (ch.role as ProbeChannelRole) {
      case 'totalPressure.pAtm':
        result.Patm = rawValue
        matchedChannelCount += 1
        break
      case 'totalPressure.tAtm':
        result.Tatm = rawValue
        // 总压校准若无独立风洞温度传感器，用大气温度近似风洞总温（TAT），
        // 后端 AtmosphericDataCalculator 据此计算马赫数/速度（spec Task 15：前端公式已删除）。
        // 低速风洞下误差可接受。
        if (!hasTunnelTemp) result.Ttunnel = rawValue
        matchedChannelCount += 1
        break
      case 'totalPressure.tTunnel':
        result.Ttunnel = rawValue
        hasTunnelTemp = true
        matchedChannelCount += 1
        break
      case 'totalPressure.pTunnelTotal':
        result.P0 = rawValue
        matchedChannelCount += 1
        break
      case 'totalPressure.pTunnelStatic':
        result.Ps = rawValue
        matchedChannelCount += 1
        break
      case 'totalPressure.pProbeTotal':
        // 探针总压不参与 Ma/V 计算，但需在侧边栏显示，单独存 PprobeTotal
        result.PprobeTotal = rawValue
        matchedChannelCount += 1
        break
      default: break
    }
  }

  if (matchedChannelCount === 0) return null
  return result
}

const currentConfig = computed(() => workflow.currentConfig.value)
const progressInfo = computed(() => workflow.progressInfo.value)
const formattedTimeInfo = computed(() => workflow.formattedTimeInfo.value)
const canStartCalibration = computed(() => workflow.canStartCalibration.value)
const startDisabledReason = computed(() => workflow.startDisabledReason.value)

// 运动控制器实时轴位置：从 currentConfig.motionAxes 配置出发，
// 通过 motionStore.statusList 查对应控制器的 AxisStatus，取实时 position/moving。
// 总压探针通常配置单 α 轴，但按配置轴数通用渲染，避免硬编码。
interface LiveAxisPosition {
  name: string
  position: number | null
  moving: boolean
}
// 直接读 motionStore.statusList（ref），在 computed 内访问 .value 建立响应式依赖，
// 300ms 轮询 refreshStatus 更新 statusList 后自动重算。与 ThreeHoleMain 保持一致。
const liveAxisPositions = computed<LiveAxisPosition[]>(() => {
  const axes = currentConfig.value?.motionAxes ?? []
  const statusList = motionStore.statusList
  return axes.map((cfg) => {
    const status = statusList.find((s) => s.id === cfg.controllerId)
    const axisStatus = status?.axes.find((a) => a.name === cfg.axis)
    return {
      name: cfg.name,
      position: typeof axisStatus?.position === 'number' ? axisStatus.position : null,
      moving: axisStatus?.moving ?? false,
    }
  })
})

// 当前目标攻角：从 progressInfo.currentPoint.coordinates['α'] 取
// progressInfo 优先用 currentPointIndex（循环顶部推进，早于 moveToPoint）作索引从 config.points 查出当前点，
// 让目标角度先于实际角度变化；后端 autoEngine 为 nil 时回退到 completedPoints。
const targetAlpha = computed(() => {
  const coords = progressInfo.value?.currentPoint
  const alpha = coords?.['α']
  return typeof alpha === 'number' ? alpha : null
})

// 实际攻角：取第一个运动轴的实时位置（总压通常单 α 轴）
const actualAlpha = computed(() => {
  const first = liveAxisPositions.value[0]
  return first?.position ?? null
})

const isMoving = computed(() => liveAxisPositions.value.some((a) => a.moving))

const latestCoefficients = computed(() => {
  if (calibrationStore.realtimeCoefficients && typeof calibrationStore.realtimeCoefficients !== 'number' && 'error' in calibrationStore.realtimeCoefficients) return calibrationStore.realtimeCoefficients
  const points = calibrationStore.dataPoints
  if (!points.length) return null
  const lastPoint = points[points.length - 1]
  if (isTotalPressureDataPoint(lastPoint)) return lastPoint.coefficients
  return null
})

const latestRawData = computed(() => {
  const points = calibrationStore.dataPoints
  if (!points.length) return null
  const lastPoint = points[points.length - 1]
  if (isTotalPressureDataPoint(lastPoint)) return {
    pProbeTotal: lastPoint.rawData.pProbeTotal,
    pTunnelTotal: lastPoint.rawData.pTunnelTotal,
    pTunnelStatic: lastPoint.rawData.pTunnelStatic,
    pAtm: lastPoint.rawData.pAtm,
    tAtm: lastPoint.rawData.tAtm,
    stdDev: lastPoint.stdDev,
  }
  return null
})

// 实时气动参数（马赫数/速度）：spec Task 15 后由后端 CalibrationStatus.livePhysics 提供，
// store 在 updateStatusFromBackend 中映射，组件保持纯展示。总压校准用风洞温度（缺失时用大气温度兜底），
// 与后端 formulas.go TAT 选取逻辑一致。
const physics = computed(() => calibrationStore.calculatedPhysics)

const completedPoints = computed(() => calibrationStore.dataPoints.length)
const totalPoints = computed(() => currentConfig.value?.points.length ?? 0)

// 当前点采样子进度：calibrationStore.status 透传后端 autoEngine.GetSampleProgress()
// currentSample=0 表示当前点尚未开始采集或已采集完成（下一轮 processPoint 开头重置）
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

// 实时 CSV 路径：校准启动后操作员需知道数据写到哪，避免"存了找不到"
const csvSavePath = computed(() => currentConfig.value?.savePath ?? '')

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
  if (s === 'running') return t.value.running
  if (s === 'paused') return t.value.statusPaused
  if (s === 'stopped') return t.value.wf_statusStopped
  if (s === 'completed') return t.value.statusDone
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

const probeChannels = computed(() => {
  return currentConfig.value?.probeChannels ?? []
})

const totalPressureLayout = computed(() => {
  return currentConfig.value?.totalPressureLayout
})

// 核心通道（探针总压/风洞总压/风洞静压）—— 大字突出显示
const coreChannels = computed(() => {
  const coreRoles = ['totalPressure.pProbeTotal', 'totalPressure.pTunnelTotal', 'totalPressure.pTunnelStatic']
  return probeChannels.value.filter((c) => coreRoles.includes(c.role || ''))
})

// 差压 = 风洞总压 - 探针总压（两者均为表压，可直接相减）。
// 任一通道缺失时为 undefined，UI 显示 '--'。
const diffPressure = computed<number | undefined>(() => {
  const pressures = calibrationStore.realtimePressures
  if (!pressures) return undefined
  const { P0, PprobeTotal } = pressures
  if (P0 === undefined || PprobeTotal === undefined) return undefined
  return P0 - PprobeTotal
})

function getDiffPressureValue(): string {
  return formatValue(diffPressure.value, getProbeChannelPrecision(currentConfig.value, 'totalPressure.pTunnelTotal'))
}

// 次要通道（PAtm/TAtm/TTunnel）—— 小字显示
const secondaryChannels = computed(() => {
  const coreRoles = ['totalPressure.pProbeTotal', 'totalPressure.pTunnelTotal', 'totalPressure.pTunnelStatic']
  return probeChannels.value.filter((c) => !coreRoles.includes(c.role || ''))
})

// 总压数据点筛选结果缓存：避免 template 中多处重复 filter 创建新数组引用
const totalPressurePoints = computed(() => {
  return calibrationStore.dataPoints.filter(isTotalPressureDataPoint)
})

// 配置的全部测点 α 序列：配置完成后测点集合已知，
// 图表据此固定横坐标范围与刻度（不随采集进度动态扩展）
const plannedAlphaValues = computed<number[]>(() => {
  return (currentConfig.value?.points ?? [])
    .map((p) => p.coordinates?.['α'])
    .filter((v): v is number => typeof v === 'number' && Number.isFinite(v))
})

function formatValue(value: number | undefined | null, precision?: number): string {
  if (value === undefined || value === null || Number.isNaN(value)) return '--'
  return value.toFixed(precision ?? 3)
}

// 马赫数/速度格式化：与 CSV 精度一致（马赫数 3 位、速度 3 位）
function formatMach(value: number | undefined): string {
  if (value === undefined || value === null) return '--'
  return value.toFixed(3)
}
function formatVelocity(value: number | undefined): string {
  if (value === undefined || value === null) return '--'
  return value.toFixed(3)
}

// getChannelValue 从 calibrationStore.realtimePressures 读取，与三孔模式一致：
// store 的 realtimePressures 是 ref，updateRealtimePressures 整体替换触发响应式更新，
// 侧边栏通道值能可靠刷新。若从 latestSnapshots Map 直接读，Map 同 key 替换 value
// 时 Vue 对内部属性的依赖追踪不够稳定，会导致侧边栏不刷新。
function getChannelValue(role: string): string {
  const pressures = calibrationStore.realtimePressures
  if (!pressures) return '--'
  switch (role) {
    case 'totalPressure.pProbeTotal':
      return formatValue(pressures.PprobeTotal, getProbeChannelPrecision(currentConfig.value, role))
    case 'totalPressure.pTunnelTotal':
      return formatValue(pressures.P0, getProbeChannelPrecision(currentConfig.value, role))
    case 'totalPressure.pTunnelStatic':
      return formatValue(pressures.Ps, getProbeChannelPrecision(currentConfig.value, role))
    case 'totalPressure.pAtm':
      return formatValue(pressures.Patm, getProbeChannelPrecision(currentConfig.value, role))
    case 'totalPressure.tAtm':
      return formatValue(pressures.Tatm, getProbeChannelPrecision(currentConfig.value, role))
    case 'totalPressure.tTunnel':
      return formatValue(pressures.Ttunnel, getProbeChannelPrecision(currentConfig.value, role))
    default:
      return '--'
  }
}

function getChannelUnit(role: string): string {
  // 温度角色固定 ℃，避免设备 profile 未配置 unit 时误显示 Pa
  if (role === 'totalPressure.tAtm' || role === 'totalPressure.tTunnel') return '℃'
  const ch = probeChannels.value.find((c: { role?: string }) => c.role === role)
  if (!ch) return 'Pa'
  const device = deviceStore.profiles?.find((d) => d.id === ch.channel.deviceId)
  const channelConfig = device?.channels[ch.channel.channelIndex]
  return channelConfig?.unit ?? 'Pa'
}

// 配置加载完成后订阅采集快照，驱动实时通道数据 + calculatedPhysics 更新
// 与 ThreeHoleMain.vue 保持一致：isLoading 变 false 后订阅，避免无配置时空跑
watch(isLoading, (loading) => {
  if (loading) return
  cleanupSubscriptions()
  unsubscribeDaqSnapshot = deviceApi.onSnapshot((payload: DataPayload) => {
    latestSnapshots.value.set(payload.deviceId, payload)
    if (!currentConfig.value || currentConfig.value.type !== 'total-pressure') return
    const snapshots = Array.from(latestSnapshots.value.values())
    const pressures = buildRealtimePressuresFromSnapshots(currentConfig.value, snapshots)
    if (pressures) {
      calibrationStore.updateRealtimePressures(pressures)
      const requiredRoles = ['totalPressure.pAtm', 'totalPressure.pTunnelTotal', 'totalPressure.pTunnelStatic', 'totalPressure.pProbeTotal']
      const realtimeReady = requiredRoles.every((role) => currentConfig.value?.probeChannels.some((channel) => channel.enabled && channel.role === role && findChannelValue(snapshots, channel.channel.deviceId, channel.channel.channelIndex) !== null))
      if (realtimeReady) {
        void calibrationStore.updateRealtimeCoefficients('total-pressure', {
          PAtm: pressures.Patm, TAtm: pressures.Tatm, PTunnelTotal: pressures.P0,
          PTunnelStatic: pressures.Ps, TTunnel: pressures.Ttunnel, PProbeTotal: pressures.PprobeTotal,
        })
      }
    }
  })
  unsubscribeDeviceStatus = deviceStore.attachStatusListener()
  unsubscribeMotionStatus = motionStore.attachStatusListener()
  // 首次拉取一次状态，避免订阅事件到达前 UI 空白
  void motionStore.refreshStatus()
  // 300ms 轮询：驱动"实际α"卡片实时显示运动控制器轴位置
  motionStatusPollTimer = setInterval(() => {
    void motionStore.refreshStatus()
  }, 300)
})

// 切换到图表 Tab 时主动触发重绘（canvas 尺寸从 0 变为实际值）
watch(activeTab, (tab) => {
  if (tab === 'chart' || tab === 'overview') {
    requestAnimationFrame(() => {
      chartRef.value?.draw()
    })
  }
})

onMounted(async () => {
  try {
    // loadSavedConfig 已在 useCalibrationWorkflow.onMounted 中调用，不重复请求
    if (!workflow.hasConfig.value) {
      feedbackStore.pushToast(t.value.tp_pleaseConfigTotalPressure, 'warning')
    }
  } finally {
    isLoading.value = false
  }
})

onUnmounted(() => {
  cleanupSubscriptions()
})
</script>

<template>
  <div data-test="total-pressure-main-shell" class="flex h-full flex-col bg-[var(--bg-canvas)] text-[var(--text-primary)]">
    <!-- Header -->
    <div class="flex items-center justify-between border-b border-[var(--border-default)] bg-[var(--bg-panel)] px-5 py-2.5">
      <div class="flex items-center gap-3">
        <UiButton variant="secondary" size="sm" @click="emit('back')">
          <ArrowLeft class="h-4 w-4" />
        </UiButton>
        <div>
          <h1 class="text-base font-bold text-[var(--text-primary)]">{{ t.tp_probeCalibration }}</h1>
        </div>
      </div>
      <div class="flex items-center gap-2">
        <!-- 开始按钮：从左侧栏移到 Header，与"配置"并列便于一键启动 -->
        <UiButton v-if="!calibrationStore.isRunning && !calibrationStore.isPaused" variant="primary" size="sm" :disabled="!canStartCalibration" :title="startDisabledReason || undefined" @click="workflow.startCalibration()">
          <Play class="h-4 w-4" />
          <span class="ml-1">{{ t.startRun }}</span>
        </UiButton>
        <UiButton variant="secondary" size="sm" @click="emit('openSettings')">
          <Settings class="h-4 w-4" />
          <span class="ml-1">{{ t.config }}</span>
        </UiButton>
        <UiButton variant="secondary" size="sm" :disabled="!canSave" @click="workflow.saveCsv">
          <Save class="h-4 w-4" />
          <span class="ml-1">{{ t.save }}</span>
        </UiButton>
        <UiButton variant="secondary" size="sm" :disabled="!canSave" @click="workflow.exportReport">
          <Download class="h-4 w-4" />
          <span class="ml-1">{{ t.tp_export }}</span>
        </UiButton>
      </div>
    </div>

    <!-- 顶部状态栏：跨全宽，校准员最频繁看的信息（状态/进度/时间/目标α/实际α/Ma/V） -->
    <div class="flex items-center gap-4 border-b border-[var(--border-default)] bg-[var(--bg-panel)] px-5 py-2.5">
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

      <!-- 目标α + 实际α：校准员持续盯的核心信息 -->
      <div class="flex items-center gap-4 border-l border-[var(--border-default)] pl-4">
        <div class="flex items-center gap-1.5">
          <Target class="h-4 w-4 text-[var(--text-muted)]" />
          <span class="text-xs text-[var(--text-muted)]">{{ t.travTarget }}</span>
          <span class="font-mono text-base font-bold text-[var(--text-primary)]">{{ targetAlpha !== null ? targetAlpha.toFixed(1) + '°' : '--' }}</span>
        </div>
        <div class="flex items-center gap-1.5">
          <span class="text-xs text-[var(--text-muted)]">{{ t.travActual }}</span>
          <span class="font-mono text-base font-bold" :style="{ color: isMoving ? `var(--accent-success)` : `var(--accent-primary)` }">{{ actualAlpha !== null ? actualAlpha.toFixed(2) + '°' : '--' }}</span>
          <span v-if="isMoving" class="flex items-center gap-1 text-xs" :style="{ color: `var(--accent-success)` }">
            <span class="h-1.5 w-1.5 animate-pulse rounded-full" :style="{ backgroundColor: `var(--accent-success)` }"></span>
            {{ t.moving }}
          </span>
        </div>
      </div>

      <!-- 当前点采样子进度：操作员盯着屏幕知道还要等多久 -->
      <div v-if="sampleProgress" class="flex items-center gap-2 border-l border-[var(--border-default)] pl-4">
        <span class="text-xs text-[var(--text-muted)]">{{ t.samples }}</span>
        <span class="font-mono text-sm font-bold text-[var(--accent-primary)]">{{ sampleProgress.current }}/{{ sampleProgress.total }}</span>
        <div class="h-1.5 w-16 overflow-hidden rounded-full bg-[var(--bg-panel-strong)]">
          <div class="h-full rounded-full bg-[var(--accent-primary)] transition-all duration-200" :style="{ width: sampleProgress.percent + '%' }"></div>
        </div>
      </div>

      <!-- 实时 CSV 路径：操作员需知道数据写到哪 -->
      <div v-if="csvSavePath" class="flex items-center gap-1 border-l border-[var(--border-default)] pl-4 min-w-0" :title="csvSavePath">
        <FileText class="h-3.5 w-3.5 text-[var(--text-muted)] flex-shrink-0" />
        <span class="text-xs text-[var(--text-muted)] truncate max-w-[180px]">{{ csvSavePath }}</span>
      </div>

      <!-- 错误详情：后端 StateError 时展示，供操作员排查 -->
      <div v-if="lastError" class="flex items-center gap-1 border-l border-[var(--border-default)] pl-4" :title="lastError">
        <span class="text-xs font-medium" :style="{ color: `var(--accent-danger)` }">⚠ {{ lastError.length > 30 ? lastError.slice(0, 30) + '...' : lastError }}</span>
      </div>

      <!-- 配置摘要折叠：校准中几乎不看，压缩到角落 -->
      <button class="ml-auto flex items-center gap-1 text-xs text-[var(--text-muted)] hover:text-[var(--text-primary)]" @click="showConfigSummary = !showConfigSummary">
        <Settings class="h-3.5 w-3.5" />
        {{ t.config }}
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
    <div v-if="showConfigSummary" class="flex items-center gap-6 border-b border-[var(--border-default)] bg-[var(--bg-panel-strong)] px-5 py-2 text-xs">
      <div><span class="text-[var(--text-muted)]">{{ t.name }}：</span><span class="font-medium text-[var(--text-primary)]">{{ currentConfig?.name || t.unconfigured }}</span></div>
      <div v-if="totalPressureLayout">
        <span class="text-[var(--text-muted)]">{{ t.tp_alphaRange }}</span>
        <span class="font-medium text-[var(--text-primary)]">{{ totalPressureLayout.alphaMin }}° ~ {{ totalPressureLayout.alphaMax }}° ({{ totalPressureLayout.alphaStep }}°)</span>
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
            <UiButton v-if="canPause" variant="warning" @click="workflow.pauseCalibration">
              <Pause class="h-4 w-4" />
              <span class="ml-1">{{ t.travPause }}</span>
            </UiButton>
            <UiButton v-if="canResume" variant="primary" @click="workflow.resumeCalibration">
              <Play class="h-4 w-4" />
              <span class="ml-1">{{ t.travResume }}</span>
            </UiButton>
            <UiButton v-if="canStop" variant="danger" @click="workflow.stopCalibration">
              <Square class="h-4 w-4" />
              <span class="ml-1">{{ t.stop }}</span>
            </UiButton>
          </div>
          <div v-if="startDisabledReason" class="mt-2 text-xs" :style="{ color: `var(--accent-warning)` }">{{ startDisabledReason }}</div>
        </div>

        <!-- 中间内容区（可滚动）：关键数据(核心压力 + Ma/V) + 其他通道 -->
        <div class="flex-1 overflow-y-auto">
          <!-- 关键数据：核心压力（探针总压/风洞总压/风洞静压）+ 实时气动参数 Ma/V，合并为单卡片节省垂直空间 -->
          <div class="border-b border-[var(--border-default)] p-3">
            <div class="mb-2 flex items-center gap-2 text-sm font-medium text-[var(--text-primary)]">
              <Gauge class="h-4 w-4 text-[var(--accent-primary)]" />
              {{ t.tp_keyData }}
            </div>
            <div class="space-y-2">
              <!-- 探针总压 / 风洞总压 / 风洞静压 核心压力：单位置于数值右侧，与三孔保持一致 -->
              <div v-for="channel in coreChannels" :key="channel.name" class="flex items-baseline justify-between rounded-lg bg-[var(--bg-panel-strong)] px-3 py-2">
                <span class="text-xs text-[var(--text-muted)]">{{ channel.name }}</span>
                <div class="text-right">
                  <span class="font-mono text-2xl font-bold text-[var(--accent-primary)]">{{ getChannelValue(channel.role || '') }}</span>
                  <span class="ml-1 text-xs text-[var(--text-muted)]">{{ getChannelUnit(channel.role || '') }}</span>
                </div>
              </div>
              <!-- 差压 = 风洞总压 - 探针总压，置于探针/风洞总压下方 -->
              <div class="flex items-baseline justify-between rounded-lg bg-[var(--bg-panel-strong)] px-3 py-2">
                <span class="text-xs text-[var(--text-muted)]">{{ t.tp_diffPressure }}</span>
                <div class="text-right">
                  <span class="font-mono text-2xl font-bold text-[var(--accent-primary)]">{{ getDiffPressureValue() }}</span>
                  <span class="ml-1 text-xs text-[var(--text-muted)]">{{ getChannelUnit('totalPressure.pTunnelTotal') }}</span>
                </div>
              </div>
              <!-- 马赫数 Ma：绿色强调，区别于压力通道；无量纲无单位 -->
              <div class="flex items-baseline justify-between rounded-lg bg-[var(--bg-panel-strong)] px-3 py-2">
                <div>
                  <span class="text-xs text-[var(--text-muted)]">{{ t.tp_machMa }}</span>
                </div>
                <span class="font-mono text-2xl font-bold text-[var(--accent-success)]">{{ physics?.machNumber !== undefined ? physics.machNumber.toFixed(3) : '--' }}</span>
              </div>
              <!-- 速度 V：绿色强调，单位 m/s。侧边栏统一保留 3 位小数，与马赫数精度对齐 -->
              <div class="flex items-baseline justify-between rounded-lg bg-[var(--bg-panel-strong)] px-3 py-2">
                <div>
                  <span class="text-xs text-[var(--text-muted)]">{{ t.tp_velocityV }}</span>
                </div>
                <div class="text-right">
                  <span class="font-mono text-2xl font-bold text-[var(--accent-success)]">{{ physics?.velocity !== undefined ? physics.velocity.toFixed(3) : '--' }}</span>
                  <span class="ml-1 text-xs text-[var(--text-muted)]">m/s</span>
                </div>
              </div>
            </div>
          </div>

          <!-- 其他通道：PAtm/TAtm/TTunnel，始终展示 -->
          <div class="border-b border-[var(--border-default)] p-3">
            <div class="mb-2 flex items-center gap-2 text-sm font-medium text-[var(--text-primary)]">
              <Wind class="h-4 w-4 text-[var(--accent-primary)]" />
              {{ t.tp_otherChannels }}
            </div>
            <div class="space-y-1.5">
              <div v-for="channel in secondaryChannels" :key="channel.name" class="flex items-center justify-between px-2 py-1.5 rounded bg-[var(--bg-panel-strong)]">
                <span class="text-xs text-[var(--text-muted)]">{{ channel.name }}</span>
                <div class="text-right">
                  <span class="font-mono text-sm font-bold text-[var(--text-primary)]">{{ getChannelValue(channel.role || '') }}</span>
                  <span class="ml-1 text-xs text-[var(--text-muted)]">{{ getChannelUnit(channel.role || '') }}</span>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- 球罐门控状态条（固定底部）：一行显示，不占独立卡片；附"编辑"入口跳配置界面 -->
        <div class="flex-shrink-0 border-t border-[var(--border-default)] p-3">
          <div class="flex items-center gap-2 whitespace-nowrap rounded-lg bg-[var(--bg-panel-strong)] px-3 py-2 text-xs">
            <!-- 左侧：状态块（标题 · 状态词） -->
            <div class="flex items-center gap-2 whitespace-nowrap">
              <span class="text-[var(--text-muted)]">{{ t.tp_sphereTankGate }}</span>
              <span class="text-[var(--text-muted)]">·</span>
              <span class="font-medium" :style="{ color: sphereTankGate.isActive.value ? `var(--accent-success)` : `var(--text-muted)` }">
                {{ sphereTankGate.isActive.value ? t.tp_activated : sphereTankGate.statusText.value }}
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
      <div class="flex flex-1 flex-col min-w-0 overflow-hidden">
        <!-- Tab 导航 -->
        <div class="flex border-b border-[var(--border-default)] bg-[var(--bg-panel)]">
          <UiButton quaternary size="sm" class="relative px-5 py-2.5 text-sm font-medium transition-colors" :class="activeTab === 'overview' ? 'text-[var(--accent-primary)]' : 'text-[var(--text-muted)] hover:text-[var(--text-primary)]'" @click="activeTab = 'overview'">
            <TrendingUp class="h-4 w-4" />
            {{ t.overview }}
            <span v-if="activeTab === 'overview'" class="absolute bottom-0 left-0 right-0 h-0.5 bg-[var(--accent-primary)] rounded-t-full"></span>
          </UiButton>
          <UiButton quaternary size="sm" class="relative px-5 py-2.5 text-sm font-medium transition-colors" :class="activeTab === 'chart' ? 'text-[var(--accent-primary)]' : 'text-[var(--text-muted)] hover:text-[var(--text-primary)]'" @click="activeTab = 'chart'">
            <TrendingUp class="h-4 w-4" />
            {{ t.chart }}
            <span v-if="activeTab === 'chart'" class="absolute bottom-0 left-0 right-0 h-0.5 bg-[var(--accent-primary)] rounded-t-full"></span>
          </UiButton>
          <UiButton quaternary size="sm" class="relative px-5 py-2.5 text-sm font-medium transition-colors" :class="activeTab === 'data' ? 'text-[var(--accent-primary)]' : 'text-[var(--text-muted)] hover:text-[var(--text-primary)]'" @click="activeTab = 'data'">
            <FileText class="h-4 w-4" />
            {{ t.data }}
            <span v-if="activeTab === 'data'" class="absolute bottom-0 left-0 right-0 h-0.5 bg-[var(--accent-primary)] rounded-t-full"></span>
          </UiButton>
        </div>

        <!-- 概览页：左列（系数+原始数据）+ 右列（K-α 曲线） -->
        <div v-if="activeTab === 'overview'" class="flex-1 overflow-hidden p-4">
          <div class="flex h-full gap-3 min-h-0">
            <!-- 左列：系数 + 原始数据 -->
            <div class="flex w-[300px] flex-col gap-3 flex-shrink-0">
              <!-- 系数卡片（大字突出） -->
              <div class="rounded-xl border border-[var(--border-default)] bg-[var(--bg-panel)] p-4 shadow-[var(--shadow-panel)]">
                <div class="mb-3 flex items-center gap-2">
                  <TrendingUp class="h-4 w-4 text-[var(--accent-primary)]" />
                  <h3 class="text-sm font-semibold text-[var(--text-primary)]">{{ t.tp_latestCoefficients }}</h3>
                </div>
                <div v-if="latestCoefficients" class="space-y-2">
                  <div class="flex items-baseline justify-between rounded-lg bg-[var(--bg-panel-strong)] px-3 py-2">
                    <span class="text-xs text-[var(--text-muted)]">CPT</span>
                    <span class="font-mono text-xl font-bold text-[var(--accent-primary)]">{{ formatValue(latestCoefficients.CPT, 4) }}</span>
                  </div>
                  <div class="flex items-baseline justify-between rounded-lg bg-[var(--bg-panel-strong)] px-3 py-2">
                    <span class="text-xs text-[var(--text-muted)]">{{ t.tp_error }}</span>
                    <span class="font-mono text-xl font-bold text-[var(--accent-primary)]">{{ formatValue(latestCoefficients.error, 4) }}</span>
                  </div>
                  <!-- 实时马赫数/速度：与系数同卡展示，便于校准员关联系数与流场状态。
                       数据源与侧边栏一致，取自 store.calculatedPhysics（后端 livePhysics 5Hz 推送），
                       而非 latestCoefficients（上一点采集完成快照），避免两点之间不刷新。 -->
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
                <div v-else class="flex h-24 items-center justify-center text-sm text-[var(--text-muted)]">{{ t.tp_noCoefficientData }}</div>
              </div>

              <!-- 原始数据卡片：与系数卡片风格一致，全宽上下并列 -->
              <div class="flex-1 rounded-xl border border-[var(--border-default)] bg-[var(--bg-panel)] p-4 shadow-[var(--shadow-panel)] overflow-auto">
                <div class="mb-3 flex items-center gap-2">
                  <Wind class="h-4 w-4 text-[var(--accent-primary)]" />
                  <h3 class="text-sm font-semibold text-[var(--text-primary)]">{{ t.tp_rawData }}</h3>
                </div>
                <div v-if="latestRawData" class="space-y-2">
                  <div class="flex items-baseline justify-between rounded-lg bg-[var(--bg-panel-strong)] px-3 py-2">
                    <span class="text-xs text-[var(--text-muted)]">{{ t.tp_probeTotal }}</span>
                    <span class="font-mono text-lg font-bold text-[var(--accent-primary)]">{{ formatValue(latestRawData.pProbeTotal, 1) }}</span>
                  </div>
                  <div class="flex items-baseline justify-between rounded-lg bg-[var(--bg-panel-strong)] px-3 py-2">
                    <span class="text-xs text-[var(--text-muted)]">{{ t.fiveHolePTotal }}</span>
                    <span class="font-mono text-lg font-bold text-[var(--accent-primary)]">{{ formatValue(latestRawData.pTunnelTotal, 1) }}</span>
                  </div>
                  <div class="flex items-baseline justify-between rounded-lg bg-[var(--bg-panel-strong)] px-3 py-2">
                    <span class="text-xs text-[var(--text-muted)]">{{ t.fiveHolePTunnelStatic }}</span>
                    <span class="font-mono text-lg font-bold text-[var(--accent-primary)]">{{ formatValue(latestRawData.pTunnelStatic, 1) }}</span>
                  </div>
                  <div class="flex items-baseline justify-between rounded-lg bg-[var(--bg-panel-strong)] px-3 py-2">
                    <span class="text-xs text-[var(--text-muted)]">{{ t.Patm }}</span>
                    <span class="font-mono text-lg font-bold text-[var(--accent-primary)]">{{ formatValue(latestRawData.pAtm, 1) }}</span>
                  </div>
                  <div class="flex items-baseline justify-between rounded-lg bg-[var(--bg-panel-strong)] px-3 py-2">
                    <span class="text-xs text-[var(--text-muted)]">{{ t.Tatm }}</span>
                    <span class="font-mono text-lg font-bold text-[var(--accent-primary)]">{{ formatValue(latestRawData.tAtm, 1) }}</span>
                  </div>
                  <div class="flex items-baseline justify-between rounded-lg bg-[var(--bg-panel-strong)] px-3 py-2">
                    <span class="text-xs text-[var(--text-muted)]">{{ t.tp_sampleStdDev }}</span>
                    <span class="font-mono text-lg font-bold text-[var(--accent-primary)]">{{ formatValue(latestRawData.stdDev, 3) }}</span>
                  </div>
                </div>
                <div v-else class="flex h-20 items-center justify-center text-sm text-[var(--text-muted)]">{{ t.tp_noRawData }}</div>
              </div>
            </div>

            <!-- 右列：K-α 曲线，概览页显示 X 轴刻度 -->
            <div class="flex flex-1 flex-col rounded-xl border border-[var(--border-default)] bg-[var(--bg-panel)] p-3 shadow-[var(--shadow-panel)] min-w-0 min-h-0">
              <h3 class="mb-1 text-xs font-semibold text-[var(--text-muted)] flex-shrink-0">{{ t.tp_cptAlphaCurve }}</h3>
              <div class="flex-1 min-h-0">
                <TotalPressureChart ref="chartRef" :data-points="totalPressurePoints" x-key="alpha" y-key="CPT" x-label="α (°)" :y-precision="chartYPrecision" :y-range-override="chartYRangeOverride" :planned-x-values="plannedAlphaValues" />
              </div>
            </div>
          </div>
        </div>

        <!-- 图表 Tab：放大查看 K-α 曲线 -->
        <div v-if="activeTab === 'chart'" class="flex-1 overflow-hidden p-4">
          <div class="flex h-full flex-col rounded-xl border border-[var(--border-default)] bg-[var(--bg-panel)] p-3 shadow-[var(--shadow-panel)] min-h-0">
            <div class="flex items-center justify-between mb-2 flex-shrink-0 flex-wrap gap-2">
              <h3 class="text-sm font-semibold text-[var(--text-primary)]">{{ t.tp_cptAlphaCurve }}</h3>
              <!-- Y 轴范围/精度控制工具条：
                   - 精度输入框立即生效（无需点应用）
                   - 最小值/最大值输入框 + 应用按钮：写入 override 触发重绘
                   - 自动按钮：清空 override 回退到基于数据点的自动范围
                   - 概览页与图表 Tab 共用同一组设置，保证两 Tab 视觉一致 -->
              <div class="flex items-center gap-2 text-xs flex-wrap">
                <span class="text-[var(--text-muted)]">{{ t.tp_yAxisRange }}</span>
                <UiInputNumber v-model="chartYMinInput" :placeholder="t.tp_yMinPlaceholder" :step="0.001" class="w-28" />
                <span class="text-[var(--text-muted)]">~</span>
                <UiInputNumber v-model="chartYMaxInput" :placeholder="t.tp_yMaxPlaceholder" :step="0.001" class="w-28" />
                <UiButton size="sm" variant="primary" @click="applyYRange">{{ t.tp_apply }}</UiButton>
                <UiButton size="sm" variant="secondary" :disabled="chartYRangeOverride === null" @click="resetYRangeToAuto">{{ t.tp_auto }}</UiButton>
                <span class="text-[var(--text-muted)] ml-2">{{ t.tp_yPrecisionLabel }}</span>
                <UiInputNumber v-model="chartYPrecisionInput" :min="0" :max="6" :step="1" class="w-20" />
                <span v-if="chartYRangeOverride" class="text-[var(--accent-success)]">{{ t.tp_yRangeManualActive }}</span>
                <span v-else class="text-[var(--text-muted)]">{{ t.tp_yRangeAutoActive }}</span>
              </div>
            </div>
            <div class="flex-1 min-h-0">
              <TotalPressureChart ref="chartRef" :data-points="totalPressurePoints" x-key="alpha" y-key="CPT" x-label="α (°)" :y-precision="chartYPrecision" :y-range-override="chartYRangeOverride" :planned-x-values="plannedAlphaValues" />
            </div>
          </div>
        </div>

        <!-- 数据 Tab：完整数据表 -->
        <div v-if="activeTab === 'data'" class="flex-1 overflow-auto p-4">
          <div class="rounded-xl border border-[var(--border-default)] bg-[var(--bg-panel)] p-4 shadow-[var(--shadow-panel)]">
            <div class="mb-3 flex items-center gap-2">
              <FileText class="h-5 w-5 text-[var(--accent-primary)]" />
              <h3 class="text-base font-semibold text-[var(--text-primary)]">{{ t.tp_calibrationDataLog }}</h3>
              <span class="ml-auto text-sm text-[var(--text-muted)]">{{ t.tp_totalRecords.replace('{count}', String(calibrationStore.dataPoints.length)) }}</span>
            </div>
            <div class="overflow-auto">
              <table class="w-full text-sm">
                <thead class="bg-[var(--bg-panel-strong)]">
                  <tr>
                    <th class="px-3 py-2 text-left text-xs font-medium text-[var(--text-muted)]">{{ t.tp_index }}</th>
                    <th class="px-3 py-2 text-left text-xs font-medium text-[var(--text-muted)]">α</th>
                    <th class="px-3 py-2 text-left text-xs font-medium text-[var(--text-muted)]">CPT</th>
                    <th class="px-3 py-2 text-left text-xs font-medium text-[var(--text-muted)]">{{ t.tp_errorPercent }}</th>
                    <th class="px-3 py-2 text-left text-xs font-medium text-[var(--text-muted)]">{{ t.tp_machMaLabel }}</th>
                    <th class="px-3 py-2 text-left text-xs font-medium text-[var(--text-muted)]">{{ t.tp_velocityMsLabel }}</th>
                    <th class="px-3 py-2 text-left text-xs font-medium text-[var(--text-muted)]">{{ t.samples }}</th>
                    <th class="px-3 py-2 text-left text-xs font-medium text-[var(--text-muted)]">{{ t.tp_stdDev }}</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-[var(--border-default)]">
                  <tr v-for="(point, index) in calibrationStore.dataPoints" :key="index" class="hover:bg-[var(--bg-panel-strong)]">
                    <td class="px-3 py-2 font-mono text-[var(--text-primary)]">{{ index + 1 }}</td>
                    <td class="px-3 py-2 font-mono text-[var(--text-primary)]">{{ isTotalPressureDataPoint(point) ? formatValue(point.alpha, 1) : '--' }}</td>
                    <td class="px-3 py-2 font-mono text-[var(--text-primary)]">{{ isTotalPressureDataPoint(point) ? formatValue(point.coefficients.CPT, 4) : '--' }}</td>
                    <td class="px-3 py-2 font-mono text-[var(--text-primary)]">{{ isTotalPressureDataPoint(point) ? formatValue(point.coefficients.error, 4) : '--' }}</td>
                    <td class="px-3 py-2 font-mono text-[var(--text-primary)]">{{ isTotalPressureDataPoint(point) ? formatMach(point.coefficients.machNumber) : '--' }}</td>
                    <td class="px-3 py-2 font-mono text-[var(--text-primary)]">{{ isTotalPressureDataPoint(point) ? formatVelocity(point.coefficients.velocity) : '--' }}</td>
                    <td class="px-3 py-2 font-mono text-[var(--text-primary)]">{{ 'sampleCount' in point ? point.sampleCount : '--' }}</td>
                    <td class="px-3 py-2 font-mono text-[var(--text-primary)]">{{ formatValue(point.stdDev, 4) }}</td>
                  </tr>
                </tbody>
              </table>
              <div v-if="calibrationStore.dataPoints.length === 0" class="py-8 text-center text-sm text-[var(--text-muted)]">{{ t.tp_noDataRecords }}</div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

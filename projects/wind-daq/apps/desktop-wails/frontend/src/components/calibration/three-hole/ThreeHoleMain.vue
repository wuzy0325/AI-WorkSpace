<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { useCalibrationStore } from '@stores/calibrationStore'
import { useDeviceStore } from '@stores/deviceStore'
import { useMotionStore } from '@stores/motionStore'
import { useFeedbackStore } from '@stores/feedbackStore'
import { useCalibrationWorkflow } from '@composables/useCalibrationWorkflow'
import type { ProbeChannelRole } from '@shared/types/calibration'
import { getProbeChannelPrecision } from '@shared/calibrationPrecision'
import { isThreeHoleDataPoint } from '@shared/calibrationDataGuards'
import { deviceApi } from '@api/deviceApi'
import type { DataPayload } from '@api/types'
import UiButton from '@components/ui/UiButton.vue'
import ThreeHoleChart from './ThreeHoleChart.vue'
import {
  Play, Pause, Square, Settings, ArrowLeft, Save, FileText, RotateCcw,
  ChevronDown, ChevronUp, Gauge, TrendingUp, Target, Wind
} from '@lucide/vue'


const emit = defineEmits<{
  back: []
  openSettings: []
}>()

const calibrationStore = useCalibrationStore()
const deviceStore = useDeviceStore()
const motionStore = useMotionStore()
const feedbackStore = useFeedbackStore()

const workflow = useCalibrationWorkflow('three-hole')
const sphereTankGate = workflow.sphereTankGate

const isLoading = ref(true)
const activeTab = ref<'overview' | 'chart' | 'data'>('overview')
const showConfigSummary = ref(false)
const latestSnapshots = ref<Map<string, DataPayload>>(new Map())

// 三个图表的 ref，用于切换 Tab 后主动触发重绘
// 使用 InstanceType 让 Vue 自动推导子组件暴露的类型，避免手写与 defineExpose 耦合
const kbChartRef = ref<InstanceType<typeof ThreeHoleChart> | null>(null)
const ktChartRef = ref<InstanceType<typeof ThreeHoleChart> | null>(null)
const sbChartRef = ref<InstanceType<typeof ThreeHoleChart> | null>(null)

// 实时采集快照订阅：deviceApi.onSnapshot 推送设备通道原始数据，
// 由 buildRealtimePressuresFromSnapshots 映射到 RealtimePressures 喂给 store，
// 驱动"实时通道数据"面板更新。与 FiveHoleMain.vue 的模式一致。
let unsubscribeDaqSnapshot: (() => void) | null = null
let unsubscribeDeviceStatus: (() => void) | null = null
let unsubscribeMotionStatus: (() => void) | null = null
// 运动控制器状态轮询定时器：后端 Wails 事件推送频率不足以支撑"当前位置"实时显示，
// 需 300ms 主动拉取 status（与 FiveHoleMain.vue 保持一致），驱动 liveAxisPositions 更新。
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

// buildRealtimePressuresFromSnapshots 按三孔通道角色映射将设备快照转为 RealtimePressures
function buildRealtimePressuresFromSnapshots(
  config: import('@shared/types/calibration').CalibrationConfig,
  snapshots: DataPayload[],
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

  for (const ch of config.probeChannels) {
    if (!ch.enabled) continue
    const rawValue = toValue(ch.channel.deviceId, ch.channel.channelIndex)
    if (rawValue === null) continue
    switch (ch.role as ProbeChannelRole) {
      case 'threeHole.p1': result.P1 = rawValue; matchedChannelCount += 1; break
      case 'threeHole.p2': result.P2 = rawValue; matchedChannelCount += 1; break
      case 'threeHole.p3': result.P3 = rawValue; matchedChannelCount += 1; break
      case 'threeHole.pAtm': result.Patm = rawValue; matchedChannelCount += 1; break
      case 'threeHole.tAtm': result.Tatm = rawValue; matchedChannelCount += 1; break
      case 'threeHole.pTotal': result.P0 = rawValue; matchedChannelCount += 1; break
      case 'threeHole.pStatic': result.Ps = rawValue; matchedChannelCount += 1; break
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
// 通过 motionStore.statusById 查到对应控制器的 AxisStatus，取实时 position/moving。
// 三孔探针通常配置单旋转轴（θ），但此处按配置轴数通用渲染，避免硬编码。
interface LiveAxisPosition {
  name: string
  position: number | null
  moving: boolean
}
// 运动控制器实时轴位置：直接读 motionStore.statusList（ref），
// 在 computed 内访问 .value 建立响应式依赖，300ms 轮询 refreshStatus 更新 statusList 后自动重算。
// 注意：不能用 motionStore.statusById()（普通函数，依赖追踪不如直接访问 ref 靠），与 FiveHoleMain 保持一致。
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

// 当前目标角度：从 progressInfo.currentPoint.coordinates['θ'] 取
// progressInfo 用 completedPoints 作索引从 config.points 查出当前点
const targetTheta = computed(() => {
  const coords = progressInfo.value?.currentPoint
  const theta = coords?.['θ']
  return typeof theta === 'number' ? theta : null
})

// 实际角度：取第一个运动轴的实时位置（三孔通常单 θ 轴）
const actualTheta = computed(() => {
  const first = liveAxisPositions.value[0]
  return first?.position ?? null
})

const isMoving = computed(() => liveAxisPositions.value.some((a) => a.moving))

const latestCoefficients = computed(() => {
  const points = calibrationStore.dataPoints
  if (!points.length) return null
  const lastPoint = points[points.length - 1]
  if (isThreeHoleDataPoint(lastPoint)) return lastPoint.coefficients
  return null
})

const latestRawData = computed(() => {
  const points = calibrationStore.dataPoints
  if (!points.length) return null
  const lastPoint = points[points.length - 1]
  if (isThreeHoleDataPoint(lastPoint)) return lastPoint.rawData
  return null
})

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
  if (calibrationStore.isPaused) return '已暂停'
  if (calibrationStore.isRunning) return '运行中'
  if (calibrationStore.completeEvent) return '已完成'
  return '空闲'
})

const statusColor = computed(() => {
  if (calibrationStore.isPaused) return 'warning'
  if (calibrationStore.isRunning) return 'success'
  if (calibrationStore.completeEvent) return 'info'
  return 'normal'
})

const canPause = computed(() => calibrationStore.isRunning && !calibrationStore.isPaused)
const canResume = computed(() => calibrationStore.isPaused)
const canStop = computed(() => calibrationStore.isRunning || calibrationStore.isPaused)
const canSave = computed(() => calibrationStore.completeEvent !== null || calibrationStore.dataPoints.length > 0)

const probeChannels = computed(() => {
  return currentConfig.value?.probeChannels ?? []
})

const threeHoleLayout = computed(() => {
  return currentConfig.value?.threeHoleLayout
})

// 核心通道（P1/P2/P3）—— 大字突出显示
const coreChannels = computed(() => {
  const coreRoles = ['threeHole.p1', 'threeHole.p2', 'threeHole.p3']
  return probeChannels.value.filter((c) => coreRoles.includes(c.role || ''))
})

// 次要通道（PAtm/TAtm/PTotal/PStatic）—— 折叠显示
const secondaryChannels = computed(() => {
  const coreRoles = ['threeHole.p1', 'threeHole.p2', 'threeHole.p3']
  return probeChannels.value.filter((c) => !coreRoles.includes(c.role || ''))
})

// 三孔数据点筛选结果缓存：避免 template 中 6 处重复 filter 创建新数组引用
const threeHolePoints = computed(() => {
  return calibrationStore.dataPoints.filter(isThreeHoleDataPoint)
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

function formatValue(value: number | undefined | null, precision?: number): string {
  if (value === undefined || value === null) return '--'
  return value.toFixed(precision ?? 3)
}

function getChannelValue(role: string): string {
  const pressures = calibrationStore.realtimePressures
  if (!pressures) return '--'
  switch (role) {
    case 'threeHole.p1': return formatValue(pressures.P1, getProbeChannelPrecision(currentConfig.value, role))
    case 'threeHole.p2': return formatValue(pressures.P2, getProbeChannelPrecision(currentConfig.value, role))
    case 'threeHole.p3': return formatValue(pressures.P3, getProbeChannelPrecision(currentConfig.value, role))
    case 'threeHole.pAtm': return formatValue(pressures.Patm, getProbeChannelPrecision(currentConfig.value, role))
    case 'threeHole.tAtm': return formatValue(pressures.Tatm, getProbeChannelPrecision(currentConfig.value, role))
    case 'threeHole.pTotal': return formatValue(pressures.P0, getProbeChannelPrecision(currentConfig.value, role))
    case 'threeHole.pStatic': return formatValue(pressures.Ps, getProbeChannelPrecision(currentConfig.value, role))
    default: return '--'
  }
}

function getChannelUnit(role: string): string {
  const ch = probeChannels.value.find((c: { role?: string }) => c.role === role)
  if (!ch) return 'Pa'
  const device = deviceStore.profiles?.find((d) => d.id === ch.channel.deviceId)
  const channelConfig = device?.channels[ch.channel.channelIndex]
  return channelConfig?.unit ?? 'Pa'
}

// 配置加载完成后订阅采集快照，驱动实时通道数据更新
// 与 FiveHoleMain.vue 保持一致：isLoading 变 false 后订阅，避免无配置时空跑
watch(isLoading, (loading) => {
  if (loading) return
  cleanupSubscriptions()
  unsubscribeDaqSnapshot = deviceApi.onSnapshot((payload: DataPayload) => {
    latestSnapshots.value.set(payload.deviceId, payload)
    if (!currentConfig.value || currentConfig.value.type !== 'three-hole') return
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
  // 300ms 轮询：驱动"当前位置"卡片实时显示运动控制器轴位置
  motionStatusPollTimer = setInterval(() => {
    void motionStore.refreshStatus()
  }, 300)
})

// 切换到图表 Tab 时主动触发重绘（canvas 尺寸从 0 变为实际值）
watch(activeTab, (tab) => {
  if (tab === 'chart') {
    requestAnimationFrame(() => {
      kbChartRef.value?.draw()
      ktChartRef.value?.draw()
      sbChartRef.value?.draw()
    })
  }
})

onMounted(async () => {
  try {
    // loadSavedConfig 已在 useCalibrationWorkflow.onMounted 中调用，不重复请求
    if (!workflow.hasConfig.value) {
      feedbackStore.pushToast('请先配置三孔探针校准参数', 'warning')
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
  <div data-test="three-hole-main-shell" class="flex h-full flex-col bg-[var(--bg-canvas)] text-[var(--text-primary)]">
    <!-- Header -->
    <div class="flex items-center justify-between border-b border-[var(--border-default)] bg-[var(--bg-panel)] px-5 py-2.5">
      <div class="flex items-center gap-3">
        <UiButton variant="secondary" size="sm" @click="emit('back')">
          <ArrowLeft class="h-4 w-4" />
        </UiButton>
        <div>
          <h1 class="text-base font-bold text-[var(--text-primary)]">三孔探针校准</h1>
          <p class="text-xs text-[var(--text-muted)]">Three-Hole Probe Calibration</p>
        </div>
      </div>
      <div class="flex items-center gap-2">
        <UiButton variant="secondary" size="sm" @click="emit('openSettings')">
          <Settings class="h-4 w-4" />
          <span class="ml-1">配置</span>
        </UiButton>
        <UiButton variant="secondary" size="sm" :disabled="!canSave" @click="workflow.saveCsv">
          <Save class="h-4 w-4" />
          <span class="ml-1">保存</span>
        </UiButton>
      </div>
    </div>

    <!-- 顶部状态栏：跨全宽，校准员最频繁看的信息（状态/进度/时间/目标θ/实际θ） -->
    <div class="flex items-center gap-4 border-b border-[var(--border-default)] bg-[var(--bg-panel)] px-5 py-2.5">
      <span
        class="rounded-full px-2 py-0.5 text-xs font-medium"
        :style="{
          backgroundColor: `color-mix(in srgb, var(${statusColorToken}) 15%, transparent)`,
          color: `var(${statusColorToken})`,
        }"
      >{{ statusText }}</span>

      <div class="flex items-center gap-2 min-w-[180px] flex-1 max-w-[280px]">
        <span class="text-xs text-[var(--text-muted)] whitespace-nowrap">进度</span>
        <div class="h-2 flex-1 overflow-hidden rounded-full bg-[var(--bg-panel-strong)]">
          <div class="h-full rounded-full bg-[var(--accent-primary)] transition-all duration-300" :style="{ width: progressPercent + '%' }"></div>
        </div>
        <span class="text-xs font-mono font-bold text-[var(--text-primary)] whitespace-nowrap">{{ formattedProgress }}</span>
      </div>

      <div v-if="formattedTimeInfo" class="flex items-center gap-3 text-xs">
        <div class="flex items-center gap-1">
          <span class="text-[var(--text-muted)]">已用</span>
          <span class="font-mono font-bold text-[var(--text-primary)]">{{ formattedTimeInfo.elapsed }}</span>
        </div>
        <div class="flex items-center gap-1">
          <span class="text-[var(--text-muted)]">剩余</span>
          <span class="font-mono font-bold text-[var(--text-primary)]">{{ formattedTimeInfo.remaining }}</span>
        </div>
      </div>

      <!-- 目标θ + 实际θ：校准员持续盯的核心信息 -->
      <div class="flex items-center gap-4 border-l border-[var(--border-default)] pl-4">
        <div class="flex items-center gap-1.5">
          <Target class="h-4 w-4 text-[var(--text-muted)]" />
          <span class="text-xs text-[var(--text-muted)]">目标</span>
          <span class="font-mono text-base font-bold text-[var(--text-primary)]">{{ targetTheta !== null ? targetTheta.toFixed(1) + '°' : '--' }}</span>
        </div>
        <div class="flex items-center gap-1.5">
          <span class="text-xs text-[var(--text-muted)]">实际</span>
          <span class="font-mono text-base font-bold" :style="{ color: isMoving ? `var(--accent-success)` : `var(--accent-primary)` }">{{ actualTheta !== null ? actualTheta.toFixed(2) + '°' : '--' }}</span>
          <span v-if="isMoving" class="flex items-center gap-1 text-xs" :style="{ color: `var(--accent-success)` }">
            <span class="h-1.5 w-1.5 animate-pulse rounded-full" :style="{ backgroundColor: `var(--accent-success)` }"></span>
            运动中
          </span>
        </div>
      </div>

      <!-- 配置摘要折叠：校准中几乎不看，压缩到角落 -->
      <button class="ml-auto flex items-center gap-1 text-xs text-[var(--text-muted)] hover:text-[var(--text-primary)]" @click="showConfigSummary = !showConfigSummary">
        <Settings class="h-3.5 w-3.5" />
        配置
        <ChevronDown v-if="showConfigSummary" class="h-3 w-3" />
        <ChevronUp v-else class="h-3 w-3" />
      </button>
    </div>

    <!-- 配置摘要展开面板（默认收起） -->
    <div v-if="showConfigSummary" class="flex items-center gap-6 border-b border-[var(--border-default)] bg-[var(--bg-panel-strong)] px-5 py-2 text-xs">
      <div><span class="text-[var(--text-muted)]">名称：</span><span class="font-medium text-[var(--text-primary)]">{{ currentConfig?.name || '未配置' }}</span></div>
      <div v-if="threeHoleLayout">
        <span class="text-[var(--text-muted)]">θ 范围：</span>
        <span class="font-medium text-[var(--text-primary)]">{{ threeHoleLayout.thetaMin }}° ~ {{ threeHoleLayout.thetaMax }}° ({{ threeHoleLayout.thetaStep }}°)</span>
      </div>
      <div><span class="text-[var(--text-muted)]">总点数：</span><span class="font-medium text-[var(--text-primary)]">{{ totalPoints }}</span></div>
      <div><span class="text-[var(--text-muted)]">驻留：</span><span class="font-medium text-[var(--text-primary)]">{{ currentConfig?.dwellTimeMs || 0 }}ms</span></div>
      <div><span class="text-[var(--text-muted)]">采样数：</span><span class="font-medium text-[var(--text-primary)]">{{ currentConfig?.samplesPerPoint || 0 }}</span></div>
    </div>

    <div class="flex flex-1 overflow-hidden">
      <!-- 左侧栏：384px，控制按钮 + 核心通道大字 + 次要通道折叠 + 球罐状态条 -->
      <div class="flex w-96 flex-col border-r border-[var(--border-default)] bg-[var(--bg-panel)] flex-shrink-0 overflow-hidden">
        <!-- 控制按钮 -->
        <div class="border-b border-[var(--border-default)] p-3">
          <div class="grid grid-cols-2 gap-2">
            <UiButton v-if="!calibrationStore.isRunning && !calibrationStore.isPaused" variant="primary" :disabled="!canStartCalibration" @click="workflow.startCalibration()">
              <Play class="h-4 w-4" />
              <span class="ml-1">开始</span>
            </UiButton>
            <UiButton v-if="canPause" variant="warning" @click="workflow.pauseCalibration">
              <Pause class="h-4 w-4" />
              <span class="ml-1">暂停</span>
            </UiButton>
            <UiButton v-if="canResume" variant="primary" @click="workflow.resumeCalibration">
              <Play class="h-4 w-4" />
              <span class="ml-1">继续</span>
            </UiButton>
            <UiButton v-if="canStop" variant="danger" @click="workflow.stopCalibration">
              <Square class="h-4 w-4" />
              <span class="ml-1">停止</span>
            </UiButton>
            <UiButton variant="secondary" @click="calibrationStore.reset">
              <RotateCcw class="h-4 w-4" />
              <span class="ml-1">重置</span>
            </UiButton>
          </div>
          <div v-if="startDisabledReason" class="mt-2 text-xs" :style="{ color: `var(--accent-warning)` }">{{ startDisabledReason }}</div>
        </div>

        <!-- 核心通道（P1/P2/P3）大字显示 -->
        <div class="border-b border-[var(--border-default)] p-3">
          <div class="mb-2 flex items-center gap-2 text-sm font-medium text-[var(--text-primary)]">
            <Gauge class="h-4 w-4 text-[var(--accent-primary)]" />
            核心压力
          </div>
          <div class="space-y-2">
            <div v-for="channel in coreChannels" :key="channel.name" class="flex items-baseline justify-between rounded-lg bg-[var(--bg-panel-strong)] px-3 py-2">
              <div>
                <div class="text-xs text-[var(--text-muted)]">{{ channel.name }}</div>
                <div class="text-xs text-[var(--text-muted)]">{{ getChannelUnit(channel.role || '') }}</div>
              </div>
              <span class="font-mono text-2xl font-bold text-[var(--accent-primary)]">{{ getChannelValue(channel.role || '') }}</span>
            </div>
          </div>
        </div>

        <!-- 其他通道：保留标签，始终展示，不再折叠 -->
        <div class="border-b border-[var(--border-default)] p-3">
          <div class="mb-2 flex items-center gap-2 text-sm font-medium text-[var(--text-primary)]">
            <Wind class="h-4 w-4 text-[var(--accent-primary)]" />
            其他通道
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

        <!-- 球罐门控状态条：压缩为一行，不占独立卡片 -->
        <div class="mt-auto border-t border-[var(--border-default)] p-3">
          <div class="flex items-center justify-between rounded-lg bg-[var(--bg-panel-strong)] px-3 py-2">
            <div class="flex items-center gap-2">
              <span class="h-2 w-2 rounded-full" :style="{ backgroundColor: sphereTankGate.isActive.value ? `var(--accent-success)` : `var(--text-muted)` }"></span>
              <span class="text-xs text-[var(--text-muted)]">球罐门控</span>
            </div>
            <div class="flex items-center gap-3 text-xs">
              <span class="font-medium" :style="{ color: sphereTankGate.isActive.value ? `var(--accent-success)` : `var(--text-muted)` }">
                {{ sphereTankGate.isActive.value ? '已激活' : sphereTankGate.statusText.value }}
              </span>
              <span class="text-[var(--text-muted)]">|</span>
              <span class="font-mono font-bold text-[var(--text-primary)]">{{ sphereTankGate.waitTimeSec.value }}s</span>
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
            概览
            <span v-if="activeTab === 'overview'" class="absolute bottom-0 left-0 right-0 h-0.5 bg-[var(--accent-primary)] rounded-t-full"></span>
          </UiButton>
          <UiButton quaternary size="sm" class="relative px-5 py-2.5 text-sm font-medium transition-colors" :class="activeTab === 'chart' ? 'text-[var(--accent-primary)]' : 'text-[var(--text-muted)] hover:text-[var(--text-primary)]'" @click="activeTab = 'chart'">
            <TrendingUp class="h-4 w-4" />
            图表
            <span v-if="activeTab === 'chart'" class="absolute bottom-0 left-0 right-0 h-0.5 bg-[var(--accent-primary)] rounded-t-full"></span>
          </UiButton>
          <UiButton quaternary size="sm" class="relative px-5 py-2.5 text-sm font-medium transition-colors" :class="activeTab === 'data' ? 'text-[var(--accent-primary)]' : 'text-[var(--text-muted)] hover:text-[var(--text-primary)]'" @click="activeTab = 'data'">
            <FileText class="h-4 w-4" />
            数据
            <span v-if="activeTab === 'data'" class="absolute bottom-0 left-0 right-0 h-0.5 bg-[var(--accent-primary)] rounded-t-full"></span>
          </UiButton>
        </div>

        <!-- 概览页：左列（系数+原始数据）+ 右列（三图表）+ 底部迷你表格 -->
        <div v-if="activeTab === 'overview'" class="flex-1 overflow-hidden p-4">
          <div class="flex h-full flex-col gap-3">
            <!-- 上部：左列系数+原始数据，右列三图表 -->
            <div class="flex flex-1 gap-3 min-h-0">
              <!-- 左列：系数 + 原始数据 -->
              <div class="flex w-[300px] flex-col gap-3 flex-shrink-0">
                <!-- 系数卡片（大字突出） -->
                <div class="rounded-xl border border-[var(--border-default)] bg-[var(--bg-panel)] p-4 shadow-[var(--shadow-panel)]">
                  <div class="mb-3 flex items-center gap-2">
                    <TrendingUp class="h-4 w-4 text-[var(--accent-primary)]" />
                    <h3 class="text-sm font-semibold text-[var(--text-primary)]">最新系数</h3>
                  </div>
                  <div v-if="latestCoefficients" class="space-y-2">
                    <div class="flex items-baseline justify-between rounded-lg bg-[var(--bg-panel-strong)] px-3 py-2">
                      <span class="text-xs text-[var(--text-muted)]">Kb</span>
                      <span class="font-mono text-xl font-bold text-[var(--accent-primary)]">{{ formatValue(latestCoefficients.Kb, 4) }}</span>
                    </div>
                    <div class="flex items-baseline justify-between rounded-lg bg-[var(--bg-panel-strong)] px-3 py-2">
                      <span class="text-xs text-[var(--text-muted)]">Kt</span>
                      <span class="font-mono text-xl font-bold text-[var(--accent-primary)]">{{ formatValue(latestCoefficients.Kt, 4) }}</span>
                    </div>
                    <div class="flex items-baseline justify-between rounded-lg bg-[var(--bg-panel-strong)] px-3 py-2">
                      <span class="text-xs text-[var(--text-muted)]">Sb</span>
                      <span class="font-mono text-xl font-bold text-[var(--accent-primary)]">{{ formatValue(latestCoefficients.Sb, 4) }}</span>
                    </div>
                  </div>
                  <div v-else class="flex h-24 items-center justify-center text-sm text-[var(--text-muted)]">暂无系数数据</div>
                </div>

                <!-- 原始数据卡片：与系数卡片风格一致，全宽上下并列 -->
                <div class="flex-1 rounded-xl border border-[var(--border-default)] bg-[var(--bg-panel)] p-4 shadow-[var(--shadow-panel)]">
                  <div class="mb-3 flex items-center gap-2">
                    <Wind class="h-4 w-4 text-[var(--accent-primary)]" />
                    <h3 class="text-sm font-semibold text-[var(--text-primary)]">原始数据</h3>
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
                      <span class="text-xs text-[var(--text-muted)]">大气压</span>
                      <span class="font-mono text-lg font-bold text-[var(--accent-primary)]">{{ formatValue(latestRawData.pAtm, 1) }}</span>
                    </div>
                    <div class="flex items-baseline justify-between rounded-lg bg-[var(--bg-panel-strong)] px-3 py-2">
                      <span class="text-xs text-[var(--text-muted)]">总压</span>
                      <span class="font-mono text-lg font-bold text-[var(--accent-primary)]">{{ formatValue(latestRawData.pTotal, 1) }}</span>
                    </div>
                    <div class="flex items-baseline justify-between rounded-lg bg-[var(--bg-panel-strong)] px-3 py-2">
                      <span class="text-xs text-[var(--text-muted)]">静压</span>
                      <span class="font-mono text-lg font-bold text-[var(--accent-primary)]">{{ formatValue(latestRawData.pStatic, 1) }}</span>
                    </div>
                  </div>
                  <div v-else class="flex h-20 items-center justify-center text-sm text-[var(--text-muted)]">暂无原始数据</div>
                </div>
              </div>

              <!-- 右列：三条曲线（Kb-θ / Kt-θ / Sb-θ），概览页隐藏 X/Y 轴标签节省空间 -->
              <div class="flex flex-1 flex-col gap-3 min-w-0">
                <div class="flex-1 rounded-xl border border-[var(--border-default)] bg-[var(--bg-panel)] p-3 shadow-[var(--shadow-panel)] min-h-0">
                  <h3 class="mb-1 text-xs font-semibold text-[var(--text-muted)]">Kb - θ 曲线</h3>
                  <div class="h-[calc(100%-1.25rem)]">
                    <ThreeHoleChart ref="kbChartRef" :data-points="threeHolePoints" x-key="theta" y-key="Kb" x-label="θ (°)" :show-x-axis-labels="false" />
                  </div>
                </div>
                <div class="flex-1 rounded-xl border border-[var(--border-default)] bg-[var(--bg-panel)] p-3 shadow-[var(--shadow-panel)] min-h-0">
                  <h3 class="mb-1 text-xs font-semibold text-[var(--text-muted)]">Kt - θ 曲线</h3>
                  <div class="h-[calc(100%-1.25rem)]">
                    <ThreeHoleChart ref="ktChartRef" :data-points="threeHolePoints" x-key="theta" y-key="Kt" x-label="θ (°)" :show-x-axis-labels="false" />
                  </div>
                </div>
                <div class="flex-1 rounded-xl border border-[var(--border-default)] bg-[var(--bg-panel)] p-3 shadow-[var(--shadow-panel)] min-h-0">
                  <h3 class="mb-1 text-xs font-semibold text-[var(--text-muted)]">Sb - θ 曲线</h3>
                  <div class="h-[calc(100%-1.25rem)]">
                    <ThreeHoleChart ref="sbChartRef" :data-points="threeHolePoints" x-key="theta" y-key="Sb" x-label="θ (°)" :show-x-axis-labels="false" />
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- 图表 Tab：放大查看三条曲线 -->
        <div v-if="activeTab === 'chart'" class="flex-1 overflow-hidden p-4">
          <div class="grid h-full grid-cols-2 gap-3">
            <div class="rounded-xl border border-[var(--border-default)] bg-[var(--bg-panel)] p-3 shadow-[var(--shadow-panel)]">
              <h3 class="mb-2 text-sm font-semibold text-[var(--text-primary)]">Kb - θ 曲线</h3>
              <div class="h-[calc(100%-2rem)]">
                <ThreeHoleChart :data-points="threeHolePoints" x-key="theta" y-key="Kb" x-label="θ (°)" y-label="Kb" />
              </div>
            </div>
            <div class="rounded-xl border border-[var(--border-default)] bg-[var(--bg-panel)] p-3 shadow-[var(--shadow-panel)]">
              <h3 class="mb-2 text-sm font-semibold text-[var(--text-primary)]">Kt - θ 曲线</h3>
              <div class="h-[calc(100%-2rem)]">
                <ThreeHoleChart :data-points="threeHolePoints" x-key="theta" y-key="Kt" x-label="θ (°)" y-label="Kt" />
              </div>
            </div>
            <div class="rounded-xl border border-[var(--border-default)] bg-[var(--bg-panel)] p-3 shadow-[var(--shadow-panel)]">
              <h3 class="mb-2 text-sm font-semibold text-[var(--text-primary)]">Sb - θ 曲线</h3>
              <div class="h-[calc(100%-2rem)]">
                <ThreeHoleChart :data-points="threeHolePoints" x-key="theta" y-key="Sb" x-label="θ (°)" y-label="Sb" />
              </div>
            </div>
          </div>
        </div>

        <!-- 数据 Tab：完整数据表 -->
        <div v-if="activeTab === 'data'" class="flex-1 overflow-auto p-4">
          <div class="rounded-xl border border-[var(--border-default)] bg-[var(--bg-panel)] p-4 shadow-[var(--shadow-panel)]">
            <div class="mb-3 flex items-center gap-2">
              <FileText class="h-5 w-5 text-[var(--accent-primary)]" />
              <h3 class="text-base font-semibold text-[var(--text-primary)]">校准数据记录</h3>
              <span class="ml-auto text-sm text-[var(--text-muted)]">共 {{ calibrationStore.dataPoints.length }} 条记录</span>
            </div>
            <div class="overflow-auto">
              <table class="w-full text-sm">
                <thead class="bg-[var(--bg-panel-strong)]">
                  <tr>
                    <th class="px-3 py-2 text-left text-xs font-medium text-[var(--text-muted)]">序号</th>
                    <th class="px-3 py-2 text-left text-xs font-medium text-[var(--text-muted)]">θ</th>
                    <th class="px-3 py-2 text-left text-xs font-medium text-[var(--text-muted)]">Kb</th>
                    <th class="px-3 py-2 text-left text-xs font-medium text-[var(--text-muted)]">Kt</th>
                    <th class="px-3 py-2 text-left text-xs font-medium text-[var(--text-muted)]">Sb</th>
                    <th class="px-3 py-2 text-left text-xs font-medium text-[var(--text-muted)]">采样数</th>
                    <th class="px-3 py-2 text-left text-xs font-medium text-[var(--text-muted)]">标准差</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-[var(--border-default)]">
                  <tr v-for="(point, index) in calibrationStore.dataPoints" :key="index" class="hover:bg-[var(--bg-panel-strong)]">
                    <td class="px-3 py-2 font-mono text-[var(--text-primary)]">{{ index + 1 }}</td>
                    <td class="px-3 py-2 font-mono text-[var(--text-primary)]">{{ isThreeHoleDataPoint(point) ? formatValue(point.coordinates['θ'], 1) : '--' }}</td>
                    <td class="px-3 py-2 font-mono text-[var(--text-primary)]">{{ isThreeHoleDataPoint(point) ? formatValue(point.coefficients.Kb, 4) : '--' }}</td>
                    <td class="px-3 py-2 font-mono text-[var(--text-primary)]">{{ isThreeHoleDataPoint(point) ? formatValue(point.coefficients.Kt, 4) : '--' }}</td>
                    <td class="px-3 py-2 font-mono text-[var(--text-primary)]">{{ isThreeHoleDataPoint(point) ? formatValue(point.coefficients.Sb, 4) : '--' }}</td>
                    <td class="px-3 py-2 font-mono text-[var(--text-primary)]">{{ 'sampleCount' in point ? point.sampleCount : '--' }}</td>
                    <td class="px-3 py-2 font-mono text-[var(--text-primary)]">{{ formatValue(point.stdDev, 4) }}</td>
                  </tr>
                </tbody>
              </table>
              <div v-if="calibrationStore.dataPoints.length === 0" class="py-8 text-center text-sm text-[var(--text-muted)]">暂无数据记录</div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

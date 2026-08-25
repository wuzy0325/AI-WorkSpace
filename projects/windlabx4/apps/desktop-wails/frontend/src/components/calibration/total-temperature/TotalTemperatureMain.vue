<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { useCalibrationStore } from '@stores/calibrationStore'
import { useDeviceStore } from '@stores/deviceStore'
import { useMotionStore } from '@stores/motionStore'
import { useFeedbackStore } from '@stores/feedbackStore'
import { useI18nStore } from '@stores/i18nStore'
import { useCalibrationWorkflow } from '@composables/useCalibrationWorkflow'
import type { CalibrationConfig, TotalTemperatureCalibrationPoint } from '@shared/types/calibration'
import { getProbeChannelPrecision } from '@shared/calibrationPrecision'
import { findChannelValue } from '@shared/calibrationSnapshotValue'
import { isTotalTemperatureDataPoint } from '@shared/calibrationDataGuards'
import { deviceApi } from '@api/deviceApi'
import type { DataPayload } from '@api/types'
import UiButton from '@components/ui/UiButton.vue'
import MotionSafetyAlertCard from '@components/shared/MotionSafetyAlertCard.vue'
import {
  Play, Pause, Square, Settings, ArrowLeft, Save, FileText,
  ChevronDown, ChevronUp, Activity, Gauge, Wind, Timer, Target, TrendingUp
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

const workflow = useCalibrationWorkflow('total-temperature')
const sphereTankGate = workflow.sphereTankGate

// 暴露 reloadSavedConfig 给父组件 CalibrationWindow：
// Settings 保存配置后父组件调 currentMainRef.reloadSavedConfig() 触发重新加载，
// 否则 currentConfig 仍是挂载时的旧值，canStartCalibration 不刷新，会一直提示"未配置"。
// 与 FiveHoleMain / ThreeHoleMain / TotalPressureMain 保持一致，
// 由 calibrationMainExpose.contract.test.ts 在编译期断言本暴露存在。
defineExpose({
  reloadSavedConfig: workflow.loadSavedConfig,
})

const isLoading = ref(true)
const showChannelPanel = ref(true)
const activeTab = ref<'overview' | 'chart' | 'data'>('overview')
const recoveryCanvas = ref<HTMLCanvasElement | null>(null)
const chartUpdateTimer = ref<ReturnType<typeof setInterval> | null>(null)
const latestSnapshots = ref<Map<string, DataPayload>>(new Map())

const currentConfig = computed(() => workflow.currentConfig.value)
const progressInfo = computed(() => workflow.progressInfo.value)
const formattedTimeInfo = computed(() => workflow.formattedTimeInfo.value)
const canStartCalibration = computed(() => workflow.canStartCalibration.value)
const startDisabledReason = computed(() => workflow.startDisabledReason.value)

const currentMach = computed(() => {
  const coords = progressInfo.value?.currentPoint
  return coords && typeof coords.Mach === 'number' ? coords.Mach : null
})

const latestCoefficients = computed(() => {
  if (typeof calibrationStore.realtimeCoefficients === 'number') return { recoveryFactor: calibrationStore.realtimeCoefficients }
  const points = calibrationStore.dataPoints
  if (!points.length) return null
  const lastPoint = points[points.length - 1]
  if (isTotalTemperatureDataPoint(lastPoint)) return { recoveryFactor: lastPoint.recoveryCoefficient }
  return null
})

let unsubscribeDaqSnapshot: (() => void) | null = null

function updateRealtimeCoefficient(config: CalibrationConfig, snapshots: DataPayload[]): void {
  let testProbeTemp: number | undefined
  let standardProbeTemp: number | undefined
  for (const channel of config.probeChannels) {
    if (!channel.enabled) continue
    const value = findChannelValue(snapshots, channel.channel.deviceId, channel.channel.channelIndex)
    if (value === null) continue
    if (channel.role === 'totalTemperature.tTotal') testProbeTemp = value
    if (channel.role === 'totalTemperature.tStatic') standardProbeTemp = value
  }
  if (testProbeTemp === undefined || standardProbeTemp === undefined || !Number.isFinite(testProbeTemp) || !Number.isFinite(standardProbeTemp)) return
  void calibrationStore.updateRealtimeCoefficients('total-temperature', { TestProbeTemp: testProbeTemp, StandardProbeTemp: standardProbeTemp })
}

const latestRawData = computed(() => {
  const points = calibrationStore.dataPoints
  if (!points.length) return null
  const lastPoint = points[points.length - 1]
  if (isTotalTemperatureDataPoint(lastPoint)) return { tTotal: lastPoint.testProbeTemp, tStatic: lastPoint.standardProbeTemp, tAtm: lastPoint.ambientTemp }
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

// 状态色 CSS 变量标识：将 statusColor 转为设计 token，替代 Tailwind 调色板硬编码。
// 与 ThreeHoleMain/TotalPressureMain 一致，统一用 color-mix 派生背景色，避免暗色主题割裂。
const statusColorToken = computed(() => {
  switch (statusColor.value) {
    case 'success': return '--accent-success'
    case 'warning': return '--accent-warning'
    case 'info': return '--accent-info'
    default: return '--text-muted'
  }
})

// 进度条填充色：总温模块原本就是绿色渐变（emerald-teal），保持绿色系；
// 校准完成后统一用醒目成功绿，与其余模块一致提示本趟已结束。
const progressBarColor = computed(() => '--accent-success')

const canPause = computed(() => calibrationStore.isRunning && !calibrationStore.isPaused)
const canResume = computed(() => calibrationStore.isPaused)
const canStop = computed(() => calibrationStore.isRunning || calibrationStore.isPaused)
const canSave = computed(() => calibrationStore.completeEvent !== null || calibrationStore.dataPoints.length > 0)

// 运动安全故障现场快照：从 calibrationStore.status.motionSafetyFailure 取，
// 后端在故障发生时写入、恢复时清空。告警卡片据此渲染/隐藏。
const motionSafetyFailure = computed(() => calibrationStore.status?.motionSafetyFailure ?? null)

const probeChannels = computed(() => {
  return currentConfig.value?.probeChannels ?? []
})

const totalTemperatureLayout = computed(() => {
  return currentConfig.value?.totalTemperatureConfig?.machRange
})

// 数据记录条数文案：复用 i18n 占位符替换，避免模板中拼接破坏语序
const recordCountText = computed(() => {
  return t.value.tt_recordCount.replace('{count}', String(calibrationStore.dataPoints.length))
})

function formatValue(value: number | undefined | null, precision?: number): string {
  if (value === undefined || value === null) return '--'
  return value.toFixed(precision ?? 3)
}

// 角色到 RealtimePressures 字段的映射。
//
// 设计约束：RealtimePressures 类型只暴露 Tatm（大气温度）一个温度字段，
// tTotal/tStatic 在该类型中无对应字段——后端未通过此通道下发试验探针/标准探针温度。
// 早期实现把三个角色都映射到 Tatm，导致侧栏三列温度同值，操作员无法区分三类温度，
// 也无法判断试验探针/标准探针温度是否已稳定。
//
// 当前修复策略：
//   - tAtm：读 pressures.Tatm（仅此角色有真值）
//   - tTotal / tStatic：返回 '--'，明确"该通道实时值未通过 RealtimePressures 暴露"
//     操作员若需查看试验探针/标准探针温度，参考右侧 latestRawData 卡片（从 dataPoints
//     最后一条读取 testProbeTemp / standardProbeTemp / ambientTemp）。
//
// 若后续后端在 RealtimePressures 中补 Ttunnel 或独立 Tprobe 字段，可在此处补 case。
function getChannelValue(role: string): string {
  const pressures = calibrationStore.realtimePressures
  if (!pressures) return '--'
  switch (role) {
    case 'totalTemperature.tTotal': return '--'
    case 'totalTemperature.tStatic': return '--'
    case 'totalTemperature.tAtm': return formatValue(pressures.Tatm, getProbeChannelPrecision(currentConfig.value, role))
    default: return '--'
  }
}

function getChannelUnit(role: string): string {
  const ch = probeChannels.value.find((c: { role?: string }) => c.role === role)
  if (!ch) return '°C'
  const device = deviceStore.profiles?.find((d) => d.id === ch.channel.deviceId)
  const channelConfig = device?.channels[ch.channel.channelIndex]
  return channelConfig?.unit ?? '°C'
}

function drawChart() {
  const canvas = recoveryCanvas.value
  if (!canvas) return
  const ctx = canvas.getContext('2d')
  if (!ctx) return

  const dpr = window.devicePixelRatio || 1
  const rect = canvas.getBoundingClientRect()
  canvas.width = rect.width * dpr
  canvas.height = rect.height * dpr
  ctx.scale(dpr, dpr)

  const width = rect.width
  const height = rect.height
  const padding = 40

  ctx.clearRect(0, 0, width, height)

  const dataPoints = calibrationStore.dataPoints
    .filter((p): p is TotalTemperatureCalibrationPoint => isTotalTemperatureDataPoint(p))
    .map((p) => ({ x: p.targetMachNumber, y: p.recoveryCoefficient }))

  if (dataPoints.length === 0) {
    ctx.fillStyle = '#64748b'
    ctx.font = '14px sans-serif'
    ctx.textAlign = 'center'
    ctx.textBaseline = 'middle'
    ctx.fillText(t.value.tt_noData, width / 2, height / 2)
    return
  }

  const xMin = Math.min(...dataPoints.map((d) => d.x)) * 1.1
  const xMax = Math.max(...dataPoints.map((d) => d.x)) * 1.1
  const yMin = Math.min(...dataPoints.map((d) => d.y)) * 1.1
  const yMax = Math.max(...dataPoints.map((d) => d.y)) * 1.1

  const xScale = (width - 2 * padding) / (xMax - xMin)
  const yScale = (height - 2 * padding) / (yMax - yMin)

  dataPoints.forEach((point, index) => {
    const x = padding + (point.x - xMin) * xScale
    const y = height - padding - (point.y - yMin) * yScale
    ctx.beginPath()
    ctx.arc(x, y, 4, 0, Math.PI * 2)
    ctx.fillStyle = index === dataPoints.length - 1 ? '#10b981' : '#3b82f6'
    ctx.fill()
    ctx.strokeStyle = '#fff'
    ctx.lineWidth = 1.5
    ctx.stroke()
  })
}

function startChartUpdate() {
  if (chartUpdateTimer.value) clearInterval(chartUpdateTimer.value)
  chartUpdateTimer.value = setInterval(() => {
    if (activeTab.value === 'chart') drawChart()
  }, calibrationStore.uiRefreshIntervalMs)
}

function stopChartUpdate() {
  if (chartUpdateTimer.value) {
    clearInterval(chartUpdateTimer.value)
    chartUpdateTimer.value = null
  }
}

watch(activeTab, (tab) => {
  if (tab === 'chart') {
    startChartUpdate()
    drawChart()
  } else {
    stopChartUpdate()
  }
})

onMounted(async () => {
  try {
    await workflow.loadSavedConfig()
    if (!workflow.hasConfig.value) {
      feedbackStore.pushToast(t.value.tt_pleaseConfigureFirst, 'warning')
    }
  } finally {
    isLoading.value = false
  }
  unsubscribeDaqSnapshot = deviceApi.onSnapshot((payload: DataPayload) => {
    latestSnapshots.value.set(payload.deviceId, payload)
    if (!currentConfig.value || currentConfig.value.type !== 'total-temperature') return
    updateRealtimeCoefficient(currentConfig.value, Array.from(latestSnapshots.value.values()))
  })
})

onUnmounted(() => {
  stopChartUpdate()
  unsubscribeDaqSnapshot?.()
})
</script>

<template>
  <div data-test="total-temperature-main-shell" class="flex h-full flex-col bg-[var(--bg-canvas)] text-[var(--text-primary)]">
    <div class="flex items-center justify-between border-b border-[var(--border-default)] bg-[var(--bg-panel)] px-5 py-3">
      <div class="flex items-center gap-3">
        <UiButton variant="secondary" size="sm" @click="emit('back')">
          <ArrowLeft class="h-4 w-4" />
        </UiButton>
        <div>
          <h1 class="text-base font-bold text-[var(--text-primary)]">{{ t.tt_totalTemperatureCalibration }}</h1>
        </div>
      </div>
      <div class="flex items-center gap-2">
        <UiButton variant="secondary" size="sm" @click="emit('openSettings')">
          <Settings class="h-4 w-4" />
          <span class="ml-1">{{ t.config }}</span>
        </UiButton>
        <UiButton variant="secondary" size="sm" :disabled="!canSave" @click="workflow.saveCsv">
          <Save class="h-4 w-4" />
          <span class="ml-1">{{ t.save }}</span>
        </UiButton>
        <UiButton variant="secondary" size="sm" :disabled="!canSave" @click="workflow.exportReport">
          <FileText class="h-4 w-4" />
          <span class="ml-1">{{ t.tt_export }}</span>
        </UiButton>
      </div>
    </div>

    <!-- 运动安全故障告警卡片：仅在 motionSafetyFailure 存在时渲染。
         独立卡片承载 6 字段结构化信息（控制器/轴/目标/实际/偏差/点号），
         单行状态栏无法承载这些信息。与遍历测试模块共用同一告警卡片组件。 -->
    <MotionSafetyAlertCard
      :failure="motionSafetyFailure"
      :t="(t as unknown as Record<string, string>)"
    />

    <div class="flex flex-1 overflow-hidden">
      <!-- 左侧边栏：固定宽度 320px，可滚动 -->
      <div class="flex w-80 flex-col border-r border-[var(--border-default)] bg-[var(--bg-panel)] overflow-y-auto flex-shrink-0">
        <div class="border-b border-[var(--border-default)] p-4">
          <div class="mb-3 flex items-center justify-between">
            <span class="text-sm text-[var(--text-muted)]">{{ t.status }}</span>
            <span class="rounded-full px-2 py-0.5 text-xs font-medium"
              :style="{
                backgroundColor: `color-mix(in srgb, var(${statusColorToken}) 15%, transparent)`,
                color: `var(${statusColorToken})`,
              }"
            >{{ statusText }}</span>
          </div>
          <div class="mb-2">
            <div class="mb-1 flex justify-between text-xs">
              <span class="text-[var(--text-muted)]">{{ t.travProgress }}</span>
              <span class="text-[var(--text-primary)]">{{ formattedProgress }}</span>
            </div>
            <div class="h-2 overflow-hidden rounded-full bg-[var(--bg-panel-strong)]">
              <div
                class="h-full rounded-full transition-all duration-300"
                :style="{ width: progressPercent + '%', backgroundColor: `var(${progressBarColor})` }"
              ></div>
            </div>
          </div>
          <div v-if="formattedTimeInfo" class="grid grid-cols-2 gap-2 text-xs">
            <div class="rounded-lg bg-[var(--bg-panel-strong)] p-2">
              <div class="text-[var(--text-muted)]">{{ t.tt_elapsedTime }}</div>
              <div class="font-mono font-bold text-[var(--text-primary)]">{{ formattedTimeInfo.elapsed }}</div>
            </div>
            <div class="rounded-lg bg-[var(--bg-panel-strong)] p-2">
              <div class="text-[var(--text-muted)]">{{ t.travEstimatedRemaining }}</div>
              <div class="font-mono font-bold text-[var(--text-primary)]">{{ formattedTimeInfo.remaining }}</div>
            </div>
          </div>
        </div>

        <div class="border-b border-[var(--border-default)] p-4">
          <div class="grid grid-cols-2 gap-2">
            <UiButton v-if="!calibrationStore.isRunning && !calibrationStore.isPaused" variant="primary" :disabled="!canStartCalibration" @click="workflow.startCalibration()">
              <Play class="h-4 w-4" />
              <span class="ml-1">{{ t.startRun }}</span>
            </UiButton>
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
              <span class="ml-1">{{ t.travStop }}</span>
            </UiButton>
        </div>
          <div v-if="startDisabledReason" class="mt-2 text-xs text-amber-500">{{ startDisabledReason }}</div>
        </div>

        <div class="border-b border-[var(--border-default)] p-4">
          <div class="mb-2 flex items-center gap-2 text-sm font-medium text-[var(--text-primary)]">
            <Target class="h-4 w-4 text-[var(--accent-primary)]" />
            {{ t.tt_currentPosition }}
          </div>
          <div v-if="currentMach !== null" class="rounded-lg bg-[var(--bg-panel-strong)] p-3 text-center">
            <div class="text-xs text-[var(--text-muted)]">{{ t.machNumber }}</div>
            <div class="font-mono text-lg font-bold text-[var(--accent-primary)]">{{ currentMach.toFixed(2) }}</div>
          </div>
          <div v-else class="rounded-lg bg-[var(--bg-panel-strong)] p-3 text-center text-sm text-[var(--text-muted)]">
            {{ t.tt_notStarted }}
          </div>
        </div>

        <div class="border-b border-[var(--border-default)] p-4">
          <!-- 球罐门控状态条：一行显示，不显示等待时间（总温无球罐压力通道） -->
          <div class="flex items-center gap-2 whitespace-nowrap rounded-lg bg-[var(--bg-panel-strong)] px-3 py-2 text-xs">
            <div class="flex items-center gap-2 whitespace-nowrap">
              <Activity class="h-4 w-4 text-[var(--accent-primary)]" />
              <span class="text-sm font-medium text-[var(--text-primary)]">{{ t.tt_sphereTankGate }}</span>
              <span class="text-[var(--text-muted)]">·</span>
              <span class="font-medium" :class="sphereTankGate.isActive.value ? 'text-emerald-500' : 'text-[var(--text-muted)]'">
                {{ sphereTankGate.isActive.value ? t.tt_activated : sphereTankGate.statusText.value }}
              </span>
            </div>
            <!-- 左右视觉分隔 -->
            <span class="text-[var(--text-muted)]">|</span>
            <!-- 球罐当前稳定时间实时显示：仅展示，不参与判定；
                 无数据时显示固定宽度占位"--.--"避免布局抖动，单位 s 紧随数字避免"没配置"错觉 -->
            <div class="flex items-center gap-1 whitespace-nowrap">
              <span class="text-[var(--text-muted)]">{{ t.wf_sphereStableTimeLabel }}</span>
              <span class="font-mono font-bold text-[var(--text-primary)] tabular-nums">
                {{ sphereTankGate.stableTimeSec.value !== null ? sphereTankGate.stableTimeSec.value.toFixed(2) : '--.--' }}{{ t.wf_sphereStableTimeUnit }}
              </span>
            </div>
          </div>
        </div>

        <div class="flex-1 overflow-auto">
          <div class="flex cursor-pointer items-center justify-between border-b border-[var(--border-default)] bg-[var(--bg-panel-strong)] p-3" @click="showChannelPanel = !showChannelPanel">
            <div class="flex items-center gap-2 text-sm font-medium text-[var(--text-primary)]">
              <Gauge class="h-4 w-4 text-[var(--accent-primary)]" />
              {{ t.tt_realtimeChannelData }}
            </div>
            <ChevronDown v-if="showChannelPanel" class="h-4 w-4 text-[var(--text-muted)]" />
            <ChevronUp v-else class="h-4 w-4 text-[var(--text-muted)]" />
          </div>
          <div v-if="showChannelPanel" class="divide-y divide-[var(--border-default)]">
            <div v-for="channel in probeChannels" :key="channel.name" class="flex items-center justify-between px-4 py-2">
              <div class="text-xs text-[var(--text-muted)]">{{ channel.name }}</div>
              <div class="text-right">
                <div class="font-mono text-sm font-bold text-[var(--text-primary)]">{{ getChannelValue(channel.role || '') }}</div>
                <div class="text-xs text-[var(--text-muted)]">{{ getChannelUnit(channel.role || '') }}</div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- 右侧主内容区：自适应宽度，最小宽度为0防止溢出 -->
      <div class="flex flex-1 flex-col min-w-0">
        <!-- 标签页导航：增强选中态样式 -->
        <div class="flex border-b border-[var(--border-default)] bg-[var(--bg-panel)]">
          <UiButton quaternary size="sm" class="relative px-5 py-3 text-sm font-medium transition-colors" :class="activeTab === 'overview' ? 'text-[var(--accent-primary)]' : 'text-[var(--text-muted)] hover:text-[var(--text-primary)]'" @click="activeTab = 'overview'">
            <Activity class="h-4 w-4" />
            {{ t.overview }}
            <span v-if="activeTab === 'overview'" class="absolute bottom-0 left-0 right-0 h-0.5 bg-[var(--accent-primary)] rounded-t-full"></span>
          </UiButton>
          <UiButton quaternary size="sm" class="relative px-5 py-3 text-sm font-medium transition-colors" :class="activeTab === 'chart' ? 'text-[var(--accent-primary)]' : 'text-[var(--text-muted)] hover:text-[var(--text-primary)]'" @click="activeTab = 'chart'">
            <TrendingUp class="h-4 w-4" />
            {{ t.chart }}
            <span v-if="activeTab === 'chart'" class="absolute bottom-0 left-0 right-0 h-0.5 bg-[var(--accent-primary)] rounded-t-full"></span>
          </UiButton>
          <UiButton quaternary size="sm" class="relative px-5 py-3 text-sm font-medium transition-colors" :class="activeTab === 'data' ? 'text-[var(--accent-primary)]' : 'text-[var(--text-muted)] hover:text-[var(--text-primary)]'" @click="activeTab = 'data'">
            <FileText class="h-4 w-4" />
            {{ t.data }}
            <span v-if="activeTab === 'data'" class="absolute bottom-0 left-0 right-0 h-0.5 bg-[var(--accent-primary)] rounded-t-full"></span>
          </UiButton>
        </div>

        <div v-if="activeTab === 'overview'" class="flex-1 overflow-auto p-5">
          <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div class="rounded-xl border border-[var(--border-default)] bg-[var(--bg-panel)] p-5 shadow-[var(--shadow-panel)]">
              <div class="mb-4 flex items-center gap-2">
                <TrendingUp class="h-5 w-5 text-[var(--accent-primary)]" />
                <h3 class="text-base font-semibold text-[var(--text-primary)]">{{ t.tt_latestCoefficients }}</h3>
              </div>
              <div v-if="latestCoefficients" class="grid grid-cols-2 gap-3">
                <div class="rounded-lg bg-[var(--bg-panel-strong)] p-3">
                  <div class="text-xs text-[var(--text-muted)]">{{ t.tt_recoveryCoeff }}</div>
                  <div class="font-mono text-lg font-bold text-[var(--accent-primary)]">{{ formatValue(latestCoefficients.recoveryFactor, 4) }}</div>
                </div>
              </div>
              <div v-else class="flex h-32 items-center justify-center text-sm text-[var(--text-muted)]">{{ t.tt_noCoefficientData }}</div>
            </div>

            <div class="rounded-xl border border-[var(--border-default)] bg-[var(--bg-panel)] p-5 shadow-[var(--shadow-panel)]">
              <div class="mb-4 flex items-center gap-2">
                <Wind class="h-5 w-5 text-[var(--accent-primary)]" />
                <h3 class="text-base font-semibold text-[var(--text-primary)]">{{ t.tt_latestRawData }}</h3>
              </div>
              <div v-if="latestRawData" class="space-y-2">
                <div class="flex justify-between rounded-lg bg-[var(--bg-panel-strong)] px-3 py-2">
                  <span class="text-xs text-[var(--text-muted)]">{{ t.tt_totalTemp }}</span>
                  <span class="font-mono text-sm font-bold text-[var(--text-primary)]">{{ formatValue(latestRawData.tTotal, 3) }} °C</span>
                </div>
                <div class="flex justify-between rounded-lg bg-[var(--bg-panel-strong)] px-3 py-2">
                  <span class="text-xs text-[var(--text-muted)]">{{ t.tt_staticTemp }}</span>
                  <span class="font-mono text-sm font-bold text-[var(--text-primary)]">{{ formatValue(latestRawData.tStatic, 3) }} °C</span>
                </div>
                <div class="flex justify-between rounded-lg bg-[var(--bg-panel-strong)] px-3 py-2">
                  <span class="text-xs text-[var(--text-muted)]">{{ t.tt_ambientTemp }}</span>
                  <span class="font-mono text-sm font-bold text-[var(--text-primary)]">{{ formatValue(latestRawData.tAtm, 3) }} °C</span>
                </div>
              </div>
              <div v-else class="flex h-32 items-center justify-center text-sm text-[var(--text-muted)]">{{ t.tt_noRawData }}</div>
            </div>

            <div class="md:col-span-2 rounded-xl border border-[var(--border-default)] bg-[var(--bg-panel)] p-5 shadow-[var(--shadow-panel)]">
              <div class="mb-4 flex items-center gap-2">
                <Settings class="h-5 w-5 text-[var(--accent-primary)]" />
                <h3 class="text-base font-semibold text-[var(--text-primary)]">{{ t.tt_configInfo }}</h3>
              </div>
              <div class="grid grid-cols-2 md:grid-cols-4 gap-4 text-sm">
                <div>
                  <div class="text-xs text-[var(--text-muted)]">{{ t.tt_configName }}</div>
                  <div class="font-medium text-[var(--text-primary)]">{{ currentConfig?.name || t.unconfigured }}</div>
                </div>
                <div>
                  <div class="text-xs text-[var(--text-muted)]">{{ t.pointLayout }}</div>
                  <div class="font-medium text-[var(--text-primary)]">
                    <span v-if="totalTemperatureLayout">
                      Mach: {{ totalTemperatureLayout.min }}~{{ totalTemperatureLayout.max }} ({{ totalTemperatureLayout.step }})
                    </span>
                    <span v-else>{{ t.unconfigured }}</span>
                  </div>
                </div>
                <div>
                  <div class="text-xs text-[var(--text-muted)]">{{ t.totalPoints }}</div>
                  <div class="font-medium text-[var(--text-primary)]">{{ totalPoints }} {{ t.point }}</div>
                </div>
                <div>
                  <div class="text-xs text-[var(--text-muted)]">{{ t.tt_acquisitionParams }}</div>
                  <div class="font-medium text-[var(--text-primary)]">{{ currentConfig?.dwellTimeMs || 0 }}ms / {{ currentConfig?.samplesPerPoint || 0 }} {{ t.tt_timesUnit }}</div>
                </div>
              </div>
            </div>
          </div>
        </div>

        <div v-if="activeTab === 'chart'" class="flex-1 overflow-hidden p-5">
          <div class="h-full rounded-xl border border-[var(--border-default)] bg-[var(--bg-panel)] p-5 shadow-[var(--shadow-panel)]">
            <h3 class="mb-4 text-base font-semibold text-[var(--text-primary)]">{{ t.tt_recoveryCoeffMachCurve }}</h3>
            <canvas ref="recoveryCanvas" class="h-[calc(100%-3rem)] w-full"></canvas>
          </div>
        </div>

        <div v-if="activeTab === 'data'" class="flex-1 overflow-auto p-5">
          <div class="rounded-xl border border-[var(--border-default)] bg-[var(--bg-panel)] p-5 shadow-[var(--shadow-panel)]">
            <div class="mb-4 flex items-center gap-2">
              <FileText class="h-5 w-5 text-[var(--accent-primary)]" />
              <h3 class="text-base font-semibold text-[var(--text-primary)]">{{ t.tt_calibrationDataRecords }}</h3>
              <span class="ml-auto text-sm text-[var(--text-muted)]">{{ recordCountText }}</span>
            </div>
            <div class="overflow-x-auto max-h-[calc(100vh-300px)]">
              <table class="w-full text-sm">
                <thead class="bg-[var(--bg-panel-strong)]">
                  <tr>
                    <th class="px-3 py-2 text-left text-xs font-medium text-[var(--text-muted)]">{{ t.tt_index }}</th>
                    <th class="px-3 py-2 text-left text-xs font-medium text-[var(--text-muted)]">Mach</th>
                    <th class="px-3 py-2 text-left text-xs font-medium text-[var(--text-muted)]">{{ t.tt_recoveryCoeff }}</th>
                    <th class="px-3 py-2 text-left text-xs font-medium text-[var(--text-muted)]">{{ t.tt_sampleCount }}</th>
                    <th class="px-3 py-2 text-left text-xs font-medium text-[var(--text-muted)]">{{ t.tt_stdDev }}</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-[var(--border-default)]">
                  <tr v-for="(point, index) in calibrationStore.dataPoints" :key="index" class="hover:bg-[var(--bg-panel-strong)]">
                    <td class="px-3 py-2 font-mono text-[var(--text-primary)]">{{ index + 1 }}</td>
                    <td class="px-3 py-2 font-mono text-[var(--text-primary)]">{{ isTotalTemperatureDataPoint(point) ? formatValue(point.targetMachNumber, 2) : '--' }}</td>
                    <td class="px-3 py-2 font-mono text-[var(--text-primary)]">{{ isTotalTemperatureDataPoint(point) ? formatValue(point.recoveryCoefficient, 4) : '--' }}</td>
                    <td class="px-3 py-2 font-mono text-[var(--text-primary)]">{{ 'sampleCount' in point ? point.sampleCount : '--' }}</td>
                    <td class="px-3 py-2 font-mono text-[var(--text-primary)]">{{ formatValue(point.stdDev, 4) }}</td>
                  </tr>
                </tbody>
              </table>
              <div v-if="calibrationStore.dataPoints.length === 0" class="py-8 text-center text-sm text-[var(--text-muted)]">{{ t.tt_noDataRecords }}</div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

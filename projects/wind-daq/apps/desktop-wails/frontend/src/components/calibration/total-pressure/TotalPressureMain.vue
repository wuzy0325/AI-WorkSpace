<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { useCalibrationStore } from '@stores/calibrationStore'
import { useDeviceStore } from '@stores/deviceStore'
import { useMotionStore } from '@stores/motionStore'
import { useFeedbackStore } from '@stores/feedbackStore'
import { useCalibrationWorkflow } from '@composables/useCalibrationWorkflow'
import type { CalibrationConfig, TotalPressureDataPoint } from '@shared/types/calibration'
import { getProbeChannelPrecision } from '@shared/calibrationPrecision'
import { isTotalPressureDataPoint } from '@shared/calibrationDataGuards'
import UiButton from '@components/ui/UiButton.vue'
import {
  Play, Pause, Square, Settings, ArrowLeft, Save, FileText, RotateCcw,
  ChevronDown, ChevronUp, Activity, Gauge, Wind, Timer, Target, TrendingUp
} from '@lucide/vue'
import { NButton } from 'naive-ui'

const emit = defineEmits<{
  back: []
  openSettings: []
}>()

const calibrationStore = useCalibrationStore()
const deviceStore = useDeviceStore()
const motionStore = useMotionStore()
const feedbackStore = useFeedbackStore()

const workflow = useCalibrationWorkflow('total-pressure')
const sphereTankGate = workflow.sphereTankGate

const isLoading = ref(true)
const showChannelPanel = ref(true)
const activeTab = ref<'overview' | 'chart' | 'data'>('overview')
const kAlphaCanvas = ref<HTMLCanvasElement | null>(null)
const chartUpdateTimer = ref<ReturnType<typeof setInterval> | null>(null)

const currentConfig = computed(() => workflow.currentConfig.value)
const progressInfo = computed(() => workflow.progressInfo.value)
const formattedTimeInfo = computed(() => workflow.formattedTimeInfo.value)
const canStartCalibration = computed(() => workflow.canStartCalibration.value)
const startDisabledReason = computed(() => workflow.startDisabledReason.value)

const currentAlpha = computed(() => {
  const coords = progressInfo.value?.currentPoint
  return coords && typeof coords.α === 'number' ? coords.α : null
})

const latestCoefficients = computed(() => {
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
    pTotal: lastPoint.rawData.pProbeTotal,
    pStatic: lastPoint.rawData.pTunnelStatic,
    pAtm: lastPoint.rawData.pAtm
  }
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

const totalPressureLayout = computed(() => {
  return currentConfig.value?.totalPressureLayout
})

function formatValue(value: number | undefined | null, precision?: number): string {
  if (value === undefined || value === null) return '--'
  return value.toFixed(precision ?? 3)
}

function getChannelValue(role: string): string {
  const pressures = calibrationStore.realtimePressures
  if (!pressures) return '--'
  switch (role) {
    case 'totalPressure.pTotal': return formatValue(pressures.P0, getProbeChannelPrecision(currentConfig.value, role))
    case 'totalPressure.pStatic': return formatValue(pressures.Patm, getProbeChannelPrecision(currentConfig.value, role))
    case 'totalPressure.pAtm': return formatValue(pressures.Patm, getProbeChannelPrecision(currentConfig.value, role))
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

function drawChart() {
  const canvas = kAlphaCanvas.value
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
    .filter((p): p is TotalPressureDataPoint => isTotalPressureDataPoint(p))
    .map((p) => ({ x: p.alpha, y: p.coefficients.CPT }))

  if (dataPoints.length === 0) {
    ctx.fillStyle = '#64748b'
    ctx.font = '14px sans-serif'
    ctx.textAlign = 'center'
    ctx.textBaseline = 'middle'
    ctx.fillText('暂无数据', width / 2, height / 2)
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
      feedbackStore.pushToast('请先配置总压探针校准参数', 'warning')
    }
  } finally {
    isLoading.value = false
  }
})

onUnmounted(() => {
  stopChartUpdate()
})
</script>

<template>
  <div data-test="total-pressure-main-shell" class="flex h-full flex-col bg-[var(--bg-canvas)] text-[var(--text-primary)]">
    <div class="flex items-center justify-between border-b border-[var(--border-default)] bg-[var(--bg-panel)] px-5 py-3">
      <div class="flex items-center gap-3">
        <UiButton variant="secondary" size="sm" @click="emit('back')">
          <ArrowLeft class="h-4 w-4" />
        </UiButton>
        <div>
          <h1 class="text-base font-bold text-[var(--text-primary)]">总压探针校准</h1>
          <p class="text-xs text-[var(--text-muted)]">Total Pressure Probe Calibration</p>
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
        <UiButton variant="secondary" size="sm" :disabled="!canSave" @click="workflow.exportReport">
          <FileText class="h-4 w-4" />
          <span class="ml-1">导出</span>
        </UiButton>
      </div>
    </div>

    <div class="flex flex-1 overflow-hidden">
      <!-- 左侧边栏：固定宽度 320px，可滚动 -->
      <div class="flex w-80 flex-col border-r border-[var(--border-default)] bg-[var(--bg-panel)] overflow-y-auto flex-shrink-0">
        <div class="border-b border-[var(--border-default)] p-4">
          <div class="mb-3 flex items-center justify-between">
            <span class="text-sm text-[var(--text-muted)]">状态</span>
            <span class="rounded-full px-2 py-0.5 text-xs font-medium"
              :class="{
                'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400': statusColor === 'success',
                'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400': statusColor === 'warning',
                'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400': statusColor === 'info',
                'bg-slate-100 text-slate-700 dark:bg-slate-800 dark:text-slate-400': statusColor === 'normal',
              }"
            >{{ statusText }}</span>
          </div>
          <div class="mb-2">
            <div class="mb-1 flex justify-between text-xs">
              <span class="text-[var(--text-muted)]">进度</span>
              <span class="text-[var(--text-primary)]">{{ formattedProgress }}</span>
            </div>
            <div class="h-2 overflow-hidden rounded-full bg-[var(--bg-panel-strong)]">
              <div class="h-full rounded-full bg-gradient-to-r from-emerald-500 to-teal-600 transition-all duration-300" :style="{ width: progressPercent + '%' }"></div>
            </div>
          </div>
          <div v-if="formattedTimeInfo" class="grid grid-cols-2 gap-2 text-xs">
            <div class="rounded-lg bg-[var(--bg-panel-strong)] p-2">
              <div class="text-[var(--text-muted)]">已用时间</div>
              <div class="font-mono font-bold text-[var(--text-primary)]">{{ formattedTimeInfo.elapsed }}</div>
            </div>
            <div class="rounded-lg bg-[var(--bg-panel-strong)] p-2">
              <div class="text-[var(--text-muted)]">预计剩余</div>
              <div class="font-mono font-bold text-[var(--text-primary)]">{{ formattedTimeInfo.remaining }}</div>
            </div>
          </div>
        </div>

        <div class="border-b border-[var(--border-default)] p-4">
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
          <div v-if="startDisabledReason" class="mt-2 text-xs text-amber-500">{{ startDisabledReason }}</div>
        </div>

        <div class="border-b border-[var(--border-default)] p-4">
          <div class="mb-2 flex items-center gap-2 text-sm font-medium text-[var(--text-primary)]">
            <Target class="h-4 w-4 text-[var(--accent-primary)]" />
            当前位置
          </div>
          <div v-if="currentAlpha !== null" class="rounded-lg bg-[var(--bg-panel-strong)] p-3 text-center">
            <div class="text-xs text-[var(--text-muted)]">攻角 α</div>
            <div class="font-mono text-lg font-bold text-[var(--accent-primary)]">{{ currentAlpha.toFixed(1) }}°</div>
          </div>
          <div v-else class="rounded-lg bg-[var(--bg-panel-strong)] p-3 text-center text-sm text-[var(--text-muted)]">
            未开始校准
          </div>
        </div>

        <div class="border-b border-[var(--border-default)] p-4">
          <div class="mb-2 flex items-center gap-2 text-sm font-medium text-[var(--text-primary)]">
            <Activity class="h-4 w-4 text-[var(--accent-primary)]" />
            球罐判定门控
          </div>
          <div class="flex items-center justify-between rounded-lg bg-[var(--bg-panel-strong)] p-3">
            <div>
              <div class="text-xs text-[var(--text-muted)]">状态</div>
              <div class="text-sm font-medium" :class="sphereTankGate.isActive.value ? 'text-emerald-500' : 'text-[var(--text-muted)]'">
                {{ sphereTankGate.isActive.value ? '已激活' : sphereTankGate.statusText.value }}
              </div>
            </div>
            <div class="text-right">
              <div class="text-xs text-[var(--text-muted)]">等待时间</div>
              <div class="font-mono text-sm text-[var(--text-primary)]">{{ sphereTankGate.waitTimeSec.value }}s</div>
            </div>
          </div>
        </div>

        <div class="flex-1 overflow-auto">
          <div class="flex cursor-pointer items-center justify-between border-b border-[var(--border-default)] bg-[var(--bg-panel-strong)] p-3" @click="showChannelPanel = !showChannelPanel">
            <div class="flex items-center gap-2 text-sm font-medium text-[var(--text-primary)]">
              <Gauge class="h-4 w-4 text-[var(--accent-primary)]" />
              实时通道数据
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
          <NButton quaternary size="small" class="relative px-5 py-3 text-sm font-medium transition-colors" :class="activeTab === 'overview' ? 'text-[var(--accent-primary)]' : 'text-[var(--text-muted)] hover:text-[var(--text-primary)]'" @click="activeTab = 'overview'">
            <template #icon>
              <Activity class="h-4 w-4" />
            </template>
            概览
            <span v-if="activeTab === 'overview'" class="absolute bottom-0 left-0 right-0 h-0.5 bg-[var(--accent-primary)] rounded-t-full"></span>
          </NButton>
          <NButton quaternary size="small" class="relative px-5 py-3 text-sm font-medium transition-colors" :class="activeTab === 'chart' ? 'text-[var(--accent-primary)]' : 'text-[var(--text-muted)] hover:text-[var(--text-primary)]'" @click="activeTab = 'chart'">
            <template #icon>
              <TrendingUp class="h-4 w-4" />
            </template>
            图表
            <span v-if="activeTab === 'chart'" class="absolute bottom-0 left-0 right-0 h-0.5 bg-[var(--accent-primary)] rounded-t-full"></span>
          </NButton>
          <NButton quaternary size="small" class="relative px-5 py-3 text-sm font-medium transition-colors" :class="activeTab === 'data' ? 'text-[var(--accent-primary)]' : 'text-[var(--text-muted)] hover:text-[var(--text-primary)]'" @click="activeTab = 'data'">
            <template #icon>
              <FileText class="h-4 w-4" />
            </template>
            数据
            <span v-if="activeTab === 'data'" class="absolute bottom-0 left-0 right-0 h-0.5 bg-[var(--accent-primary)] rounded-t-full"></span>
          </NButton>
        </div>

        <div v-if="activeTab === 'overview'" class="flex-1 overflow-auto p-5">
          <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div class="rounded-xl border border-[var(--border-default)] bg-[var(--bg-panel)] p-5 shadow-[var(--shadow-panel)]">
              <div class="mb-4 flex items-center gap-2">
                <TrendingUp class="h-5 w-5 text-[var(--accent-primary)]" />
                <h3 class="text-base font-semibold text-[var(--text-primary)]">最新系数</h3>
              </div>
              <div v-if="latestCoefficients" class="grid grid-cols-2 gap-3">
                <div class="rounded-lg bg-[var(--bg-panel-strong)] p-3">
                  <div class="text-xs text-[var(--text-muted)]">CPT</div>
                  <div class="font-mono text-lg font-bold text-[var(--accent-primary)]">{{ formatValue(latestCoefficients.CPT, 4) }}</div>
                </div>
                <div class="rounded-lg bg-[var(--bg-panel-strong)] p-3">
                  <div class="text-xs text-[var(--text-muted)]">误差</div>
                  <div class="font-mono text-lg font-bold text-[var(--accent-primary)]">{{ formatValue(latestCoefficients.error, 4) }}</div>
                </div>
                <div class="rounded-lg bg-[var(--bg-panel-strong)] p-3">
                  <div class="text-xs text-[var(--text-muted)]">马赫数</div>
                  <div class="font-mono text-lg font-bold text-[var(--accent-primary)]">{{ formatValue(latestCoefficients.machNumber, 4) }}</div>
                </div>
              </div>
              <div v-else class="flex h-32 items-center justify-center text-sm text-[var(--text-muted)]">暂无系数数据</div>
            </div>

            <div class="rounded-xl border border-[var(--border-default)] bg-[var(--bg-panel)] p-5 shadow-[var(--shadow-panel)]">
              <div class="mb-4 flex items-center gap-2">
                <Wind class="h-5 w-5 text-[var(--accent-primary)]" />
                <h3 class="text-base font-semibold text-[var(--text-primary)]">最新原始数据</h3>
              </div>
              <div v-if="latestRawData" class="space-y-2">
                <div class="flex justify-between rounded-lg bg-[var(--bg-panel-strong)] px-3 py-2">
                  <span class="text-xs text-[var(--text-muted)]">总压</span>
                  <span class="font-mono text-sm font-bold text-[var(--text-primary)]">{{ formatValue(latestRawData.pTotal, 3) }} Pa</span>
                </div>
                <div class="flex justify-between rounded-lg bg-[var(--bg-panel-strong)] px-3 py-2">
                  <span class="text-xs text-[var(--text-muted)]">静压</span>
                  <span class="font-mono text-sm font-bold text-[var(--text-primary)]">{{ formatValue(latestRawData.pStatic, 3) }} Pa</span>
                </div>
                <div class="flex justify-between rounded-lg bg-[var(--bg-panel-strong)] px-3 py-2">
                  <span class="text-xs text-[var(--text-muted)]">大气压</span>
                  <span class="font-mono text-sm font-bold text-[var(--text-primary)]">{{ formatValue(latestRawData.pAtm, 3) }} Pa</span>
                </div>
              </div>
              <div v-else class="flex h-32 items-center justify-center text-sm text-[var(--text-muted)]">暂无原始数据</div>
            </div>

            <div class="md:col-span-2 rounded-xl border border-[var(--border-default)] bg-[var(--bg-panel)] p-5 shadow-[var(--shadow-panel)]">
              <div class="mb-4 flex items-center gap-2">
                <Settings class="h-5 w-5 text-[var(--accent-primary)]" />
                <h3 class="text-base font-semibold text-[var(--text-primary)]">配置信息</h3>
              </div>
              <div class="grid grid-cols-2 md:grid-cols-4 gap-4 text-sm">
                <div>
                  <div class="text-xs text-[var(--text-muted)]">配置名称</div>
                  <div class="font-medium text-[var(--text-primary)]">{{ currentConfig?.name || '未配置' }}</div>
                </div>
                <div>
                  <div class="text-xs text-[var(--text-muted)]">点位布局</div>
                  <div class="font-medium text-[var(--text-primary)]">
                    <span v-if="totalPressureLayout">
                      α: {{ totalPressureLayout.alphaMin }}°~{{ totalPressureLayout.alphaMax }}° ({{ totalPressureLayout.alphaStep }}°)
                    </span>
                    <span v-else>未配置</span>
                  </div>
                </div>
                <div>
                  <div class="text-xs text-[var(--text-muted)]">总点数</div>
                  <div class="font-medium text-[var(--text-primary)]">{{ totalPoints }} 点</div>
                </div>
                <div>
                  <div class="text-xs text-[var(--text-muted)]">采集参数</div>
                  <div class="font-medium text-[var(--text-primary)]">{{ currentConfig?.dwellTimeMs || 0 }}ms / {{ currentConfig?.samplesPerPoint || 0 }}次</div>
                </div>
              </div>
            </div>
          </div>
        </div>

        <div v-if="activeTab === 'chart'" class="flex-1 overflow-hidden p-5">
          <div class="h-full rounded-xl border border-[var(--border-default)] bg-[var(--bg-panel)] p-5 shadow-[var(--shadow-panel)]">
            <h3 class="mb-4 text-base font-semibold text-[var(--text-primary)]">K-α 曲线</h3>
            <canvas ref="kAlphaCanvas" class="h-[calc(100%-3rem)] w-full"></canvas>
          </div>
        </div>

        <div v-if="activeTab === 'data'" class="flex-1 overflow-auto p-5">
          <div class="rounded-xl border border-[var(--border-default)] bg-[var(--bg-panel)] p-5 shadow-[var(--shadow-panel)]">
            <div class="mb-4 flex items-center gap-2">
              <FileText class="h-5 w-5 text-[var(--accent-primary)]" />
              <h3 class="text-base font-semibold text-[var(--text-primary)]">校准数据记录</h3>
              <span class="ml-auto text-sm text-[var(--text-muted)]">共 {{ calibrationStore.dataPoints.length }} 条记录</span>
            </div>
            <div class="overflow-x-auto max-h-[calc(100vh-300px)]">
              <table class="w-full text-sm">
                <thead class="bg-[var(--bg-panel-strong)]">
                  <tr>
                    <th class="px-3 py-2 text-left text-xs font-medium text-[var(--text-muted)]">序号</th>
                    <th class="px-3 py-2 text-left text-xs font-medium text-[var(--text-muted)]">α</th>
                    <th class="px-3 py-2 text-left text-xs font-medium text-[var(--text-muted)]">K</th>
                    <th class="px-3 py-2 text-left text-xs font-medium text-[var(--text-muted)]">采样数</th>
                    <th class="px-3 py-2 text-left text-xs font-medium text-[var(--text-muted)]">标准差</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-[var(--border-default)]">
                  <tr v-for="(point, index) in calibrationStore.dataPoints" :key="index" class="hover:bg-[var(--bg-panel-strong)]">
                    <td class="px-3 py-2 font-mono text-[var(--text-primary)]">{{ index + 1 }}</td>
                    <td class="px-3 py-2 font-mono text-[var(--text-primary)]">{{ isTotalPressureDataPoint(point) ? formatValue(point.alpha, 1) : '--' }}</td>
                    <td class="px-3 py-2 font-mono text-[var(--text-primary)]">{{ isTotalPressureDataPoint(point) ? formatValue(point.coefficients.CPT, 4) : '--' }}</td>
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

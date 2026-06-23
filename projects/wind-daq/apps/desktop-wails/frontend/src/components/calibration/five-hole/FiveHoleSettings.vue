<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useDeviceStore } from '@stores/deviceStore'
import { useMotionStore } from '@stores/motionStore'
import { useFeedbackStore } from '@stores/feedbackStore'
import { useI18nStore } from '@stores/i18nStore'
import { calibrationApi } from '@api/calibrationApi'
import type {
  CalibrationConfig,
  ProbeChannelConfig,
  MotionAxisConfig,
  ChannelRef,
} from '@shared/types/calibration'
import {
  applyCalibrationPrecisionDefaults,
  DEFAULT_CALIBRATION_MACH_PRECISION,
  DEFAULT_CALIBRATION_PROBE_PRECISION,
  DEFAULT_CALIBRATION_VELOCITY_PRECISION,
} from '@shared/calibrationPrecision'
import { generateFiveHoleSnakePoints } from './motionCalibrationUtils'
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
} from '@lucide/vue'
import UiButton from '@components/ui/UiButton.vue'

const emit = defineEmits<{
  close: []
  saved: [config: CalibrationConfig]
}>()

const deviceStore = useDeviceStore()
const motionStore = useMotionStore()
const feedbackStore = useFeedbackStore()
const { t } = useI18nStore()

const isLoading = ref(true)
const isSaving = ref(false)
const currentStep = ref(0)
const steps = computed(() => [
  { label: t.stepBasic || '基本设置' },
  { label: t.stepHardware || '硬件配置' },
  { label: t.stepConfirm || '确认保存' },
])

const pointLayout = ref({
  alphaMin: -30,
  alphaMax: 30,
  alphaStep: 5,
  betaMin: -30,
  betaMax: 30,
  betaStep: 5,
})

const pointCount = computed(() => {
  const alphaPoints = Math.floor((pointLayout.value.alphaMax - pointLayout.value.alphaMin) / pointLayout.value.alphaStep) + 1
  const betaPoints = Math.floor((pointLayout.value.betaMax - pointLayout.value.betaMin) / pointLayout.value.betaStep) + 1
  return alphaPoints * betaPoints
})

const dwellTimeMs = ref(2000)
const samplesPerPoint = ref(10)
const calibrationName = ref('')
const sphereTankGateEnabled = ref(false)
const sphereTankWaitTimeSec = ref(3)
const sphereTankStableChannel = ref<ChannelRef>({ deviceId: '', channelIndex: 0 })
const machNumberPrecision = ref<number>(DEFAULT_CALIBRATION_MACH_PRECISION)
const velocityPrecision = ref<number>(DEFAULT_CALIBRATION_VELOCITY_PRECISION)

const probeChannels = ref<ProbeChannelConfig[]>([
  { name: 'P1 (Lower)', role: 'fiveHole.p1', channel: { deviceId: '', channelIndex: 0 }, enabled: true, precision: DEFAULT_CALIBRATION_PROBE_PRECISION },
  { name: 'P2 (Center)', role: 'fiveHole.p2', channel: { deviceId: '', channelIndex: 1 }, enabled: true, precision: DEFAULT_CALIBRATION_PROBE_PRECISION },
  { name: 'P3 (Upper)', role: 'fiveHole.p3', channel: { deviceId: '', channelIndex: 2 }, enabled: true, precision: DEFAULT_CALIBRATION_PROBE_PRECISION },
  { name: 'P4 (Left)', role: 'fiveHole.p4', channel: { deviceId: '', channelIndex: 3 }, enabled: true, precision: DEFAULT_CALIBRATION_PROBE_PRECISION },
  { name: 'P5 (Right)', role: 'fiveHole.p5', channel: { deviceId: '', channelIndex: 4 }, enabled: true, precision: DEFAULT_CALIBRATION_PROBE_PRECISION },
  { name: 'Atm Pressure', role: 'fiveHole.pAtm', channel: { deviceId: '', channelIndex: 16 }, enabled: true, precision: DEFAULT_CALIBRATION_PROBE_PRECISION },
  { name: 'Atm Temp', role: 'fiveHole.tAtm', channel: { deviceId: '', channelIndex: 17 }, enabled: true, precision: DEFAULT_CALIBRATION_PROBE_PRECISION },
  { name: 'Tunnel Total Pressure', role: 'fiveHole.pTotal', channel: { deviceId: '', channelIndex: -1 }, enabled: true, precision: DEFAULT_CALIBRATION_PROBE_PRECISION },
  { name: 'Tunnel Static Pressure', role: 'fiveHole.pTunnelStatic', channel: { deviceId: '', channelIndex: -1 }, enabled: true, precision: DEFAULT_CALIBRATION_PROBE_PRECISION },
  { name: 'Tunnel Temperature', role: 'fiveHole.tTunnel', channel: { deviceId: '', channelIndex: -1 }, enabled: true, precision: DEFAULT_CALIBRATION_PROBE_PRECISION },
])

const motionAxes = ref<MotionAxisConfig[]>([
  { name: 'Alpha', controllerId: '', axis: 'X' },
  { name: 'Beta', controllerId: '', axis: 'Y' },
])

const deviceList = computed(() => deviceStore.profiles)
const motionControllerList = computed(() => motionStore.profiles)

const REQUIRED_CHANNEL_ROLES = [
  'fiveHole.p1', 'fiveHole.p2', 'fiveHole.p3', 'fiveHole.p4', 'fiveHole.p5',
  'fiveHole.pAtm', 'fiveHole.tAtm', 'fiveHole.pTotal', 'fiveHole.pTunnelStatic', 'fiveHole.tTunnel',
] as const

// 通道分组：探针五孔（P1-P5）
const probeGroupChannels = computed(() =>
  probeChannels.value.filter((ch) =>
    ['fiveHole.p1', 'fiveHole.p2', 'fiveHole.p3', 'fiveHole.p4', 'fiveHole.p5'].includes(ch.role || ''),
  ),
)

// 通道分组：大气环境
const atmosphereGroupChannels = computed(() =>
  probeChannels.value.filter((ch) =>
    ['fiveHole.pAtm', 'fiveHole.tAtm'].includes(ch.role || ''),
  ),
)

// 通道分组：风洞参数
const windTunnelGroupChannels = computed(() =>
  probeChannels.value.filter((ch) =>
    ['fiveHole.pTotal', 'fiveHole.pTunnelStatic', 'fiveHole.tTunnel'].includes(ch.role || ''),
  ),
)

// 通道映射进度：已正确映射的通道数 / 必需通道总数
const mappedChannelCount = computed(
  () =>
    probeChannels.value.filter(
      (ch) => ch.enabled && ch.channel.deviceId && ch.channel.channelIndex >= 0,
    ).length,
)
const totalRequiredChannelCount = REQUIRED_CHANNEL_ROLES.length

// 点阵预览：将 α-β 网格归一化到 SVG 坐标（viewBox 200×200，绘图区 20~180）
const previewDots = computed<{ cx: number; cy: number }[]>(() => {
  const { alphaMin, alphaMax, alphaStep, betaMin, betaMax, betaStep } = pointLayout.value
  if (alphaStep <= 0 || betaStep <= 0 || alphaMax <= alphaMin || betaMax <= betaMin) {
    return []
  }
  const xRange = alphaMax - alphaMin
  const yRange = betaMax - betaMin
  const dots: { cx: number; cy: number }[] = []
  // 浮点累加避免步长精度问题
  for (let a = alphaMin; a <= alphaMax + 1e-6; a += alphaStep) {
    for (let b = betaMin; b <= betaMax + 1e-6; b += betaStep) {
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
    if (calibrationName.value.trim() === '') errors.push(t.enterConfigName || '请输入配置名称')
    if (pointLayout.value.alphaStep <= 0 || pointLayout.value.betaStep <= 0) errors.push(t.stepMustPositive || '步长必须为正数')
    if (pointLayout.value.alphaMax <= pointLayout.value.alphaMin || pointLayout.value.betaMax <= pointLayout.value.betaMin) errors.push(t.maxGreaterThanMin || '最大值必须大于最小值')
    const alphaRange = pointLayout.value.alphaMax - pointLayout.value.alphaMin
    const betaRange = pointLayout.value.betaMax - pointLayout.value.betaMin
    if (alphaRange % pointLayout.value.alphaStep !== 0 || betaRange % pointLayout.value.betaStep !== 0) errors.push(t.rangeDivisible || '范围必须能被步长整除')
    if (dwellTimeMs.value < 100) errors.push(t.dwellTimeMin || '驻留时间至少100ms')
    if (samplesPerPoint.value < 1) errors.push(t.samplesMin || '采样次数至少为1')
    return errors
  }
  if (currentStep.value === 1) {
    const errors: string[] = []
    const enabledRoles = new Set(probeChannels.value.filter((ch) => ch.enabled).map((ch) => ch.role))
    const missingRoles = REQUIRED_CHANNEL_ROLES.filter((role) => !enabledRoles.has(role))
    if (missingRoles.length > 0) errors.push(t.requiredChannelsMissing || '缺少必需的通道')
    const invalidChannel = probeChannels.value.find((ch) => ch.enabled && (!ch.channel.deviceId || ch.channel.channelIndex < 0))
    if (invalidChannel) errors.push(`${t.channelMapIncomplete || '通道映射不完整'}: ${invalidChannel.name}`)
    const missingControllerAxis = motionAxes.value.find((axis) => !axis.controllerId)
    if (missingControllerAxis) errors.push(`${t.axisNotBound || '轴未绑定'}: ${missingControllerAxis.name}`)
    if (sphereTankGateEnabled.value && !sphereTankStableChannel.value.deviceId) errors.push(t.sphereTankRequiresDevice || '球罐门控需要选择设备')
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

function generatePoints() { return generateFiveHoleSnakePoints(pointLayout.value) }

async function saveConfig() {
  isSaving.value = true
  try {
    const config: CalibrationConfig = {
      type: 'five-hole',
      name: calibrationName.value,
      probeChannels: probeChannels.value.filter((ch) => ch.enabled),
      motionAxes: motionAxes.value,
      points: await generatePoints(),
      dwellTimeMs: dwellTimeMs.value,
      samplesPerPoint: samplesPerPoint.value,
      savePath: '',
      fiveHoleLayout: pointLayout.value,
      derivedValuePrecision: { machNumber: machNumberPrecision.value, velocity: velocityPrecision.value },
      sphereTankGate: {
        enabled: sphereTankGateEnabled.value,
        waitTimeSec: Math.max(0, sphereTankWaitTimeSec.value),
        stableTimeChannel: { ...sphereTankStableChannel.value },
      },
    }
    const normalizedConfig = applyCalibrationPrecisionDefaults(config)
    const res = await calibrationApi.saveConfig('five-hole', normalizedConfig)
    if (!res.success) throw new Error(res.error || '保存失败')
    emit('saved', normalizedConfig)
    emit('close')
  } catch (err) {
    feedbackStore.pushToast('保存失败: ' + (err instanceof Error ? err.message : String(err)), 'error')
  } finally { isSaving.value = false }
}

async function loadSavedConfig() {
  try {
    const res = await calibrationApi.getConfig('five-hole')
    const config = res.success && res.data ? applyCalibrationPrecisionDefaults(res.data) : null
    if (!config) return
    calibrationName.value = config.name
    if (config.fiveHoleLayout) pointLayout.value = { ...config.fiveHoleLayout }
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
    dwellTimeMs.value = config.dwellTimeMs
    samplesPerPoint.value = config.samplesPerPoint
    machNumberPrecision.value = config.derivedValuePrecision?.machNumber ?? DEFAULT_CALIBRATION_MACH_PRECISION
    velocityPrecision.value = config.derivedValuePrecision?.velocity ?? DEFAULT_CALIBRATION_VELOCITY_PRECISION
    if (config.sphereTankGate) {
      sphereTankGateEnabled.value = config.sphereTankGate.enabled
      sphereTankWaitTimeSec.value = config.sphereTankGate.waitTimeSec
      sphereTankStableChannel.value = { ...config.sphereTankGate.stableTimeChannel }
    }
  } catch { /* ignore */ }
}

onMounted(async () => {
  try {
    await Promise.all([deviceStore.refreshProfiles(), motionStore.refreshProfiles(), loadSavedConfig()])
    if (!calibrationName.value) calibrationName.value = `五孔探针-${new Date().toLocaleDateString()}`
  } finally { isLoading.value = false }
})

const axisOptions = [
  { label: 'X 轴', value: 'X' },
  { label: 'Y 轴', value: 'Y' },
  { label: 'Z 轴', value: 'Z' },
  { label: 'U 轴', value: 'U' },
]

// 通道表格列模板（三个分组共用）
const channelColumns = [
  { key: 'enabled', label: '启用', width: '48px' },
  { key: 'name', label: '名称', width: '' },
  { key: 'device', label: '数据源', width: '' },
  { key: 'channel', label: '通道', width: '100px' },
  { key: 'precision', label: '精度', width: '80px' },
] as const
</script>

<template>
  <UiDialog :show="true" width="960px" title="五孔探针校准配置" closable @update:show="emit('close')">
    <UiSteps :current="currentStep" class="steps-mb">
      <UiStep v-for="(step, idx) in steps" :key="idx" :title="step.label" :disabled="idx > currentStep" />
    </UiSteps>

    <UiSpin v-if="isLoading" class="spinner" />

    <template v-else>
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

      <!-- 步骤 1：基本设置 -->
      <div v-if="currentStep === 0" class="step-content">
        <UiPanel class="section-card">
          <template #header>
            <div class="section-header">
              <FileText :size="14" />
              <span>配置名称</span>
            </div>
          </template>
          <UiInput v-model="calibrationName" placeholder="例如：五孔探针-2026-001" />
        </UiPanel>

        <UiPanel class="section-card">
          <template #header>
            <div class="section-header">
              <Move3D :size="14" />
              <span>点位布局</span>
            </div>
          </template>
          <div class="layout-grid">
            <!-- 左侧：α / β 参数（扁平布局，避免卡片套卡片） -->
            <div class="layout-params">
              <div class="angle-block">
                <div class="angle-block-header">
                  <span class="angle-tag alpha-tag">α</span>
                  <span class="angle-block-title">攻角范围</span>
                </div>
                <div class="angle-fields">
                  <div class="field">
                    <span class="field-label">最小 (°)</span>
                    <UiInputNumber v-model="pointLayout.alphaMin" style="width:100%" />
                  </div>
                  <div class="field">
                    <span class="field-label">最大 (°)</span>
                    <UiInputNumber v-model="pointLayout.alphaMax" style="width:100%" />
                  </div>
                  <div class="field">
                    <span class="field-label">步长 (°)</span>
                    <UiInputNumber v-model="pointLayout.alphaStep" :min="1" style="width:100%" />
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
                    <UiInputNumber v-model="pointLayout.betaMin" style="width:100%" />
                  </div>
                  <div class="field">
                    <span class="field-label">最大 (°)</span>
                    <UiInputNumber v-model="pointLayout.betaMax" style="width:100%" />
                  </div>
                  <div class="field">
                    <span class="field-label">步长 (°)</span>
                    <UiInputNumber v-model="pointLayout.betaStep" :min="1" style="width:100%" />
                  </div>
                </div>
              </div>
            </div>
            <!-- 右侧：点阵预览，直观展示 α-β 网格分布 -->
            <div class="layout-preview">
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
          </div>
        </UiPanel>

        <!-- 采集参数 / 输出精度：双列面板，职责清晰分离 -->
        <div class="two-col-panels">
          <UiPanel class="section-card">
            <template #header>
              <div class="section-header">
                <Timer :size="14" />
                <span>采集参数</span>
              </div>
            </template>
            <div class="param-grid-2">
              <div class="field">
                <span class="field-label">驻留时间 (ms)</span>
                <UiInputNumber v-model="dwellTimeMs" :min="100" :step="100" style="width:100%" />
              </div>
              <div class="field">
                <span class="field-label">每点采样数</span>
                <UiInputNumber v-model="samplesPerPoint" :min="1" :max="1000" style="width:100%" />
              </div>
            </div>
          </UiPanel>

          <UiPanel class="section-card">
            <template #header>
              <div class="section-header">
                <Target :size="14" />
                <span>输出精度</span>
              </div>
            </template>
            <div class="param-grid-2">
              <div class="field">
                <span class="field-label">马赫数精度</span>
                <UiInputNumber v-model="machNumberPrecision" :min="0" :max="8" style="width:100%" />
              </div>
              <div class="field">
                <span class="field-label">流速精度 (m/s)</span>
                <UiInputNumber v-model="velocityPrecision" :min="0" :max="8" style="width:100%" />
              </div>
            </div>
          </UiPanel>
        </div>
      </div>

      <!-- 步骤 2：硬件配置 -->
      <div v-if="currentStep === 1" class="step-content">
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

        <!-- 探针五孔 -->
        <UiPanel class="section-card">
          <template #header>
            <div class="section-header">
              <Activity :size="14" />
              <span>探针五孔</span>
              <span class="section-count">{{ probeGroupChannels.length }} 通道</span>
            </div>
          </template>
          <div class="table-wrap">
            <table class="ntable">
              <thead>
                <tr>
                  <th v-for="col in channelColumns" :key="col.key" :style="col.width ? { width: col.width } : {}">{{ col.label }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="ch in probeGroupChannels" :key="ch.name">
                  <td class="cell-center"><UiCheckbox v-model:checked="ch.enabled" /></td>
                  <td><span class="cell-name">{{ ch.name }}</span></td>
                  <td>
                    <UiSelect
                      v-model="ch.channel.deviceId"
                      :options="deviceList.map(d => ({ label: `${d.name} (${d.type})`, value: d.id }))"
                      placeholder="选择设备"
                      :disabled="!ch.enabled"
                    />
                  </td>
                  <td><UiInputNumber v-model="ch.channel.channelIndex" :min="-1" :max="100" style="width:100%" :disabled="!ch.enabled" /></td>
                  <td><UiInputNumber v-model="ch.precision" :min="0" :max="8" style="width:100%" :disabled="!ch.enabled" /></td>
                </tr>
              </tbody>
            </table>
          </div>
        </UiPanel>

        <!-- 大气环境 -->
        <UiPanel class="section-card">
          <template #header>
            <div class="section-header">
              <Wind :size="14" />
              <span>大气环境</span>
              <span class="section-count">{{ atmosphereGroupChannels.length }} 通道</span>
            </div>
          </template>
          <div class="table-wrap">
            <table class="ntable">
              <thead>
                <tr>
                  <th v-for="col in channelColumns" :key="col.key" :style="col.width ? { width: col.width } : {}">{{ col.label }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="ch in atmosphereGroupChannels" :key="ch.name">
                  <td class="cell-center"><UiCheckbox v-model:checked="ch.enabled" /></td>
                  <td><span class="cell-name">{{ ch.name }}</span></td>
                  <td>
                    <UiSelect
                      v-model="ch.channel.deviceId"
                      :options="deviceList.map(d => ({ label: `${d.name} (${d.type})`, value: d.id }))"
                      placeholder="选择设备"
                      :disabled="!ch.enabled"
                    />
                  </td>
                  <td><UiInputNumber v-model="ch.channel.channelIndex" :min="-1" :max="100" style="width:100%" :disabled="!ch.enabled" /></td>
                  <td><UiInputNumber v-model="ch.precision" :min="0" :max="8" style="width:100%" :disabled="!ch.enabled" /></td>
                </tr>
              </tbody>
            </table>
          </div>
        </UiPanel>

        <!-- 风洞参数 -->
        <UiPanel class="section-card">
          <template #header>
            <div class="section-header">
              <Gauge :size="14" />
              <span>风洞参数</span>
              <span class="section-count">{{ windTunnelGroupChannels.length }} 通道</span>
            </div>
          </template>
          <div class="table-wrap">
            <table class="ntable">
              <thead>
                <tr>
                  <th v-for="col in channelColumns" :key="col.key" :style="col.width ? { width: col.width } : {}">{{ col.label }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="ch in windTunnelGroupChannels" :key="ch.name">
                  <td class="cell-center"><UiCheckbox v-model:checked="ch.enabled" /></td>
                  <td><span class="cell-name">{{ ch.name }}</span></td>
                  <td>
                    <UiSelect
                      v-model="ch.channel.deviceId"
                      :options="deviceList.map(d => ({ label: `${d.name} (${d.type})`, value: d.id }))"
                      placeholder="选择设备"
                      :disabled="!ch.enabled"
                    />
                  </td>
                  <td><UiInputNumber v-model="ch.channel.channelIndex" :min="-1" :max="100" style="width:100%" :disabled="!ch.enabled" /></td>
                  <td><UiInputNumber v-model="ch.precision" :min="0" :max="8" style="width:100%" :disabled="!ch.enabled" /></td>
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
              <thead><tr><th>坐标轴</th><th>运动控制器</th><th>物理轴</th></tr></thead>
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
              <UiInputNumber v-model="sphereTankWaitTimeSec" :min="0" :step="0.1" style="width:100%" />
            </div>
            <div class="field">
              <span class="field-label">稳定通道设备</span>
              <UiSelect v-model="sphereTankStableChannel.deviceId" :options="deviceList.map(d => ({ label: d.name, value: d.id }))" placeholder="选择设备" />
            </div>
            <div class="field">
              <span class="field-label">稳定通道索引</span>
              <UiInputNumber v-model="sphereTankStableChannel.channelIndex" :min="0" style="width:100%" />
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
          </div>
        </UiPanel>
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
/* 步骤内容容器：整体降一档间距 */
.step-content {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.section-card {
  font-size: var(--text-sm);
}

/* 区块标题：图标 + 文字 + 计数 */
.section-header {
  display: flex;
  align-items: center;
  gap: var(--space-1-5);
  font-size: var(--text-sm);
  font-weight: 600;
  color: var(--text-primary);
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
  gap: 2px;
}

.field-label {
  font-size: var(--text-xs);
  color: var(--text-tertiary);
}

/* 点位布局：左侧参数 + 右侧预览 */
.layout-grid {
  display: grid;
  grid-template-columns: 1fr 180px;
  gap: var(--space-3);
  align-items: start;
}

.layout-params {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
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
  gap: var(--space-1-5);
}

/* 点阵预览 */
.layout-preview {
  display: flex;
  flex-direction: column;
  gap: var(--space-1-5);
  padding: var(--space-2);
  border-radius: var(--radius-md);
  border: 1px solid var(--border-default);
  background: var(--bg-panel-strong);
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
  width: 148px;
  height: 148px;
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

/* 双列面板：采集参数 + 输出精度 */
.two-col-panels {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--space-2);
}

.param-grid-2 {
  display: grid;
  grid-template-columns: 1fr 1fr;
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

.ntable td {
  padding: var(--space-1) 8px;
  border-bottom: 1px solid var(--border-default);
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
}

/* 球罐判定门控 */
.header-toggle {
  margin-left: auto;
  font-size: var(--text-xs);
}

.sphere-grid {
  display: grid;
  grid-template-columns: 1fr 1fr 1fr;
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

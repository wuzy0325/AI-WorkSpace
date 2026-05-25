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
  FiveHolePointLayout,
  ChannelRef,
} from '@shared/types/calibration'
import {
  applyCalibrationPrecisionDefaults,
  DEFAULT_CALIBRATION_MACH_PRECISION,
  DEFAULT_CALIBRATION_PROBE_PRECISION,
  DEFAULT_CALIBRATION_VELOCITY_PRECISION,
} from '@shared/calibrationPrecision'
import { generateFiveHoleSnakePoints } from './motionCalibrationUtils'
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
const steps = computed(() => [t.stepBasic || '基本设置', t.stepHardware || '硬件配置', t.stepConfirm || '确认保存'])

const pointLayout = ref<FiveHolePointLayout>({
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
const machNumberPrecision = ref(DEFAULT_CALIBRATION_MACH_PRECISION)
const velocityPrecision = ref(DEFAULT_CALIBRATION_VELOCITY_PRECISION)

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

function nextStep() {
  if (currentStep.value < steps.value.length - 1) currentStep.value++
}

function prevStep() {
  if (currentStep.value > 0) currentStep.value--
}

function generatePoints() {
  return generateFiveHoleSnakePoints(pointLayout.value)
}

async function saveConfig() {
  isSaving.value = true
  try {
    const config: CalibrationConfig = {
      type: 'five-hole',
      name: calibrationName.value,
      probeChannels: probeChannels.value.filter((ch) => ch.enabled),
      motionAxes: motionAxes.value,
      points: generatePoints(),
      dwellTimeMs: dwellTimeMs.value,
      samplesPerPoint: samplesPerPoint.value,
      savePath: '',
      fiveHoleLayout: pointLayout.value,
      derivedValuePrecision: {
        machNumber: machNumberPrecision.value,
        velocity: velocityPrecision.value,
      },
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
    console.error('Failed to save config:', err)
    feedbackStore.pushToast('保存失败: ' + (err instanceof Error ? err.message : String(err)), 'error')
  } finally {
    isSaving.value = false
  }
}

async function loadSavedConfig() {
  try {
    const res = await calibrationApi.getConfig('five-hole')
    const config = res.success && res.data ? applyCalibrationPrecisionDefaults(res.data) : null
    if (config) {
      calibrationName.value = config.name
      if (config.fiveHoleLayout) pointLayout.value = { ...config.fiveHoleLayout }
      if (config.probeChannels) {
        config.probeChannels.forEach((savedCh) => {
          const existingCh = probeChannels.value.find((ch) => (ch.role ? ch.role === savedCh.role : ch.name === savedCh.name))
          if (existingCh) {
            existingCh.channel = { ...savedCh.channel }
            existingCh.enabled = savedCh.enabled
            existingCh.role = savedCh.role ?? existingCh.role
            existingCh.precision = savedCh.precision
          }
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
    }
  } catch (err) {
    console.error('Failed to load saved config:', err)
  }
}

onMounted(async () => {
  try {
    await Promise.all([deviceStore.refreshProfiles(), motionStore.refreshProfiles(), loadSavedConfig()])
    if (!calibrationName.value) calibrationName.value = `五孔探针-${new Date().toLocaleDateString()}`
  } finally {
    isLoading.value = false
  }
})
</script>

<template>
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-[rgba(15,23,42,0.28)] backdrop-blur-[2px]">
    <div data-test="five-hole-settings-shell" class="flex max-h-[90vh] w-[92vw] max-w-[980px] flex-col rounded-[var(--radius-md)] border border-[var(--border-default)] bg-[var(--bg-panel)] text-[var(--text-primary)] shadow-[0_24px_60px_rgba(15,23,42,0.12)]">
      <!-- 头部 -->
      <div class="flex items-center justify-between border-b border-[var(--border-default)] px-6 py-4">
        <div>
          <h1 class="text-xl font-bold text-[var(--text-primary)]">五孔探针校准配置</h1>
          <p class="text-sm text-[var(--text-muted)]">配置点位布局、通道映射和运动轴参数</p>
        </div>
        <UiButton variant="secondary" size="sm" @click="emit('close')">
          <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
          </svg>
        </UiButton>
      </div>

      <!-- 步骤指示器 -->
      <div data-test="five-hole-settings-steps" class="border-b border-[var(--border-default)] bg-[var(--bg-panel-strong)] px-6 py-3">
        <div class="flex items-center justify-center gap-2">
          <div v-for="(step, idx) in steps" :key="idx" class="flex items-center">
            <div class="flex h-8 w-8 cursor-pointer items-center justify-center rounded-[var(--radius-sm)] border text-sm font-medium transition-all"
              :class="[
                idx === currentStep ? 'border-[var(--accent-primary)] bg-[var(--accent-primary)] text-white' :
                idx < currentStep ? 'border-[var(--accent-success)] bg-[var(--accent-success)] text-white' :
                'border-[var(--border-default)] bg-[var(--bg-panel)] text-[var(--text-muted)] hover:bg-[var(--bg-panel-strong)]'
              ]"
              @click="idx <= currentStep && (currentStep = idx)"
            >
              {{ idx < currentStep ? '✓' : idx + 1 }}
            </div>
            <div v-if="idx < steps.length - 1" class="mx-1 h-0.5 w-12" :class="idx < currentStep ? 'bg-[var(--accent-success)]' : 'bg-[var(--border-default)]'"></div>
          </div>
        </div>
      </div>

      <!-- 内容区域 -->
      <div class="flex-1 overflow-auto p-6">
        <div v-if="isLoading" class="flex items-center justify-center h-full">
          <div class="text-center">
            <div class="mx-auto mb-4 h-8 w-8 animate-spin rounded-full border-2 border-[var(--accent-primary)] border-t-transparent"></div>
            <p class="text-[var(--text-muted)]">加载中...</p>
          </div>
        </div>

        <div v-if="!isLoading && currentStepErrors.length > 0" class="mt-4 rounded-[var(--radius-sm)] border border-[color:color-mix(in_srgb,var(--accent-warning)_35%,var(--border-default))] bg-[color:color-mix(in_srgb,var(--accent-warning)_10%,white)] px-4 py-3 text-sm text-[color:color-mix(in_srgb,var(--accent-warning)_75%,#7c2d12)]">
          <div class="font-medium mb-1">请修正以下错误</div>
          <div>{{ currentStepErrors[0] }}</div>
        </div>

        <!-- 步骤 0: 基本设置 -->
        <div v-if="!isLoading && currentStep === 0" class="max-w-3xl mx-auto space-y-6">
          <div class="rounded-[var(--radius-md)] border border-[var(--border-default)] bg-[var(--bg-panel)] p-6 shadow-[var(--shadow-panel)]">
            <h3 class="text-lg font-semibold mb-4">配置名称</h3>
            <input v-model="calibrationName" type="text" class="w-full rounded-[var(--radius-sm)] border border-[var(--border-default)] bg-[var(--bg-panel)] px-4 py-2 text-[var(--text-primary)] placeholder-[var(--text-muted)] focus:border-[var(--accent-primary)] focus:outline-none" placeholder="输入配置名称" />
          </div>

          <div class="rounded-[var(--radius-md)] border border-[var(--border-default)] bg-[var(--bg-panel)] p-6 shadow-[var(--shadow-panel)]">
            <h3 class="text-lg font-semibold mb-4">点位布局</h3>
            <div class="space-y-4">
              <div>
                <label class="mb-2 block text-sm text-[var(--text-muted)]">攻角 α 范围</label>
                <div class="grid grid-cols-3 gap-4">
                  <div><label class="mb-1 block text-xs text-[var(--text-muted)]">最小值</label><input v-model.number="pointLayout.alphaMin" type="number" step="1" class="w-full rounded-[var(--radius-sm)] border border-[var(--border-default)] bg-[var(--bg-panel)] px-4 py-2 text-[var(--text-primary)] focus:border-[var(--accent-primary)] focus:outline-none" /></div>
                  <div><label class="mb-1 block text-xs text-[var(--text-muted)]">最大值</label><input v-model.number="pointLayout.alphaMax" type="number" step="1" class="w-full rounded-[var(--radius-sm)] border border-[var(--border-default)] bg-[var(--bg-panel)] px-4 py-2 text-[var(--text-primary)] focus:border-[var(--accent-primary)] focus:outline-none" /></div>
                  <div><label class="mb-1 block text-xs text-[var(--text-muted)]">步长</label><input v-model.number="pointLayout.alphaStep" type="number" min="1" step="1" class="w-full rounded-[var(--radius-sm)] border border-[var(--border-default)] bg-[var(--bg-panel)] px-4 py-2 text-[var(--text-primary)] focus:border-[var(--accent-primary)] focus:outline-none" /></div>
                </div>
              </div>
              <div>
                <label class="mb-2 block text-sm text-[var(--text-muted)]">侧滑角 β 范围</label>
                <div class="grid grid-cols-3 gap-4">
                  <div><label class="mb-1 block text-xs text-[var(--text-muted)]">最小值</label><input v-model.number="pointLayout.betaMin" type="number" step="1" class="w-full rounded-[var(--radius-sm)] border border-[var(--border-default)] bg-[var(--bg-panel)] px-4 py-2 text-[var(--text-primary)] focus:border-[var(--accent-primary)] focus:outline-none" /></div>
                  <div><label class="mb-1 block text-xs text-[var(--text-muted)]">最大值</label><input v-model.number="pointLayout.betaMax" type="number" step="1" class="w-full rounded-[var(--radius-sm)] border border-[var(--border-default)] bg-[var(--bg-panel)] px-4 py-2 text-[var(--text-primary)] focus:border-[var(--accent-primary)] focus:outline-none" /></div>
                  <div><label class="mb-1 block text-xs text-[var(--text-muted)]">步长</label><input v-model.number="pointLayout.betaStep" type="number" min="1" step="1" class="w-full rounded-[var(--radius-sm)] border border-[var(--border-default)] bg-[var(--bg-panel)] px-4 py-2 text-[var(--text-primary)] focus:border-[var(--accent-primary)] focus:outline-none" /></div>
                </div>
              </div>
            </div>
            <div class="mt-4 rounded-[var(--radius-sm)] border border-[color:color-mix(in_srgb,var(--accent-primary)_35%,var(--border-default))] bg-[color:color-mix(in_srgb,var(--accent-primary)_8%,white)] p-4">
              <div class="flex items-center justify-between">
                <span class="text-[var(--text-muted)]">总点数</span>
                <span class="text-2xl font-bold text-[var(--accent-primary)]">{{ pointCount }} 点</span>
              </div>
            </div>
          </div>

          <div class="rounded-[var(--radius-md)] border border-[var(--border-default)] bg-[var(--bg-panel)] p-6 shadow-[var(--shadow-panel)]">
            <h3 class="text-lg font-semibold mb-4">采集参数</h3>
            <div class="grid grid-cols-2 gap-4">
              <div><label class="mb-2 block text-sm text-[var(--text-muted)]">驻留时间 (ms)</label><input v-model.number="dwellTimeMs" type="number" min="100" step="100" class="w-full rounded-[var(--radius-sm)] border border-[var(--border-default)] bg-[var(--bg-panel)] px-4 py-2 text-[var(--text-primary)] focus:border-[var(--accent-primary)] focus:outline-none" /></div>
              <div><label class="mb-2 block text-sm text-[var(--text-muted)]">每点采样数</label><input v-model.number="samplesPerPoint" type="number" min="1" max="1000" class="w-full rounded-[var(--radius-sm)] border border-[var(--border-default)] bg-[var(--bg-panel)] px-4 py-2 text-[var(--text-primary)] focus:border-[var(--accent-primary)] focus:outline-none" /></div>
            </div>
            <div class="mt-4 grid grid-cols-2 gap-4">
              <div><label class="mb-2 block text-sm text-[var(--text-muted)]">马赫数精度</label><input v-model.number="machNumberPrecision" type="number" min="0" max="8" class="w-full rounded-[var(--radius-sm)] border border-[var(--border-default)] bg-[var(--bg-panel)] px-4 py-2 text-[var(--text-primary)] focus:border-[var(--accent-primary)] focus:outline-none" /></div>
              <div><label class="mb-2 block text-sm text-[var(--text-muted)]">流速精度 (m/s)</label><input v-model.number="velocityPrecision" type="number" min="0" max="8" class="w-full rounded-[var(--radius-sm)] border border-[var(--border-default)] bg-[var(--bg-panel)] px-4 py-2 text-[var(--text-primary)] focus:border-[var(--accent-primary)] focus:outline-none" /></div>
            </div>
          </div>
        </div>

        <!-- 步骤 1: 硬件配置 -->
        <div v-if="!isLoading && currentStep === 1" class="max-w-4xl mx-auto space-y-6">
          <div class="rounded-[var(--radius-md)] border border-[var(--border-default)] bg-[var(--bg-panel)] p-6 shadow-[var(--shadow-panel)]">
            <h3 class="text-lg font-semibold mb-4">探针通道映射</h3>
            <div class="overflow-hidden">
              <table class="w-full">
                <thead class="bg-[var(--bg-panel-strong)]">
                  <tr>
                    <th class="px-4 py-3 text-left text-sm font-medium text-[var(--text-muted)]">启用</th>
                    <th class="px-4 py-3 text-left text-sm font-medium text-[var(--text-muted)]">名称</th>
                    <th class="px-4 py-3 text-left text-sm font-medium text-[var(--text-muted)]">数据源</th>
                    <th class="px-4 py-3 text-left text-sm font-medium text-[var(--text-muted)]">通道索引</th>
                    <th class="px-4 py-3 text-left text-sm font-medium text-[var(--text-muted)]">精度</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-[var(--border-default)]">
                  <tr v-for="channel in probeChannels" :key="channel.name" class="hover:bg-[var(--bg-panel-strong)]">
                    <td class="px-4 py-3"><input v-model="channel.enabled" type="checkbox" class="h-4 w-4 rounded border-[var(--border-default)] bg-[var(--bg-panel)] text-[var(--accent-primary)]" /></td>
                    <td class="px-4 py-3 text-sm text-[var(--text-primary)]">{{ channel.name }}</td>
                    <td class="px-4 py-3">
                      <select v-model="channel.channel.deviceId" :disabled="!channel.enabled" class="w-full rounded-[var(--radius-sm)] border border-[var(--border-default)] bg-[var(--bg-panel)] px-3 py-1.5 text-sm text-[var(--text-primary)] disabled:opacity-50">
                        <option value="">选择设备</option>
                        <option v-for="device in deviceList" :key="device.id" :value="device.id">{{ device.name }} ({{ device.type }})</option>
                      </select>
                    </td>
                    <td class="px-4 py-3"><input v-model.number="channel.channel.channelIndex" type="number" min="-1" max="100" :disabled="!channel.enabled" class="w-20 rounded-[var(--radius-sm)] border border-[var(--border-default)] bg-[var(--bg-panel)] px-3 py-1.5 text-sm text-[var(--text-primary)] disabled:opacity-50" /></td>
                    <td class="px-4 py-3"><input v-model.number="channel.precision" type="number" min="0" max="8" :disabled="!channel.enabled" class="w-20 rounded-[var(--radius-sm)] border border-[var(--border-default)] bg-[var(--bg-panel)] px-3 py-1.5 text-sm text-[var(--text-primary)] disabled:opacity-50" /></td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>

          <div class="rounded-[var(--radius-md)] border border-[var(--border-default)] bg-[var(--bg-panel)] p-6 shadow-[var(--shadow-panel)]">
            <h3 class="text-lg font-semibold mb-4">运动轴配置</h3>
            <div class="overflow-hidden">
              <table class="w-full">
                <thead class="bg-[var(--bg-panel-strong)]">
                  <tr>
                    <th class="px-4 py-3 text-left text-sm font-medium text-[var(--text-muted)]">坐标轴</th>
                    <th class="px-4 py-3 text-left text-sm font-medium text-[var(--text-muted)]">运动控制器</th>
                    <th class="px-4 py-3 text-left text-sm font-medium text-[var(--text-muted)]">物理轴</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-[var(--border-default)]">
                  <tr v-for="axis in motionAxes" :key="axis.name" class="hover:bg-[var(--bg-panel-strong)]">
                    <td class="px-4 py-3"><span class="text-lg font-bold text-[var(--accent-primary)]">{{ axis.name }}</span></td>
                    <td class="px-4 py-3">
                      <select v-model="axis.controllerId" class="w-full rounded-[var(--radius-sm)] border border-[var(--border-default)] bg-[var(--bg-panel)] px-3 py-1.5 text-sm text-[var(--text-primary)]">
                        <option value="">选择控制器</option>
                        <option v-for="controller in motionControllerList" :key="controller.id" :value="controller.id">{{ controller.name }} ({{ controller.type }})</option>
                      </select>
                    </td>
                    <td class="px-4 py-3">
                      <select v-model="axis.axis" class="w-full rounded-[var(--radius-sm)] border border-[var(--border-default)] bg-[var(--bg-panel)] px-3 py-1.5 text-sm text-[var(--text-primary)]">
                        <option value="X">X轴</option>
                        <option value="Y">Y轴</option>
                        <option value="Z">Z轴</option>
                        <option value="U">U轴</option>
                      </select>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>

          <div class="rounded-[var(--radius-md)] border border-[var(--border-default)] bg-[var(--bg-panel)] p-6 shadow-[var(--shadow-panel)]">
            <h3 class="text-lg font-semibold mb-4">球罐判定门控</h3>
            <div class="grid grid-cols-3 gap-4 items-end">
              <label class="flex items-center gap-2 text-sm text-[var(--text-primary)]">
                <input v-model="sphereTankGateEnabled" type="checkbox" class="h-4 w-4 rounded border-[var(--border-default)] bg-[var(--bg-panel)] text-[var(--accent-primary)]" />
                启用球罐判定
              </label>
              <div><label class="mb-1 block text-sm text-[var(--text-muted)]">等待时间 (秒)</label><input v-model.number="sphereTankWaitTimeSec" type="number" min="0" step="0.1" class="w-full rounded-[var(--radius-sm)] border border-[var(--border-default)] bg-[var(--bg-panel)] px-3 py-1.5 text-sm text-[var(--text-primary)]" /></div>
              <div class="text-xs text-[var(--text-muted)]">球罐稳定后才开始采集</div>
            </div>
            <div class="grid grid-cols-2 gap-4 mt-3">
              <div><label class="mb-1 block text-sm text-[var(--text-muted)]">稳定通道设备</label><select v-model="sphereTankStableChannel.deviceId" class="w-full rounded-[var(--radius-sm)] border border-[var(--border-default)] bg-[var(--bg-panel)] px-3 py-1.5 text-sm text-[var(--text-primary)]"><option value="">选择设备</option><option v-for="device in deviceList" :key="device.id" :value="device.id">{{ device.name }}</option></select></div>
              <div><label class="mb-1 block text-sm text-[var(--text-muted)]">稳定通道索引</label><input v-model.number="sphereTankStableChannel.channelIndex" type="number" min="0" class="w-full rounded-[var(--radius-sm)] border border-[var(--border-default)] bg-[var(--bg-panel)] px-3 py-1.5 text-sm text-[var(--text-primary)]" /></div>
            </div>
          </div>
        </div>

        <!-- 步骤 2: 确认保存 -->
        <div v-if="!isLoading && currentStep === 2" class="max-w-3xl mx-auto space-y-6">
          <div class="rounded-[var(--radius-md)] border border-[var(--border-default)] bg-[var(--bg-panel)] p-6 shadow-[var(--shadow-panel)]">
            <h3 class="text-lg font-semibold mb-4">配置摘要</h3>
            <div class="space-y-3 text-sm">
              <div class="flex justify-between border-b border-[var(--border-default)] py-2"><span class="text-[var(--text-muted)]">配置名称</span><span class="text-[var(--text-primary)]">{{ calibrationName }}</span></div>
              <div class="flex justify-between border-b border-[var(--border-default)] py-2"><span class="text-[var(--text-muted)]">校准类型</span><span class="text-[var(--text-primary)]">五孔探针</span></div>
              <div class="flex justify-between border-b border-[var(--border-default)] py-2"><span class="text-[var(--text-muted)]">点位布局</span><span class="text-[var(--text-primary)]">α: {{ pointLayout.alphaMin }}° ~ {{ pointLayout.alphaMax }}° (步长 {{ pointLayout.alphaStep }}°), β: {{ pointLayout.betaMin }}° ~ {{ pointLayout.betaMax }}° (步长 {{ pointLayout.betaStep }}°)</span></div>
              <div class="flex justify-between border-b border-[var(--border-default)] py-2"><span class="text-[var(--text-muted)]">总点数</span><span class="font-bold text-[var(--accent-primary)]">{{ pointCount }} 点</span></div>
              <div class="flex justify-between border-b border-[var(--border-default)] py-2"><span class="text-[var(--text-muted)]">启用探针</span><span class="text-[var(--text-primary)]">{{ probeChannels.filter((ch) => ch.enabled).length }} 个</span></div>
              <div class="flex justify-between border-b border-[var(--border-default)] py-2"><span class="text-[var(--text-muted)]">驻留时间</span><span class="text-[var(--text-primary)]">{{ dwellTimeMs }} ms</span></div>
              <div class="flex justify-between py-2"><span class="text-[var(--text-muted)]">每点采样数</span><span class="text-[var(--text-primary)]">{{ samplesPerPoint }}</span></div>
            </div>
          </div>
        </div>
      </div>

      <!-- 底部按钮 -->
      <div class="flex items-center justify-between border-t border-[var(--border-default)] bg-[var(--bg-panel-strong)] px-6 py-4">
        <UiButton v-if="currentStep > 0" variant="secondary" @click="prevStep">上一步</UiButton>
        <div v-else></div>
        <div class="flex items-center gap-3">
          <span class="text-sm text-[var(--text-muted)]">步骤 {{ currentStep + 1 }} / {{ steps.length }}</span>
          <UiButton v-if="currentStep < steps.length - 1" variant="primary" :disabled="!isStepValid" @click="nextStep">下一步</UiButton>
          <UiButton v-else variant="primary" :disabled="!isStepValid || isSaving" @click="saveConfig">
            <svg v-if="isSaving" class="animate-spin w-4 h-4 mr-1" fill="none" viewBox="0 0 24 24"><circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle><path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path></svg>
            {{ isSaving ? '保存中...' : '保存配置' }}
          </UiButton>
        </div>
      </div>
    </div>
  </div>
</template>

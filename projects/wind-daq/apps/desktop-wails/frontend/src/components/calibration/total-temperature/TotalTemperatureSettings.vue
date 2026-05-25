<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useDeviceStore } from '@stores/deviceStore'
import { useMotionStore } from '@stores/motionStore'
import { useFeedbackStore } from '@stores/feedbackStore'
import { calibrationApi } from '@api/calibrationApi'
import UiButton from '@components/ui/UiButton.vue'
import type {
  CalibrationConfig,
  ProbeChannelConfig,
  MotionAxisConfig,
  ChannelRef
} from '@shared/types/calibration'
import {
  applyCalibrationPrecisionDefaults,
  DEFAULT_CALIBRATION_PROBE_PRECISION
} from '@shared/calibrationPrecision'

const emit = defineEmits<{
  close: []
  saved: [config: CalibrationConfig]
}>()

const deviceStore = useDeviceStore()
const motionStore = useMotionStore()
const feedbackStore = useFeedbackStore()

const isLoading = ref(true)
const isSaving = ref(false)

const currentStep = ref(0)
const steps = ['基本设置', '硬件配置', '确认保存']

const pointLayout = ref<{ machMin: number; machMax: number; machStep: number }>({
  machMin: 0.3,
  machMax: 2.0,
  machStep: 0.1
})

const pointCount = computed(() => {
  return Math.floor((pointLayout.value.machMax - pointLayout.value.machMin) / pointLayout.value.machStep) + 1
})

const dwellTimeMs = ref(2000)
const samplesPerPoint = ref(10)
const calibrationName = ref(`总温探针校准-${new Date().toLocaleDateString()}`)
const sphereTankGateEnabled = ref(false)
const sphereTankWaitTimeSec = ref(3)
const sphereTankStableChannel = ref<ChannelRef>({ deviceId: '', channelIndex: 0 })

const probeChannels = ref<ProbeChannelConfig[]>([
  { name: '总温', role: 'totalTemperature.tTotal', channel: { deviceId: '', channelIndex: 0 }, enabled: true, precision: DEFAULT_CALIBRATION_PROBE_PRECISION },
  { name: '静温', role: 'totalTemperature.tStatic', channel: { deviceId: '', channelIndex: 1 }, enabled: true, precision: DEFAULT_CALIBRATION_PROBE_PRECISION },
  { name: '大气温度', role: 'totalTemperature.tAtm', channel: { deviceId: '', channelIndex: 16 }, enabled: true, precision: DEFAULT_CALIBRATION_PROBE_PRECISION }
])

const motionAxes = ref<MotionAxisConfig[]>([
  { name: 'Mach', controllerId: '', axis: 'X' }
])

const deviceList = computed(() => deviceStore.profiles)
const motionControllerList = computed(() => motionStore.profiles)

const REQUIRED_CHANNEL_ROLES = [
  'totalTemperature.tTotal',
  'totalTemperature.tStatic',
  'totalTemperature.tAtm'
] as const

const currentStepErrors = computed<string[]>(() => {
  if (currentStep.value === 0) {
    const errors: string[] = []
    if (calibrationName.value.trim() === '') errors.push('请输入配置名称')
    if (pointLayout.value.machStep <= 0) errors.push('Mach步长必须大于 0')
    if (pointLayout.value.machMax <= pointLayout.value.machMin) errors.push('Mach最大值必须大于最小值')
    if (dwellTimeMs.value < 100) errors.push('稳定等待时间不能小于 100 ms')
    if (samplesPerPoint.value < 1) errors.push('每点采样次数不能小于 1')
    return errors
  }

  if (currentStep.value === 1) {
    const errors: string[] = []
    const enabledRoles = new Set(probeChannels.value.filter(ch => ch.enabled).map(ch => ch.role))
    const missingRoles = REQUIRED_CHANNEL_ROLES.filter(role => !enabledRoles.has(role))
    if (missingRoles.length > 0) errors.push('总温/静温/大气温度必须启用并绑定')
    const invalidChannel = probeChannels.value.find(ch => ch.enabled && (!ch.channel.deviceId || ch.channel.channelIndex < 0))
    if (invalidChannel) errors.push(`通道映射未完成：${invalidChannel.name}`)
    if (motionAxes.value.some(axis => !axis.controllerId)) errors.push('运动轴必须绑定控制器')
    if (sphereTankGateEnabled.value && !sphereTankStableChannel.value.deviceId) {
      errors.push('球罐判定启用时必须配置稳定时间设备')
    }
    return errors
  }

  return []
})

const isStepValid = computed(() => {
  if (currentStep.value === 2) return true
  return currentStepErrors.value.length === 0
})

function nextStep() {
  if (currentStep.value < steps.length - 1) currentStep.value++
}

function prevStep() {
  if (currentStep.value > 0) currentStep.value--
}

function generatePoints() {
  const points = []
  let id = 0
  for (let mach = pointLayout.value.machMin; mach <= pointLayout.value.machMax; mach += pointLayout.value.machStep) {
    points.push({ id: id++, coordinates: { Mach: Math.round(mach * 100) / 100 } })
  }
  return points
}

async function saveConfig() {
  isSaving.value = true
  try {
    const config: CalibrationConfig = {
      type: 'total-temperature',
      name: calibrationName.value,
      probeChannels: probeChannels.value.filter(ch => ch.enabled),
      motionAxes: motionAxes.value,
      points: generatePoints(),
      dwellTimeMs: dwellTimeMs.value,
      samplesPerPoint: samplesPerPoint.value,
      savePath: '',
      totalTemperatureConfig: {
        machRange: { min: pointLayout.value.machMin, max: pointLayout.value.machMax, step: pointLayout.value.machStep },
        probeChannels: {
          testProbe: probeChannels.value.find(ch => ch.role === 'totalTemperature.tTotal')?.channel ?? { deviceId: '', channelIndex: 0 },
          standardProbe: probeChannels.value.find(ch => ch.role === 'totalTemperature.tStatic')?.channel ?? { deviceId: '', channelIndex: 0 },
          totalPressure: { deviceId: '', channelIndex: 0 },
          staticPressure: { deviceId: '', channelIndex: 0 },
          atmosphericPressure: { deviceId: '', channelIndex: 0 },
          atmosphericTemperature: { deviceId: '', channelIndex: 0 }
        },
        targetMachNumbers: generatePoints().map(p => p.coordinates.Mach),
        stabilityCriteria: { sampleCount: samplesPerPoint.value, maxStdDev: 0.1, sampleInterval: 100 },
        sampleInterval: 100
      },
      sphereTankGate: {
        enabled: sphereTankGateEnabled.value,
        waitTimeSec: Math.max(0, sphereTankWaitTimeSec.value),
        stableTimeChannel: { ...sphereTankStableChannel.value }
      }
    }

    const normalizedConfig = applyCalibrationPrecisionDefaults(config)
    const res = await calibrationApi.saveConfig('total-temperature', JSON.parse(JSON.stringify(normalizedConfig)))
    if (!res.success) {
      throw new Error(res.error || '保存配置失败')
    }

    emit('saved', normalizedConfig)
    emit('close')
  } catch (err) {
    console.error('Failed to save config:', err)
    feedbackStore.pushToast('保存配置失败: ' + (err instanceof Error ? err.message : String(err)), 'error')
  } finally {
    isSaving.value = false
  }
}

async function loadSavedConfig() {
  try {
    const res = await calibrationApi.getConfig('total-temperature')
    const config = res.success && res.data ? applyCalibrationPrecisionDefaults(res.data) : null
    if (config) {
      calibrationName.value = config.name
      if (config.totalTemperatureConfig?.machRange) {
        pointLayout.value.machMin = config.totalTemperatureConfig.machRange.min
        pointLayout.value.machMax = config.totalTemperatureConfig.machRange.max
        pointLayout.value.machStep = config.totalTemperatureConfig.machRange.step
      }
      if (config.probeChannels) {
        config.probeChannels.forEach(savedCh => {
          const existingCh = probeChannels.value.find(ch => ch.role ? ch.role === savedCh.role : ch.name === savedCh.name)
          if (existingCh) {
            existingCh.channel = { ...savedCh.channel }
            existingCh.enabled = savedCh.enabled
            existingCh.role = savedCh.role
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
    await Promise.all([
      deviceStore.refreshProfiles(),
      motionStore.refreshProfiles(),
      loadSavedConfig()
    ])
  } finally {
    isLoading.value = false
  }
})
</script>

<template>
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/35 backdrop-blur-sm">
    <div data-test="total-temperature-settings-shell" class="flex max-h-[90vh] w-[92vw] max-w-[980px] flex-col overflow-hidden rounded-[var(--radius-lg)] border border-[var(--border-default)] bg-[var(--bg-panel)] text-[var(--text-primary)] shadow-[0_24px_80px_rgba(15,23,42,0.16)]">
      <div class="flex items-center justify-between border-b border-[var(--border-default)] px-6 py-4">
        <div>
          <p class="text-[11px] font-semibold uppercase tracking-[0.2em] text-[var(--text-muted)]">Configuration</p>
          <h1 class="text-xl font-bold text-[var(--text-primary)]">总温探针校准配置</h1>
          <p class="text-sm text-[var(--text-secondary)]">配置概览与硬件映射会在保存后自动复用</p>
        </div>
        <button @click="emit('close')" class="rounded p-2 text-[var(--text-secondary)] transition-colors hover:bg-[var(--bg-panel-strong)] hover:text-[var(--text-primary)]">
          <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>
      </div>

      <div data-test="total-temperature-settings-stepper" class="border-b border-[var(--border-default)] bg-[var(--bg-panel-strong)] px-6 py-3">
        <div class="flex items-center justify-center gap-2">
          <div v-for="(step, idx) in steps" :key="idx" class="flex items-center">
            <div class="w-8 h-8 rounded-full flex items-center justify-center text-sm font-medium transition-all cursor-pointer"
              :class="[
                idx === currentStep ? 'bg-[var(--accent-primary)] text-white' : idx < currentStep ? 'bg-[var(--accent-success)] text-white' : 'bg-[var(--bg-panel)] text-[var(--text-muted)] border border-[var(--border-default)] hover:bg-[var(--bg-panel-strong)]'
              ]"
              @click="idx <= currentStep && (currentStep = idx)">
              {{ idx < currentStep ? '✓' : idx + 1 }}
            </div>
            <div v-if="idx < steps.length - 1" class="w-12 h-0.5 mx-1" :class="idx < currentStep ? 'bg-[var(--accent-success)]' : 'bg-[var(--border-default)]'"></div>
          </div>
        </div>
      </div>

      <div class="flex-1 overflow-auto p-6">
        <div v-if="isLoading" class="flex items-center justify-center h-full">
          <div class="text-center">
            <div class="animate-spin w-8 h-8 border-2 border-[var(--accent-primary)] border-t-transparent rounded-full mx-auto mb-4"></div>
            <p class="text-[var(--text-muted)]">正在加载...</p>
          </div>
        </div>

        <div v-else-if="currentStep === 0" class="max-w-3xl mx-auto space-y-6">
          <div class="bg-[var(--bg-panel)] border border-[var(--border-default)] rounded-[var(--radius-md)] p-6">
            <h3 class="text-lg font-semibold mb-4 text-[var(--text-primary)]">配置名称</h3>
            <input v-model="calibrationName" type="text" class="w-full px-4 py-2 bg-[var(--bg-panel-strong)] border border-[var(--border-default)] rounded-[var(--radius-sm)] text-[var(--text-primary)] placeholder:text-[var(--text-muted)] focus:border-[var(--accent-primary)] focus:outline-none transition-colors" placeholder="输入配置名称" />
          </div>

          <div class="bg-[var(--bg-panel)] border border-[var(--border-default)] rounded-[var(--radius-md)] p-6">
            <h3 class="text-lg font-semibold mb-4 text-[var(--text-primary)]">点位布局</h3>
            <div class="space-y-4">
              <div>
                <label class="block text-sm text-[var(--text-secondary)] mb-2">Mach数范围</label>
                <div class="grid grid-cols-3 gap-4">
                  <div>
                    <label class="block text-xs text-[var(--text-muted)] mb-1">最小值</label>
                    <input v-model.number="pointLayout.machMin" type="number" step="0.1" class="w-full px-4 py-2 bg-[var(--bg-panel-strong)] border border-[var(--border-default)] rounded-[var(--radius-sm)] text-[var(--text-primary)] focus:border-[var(--accent-primary)] focus:outline-none transition-colors" />
                  </div>
                  <div>
                    <label class="block text-xs text-[var(--text-muted)] mb-1">最大值</label>
                    <input v-model.number="pointLayout.machMax" type="number" step="0.1" class="w-full px-4 py-2 bg-[var(--bg-panel-strong)] border border-[var(--border-default)] rounded-[var(--radius-sm)] text-[var(--text-primary)] focus:border-[var(--accent-primary)] focus:outline-none transition-colors" />
                  </div>
                  <div>
                    <label class="block text-xs text-[var(--text-muted)] mb-1">步长</label>
                    <input v-model.number="pointLayout.machStep" type="number" min="0.01" step="0.1" class="w-full px-4 py-2 bg-[var(--bg-panel-strong)] border border-[var(--border-default)] rounded-[var(--radius-sm)] text-[var(--text-primary)] focus:border-[var(--accent-primary)] focus:outline-none transition-colors" />
                  </div>
                </div>
              </div>
            </div>
            <div class="mt-4 bg-[var(--accent-primary)]/10 border border-[var(--accent-primary)]/30 rounded-[var(--radius-md)] p-4">
              <div class="flex items-center justify-between">
                <span class="text-[var(--accent-primary)]">总校准点数</span>
                <span class="text-2xl font-bold text-[var(--accent-primary)]">{{ pointCount }} 点</span>
              </div>
            </div>
          </div>

          <div class="bg-[var(--bg-panel)] border border-[var(--border-default)] rounded-[var(--radius-md)] p-6">
            <h3 class="text-lg font-semibold mb-4 text-[var(--text-primary)]">采集参数</h3>
            <div class="grid grid-cols-2 gap-4">
              <div>
                <label class="block text-sm text-[var(--text-secondary)] mb-2">稳定等待时间 (毫秒)</label>
                <input v-model.number="dwellTimeMs" type="number" min="100" step="100" class="w-full px-4 py-2 bg-[var(--bg-panel-strong)] border border-[var(--border-default)] rounded-[var(--radius-sm)] text-[var(--text-primary)] focus:border-[var(--accent-primary)] focus:outline-none transition-colors" />
                <p class="text-xs text-[var(--text-muted)] mt-1">到达点位后等待稳定的时间</p>
              </div>
              <div>
                <label class="block text-sm text-[var(--text-secondary)] mb-2">每点采样次数</label>
                <input v-model.number="samplesPerPoint" type="number" min="1" max="1000" class="w-full px-4 py-2 bg-[var(--bg-panel-strong)] border border-[var(--border-default)] rounded-[var(--radius-sm)] text-[var(--text-primary)] focus:border-[var(--accent-primary)] focus:outline-none transition-colors" />
                <p class="text-xs text-[var(--text-muted)] mt-1">每个点位采集的样本数量，取平均值</p>
              </div>
            </div>
          </div>
        </div>

        <div v-else-if="currentStep === 1" class="max-w-4xl mx-auto space-y-6">
          <div class="bg-[var(--bg-panel)] border border-[var(--border-default)] rounded-[var(--radius-md)] p-6">
            <h3 class="text-lg font-semibold mb-4 text-[var(--text-primary)]">测点通道映射</h3>
            <div class="overflow-hidden rounded-[var(--radius-sm)] border border-[var(--border-default)]">
              <table class="w-full">
                <thead class="bg-[var(--bg-panel-strong)]">
                  <tr>
                    <th class="px-4 py-3 text-left text-sm font-medium text-[var(--text-secondary)]">启用</th>
                    <th class="px-4 py-3 text-left text-sm font-medium text-[var(--text-secondary)]">测点名称</th>
                    <th class="px-4 py-3 text-left text-sm font-medium text-[var(--text-secondary)]">数据源设备</th>
                    <th class="px-4 py-3 text-left text-sm font-medium text-[var(--text-secondary)]">通道索引</th>
                    <th class="px-4 py-3 text-left text-sm font-medium text-[var(--text-secondary)]">精度</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-[var(--border-default)]">
                  <tr v-for="channel in probeChannels" :key="channel.name" class="hover:bg-[var(--bg-panel-strong)]/50">
                    <td class="px-4 py-3">
                      <input v-model="channel.enabled" type="checkbox" class="w-4 h-4 rounded border-[var(--border-default)] bg-[var(--bg-panel-strong)] text-[var(--accent-primary)] focus:ring-[var(--accent-primary)] focus:ring-offset-0" />
                    </td>
                    <td class="px-4 py-3 text-sm text-[var(--text-secondary)]">{{ channel.name }}</td>
                    <td class="px-4 py-3">
                      <select v-model="channel.channel.deviceId" :disabled="!channel.enabled" class="w-full px-3 py-1.5 bg-[var(--bg-panel-strong)] border border-[var(--border-default)] rounded-[var(--radius-sm)] text-sm text-[var(--text-primary)] focus:border-[var(--accent-primary)] focus:outline-none disabled:opacity-50">
                        <option value="">选择设备</option>
                        <option v-for="device in deviceList" :key="device.id" :value="device.id">{{ device.name }} ({{ device.type }})</option>
                      </select>
                    </td>
                    <td class="px-4 py-3">
                      <input v-model.number="channel.channel.channelIndex" type="number" min="-1" max="100" :disabled="!channel.enabled" class="w-20 px-3 py-1.5 bg-[var(--bg-panel-strong)] border border-[var(--border-default)] rounded-[var(--radius-sm)] text-sm text-[var(--text-primary)] focus:border-[var(--accent-primary)] focus:outline-none disabled:opacity-50" />
                    </td>
                    <td class="px-4 py-3">
                      <input v-model.number="channel.precision" type="number" min="0" max="8" :disabled="!channel.enabled" class="w-20 px-3 py-1.5 bg-[var(--bg-panel-strong)] border border-[var(--border-default)] rounded-[var(--radius-sm)] text-sm text-[var(--text-primary)] focus:border-[var(--accent-primary)] focus:outline-none disabled:opacity-50" />
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>

          <div class="bg-[var(--bg-panel)] border border-[var(--border-default)] rounded-[var(--radius-md)] p-6">
            <h3 class="text-lg font-semibold mb-4 text-[var(--text-primary)]">运动轴配置</h3>
            <div class="overflow-hidden rounded-[var(--radius-sm)] border border-[var(--border-default)]">
              <table class="w-full">
                <thead class="bg-[var(--bg-panel-strong)]">
                  <tr>
                    <th class="px-4 py-3 text-left text-sm font-medium text-[var(--text-secondary)]">坐标轴</th>
                    <th class="px-4 py-3 text-left text-sm font-medium text-[var(--text-secondary)]">运动控制器</th>
                    <th class="px-4 py-3 text-left text-sm font-medium text-[var(--text-secondary)]">物理轴</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-[var(--border-default)]">
                  <tr v-for="axis in motionAxes" :key="axis.name" class="hover:bg-[var(--bg-panel-strong)]/50">
                    <td class="px-4 py-3">
                      <span class="text-lg font-bold text-[var(--accent-primary)]">{{ axis.name }}</span>
                    </td>
                    <td class="px-4 py-3">
                      <select v-model="axis.controllerId" class="w-full px-3 py-1.5 bg-[var(--bg-panel-strong)] border border-[var(--border-default)] rounded-[var(--radius-sm)] text-sm text-[var(--text-primary)] focus:border-[var(--accent-primary)] focus:outline-none">
                        <option value="">选择控制器</option>
                        <option v-for="controller in motionControllerList" :key="controller.id" :value="controller.id">{{ controller.name }} ({{ controller.type }})</option>
                      </select>
                    </td>
                    <td class="px-4 py-3">
                      <select v-model="axis.axis" class="w-full px-3 py-1.5 bg-[var(--bg-panel-strong)] border border-[var(--border-default)] rounded-[var(--radius-sm)] text-sm text-[var(--text-primary)] focus:border-[var(--accent-primary)] focus:outline-none">
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

          <div class="bg-[var(--bg-panel)] border border-[var(--border-default)] rounded-[var(--radius-md)] p-6">
            <h3 class="text-lg font-semibold mb-4 text-[var(--text-primary)]">球罐稳定判定（PXI）</h3>
            <div class="grid grid-cols-3 gap-4 items-end">
              <label class="flex items-center gap-2 text-sm text-[var(--text-secondary)]">
                <input v-model="sphereTankGateEnabled" type="checkbox" class="w-4 h-4 rounded border-[var(--border-default)] bg-[var(--bg-panel-strong)] text-[var(--accent-primary)] focus:ring-[var(--accent-primary)] focus:ring-offset-0" />
                启用球罐判定
              </label>
              <div>
                <label class="block text-sm text-[var(--text-secondary)] mb-1">等待时间(秒)</label>
                <input v-model.number="sphereTankWaitTimeSec" type="number" min="0" step="0.1" class="w-full px-3 py-1.5 bg-[var(--bg-panel-strong)] border border-[var(--border-default)] rounded-[var(--radius-sm)] text-sm text-[var(--text-primary)] focus:border-[var(--accent-primary)] focus:outline-none" />
              </div>
              <div class="text-xs text-[var(--text-muted)]">采集前需满足：稳定时间 >= 等待时间</div>
            </div>
            <div class="grid grid-cols-2 gap-4 mt-3">
              <div>
                <label class="block text-sm text-[var(--text-secondary)] mb-1">PXI设备</label>
                <select v-model="sphereTankStableChannel.deviceId" class="w-full px-3 py-1.5 bg-[var(--bg-panel-strong)] border border-[var(--border-default)] rounded-[var(--radius-sm)] text-sm text-[var(--text-primary)] focus:border-[var(--accent-primary)] focus:outline-none">
                  <option value="">选择设备</option>
                  <option v-for="device in deviceList" :key="device.id" :value="device.id">{{ device.name }} ({{ device.type }})</option>
                </select>
              </div>
              <div>
                <label class="block text-sm text-[var(--text-secondary)] mb-1">稳定时间通道</label>
                <input v-model.number="sphereTankStableChannel.channelIndex" type="number" min="0" class="w-full px-3 py-1.5 bg-[var(--bg-panel-strong)] border border-[var(--border-default)] rounded-[var(--radius-sm)] text-sm text-[var(--text-primary)] focus:border-[var(--accent-primary)] focus:outline-none" />
              </div>
            </div>
          </div>
        </div>

        <div v-else-if="currentStep === 2" class="max-w-3xl mx-auto space-y-6">
          <div class="bg-[var(--bg-panel)] border border-[var(--border-default)] rounded-[var(--radius-md)] p-6">
            <h3 class="text-lg font-semibold mb-4 text-[var(--text-primary)]">配置摘要</h3>
            <div class="space-y-3 text-sm">
              <div class="flex justify-between py-2 border-b border-[var(--border-default)]">
                <span class="text-[var(--text-secondary)]">配置名称</span>
                <span class="text-[var(--text-primary)]">{{ calibrationName }}</span>
              </div>
              <div class="flex justify-between py-2 border-b border-[var(--border-default)]">
                <span class="text-[var(--text-secondary)]">校准类型</span>
                <span class="text-[var(--text-primary)]">总温探针</span>
              </div>
              <div class="flex justify-between py-2 border-b border-[var(--border-default)]">
                <span class="text-[var(--text-secondary)]">点位布局</span>
                <span class="text-[var(--text-primary)]">Mach: {{ pointLayout.machMin }} ~ {{ pointLayout.machMax }} (步长 {{ pointLayout.machStep }})</span>
              </div>
              <div class="flex justify-between py-2 border-b border-[var(--border-default)]">
                <span class="text-[var(--text-secondary)]">总点数</span>
                <span class="text-[var(--accent-primary)] font-bold">{{ pointCount }} 点</span>
              </div>
              <div class="flex justify-between py-2 border-b border-[var(--border-default)]">
                <span class="text-[var(--text-secondary)]">启用测点</span>
                <span class="text-[var(--text-primary)]">{{ probeChannels.filter(ch => ch.enabled).length }} 个</span>
              </div>
              <div class="flex justify-between py-2 border-b border-[var(--border-default)]">
                <span class="text-[var(--text-secondary)]">稳定时间</span>
                <span class="text-[var(--text-primary)]">{{ dwellTimeMs }} ms</span>
              </div>
              <div class="flex justify-between py-2">
                <span class="text-[var(--text-secondary)]">每点采样</span>
                <span class="text-[var(--text-primary)]">{{ samplesPerPoint }} 次</span>
              </div>
            </div>
          </div>

          <div class="bg-[var(--accent-primary)]/10 border border-[var(--accent-primary)]/30 rounded-[var(--radius-md)] p-4">
            <div class="flex items-start gap-3">
              <svg class="w-5 h-5 text-[var(--accent-primary)] mt-0.5" fill="currentColor" viewBox="0 0 20 20">
                <path fill-rule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7-4a1 1 0 11-2 0 1 1 0 012 0zM9 9a1 1 0 000 2v3a1 1 0 001 1h1a1 1 0 100-2v-3a 1 1 0 00-1-1H9z" clip-rule="evenodd" />
              </svg>
              <div class="text-sm">
                <p class="font-medium text-[var(--accent-primary)]">配置保存说明</p>
                <p class="mt-1 text-[var(--text-secondary)]">保存后，配置将自动存储。下次进入总温探针校准时，将自动加载此配置。</p>
              </div>
            </div>
          </div>
        </div>

        <div v-if="!isLoading && currentStepErrors.length > 0" class="mt-4 rounded-lg border border-amber-500/40 bg-amber-500/10 px-4 py-3 text-sm text-amber-200">
          <div class="font-medium mb-1">请先修正以下问题：</div>
          <div>{{ currentStepErrors[0] }}</div>
        </div>
      </div>

      <div class="px-6 py-4 border-t border-[var(--border-default)] flex items-center justify-between">
        <UiButton v-if="currentStep > 0" variant="secondary" @click="prevStep">上一步</UiButton>
        <div v-else></div>

        <div class="flex items-center gap-3">
          <span class="text-sm text-[var(--text-muted)]">步骤 {{ currentStep + 1 }} / {{ steps.length }}</span>
          <UiButton v-if="currentStep < steps.length - 1" variant="primary" :disabled="!isStepValid" @click="nextStep">下一步</UiButton>
          <UiButton v-else variant="primary" :disabled="!isStepValid || isSaving" @click="saveConfig">
            <svg v-if="isSaving" class="animate-spin w-4 h-4" fill="none" viewBox="0 0 24 24">
              <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
              <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
            </svg>
            {{ isSaving ? '保存中...' : '保存配置' }}
          </UiButton>
        </div>
      </div>
    </div>
  </div>
</template>

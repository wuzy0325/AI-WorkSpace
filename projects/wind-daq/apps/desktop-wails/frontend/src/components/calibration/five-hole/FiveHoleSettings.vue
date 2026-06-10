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
</script>

<template>
  <UiDialog :show="true" width="1020px" title="五孔探针校准配置" closable @close="emit('close')">
    <template #header-extra>
      <span class="ntext-label">配置点位布局、通道映射和运动轴参数</span>
    </template>

    <UiSteps :current="currentStep" class="steps-mb">
      <UiStep v-for="(step, idx) in steps" :key="idx" :title="step.label" :disabled="idx > currentStep" />
    </UiSteps>

    <UiSpin v-if="isLoading" class="spinner" />

    <template v-else>
      <UiAlert v-if="currentStepErrors.length > 0" type="warning" class="alert-error">
        <template #header>请修正以下错误</template>
        {{ currentStepErrors[0] }}
      </UiAlert>

      <div v-if="currentStep === 0" class="step-content">
        <UiPanel class="section-card">
          <template #header><span class="ntext-header">配置名称</span></template>
          <UiInput v-model="calibrationName" placeholder="输入配置名称" />
        </UiPanel>

        <UiPanel class="section-card">
          <template #header><span class="ntext-header">点位布局</span></template>
          <div class="angle-section">
            <div class="angle-group">
              <div class="angle-label"><Move3D :size="14" /><span class="ntext-summary">攻角 α 范围</span></div>
              <div class="angle-grid">
                <div><span class="ntext-label">最小值 (°)</span><UiInputNumber v-model="pointLayout.alphaMin" style="width:100%" /></div>
                <div><span class="ntext-label">最大值 (°)</span><UiInputNumber v-model="pointLayout.alphaMax" style="width:100%" /></div>
                <div><span class="ntext-label">步长 (°)</span><UiInputNumber v-model="pointLayout.alphaStep" style="width:100%" :min="1" /></div>
              </div>
            </div>
            <div class="angle-group">
              <div class="angle-label"><Move3D :size="14" /><span class="ntext-summary">侧滑角 β 范围</span></div>
              <div class="angle-grid">
                <div><span class="ntext-label">最小值 (°)</span><UiInputNumber v-model="pointLayout.betaMin" style="width:100%" /></div>
                <div><span class="ntext-label">最大值 (°)</span><UiInputNumber v-model="pointLayout.betaMax" style="width:100%" /></div>
                <div><span class="ntext-label">步长 (°)</span><UiInputNumber v-model="pointLayout.betaStep" style="width:100%" :min="1" /></div>
              </div>
            </div>
          </div>
          <div class="point-summary">
            <LayoutGrid :size="16" />
            <span class="ntext-summary">总点数</span>
            <span class="ntext-count">{{ pointCount }}</span>
            <span class="ntext-summary">点</span>
          </div>
        </UiPanel>

        <UiPanel class="section-card">
          <template #header><span class="ntext-header">采集参数</span></template>
          <div class="param-grid">
            <div><span class="ntext-label">驻留时间 (ms)</span><UiInputNumber v-model="dwellTimeMs" :min="100" :step="100" style="width:100%" /></div>
            <div><span class="ntext-label">每点采样数</span><UiInputNumber v-model="samplesPerPoint" :min="1" :max="1000" style="width:100%" /></div>
            <div><span class="ntext-label">马赫数精度</span><UiInputNumber v-model="machNumberPrecision" :min="0" :max="8" style="width:100%" /></div>
            <div><span class="ntext-label">流速精度 (m/s)</span><UiInputNumber v-model="velocityPrecision" :min="0" :max="8" style="width:100%" /></div>
          </div>
        </UiPanel>
      </div>

      <div v-if="currentStep === 1" class="step-content">
        <UiPanel class="section-card">
          <template #header><span class="ntext-header">探针通道映射</span></template>
          <div class="table-wrap">
            <table class="ntable">
              <thead><tr>
                <th style="width:48px">启用</th><th>名称</th><th>数据源</th><th style="width:100px">通道</th><th style="width:80px">精度</th>
              </tr></thead>
              <tbody>
                <tr v-for="ch in probeChannels" :key="ch.name">
                  <td class="cell-center"><UiCheckbox v-model:checked="ch.enabled" /></td>
                  <td><span class="ntext-summary">{{ ch.name }}</span></td>
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

        <UiPanel class="section-card">
          <template #header><span class="ntext-header">运动轴配置</span></template>
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

        <UiPanel class="section-card">
          <template #header><span class="ntext-header">球罐判定门控</span></template>
          <div class="flex-col-gap">
            <UiCheckbox v-model:checked="sphereTankGateEnabled">启用球罐判定</UiCheckbox>
            <div v-if="sphereTankGateEnabled" class="sphere-grid">
              <div><span class="ntext-label">等待时间 (秒)</span><UiInputNumber v-model="sphereTankWaitTimeSec" :min="0" :step="0.1" style="width:100%" /></div>
              <div><span class="ntext-label">稳定通道设备</span><UiSelect v-model="sphereTankStableChannel.deviceId" :options="deviceList.map(d => ({ label: d.name, value: d.id }))" placeholder="选择设备" /></div>
              <div><span class="ntext-label">稳定通道索引</span><UiInputNumber v-model="sphereTankStableChannel.channelIndex" :min="0" style="width:100%" /></div>
              <div class="angle-end"><span class="ntext-label">球罐稳定后才开始采集</span></div>
            </div>
          </div>
        </UiPanel>
      </div>

      <div v-if="currentStep === 2" class="step-content">
        <UiPanel class="section-card">
          <template #header><span class="ntext-header">配置摘要</span></template>
          <div class="summary-grid">
            <div class="summary-row"><span class="ntext-summary">配置名称</span><span>{{ calibrationName }}</span></div>
            <div class="summary-row"><span class="ntext-summary">校准类型</span><span>五孔探针</span></div>
            <div class="summary-row"><span class="ntext-summary">点位布局</span><span>α: {{ pointLayout.alphaMin }}° ~ {{ pointLayout.alphaMax }}° (步长 {{ pointLayout.alphaStep }}°) / β: {{ pointLayout.betaMin }}° ~ {{ pointLayout.betaMax }}° (步长 {{ pointLayout.betaStep }}°)</span></div>
            <div class="summary-row"><span class="ntext-summary">总点数</span><span class="ntext-count" style="font-weight:700">{{ pointCount }} 点</span></div>
            <div class="summary-row"><span class="ntext-summary">启用探针</span><span>{{ probeChannels.filter(ch => ch.enabled).length }} 个</span></div>
            <div class="summary-row"><span class="ntext-summary">驻留时间</span><span>{{ dwellTimeMs }} ms</span></div>
            <div class="summary-row"><span class="ntext-summary">每点采样数</span><span>{{ samplesPerPoint }}</span></div>
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
.step-content {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.section-card {
  font-size: var(--text-sm);
}

.angle-section {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.angle-group {
  padding: 10px;
  border-radius: var(--radius-md);
  border: 1px solid var(--border-default);
  background: var(--bg-panel-strong);
}

.angle-label {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: var(--space-2);
}

.angle-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: var(--space-2);
}

.point-summary {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  margin-top: var(--space-3);
  padding: 10px;
  border-radius: var(--radius-md);
  border: 1px solid color-mix(in srgb, var(--accent-primary) 20%, transparent);
  background: color-mix(in srgb, var(--accent-primary) 5%, transparent);
}

.param-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px;
}

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
  padding: var(--space-2) 10px;
  font-weight: 600;
  font-size: var(--text-xs);
  color: var(--text-muted);
  background: var(--bg-panel-strong);
  border-bottom: 1px solid var(--border-default);
}

.ntable td {
  padding: 6px 10px;
  border-bottom: 1px solid var(--border-default);
}

.ntable tbody tr:hover {
  background: color-mix(in srgb, var(--accent-primary) 3%, transparent);
}

.summary-grid {
  display: flex;
  flex-direction: column;
}

.summary-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: var(--space-2) 0;
  border-bottom: 1px solid var(--border-default);
  gap: var(--space-3);
}

.summary-row:last-child {
  border-bottom: none;
}

.ntext-header { font-size:var(--text-sm); font-weight:600; }
.ntext-label { font-size:var(--text-xs); color:var(--text-tertiary); }
.ntext-summary { font-size:var(--text-sm); }
.ntext-count { font-size:22px; font-weight:700; color:var(--accent-primary); }
.steps-mb { margin-bottom:var(--space-4); }
.spinner { display:flex; justify-content:center; padding:var(--space-10) 0; }
.alert-error { margin-bottom:var(--space-3); font-size:var(--text-sm); }
.footer-bar { display:flex; align-items:center; justify-content:space-between; width:100%; }
.footer-actions { display:flex; align-items:center; gap:var(--space-3); }
.step-indicator { font-size:var(--text-xs); color:var(--text-tertiary); }
.icon-left { margin-right:var(--space-1); }
.icon-right { margin-left:var(--space-1); }
.flex-col-gap { display:flex; flex-direction:column; gap:10px; }
.sphere-grid { display:grid; grid-template-columns:1fr 1fr; gap:10px; }
.checkbox-mb { margin-bottom:var(--space-2); }
.cell-center { text-align:center; }
.angle-end { display:flex; align-items:flex-end; }
</style>
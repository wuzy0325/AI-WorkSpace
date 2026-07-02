<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useDeviceStore } from '@stores/deviceStore'
import { useMotionStore } from '@stores/motionStore'
import { useFeedbackStore } from '@stores/feedbackStore'
import { calibrationApi } from '@api/calibrationApi'
import type { CalibrationConfig, ProbeChannelConfig, MotionAxisConfig, ChannelRef } from '@shared/types/calibration'
import { applyCalibrationPrecisionDefaults, DEFAULT_CALIBRATION_PROBE_PRECISION } from '@shared/calibrationPrecision'
import UiButton from '@components/ui/UiButton.vue'
import UiAlert from '@components/ui/UiAlert.vue'
import { reportAllSettledFailures } from '@utils/allSettledReport'
import UiPanel from '@components/ui/UiPanel.vue'
import UiCheckbox from '@components/ui/UiCheckbox.vue'
import UiInput from '@components/ui/UiInput.vue'
import UiInputNumber from '@components/ui/UiInputNumber.vue'
import UiDialog from '@components/ui/UiDialog.vue'
import UiSelect from '@components/ui/UiSelect.vue'
import UiSpin from '@components/ui/UiSpin.vue'
import UiStep from '@components/ui/UiStep.vue'
import UiSteps from '@components/ui/UiSteps.vue'
import UiStatusBadge from '@components/ui/UiStatusBadge.vue'

const emit = defineEmits<{ close: []; saved: [config: CalibrationConfig] }>()
const deviceStore = useDeviceStore()
const motionStore = useMotionStore()
const feedbackStore = useFeedbackStore()

const isLoading = ref(true)
const isSaving = ref(false)
const currentStep = ref(0)
const steps = ['基本设置', '硬件配置', '确认保存']

const pointLayout = ref({ machMin: 0.3, machMax: 2.0, machStep: 0.1 })
const pointCount = computed(() => Math.floor((pointLayout.value.machMax - pointLayout.value.machMin) / pointLayout.value.machStep) + 1)
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

const motionAxes = ref<MotionAxisConfig[]>([{ name: 'Mach', controllerId: '', axis: 'X' }])
const deviceList = computed(() => deviceStore.profiles)
const motionControllerList = computed(() => motionStore.profiles)
const REQUIRED_CHANNEL_ROLES = ['totalTemperature.tTotal', 'totalTemperature.tStatic', 'totalTemperature.tAtm'] as const

const currentStepErrors = computed<string[]>(() => {
  if (currentStep.value === 0) {
    const errors: string[] = []
    if (!calibrationName.value.trim()) errors.push('请输入配置名称')
    if (pointLayout.value.machStep <= 0) errors.push('Mach步长必须大于 0')
    if (pointLayout.value.machMax <= pointLayout.value.machMin) errors.push('Mach最大值必须大于最小值')
    if (dwellTimeMs.value < 100) errors.push('稳定等待时间不能小于 100 ms')
    if (samplesPerPoint.value < 1) errors.push('每点采样次数不能小于 1')
    return errors
  }
  if (currentStep.value === 1) {
    const errors: string[] = []
    if (probeChannels.value.filter(ch => ch.enabled).some(ch => !ch.channel.deviceId || ch.channel.channelIndex < 0)) errors.push('通道映射未完成')
    if (motionAxes.value.some(a => !a.controllerId)) errors.push('运动轴必须绑定控制器')
    if (sphereTankGateEnabled.value && !sphereTankStableChannel.value.deviceId) errors.push('球罐判定需要选择设备')
    return errors
  }
  return []
})

const isStepValid = computed(() => currentStep.value === 2 || currentStepErrors.value.length === 0)

function nextStep() { if (currentStep.value < steps.length - 1) currentStep.value++ }
function prevStep() { if (currentStep.value > 0) currentStep.value-- }

function generatePoints() {
  const points = []
  for (let m = pointLayout.value.machMin; m <= pointLayout.value.machMax; m += pointLayout.value.machStep)
    points.push({ id: points.length, coordinates: { Mach: Math.round(m * 100) / 100 } })
  return points
}

async function saveConfig() {
  isSaving.value = true
  try {
    const config: CalibrationConfig = {
      type: 'total-temperature', name: calibrationName.value,
      probeChannels: probeChannels.value.filter(ch => ch.enabled), motionAxes: motionAxes.value,
      points: generatePoints(), dwellTimeMs: dwellTimeMs.value, samplesPerPoint: samplesPerPoint.value, savePath: '',
      totalTemperatureConfig: {
        machRange: { min: pointLayout.value.machMin, max: pointLayout.value.machMax, step: pointLayout.value.machStep },
        probeChannels: {
          testProbe: probeChannels.value.find(ch => ch.role === 'totalTemperature.tTotal')?.channel ?? { deviceId: '', channelIndex: 0 },
          standardProbe: probeChannels.value.find(ch => ch.role === 'totalTemperature.tStatic')?.channel ?? { deviceId: '', channelIndex: 0 },
          totalPressure: { deviceId: '', channelIndex: 0 }, staticPressure: { deviceId: '', channelIndex: 0 },
          atmosphericPressure: { deviceId: '', channelIndex: 0 }, atmosphericTemperature: { deviceId: '', channelIndex: 0 }
        },
        targetMachNumbers: generatePoints().map(p => p.coordinates.Mach),
        stabilityCriteria: { sampleCount: samplesPerPoint.value, maxStdDev: 0.1, sampleInterval: 100 }, sampleInterval: 100
      },
      sphereTankGate: { enabled: sphereTankGateEnabled.value, waitTimeSec: Math.max(0, sphereTankWaitTimeSec.value), stableTimeChannel: { ...sphereTankStableChannel.value } }
    }
    const res = await calibrationApi.saveConfig('total-temperature', JSON.parse(JSON.stringify(applyCalibrationPrecisionDefaults(config))))
    if (!res.success) throw new Error(res.error || '保存失败')
    emit('saved', applyCalibrationPrecisionDefaults(config))
    emit('close')
  } catch (err) {
    feedbackStore.pushToast('保存失败: ' + (err instanceof Error ? err.message : String(err)), 'error')
  } finally { isSaving.value = false }
}

async function loadSavedConfig() {
  try {
    const res = await calibrationApi.getConfig('total-temperature')
    const config = res.success && res.data ? applyCalibrationPrecisionDefaults(res.data) : null
    if (!config) return
    calibrationName.value = config.name
    if (config.totalTemperatureConfig?.machRange) pointLayout.value = { ...pointLayout.value, ...config.totalTemperatureConfig.machRange }
    config.probeChannels?.forEach(sc => { const ec = probeChannels.value.find(c => c.role ? c.role === sc.role : c.name === sc.name); if (ec) { ec.channel = { ...sc.channel }; ec.enabled = sc.enabled; ec.role = sc.role; ec.precision = sc.precision } })
    config.motionAxes?.forEach((sa, i) => { if (motionAxes.value[i]) motionAxes.value[i] = { ...sa } })
    dwellTimeMs.value = config.dwellTimeMs; samplesPerPoint.value = config.samplesPerPoint
    if (config.sphereTankGate) { sphereTankGateEnabled.value = config.sphereTankGate.enabled; sphereTankWaitTimeSec.value = config.sphereTankGate.waitTimeSec; sphereTankStableChannel.value = { ...config.sphereTankGate.stableTimeChannel } }
  } catch { /* ok */ }
}

onMounted(async () => {
  try {
    const results = await Promise.allSettled([deviceStore.refreshProfiles(), motionStore.refreshProfiles(), loadSavedConfig()])
    reportAllSettledFailures(
      results,
      ['设备列表', '运动控制器列表', '总温校准配置'],
      feedbackStore.pushToast,
    )
  }
  finally { isLoading.value = false }
})

const axisOptions = [{ label: 'X 轴', value: 'X' }, { label: 'Y 轴', value: 'Y' }, { label: 'Z 轴', value: 'Z' }, { label: 'U 轴', value: 'U' }]

// 通道索引枚举选项：UI 显示 CH1~CH18（1-based），内部 value 仍为数组索引 0~17
// 通道序号从 1 开始更符合操作员直觉，对应底层数组的 0-based 索引
const channelIndexOptions = Array.from({ length: 18 }, (_, i) => ({ label: `CH${i + 1}`, value: String(i) }))
</script>

<template>
  <UiDialog :show="true" title="总温探针校准配置" closable width="980px" @update:show="emit('close')">
    <UiSteps :current="currentStep" class="steps-mb">
      <UiStep v-for="(s, i) in steps" :key="i" :title="s" />
    </UiSteps>

    <UiSpin v-if="isLoading" :show="true" class="spinner" />

    <template v-else>
      <UiAlert v-if="currentStepErrors.length > 0" type="warning" title="请先修正以下问题" class="alert-error">{{ currentStepErrors[0] }}</UiAlert>

      <div v-if="currentStep === 0" class="step-content">
        <UiPanel class="section-card">
          <template #header><span class="section-header">配置名称</span></template>
          <UiInput v-model="calibrationName" placeholder="输入配置名称" />
        </UiPanel>
        <UiPanel class="section-card">
          <template #header><span class="section-header">点位布局</span></template>
          <span class="section-desc">Mach 数范围</span>
          <div class="mach-grid">
            <div><span class="field-label">最小值</span><UiInputNumber v-model="pointLayout.machMin" :step="0.1" style="width:100%" /></div>
            <div><span class="field-label">最大值</span><UiInputNumber v-model="pointLayout.machMax" :step="0.1" style="width:100%" /></div>
            <div><span class="field-label">步长</span><UiInputNumber v-model="pointLayout.machStep" :min="0.01" :step="0.1" style="width:100%" /></div>
          </div>
          <div class="point-summary">
            <span class="summary-label" style="font-size:var(--text-sm)">总校准点数</span>
            <span class="point-count">{{ pointCount }} 点</span>
          </div>
        </UiPanel>
        <UiPanel class="section-card">
          <template #header><span class="section-header">采集参数</span></template>
          <div class="param-grid">
            <div><span class="field-label">稳定等待时间 (毫秒)</span><UiInputNumber v-model="dwellTimeMs" :min="100" :step="100" style="width:100%" /><span class="hint-text">到达点位后等待稳定的时间</span></div>
            <div><span class="field-label">每点采样次数</span><UiInputNumber v-model="samplesPerPoint" :min="1" :max="1000" style="width:100%" /><span class="hint-text">每个点位采集的样本数量，取平均值</span></div>
          </div>
        </UiPanel>
      </div>

      <div v-if="currentStep === 1" class="step-content">
        <UiPanel class="section-card">
          <template #header><span class="section-header">测点通道映射</span></template>
          <div class="table-wrap">
            <table class="ntable"><thead><tr><th style="width:48px">启用</th><th>测点名称</th><th>数据源设备</th><th style="width:100px">通道索引</th><th style="width:80px">精度</th></tr></thead>
              <tbody>
                <tr v-for="ch in probeChannels" :key="ch.name">
                  <td class="cell-center"><UiCheckbox v-model:checked="ch.enabled" /></td>
                  <td><span class="cell-name">{{ ch.name }}</span></td>
                  <td><UiSelect v-model="ch.channel.deviceId" :options="deviceList.map(d => ({ label: `${d.name} (${d.type})`, value: d.id }))" placeholder="选择设备" style="min-width:140px" :disabled="!ch.enabled" :fallback="false" /></td>
                  <td><UiSelect
                    :model-value="ch.channel.channelIndex >= 0 ? String(ch.channel.channelIndex) : ''"
                    @update:model-value="ch.channel.channelIndex = $event !== '' ? Number($event) : -1"
                    :options="channelIndexOptions"
                    placeholder="未分配"
                    :disabled="!ch.enabled"
                  /></td>
                  <td><UiInputNumber v-model="ch.precision" :min="0" :max="8" style="width:100%" :disabled="!ch.enabled" /></td>
                </tr>
              </tbody>
            </table>
          </div>
        </UiPanel>
        <UiPanel class="section-card">
          <template #header><span class="section-header">运动轴配置</span></template>
          <div class="table-wrap">
            <table class="ntable"><thead><tr><th>坐标轴</th><th>运动控制器</th><th>物理轴</th></tr></thead>
              <tbody>
                <tr v-for="ax in motionAxes" :key="ax.name">
                  <td><UiStatusBadge status="connected">{{ ax.name }}</UiStatusBadge></td>
                  <td><UiSelect v-model="ax.controllerId" :options="motionControllerList.map(c => ({ label: `${c.name} (${c.type})`, value: c.id }))" placeholder="选择控制器" style="min-width:160px" /></td>
                  <td><UiSelect v-model="ax.axis" :options="axisOptions" style="width:100px" /></td>
                </tr>
              </tbody>
            </table>
          </div>
        </UiPanel>
        <UiPanel class="section-card">
          <template #header><span class="section-header">球罐稳定判定（PXI）</span></template>
          <UiCheckbox v-model:checked="sphereTankGateEnabled" class="checkbox-mb">启用球罐判定</UiCheckbox>
          <div v-if="sphereTankGateEnabled" class="sphere-grid">
            <div><span class="field-label">等待时间 (秒)</span><UiInputNumber v-model="sphereTankWaitTimeSec" :min="0" :step="0.1" style="width:100%" /></div>
            <div><span class="field-label">PXI 设备</span><UiSelect v-model="sphereTankStableChannel.deviceId" :options="deviceList.map(d => ({ label: `${d.name} (${d.type})`, value: d.id }))" placeholder="选择设备" style="width:100%" :fallback="false" /></div>
            <div><span class="field-label">稳定通道</span><UiSelect
                    :model-value="sphereTankStableChannel.channelIndex >= 0 ? String(sphereTankStableChannel.channelIndex) : ''"
                    @update:model-value="sphereTankStableChannel.channelIndex = $event !== '' ? Number($event) : 0"
                    :options="channelIndexOptions"
                    placeholder="选择通道"
                  /></div>
            <div class="angle-end"><span class="field-label">采集前需满足：稳定时间 >= 等待时间</span></div>
          </div>
        </UiPanel>
      </div>

      <div v-if="currentStep === 2" class="step-content">
        <UiPanel class="section-card">
          <template #header><span class="section-header">配置摘要</span></template>
          <div class="summary-grid">
            <div class="summary-row"><span class="summary-label">配置名称</span><span class="summary-value">{{ calibrationName }}</span></div>
            <div class="summary-row"><span class="summary-label">校准类型</span><span class="summary-value">总温探针</span></div>
            <div class="summary-row"><span class="summary-label">点位布局</span><span class="summary-value">Mach: {{ pointLayout.machMin }} ~ {{ pointLayout.machMax }}（步长 {{ pointLayout.machStep }}）</span></div>
            <div class="summary-row"><span class="summary-label">总点数</span><span class="accent-bold">{{ pointCount }} 点</span></div>
            <div class="summary-row"><span class="summary-label">启用测点</span><span class="summary-value">{{ probeChannels.filter(ch => ch.enabled).length }} 个</span></div>
            <div class="summary-row"><span class="summary-label">稳定时间</span><span class="summary-value">{{ dwellTimeMs }} ms</span></div>
            <div class="summary-row"><span class="summary-label">每点采样</span><span class="summary-value">{{ samplesPerPoint }} 次</span></div>
          </div>
        </UiPanel>
      </div>
    </template>

    <template #footer>
      <div class="footer-bar">
        <div><UiButton v-if="currentStep > 0" variant="secondary" size="sm" @click="prevStep">上一步</UiButton></div>
        <div class="footer-actions">
          <span class="step-indicator">步骤 {{ currentStep + 1 }} / {{ steps.length }}</span>
          <UiButton v-if="currentStep < steps.length - 1" variant="primary" size="sm" :disabled="!isStepValid" @click="nextStep">下一步</UiButton>
          <UiButton v-else size="sm" variant="primary" :loading="isSaving" :disabled="!isStepValid" @click="saveConfig">保存配置</UiButton>
        </div>
      </div>
    </template>
  </UiDialog>
</template>

<style scoped>
.step-content { display:flex; flex-direction:column; gap:var(--space-3); }
.section-card { font-size:var(--text-sm); }
.mach-grid { display:grid; grid-template-columns:repeat(3,1fr); gap:var(--space-2); margin-bottom:var(--space-3); }
.point-summary { display:flex; align-items:center; gap:var(--space-2); padding:10px; border-radius:var(--radius-md); border:1px solid color-mix(in srgb, var(--accent-primary) 20%, transparent); background:color-mix(in srgb, var(--accent-primary) 5%, transparent); }
.param-grid { display:grid; grid-template-columns:1fr 1fr; gap:10px; }
.table-wrap { overflow-x:auto; }
.ntable { width:100%; border-collapse:collapse; font-size:var(--text-sm); }
.ntable th { text-align:left; padding:var(--space-2) 10px; font-weight:600; font-size:var(--text-xs); color:var(--text-muted); background:var(--bg-panel-strong); border-bottom:1px solid var(--border-default); }
.ntable td { padding:6px 10px; border-bottom:1px solid var(--border-default); }
.ntable tbody tr:hover { background:color-mix(in srgb, var(--accent-primary) 3%, transparent); }
.sphere-grid { display:grid; grid-template-columns:1fr 1fr; gap:10px; }
.summary-grid { display:flex; flex-direction:column; }
.summary-row { display:flex; justify-content:space-between; align-items:center; padding:var(--space-2) 0; border-bottom:1px solid var(--border-default); gap:var(--space-3); font-size:var(--text-sm); }
.summary-row:last-child { border-bottom:none; }

.section-header { font-size:var(--text-sm); font-weight:600; color:var(--text-primary); }
.section-desc { font-size:var(--text-xs); display:block; margin-bottom:var(--space-2); color:var(--text-muted); }
.field-label { font-size:var(--text-xs); color:var(--text-muted); }
.hint-text { font-size:10px; display:block; margin-top:2px; color:var(--text-muted); }
.summary-label { color:var(--text-muted); }
.summary-value { color:var(--text-primary); }
.point-count { font-size:22px; font-weight:700; color:var(--accent-primary); }
.accent-bold { color:var(--accent-primary); font-weight:700; }
.checkbox-mb { margin-bottom:var(--space-2); }
.cell-center { text-align:center; }
.cell-name { font-size:var(--text-sm); color:var(--text-primary); }
.angle-end { display:flex; align-items:flex-end; }
.steps-mb { margin-bottom:var(--space-4); }
.spinner { display:flex; justify-content:center; padding:var(--space-10) 0; }
.alert-error { margin-bottom:var(--space-3); font-size:var(--text-sm); }
.footer-bar { display:flex; align-items:center; justify-content:space-between; width:100%; }
.footer-actions { display:flex; align-items:center; gap:var(--space-3); }
.step-indicator { font-size:var(--text-xs); color:var(--text-muted); }
</style>
